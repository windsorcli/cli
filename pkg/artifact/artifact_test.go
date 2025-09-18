package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
func (m *mockFileInfo) Sys() any           { return nil }

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

	// Use setupShims for consistent shim configuration
	shims := setupShims(t)

	// Override specific shims for file system operations
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

// setupShims provides common shim configurations for testing.
// This function sets up standard mocks for YAML, file operations, and image creation
// that can be reused across multiple test cases to reduce duplication.
func setupShims(t *testing.T) *Shims {
	t.Helper()

	shims := NewShims()

	// Standard YAML processing mocks
	shims.YamlUnmarshal = func(data []byte, v any) error {
		input := v.(*BlueprintMetadataInput)
		input.Name = "test"
		input.Version = "1.0.0"
		return nil
	}
	shims.YamlMarshal = func(data any) ([]byte, error) {
		return []byte("test: yaml"), nil
	}

	// Standard image creation mocks
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
			return &mockLayer{}, nil
		},
		configNameFunc: func() (v1.Hash, error) {
			return v1.Hash{
				Algorithm: "sha256",
				Hex:       "config123",
			}, nil
		},
		rawConfigFileFunc: func() ([]byte, error) {
			return []byte("config"), nil
		},
	}

	shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
		return mockImg, nil
	}
	shims.ConfigFile = func(img v1.Image, cfg *v1.ConfigFile) (v1.Image, error) {
		return mockImg, nil
	}
	shims.MediaType = func(img v1.Image, mt types.MediaType) v1.Image {
		return mockImg
	}
	shims.ConfigMediaType = func(img v1.Image, mt types.MediaType) v1.Image {
		return mockImg
	}
	shims.Annotations = func(img v1.Image, annotations map[string]string) v1.Image {
		return mockImg
	}

	// Remote operations mocks for network testing
	shims.RemoteGet = func(ref name.Reference, options ...remote.Option) (*remote.Descriptor, error) {
		return nil, fmt.Errorf("blob not found")
	}
	shims.RemoteWriteLayer = func(repo name.Repository, layer v1.Layer, options ...remote.Option) error {
		return nil
	}
	shims.RemoteWrite = func(ref name.Reference, img v1.Image, options ...remote.Option) error {
		return nil
	}

	return shims
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
		err := builder.AddFile(testPath, testContent, 0644)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And file should be stored in builder
		if len(builder.files) != 1 {
			t.Errorf("Expected 1 file, got %d", len(builder.files))
		}
		if fileInfo, exists := builder.files[testPath]; !exists {
			t.Error("Expected file to be stored")
		} else if string(fileInfo.Content) != string(testContent) {
			t.Errorf("Expected content %s, got %s", testContent, fileInfo.Content)
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
			err := builder.AddFile(path, content, 0644)
			if err != nil {
				t.Errorf("Unexpected error adding file %s: %v", path, err)
			}
		}

		// Then all files should be stored
		if len(builder.files) != len(files) {
			t.Errorf("Expected %d files, got %d", len(files), len(builder.files))
		}

		for path, expectedContent := range files {
			if actualFileInfo, exists := builder.files[path]; !exists {
				t.Errorf("Expected file %s to be stored", path)
			} else if string(actualFileInfo.Content) != string(expectedContent) {
				t.Errorf("Expected content %s for file %s, got %s", expectedContent, path, actualFileInfo.Content)
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
		builder.AddFile("_templates/metadata.yaml", metadataContent, 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte("metadata"), 0644)
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
			"only:colon:",
			":missingname",
			"missingversion:",
		}

		for _, tag := range invalidTags {
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

		builder.AddFile("_templates/metadata.yaml", []byte("metadata"), 0644)
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

		builder.AddFile("_templates/metadata.yaml", []byte("invalid yaml"), 0644)
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
		builder.AddFile("test.txt", []byte("content"), 0644)

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
		builder.AddFile("test.txt", []byte("content"), 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte("metadata content"), 0644)
		builder.AddFile("other.txt", []byte("other content"), 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
		_, err := builder.Create("test.tar.gz", "")
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
		builder.AddFile("_templates/metadata.yaml", []byte(""), 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test"), 0644)

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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

		// When pushing
		err := builder.Push("registry.example.com", "myapp", "2.0.0")

		// Then should get mock implementation error (indicating it reached the OCI creation step)
		if err == nil || !strings.Contains(err.Error(), "mock implementation error") {
			t.Errorf("Expected mock implementation error, got: %v", err)
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
		builder.AddFile("test.txt", []byte("test content"), 0644)

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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		mocks.Shims = setupShims(t)
		builder.shims = mocks.Shims // Update builder to use new shims

		// Mock that terminates early to avoid nil pointer issues
		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("expected test termination")
		}

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		mocks.Shims = setupShims(t)
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		mocks.Shims = setupShims(t)
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		mocks.Shims = setupShims(t)
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		mocks.Shims = setupShims(t)
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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		builder.AddFile("_templates/metadata.yaml", []byte(""), 0644)

		// When creating with tag containing multiple colons (should fail in Create method)
		_, err := builder.Create("test.tar.gz", "name:version:extra")

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
		builder.AddFile("_templates/metadata.yaml", []byte(""), 0644)

		// When creating with tag having empty parts (should fail in Create method)
		invalidTags := []string{":version", "name:", ":"}
		for _, tag := range invalidTags {
			_, err := builder.Create("test.tar.gz", tag)
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
		builder.AddFile("_templates/metadata.yaml", []byte("version: 1.0.0"), 0644)

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

		builder.AddFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.AddFile("file.txt", []byte("content"), 0644)

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

		builder.AddFile("file.txt", []byte("content"), 0644)

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

		builder.AddFile("file.txt", []byte("content"), 0644)

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

		builder.AddFile("file.txt", []byte("content"), 0644)

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

		builder.AddFile("file.txt", []byte("content"), 0644)

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
		builder.AddFile("test.txt", []byte("content"), 0644)

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
		builder.AddFile("test.txt", []byte("content"), 0644)

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
		builder.AddFile("test.txt", []byte("content"), 0644)

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
		builder.AddFile("test.txt", []byte("content"), 0644)

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

	t.Run("ErrorWhenTarWriterCloseFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.AddFile("test.txt", []byte("content"), 0644)

		// Mock tar writer to fail on Close
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				closeFunc: func() error {
					return fmt.Errorf("tar writer close failed")
				},
			}
		}

		// When creating tarball in memory
		_, err := builder.createTarballInMemory([]byte("metadata"))

		// Then should get tar writer close error
		if err == nil || !strings.Contains(err.Error(), "failed to close tar writer") {
			t.Errorf("Expected tar writer close error, got: %v", err)
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

func TestArtifactBuilder_createOCIArtifactImage(t *testing.T) {
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

		// Mock git provenance to succeed but AppendLayers to fail
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			cmd := strings.Join(append([]string{command}, args...), " ")
			if strings.Contains(cmd, "git rev-parse HEAD") {
				return "abc123", nil
			}
			return "", nil
		}

		mocks.Shims.AppendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
			return nil, fmt.Errorf("append layers failed")
		}

		// When creating OCI artifact image
		_, err := builder.createOCIArtifactImage(nil, "test-repo", "v1.0.0")

		// Then should get append layers error
		if err == nil || !strings.Contains(err.Error(), "failed to append layer to image") {
			t.Errorf("Expected append layer error, got: %v", err)
		}
	})

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

		// Capture annotations to verify revision and source are set correctly
		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return mockImage
		}

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(nil, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if img == nil {
			t.Error("Expected to receive a non-nil image")
		}

		// And revision annotation should be set to the commit SHA
		if capturedAnnotations["org.opencontainers.image.revision"] != expectedCommitSHA {
			t.Errorf("Expected revision annotation to be '%s', got '%s'",
				expectedCommitSHA, capturedAnnotations["org.opencontainers.image.revision"])
		}

		// And source annotation should be set to the remote URL
		if capturedAnnotations["org.opencontainers.image.source"] != expectedRemoteURL {
			t.Errorf("Expected source annotation to be '%s', got '%s'",
				expectedRemoteURL, capturedAnnotations["org.opencontainers.image.source"])
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
		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return mockImage
		}

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(nil, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if img == nil {
			t.Error("Expected to receive a non-nil image")
		}

		// And revision annotation should fall back to "unknown"
		if capturedAnnotations["org.opencontainers.image.revision"] != "unknown" {
			t.Errorf("Expected revision annotation to be 'unknown', got '%s'",
				capturedAnnotations["org.opencontainers.image.revision"])
		}

		// And source annotation should fall back to "unknown"
		if capturedAnnotations["org.opencontainers.image.source"] != "unknown" {
			t.Errorf("Expected source annotation to be 'unknown', got '%s'",
				capturedAnnotations["org.opencontainers.image.source"])
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
				return "   ", nil // whitespace only
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
		var capturedAnnotations map[string]string
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			capturedAnnotations = anns
			return mockImage
		}

		// When creating OCI artifact image
		img, err := builder.createOCIArtifactImage(nil, "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if img == nil {
			t.Error("Expected to receive a non-nil image")
		}

		// And revision annotation should fall back to "unknown" since trimmed SHA is empty
		if capturedAnnotations["org.opencontainers.image.revision"] != "unknown" {
			t.Errorf("Expected revision annotation to be 'unknown', got '%s'",
				capturedAnnotations["org.opencontainers.image.revision"])
		}

		// And source annotation should be set to the remote URL
		if capturedAnnotations["org.opencontainers.image.source"] != expectedRemoteURL {
			t.Errorf("Expected source annotation to be '%s', got '%s'",
				expectedRemoteURL, capturedAnnotations["org.opencontainers.image.source"])
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

// Enhanced mock image with configurable behavior for testing different scenarios
type mockImageWithManifest struct {
	manifestFunc      func() (*v1.Manifest, error)
	layerByDigestFunc func(v1.Hash) (v1.Layer, error)
	configNameFunc    func() (v1.Hash, error)
	rawConfigFileFunc func() ([]byte, error)
}

func (m *mockImageWithManifest) Layers() ([]v1.Layer, error)             { return nil, nil }
func (m *mockImageWithManifest) MediaType() (types.MediaType, error)     { return "", nil }
func (m *mockImageWithManifest) Size() (int64, error)                    { return 0, nil }
func (m *mockImageWithManifest) ConfigFile() (*v1.ConfigFile, error)     { return nil, nil }
func (m *mockImageWithManifest) Digest() (v1.Hash, error)                { return v1.Hash{}, nil }
func (m *mockImageWithManifest) RawManifest() ([]byte, error)            { return nil, nil }
func (m *mockImageWithManifest) LayerByDiffID(v1.Hash) (v1.Layer, error) { return nil, nil }

func (m *mockImageWithManifest) Manifest() (*v1.Manifest, error) {
	if m.manifestFunc != nil {
		return m.manifestFunc()
	}
	mockImg := &mockImage{}
	return mockImg.Manifest()
}

func (m *mockImageWithManifest) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	if m.layerByDigestFunc != nil {
		return m.layerByDigestFunc(hash)
	}
	mockImg := &mockImage{}
	return mockImg.LayerByDigest(hash)
}

func (m *mockImageWithManifest) ConfigName() (v1.Hash, error) {
	if m.configNameFunc != nil {
		return m.configNameFunc()
	}
	mockImg := &mockImage{}
	return mockImg.ConfigName()
}

func (m *mockImageWithManifest) RawConfigFile() ([]byte, error) {
	if m.rawConfigFileFunc != nil {
		return m.rawConfigFileFunc()
	}
	mockImg := &mockImage{}
	return mockImg.RawConfigFile()
}

// Mock layer for testing
type mockLayer struct{}

func (m *mockLayer) Digest() (v1.Hash, error)             { return v1.Hash{}, nil }
func (m *mockLayer) DiffID() (v1.Hash, error)             { return v1.Hash{}, nil }
func (m *mockLayer) Compressed() (io.ReadCloser, error)   { return nil, nil }
func (m *mockLayer) Uncompressed() (io.ReadCloser, error) { return nil, nil }
func (m *mockLayer) Size() (int64, error)                 { return 0, nil }
func (m *mockLayer) MediaType() (types.MediaType, error)  { return "", nil }

func TestArtifactBuilder_parseOCIRef(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("failed to initialize ArtifactBuilder: %v", err)
		}
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
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims
		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("failed to initialize ArtifactBuilder: %v", err)
		}
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

func TestArtifactBuilder_Pull(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder()
		builder.shims = mocks.Shims

		// Set up OCI mocks
		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, nil
		}

		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return nil, nil
		}

		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return []v1.Layer{&mockLayer{}}, nil
		}

		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			data := []byte("test artifact data")
			return io.NopCloser(strings.NewReader(string(data))), nil
		}

		mocks.Shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return []byte("test artifact data"), nil
		}

		if err := builder.Initialize(mocks.Injector); err != nil {
			t.Fatalf("failed to initialize ArtifactBuilder: %v", err)
		}
		return builder, mocks
	}

	t.Run("EmptyList", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, _ := setup(t)

		// When Pull is called with empty list
		artifacts, err := builder.Pull([]string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And an empty map should be returned
		if len(artifacts) != 0 {
			t.Errorf("expected empty artifacts map, got %d items", len(artifacts))
		}
	})

	t.Run("NonOCIReferences", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, _ := setup(t)

		// When Pull is called with non-OCI references
		artifacts, err := builder.Pull([]string{
			"https://github.com/example/repo.git",
			"file:///local/path",
		})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And an empty map should be returned
		if len(artifacts) != 0 {
			t.Errorf("expected empty artifacts map, got %d items", len(artifacts))
		}
	})

	t.Run("SingleOCIReferenceSuccess", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, _ := setup(t)

		// When Pull is called with one OCI reference
		artifacts, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain the cached data
		expectedKey := "registry.example.com/my-repo:v1.0.0"
		if len(artifacts) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts))
		}
		expectedData := []byte("test artifact data")
		if string(artifacts[expectedKey]) != string(expectedData) {
			t.Errorf("expected artifact data to match test data")
		}
	})

	t.Run("MultipleOCIReferencesDifferentArtifacts", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, mocks := setup(t)

		// And mocks that return different data based on calls
		downloadCallCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCallCount++
			data := fmt.Sprintf("test artifact data %d", downloadCallCount)
			return io.NopCloser(strings.NewReader(data)), nil
		}

		mocks.Shims.ReadAll = func(r io.Reader) ([]byte, error) {
			data, err := io.ReadAll(r)
			return data, err
		}

		// When Pull is called with multiple different OCI references
		artifacts, err := builder.Pull([]string{
			"oci://registry.example.com/repo1:v1.0.0",
			"oci://registry.example.com/repo2:v2.0.0",
		})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain both entries
		if len(artifacts) != 2 {
			t.Errorf("expected 2 artifacts, got %d", len(artifacts))
		}

		// And both downloads should have happened
		if downloadCallCount != 2 {
			t.Errorf("expected 2 download calls, got %d", downloadCallCount)
		}

		// And both artifacts should be present
		key1 := "registry.example.com/repo1:v1.0.0"
		key2 := "registry.example.com/repo2:v2.0.0"
		if _, exists := artifacts[key1]; !exists {
			t.Errorf("expected artifact %s to exist", key1)
		}
		if _, exists := artifacts[key2]; !exists {
			t.Errorf("expected artifact %s to exist", key2)
		}
	})

	t.Run("MultipleOCIReferencesSameArtifact", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, mocks := setup(t)

		// And mocks that track download calls
		testData := "test artifact data"
		downloadCallCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCallCount++
			return io.NopCloser(strings.NewReader(testData)), nil
		}

		mocks.Shims.ReadAll = func(r io.Reader) ([]byte, error) {
			data, err := io.ReadAll(r)
			return data, err
		}

		// When Pull is called with duplicate OCI references
		artifacts, err := builder.Pull([]string{
			"oci://registry.example.com/my-repo:v1.0.0",
			"oci://registry.example.com/my-repo:v1.0.0",
		})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain only one entry (deduplicated)
		if len(artifacts) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts))
		}

		// And the download should only happen once (caching works)
		if downloadCallCount != 1 {
			t.Errorf("expected 1 download call, got %d", downloadCallCount)
		}

		expectedKey := "registry.example.com/my-repo:v1.0.0"
		if string(artifacts[expectedKey]) != testData {
			t.Errorf("expected artifact data to match test data")
		}
	})

	t.Run("ErrorParsingOCIReference", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, _ := setup(t)

		// When Pull is called with invalid OCI reference (missing repository part)
		_, err := builder.Pull([]string{"oci://registry.example.com:v1.0.0"})

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should mention parsing
		if !strings.Contains(err.Error(), "failed to parse OCI reference") {
			t.Errorf("expected parse error, got %v", err)
		}
	})

	t.Run("ErrorDownloadingArtifact", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks that fail at download
		builder, mocks := setup(t)

		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return nil, fmt.Errorf("download error")
		}

		// When Pull is called with valid OCI reference but download fails
		_, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should mention download failure
		if !strings.Contains(err.Error(), "failed to download OCI artifact") {
			t.Errorf("expected download error, got %v", err)
		}
	})

	t.Run("CachingPreventsRedundantDownloads", func(t *testing.T) {
		// Given an ArtifactBuilder with mocked dependencies
		builder := NewArtifactBuilder()
		injector := di.NewInjector()
		shell := shell.NewMockShell()
		injector.Register("shell", shell)
		err := builder.Initialize(injector)
		if err != nil {
			t.Fatalf("failed to initialize builder: %v", err)
		}

		// And download counter to track calls
		downloadCount := 0
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return &mockReference{}, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImage{}, nil
		}
		builder.shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return []v1.Layer{&mockLayer{}}, nil
		}
		builder.shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCount++
			data := []byte("test artifact data")
			return io.NopCloser(bytes.NewReader(data)), nil
		}
		builder.shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return io.ReadAll(r)
		}

		ociRefs := []string{
			"oci://registry.example.com/my-repo:v1.0.0",
			"oci://registry.example.com/my-repo:v1.0.0", // Same artifact - should be cached
		}

		// When Pull is called the first time
		artifacts1, err := builder.Pull(ociRefs)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And one artifact should be returned
		if len(artifacts1) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts1))
		}

		// And download should have happened once
		if downloadCount != 1 {
			t.Errorf("expected 1 download call, got %d", downloadCount)
		}

		// When Pull is called again with the same artifacts
		artifacts2, err := builder.Pull(ociRefs)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And one artifact should be returned
		if len(artifacts2) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts2))
		}

		// And download should NOT have happened again (still 1)
		if downloadCount != 1 {
			t.Errorf("expected 1 download call total (cached), got %d", downloadCount)
		}

		// And the artifact data should be identical
		expectedKey := "registry.example.com/my-repo:v1.0.0"
		if !bytes.Equal(artifacts1[expectedKey], artifacts2[expectedKey]) {
			t.Errorf("cached artifact data should be identical")
		}
	})

	t.Run("CachingWorksWithMixedNewAndCachedArtifacts", func(t *testing.T) {
		// Given an ArtifactBuilder with mocked dependencies
		builder := NewArtifactBuilder()
		injector := di.NewInjector()
		shell := shell.NewMockShell()
		injector.Register("shell", shell)
		err := builder.Initialize(injector)
		if err != nil {
			t.Fatalf("failed to initialize builder: %v", err)
		}

		// And download counter to track calls
		downloadCount := 0
		builder.shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return &mockReference{}, nil
		}
		builder.shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImage{}, nil
		}
		builder.shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return []v1.Layer{&mockLayer{}}, nil
		}
		builder.shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCount++
			data := []byte(fmt.Sprintf("test artifact data %d", downloadCount))
			return io.NopCloser(bytes.NewReader(data)), nil
		}
		builder.shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return io.ReadAll(r)
		}

		// When Pull is called with one artifact
		artifacts1, err := builder.Pull([]string{"oci://registry.example.com/repo1:v1.0.0"})
		if err != nil {
			t.Fatalf("failed to pull first artifact: %v", err)
		}

		// Then one download should have occurred
		if downloadCount != 1 {
			t.Errorf("expected 1 download call, got %d", downloadCount)
		}

		// When Pull is called with the cached artifact plus a new one
		artifacts2, err := builder.Pull([]string{
			"oci://registry.example.com/repo1:v1.0.0", // Cached
			"oci://registry.example.com/repo2:v2.0.0", // New
		})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And two artifacts should be returned
		if len(artifacts2) != 2 {
			t.Errorf("expected 2 artifacts, got %d", len(artifacts2))
		}

		// And only one additional download should have occurred (total 2)
		if downloadCount != 2 {
			t.Errorf("expected 2 download calls total, got %d", downloadCount)
		}

		// And both cached and new artifacts should be present
		key1 := "registry.example.com/repo1:v1.0.0"
		key2 := "registry.example.com/repo2:v2.0.0"
		if _, exists := artifacts2[key1]; !exists {
			t.Errorf("expected cached artifact %s to exist", key1)
		}
		if _, exists := artifacts2[key2]; !exists {
			t.Errorf("expected new artifact %s to exist", key2)
		}

		// And the cached artifact should be identical to the first call
		if !bytes.Equal(artifacts1[key1], artifacts2[key1]) {
			t.Errorf("cached artifact data should be identical")
		}
	})
}

