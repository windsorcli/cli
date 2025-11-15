package artifact

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestArtifactBuilder_resolveOutputPath(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("GeneratesFilenameInCurrentDirectory", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When resolving path for current directory
		actualPath := builder.resolveOutputPath(".", "testproject", "v1.0.0")

		// Then filename should be generated in current directory
		expectedPath := "testproject-v1.0.0.tar.gz"
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("GeneratesFilenameInSpecifiedDirectory", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When resolving path for directory without extension
		actualPath := builder.resolveOutputPath("output", "testproject", "v1.0.0")

		// Then filename should be generated in that directory
		expectedPath := filepath.Join("output", "testproject-v1.0.0.tar.gz")
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("GeneratesFilenameWithTrailingSlash", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When resolving path with trailing slash
		actualPath := builder.resolveOutputPath("output/", "testproject", "v1.0.0")

		// Then filename should be generated in that directory
		expectedPath := filepath.Join("output", "testproject-v1.0.0.tar.gz")
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("UsesExplicitFilename", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When resolving path with explicit filename
		explicitPath := "custom-name.tar.gz"
		actualPath := builder.resolveOutputPath(explicitPath, "testproject", "v1.0.0")

		// Then explicit filename should be used
		if actualPath != explicitPath {
			t.Errorf("Expected path %s, got %s", explicitPath, actualPath)
		}
	})

	t.Run("UsesExplicitPathWithFilename", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When resolving path with directory and filename
		explicitPath := filepath.Join("output", "custom-name.tar.gz")
		actualPath := builder.resolveOutputPath(explicitPath, "testproject", "v1.0.0")

		// Then explicit path should be used
		if actualPath != explicitPath {
			t.Errorf("Expected path %s, got %s", explicitPath, actualPath)
		}
	})
}
func TestArtifactBuilder_createTarballInMemory(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithFiles", func(t *testing.T) {
		// Given a builder with files and metadata
		builder, _ := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		builder.addFile("other.txt", []byte("other content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// When creating tarball in memory
		result, err := builder.createTarballInMemory(metadata)

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) == 0 {
			t.Error("Expected non-empty tarball")
		}
	})

	t.Run("SuccessSkipsMetadataFile", func(t *testing.T) {
		// Given a builder with _templates/metadata.yaml file
		builder, _ := setup(t)
		builder.addFile("_templates/metadata.yaml", []byte("original metadata"), 0644)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// When creating tarball in memory
		result, err := builder.createTarballInMemory(metadata)

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) == 0 {
			t.Error("Expected non-empty tarball")
		}
	})

	t.Run("ErrorWhenTarWriterWriteHeaderFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// Mock tar writer to fail on WriteHeader for metadata
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return fmt.Errorf("write header failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory(metadata)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when tar writer header fails")
		}
		if !strings.Contains(err.Error(), "failed to write metadata header") {
			t.Errorf("Expected error to contain 'failed to write metadata header', got %v", err)
		}
	})

	t.Run("ErrorWhenTarWriterWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// Mock tar writer to fail on Write for metadata
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return nil
				},
				writeFunc: func([]byte) (int, error) {
					return 0, fmt.Errorf("write failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory(metadata)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when tar writer write fails")
		}
		if !strings.Contains(err.Error(), "failed to write metadata") {
			t.Errorf("Expected error to contain 'failed to write metadata', got %v", err)
		}
	})

	t.Run("ErrorWhenFileHeaderWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		headerCount := 0
		// Mock tar writer to fail on second WriteHeader (for file)
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(hdr *tar.Header) error {
					headerCount++
					if headerCount > 1 {
						return fmt.Errorf("file header write failed")
					}
					return nil
				},
				writeFunc: func([]byte) (int, error) {
					return 100, nil
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory(metadata)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when file header write fails")
		}
		if !strings.Contains(err.Error(), "failed to write header for test.txt") {
			t.Errorf("Expected error to contain 'failed to write header for test.txt', got %v", err)
		}
	})

	t.Run("ErrorWhenFileContentWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		writeCount := 0
		// Mock tar writer to fail on second Write (for file content)
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return nil
				},
				writeFunc: func([]byte) (int, error) {
					writeCount++
					if writeCount > 1 {
						return 0, fmt.Errorf("file content write failed")
					}
					return 100, nil
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory(metadata)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when file content write fails")
		}
		if !strings.Contains(err.Error(), "failed to write content for test.txt") {
			t.Errorf("Expected error to contain 'failed to write content for test.txt', got %v", err)
		}
	})

	t.Run("ErrorWhenTarWriterCloseFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// Mock tar writer to fail on Close
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return nil
				},
				writeFunc: func([]byte) (int, error) {
					return 100, nil
				},
				closeFunc: func() error {
					return fmt.Errorf("tar writer close failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory(metadata)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when tar writer close fails")
		}
		if !strings.Contains(err.Error(), "failed to close tar writer") {
			t.Errorf("Expected error to contain 'failed to close tar writer', got %v", err)
		}
	})

	t.Run("SuccessWithEmptyFiles", func(t *testing.T) {
		// Given a builder with no files, only metadata
		builder, _ := setup(t)
		metadata := []byte("name: test\nversion: v1.0.0\n")

		// When creating tarball in memory
		result, err := builder.createTarballInMemory(metadata)

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) == 0 {
			t.Error("Expected non-empty tarball")
		}
	})

}

