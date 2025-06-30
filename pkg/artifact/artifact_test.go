package artifact

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Helpers and Mocks
// =============================================================================

type MockTarWriter struct {
	WriteHeaderFunc func(hdr *tar.Header) error
	WriteFunc       func(b []byte) (int, error)
	CloseFunc       func() error
}

func (m *MockTarWriter) WriteHeader(hdr *tar.Header) error {
	if m.WriteHeaderFunc != nil {
		return m.WriteHeaderFunc(hdr)
	}
	return nil
}

func (m *MockTarWriter) Write(b []byte) (int, error) {
	if m.WriteFunc != nil {
		return m.WriteFunc(b)
	}
	return len(b), nil
}

func (m *MockTarWriter) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

type mockFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

type mockReadCloser struct {
	io.ReadCloser
	CloseFunc func() error
}

func (m *mockReadCloser) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return m.ReadCloser.Close()
}

// =============================================================================
// Test Setup
// =============================================================================

type SetupOptions struct {
	Injector  di.Injector
	ConfigStr string
}

type Mocks struct {
	Injector  di.Injector
	Shims     *Shims
	MockShell *shell.MockShell
	TempDir   string
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if opts == nil {
		opts = []*SetupOptions{{}}
	}

	if opts[0].Injector == nil {
		opts[0].Injector = di.NewMockInjector()
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	mockShell := shell.NewMockShell()
	setupDefaultShellBehavior(mockShell)

	mocks := &Mocks{
		Injector:  opts[0].Injector,
		Shims:     setupShims(t),
		MockShell: mockShell,
		TempDir:   tmpDir,
	}

	mocks.Injector.Register("shell", mockShell)

	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return mocks
}

func setupShims(t *testing.T) *Shims {
	t.Helper()
	return NewShims()
}

func setupDefaultShellBehavior(mockShell *shell.MockShell) {
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		cmd := strings.Join(append([]string{command}, args...), " ")
		switch {
		case strings.Contains(cmd, "git rev-parse HEAD"):
			return "abc123def456", nil
		case strings.Contains(cmd, "git tag --points-at HEAD"):
			return "v1.0.0", nil
		case strings.Contains(cmd, "git config --get remote.origin.url"):
			return "https://github.com/test/repo.git", nil
		case strings.Contains(cmd, "git config user.name"):
			return "Test User", nil
		case strings.Contains(cmd, "git config user.email"):
			return "test@example.com", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", cmd)
		}
	}
}

func setupArtifactWithFiles(t *testing.T, builder *ArtifactBuilder) {
	t.Helper()

	metadataContent := `name: test-blueprint
description: A test blueprint
`

	if err := builder.AddFile("_templates/metadata.yaml", []byte(metadataContent)); err != nil {
		t.Fatalf("Failed to add _templates/metadata.yaml: %v", err)
	}

	if err := builder.AddFile("test.txt", []byte("test content")); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewArtifactBuilder(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given no preconditions

		// When creating a new artifact builder
		builder := NewArtifactBuilder()

		// Then it should be properly initialized
		if builder == nil {
			t.Error("Expected builder to be non-nil")
		}
		if builder.shims == nil {
			t.Error("Expected shims to be initialized")
		}
		if builder.shell != nil {
			t.Error("Expected shell to be nil before initialization")
		}
		if builder.files == nil {
			t.Error("Expected files map to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArtifactBuilder_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given an artifact builder with a mock injector that has shell registered
		mocks := setupMocks(t)
		builder := NewArtifactBuilder()

		// When initializing the builder
		err := builder.Initialize(mocks.Injector)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And shell should be resolved from injector
		if builder.shell == nil {
			t.Error("Expected shell to be resolved from injector")
		}
		if builder.shell != mocks.MockShell {
			t.Error("Expected shell to be the same instance from injector")
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		// Given an artifact builder with nil injector
		builder := NewArtifactBuilder()

		// When initializing the builder
		err := builder.Initialize(nil)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error with nil injector, got %v", err)
		}

		// And shell should remain nil
		if builder.shell != nil {
			t.Error("Expected shell to remain nil with nil injector")
		}
	})

	t.Run("ShellNotRegistered", func(t *testing.T) {
		// Given an artifact builder with injector that has no shell registered
		injector := di.NewMockInjector()
		builder := NewArtifactBuilder()

		// When initializing the builder
		err := builder.Initialize(injector)

		// Then it should fail with shell resolution error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve shell from injector") {
			t.Errorf("Expected shell resolution error, got %v", err)
		}

		// And shell should remain nil
		if builder.shell != nil {
			t.Error("Expected shell to remain nil after failed initialization")
		}
	})

	t.Run("ShellWrongType", func(t *testing.T) {
		// Given an artifact builder with injector that has wrong type registered as shell
		injector := di.NewMockInjector()
		injector.Register("shell", "not a shell")
		builder := NewArtifactBuilder()

		// When initializing the builder
		err := builder.Initialize(injector)

		// Then it should fail with type assertion error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve shell from injector") {
			t.Errorf("Expected shell type assertion error, got %v", err)
		}

		// And shell should remain nil
		if builder.shell != nil {
			t.Error("Expected shell to remain nil after failed initialization")
		}
	})
}

