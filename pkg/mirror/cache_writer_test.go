package mirror

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCacheWriter_WriteOCI(t *testing.T) {
	t.Run("WritesSinglePlatformImageToDisk", func(t *testing.T) {
		// Given a single-platform image with one layer
		tmpDir := t.TempDir()
		shims := NewShims()
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		shims.CraneManifest = func(ref string) ([]byte, error) {
			return []byte(`{"schemaVersion":2}`), nil
		}
		shims.RemoteGet = func(r name.Reference, opts ...remote.Option) (*remote.Descriptor, error) {
			return &remote.Descriptor{
				Descriptor: v1.Descriptor{
					MediaType: types.DockerManifestSchema2,
				},
			}, nil
		}
		shims.RemoteImage = func(r name.Reference, opts ...remote.Option) (v1.Image, error) {
			return &stubImage{
				manifest: &v1.Manifest{
					Layers: []v1.Descriptor{
						{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}},
					},
				},
				layers: map[v1.Hash]v1.Layer{
					{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}: stubLayer{},
				},
			}, nil
		}
		shims.MkdirAll = os.MkdirAll
		shims.WriteFile = os.WriteFile
		shims.Stat = os.Stat

		w := NewCacheWriter(shims, tmpDir)

		// When writing an OCI image
		if err := w.WriteOCI("docker.io/library/alpine:3.21"); err != nil {
			t.Fatalf("WriteOCI: %v", err)
		}

		// Then manifest reference file should exist
		refPath := filepath.Join(tmpDir, "manifests", "docker.io", "library/alpine", "reference", "3.21")
		if _, err := os.Stat(refPath); err != nil {
			t.Errorf("expected manifest reference at %s, got error: %v", refPath, err)
		}

		// And manifest digest file should exist
		digestDir := filepath.Join(tmpDir, "manifests", "docker.io", "library/alpine", "digest")
		entries, err := os.ReadDir(digestDir)
		if err != nil {
			t.Errorf("expected digest dir at %s, got error: %v", digestDir, err)
		}
		if len(entries) == 0 {
			t.Error("expected at least one digest entry")
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "sha256-") {
				t.Errorf("expected digest filename with sha256- prefix, got %q", e.Name())
			}
		}
	})

	t.Run("SkipsExistingBlobs", func(t *testing.T) {
		// Given a stat shim that always reports files exist
		shims := NewShims()
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		shims.CraneManifest = func(ref string) ([]byte, error) {
			return []byte(`{"schemaVersion":2}`), nil
		}
		shims.RemoteGet = func(r name.Reference, opts ...remote.Option) (*remote.Descriptor, error) {
			return &remote.Descriptor{
				Descriptor: v1.Descriptor{MediaType: types.DockerManifestSchema2},
			}, nil
		}
		shims.RemoteImage = func(r name.Reference, opts ...remote.Option) (v1.Image, error) {
			return &stubImage{
				manifest: &v1.Manifest{
					Layers: []v1.Descriptor{
						{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}},
					},
				},
				layers: map[v1.Hash]v1.Layer{
					{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}: stubLayer{},
				},
			}, nil
		}
		shims.MkdirAll = os.MkdirAll
		var writtenPaths []string
		shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPaths = append(writtenPaths, name)
			return os.WriteFile(name, data, perm)
		}
		// Stat always succeeds for blob paths to trigger skip
		shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "blob/") {
				return nil, nil
			}
			return os.Stat(name)
		}

		w := NewCacheWriter(shims, t.TempDir())
		if err := w.WriteOCI("docker.io/library/alpine:3.21"); err != nil {
			t.Fatalf("WriteOCI: %v", err)
		}

		// Then blobs should be skipped
		if w.Skipped.Load() == 0 {
			t.Error("expected skipped counter > 0 for existing blobs")
		}
		// And no blob files should have been written
		for _, p := range writtenPaths {
			if strings.Contains(p, "blob/") {
				t.Errorf("expected no blob writes, but wrote %q", p)
			}
		}
	})

	t.Run("ErrorsOnParseFailure", func(t *testing.T) {
		shims := NewShims()
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, errors.New("bad ref")
		}

		w := NewCacheWriter(shims, t.TempDir())
		if err := w.WriteOCI(":::bad"); err == nil {
			t.Error("expected error for bad reference")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestCacheWriter_rewriteRegistry(t *testing.T) {
	t.Run("DockerHubDefault", func(t *testing.T) {
		if got := rewriteRegistry(name.DefaultRegistry); got != "docker.io" {
			t.Errorf("got %q, want docker.io", got)
		}
	})
	t.Run("DockerHubIndex", func(t *testing.T) {
		if got := rewriteRegistry("index.docker.io"); got != "docker.io" {
			t.Errorf("got %q, want docker.io", got)
		}
	})
	t.Run("PortRewrite", func(t *testing.T) {
		if got := rewriteRegistry("registry:5000"); got != "registry_5000_" {
			t.Errorf("got %q, want registry_5000_", got)
		}
	})
	t.Run("PlainRegistry", func(t *testing.T) {
		if got := rewriteRegistry("ghcr.io"); got != "ghcr.io" {
			t.Errorf("got %q, want ghcr.io", got)
		}
	})
}

func TestCacheWriter_blobName(t *testing.T) {
	h := v1.Hash{Algorithm: "sha256", Hex: "abc123"}
	if got := blobName(h); got != "sha256-abc123" {
		t.Errorf("got %q, want sha256-abc123", got)
	}
}