// =============================================================================
// Test generateMetadataWithNameVersion
// =============================================================================

func TestArtifactBuilder_generateMetadataWithNameVersion(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithGitProvenanceAndBuilderInfo", func(t *testing.T) {
		// Given a builder with shell configured
		builder, mocks := setup(t)

		// Mock successful git operations
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "abc123def456", nil
			case strings.Contains(cmd, "git describe --tags --exact-match HEAD"):
				return "v1.0.0", nil
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "https://github.com/example/repo.git", nil
			case strings.Contains(cmd, "git config --get user.name"):
				return "Test User", nil
			case strings.Contains(cmd, "git config --get user.email"):
				return "test@example.com", nil
			default:
				return "", nil
			}
		}

		// Override YamlMarshal to actually marshal the data
		mocks.Shims.YamlMarshal = func(data any) ([]byte, error) {
			return yaml.Marshal(data)
		}

		input := BlueprintMetadataInput{
			Description: "Test description",
			Author:      "Test Author",
			Tags:        []string{"test", "example"},
			Homepage:    "https://example.com",
			License:     "MIT",
			CliVersion:  ">=1.0.0",
		}

		// When generating metadata
		metadata, err := builder.generateMetadataWithNameVersion(input, "testapp", "1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if metadata == nil {
			t.Error("Expected metadata to be generated")
		}

		// And cliVersion should be preserved in generated metadata
		var generatedMetadata BlueprintMetadata
		if err := yaml.Unmarshal(metadata, &generatedMetadata); err != nil {
			t.Fatalf("Failed to unmarshal generated metadata: %v", err)
		}
		if generatedMetadata.CliVersion != ">=1.0.0" {
			t.Errorf("Expected cliVersion to be '>=1.0.0', got '%s'", generatedMetadata.CliVersion)
		}
	})

	t.Run("SuccessWithGitProvenanceFailure", func(t *testing.T) {
		// Given a builder with shell configured to fail git operations
		builder, mocks := setup(t)

		// Mock git operations to fail
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("git command failed")
		}

		input := BlueprintMetadataInput{
			Description: "Test description",
		}

		// When generating metadata
		metadata, err := builder.generateMetadataWithNameVersion(input, "testapp", "1.0.0")

		// Then should succeed with empty git provenance
		if err != nil {
			t.Errorf("Expected success despite git failures, got error: %v", err)
		}
		if metadata == nil {
			t.Error("Expected metadata to be generated")
		}
	})

	t.Run("ErrorWhenYamlMarshalFails", func(t *testing.T) {
		// Given a builder with failing YAML marshal
		builder, mocks := setup(t)
		mocks.Shims.YamlMarshal = func(data any) ([]byte, error) {
			return nil, fmt.Errorf("yaml marshal failed")
		}

		input := BlueprintMetadataInput{}

		// When generating metadata
		_, err := builder.generateMetadataWithNameVersion(input, "testapp", "1.0.0")

		// Then should get marshal error
		if err == nil || !strings.Contains(err.Error(), "yaml marshal failed") {
			t.Errorf("Expected yaml marshal error, got: %v", err)
		}
	})
}