func TestArtifactBuilder_Create(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims

		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("Failed to initialize builder: %v", err)
		}

		return builder, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given an artifact with files
		builder, mocks := setup(t)
		setupArtifactWithFiles(t, builder)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// When creating an artifact
		actualOutputPath, err := builder.Create(outputPath, "test:1.0.0")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if actualOutputPath != outputPath {
			t.Errorf("Expected output path %q, got %q", outputPath, actualOutputPath)
		}

		// And the output file should exist
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Error("Expected output file to exist")
		}
	})

	t.Run("NoTagNoMetadata", func(t *testing.T) {
		// Given an artifact without tag or metadata.yaml
		builder, _ := setup(t)
		outputPath := filepath.Join("output.tar.gz")

		// When creating an artifact without tag
		_, err := builder.Create(outputPath, "")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for missing name, got nil")
		}
		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("Expected name required error, got %v", err)
		}
	})

	t.Run("OutputPathError", func(t *testing.T) {
		// Given an artifact with files but invalid output path
		builder, _ := setup(t)
		setupArtifactWithFiles(t, builder)
		outputPath := "/invalid/path/output.tar.gz"

		// Mock file creation to fail
		builder.shims.Create = func(name string) (io.WriteCloser, error) {
			return nil, fmt.Errorf("permission denied")
		}

		// When creating an artifact
		_, err := builder.Create(outputPath, "test:1.0.0")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for invalid output path, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create output file") {
			t.Errorf("Expected output file creation error, got %v", err)
		}
	})

	t.Run("MetadataGenerationError", func(t *testing.T) {
		// Given an artifact with invalid metadata.yaml content
		builder, _ := setup(t)
		outputPath := filepath.Join("output.tar.gz")

		// Add invalid metadata content
		if err := builder.AddFile("_templates/metadata.yaml", []byte("invalid yaml content: [")); err != nil {
			t.Fatalf("Failed to add invalid _templates/metadata.yaml: %v", err)
		}

		// When creating an artifact
		_, err := builder.Create(outputPath, "test:1.0.0")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for invalid _templates/metadata.yaml, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse metadata.yaml") {
			t.Errorf("Expected metadata parsing error, got %v", err)
		}
	})

	t.Run("TarWriteHeaderError", func(t *testing.T) {
		// Given an artifact with files but tar writer that fails on WriteHeader
		builder, mocks := setup(t)
		setupArtifactWithFiles(t, builder)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock tar writer to fail on WriteHeader
		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &MockTarWriter{
				WriteHeaderFunc: func(hdr *tar.Header) error {
					return fmt.Errorf("tar write header failed")
				},
			}
		}

		// When creating an artifact
		_, err := builder.Create(outputPath, "test:1.0.0")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for tar write header failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write metadata header") {
			t.Errorf("Expected tar write header error, got %v", err)
		}
	})

	t.Run("TarWriteContentError", func(t *testing.T) {
		// Given an artifact with files but tar writer that fails on Write
		builder, mocks := setup(t)
		setupArtifactWithFiles(t, builder)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		headerWritten := false
		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &MockTarWriter{
				WriteHeaderFunc: func(hdr *tar.Header) error {
					return nil
				},
				WriteFunc: func(b []byte) (int, error) {
					if !headerWritten {
						headerWritten = true
						return 0, fmt.Errorf("tar write content failed")
					}
					return len(b), nil
				},
			}
		}

		// When creating an artifact
		_, err := builder.Create(outputPath, "test:1.0.0")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for tar write content failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write metadata") {
			t.Errorf("Expected tar write content error, got %v", err)
		}
	})

	// The remaining test cases are not relevant for the new architecture
	// since we no longer walk build directories - we iterate over stored files
}

