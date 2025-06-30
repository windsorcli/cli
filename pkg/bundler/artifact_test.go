package bundler

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ArtifactBuilder tests provide comprehensive test coverage for artifact creation functionality.
// It provides test utilities and mock configurations for validating artifact operations.
// The test suite verifies metadata generation, tar.gz creation, and git information extraction.
// It ensures proper error handling, edge case coverage, and adherence to Windsor CLI style guidelines.

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
func (m *mockFileInfo) Sys() interface{}   { return nil }

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

func setupBuildDirectory(t *testing.T, mocks *Mocks) string {
	t.Helper()
	buildDir := filepath.Join(mocks.TempDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("Failed to create build directory: %v", err)
	}

	blueprintContent := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
  description: A test blueprint
spec:
  components: []
`

	blueprintPath := filepath.Join(buildDir, "blueprint.yaml")
	if err := os.WriteFile(blueprintPath, []byte(blueprintContent), 0644); err != nil {
		t.Fatalf("Failed to create blueprint.yaml: %v", err)
	}

	testFilePath := filepath.Join(buildDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	return buildDir
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
		// Given a valid build directory
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output file should exist
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Error("Expected output file to exist")
		}
	})

	t.Run("InvalidBuildDirectory", func(t *testing.T) {
		// Given an invalid build directory
		builder, mocks := setup(t)
		buildDir := filepath.Join(mocks.TempDir, "nonexistent")
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for invalid build directory, got nil")
		}
		if !strings.Contains(err.Error(), "failed to generate metadata") {
			t.Errorf("Expected metadata generation error, got %v", err)
		}
	})

	t.Run("OutputPathError", func(t *testing.T) {
		// Given a valid build directory but invalid output path
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := "/invalid/path/output.tar.gz"

		// Mock file creation to fail
		builder.shims.Create = func(name string) (io.WriteCloser, error) {
			return nil, fmt.Errorf("permission denied")
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for invalid output path, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create output file") {
			t.Errorf("Expected output file creation error, got %v", err)
		}
	})

	t.Run("MetadataGenerationError", func(t *testing.T) {
		// Given a build directory without blueprint.yaml
		builder, mocks := setup(t)
		buildDir := filepath.Join(mocks.TempDir, "build")
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			t.Fatalf("Failed to create build directory: %v", err)
		}
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for missing blueprint.yaml, got nil")
		}
		if !strings.Contains(err.Error(), "failed to generate metadata") {
			t.Errorf("Expected metadata generation error, got %v", err)
		}
	})

	t.Run("TarWriteHeaderError", func(t *testing.T) {
		// Given a valid build directory but tar writer that fails on WriteHeader
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
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
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for tar write header failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write metadata header") {
			t.Errorf("Expected tar write header error, got %v", err)
		}
	})

	t.Run("TarWriteContentError", func(t *testing.T) {
		// Given a valid build directory but tar writer that fails on Write
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
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
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for tar write content failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write metadata") {
			t.Errorf("Expected tar write content error, got %v", err)
		}
	})

	t.Run("WalkError", func(t *testing.T) {
		// Given a valid build directory but walk function that fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock walk to fail
		builder.shims.Walk = func(root string, walkFn filepath.WalkFunc) error {
			return fmt.Errorf("walk failed")
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for walk failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk build directory") {
			t.Errorf("Expected walk error, got %v", err)
		}
	})

	t.Run("WalkFilePathRelError", func(t *testing.T) {
		// Given a valid build directory but FilepathRel that fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock FilepathRel to fail
		builder.shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("filepath rel failed")
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for filepath rel failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get relative path") {
			t.Errorf("Expected filepath rel error, got %v", err)
		}
	})

	t.Run("WalkFileInfoHeaderError", func(t *testing.T) {
		// Given a directory with a file that causes tar.FileInfoHeader to fail
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Create a subdirectory to test directory handling
		subDir := filepath.Join(buildDir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should succeed and handle directories
		if err != nil {
			t.Errorf("Expected no error for directory handling, got %v", err)
		}
	})

	t.Run("WalkFileOpenError", func(t *testing.T) {
		// Given a valid build directory but file open that fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock file open to fail for non-directory files
		builder.shims.Open = func(name string) (io.ReadCloser, error) {
			return nil, fmt.Errorf("file open failed")
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for file open failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to open file") {
			t.Errorf("Expected file open error, got %v", err)
		}
	})

	t.Run("WalkFileCopyError", func(t *testing.T) {
		// Given a valid build directory but copy that fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock copy to fail
		builder.shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, fmt.Errorf("copy failed")
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for copy failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to copy file") {
			t.Errorf("Expected copy error, got %v", err)
		}
	})

	t.Run("WalkFunctionError", func(t *testing.T) {
		// Given a valid build directory but walk function that calls the walkfn with an error
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Mock walk to call walkfn with an error
		builder.shims.Walk = func(root string, walkFn filepath.WalkFunc) error {
			// Simulate walk calling the function with an error
			return walkFn(root, nil, fmt.Errorf("walk function error"))
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for walk function error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk build directory") {
			t.Errorf("Expected walk error, got %v", err)
		}
	})

	t.Run("WalkTarFileInfoHeaderError", func(t *testing.T) {
		// Given a valid build directory but tar.FileInfoHeader fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Create a test file to work with
		testFile := filepath.Join(buildDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Mock walk to use a problematic file info that causes tar.FileInfoHeader to fail
		builder.shims.Walk = func(root string, walkFn filepath.WalkFunc) error {
			// First call with root directory (should be skipped)
			err := walkFn(root, &mockFileInfo{name: ".", isDir: true}, nil)
			if err != nil {
				return err
			}

			// Second call with a file that will cause tar.FileInfoHeader to fail
			problematicInfo := &mockFileInfo{
				name:  "test.txt",
				size:  -1, // negative size should cause tar.FileInfoHeader to fail
				mode:  0644,
				isDir: false,
			}
			return walkFn(testFile, problematicInfo, nil)
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for tar FileInfoHeader failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write header for test.txt") {
			t.Errorf("Expected tar write header error, got %v", err)
		}
	})

	t.Run("CreateWithFileCloseError", func(t *testing.T) {
		// Given a valid build directory but file operations that test close operations
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)
		outputPath := filepath.Join(mocks.TempDir, "output.tar.gz")

		// Create a subdirectory and file to ensure all code paths are covered
		subDir := filepath.Join(buildDir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		testFile := filepath.Join(subDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Mock file close to simulate potential close issues (but don't fail)
		fileCloseCount := 0
		builder.shims.Open = func(name string) (io.ReadCloser, error) {
			file, err := os.Open(name)
			if err != nil {
				return nil, err
			}
			return &mockReadCloser{
				ReadCloser: file,
				CloseFunc: func() error {
					fileCloseCount++
					return file.Close()
				},
			}, nil
		}

		// When creating an artifact
		err := builder.Create(buildDir, outputPath)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And files should have been opened and closed
		if fileCloseCount == 0 {
			t.Error("Expected files to be opened and closed")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArtifactBuilder_generateMetadata(t *testing.T) {
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
		// Given a valid build directory
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)

		// When generating metadata
		metadata, err := builder.generateMetadata(buildDir)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And metadata should be valid JSON
		var metadataObj BlueprintMetadata
		if err := json.Unmarshal(metadata, &metadataObj); err != nil {
			t.Errorf("Expected valid JSON metadata, got error: %v", err)
		}

		// And metadata should contain expected values
		if metadataObj.Version != "v1" {
			t.Errorf("Expected version 'v1', got %s", metadataObj.Version)
		}
		if metadataObj.Name != "test-blueprint" {
			t.Errorf("Expected name 'test-blueprint', got %s", metadataObj.Name)
		}
		if metadataObj.Git.CommitSHA != "abc123def456" {
			t.Errorf("Expected commit SHA 'abc123def456', got %s", metadataObj.Git.CommitSHA)
		}
		if metadataObj.Builder.User != "Test User" {
			t.Errorf("Expected user 'Test User', got %s", metadataObj.Builder.User)
		}
	})

	t.Run("MissingBlueprintFile", func(t *testing.T) {
		// Given a build directory without blueprint.yaml
		builder, mocks := setup(t)
		buildDir := filepath.Join(mocks.TempDir, "build")
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			t.Fatalf("Failed to create build directory: %v", err)
		}

		// When generating metadata
		_, err := builder.generateMetadata(buildDir)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for missing blueprint.yaml, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read blueprint.yaml") {
			t.Errorf("Expected blueprint read error, got %v", err)
		}
	})

	t.Run("InvalidBlueprintYAML", func(t *testing.T) {
		// Given a build directory with invalid blueprint.yaml
		builder, mocks := setup(t)
		buildDir := filepath.Join(mocks.TempDir, "build")
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			t.Fatalf("Failed to create build directory: %v", err)
		}

		blueprintPath := filepath.Join(buildDir, "blueprint.yaml")
		if err := os.WriteFile(blueprintPath, []byte("invalid yaml content ["), 0644); err != nil {
			t.Fatalf("Failed to create invalid blueprint.yaml: %v", err)
		}

		// When generating metadata
		_, err := builder.generateMetadata(buildDir)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for invalid blueprint.yaml, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse blueprint.yaml") {
			t.Errorf("Expected blueprint parse error, got %v", err)
		}
	})

	t.Run("GitProvenanceError", func(t *testing.T) {
		// Given a build directory but git provenance fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)

		// Mock git command to fail
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "git" && len(args) > 0 && args[0] == "rev-parse" {
				return "", fmt.Errorf("not a git repository")
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When generating metadata
		_, err := builder.generateMetadata(buildDir)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for git provenance failure, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get git information") {
			t.Errorf("Expected git information error, got %v", err)
		}
	})

	t.Run("JsonMarshalError", func(t *testing.T) {
		// Given a valid build directory but JSON marshal that fails
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)

		// Mock JSON marshal to fail
		builder.shims.JsonMarshal = func(data any) ([]byte, error) {
			return nil, fmt.Errorf("json marshal failed")
		}

		// When generating metadata
		_, err := builder.generateMetadata(buildDir)

		// Then it should fail
		if err == nil {
			t.Error("Expected error for JSON marshal failure, got nil")
		}
		if !strings.Contains(err.Error(), "json marshal failed") {
			t.Errorf("Expected JSON marshal error, got %v", err)
		}
	})

	t.Run("BuilderInfoErrorPath", func(t *testing.T) {
		// Given a build directory
		builder, mocks := setup(t)
		buildDir := setupBuildDirectory(t, mocks)

		// The getBuilderInfo method never actually returns an error in current implementation
		// It only calls shell.ExecSilent which can fail, but failures are ignored (lines don't error)
		// So the error path on lines 203-206 may be unreachable with current code
		// This test documents that behavior

		// When generating metadata with normal conditions
		metadata, err := builder.generateMetadata(buildDir)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(metadata) == 0 {
			t.Error("Expected metadata to be generated")
		}
	})
}

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