func TestArtifactBuilder_getGitProvenance(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithAllGitInfo", func(t *testing.T) {
		// Given a builder with successful git operations
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "  abc123def456  ", nil // With whitespace to test trimming
			case strings.Contains(cmd, "git describe --tags --exact-match HEAD"):
				return "  v1.0.0  ", nil // With whitespace to test trimming
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "  https://github.com/example/repo.git  ", nil // With whitespace
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting git provenance
		provenance, err := builder.getGitProvenance()

		// Then should succeed with trimmed values
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if provenance.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA 'abc123def456', got '%s'", provenance.CommitSHA)
		}
		if provenance.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", provenance.Tag)
		}
		if provenance.RemoteURL != "https://github.com/example/repo.git" {
			t.Errorf("Expected remote URL 'https://github.com/example/repo.git', got '%s'", provenance.RemoteURL)
		}
	})

	t.Run("ErrorWhenCommitSHAFails", func(t *testing.T) {
		// Given a builder with failing commit SHA command
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			if strings.Contains(cmd, "git rev-parse HEAD") {
				return "", fmt.Errorf("not a git repository")
			}
			return "", nil
		}

		// When getting git provenance
		_, err := builder.getGitProvenance()

		// Then should get commit SHA error
		if err == nil || !strings.Contains(err.Error(), "failed to get commit SHA") {
			t.Errorf("Expected commit SHA error, got: %v", err)
		}
	})

	t.Run("SuccessWithMissingTag", func(t *testing.T) {
		// Given a builder where tag command fails but others succeed
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "abc123def456", nil
			case strings.Contains(cmd, "git describe --tags --exact-match HEAD"):
				return "", fmt.Errorf("no tag found") // Tag command fails
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "https://github.com/example/repo.git", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting git provenance
		provenance, err := builder.getGitProvenance()

		// Then should succeed with empty tag
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if provenance.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA 'abc123def456', got '%s'", provenance.CommitSHA)
		}
		if provenance.Tag != "" {
			t.Errorf("Expected empty tag, got '%s'", provenance.Tag)
		}
		if provenance.RemoteURL != "https://github.com/example/repo.git" {
			t.Errorf("Expected remote URL, got '%s'", provenance.RemoteURL)
		}
	})

	t.Run("SuccessWithMissingRemoteURL", func(t *testing.T) {
		// Given a builder where remote URL command fails
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "abc123def456", nil
			case strings.Contains(cmd, "git describe --tags --exact-match HEAD"):
				return "v1.0.0", nil
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "", fmt.Errorf("no remote configured") // Remote URL fails
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting git provenance
		provenance, err := builder.getGitProvenance()

		// Then should succeed with empty remote URL
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if provenance.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA 'abc123def456', got '%s'", provenance.CommitSHA)
		}
		if provenance.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", provenance.Tag)
		}
		if provenance.RemoteURL != "" {
			t.Errorf("Expected empty remote URL, got '%s'", provenance.RemoteURL)
		}
	})
}