// =============================================================================
// Test generateMetadataWithNameVersion
// =============================================================================

func TestArtifactBuilder_generateMetadataWithNameVersion(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims

		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("Failed to initialize builder: %v", err)
		}

		return builder, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given valid input metadata
		builder, _ := setup(t)
		input := BlueprintMetadataInput{
			Description: "A test blueprint",
			Author:      "Test Author",
		}

		// When generating metadata with name and version
		metadata, err := builder.generateMetadataWithNameVersion(input, "test-blueprint", "1.0.0")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if metadata == nil {
			t.Error("Expected metadata to be generated")
		}

		// And metadata should be valid YAML
		var parsedMetadata map[string]any
		if err := yaml.Unmarshal(metadata, &parsedMetadata); err != nil {
			t.Errorf("Expected valid YAML metadata, got %v", err)
		}

		// And should contain expected fields
		if parsedMetadata["name"] != "test-blueprint" {
			t.Errorf("Expected name to be 'test-blueprint', got %v", parsedMetadata["name"])
		}
		if parsedMetadata["version"] != "1.0.0" {
			t.Errorf("Expected version to be '1.0.0', got %v", parsedMetadata["version"])
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		// Given empty input metadata
		builder, _ := setup(t)
		input := BlueprintMetadataInput{}

		// When generating metadata with name and version
		metadata, err := builder.generateMetadataWithNameVersion(input, "test-app", "2.0.0")

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And metadata should contain provided name and version
		var parsedMetadata map[string]any
		if err := yaml.Unmarshal(metadata, &parsedMetadata); err != nil {
			t.Errorf("Expected valid YAML metadata, got %v", err)
		}

		if parsedMetadata["name"] != "test-app" {
			t.Errorf("Expected name to be 'test-app', got %v", parsedMetadata["name"])
		}
		if parsedMetadata["version"] != "2.0.0" {
			t.Errorf("Expected version to be '2.0.0', got %v", parsedMetadata["version"])
		}
	})

	t.Run("YamlMarshalError", func(t *testing.T) {
		// Given valid input but YAML marshal that fails
		builder, _ := setup(t)
		input := BlueprintMetadataInput{
			Description: "A test blueprint",
		}

		// Mock YAML marshal to fail
		builder.shims.YamlMarshal = func(data any) ([]byte, error) {
			return nil, fmt.Errorf("YAML marshal failed")
		}

		// When generating metadata
		_, err := builder.generateMetadataWithNameVersion(input, "test-blueprint", "1.0.0")

		// Then it should fail
		if err == nil {
			t.Error("Expected error for YAML marshal failure, got nil")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArtifactBuilder_getGitProvenance(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		builder := NewArtifactBuilder()

		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("Failed to initialize builder: %v", err)
		}

		return builder, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a builder with working git commands
		builder, _ := setup(t)

		// When getting git provenance
		gitInfo, err := builder.getGitProvenance()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And git info should contain expected values
		if gitInfo.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA 'abc123def456', got %s", gitInfo.CommitSHA)
		}
		if gitInfo.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got %s", gitInfo.Tag)
		}
		if gitInfo.RemoteURL != "https://github.com/test/repo.git" {
			t.Errorf("Expected remote URL 'https://github.com/test/repo.git', got %s", gitInfo.RemoteURL)
		}
	})

	t.Run("CommitSHAError", func(t *testing.T) {
		// Given a builder with failing git rev-parse
		builder, mocks := setup(t)
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) > 0 && args[0] == "rev-parse" {
				return "", fmt.Errorf("not a git repository")
			}
			return "", nil
		}

		// When getting git provenance
		_, err := builder.getGitProvenance()

		// Then it should fail
		if err == nil {
			t.Error("Expected error for commit SHA failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get commit SHA") {
			t.Errorf("Expected commit SHA error, got %v", err)
		}
	})

	t.Run("RemoteURLError", func(t *testing.T) {
		// Given a builder with failing git remote URL
		builder, mocks := setup(t)
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "abc123def456", nil
			case strings.Contains(cmd, "git tag --points-at HEAD"):
				return "v1.0.0", nil
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "", fmt.Errorf("no remote configured")
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting git provenance
		_, err := builder.getGitProvenance()

		// Then it should fail
		if err == nil {
			t.Error("Expected error for remote URL failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get remote URL") {
			t.Errorf("Expected remote URL error, got %v", err)
		}
	})

	t.Run("NoTag", func(t *testing.T) {
		// Given a builder where git tag command fails
		builder, mocks := setup(t)
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git rev-parse HEAD"):
				return "abc123def456", nil
			case strings.Contains(cmd, "git tag --points-at HEAD"):
				return "", fmt.Errorf("no tag found")
			case strings.Contains(cmd, "git config --get remote.origin.url"):
				return "https://github.com/test/repo.git", nil
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting git provenance
		gitInfo, err := builder.getGitProvenance()

		// Then it should succeed with empty tag
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if gitInfo.Tag != "" {
			t.Errorf("Expected empty tag, got %s", gitInfo.Tag)
		}
		if gitInfo.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA to still be set, got %s", gitInfo.CommitSHA)
		}
	})
}

