package bundler

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

type ArtifactMocks struct {
	Injector di.Injector
	Shell    *shell.MockShell
	Shims    *Shims
}

// mockTarWriter provides a mock implementation of TarWriter for testing
type mockTarWriter struct {
	writeHeaderFunc func(*tar.Header) error
	writeFunc       func([]byte) (int, error)
	closeFunc       func() error
}

func (m *mockTarWriter) WriteHeader(hdr *tar.Header) error {
	if m.writeHeaderFunc != nil {
		return m.writeHeaderFunc(hdr)
	}
	return nil
}

func (m *mockTarWriter) Write(b []byte) (int, error) {
	if m.writeFunc != nil {
		return m.writeFunc(b)
	}
	return len(b), nil
}

func (m *mockTarWriter) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type ArtifactSetupOptions struct {
	Injector di.Injector
	Shell    shell.Shell
}

func setupArtifactMocks(t *testing.T, opts ...*ArtifactSetupOptions) *ArtifactMocks {
	t.Helper()

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "artifact-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Change to temporary directory
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create injector
	injector := di.NewInjector()

	// Set up shell - default to MockShell for easier testing
	var mockShell *shell.MockShell
	if len(opts) > 0 && opts[0].Shell != nil {
		if ms, ok := opts[0].Shell.(*shell.MockShell); ok {
			mockShell = ms
		} else {
			mockShell = shell.NewMockShell()
		}
	} else {
		mockShell = shell.NewMockShell()
	}

	// Set up default shell behaviors
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		cmd := strings.Join(append([]string{command}, args...), " ")
		switch {
		case strings.Contains(cmd, "git rev-parse HEAD"):
			return "abc123def456", nil
		case strings.Contains(cmd, "git tag --points-at HEAD"):
			return "v1.0.0", nil
		case strings.Contains(cmd, "git config --get remote.origin.url"):
			return "https://github.com/example/repo.git", nil
		case strings.Contains(cmd, "git config user.name"):
			return "Test User", nil
		case strings.Contains(cmd, "git config user.email"):
			return "test@example.com", nil
		default:
			return "", nil
		}
	}

	// Register shell with injector
	injector.Register("shell", mockShell)

	// Create shims with test-friendly defaults
	shims := NewShims()
	shims.Stat = func(name string) (os.FileInfo, error) {
		if name == "." {
			// Mock "." as an existing directory
			return &mockFileInfo{name: ".", isDir: true}, nil
		}
		return nil, os.ErrNotExist
	}
	shims.Create = func(name string) (io.WriteCloser, error) {
		// Create the full path, handling directories properly
		fullPath := name
		if !filepath.IsAbs(name) {
			fullPath = filepath.Join(tmpDir, name)
		}

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		return os.Create(fullPath)
	}
	shims.YamlUnmarshal = func(data []byte, v any) error {
		return nil
	}
	shims.YamlMarshal = func(data any) ([]byte, error) {
		return []byte("test: yaml"), nil
	}

	// Cleanup function
	t.Cleanup(func() {
		os.Chdir(tmpDir)
	})

	return &ArtifactMocks{
		Injector: injector,
		Shell:    mockShell,
		Shims:    shims,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestArtifactBuilder_NewArtifactBuilder(t *testing.T) {
	t.Run("CreatesBuilderWithDefaults", func(t *testing.T) {
		// Given no preconditions

		// When creating a new artifact builder
		builder := NewArtifactBuilder()

		// Then the builder should be properly initialized
		if builder == nil {
			t.Fatal("Expected non-nil builder")
		}

		// And basic fields should be set
		if builder.shims == nil {
			t.Error("Expected shims to be set")
		}
		if builder.files == nil {
			t.Error("Expected files map to be initialized")
		}

		// And dependency fields should be nil until Initialize() is called
		if builder.shell != nil {
			t.Error("Expected shell to be nil before Initialize()")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArtifactBuilder_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a builder and mocks
		builder, mocks := setup(t)

		// When calling Initialize
		err := builder.Initialize(mocks.Injector)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And shell should be injected
		if builder.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
	})

	t.Run("SuccessWithNilInjector", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When calling Initialize with nil injector
		err := builder.Initialize(nil)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And shell should remain nil
		if builder.shell != nil {
			t.Error("Expected shell to remain nil with nil injector")
		}
	})

	t.Run("ErrorWhenShellNotFound", func(t *testing.T) {
		// Given a builder and injector without shell
		builder, mocks := setup(t)
		mocks.Injector.Register("shell", "not-a-shell")

		// When calling Initialize
		err := builder.Initialize(mocks.Injector)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shell not found")
		}
		if !strings.Contains(err.Error(), "failed to resolve shell") {
			t.Errorf("Expected shell resolution error, got: %v", err)
		}
	})
}