func TestArtifactBuilder_getBuilderInfo(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithUserAndEmail", func(t *testing.T) {
		// Given a builder with configured git user info
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git config --get user.name"):
				return "  Test User  ", nil // With whitespace to test trimming
			case strings.Contains(cmd, "git config --get user.email"):
				return "  test@example.com  ", nil // With whitespace to test trimming
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then should succeed with trimmed values
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if builderInfo.User != "Test User" {
			t.Errorf("Expected user 'Test User', got '%s'", builderInfo.User)
		}
		if builderInfo.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got '%s'", builderInfo.Email)
		}
	})

	t.Run("SuccessWithMissingUserName", func(t *testing.T) {
		// Given a builder where user name is not configured
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git config --get user.name"):
				return "", fmt.Errorf("user.name not configured")
			case strings.Contains(cmd, "git config --get user.email"):
				return "test@example.com", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then should succeed with empty user name
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if builderInfo.User != "" {
			t.Errorf("Expected empty user, got '%s'", builderInfo.User)
		}
		if builderInfo.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got '%s'", builderInfo.Email)
		}
	})

	t.Run("SuccessWithMissingEmail", func(t *testing.T) {
		// Given a builder where email is not configured
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git config --get user.name"):
				return "Test User", nil
			case strings.Contains(cmd, "git config --get user.email"):
				return "", fmt.Errorf("user.email not configured")
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then should succeed with empty email
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if builderInfo.User != "Test User" {
			t.Errorf("Expected user 'Test User', got '%s'", builderInfo.User)
		}
		if builderInfo.Email != "" {
			t.Errorf("Expected empty email, got '%s'", builderInfo.Email)
		}
	})

	t.Run("SuccessWithBothMissing", func(t *testing.T) {
		// Given a builder where both user and email are not configured
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("git config not found")
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then should succeed with empty values
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if builderInfo.User != "" {
			t.Errorf("Expected empty user, got '%s'", builderInfo.User)
		}
		if builderInfo.Email != "" {
			t.Errorf("Expected empty email, got '%s'", builderInfo.Email)
		}
	})
}

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
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if img == nil {
			t.Error("Expected non-nil image")
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
		_, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0")

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
		_, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0")

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
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0")

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
		img, err := builder.createOCIArtifactImage(layer, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if img == nil {
			t.Error("Expected non-nil image")
		}
	})
}