func TestArtifactBuilder_getBuilderInfo(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		builder := NewArtifactBuilder()

		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("Failed to initialize builder: %v", err)
		}

		return builder, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a builder with working git config commands
		builder, _ := setup(t)

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And builder info should contain expected values
		if builderInfo.User != "Test User" {
			t.Errorf("Expected user 'Test User', got %s", builderInfo.User)
		}
		if builderInfo.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got %s", builderInfo.Email)
		}
	})

	t.Run("GitConfigFailure", func(t *testing.T) {
		// Given a builder where git config commands fail
		builder, mocks := setup(t)
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("git config failed")
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then it should succeed with empty values
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if builderInfo.User != "" {
			t.Errorf("Expected empty user, got %s", builderInfo.User)
		}
		if builderInfo.Email != "" {
			t.Errorf("Expected empty email, got %s", builderInfo.Email)
		}
	})

	t.Run("PartialGitConfig", func(t *testing.T) {
		// Given a builder where only user.name is configured
		builder, mocks := setup(t)
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			switch {
			case strings.Contains(cmd, "git config user.name"):
				return "Test User", nil
			case strings.Contains(cmd, "git config user.email"):
				return "", fmt.Errorf("no email configured")
			default:
				return "", fmt.Errorf("unexpected command: %s", cmd)
			}
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then it should succeed with partial info
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if builderInfo.User != "Test User" {
			t.Errorf("Expected user 'Test User', got %s", builderInfo.User)
		}
		if builderInfo.Email != "" {
			t.Errorf("Expected empty email, got %s", builderInfo.Email)
		}
	})
}