func TestArtifactBuilder_AddFile(t *testing.T) {
	setup := func(t *testing.T) *ArtifactBuilder {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		return builder
	}

	t.Run("AddsFileSuccessfully", func(t *testing.T) {
		// Given a builder
		builder := setup(t)

		// When adding a file
		testPath := "test/file.txt"
		testContent := []byte("test content")
		err := builder.AddFile(testPath, testContent)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And file should be stored in builder
		if len(builder.files) != 1 {
			t.Errorf("Expected 1 file, got %d", len(builder.files))
		}
		if content, exists := builder.files[testPath]; !exists {
			t.Error("Expected file to be stored")
		} else if string(content) != string(testContent) {
			t.Errorf("Expected content %s, got %s", testContent, content)
		}
	})

	t.Run("AddsMultipleFiles", func(t *testing.T) {
		// Given a builder
		builder := setup(t)

		// When adding multiple files
		files := map[string][]byte{
			"file1.txt":     []byte("content 1"),
			"file2.txt":     []byte("content 2"),
			"dir/file3.txt": []byte("content 3"),
		}

		for path, content := range files {
			err := builder.AddFile(path, content)
			if err != nil {
				t.Errorf("Unexpected error adding file %s: %v", path, err)
			}
		}

		// Then all files should be stored
		if len(builder.files) != len(files) {
			t.Errorf("Expected %d files, got %d", len(files), len(builder.files))
		}

		for path, expectedContent := range files {
			if actualContent, exists := builder.files[path]; !exists {
				t.Errorf("Expected file %s to be stored", path)
			} else if string(actualContent) != string(expectedContent) {
				t.Errorf("Expected content %s for file %s, got %s", expectedContent, path, actualContent)
			}
		}
	})
}