func TestArtifactBuilder_parseOCIRef(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ValidOCIReference", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When parseOCIRef is called with valid OCI reference
		registry, repository, tag, err := builder.parseOCIRef("oci://registry.example.com/my-repo:v1.0.0")

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

		// When parseOCIRef is called with invalid prefix
		_, _, _, err := builder.parseOCIRef("https://registry.example.com/my-repo:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format: https://registry.example.com/my-repo:v1.0.0"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MissingTag", func(t *testing.T) {
		// Given an ArtifactBuilder
		builder, _ := setup(t)

		// When parseOCIRef is called with missing tag
		_, _, _, err := builder.parseOCIRef("oci://registry.example.com/my-repo")

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

		// When parseOCIRef is called with missing repository
		_, _, _, err := builder.parseOCIRef("oci://registry.example.com:v1.0.0")

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

		// When parseOCIRef is called with nested repository path
		registry, repository, tag, err := builder.parseOCIRef("oci://registry.example.com/organization/my-repo:v1.0.0")

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
}

func TestArtifactBuilder_findMatchingProcessor(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("FindsMatchingProcessor", func(t *testing.T) {
		// Given a builder with processors
		builder, _ := setup(t)

		processors := []PathProcessor{
			{Pattern: "contexts/_template"},
			{Pattern: "kustomize"},
			{Pattern: "terraform"},
		}

		// When finding matching processor
		processor := builder.findMatchingProcessor("kustomize/file.yaml", processors)

		// Then should find the kustomize processor
		if processor == nil {
			t.Error("Expected to find matching processor")
		}
		if processor.Pattern != "kustomize" {
			t.Errorf("Expected kustomize pattern, got %s", processor.Pattern)
		}
	})

	t.Run("ReturnsNilForNoMatch", func(t *testing.T) {
		// Given a builder with processors
		builder, _ := setup(t)

		processors := []PathProcessor{
			{Pattern: "contexts/_template"},
			{Pattern: "kustomize"},
			{Pattern: "terraform"},
		}

		// When finding matching processor for non-matching path
		processor := builder.findMatchingProcessor("other/file.txt", processors)

		// Then should return nil
		if processor != nil {
			t.Error("Expected no matching processor")
		}
	})

	t.Run("MatchesFirstProcessor", func(t *testing.T) {
		// Given a builder with overlapping processors
		builder, _ := setup(t)

		processors := []PathProcessor{
			{Pattern: "test"},
			{Pattern: "test/sub"},
		}

		// When finding matching processor
		processor := builder.findMatchingProcessor("test/file.txt", processors)

		// Then should find the first matching processor
		if processor == nil {
			t.Error("Expected to find matching processor")
		}
		if processor.Pattern != "test" {
			t.Errorf("Expected test pattern, got %s", processor.Pattern)
		}
	})
}

// TestArtifactBuilder_shouldSkipTerraformFile tests the shouldSkipTerraformFile method of ArtifactBuilder
func TestArtifactBuilder_shouldSkipTerraformFile(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SkipsTerraformStateFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking terraform state files
		shouldSkip := builder.shouldSkipTerraformFile("terraform.tfstate")
		shouldSkipBackup := builder.shouldSkipTerraformFile("terraform.tfstate.backup")

		// Then should skip both
		if !shouldSkip {
			t.Error("Expected to skip terraform.tfstate")
		}
		if !shouldSkipBackup {
			t.Error("Expected to skip terraform.tfstate.backup")
		}
	})

	t.Run("SkipsTerraformOverrideFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking terraform override files
		shouldSkip := builder.shouldSkipTerraformFile("override.tf")
		shouldSkipJson := builder.shouldSkipTerraformFile("override.tf.json")
		shouldSkipUnderscore := builder.shouldSkipTerraformFile("test_override.tf")

		// Then should skip all
		if !shouldSkip {
			t.Error("Expected to skip override.tf")
		}
		if !shouldSkipJson {
			t.Error("Expected to skip override.tf.json")
		}
		if !shouldSkipUnderscore {
			t.Error("Expected to skip test_override.tf")
		}
	})

	t.Run("SkipsTerraformVarsFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking terraform vars files
		shouldSkip := builder.shouldSkipTerraformFile("terraform.tfvars")
		shouldSkipJson := builder.shouldSkipTerraformFile("terraform.tfvars.json")

		// Then should skip both
		if !shouldSkip {
			t.Error("Expected to skip terraform.tfvars")
		}
		if !shouldSkipJson {
			t.Error("Expected to skip terraform.tfvars.json")
		}
	})

	t.Run("SkipsTerraformPlanFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking terraform plan files
		shouldSkip := builder.shouldSkipTerraformFile("terraform.tfplan")

		// Then should skip
		if !shouldSkip {
			t.Error("Expected to skip terraform.tfplan")
		}
	})

	t.Run("SkipsTerraformConfigFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking terraform config files
		shouldSkipRc := builder.shouldSkipTerraformFile(".terraformrc")
		shouldSkipTerraformRc := builder.shouldSkipTerraformFile("terraform.rc")

		// Then should skip both
		if !shouldSkipRc {
			t.Error("Expected to skip .terraformrc")
		}
		if !shouldSkipTerraformRc {
			t.Error("Expected to skip terraform.rc")
		}
	})

	t.Run("SkipsCrashLogFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking crash log files
		shouldSkip := builder.shouldSkipTerraformFile("crash.log")
		shouldSkipPrefixed := builder.shouldSkipTerraformFile("crash.123.log")

		// Then should skip both
		if !shouldSkip {
			t.Error("Expected to skip crash.log")
		}
		if !shouldSkipPrefixed {
			t.Error("Expected to skip crash.123.log")
		}
	})

	t.Run("DoesNotSkipRegularFiles", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When checking regular terraform files
		shouldSkip := builder.shouldSkipTerraformFile("main.tf")
		shouldSkipVar := builder.shouldSkipTerraformFile("variables.tf")
		shouldSkipOutput := builder.shouldSkipTerraformFile("outputs.tf")

		// Then should not skip any
		if shouldSkip {
			t.Error("Expected not to skip main.tf")
		}
		if shouldSkipVar {
			t.Error("Expected not to skip variables.tf")
		}
		if shouldSkipOutput {
			t.Error("Expected not to skip outputs.tf")
		}
	})
}

