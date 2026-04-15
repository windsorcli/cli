package mirror

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// =============================================================================
// Test Setup
// =============================================================================

// stubImage is a minimal v1.Image used to record writes in copier tests. Only
// the methods exercised by Copier are implemented; others return zero values.
type stubImage struct {
	manifest *v1.Manifest
	layers   map[v1.Hash]v1.Layer
}

func (s *stubImage) Layers() ([]v1.Layer, error)       { return nil, nil }
func (s *stubImage) MediaType() (types.MediaType, error) {
	return types.DockerManifestSchema2, nil
}
func (s *stubImage) Size() (int64, error)              { return 0, nil }
func (s *stubImage) ConfigName() (v1.Hash, error)      { return v1.Hash{}, nil }
func (s *stubImage) ConfigFile() (*v1.ConfigFile, error) {
	return &v1.ConfigFile{}, nil
}
func (s *stubImage) RawConfigFile() ([]byte, error)    { return []byte("{}"), nil }
func (s *stubImage) Digest() (v1.Hash, error)          { return v1.Hash{}, nil }
func (s *stubImage) Manifest() (*v1.Manifest, error)   { return s.manifest, nil }
func (s *stubImage) RawManifest() ([]byte, error)      { return nil, nil }
func (s *stubImage) LayerByDigest(h v1.Hash) (v1.Layer, error) {
	if l, ok := s.layers[h]; ok {
		return l, nil
	}
	return nil, errors.New("not found")
}
func (s *stubImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { return nil, nil }

// stubLayer is a trivial v1.Layer.
type stubLayer struct{}

func (stubLayer) Digest() (v1.Hash, error)             { return v1.Hash{}, nil }
func (stubLayer) DiffID() (v1.Hash, error)             { return v1.Hash{}, nil }
func (stubLayer) Compressed() (io.ReadCloser, error)   { return nil, nil }
func (stubLayer) Uncompressed() (io.ReadCloser, error) { return nil, nil }
func (stubLayer) Size() (int64, error)                 { return 0, nil }
func (stubLayer) MediaType() (types.MediaType, error)  { return types.OCILayer, nil }

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCopier_CopyOCI(t *testing.T) {
	t.Run("InvokesCraneCopyWithRewrittenDest", func(t *testing.T) {
		// Given the destination registry does not yet have the artifact
		shims := NewShims()
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		shims.RemoteGet = func(r name.Reference, opts ...remote.Option) (*remote.Descriptor, error) {
			return nil, errors.New("not cached")
		}
		var copiedSrc, copiedDst string
		shims.CraneCopy = func(src, dst string) error {
			copiedSrc, copiedDst = src, dst
			return nil
		}

		c := NewCopier(shims, "localhost:5000")

		// When copying
		if err := c.CopyOCI("ghcr.io/example/foo:v1"); err != nil {
			t.Fatalf("CopyOCI: %v", err)
		}

		// Then crane copy is invoked with src and host-prefixed dst
		if copiedSrc != "ghcr.io/example/foo:v1" {
			t.Errorf("unexpected src %q", copiedSrc)
		}
		if !strings.HasPrefix(copiedDst, "localhost:5000/ghcr.io/example/foo") {
			t.Errorf("unexpected dst %q", copiedDst)
		}
	})

	t.Run("SkipsCopyWhenDestinationAlreadyHasArtifact", func(t *testing.T) {
		// Given the destination registry already has a manifest at dst
		shims := NewShims()
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		shims.RemoteGet = func(r name.Reference, opts ...remote.Option) (*remote.Descriptor, error) {
			return &remote.Descriptor{Descriptor: v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("b", 64)}}}, nil
		}
		var copyCalled bool
		shims.CraneCopy = func(src, dst string) error {
			copyCalled = true
			return nil
		}

		c := NewCopier(shims, "localhost:5000")
		if err := c.CopyOCI("ghcr.io/example/foo:v1"); err != nil {
			t.Fatalf("CopyOCI: %v", err)
		}
		if copyCalled {
			t.Error("expected crane copy to be skipped when destination already has the artifact")
		}
	})
}

func TestCopier_CopyHelmHTTPS(t *testing.T) {
	t.Run("FetchesWrapAndPushes", func(t *testing.T) {
		// Given an HTTP shim that returns index, chart, and no provenance
		index := `entries:
  widget:
    - name: widget
      version: 1.0.0
      apiVersion: v2
      urls: [https://cdn.example.com/widget-1.0.0.tgz]
`
		shims := NewShims()
		shims.YamlUnmarshal = yaml.Unmarshal
		shims.HttpGet = func(url string) (*http.Response, error) {
			switch {
			case strings.HasSuffix(url, "/index.yaml"):
				return newHTTPResponse(200, index), nil
			case strings.HasSuffix(url, ".tgz"):
				return newHTTPResponse(200, "chart-bytes"), nil
			case strings.HasSuffix(url, ".prov"):
				return newHTTPResponse(404, ""), nil
			}
			return newHTTPResponse(404, ""), nil
		}
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		shims.RemoteGet = func(r name.Reference, opts ...remote.Option) (*remote.Descriptor, error) {
			return nil, errors.New("not cached")
		}
		var wroteTo string
		shims.RemoteWriteLayer = func(repo name.Repository, layer v1.Layer, opts ...remote.Option) error {
			return nil
		}
		shims.RemoteWrite = func(r name.Reference, img v1.Image, opts ...remote.Option) error {
			wroteTo = r.Name()
			return nil
		}

		c := NewCopier(shims, "localhost:5000")

		// When copying
		err := c.CopyHelmHTTPS(HelmHTTPSEntry{
			Repository: "https://helm.example.com",
			ChartName:  "widget",
			Version:    "1.0.0",
		})

		// Then no error and the destination is under helm/<repo-host>/<chart>:<ver>
		if err != nil {
			t.Fatalf("CopyHelmHTTPS: %v", err)
		}
		if !strings.HasPrefix(wroteTo, "localhost:5000/helm/helm.example.com/widget") {
			t.Errorf("unexpected dest %q", wroteTo)
		}
	})
}