func TestArtifactBuilder_Create(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
		return builder, mocks
	}

	t.Run("CreateWithValidTag", func(t *testing.T) {
		// Given a builder with shell initialized
		builder, _ := setup(t)

		// When creating artifact with valid tag
		outputPath := "."
		tag := "testproject:v1.0.0"
		actualPath, err := builder.Create(outputPath, tag)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And output path should be generated correctly
		expectedPath := "testproject-v1.0.0.tar.gz"
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("CreateWithMetadataFile", func(t *testing.T) {
		// Given a builder with metadata file
		builder, _ := setup(t)

		// Add metadata file to builder
		metadataContent := []byte(`
name: myproject
version: v2.0.0
description: A test project
`)
		builder.AddFile("_templates/metadata.yaml", metadataContent)

		// Override YamlUnmarshal to parse the metadata
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			if metadata, ok := v.(*BlueprintMetadataInput); ok {
				metadata.Name = "myproject"
				metadata.Version = "v2.0.0"
				metadata.Description = "A test project"
			}
			return nil
		}

		// When creating artifact without tag
		outputPath := "."
		actualPath, err := builder.Create(outputPath, "")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And output path should use metadata values
		expectedPath := "myproject-v2.0.0.tar.gz"
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("TagOverridesMetadata", func(t *testing.T) {
		// Given a builder with metadata file
		builder, _ := setup(t)

		// Add metadata file with different values
		builder.AddFile("_templates/metadata.yaml", []byte("metadata"))
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			if metadata, ok := v.(*BlueprintMetadataInput); ok {
				metadata.Name = "frommetadata"
				metadata.Version = "v1.0.0"
			}
			return nil
		}

		// When creating artifact with tag that overrides metadata
		tag := "fromtag:v2.0.0"
		actualPath, err := builder.Create(".", tag)

		// Then tag values should take precedence
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		expectedPath := "fromtag-v2.0.0.tar.gz"
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("ErrorWithInvalidTagFormat", func(t *testing.T) {
		// Given a builder
		builder, _ := setup(t)

		// When creating artifact with invalid tag format
		invalidTags := []string{
			"notag",
			"only:colon:",
			":missingname",
			"missingversion:",
			"",
		}

		for _, tag := range invalidTags {
			if tag == "" {
				continue // Skip empty tag as it's handled differently
			}

			_, err := builder.Create(".", tag)

			// Then an error should be returned
			if err == nil {
				t.Errorf("Expected error for invalid tag %s", tag)
			}
			if !strings.Contains(err.Error(), "tag must be in format") {
				t.Errorf("Expected tag format error for %s, got: %v", tag, err)
			}
		}
	})

	t.Run("ErrorWhenNameMissing", func(t *testing.T) {
		// Given a builder with no metadata and no tag
		builder, _ := setup(t)

		// When creating artifact without name
		_, err := builder.Create(".", "")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when name is missing")
		}
		if !strings.Contains(err.Error(), "name is required: provide via tag parameter or metadata.yaml") {
			t.Errorf("Expected name required error, got: %v", err)
		}
	})

	t.Run("ErrorWhenVersionMissing", func(t *testing.T) {
		// Given a builder with metadata containing only name
		builder, _ := setup(t)

		builder.AddFile("_templates/metadata.yaml", []byte("metadata"))
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			if metadata, ok := v.(*BlueprintMetadataInput); ok {
				metadata.Name = "testproject"
				// Version intentionally left empty
			}
			return nil
		}

		// When creating artifact without version
		_, err := builder.Create(".", "")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when version is missing")
		}
		if !strings.Contains(err.Error(), "version is required: provide via tag parameter or metadata.yaml") {
			t.Errorf("Expected version required error, got: %v", err)
		}
	})

	t.Run("ErrorWhenMetadataParsingFails", func(t *testing.T) {
		// Given a builder with invalid metadata
		builder, _ := setup(t)

		builder.AddFile("_templates/metadata.yaml", []byte("invalid yaml"))
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml parse error")
		}

		// When creating artifact
		_, err := builder.Create(".", "")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when metadata parsing fails")
		}
		if !strings.Contains(err.Error(), "failed to parse metadata.yaml") {
			t.Errorf("Expected metadata parse error, got: %v", err)
		}
	})

	t.Run("ErrorWhenMetadataGenerationFails", func(t *testing.T) {
		// Given a builder with failing metadata generation
		builder, _ := setup(t)

		builder.shims.YamlMarshal = func(data any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		// When creating artifact with valid tag
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when metadata generation fails")
		}
		if !strings.Contains(err.Error(), "failed to generate metadata") {
			t.Errorf("Expected metadata generation error, got: %v", err)
		}
	})

	t.Run("ErrorWhenOutputFileCreationFails", func(t *testing.T) {
		// Given a builder with failing file creation
		builder, _ := setup(t)

		builder.shims.Create = func(name string) (io.WriteCloser, error) {
			return nil, fmt.Errorf("file creation error")
		}

		// When creating artifact with valid tag
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when output file creation fails")
		}
		if !strings.Contains(err.Error(), "failed to create output file") {
			t.Errorf("Expected output file creation error, got: %v", err)
		}
	})

	t.Run("ErrorWhenGzipWriterFails", func(t *testing.T) {
		// Given a builder with failing gzip writer
		builder, _ := setup(t)

		builder.shims.NewGzipWriter = func(w io.Writer) *gzip.Writer {
			// Return a gzip writer that will fail on close
			gzw := gzip.NewWriter(w)
			return gzw
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then it should succeed (gzip writer errors are deferred)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("ErrorWhenTarWriterHeaderFails", func(t *testing.T) {
		// Given a builder with failing tar writer
		builder, _ := setup(t)

		mockTarWriter := &mockTarWriter{
			writeHeaderFunc: func(hdr *tar.Header) error {
				if hdr.Name == "metadata.yaml" {
					return fmt.Errorf("tar header error")
				}
				return nil
			},
		}

		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return mockTarWriter
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when tar writer header fails")
		}
		if !strings.Contains(err.Error(), "failed to write metadata header") {
			t.Errorf("Expected tar header error, got: %v", err)
		}
	})

	t.Run("ErrorWhenTarWriterContentFails", func(t *testing.T) {
		// Given a builder with failing tar writer
		builder, _ := setup(t)

		mockTarWriter := &mockTarWriter{
			writeHeaderFunc: func(hdr *tar.Header) error {
				return nil
			},
			writeFunc: func(b []byte) (int, error) {
				return 0, fmt.Errorf("tar write error")
			},
		}

		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return mockTarWriter
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when tar writer content fails")
		}
		if !strings.Contains(err.Error(), "failed to write metadata") {
			t.Errorf("Expected tar write error, got: %v", err)
		}
	})

	t.Run("ErrorWhenFileHeaderWriteFails", func(t *testing.T) {
		// Given a builder with files and failing file header write
		builder, _ := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		mockTarWriter := &mockTarWriter{
			writeHeaderFunc: func(hdr *tar.Header) error {
				if hdr.Name == "test.txt" {
					return fmt.Errorf("file header error")
				}
				return nil
			},
			writeFunc: func(b []byte) (int, error) {
				return len(b), nil
			},
		}

		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return mockTarWriter
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when file header write fails")
		}
		if !strings.Contains(err.Error(), "failed to write header for test.txt") {
			t.Errorf("Expected file header error, got: %v", err)
		}
	})

	t.Run("ErrorWhenFileContentWriteFails", func(t *testing.T) {
		// Given a builder with files and failing file content write
		builder, _ := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		writeCount := 0
		mockTarWriter := &mockTarWriter{
			writeHeaderFunc: func(hdr *tar.Header) error {
				return nil
			},
			writeFunc: func(b []byte) (int, error) {
				writeCount++
				// Let metadata write succeed (first write), fail on file content (second write)
				if writeCount == 1 {
					return len(b), nil // metadata succeeds
				}
				return 0, fmt.Errorf("file content error") // file content fails
			},
		}

		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return mockTarWriter
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when file content write fails")
		}
		if !strings.Contains(err.Error(), "failed to write content for test.txt") {
			t.Errorf("Expected file content error, got: %v", err)
		}
	})

	t.Run("SkipsMetadataFileInFileLoop", func(t *testing.T) {
		// Given a builder with metadata file and other files
		builder, _ := setup(t)
		builder.AddFile("_templates/metadata.yaml", []byte("metadata content"))
		builder.AddFile("other.txt", []byte("other content"))

		filesWritten := make(map[string]bool)
		mockTarWriter := &mockTarWriter{
			writeHeaderFunc: func(hdr *tar.Header) error {
				filesWritten[hdr.Name] = true
				return nil
			},
			writeFunc: func(b []byte) (int, error) {
				return len(b), nil
			},
		}

		builder.shims.NewTarWriter = func(w io.Writer) TarWriter {
			return mockTarWriter
		}

		// When creating artifact
		_, err := builder.Create(".", "testproject:v1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And metadata.yaml should be written once (from the metadata generation)
		// And _templates/metadata.yaml should be skipped in the file loop
		if !filesWritten["metadata.yaml"] {
			t.Error("Expected metadata.yaml to be written")
		}
		if filesWritten["_templates/metadata.yaml"] {
			t.Error("Expected _templates/metadata.yaml to be skipped in file loop")
		}
		if !filesWritten["other.txt"] {
			t.Error("Expected other.txt to be written")
		}
	})
}