// TestArtifactBuilder_walkAndProcessFiles tests the walkAndProcessFiles method of ArtifactBuilder
func TestArtifactBuilder_walkAndProcessFiles(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithMatchingFiles", func(t *testing.T) {
		// Given a builder with processors
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("test/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "test" {
				fn("test", &mockFileInfo{name: "test", isDir: true}, nil)
				fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, nil)
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "file.txt", nil
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should have added files
		if len(builder.files) == 0 {
			t.Error("Expected files to be added")
		}
	})

	t.Run("SuccessWithNoMatchingFiles", func(t *testing.T) {
		// Given a builder with processors that don't match
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "other",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("other/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "test" {
				fn("test", &mockFileInfo{name: "test", isDir: true}, nil)
				fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, nil)
			}
			return nil
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should not have added files
		if len(builder.files) != 0 {
			t.Error("Expected no files to be added")
		}
	})

	t.Run("SuccessWithSkipTerraformDirectory", func(t *testing.T) {
		// Given a builder with terraform directory
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "terraform",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("terraform/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "terraform" {
				return &mockFileInfo{name: "terraform", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function with .terraform directory
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "terraform" {
				fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil)
				fn("terraform/.terraform", &mockFileInfo{name: ".terraform", isDir: true}, nil)
				fn("terraform/main.tf", &mockFileInfo{name: "main.tf", isDir: false}, nil)
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "main.tf", nil
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should have added files (but not .terraform contents)
		if len(builder.files) == 0 {
			t.Error("Expected files to be added")
		}
	})

	t.Run("SuccessWithMissingDirectories", func(t *testing.T) {
		// Given a builder with missing directories
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "missing",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("missing/"+relPath, data, mode)
				},
			},
		}

		// Mock directory doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnWalkFailure", func(t *testing.T) {
		// Given a builder with walk error
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("test/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		expectedError := fmt.Errorf("walk error")
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return expectedError
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk directory") {
			t.Errorf("Expected walk error, got %v", err)
		}
	})

	t.Run("ErrorWhenWalkCallbackFails", func(t *testing.T) {
		// Given a builder where walk callback returns error
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("test/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function to call callback with error
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, fmt.Errorf("callback error"))
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk directory") {
			t.Errorf("Expected walk error, got %v", err)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		// Given a builder where ReadFile fails
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("test/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function - should return error from callback
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "test" {
				if err := fn("test", &mockFileInfo{name: "test", isDir: true}, nil); err != nil {
					return err
				}
				if err := fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, nil); err != nil {
					return err
				}
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, fmt.Errorf("read file error")
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "file.txt", nil
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read file") {
			t.Errorf("Expected read file error, got %v", err)
		}
	})

	t.Run("ErrorWhenFilepathRelFails", func(t *testing.T) {
		// Given a builder where FilepathRel fails
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return builder.addFile("test/"+relPath, data, mode)
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function - should return error from callback
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "test" {
				if err := fn("test", &mockFileInfo{name: "test", isDir: true}, nil); err != nil {
					return err
				}
				if err := fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, nil); err != nil {
					return err
				}
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("filepath rel error")
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get relative path") {
			t.Errorf("Expected filepath rel error, got %v", err)
		}
	})

	t.Run("ErrorWhenHandlerFails", func(t *testing.T) {
		// Given a builder where handler fails
		builder, mocks := setup(t)

		processors := []PathProcessor{
			{
				Pattern: "test",
				Handler: func(relPath string, data []byte, mode os.FileMode) error {
					return fmt.Errorf("handler error")
				},
			},
		}

		// Mock directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "test" {
				return &mockFileInfo{name: "test", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock walk function - should return error from callback
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "test" {
				if err := fn("test", &mockFileInfo{name: "test", isDir: true}, nil); err != nil {
					return err
				}
				if err := fn("test/file.txt", &mockFileInfo{name: "file.txt", isDir: false}, nil); err != nil {
					return err
				}
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "file.txt", nil
		}

		// When walking and processing files
		err := builder.walkAndProcessFiles(processors)

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk directory") {
			t.Errorf("Expected walk error, got %v", err)
		}
	})
}

// =============================================================================
// Test Helper Functions
// =============================================================================