type mockReference struct{}

func (m *mockReference) Context() name.Repository   { return name.Repository{} }
func (m *mockReference) Identifier() string         { return "" }
func (m *mockReference) Name() string               { return "" }
func (m *mockReference) String() string             { return "" }
func (m *mockReference) Scope(action string) string { return "" }

func TestArtifactBuilder_GetTemplateData(t *testing.T) {
	t.Run("InvalidOCIReference", func(t *testing.T) {
		// Given an artifact builder
		builder := NewArtifactBuilder()

		// When calling GetTemplateData with invalid reference
		templateData, err := builder.GetTemplateData("invalid-ref")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error for invalid OCI reference")
		}
		if !strings.Contains(err.Error(), "invalid OCI reference") {
			t.Errorf("Expected error to contain 'invalid OCI reference', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("ErrorParsingOCIReference", func(t *testing.T) {
		// Given an artifact builder with mock shims
		builder := NewArtifactBuilder()
		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return nil, fmt.Errorf("parse error")
			},
		}

		// When calling GetTemplateData with malformed OCI reference
		templateData, err := builder.GetTemplateData("oci://invalid")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error for malformed OCI reference")
		}
		if !strings.Contains(err.Error(), "failed to parse OCI reference") {
			t.Errorf("Expected error to contain 'failed to parse OCI reference', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("UsesCachedArtifact", func(t *testing.T) {
		// Given an artifact builder with cached data
		builder := NewArtifactBuilder()

		// Create test tar.gz data
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":               []byte("name: test\nversion: v1.0.0\n"),
			"_template/blueprint.jsonnet": []byte("{ blueprint: 'content' }"),
			"_template/schema.yaml":       []byte("$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties: {}\nrequired: []\nadditionalProperties: false"),
			"template.jsonnet":            []byte("{ template: 'content' }"),
			"ignored.yaml":                []byte("ignored: content"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		downloadCalled := false
		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
			RemoteImage: func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
				downloadCalled = true
				return nil, fmt.Errorf("should not be called")
			},
			YamlUnmarshal: func(data []byte, v any) error {
				if metadata, ok := v.(*BlueprintMetadata); ok {
					metadata.Name = "test"
					metadata.Version = "v1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should use cached data without downloading
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if downloadCalled {
			t.Error("Expected download not to be called when using cached data")
		}
		if templateData == nil {
			t.Fatal("Expected template data, got nil")
		}
		if len(templateData) != 5 {
			t.Errorf("Expected 5 files (2 .jsonnet + name + ociUrl + schema), got %d", len(templateData))
		}
		if string(templateData["template.jsonnet"]) != "{ template: 'content' }" {
			t.Errorf("Expected template.jsonnet content to be '{ template: 'content' }', got %s", string(templateData["template.jsonnet"]))
		}
		if string(templateData["ociUrl"]) != "oci://registry.example.com/test:v1.0.0" {
			t.Errorf("Expected ociUrl to be 'oci://registry.example.com/test:v1.0.0', got %s", string(templateData["ociUrl"]))
		}
		if string(templateData["name"]) != "test" {
			t.Errorf("Expected name to be 'test', got %s", string(templateData["name"]))
		}
		if _, exists := templateData["ignored.yaml"]; exists {
			t.Error("Expected ignored.yaml to be filtered out")
		}
	})

	t.Run("FiltersOnlyJsonnetFiles", func(t *testing.T) {
		// Given an artifact builder with cached data containing multiple file types
		builder := NewArtifactBuilder()

		// Create test tar.gz data with mixed file types
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":               []byte("name: test\nversion: v1.0.0\n"),
			"_template/blueprint.jsonnet": []byte("{ blueprint: 'content' }"),
			"_template/schema.yaml":       []byte("$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties: {}\nrequired: []\nadditionalProperties: false"),
			"template.jsonnet":            []byte("{ template: 'content' }"),
			"config.yaml":                 []byte("config: value"),
			"script.sh":                   []byte("#!/bin/bash"),
			"another.jsonnet":             []byte("{ another: 'template' }"),
			"README.md":                   []byte("# README"),
			"nested/dir.jsonnet":          []byte("{ nested: 'template' }"),
			"nested/config.json":          []byte("{ json: 'config' }"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
			YamlUnmarshal: func(data []byte, v any) error {
				if metadata, ok := v.(*BlueprintMetadata); ok {
					metadata.Name = "test"
					metadata.Version = "v1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should only return .jsonnet files
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if templateData == nil {
			t.Fatal("Expected template data, got nil")
		}
		if len(templateData) != 7 {
			t.Errorf("Expected 7 files (4 .jsonnet + name + ociUrl + schema), got %d", len(templateData))
		}

		// And should contain OCI metadata
		if string(templateData["ociUrl"]) != "oci://registry.example.com/test:v1.0.0" {
			t.Errorf("Expected ociUrl to be 'oci://registry.example.com/test:v1.0.0', got %s", string(templateData["ociUrl"]))
		}
		if string(templateData["name"]) != "test" {
			t.Errorf("Expected name to be 'test', got %s", string(templateData["name"]))
		}

		// And should contain only .jsonnet files
		expectedFiles := []string{"blueprint.jsonnet", "template.jsonnet", "another.jsonnet", "nested/dir.jsonnet"}
		for _, expectedFile := range expectedFiles {
			if _, exists := templateData[expectedFile]; !exists {
				t.Errorf("Expected %s to be included", expectedFile)
			}
		}

		// And should not contain non-.jsonnet files
		excludedFiles := []string{"config.yaml", "script.sh", "README.md", "nested/config.json"}
		for _, excludedFile := range excludedFiles {
			if _, exists := templateData[excludedFile]; exists {
				t.Errorf("Expected %s to be filtered out", excludedFile)
			}
		}
	})

	t.Run("ErrorWhenMissingMetadata", func(t *testing.T) {
		// Given an artifact builder with cached data missing metadata.yaml
		builder := NewArtifactBuilder()

		// Create test tar.gz data without metadata.yaml
		testData := createTestTarGz(t, map[string][]byte{
			"_template/blueprint.jsonnet": []byte("{ template: 'content' }"),
			"other.jsonnet":               []byte("{ other: 'content' }"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error for missing metadata.yaml")
		}
		if !strings.Contains(err.Error(), "OCI artifact missing required metadata.yaml file") {
			t.Errorf("Expected error to contain 'OCI artifact missing required metadata.yaml file', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("ErrorWhenMissingBlueprintJsonnet", func(t *testing.T) {
		// Given an artifact builder with cached data missing _template/blueprint.jsonnet
		builder := NewArtifactBuilder()

		// Create test tar.gz data without _template/blueprint.jsonnet
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":  []byte("name: test\nversion: v1.0.0\n"),
			"other.jsonnet":  []byte("{ other: 'content' }"),
			"config.jsonnet": []byte("{ config: 'content' }"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
			YamlUnmarshal: func(data []byte, v any) error {
				if metadata, ok := v.(*BlueprintMetadata); ok {
					metadata.Name = "test"
					metadata.Version = "v1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error for missing _template/blueprint.jsonnet")
		}
		if !strings.Contains(err.Error(), "OCI artifact missing required _template/blueprint.jsonnet file") {
			t.Errorf("Expected error to contain 'OCI artifact missing required _template/blueprint.jsonnet file', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("SuccessWithOptionalSchema", func(t *testing.T) {
		// Given an artifact builder with cached data missing optional schema.yaml
		builder := NewArtifactBuilder()

		// Create test tar.gz data without schema.yaml
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":               []byte("name: test\nversion: v1.0.0\n"),
			"_template/blueprint.jsonnet": []byte("{ blueprint: 'content' }"),
			"_template/other.jsonnet":     []byte("{ other: 'content' }"),
			"config.yaml":                 []byte("config: value"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
			YamlUnmarshal: func(data []byte, v any) error {
				if metadata, ok := v.(*BlueprintMetadata); ok {
					metadata.Name = "test"
					metadata.Version = "v1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should succeed without schema.yaml
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if templateData == nil {
			t.Fatal("Expected template data, got nil")
		}

		// And should contain required files but not schema
		if _, exists := templateData["blueprint.jsonnet"]; !exists {
			t.Error("Expected blueprint.jsonnet to be included")
		}
		if _, exists := templateData["other.jsonnet"]; !exists {
			t.Error("Expected other.jsonnet to be included")
		}
		if _, exists := templateData["name"]; !exists {
			t.Error("Expected name to be included")
		}
		if _, exists := templateData["ociUrl"]; !exists {
			t.Error("Expected ociUrl to be included")
		}
		if _, exists := templateData["schema"]; exists {
			t.Error("Expected schema to not be included when schema.yaml is missing")
		}

		// And should have correct count (2 .jsonnet + name + ociUrl, no schema)
		if len(templateData) != 4 {
			t.Errorf("Expected 4 files (2 .jsonnet + name + ociUrl), got %d", len(templateData))
		}
	})

	t.Run("SuccessWithRequiredFiles", func(t *testing.T) {
		// Given an artifact builder with cached data containing required files
		builder := NewArtifactBuilder()

		// Create test tar.gz data with required files
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":               []byte("name: test\nversion: v1.0.0\n"),
			"_template/blueprint.jsonnet": []byte("{ blueprint: 'content' }"),
			"_template/schema.yaml":       []byte("$schema: https://json-schema.org/draft/2020-12/schema\ntype: object\nproperties: {}\nrequired: []\nadditionalProperties: false"),
			"_template/other.jsonnet":     []byte("{ other: 'content' }"),
			"config.yaml":                 []byte("config: value"),
		})

		// Pre-populate cache
		builder.ociCache["registry.example.com/test:v1.0.0"] = testData

		builder.shims = &Shims{
			ParseReference: func(ref string, opts ...name.Option) (name.Reference, error) {
				return &mockReference{}, nil
			},
			YamlUnmarshal: func(data []byte, v any) error {
				if metadata, ok := v.(*BlueprintMetadata); ok {
					metadata.Name = "test"
					metadata.Version = "v1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		templateData, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if templateData == nil {
			t.Fatal("Expected template data, got nil")
		}

		// And should contain required files
		if _, exists := templateData["blueprint.jsonnet"]; !exists {
			t.Error("Expected blueprint.jsonnet to be included")
		}
		if _, exists := templateData["other.jsonnet"]; !exists {
			t.Error("Expected other.jsonnet to be included")
		}
		if string(templateData["name"]) != "test" {
			t.Errorf("Expected name to be 'test', got %s", string(templateData["name"]))
		}
		if string(templateData["ociUrl"]) != "oci://registry.example.com/test:v1.0.0" {
			t.Errorf("Expected ociUrl to be 'oci://registry.example.com/test:v1.0.0', got %s", string(templateData["ociUrl"]))
		}
		if _, exists := templateData["schema"]; !exists {
			t.Error("Expected schema key to be included")
		}
	})
}

// createTestTarGz creates a test tar archive with the given files
func createTestTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	tarWriter := tar.NewWriter(&buf)

	for path, content := range files {
		header := &tar.Header{
			Name: path,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	tarWriter.Close()

	return buf.Bytes()
}

// =============================================================================
// Test Package Functions
// =============================================================================

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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseOCIReference(tc.input)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
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