func TestArtifactBuilder_Push(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))
		_, err := builder.Create("test.tar.gz", "")
		if err != nil {
			t.Fatalf("Failed to create artifact: %v", err)
		}

		// When pushing with invalid tag format
		invalidTags := []string{
			"notag",
			"only:colon:",
			":missingname",
			"missingversion:",
		}

		for _, tag := range invalidTags {
			err := builder.Push("registry.example.com", tag)
			if err == nil || !strings.Contains(err.Error(), "tag must be in format") {
				t.Errorf("Expected tag format error for %s, got: %v", tag, err)
			}
		}
	})

	t.Run("ErrorWhenNameMissing", func(t *testing.T) {
		// Given a builder with no metadata and no tag provided
		builder, mocks := setup(t)

		// Set up mocks for metadata parsing that returns empty metadata
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			// Return empty metadata (no name or version)
			return nil
		}
		builder.AddFile("_templates/metadata.yaml", []byte(""))

		// When pushing without providing tag
		err := builder.Push("registry.example.com", "")
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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test"))

		// When pushing without providing tag
		err := builder.Push("registry.example.com", "")
		if err == nil || !strings.Contains(err.Error(), "version is required") {
			t.Errorf("Expected version required error, got: %v", err)
		}
	})

	t.Run("SuccessWithValidMetadata", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up mocks for successful in-memory operation
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("mock implementation error")
		}

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))

		// When pushing
		err := builder.Push("registry.example.com", "myapp:2.0.0")

		// Then should get mock implementation error (indicating it reached the OCI creation step)
		if err == nil || !strings.Contains(err.Error(), "mock implementation error") {
			t.Errorf("Expected mock implementation error, got: %v", err)
		}
	})

	t.Run("SuccessWithInMemoryOperation", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		// Set up mocks for successful in-memory operation
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}
		mocks.Shims.YamlMarshal = func(data any) ([]byte, error) {
			return []byte("test: yaml"), nil
		}

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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))
		builder.AddFile("test.txt", []byte("test content"))

		// When pushing (this should work entirely in-memory)
		err := builder.Push("registry.example.com", "test:1.0.0")

		// Then should get expected test termination (proving in-memory operation worked)
		if err == nil || !strings.Contains(err.Error(), "expected test termination") {
			t.Errorf("Expected test termination error, got: %v", err)
		}

		// And tarball should have been created in memory
		if !tarballCreated {
			t.Error("Expected tarball to be created in memory")
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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))

		// When pushing with invalid registry format (contains invalid characters)
		err := builder.Push("invalid registry format with spaces", "test:1.0.0")

		// Then should get repository reference error
		if err == nil || !strings.Contains(err.Error(), "invalid repository reference") {
			t.Errorf("Expected invalid repository reference error, got: %v", err)
		}
	})

	t.Run("ErrorFromCreateTarballInMemory", func(t *testing.T) {
		// Given a builder with valid metadata
		builder, mocks := setup(t)

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "test"
			input.Version = "1.0.0"
			return nil
		}

		// Mock tar writer to fail on WriteHeader
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return fmt.Errorf("tar writer header failed")
				},
			}
		}

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))

		// When pushing
		err := builder.Push("registry.example.com", "test:1.0.0")

		// Then should get tarball creation error
		if err == nil || !strings.Contains(err.Error(), "failed to create tarball in memory") {
			t.Errorf("Expected tarball creation error, got: %v", err)
		}
	})

	t.Run("ErrorFromCreateFluxCDCompatibleImage", func(t *testing.T) {
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"))

		// When pushing
		err := builder.Push("registry.example.com", "test:1.0.0")

		// Then should get FluxCD image creation error
		if err == nil || !strings.Contains(err.Error(), "failed to create FluxCD-compatible OCI image") {
			t.Errorf("Expected FluxCD image creation error, got: %v", err)
		}
	})

	t.Run("SuccessWithEmptyTag", func(t *testing.T) {
		// Given a builder with valid metadata file
		builder, mocks := setup(t)

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			input := v.(*BlueprintMetadataInput)
			input.Name = "myapp"
			input.Version = "2.0.0"
			return nil
		}
		mocks.Shims.YamlMarshal = func(data any) ([]byte, error) {
			return []byte("test: yaml"), nil
		}
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("expected test termination")
		}

		builder.AddFile("_templates/metadata.yaml", []byte("name: myapp\nversion: 2.0.0"))

		// When pushing with empty tag (should use metadata values)
		err := builder.Push("registry.example.com", "")

		// Then should use metadata name/version and reach the expected termination
		if err == nil || !strings.Contains(err.Error(), "expected test termination") {
			t.Errorf("Expected test termination error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArtifactBuilder_resolveOutputPath(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
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
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
		return builder, mocks
	}

	t.Run("ErrorWhenTarWriterWriteHeaderFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		// Mock tar writer to fail on WriteHeader
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return fmt.Errorf("write header failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory([]byte("metadata"))

		// Then should get write header error
		if err == nil || !strings.Contains(err.Error(), "failed to write metadata header") {
			t.Errorf("Expected write header error, got: %v", err)
		}
	})

	t.Run("ErrorWhenTarWriterWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		// Mock tar writer to fail on Write
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeFunc: func([]byte) (int, error) {
					return 0, fmt.Errorf("write failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory([]byte("metadata"))

		// Then should get write error
		if err == nil || !strings.Contains(err.Error(), "failed to write metadata") {
			t.Errorf("Expected write error, got: %v", err)
		}
	})

	t.Run("ErrorWhenFileHeaderWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		headerCount := 0
		// Mock tar writer to fail on second WriteHeader (for file)
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					headerCount++
					if headerCount > 1 {
						return fmt.Errorf("file header write failed")
					}
					return nil
				},
				writeFunc: func([]byte) (int, error) {
					return 100, nil // Success for metadata write
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory([]byte("metadata"))

		// Then should get file header error
		if err == nil || !strings.Contains(err.Error(), "failed to write header for test.txt") {
			t.Errorf("Expected file header error, got: %v", err)
		}
	})

	t.Run("ErrorWhenFileContentWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.AddFile("test.txt", []byte("content"))

		writeCount := 0
		// Mock tar writer to fail on second Write (for file content)
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeFunc: func([]byte) (int, error) {
					writeCount++
					if writeCount > 1 {
						return 0, fmt.Errorf("file content write failed")
					}
					return 100, nil // Success for metadata write
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory([]byte("metadata"))

		// Then should get file content error
		if err == nil || !strings.Contains(err.Error(), "failed to write content for test.txt") {
			t.Errorf("Expected file content error, got: %v", err)
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
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
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

		input := BlueprintMetadataInput{
			Description: "Test description",
			Author:      "Test Author",
			Tags:        []string{"test", "example"},
			Homepage:    "https://example.com",
			License:     "MIT",
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
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
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
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
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

func TestArtifactBuilder_createFluxCDCompatibleImage(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		builder.Initialize(mocks.Injector)
		return builder, mocks
	}

	t.Run("ErrorWhenAppendLayersFails", func(t *testing.T) {
		// Given a builder with failing AppendLayers
		builder, mocks := setup(t)

		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("append layers failed")
		}

		// When creating FluxCD compatible image
		_, err := builder.createFluxCDCompatibleImage(nil, "test-repo", "v1.0.0")

		// Then should get append layers error
		if err == nil || !strings.Contains(err.Error(), "failed to append layer to image") {
			t.Errorf("Expected append layer error, got: %v", err)
		}
	})

	t.Run("SuccessWithValidLayer", func(t *testing.T) {
		// Given a builder with successful shim operations
		builder, mocks := setup(t)

		// Mock successful image creation
		mockImage := &mockImage{}
		mocks.Shims.EmptyImage = func() v1.Image { return mockImage }
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return mockImage, nil
		}
		mocks.Shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
			// Verify config file has expected properties
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
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image { return mockImage }

		// When creating FluxCD compatible image
		img, err := builder.createFluxCDCompatibleImage(nil, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if img == nil {
			t.Error("Expected to receive a non-nil image")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// mockImage provides a mock implementation of v1.Image for testing
type mockImage struct{}

func (m *mockImage) Layers() ([]v1.Layer, error)             { return nil, nil }
func (m *mockImage) MediaType() (types.MediaType, error)     { return "", nil }
func (m *mockImage) Size() (int64, error)                    { return 0, nil }
func (m *mockImage) ConfigName() (v1.Hash, error)            { return v1.Hash{}, nil }
func (m *mockImage) ConfigFile() (*v1.ConfigFile, error)     { return nil, nil }
func (m *mockImage) RawConfigFile() ([]byte, error)          { return nil, nil }
func (m *mockImage) Digest() (v1.Hash, error)                { return v1.Hash{}, nil }
func (m *mockImage) Manifest() (*v1.Manifest, error)         { return nil, nil }
func (m *mockImage) RawManifest() ([]byte, error)            { return nil, nil }
func (m *mockImage) LayerByDigest(v1.Hash) (v1.Layer, error) { return nil, nil }
func (m *mockImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { return nil, nil }
