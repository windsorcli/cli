package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Setup
// =============================================================================

// setTestCliVersion temporarily overrides constants.Version for the duration of the calling test,
// restoring the original value via t.Cleanup. constants.Version is process-global, not per-test
// state — any test using this helper, and any sibling subtest in this package that reads
// constants.Version, MUST NOT run under t.Parallel(); a parallel subtest would race this mutation
// against concurrent reads (or another parallel mutation). This package has no t.Parallel() calls
// today; if one is ever added, audit for this helper's callers first.
func setTestCliVersion(t *testing.T, v string) {
	t.Helper()
	original := constants.Version
	constants.Version = v
	t.Cleanup(func() { constants.Version = original })
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArtifactBuilder_ParseOCIRef(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ValidOCIReference", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with valid OCI reference
		registry, repository, tag, err := builder.ParseOCIRef("oci://registry.example.com/my-repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the components should be parsed correctly
		if registry != "registry.example.com" {
			t.Errorf("expected registry 'registry.example.com', got %s", registry)
		}
		if repository != "my-repo" {
			t.Errorf("expected repository 'my-repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})

	t.Run("InvalidOCIPrefix", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with invalid prefix
		_, _, _, err := builder.ParseOCIRef("https://registry.example.com/my-repo:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format: https://registry.example.com/my-repo:v1.0.0"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}

		// And the error should carry the OCI reference format hint
		if we := werror.Find(err); we == nil || we.Remediation != ociRefFormatHint {
			t.Errorf("expected the OCI reference format hint, got %v", we)
		}
	})

	t.Run("MissingTag", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with missing tag
		_, _, _, err := builder.ParseOCIRef("oci://registry.example.com/my-repo")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format, expected registry/repository:tag: oci://registry.example.com/my-repo"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MissingRepository", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with missing repository
		_, _, _, err := builder.ParseOCIRef("oci://registry.example.com:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format, expected registry/repository:tag: oci://registry.example.com:v1.0.0"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NestedRepositoryPath", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with nested repository path
		registry, repository, tag, err := builder.ParseOCIRef("oci://registry.example.com/organization/my-repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the components should be parsed correctly
		if registry != "registry.example.com" {
			t.Errorf("expected registry 'registry.example.com', got %s", registry)
		}
		if repository != "organization/my-repo" {
			t.Errorf("expected repository 'organization/my-repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})

	t.Run("RegistryWithPort", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with a registry host that includes an explicit port
		registry, repository, tag, err := builder.ParseOCIRef("oci://localhost:5050/my-repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the port is retained as part of the registry, not mistaken for the tag separator
		if registry != "localhost:5050" {
			t.Errorf("expected registry 'localhost:5050', got %s", registry)
		}
		if repository != "my-repo" {
			t.Errorf("expected repository 'my-repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})

	t.Run("RegistryWithPortAndNestedRepositoryPath", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with both a port and a multi-segment repository path
		registry, repository, tag, err := builder.ParseOCIRef("oci://localhost:5050/org/project/repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the port and full nested path are both parsed correctly
		if registry != "localhost:5050" {
			t.Errorf("expected registry 'localhost:5050', got %s", registry)
		}
		if repository != "org/project/repo" {
			t.Errorf("expected repository 'org/project/repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})

	t.Run("DigestReference", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When ParseOCIRef is called with an "@sha256:" digest instead of a tag
		registry, repository, tag, err := builder.ParseOCIRef("oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the digest is not mistaken for a tag containing a colon
		if registry != "ghcr.io" {
			t.Errorf("expected registry 'ghcr.io', got %s", registry)
		}
		if repository != "windsorcli/core" {
			t.Errorf("expected repository 'windsorcli/core', got %s", repository)
		}
		if tag != "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234" {
			t.Errorf("expected tag 'sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234', got %s", tag)
		}
	})
}

func TestArtifactBuilder_ListTags(t *testing.T) {
	setup := func(t *testing.T) *ArtifactBuilder {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder
	}

	t.Run("ReturnsTagsForRepository", func(t *testing.T) {
		// Given a registry that lists two tags for the repository
		builder := setup(t)
		var listedRepo string
		builder.shims.RemoteList = func(repo name.Repository, options ...remote.Option) ([]string, error) {
			listedRepo = repo.String()
			return []string{"v0.5.0", "v0.6.0"}, nil
		}

		// When listing tags for an OCI reference (the tag in the ref is ignored)
		tags, err := builder.ListTags("oci://ghcr.io/windsorcli/core:v0.6.0")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it lists the repository's tags
		if !strings.Contains(listedRepo, "ghcr.io/windsorcli/core") {
			t.Errorf("Expected the repository to be listed, got %q", listedRepo)
		}
		if len(tags) != 2 || tags[1] != "v0.6.0" {
			t.Errorf("Expected [v0.5.0 v0.6.0], got %v", tags)
		}
	})

	t.Run("ErrorsOnMalformedReference", func(t *testing.T) {
		builder := setup(t)
		if _, err := builder.ListTags("not-an-oci-ref"); err == nil {
			t.Error("Expected an error for a malformed OCI reference")
		}
	})

	t.Run("PropagatesRegistryError", func(t *testing.T) {
		builder := setup(t)
		builder.shims.RemoteList = func(repo name.Repository, options ...remote.Option) ([]string, error) {
			return nil, fmt.Errorf("registry down")
		}
		if _, err := builder.ListTags("oci://ghcr.io/windsorcli/core:v0.6.0"); err == nil {
			t.Error("Expected the registry error to surface")
		}
	})
}

func TestArtifactBuilder_GetCliVersionConstraint(t *testing.T) {
	setup := func(t *testing.T) *ArtifactBuilder {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder
	}

	t.Run("ReturnsDeclaredConstraint", func(t *testing.T) {
		// Given a registry whose manifest carries the cli-version annotation. ImageLayers is
		// deliberately left unset (nil) — a call to it panics, proving GetCliVersionConstraint
		// never touches the layer blob the way a full Pull would.
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImageWithManifest{
				manifestFunc: func() (*v1.Manifest, error) {
					return &v1.Manifest{Annotations: map[string]string{CliVersionAnnotationKey: ">=0.9.0"}}, nil
				},
			}, nil
		}

		// When reading the constraint for a candidate tag
		constraint, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.9.2")

		// Then it returns the declared constraint, manifest-only
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if constraint != ">=0.9.0" {
			t.Errorf("Expected constraint >=0.9.0, got %q", constraint)
		}
	})

	t.Run("ReturnsEmptyWhenAnnotationAbsent", func(t *testing.T) {
		// Given a manifest with no cli-version annotation (an older tag published before this
		// annotation existed, or an artifact that never declared cliVersion)
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImageWithManifest{
				manifestFunc: func() (*v1.Manifest, error) {
					return &v1.Manifest{Annotations: map[string]string{}}, nil
				},
			}, nil
		}

		// When reading the constraint
		constraint, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.5.0")

		// Then it returns empty, not an error — unconstrained, same as ValidateCliVersion treats
		// an empty constraint
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if constraint != "" {
			t.Errorf("Expected empty constraint, got %q", constraint)
		}
	})

	t.Run("ErrorsOnMalformedReference", func(t *testing.T) {
		builder := setup(t)
		if _, err := builder.GetCliVersionConstraint("not-an-oci-ref"); err == nil {
			t.Error("Expected an error for a malformed OCI reference")
		}
	})

	t.Run("ErrorWhenParseReferenceFails", func(t *testing.T) {
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, fmt.Errorf("parse error")
		}

		_, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.9.2")
		if err == nil || !strings.Contains(err.Error(), "failed to parse reference") {
			t.Errorf("Expected parse reference error, got %v", err)
		}
	})

	t.Run("ErrorWhenRemoteImageFails", func(t *testing.T) {
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, fmt.Errorf("remote error")
		}

		_, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.9.2")
		if err == nil || !strings.Contains(err.Error(), "failed to get image manifest") {
			t.Errorf("Expected image manifest error, got %v", err)
		}
	})

	t.Run("ErrorWhenManifestReadFails", func(t *testing.T) {
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImageWithManifest{
				manifestFunc: func() (*v1.Manifest, error) {
					return nil, fmt.Errorf("manifest read error")
				},
			}, nil
		}

		_, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.9.2")
		if err == nil || !strings.Contains(err.Error(), "failed to read manifest") {
			t.Errorf("Expected manifest read error, got %v", err)
		}
	})

	t.Run("RetriesAnonymouslyWhenKeychainRejectedByRegistry", func(t *testing.T) {
		// Given a registry that rejects keychain credentials but accepts anonymous access,
		// mirroring downloadOCIArtifact's identical fallback behavior
		builder := setup(t)
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		callCount := 0
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			callCount++
			if callCount == 1 {
				return nil, &transport.Error{StatusCode: http.StatusForbidden, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}
			}
			return &mockImageWithManifest{
				manifestFunc: func() (*v1.Manifest, error) {
					return &v1.Manifest{Annotations: map[string]string{CliVersionAnnotationKey: ">=0.9.0"}}, nil
				},
			}, nil
		}

		constraint, err := builder.GetCliVersionConstraint("oci://ghcr.io/windsorcli/core:v0.9.2")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if constraint != ">=0.9.0" {
			t.Errorf("Expected constraint >=0.9.0, got %q", constraint)
		}
		if callCount != 2 {
			t.Errorf("Expected 2 RemoteImage calls (keychain then anonymous), got %d", callCount)
		}
	})
}

func TestArtifactBuilder_VerifyCliVersionCompatibility(t *testing.T) {
	setup := func(t *testing.T, metadataYAML []byte) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, nil
		}
		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return []v1.Layer{&mockLayer{}}, nil
		}

		files := map[string][]byte{"_template/test.yaml": []byte("test content")}
		if metadataYAML != nil {
			files["metadata.yaml"] = metadataYAML
		}
		testTarData := createTestTarGz(t, files)
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}
		mocks.Shims.ReadAll = io.ReadAll

		mocks.Shims.Stat = os.Stat
		mocks.Shims.ReadFile = os.ReadFile
		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
				return nil, err
			}
			return os.Create(name)
		}
		// setupDefaultShims's YamlUnmarshal stub only ever sets Name/Version, ignoring actual
		// content — VerifyCliVersionCompatibility needs the real cliVersion field, so parse for
		// real here.
		mocks.Shims.YamlUnmarshal = yaml.Unmarshal

		return builder, mocks
	}

	t.Run("ReturnsNilWhenConstraintSatisfied", func(t *testing.T) {
		setTestCliVersion(t, "1.0.0")
		builder, _ := setup(t, []byte("name: test\nversion: v1.0.0\ncliVersion: \">=0.9.0\"\n"))

		err := builder.VerifyCliVersionCompatibility("oci://registry.example.com/my-repo:v1.0.0")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConstraintNotSatisfied", func(t *testing.T) {
		setTestCliVersion(t, "0.1.0")
		builder, _ := setup(t, []byte("name: test\nversion: v1.0.0\ncliVersion: \">=0.9.0\"\n"))

		err := builder.VerifyCliVersionCompatibility("oci://registry.example.com/my-repo:v1.0.0")

		if err == nil {
			t.Error("Expected an error for an unsatisfied constraint")
		}
	})

	t.Run("ReturnsNilWhenNoMetadataFile", func(t *testing.T) {
		builder, _ := setup(t, nil)

		err := builder.VerifyCliVersionCompatibility("oci://registry.example.com/my-repo:v1.0.0")

		if err != nil {
			t.Errorf("Expected no error when metadata.yaml is absent, got %v", err)
		}
	})

	t.Run("ReturnsNilWhenCliVersionFieldAbsent", func(t *testing.T) {
		builder, _ := setup(t, []byte("name: test\nversion: v1.0.0\n"))

		err := builder.VerifyCliVersionCompatibility("oci://registry.example.com/my-repo:v1.0.0")

		if err != nil {
			t.Errorf("Expected no error when cliVersion is undeclared, got %v", err)
		}
	})

	t.Run("PropagatesPullError", func(t *testing.T) {
		builder, mocks := setup(t, []byte("name: test\nversion: v1.0.0\n"))
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, fmt.Errorf("registry unreachable")
		}

		err := builder.VerifyCliVersionCompatibility("oci://registry.example.com/my-repo:v1.0.0")

		if err == nil || !strings.Contains(err.Error(), "failed to pull") {
			t.Errorf("Expected a wrapped pull error, got %v", err)
		}
	})
}

func TestArtifactBuilder_Push(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ErrorWithInvalidTagFormat", func(t *testing.T) {
		// Given a builder
		builder, mocks := setup(t)

		// Create an artifact first
		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			return os.Create(name)
		}
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "test.tar.gz")
		_, err := builder.Write(outputFile, "")
		if err != nil {
			t.Fatalf("Failed to create artifact: %v", err)
		}

		// When pushing with invalid tag format (only colon-separated tags that are malformed)
		invalidTags := []string{
			"only:colon:",     // has colon but empty version
			":missingname",    // has colon but empty name
			"missingversion:", // has colon but empty version
		}

		for _, tag := range invalidTags {
			err := builder.Push("registry.example.com", "test", tag)
			if err == nil || !strings.Contains(err.Error(), "failed to parse repository reference") {
				t.Errorf("Expected repository reference error for %s, got: %v", tag, err)
			}
		}
	})

	t.Run("ErrorWhenNameMissing", func(t *testing.T) {
		// Given a builder with no metadata and no repoName provided (simulating Create method usage)
		builder, mocks := setup(t)

		// Set up mocks for metadata parsing that returns empty metadata
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			// Return empty metadata (no name or version)
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte(""), 0644)

		// When pushing with empty repoName (simulates Create method scenario)
		err := builder.Push("registry.example.com", "", "")
		if err == nil || !strings.Contains(err.Error(), "name is required") {
			t.Errorf("Expected name required error, got: %v", err)
		}
	})

	t.Run("ErrorWhenVersionMissing", func(t *testing.T) {
		// Given a builder with incomplete metadata (name but no version)
		builder, mocks := setup(t)

		// Set up mocks for metadata parsing that returns only name
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			// No version set
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte("name: test"), 0644)

		// When pushing without providing tag
		err := builder.Push("registry.example.com", "test", "")
		if err == nil || !strings.Contains(err.Error(), "version is required") {
			t.Errorf("Expected version required error, got: %v", err)
		}
	})

	t.Run("SuccessWithValidMetadata", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up custom AppendLayers behavior for testing
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("mock implementation error")
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "myapp", "2.0.0")

		// Then should get mock implementation error (indicating it reached the OCI creation step)
		if err == nil || !strings.Contains(err.Error(), "mock implementation error") {
			t.Errorf("Expected mock implementation error, got: %v", err)
		}
	})

	t.Run("PropagatesCliVersionConstraintToManifestAnnotation", func(t *testing.T) {
		// Given a builder whose _template/metadata.yaml declares a cliVersion constraint. The
		// package default YamlUnmarshal stub (setupDefaultShims) ignores file content and only
		// ever sets Name/Version, so CliVersion needs its own override here.
		builder, mocks := setup(t)

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			input.CliVersion = ">=0.9.0"
			return nil
		}

		mockImg := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImg }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImg }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImg }

		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return &mockImageWithManifest{
				manifestFunc: func() (*v1.Manifest, error) {
					return nil, fmt.Errorf("stop after annotations captured")
				},
			}
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0\ncliVersion: \">=0.9.0\""), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "myapp", "2.0.0")

		// Then execution reached the manifest step (proving OCI image creation completed) and the
		// constraint declared in metadata.yaml was mirrored into the manifest annotation
		if err == nil || !strings.Contains(err.Error(), "stop after annotations captured") {
			t.Fatalf("Expected to reach manifest step, got: %v", err)
		}
		if capturedAnnotations[CliVersionAnnotationKey] != ">=0.9.0" {
			t.Errorf("Expected %s annotation %q, got %q", CliVersionAnnotationKey, ">=0.9.0", capturedAnnotations[CliVersionAnnotationKey])
		}
	})

	t.Run("SuccessWithInMemoryOperation", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Mock the tarball creation to verify in-memory operation
		tarballCreated := false
		mocks.Shims.NewGzipWriter = func(w io.Writer) *gzip.Writer {
			tarballCreated = true
			return gzip.NewWriter(w)
		}

		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			// Verify that we got here with in-memory content
			if !tarballCreated {
				return nil, fmt.Errorf("tarball was not created in memory")
			}
			return nil, fmt.Errorf("expected test termination")
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
		builder.addFile("test.txt", []byte("test content"), 0644)

		// When pushing (this should work entirely in-memory)
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get expected test termination (proving in-memory operation worked)
		if err == nil || !strings.Contains(err.Error(), "expected test termination") {
			t.Errorf("Expected test termination error, got: %v", err)
		}

		// And tarball should have been created in memory
		if !tarballCreated {
			t.Error("Expected tarball to be created in memory")
		}
	})

	t.Run("ErrorFromCreateTarballInMemory", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Mock tar writer to fail on WriteHeader
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return fmt.Errorf("tar writer header failed")
				},
			}
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get tarball creation error
		if err == nil || !strings.Contains(err.Error(), "failed to create tarball in memory") {
			t.Errorf("Expected tarball creation error, got: %v", err)
		}
	})

	t.Run("ErrorWithInvalidRepositoryReference", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing with invalid registry format (contains invalid characters)
		err := builder.Push("invalid registry format with spaces", "test", "1.0.0")

		// Then should get repository reference error
		if err == nil || !strings.Contains(err.Error(), "failed to parse repository reference") {
			t.Errorf("Expected invalid repository reference error, got: %v", err)
		}
	})

	t.Run("ErrorFromCreateOCIArtifactImage", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}
		mocks.Shims.YamlMarshal = func(data any) ([]byte, error) {
			return []byte("test: yaml"), nil
		}

		// Mock AppendLayers to fail
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("config file mutation failed")
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get OCI image creation error
		if err == nil || !strings.Contains(err.Error(), "failed to create OCI image") {
			t.Errorf("Expected OCI image creation error, got: %v", err)
		}
	})

	t.Run("SuccessWithEmptyTag", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up standard mocks
		mocks.Shims = setupDefaultShims()
		builder.shims = mocks.Shims

		// Mock that terminates early to avoid nil pointer issues
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("expected test termination")
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing with empty tag (should use version from metadata)
		err := builder.Push("registry.example.com", "test", "")

		// Then should get expected test termination (verifying it gets to image creation step)
		if err == nil || !strings.Contains(err.Error(), "expected test termination") {
			t.Errorf("Expected test termination error, got: %v", err)
		}
	})

	t.Run("ErrorFromImageManifest", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up standard mocks with custom manifest behavior
		mocks.Shims = setupDefaultShims()
		builder.shims = mocks.Shims // Update builder to use new shims

		// Mock successful image creation but failing manifest
		mockImg := &mockImageWithManifest{
			manifestFunc: func() (*v1.Manifest, error) {
				return nil, fmt.Errorf("manifest generation failed")
			},
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
			return mockImg
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get manifest error
		if err == nil || !strings.Contains(err.Error(), "failed to get image manifest") {
			t.Errorf("Expected manifest error, got: %v", err)
		}
	})

	t.Run("ErrorFromLayerByDigest", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up standard mocks with custom layer behavior
		mocks.Shims = setupDefaultShims()
		builder.shims = mocks.Shims // Update builder to use new shims

		// Mock image with manifest but failing layer access
		mockImg := &mockImageWithManifest{
			manifestFunc: func() (*v1.Manifest, error) {
				return &v1.Manifest{
					Layers: []v1.Descriptor{
						{
							Digest: v1.Hash{
								Algorithm: "sha256",
								Hex:       "abc123",
							},
							Size: 1000,
						},
					},
				}, nil
			},
			layerByDigestFunc: func(hash v1.Hash) (v1.Layer, error) {
				return nil, fmt.Errorf("layer access failed")
			},
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
			return mockImg
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get layer access error
		if err == nil || !strings.Contains(err.Error(), "failed to get layer") {
			t.Errorf("Expected layer access error, got: %v", err)
		}
	})

	t.Run("ErrorFromConfigName", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up standard mocks with custom config behavior
		mocks.Shims = setupDefaultShims()
		builder.shims = mocks.Shims // Update builder to use new shims

		// Mock image with empty manifest but failing config name
		mockImg := &mockImageWithManifest{
			manifestFunc: func() (*v1.Manifest, error) {
				return &v1.Manifest{
					Layers: []v1.Descriptor{}, // Empty layers to skip layer upload
				}, nil
			},
			configNameFunc: func() (v1.Hash, error) {
				return v1.Hash{}, fmt.Errorf("config name failed")
			},
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
			return mockImg
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get config digest error
		if err == nil || !strings.Contains(err.Error(), "failed to get config digest") {
			t.Errorf("Expected config digest error, got: %v", err)
		}
	})

	t.Run("ErrorFromRawConfigFile", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up standard mocks with custom config file behavior
		mocks.Shims = setupDefaultShims()
		builder.shims = mocks.Shims // Update builder to use new shims

		// Mock image with successful config name but failing raw config
		mockImg := &mockImageWithManifest{
			manifestFunc: func() (*v1.Manifest, error) {
				return &v1.Manifest{
					Layers: []v1.Descriptor{}, // Empty layers to skip layer upload
				}, nil
			},
			configNameFunc: func() (v1.Hash, error) {
				return v1.Hash{
					Algorithm: "sha256",
					Hex:       "def456",
				}, nil
			},
			rawConfigFileFunc: func() ([]byte, error) {
				return nil, fmt.Errorf("raw config failed")
			},
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
			return mockImg
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing (assuming remote.Get will fail for config, triggering upload path)
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then should get config file error
		if err == nil || !strings.Contains(err.Error(), "failed to get config file") {
			t.Errorf("Expected config file error, got: %v", err)
		}
	})

	// Edge Cases for parseTagAndResolveMetadata Coverage
	t.Run("EdgeCaseWithMultipleColonTag", func(t *testing.T) {
		// Given a builder with metadata
		builder, mocks := setup(t)

		// Set up YAML unmarshal to return empty values
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			// Return empty input so tag parsing is tested
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte(""), 0644)

		// When creating with tag containing multiple colons (should fail in Create method)
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "test.tar.gz")
		_, err := builder.Write(outputFile, "name:version:extra")

		// Then should get tag format error
		if err == nil || !strings.Contains(err.Error(), "tag must be in format 'name:version'") {
			t.Errorf("Expected tag format error, got: %v", err)
		}
	})

	t.Run("EdgeCaseWithEmptyTagParts", func(t *testing.T) {
		// Given a builder with metadata
		builder, mocks := setup(t)

		// Set up YAML unmarshal to return empty values
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte(""), 0644)

		// When creating with tag having empty parts (should fail in Create method)
		invalidTags := []string{":version", "name:", ":"}
		for _, tag := range invalidTags {
			tmpDir := t.TempDir()
			outputFile := filepath.Join(tmpDir, "test.tar.gz")
			_, err := builder.Write(outputFile, tag)
			if err == nil || !strings.Contains(err.Error(), "tag must be in format 'name:version'") {
				t.Errorf("Expected tag format error for '%s', got: %v", tag, err)
			}
		}
	})

	t.Run("EdgeCaseWithRepoNameFallback", func(t *testing.T) {
		// Given a builder with metadata that has no name
		builder, mocks := setup(t)

		// Set up YAML unmarshal to return metadata without name
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			// No name set, should fall back to repoName
			input.Version = "1.0.0"
			return nil
		}
		builder.addFile("_template/metadata.yaml", []byte("version: 1.0.0"), 0644)

		// Mock to terminate early after metadata resolution
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("test termination - repoName fallback worked")
		}

		// When pushing (should use repoName as name fallback)
		err := builder.Push("registry.example.com", "fallback-name", "2.0.0")

		// Then should reach AppendLayers (proving repoName fallback worked)
		if err == nil || !strings.Contains(err.Error(), "test termination - repoName fallback worked") {
			t.Errorf("Expected repoName fallback to work, got: %v", err)
		}
	})

	// Push Method Additional Coverage Tests
	t.Run("SuccessPathWithEmptyTagName", func(t *testing.T) {
		// Given a builder with valid metadata but no tag
		builder, mocks := setup(t)

		// Mock to terminate early after URL construction
		mockImg := &mockImageWithManifest{
			manifestFunc: func() (*v1.Manifest, error) {
				// This tests the empty tagName path (repoURL without tag)
				return nil, fmt.Errorf("test termination after URL construction")
			},
		}

		// Override image creation to return our mock
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImg, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
			return mockImg
		}
		mocks.Shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
			return mockImg
		}

		builder.addFile("_template/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing with empty tag (should construct URL without tag)
		err := builder.Push("registry.example.com", "test", "")

		// Then should get expected test termination
		if err == nil || !strings.Contains(err.Error(), "test termination after URL construction") {
			t.Errorf("Expected URL construction termination, got: %v", err)
		}
	})

	t.Run("ErrorFromRemoteWriteLayer", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, mocks := setup(t)

		// And RemoteWriteLayer fails
		mocks.Shims.RemoteWriteLayer = func(repo name.Repository, layer v1.Layer, options ...remote.Option) error {
			return fmt.Errorf("layer upload failed")
		}

		builder.addFile("file.txt", []byte("content"), 0644)

		// When calling Push
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from layer upload failure")
		}
		if !strings.Contains(err.Error(), "failed to upload layer") {
			t.Errorf("Expected layer upload error, got: %v", err)
		}
	})

	t.Run("ErrorFromRemoteWrite", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, mocks := setup(t)

		// And RemoteWrite fails
		mocks.Shims.RemoteWrite = func(ref name.Reference, img v1.Image, options ...remote.Option) error {
			return fmt.Errorf("manifest upload failed")
		}

		builder.addFile("file.txt", []byte("content"), 0644)

		// When calling Push
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from manifest upload failure")
		}
		if !strings.Contains(err.Error(), "failed to push artifact to registry") {
			t.Errorf("Expected manifest upload error, got: %v", err)
		}
	})

	t.Run("SuccessWithBlobsExisting", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, mocks := setup(t)

		// And RemoteGet succeeds (blobs exist)
		mocks.Shims.RemoteGet = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
			return &remote.Descriptor{}, nil
		}

		builder.addFile("file.txt", []byte("content"), 0644)

		// When calling Push
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("SuccessWithBlobsNotExisting", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, mocks := setup(t)

		// And RemoteGet fails (blobs don't exist, need upload)
		mocks.Shims.RemoteGet = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
			return nil, fmt.Errorf("blob not found")
		}
		// And RemoteWriteLayer succeeds
		mocks.Shims.RemoteWriteLayer = func(repo name.Repository, layer v1.Layer, options ...remote.Option) error {
			return nil
		}

		builder.addFile("file.txt", []byte("content"), 0644)

		// When calling Push
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("ErrorFromConfigUpload", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, mocks := setup(t)

		// And config blob doesn't exist but upload fails
		callCount := 0
		mocks.Shims.RemoteGet = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
			return nil, fmt.Errorf("blob not found")
		}
		mocks.Shims.RemoteWriteLayer = func(repo name.Repository, layer v1.Layer, options ...remote.Option) error {
			callCount++
			if callCount == 1 {
				// First call (layer upload) succeeds
				return nil
			}
			// Second call (config upload) fails
			return fmt.Errorf("config upload failed")
		}

		builder.addFile("file.txt", []byte("content"), 0644)

		// When calling Push
		err := builder.Push("registry.example.com", "test", "1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from config upload failure")
		}
		if !strings.Contains(err.Error(), "failed to upload config") {
			t.Errorf("Expected config upload error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArtifactBuilder_createOCIArtifactImage(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithValidLayer", func(t *testing.T) {
		// Given a builder with successful shim operations
		builder, mocks := setup(t)

		// Mock git provenance to return test data
		expectedCommitSHA := "abc123def456"
		expectedRemoteURL := "https://github.com/user/repo.git"
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			if strings.Contains(cmd, "git rev-parse HEAD") {
				return expectedCommitSHA, nil
			}
			if strings.Contains(cmd, "git config --get remote.origin.url") {
				return expectedRemoteURL, nil
			}
			return "", nil
		}

		// Mock successful image creation
		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			if cfg.Architecture != "amd64" {
				return nil, fmt.Errorf("expected amd64 architecture, got %s", cfg.Architecture)
			}
			if cfg.OS != "linux" {
				return nil, fmt.Errorf("expected linux OS, got %s", cfg.OS)
			}
			if cfg.Config.Labels["org.opencontainers.image.title"] != "test-repo" {
				return nil, fmt.Errorf("expected title label to be test-repo")
			}
			return mockImage, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			if anns["org.opencontainers.image.revision"] != expectedCommitSHA {
				t.Errorf("Expected revision %s, got %s", expectedCommitSHA, anns["org.opencontainers.image.revision"])
			}
			if anns["org.opencontainers.image.source"] != expectedRemoteURL {
				t.Errorf("Expected source %s, got %s", expectedRemoteURL, anns["org.opencontainers.image.source"])
			}
			return mockImage
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", "")

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if img == nil {
			t.Error("Expected non-nil image")
		}
	})

	t.Run("SetsCliVersionAnnotationWhenDeclared", func(t *testing.T) {
		// Given a builder pushing an artifact whose _template/metadata.yaml declares cliVersion
		builder, mocks := setup(t)

		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }

		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return mockImage
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image with a declared constraint
		if _, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", ">=0.9.0"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the manifest annotation mirrors the same constraint metadata.yaml carries
		if capturedAnnotations[CliVersionAnnotationKey] != ">=0.9.0" {
			t.Errorf("Expected %s annotation %q, got %q", CliVersionAnnotationKey, ">=0.9.0", capturedAnnotations[CliVersionAnnotationKey])
		}
	})

	t.Run("OmitsCliVersionAnnotationWhenNotDeclared", func(t *testing.T) {
		// Given a builder pushing an artifact with no cliVersion declared
		builder, mocks := setup(t)

		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }

		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return mockImage
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image with an empty constraint
		if _, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", ""); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no cli-version annotation key is written at all
		if _, ok := capturedAnnotations[CliVersionAnnotationKey]; ok {
			t.Errorf("Expected no %s annotation when cliVersion is undeclared, got %q", CliVersionAnnotationKey, capturedAnnotations[CliVersionAnnotationKey])
		}
	})

	t.Run("ErrorWhenAppendLayersFails", func(t *testing.T) {
		// Given a builder with failing AppendLayers
		builder, mocks := setup(t)

		// Mock git provenance to succeed but AppendLayers to fail
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			if strings.Contains(cmd, "git rev-parse HEAD") {
				return "abc123", nil
			}
			return "", nil
		}

		mocks.Shims.EmptyImage = func() v1.Image { return &mockImage{} }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("append layers failed")
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image
		_, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", "")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when AppendLayers fails")
		}
		if !strings.Contains(err.Error(), "failed to append layer to image") {
			t.Errorf("Expected error to contain 'failed to append layer to image', got %v", err)
		}
	})

	t.Run("ErrorWhenConfigFileFails", func(t *testing.T) {
		// Given a builder with failing ConfigFile
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}

		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return nil, fmt.Errorf("config file failed")
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image
		_, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", "")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when ConfigFile fails")
		}
		if !strings.Contains(err.Error(), "failed to set config file") {
			t.Errorf("Expected error to contain 'failed to set config file', got %v", err)
		}
	})

	t.Run("SuccessWithGitProvenanceFallback", func(t *testing.T) {
		// Given a builder where git provenance fails
		builder, mocks := setup(t)

		// Mock git provenance to fail
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("git command failed")
		}

		// Mock successful image creation
		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }

		// Capture annotations to verify fallback revision and source
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			if anns["org.opencontainers.image.revision"] != "unknown" {
				t.Errorf("Expected revision 'unknown', got %s", anns["org.opencontainers.image.revision"])
			}
			if anns["org.opencontainers.image.source"] != "unknown" {
				t.Errorf("Expected source 'unknown', got %s", anns["org.opencontainers.image.source"])
			}
			return mockImage
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", "")

		// Then should succeed with fallback values
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if img == nil {
			t.Error("Expected non-nil image")
		}
	})

	t.Run("SuccessWithEmptyCommitSHA", func(t *testing.T) {
		// Given a builder where git returns empty commit SHA but valid remote URL
		builder, mocks := setup(t)

		expectedRemoteURL := "https://github.com/user/empty-sha-repo.git"
		// Mock git provenance to return empty commit SHA but valid remote URL
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			if strings.Contains(cmd, "git rev-parse HEAD") {
				return "   ", nil
			}
			if strings.Contains(cmd, "git config --get remote.origin.url") {
				return expectedRemoteURL, nil
			}
			return "", nil
		}

		// Mock successful image creation
		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }
		mocks.Shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image { return mockImage }

		// Capture annotations to verify fallback revision but valid source
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			if anns["org.opencontainers.image.revision"] != "unknown" {
				t.Errorf("Expected revision 'unknown' for empty SHA, got %s", anns["org.opencontainers.image.revision"])
			}
			if anns["org.opencontainers.image.source"] != expectedRemoteURL {
				t.Errorf("Expected source %s, got %s", expectedRemoteURL, anns["org.opencontainers.image.source"])
			}
			return mockImage
		}

		layer := static.NewLayer([]byte("test"), types.DockerLayer)

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0", "")

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if img == nil {
			t.Error("Expected non-nil image")
		}
	})
}

