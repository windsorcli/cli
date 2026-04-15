package mirror

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
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

// fakeLayer implements v1.Layer enough for resolver testing.
type fakeLayer struct {
	body []byte
}

func (l *fakeLayer) Digest() (v1.Hash, error)             { return v1.Hash{}, nil }
func (l *fakeLayer) DiffID() (v1.Hash, error)             { return v1.Hash{}, nil }
func (l *fakeLayer) Compressed() (io.ReadCloser, error)   { return io.NopCloser(bytes.NewReader(l.body)), nil }
func (l *fakeLayer) Uncompressed() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(l.body)), nil }
func (l *fakeLayer) Size() (int64, error)                 { return int64(len(l.body)), nil }
func (l *fakeLayer) MediaType() (types.MediaType, error)  { return types.OCILayer, nil }

// buildGzippedTar produces a gzip-compressed tar archive with the given
// name→content entries.
func buildGzippedTar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// newResolverShims returns shims that return a stubbed OCI image whose first
// layer, when uncompressed, is the provided tar.gz bytes.
func newResolverShims(payload []byte) *Shims {
	s := NewShims()
	s.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
		return name.ParseReference(ref, name.WeakValidation)
	}
	s.RemoteImage = func(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
		return nil, nil
	}
	s.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
		return []v1.Layer{&fakeLayer{body: payload}}, nil
	}
	s.LayerUncompressed = func(l v1.Layer) (io.ReadCloser, error) {
		return l.Uncompressed()
	}
	s.YamlUnmarshal = yaml.Unmarshal
	return s
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestResolver_Resolve(t *testing.T) {
	t.Run("WalksSourcesAndManifest", func(t *testing.T) {
		// Given a blueprint containing one oci source and a manifest with one
		// docker image and one HTTPS helm chart.
		bpYAML := []byte(`kind: Blueprint
metadata: {name: root}
sources:
  - name: child
    url: oci://ghcr.io/example/child:v1
`)
		childBpYAML := []byte(`kind: Blueprint
metadata: {name: child}
`)
		rootManifest := []byte(`version: v1alpha1
artifacts:
  - type: docker
    transport: oci
    reference: ghcr.io/foo/bar:v1
  - type: helm
    transport: oci
    repository: https://helm.example.com
    reference: widget
    version: 1.2.3
  - type: git-tag
    transport: git
    reference: some/cli
    version: v0.1.0
`)
		childManifest := []byte(`version: v1alpha1
artifacts:
  - type: docker
    transport: oci
    reference: alpine:3.21
`)

		rootTar := buildGzippedTar(t, map[string][]byte{
			"_template/blueprint.yaml": bpYAML,
			"artifact-manifest.yaml":   rootManifest,
		})
		childTar := buildGzippedTar(t, map[string][]byte{
			"_template/blueprint.yaml": childBpYAML,
			"artifact-manifest.yaml":   childManifest,
		})

		// Route each ref to its corresponding tar payload.
		payloadByRef := map[string][]byte{
			"ghcr.io/example/root:v1":  rootTar,
			"ghcr.io/example/child:v1": childTar,
		}
		shims := NewShims()
		shims.YamlUnmarshal = yaml.Unmarshal
		shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return name.ParseReference(ref, name.WeakValidation)
		}
		var currentRef string
		shims.RemoteImage = func(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
			currentRef = ref.Name()
			return nil, nil
		}
		shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			for k, v := range payloadByRef {
				if k == currentRef || currentRef == "index.docker.io/"+k || // unlikely
					currentRef == "ghcr.io/"+k[len("ghcr.io/"):] {
					return []v1.Layer{&fakeLayer{body: v}}, nil
				}
			}
			for k, v := range payloadByRef {
				if len(currentRef) > 0 && containsSubstr(currentRef, k) {
					return []v1.Layer{&fakeLayer{body: v}}, nil
				}
			}
			return nil, errors.New("no payload for " + currentRef)
		}
		shims.LayerUncompressed = func(l v1.Layer) (io.ReadCloser, error) { return l.Uncompressed() }

		r := NewResolver(shims)

		// When resolving from the root blueprint
		plan, err := r.Resolve([]string{"oci://ghcr.io/example/root:v1"})

		// Then the plan contains root+child blueprints, both docker images,
		// the helm HTTPS entry, and one skipped git-tag.
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if len(plan.Blueprints) != 2 {
			t.Errorf("expected 2 blueprints, got %d: %v", len(plan.Blueprints), plan.Blueprints)
		}
		if len(plan.DockerImages) != 2 {
			t.Errorf("expected 2 docker images, got %d: %v", len(plan.DockerImages), plan.DockerImages)
		}
		if len(plan.HelmHTTPS) != 1 {
			t.Errorf("expected 1 helm https entry, got %d", len(plan.HelmHTTPS))
		}
		if len(plan.Skipped) != 0 {
			t.Errorf("expected no skipped entries (non-OCI types now silently ignored), got %+v", plan.Skipped)
		}
	})

	t.Run("DedupesVisitedBlueprints", func(t *testing.T) {
		// Given two seeds that both reference the same blueprint ref
		payload := buildGzippedTar(t, map[string][]byte{})
		shims := newResolverShims(payload)

		r := NewResolver(shims)

		// When resolving with duplicate seeds
		plan, err := r.Resolve([]string{"oci://ghcr.io/x/y:v1", "oci://ghcr.io/x/y:v1"})

		// Then only one blueprint entry is recorded
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if len(plan.Blueprints) != 1 {
			t.Errorf("expected 1 dedeuplicated blueprint, got %d", len(plan.Blueprints))
		}
	})

	t.Run("IgnoresNonOCISeeds", func(t *testing.T) {
		// Given a seed list with only non-OCI URLs, no OCI lookups should occur
		r := NewResolver(NewShims())

		// When resolving
		plan, err := r.Resolve([]string{"https://example.com/foo.tgz"})

		// Then no blueprints are recorded and no error is returned
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if len(plan.Blueprints) != 0 {
			t.Errorf("expected 0 blueprints, got %d", len(plan.Blueprints))
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestResolver_canonicalDockerImage(t *testing.T) {
	t.Run("BareNameGetsLibrary", func(t *testing.T) {
		// Given a bare image name with tag
		// When canonicalised
		got := canonicalDockerImage("alpine:3.21")
		// Then docker.io/library is prefixed
		if want := "docker.io/library/alpine:3.21"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("UserImageGetsDockerIOPrefix", func(t *testing.T) {
		// Given a user/image reference
		got := canonicalDockerImage("bitnami/redis:7.2")
		// Then docker.io is prefixed (no library segment)
		if want := "docker.io/bitnami/redis:7.2"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("ExplicitHostLeftAlone", func(t *testing.T) {
		// Given a reference with an explicit host (detected by '.' in first segment)
		got := canonicalDockerImage("ghcr.io/foo/bar:v1")
		if want := "ghcr.io/foo/bar:v1"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("PortInHostLeftAlone", func(t *testing.T) {
		// Given a reference with port
		got := canonicalDockerImage("registry:5000/foo/bar:v1")
		if want := "registry:5000/foo/bar:v1"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("LocalhostLeftAlone", func(t *testing.T) {
		got := canonicalDockerImage("localhost/foo:v1")
		if want := "localhost/foo:v1"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("DigestReferencePreserved", func(t *testing.T) {
		got := canonicalDockerImage("alpine@sha256:abc")
		if want := "docker.io/library/alpine@sha256:abc"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("EmptyStringYieldsEmpty", func(t *testing.T) {
		if got := canonicalDockerImage(""); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestResolver_canonicalOCI(t *testing.T) {
	t.Run("StripsOCIPrefix", func(t *testing.T) {
		if got := canonicalOCI("oci://ghcr.io/foo/bar:v1"); got != "ghcr.io/foo/bar:v1" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("RejectsNonOCI", func(t *testing.T) {
		if got := canonicalOCI("https://example.com"); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestResolver_extractBlueprintArtifactFiles(t *testing.T) {
	t.Run("ReadsBothFilesWhenPresent", func(t *testing.T) {
		// Given a tar containing both files of interest
		bp := []byte(`kind: Blueprint
metadata: {name: x}
`)
		mf := []byte(`version: v1alpha1
artifacts: []
`)
		raw := buildGzippedTar(t, map[string][]byte{
			"_template/blueprint.yaml": bp,
			"artifact-manifest.yaml":   mf,
			"other.txt":                []byte("ignored"),
		})
		shims := NewShims()
		shims.YamlUnmarshal = yaml.Unmarshal

		// When extracting
		bpBytes, manifest, err := extractBlueprintArtifactFiles(raw, shims)

		// Then both files are returned and the manifest parses
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !bytes.Equal(bpBytes, bp) {
			t.Errorf("blueprint bytes mismatch")
		}
		if manifest == nil || manifest.Version != "v1alpha1" {
			t.Errorf("manifest not parsed: %+v", manifest)
		}
	})

	t.Run("TolerateMissingManifest", func(t *testing.T) {
		// Given a tar without a bundled manifest
		raw := buildGzippedTar(t, map[string][]byte{
			"_template/blueprint.yaml": []byte("kind: Blueprint\n"),
		})
		// When extracting
		bpBytes, manifest, err := extractBlueprintArtifactFiles(raw, NewShims())
		// Then manifest is nil but no error
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if len(bpBytes) == 0 {
			t.Errorf("expected blueprint yaml")
		}
		if manifest != nil {
			t.Errorf("expected nil manifest, got %+v", manifest)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// containsSubstr reports whether s contains sub.
func containsSubstr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

// indexOf is a minimal strings.Index replacement to avoid importing strings
// where only substring check is required.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