func TestArtifactBuilder_downloadOCIArtifact(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ParseReferenceSuccess", func(t *testing.T) {
		// Given an ArtifactBuilder with successful parse but failing remote
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil // Success
		}

		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, fmt.Errorf("remote image error") // Fail at next step
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then it should fail at remote image step
		if err == nil {
			t.Error("expected error for remote image failure")
		}
		if !strings.Contains(err.Error(), "failed to get image") {
			t.Errorf("expected remote image error, got %v", err)
		}
	})

	t.Run("ParseReferenceError", func(t *testing.T) {
		// Given an ArtifactBuilder with parse reference error
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, fmt.Errorf("parse error")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then it should return parse reference error
		if err == nil {
			t.Error("expected error for parse reference failure")
		}
		if !strings.Contains(err.Error(), "failed to parse reference") {
			t.Errorf("expected parse reference error, got %v", err)
		}
	})

	t.Run("RemoteImageError", func(t *testing.T) {
		// Given an ArtifactBuilder with remote image error
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, fmt.Errorf("remote error")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then it should return remote image error
		if err == nil {
			t.Error("expected error for remote image failure")
		}
		if !strings.Contains(err.Error(), "failed to get image") {
			t.Errorf("expected remote image error, got %v", err)
		}
	})

	t.Run("ImageLayersError", func(t *testing.T) {
		// Given an ArtifactBuilder with image layers error
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, nil
		}

		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return nil, fmt.Errorf("layers error")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then it should return image layers error
		if err == nil {
			t.Error("expected error for image layers failure")
		}
		if !strings.Contains(err.Error(), "failed to get image layers") {
			t.Errorf("expected image layers error, got %v", err)
		}
	})

	t.Run("PassesAuthKeychainOption", func(t *testing.T) {
		// Given an ArtifactBuilder that captures the options passed to RemoteImage
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		var capturedOptions []remote.Option
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			capturedOptions = options
			return nil, fmt.Errorf("stop here")
		}

		// When downloadOCIArtifact is called
		_, _ = builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then exactly one option (the keychain auth option) should be forwarded so private registries authenticate
		if len(capturedOptions) != 1 {
			t.Fatalf("expected 1 remote option to be forwarded for keychain auth, got %d", len(capturedOptions))
		}
	})

	t.Run("RetriesAnonymouslyWhenKeychainRejectedByRegistry", func(t *testing.T) {
		// Given a registry that rejects keychain credentials but accepts anonymous access
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		callCount := 0
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			callCount++
			if callCount == 1 {
				return nil, &transport.Error{StatusCode: http.StatusForbidden, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}
			}
			return nil, nil
		}
		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return nil, fmt.Errorf("stop after retry succeeded")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("ghcr.io", "windsorcli/core", "v0.7.0-rc.1")

		// Then RemoteImage should be called twice (keychain then anonymous)
		if callCount != 2 {
			t.Fatalf("expected 2 RemoteImage calls (keychain + anonymous retry), got %d", callCount)
		}
		// And execution should proceed past the get-image step, reaching ImageLayers
		if err == nil || !strings.Contains(err.Error(), "failed to get image layers") {
			t.Fatalf("expected retry to succeed and execution to reach ImageLayers, got %v", err)
		}
	})

	t.Run("RetriesAnonymouslyOnDeniedDiagnosticWithNonAuthStatus", func(t *testing.T) {
		// Given a *transport.Error whose HTTP status is not 401/403 but whose Diagnostic.Code is
		// DENIED — the ghcr.io token-endpoint shape that IsAuthenticationError catches and
		// that the old inline status-only gate would have missed
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		callCount := 0
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			callCount++
			if callCount == 1 {
				return nil, &transport.Error{StatusCode: http.StatusOK, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}
			}
			return nil, nil
		}
		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return nil, fmt.Errorf("stop after retry succeeded")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("ghcr.io", "windsorcli/core", "v0.7.0-rc.1")

		// Then the anonymous retry should fire on the diagnostic-only auth refusal
		if callCount != 2 {
			t.Fatalf("expected 2 RemoteImage calls (keychain + anonymous retry on DENIED diagnostic), got %d", callCount)
		}
		if err == nil || !strings.Contains(err.Error(), "failed to get image layers") {
			t.Fatalf("expected retry to succeed and execution to reach ImageLayers, got %v", err)
		}
	})

	t.Run("PreservesOriginalErrorWhenAnonymousRetryAlsoFails", func(t *testing.T) {
		// Given a registry that rejects both keychain and anonymous access with distinguishable errors
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		callCount := 0
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			callCount++
			if callCount == 1 {
				return nil, &transport.Error{StatusCode: http.StatusUnauthorized, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode, Message: "keychain-denied-marker"}}}
			}
			return nil, &transport.Error{StatusCode: http.StatusNotFound, Errors: []transport.Diagnostic{{Code: transport.ManifestUnknownErrorCode, Message: "anon-not-found-marker"}}}
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("ghcr.io", "private/repo", "v1.0.0")

		// Then the keychain error should be surfaced (it carries the real auth failure reason),
		// not the anonymous-retry error
		if err == nil {
			t.Fatal("expected error when both attempts fail")
		}
		if !strings.Contains(err.Error(), "keychain-denied-marker") {
			t.Errorf("expected original keychain error to be preserved, got %v", err)
		}
		if strings.Contains(err.Error(), "anon-not-found-marker") {
			t.Errorf("expected anonymous-retry error to be suppressed, got %v", err)
		}
	})

	t.Run("DoesNotRetryOnNonAuthError", func(t *testing.T) {
		// Given a non-authentication failure on the first RemoteImage call
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		callCount := 0
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			callCount++
			return nil, fmt.Errorf("connection refused")
		}

		// When downloadOCIArtifact is called
		_, err := builder.downloadOCIArtifact("registry.example.com", "modules", "v1.0.0")

		// Then no anonymous retry should happen
		if callCount != 1 {
			t.Fatalf("expected exactly 1 RemoteImage call for non-auth error, got %d", callCount)
		}
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("expected original error to be returned, got %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestFormatOCIRef(t *testing.T) {
	t.Run("Tag", func(t *testing.T) {
		// When FormatOCIRef is called with a plain tag
		result := FormatOCIRef("ghcr.io", "windsorcli/core", "v1.0.0")

		// Then it joins registry and repository with a colon
		if result != "ghcr.io/windsorcli/core:v1.0.0" {
			t.Errorf("expected 'ghcr.io/windsorcli/core:v1.0.0', got %s", result)
		}
	})

	t.Run("Digest", func(t *testing.T) {
		// When FormatOCIRef is called with a "sha256:<hex>" digest
		result := FormatOCIRef("ghcr.io", "windsorcli/core", "sha256:abcd1234")

		// Then it joins registry and repository with an "@", not a colon
		if result != "ghcr.io/windsorcli/core@sha256:abcd1234" {
			t.Errorf("expected 'ghcr.io/windsorcli/core@sha256:abcd1234', got %s", result)
		}
	})
}

func TestResolveCompatibleTag(t *testing.T) {
	t.Run("PicksHighestCompatibleSkippingIncompatibleNewest", func(t *testing.T) {
		setTestCliVersion(t, "0.8.1")
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.7.0" {
				return ">=99.0.0", nil
			}
			return "", nil
		}

		tag, v, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.5.0", "v0.6.0", "v0.7.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !ok || tag != "v0.6.0" {
			t.Fatalf("Expected v0.6.0 (skipping incompatible v0.7.0), got tag=%q ok=%v", tag, ok)
		}
		if v == nil || v.String() != "0.6.0" {
			t.Errorf("Expected parsed 0.6.0, got %v", v)
		}
	})

	t.Run("SkipsPrereleaseTagsEvenWhenCompatible", func(t *testing.T) {
		setTestCliVersion(t, "0.8.1")
		mock := NewMockArtifact()

		tag, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.6.0", "v0.7.0-rc.1"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !ok || tag != "v0.6.0" {
			t.Errorf("Expected v0.6.0 (prerelease excluded regardless of compatibility), got tag=%q ok=%v", tag, ok)
		}
	})

	t.Run("FallsBackToVerifyWhenAnnotationAbsentAndAccepts", func(t *testing.T) {
		// Given a candidate whose manifest carries no cliVersion annotation (an artifact
		// published before that annotation existed) but whose in-tarball metadata.yaml, checked
		// via the fallback, is actually compatible
		mock := NewMockArtifact()
		var verifiedRef string
		mock.VerifyCliVersionCompatibilityFunc = func(ociRef string) error {
			verifiedRef = ociRef
			return nil
		}

		tag, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !ok || tag != "v0.6.0" {
			t.Errorf("Expected the fallback-verified tag to be accepted, got tag=%q ok=%v", tag, ok)
		}
		if verifiedRef != "oci://ghcr.io/windsorcli/core:v0.6.0" {
			t.Errorf("Expected VerifyCliVersionCompatibility to be called with the candidate ref, got %q", verifiedRef)
		}
	})

	t.Run("FallsBackToVerifyWhenAnnotationAbsentAndRejectsThenTriesNext", func(t *testing.T) {
		// Given the newest candidate has no annotation and fails verification, but an older
		// candidate does carry a satisfied annotation
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.6.0" {
				return "", nil // no annotation on the newest tag
			}
			return "", nil // no annotation on the older tag either — both fall back to verify
		}
		mock.VerifyCliVersionCompatibilityFunc = func(ociRef string) error {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.6.0" {
				return fmt.Errorf("CLI version does not satisfy required constraint")
			}
			return nil
		}

		tag, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.5.0", "v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !ok || tag != "v0.5.0" {
			t.Errorf("Expected v0.5.0 (newest rejected by verify fallback), got tag=%q ok=%v", tag, ok)
		}
	})

	t.Run("SkipsVerifyFallbackWhenAnnotationIsPresent", func(t *testing.T) {
		// Given a candidate whose manifest DOES carry an annotation — the cheap path must stay
		// cheap and never fall back to a pull-based verification
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			return ">=0.1.0", nil
		}
		verifyCalled := false
		mock.VerifyCliVersionCompatibilityFunc = func(ociRef string) error {
			verifyCalled = true
			return nil
		}
		setTestCliVersion(t, "1.0.0")

		tag, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !ok || tag != "v0.6.0" {
			t.Errorf("Expected v0.6.0, got tag=%q ok=%v", tag, ok)
		}
		if verifyCalled {
			t.Error("Expected VerifyCliVersionCompatibility not to be called when the annotation is present")
		}
	})

	t.Run("FalseWhenAllCandidatesFailBothAnnotationAndVerifyFallback", func(t *testing.T) {
		mock := NewMockArtifact()
		mock.VerifyCliVersionCompatibilityFunc = func(ociRef string) error {
			return fmt.Errorf("incompatible")
		}

		_, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.5.0", "v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ok {
			t.Error("Expected ok=false when every candidate fails both the annotation check and the verify fallback")
		}
	})

	t.Run("FalseWhenNoTagIsCompatible", func(t *testing.T) {
		setTestCliVersion(t, "0.1.0")
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			return ">=99.0.0", nil
		}

		_, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.5.0", "v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ok {
			t.Error("Expected ok=false when no tag is compatible")
		}
	})

	t.Run("PropagatesConstraintCheckError", func(t *testing.T) {
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			return "", fmt.Errorf("registry unreachable")
		}

		_, _, _, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.6.0"})

		if err == nil {
			t.Error("Expected the registry error to propagate")
		}
	})

	t.Run("SkipsCandidateOnConstraintCheckErrorInsteadOfAbortingWalk", func(t *testing.T) {
		// Given a transient manifest-fetch error on only the newest candidate, with an older
		// candidate that checks out fine
		mock := NewMockArtifact()
		mock.GetCliVersionConstraintFunc = func(ociRef string) (string, error) {
			if ociRef == "oci://ghcr.io/windsorcli/core:v0.6.0" {
				return "", fmt.Errorf("registry hiccup")
			}
			return "", nil
		}

		tag, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v0.5.0", "v0.6.0"})

		if err != nil {
			t.Fatalf("Expected no error — an older candidate resolved fine, got %v", err)
		}
		if !ok || tag != "v0.5.0" {
			t.Errorf("Expected v0.5.0 (newest skipped on a check error, not an aborted walk), got tag=%q ok=%v", tag, ok)
		}
	})

	t.Run("FalseWhenNoStableSemverTagPresent", func(t *testing.T) {
		mock := NewMockArtifact()

		_, _, ok, err := ResolveCompatibleTag(mock, "oci://ghcr.io/windsorcli/core", []string{"v1.0.0-rc.1", "garbage"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ok {
			t.Error("Expected ok=false among prereleases/junk")
		}
	})
}

func TestParseOCIReference(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    *OCIArtifactInfo
		expectError bool
	}{
		{
			name:        "EmptyString",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:  "FullOCIURL",
			input: "oci://ghcr.io/windsorcli/core:v1.0.0",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core:v1.0.0",
				Tag:  "v1.0.0",
			},
			expectError: false,
		},
		{
			name:  "ShortFormat",
			input: "windsorcli/core:v1.0.0",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core:v1.0.0",
				Tag:  "v1.0.0",
			},
			expectError: false,
		},
		{
			name:        "MissingVersion",
			input:       "windsorcli/core",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "InvalidFormat",
			input:       "core:v1.0.0",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "EmptyVersion",
			input:       "windsorcli/core:",
			expected:    nil,
			expectError: true,
		},
		{
			name:  "FullOCIURLWithDigest",
			input: "oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
				Tag:  "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			},
			expectError: false,
		},
		{
			name:  "ShortFormatWithDigest",
			input: "windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
				Tag:  "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseOCIReference(tc.input)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if we := werror.Find(err); we == nil || we.Remediation != blueprintVersionHint {
					t.Errorf("Expected the blueprint version hint, got %v", we)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.expected == nil {
				if result != nil {
					t.Errorf("Expected nil result but got: %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("Expected result but got nil")
				return
			}

			if result.Name != tc.expected.Name {
				t.Errorf("Expected name %s but got %s", tc.expected.Name, result.Name)
			}

			if result.URL != tc.expected.URL {
				t.Errorf("Expected URL %s but got %s", tc.expected.URL, result.URL)
			}

			if result.Tag != tc.expected.Tag {
				t.Errorf("Expected tag %s but got %s", tc.expected.Tag, result.Tag)
			}
		})
	}
}

func TestParseRegistryURL(t *testing.T) {
	t.Run("ParsesRegistryURLWithTag", func(t *testing.T) {
		// Given a registry URL with tag
		url := "ghcr.io/windsorcli/core:v1.0.0"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct
		if registryBase != "ghcr.io" {
			t.Errorf("Expected registryBase 'ghcr.io', got '%s'", registryBase)
		}
		if repoName != "windsorcli/core" {
			t.Errorf("Expected repoName 'windsorcli/core', got '%s'", repoName)
		}
		if tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithoutTag", func(t *testing.T) {
		// Given a registry URL without tag
		url := "docker.io/myuser/myblueprint"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct
		if registryBase != "docker.io" {
			t.Errorf("Expected registryBase 'docker.io', got '%s'", registryBase)
		}
		if repoName != "myuser/myblueprint" {
			t.Errorf("Expected repoName 'myuser/myblueprint', got '%s'", repoName)
		}
		if tag != "" {
			t.Errorf("Expected empty tag, got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithOCIPrefix", func(t *testing.T) {
		// Given a registry URL with oci:// prefix
		url := "oci://registry.example.com/namespace/repo:latest"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And prefix should be stripped
		if registryBase != "registry.example.com" {
			t.Errorf("Expected registryBase 'registry.example.com', got '%s'", registryBase)
		}
		if repoName != "namespace/repo" {
			t.Errorf("Expected repoName 'namespace/repo', got '%s'", repoName)
		}
		if tag != "latest" {
			t.Errorf("Expected tag 'latest', got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithMultipleSlashes", func(t *testing.T) {
		// Given a registry URL with nested repository path
		url := "registry.com/org/project/subproject:v2.0"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And full repository path should be preserved
		if registryBase != "registry.com" {
			t.Errorf("Expected registryBase 'registry.com', got '%s'", registryBase)
		}
		if repoName != "org/project/subproject" {
			t.Errorf("Expected repoName 'org/project/subproject', got '%s'", repoName)
		}
		if tag != "v2.0" {
			t.Errorf("Expected tag 'v2.0', got '%s'", tag)
		}
	})

	t.Run("ReturnsErrorForInvalidFormatWithoutSlash", func(t *testing.T) {
		// Given an invalid registry URL without slash
		url := "registry.example.com"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then error should be returned
		if err == nil {
			t.Error("Expected error for invalid format, got nil")
		}

		// And error should indicate invalid format with a repository path hint
		if !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected error about invalid format, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "repository path") {
			t.Errorf("Expected a repository path hint, got %v", we)
		}

		// And components should be empty
		if registryBase != "" || repoName != "" || tag != "" {
			t.Errorf("Expected empty components on error, got: base=%s, repo=%s, tag=%s", registryBase, repoName, tag)
		}
	})

	t.Run("ReturnsErrorForEmptyString", func(t *testing.T) {
		// Given an empty URL
		url := ""

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then error should be returned
		if err == nil {
			t.Error("Expected error for empty string, got nil")
		}

		// And components should be empty
		if registryBase != "" || repoName != "" || tag != "" {
			t.Errorf("Expected empty components on error, got: base=%s, repo=%s, tag=%s", registryBase, repoName, tag)
		}
	})

	t.Run("HandlesRegistryURLWithColonInTag", func(t *testing.T) {
		// Given a registry URL with multiple colons (edge case)
		// The parser uses the last colon to separate repo from tag
		url := "registry.com/repo:tag:with:colons"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed using last colon
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct (last colon is used for tag)
		if registryBase != "registry.com" {
			t.Errorf("Expected registryBase 'registry.com', got '%s'", registryBase)
		}
		if repoName != "repo:tag:with" {
			t.Errorf("Expected repoName 'repo:tag:with', got '%s'", repoName)
		}
		if tag != "colons" {
			t.Errorf("Expected tag 'colons', got '%s'", tag)
		}
	})
}

func TestIsAuthenticationError(t *testing.T) {
	t.Run("ReturnsTrueForWrappedTransportError401", func(t *testing.T) {
		// Given a *transport.Error with HTTP 401 wrapped by fmt.Errorf %w (the shape the push
		// path actually produces: composer.Push wraps artifact.Push wraps shim's transport.Error)
		err := fmt.Errorf("failed to push artifact: %w", &transport.Error{StatusCode: http.StatusUnauthorized})

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then errors.As should reach through the wrap chain and classify as auth
		if !result {
			t.Error("Expected true for wrapped transport.Error with status 401")
		}
	})

	t.Run("ReturnsTrueForWrappedTransportError403", func(t *testing.T) {
		// Given a *transport.Error with HTTP 403 wrapped through the push call chain
		err := fmt.Errorf("failed to push artifact: %w", &transport.Error{StatusCode: http.StatusForbidden})

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should be classified as auth
		if !result {
			t.Error("Expected true for wrapped transport.Error with status 403")
		}
	})

	t.Run("ReturnsTrueForTransportErrorWithUnauthorizedDiagnostic", func(t *testing.T) {
		// Given a *transport.Error whose HTTP status is not 401/403 but whose diagnostic
		// payload carries an UNAUTHORIZED code (some registries return 200 OK on the token
		// endpoint with the auth refusal embedded in the body diagnostics)
		err := &transport.Error{StatusCode: http.StatusOK, Errors: []transport.Diagnostic{{Code: transport.UnauthorizedErrorCode}}}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then the diagnostic code should be honored
		if !result {
			t.Error("Expected true for transport.Error with UNAUTHORIZED diagnostic code")
		}
	})

	t.Run("ReturnsTrueForTransportErrorWithDeniedDiagnostic", func(t *testing.T) {
		// Given a *transport.Error carrying a DENIED diagnostic code (ghcr.io's token-endpoint
		// shape — the exact case that triggered this fix)
		err := &transport.Error{StatusCode: http.StatusOK, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then the diagnostic code should be honored
		if !result {
			t.Error("Expected true for transport.Error with DENIED diagnostic code")
		}
	})

	t.Run("ReturnsFalseForTransportErrorWithNonAuthStatus", func(t *testing.T) {
		// Given a *transport.Error with a status code unrelated to auth (404 from a missing manifest)
		err := &transport.Error{StatusCode: http.StatusNotFound}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should NOT be classified as auth — the prior substring impl would falsely
		// match "404" against any error containing that digit
		if result {
			t.Error("Expected false for transport.Error 404 — not an auth failure")
		}
	})

	t.Run("ReturnsFalseForNilError", func(t *testing.T) {
		// Given a nil error
		var err error

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for nil error")
		}
	})

	t.Run("ReturnsFalseForNonTransportError", func(t *testing.T) {
		// Given an error that is not a *transport.Error and contains no embedded one — for example
		// a network timeout or a pre-flight error before any registry round-trip
		err := fmt.Errorf("network timeout: dial tcp: i/o timeout")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false — only typed registry auth errors qualify
		if result {
			t.Error("Expected false for non-transport error")
		}
	})
}

func TestValidateCliVersion(t *testing.T) {
	t.Run("ReturnsNilWhenConstraintIsEmpty", func(t *testing.T) {
		// Given an empty constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for empty constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenCliVersionIsEmpty", func(t *testing.T) {
		// Given an empty CLI version
		// When validating
		err := ValidateCliVersion("", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for empty CLI version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForDevVersion", func(t *testing.T) {
		// Given dev version
		// When validating
		err := ValidateCliVersion("dev", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for dev version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForMainVersion", func(t *testing.T) {
		// Given main version
		// When validating
		err := ValidateCliVersion("main", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for main version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForLatestVersion", func(t *testing.T) {
		// Given latest version
		// When validating
		err := ValidateCliVersion("latest", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for latest version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForNightlyVersion", func(t *testing.T) {
		// Given the nightly release's snapshot version string
		// When validating
		err := ValidateCliVersion("nightly-SNAPSHOT-c6a65a53", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for nightly version, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidCliVersionFormat", func(t *testing.T) {
		// Given an invalid CLI version format
		// When validating
		err := ValidateCliVersion("invalid-version", ">=1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid CLI version format")
		}
		if !strings.Contains(err.Error(), "invalid CLI version format") {
			t.Errorf("Expected error to contain 'invalid CLI version format', got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidConstraint", func(t *testing.T) {
		// Given an invalid constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "invalid-constraint")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid constraint")
		}
		if !strings.Contains(err.Error(), "invalid cliVersion constraint") {
			t.Errorf("Expected error to contain 'invalid cliVersion constraint', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionDoesNotSatisfyConstraint", func(t *testing.T) {
		// Given a version that doesn't satisfy constraint
		// When validating
		err := ValidateCliVersion("1.0.0", ">=2.0.0")

		// Then should return error with an install hint
		if err == nil {
			t.Error("Expected error when version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "Install a Windsor CLI version") {
			t.Errorf("Expected an install hint, got %v", we)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesGreaterThanConstraint", func(t *testing.T) {
		// Given a version that satisfies >= constraint
		// When validating
		err := ValidateCliVersion("2.0.0", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesLessThanConstraint", func(t *testing.T) {
		// Given a version that satisfies < constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "<2.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesRangeConstraint", func(t *testing.T) {
		// Given a version that satisfies range constraint
		// When validating
		err := ValidateCliVersion("1.5.0", ">=1.0.0 <2.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied range constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionOutsideRange", func(t *testing.T) {
		// Given a version outside range
		// When validating
		err := ValidateCliVersion("2.5.0", ">=1.0.0 <2.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when version outside range")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesTildeConstraint", func(t *testing.T) {
		// Given a version that satisfies ~ constraint
		// When validating
		err := ValidateCliVersion("1.2.3", "~1.2.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied tilde constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionDoesNotSatisfyTildeConstraint", func(t *testing.T) {
		// Given a version that doesn't satisfy ~ constraint
		// When validating
		err := ValidateCliVersion("1.3.0", "~1.2.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when version doesn't satisfy tilde constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionWithVPrefixSatisfiesConstraint", func(t *testing.T) {
		// Given a version with v prefix that satisfies constraint
		// When validating
		err := ValidateCliVersion("v1.0.0", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for v-prefixed version satisfying constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionWithVPrefixDoesNotSatisfyConstraint", func(t *testing.T) {
		// Given a version with v prefix that doesn't satisfy constraint
		// When validating
		err := ValidateCliVersion("v0.5.0", ">=1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when v-prefixed version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenReleaseCandidateSatisfiesGreaterThanEqualConstraint", func(t *testing.T) {
		// Given a release-candidate CLI version whose release satisfies a >= constraint
		// When validating
		err := ValidateCliVersion("0.9.0-rc.1", ">=0.9.0")

		// Then should return nil — the RC is treated as its release version
		if err != nil {
			t.Errorf("Expected nil for RC version satisfying constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenReleaseCandidateWithVPrefixSatisfiesConstraint", func(t *testing.T) {
		// Given a v-prefixed release-candidate CLI version
		// When validating
		err := ValidateCliVersion("v0.9.0-rc.1", ">=0.9.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for v-prefixed RC version satisfying constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReleaseCandidateDoesNotSatisfyConstraint", func(t *testing.T) {
		// Given a release-candidate CLI version whose release does not satisfy the constraint
		// When validating
		err := ValidateCliVersion("0.8.0-rc.1", ">=0.9.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when RC's release version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})
}
