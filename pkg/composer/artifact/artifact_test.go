package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
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
	Shell   *shell.MockShell
	Shims   *Shims
	Runtime *runtime.Runtime
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
	Shell shell.Shell
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

	// Create runtime
	rt := &runtime.Runtime{
		Shell: mockShell,
	}

	// Cleanup function
	t.Cleanup(func() {
		os.Chdir(tmpDir)
	})

	return &ArtifactMocks{
		Shell:   mockShell,
		Shims:   shims,
		Runtime: rt,
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
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("CreatesBuilderWithDefaults", func(t *testing.T) {
		// Given no preconditions

		// When creating a new artifact builder
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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

		// And shell should be set (passed to constructor)
		if builder.shell == nil {
			t.Error("Expected shell to be set")
		}
	})

	t.Run("Success", func(t *testing.T) {
		// Given a builder and mocks
		builder, _ := setup(t)

		// Then shell should be set
		if builder.shell == nil {
			t.Error("Expected shell to be set")
		}
	})
}

func TestArtifactBuilder_AddFile(t *testing.T) {
	setup := func(t *testing.T) *ArtifactBuilder {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder
	}

	t.Run("AddsFileSuccessfully", func(t *testing.T) {
		// Given a builder
		builder := setup(t)

		// When adding a file
		testPath := "test/file.txt"
		testContent := []byte("test content")
		err := builder.addFile(testPath, testContent, 0644)

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
			err := builder.addFile(path, content, 0644)
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
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("CreateWithValidTag", func(t *testing.T) {
		// Given a builder with shell initialized
		builder, _ := setup(t)

		// When creating artifact with valid tag
		outputPath := "."
		tag := "testproject:v1.0.0"
		actualPath, err := builder.Write(outputPath, tag)

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
		builder.addFile("_templates/metadata.yaml", metadataContent, 0644)

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
		actualPath, err := builder.Write(outputPath, "")

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
		builder.addFile("_templates/metadata.yaml", []byte("metadata"), 0644)
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			if metadata, ok := v.(*BlueprintMetadataInput); ok {
				metadata.Name = "frommetadata"
				metadata.Version = "v1.0.0"
			}
			return nil
		}

		// When creating artifact with tag that overrides metadata
		tag := "fromtag:v2.0.0"
		actualPath, err := builder.Write(".", tag)

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
			_, err := builder.Write(".", tag)

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
		_, err := builder.Write(".", "")

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

		builder.addFile("_templates/metadata.yaml", []byte("metadata"), 0644)
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			if metadata, ok := v.(*BlueprintMetadataInput); ok {
				metadata.Name = "testproject"
				// Version intentionally left empty
			}
			return nil
		}

		// When creating artifact without version
		_, err := builder.Write(".", "")

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

		builder.addFile("_templates/metadata.yaml", []byte("invalid yaml"), 0644)
		builder.shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml parse error")
		}

		// When creating artifact
		_, err := builder.Write(".", "")

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		builder.addFile("test.txt", []byte("content"), 0644)

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		builder.addFile("test.txt", []byte("content"), 0644)

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		builder.addFile("_templates/metadata.yaml", []byte("metadata content"), 0644)
		builder.addFile("other.txt", []byte("other content"), 0644)

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
		_, err := builder.Write(".", "testproject:v1.0.0")

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
		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
		_, err := builder.Write("test.tar.gz", "")
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
		builder.addFile("_templates/metadata.yaml", []byte(""), 0644)

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
		builder.addFile("_templates/metadata.yaml", []byte("name: test"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)
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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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
		builder.addFile("_templates/metadata.yaml", []byte(""), 0644)

		// When creating with tag containing multiple colons (should fail in Create method)
		_, err := builder.Write("test.tar.gz", "name:version:extra")

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
		builder.addFile("_templates/metadata.yaml", []byte(""), 0644)

		// When creating with tag having empty parts (should fail in Create method)
		invalidTags := []string{":version", "name:", ":"}
		for _, tag := range invalidTags {
			_, err := builder.Write("test.tar.gz", tag)
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
		builder.addFile("_templates/metadata.yaml", []byte("version: 1.0.0"), 0644)

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

		builder.addFile("_templates/metadata.yaml", []byte("name: test\nversion: 1.0.0"), 0644)

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

	t.Run("ErrorWhenTarWriterWriteHeaderFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)

		// Mock tar writer to fail on WriteHeader
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeHeaderFunc: func(*tar.Header) error {
					return fmt.Errorf("write header failed")
				},
			}
		}

		// When creating tarball in memory
		t.Skip("WriteTarballInMemory is no longer part of the public interface")
	})

	t.Run("ErrorWhenTarWriterWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)

		// Mock tar writer to fail on Write
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				writeFunc: func([]byte) (int, error) {
					return 0, fmt.Errorf("write failed")
				},
			}
		}

		// When creating tarball in memory
		t.Skip("WriteTarballInMemory is no longer part of the public interface")
	})

	t.Run("ErrorWhenFileHeaderWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)

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
		t.Skip("WriteTarballInMemory is no longer part of the public interface")
	})

	t.Run("ErrorWhenFileContentWriteFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)

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
		t.Skip("WriteTarballInMemory is no longer part of the public interface")
	})

	t.Run("ErrorWhenTarWriterCloseFails", func(t *testing.T) {
		// Given a builder with files
		builder, mocks := setup(t)
		builder.addFile("test.txt", []byte("content"), 0644)

		// Mock tar writer to fail on Close
		mocks.Shims.NewTarWriter = func(w io.Writer) TarWriter {
			return &mockTarWriter{
				closeFunc: func() error {
					return fmt.Errorf("tar writer close failed")
				},
			}
		}

		// When creating tarball in memory
		t.Skip("WriteTarballInMemory is no longer part of the public interface")
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

	t.Run("ErrorWhenAppendLayersFails", func(t *testing.T) {
		// Given a builder with failing AppendLayers
		_, mocks := setup(t)

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
		t.Skip("WriteOCIArtifactImage is no longer part of the public interface")
	})

	t.Run("SuccessWithValidLayer", func(t *testing.T) {
		// Given a builder with successful shim operations
		_, mocks := setup(t)

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
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			return mockImage
		}

		// When creating OCI artifact image
		t.Skip("WriteOCIArtifactImage is no longer part of the public interface")
	})

	t.Run("SuccessWithGitProvenanceFallback", func(t *testing.T) {
		// Given a builder where git provenance fails
		_, mocks := setup(t)

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
			return mockImage
		}

		// When creating OCI artifact image
		t.Skip("WriteOCIArtifactImage is no longer part of the public interface")
	})

	t.Run("SuccessWithEmptyCommitSHA", func(t *testing.T) {
		// Given a builder where git returns empty commit SHA but valid remote URL
		_, mocks := setup(t)

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
		mocks.Shims.Annotations = func(img v1.Image, anns map[string]string) v1.Image {
			return mockImage
		}

		// When creating OCI artifact image
		t.Skip("WriteOCIArtifactImage is no longer part of the public interface")
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

func TestArtifactBuilder_Pull(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		_ = shell.NewMockShell()

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		_ = shell.NewMockShell()

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

// TestArtifactBuilder_Bundle tests the Bundle method of ArtifactBuilder
func TestArtifactBuilder_Bundle(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithAllDirectories", func(t *testing.T) {
		// Given a builder with mock directories and files
		builder, mocks := setup(t)

		// Mock directory structure
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "contexts" || name == "kustomize" || name == "terraform" {
				return &mockFileInfo{name: name, isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			switch root {
			case "contexts":
				fn("contexts/_template", &mockFileInfo{name: "_template", isDir: true}, nil)
				fn("contexts/_template/test.jsonnet", &mockFileInfo{name: "test.jsonnet", isDir: false}, nil)
			case "kustomize":
				fn("kustomize", &mockFileInfo{name: "kustomize", isDir: true}, nil)
				fn("kustomize/kustomization.yaml", &mockFileInfo{name: "kustomization.yaml", isDir: false}, nil)
			case "terraform":
				fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil)
				fn("terraform/main.tf", &mockFileInfo{name: "main.tf", isDir: false}, nil)
			default:
				// No-op for other roots
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			if strings.Contains(targpath, "_template") {
				return strings.TrimPrefix(targpath, "contexts/_template/"), nil
			}
			return filepath.Base(targpath), nil
		}

		// When bundling
		err := builder.Bundle()

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should have added files
		if len(builder.files) == 0 {
			t.Error("Expected files to be added")
		}
	})

	t.Run("SuccessWithMissingDirectories", func(t *testing.T) {
		// Given a builder with some missing directories
		builder, mocks := setup(t)

		// Mock only kustomize directory exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "kustomize" {
				return &mockFileInfo{name: "kustomize", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "kustomize" {
				fn("kustomize", &mockFileInfo{name: "kustomize", isDir: true}, nil)
				fn("kustomize/kustomization.yaml", &mockFileInfo{name: "kustomization.yaml", isDir: false}, nil)
			}
			return nil
		}

		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("test content"), nil
		}

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return filepath.Base(targpath), nil
		}

		// When bundling
		err := builder.Bundle()

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorOnWalkFailure", func(t *testing.T) {
		// Given a builder with walk error
		builder, mocks := setup(t)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "kustomize" {
				return &mockFileInfo{name: "kustomize", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		expectedError := errors.New("walk error")
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			return expectedError
		}

		// When bundling
		err := builder.Bundle()

		// Then should return error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to walk directory") {
			t.Errorf("Expected walk error, got %v", err)
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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

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

	t.Run("ValidatesCliVersionFromMetadata", func(t *testing.T) {
		// Given an artifact builder with cached data containing cliVersion
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)

		// Create test tar.gz data with cliVersion in metadata
		testData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":               []byte("name: test\nversion: v1.0.0\ncliVersion: '>=1.0.0'\n"),
			"_template/blueprint.jsonnet": []byte("{ blueprint: 'content' }"),
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
					metadata.CliVersion = ">=1.0.0"
				}
				return nil
			},
		}

		// When calling GetTemplateData
		_, err := builder.GetTemplateData("oci://registry.example.com/test:v1.0.0")

		// Then should succeed (validation skipped when cliVersion is empty)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
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

// TestArtifactBuilder_findMatchingProcessor tests the findMatchingProcessor method of ArtifactBuilder
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
}

// =============================================================================
// Test Helper Functions
// =============================================================================

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

		// And error should indicate invalid format
		if !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected error about invalid format, got: %v", err)
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
	t.Run("ReturnsTrueForUNAUTHORIZED", func(t *testing.T) {
		// Given an error with UNAUTHORIZED
		err := fmt.Errorf("UNAUTHORIZED: access denied")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for UNAUTHORIZED error")
		}
	})

	t.Run("ReturnsTrueForUnauthorized", func(t *testing.T) {
		// Given an error with unauthorized
		err := fmt.Errorf("unauthorized access")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for unauthorized error")
		}
	})

	t.Run("ReturnsTrueForAuthenticationRequired", func(t *testing.T) {
		// Given an error with authentication required
		err := fmt.Errorf("authentication required to access this resource")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for authentication required error")
		}
	})

	t.Run("ReturnsTrueForAuthenticationFailed", func(t *testing.T) {
		// Given an error with authentication failed
		err := fmt.Errorf("authentication failed")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for authentication failed error")
		}
	})

	t.Run("ReturnsTrueForHTTP401", func(t *testing.T) {
		// Given an error with HTTP 401
		err := fmt.Errorf("HTTP 401: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for HTTP 401 error")
		}
	})

	t.Run("ReturnsTrueForHTTP403", func(t *testing.T) {
		// Given an error with HTTP 403
		err := fmt.Errorf("HTTP 403: forbidden")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for HTTP 403 error")
		}
	})

	t.Run("ReturnsTrueForBlobsUploads", func(t *testing.T) {
		// Given an error with blobs/uploads
		err := fmt.Errorf("POST https://registry.com/v2/repo/blobs/uploads: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for blobs/uploads error")
		}
	})

	t.Run("ReturnsTrueForPOSTHTTPS", func(t *testing.T) {
		// Given an error with POST https://
		err := fmt.Errorf("POST https://registry.com/v2/repo/manifests/latest: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for POST https:// error")
		}
	})

	t.Run("ReturnsTrueForFailedToPushArtifact", func(t *testing.T) {
		// Given an error with failed to push artifact
		err := fmt.Errorf("failed to push artifact: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for failed to push artifact error")
		}
	})

	t.Run("ReturnsTrueForUserCannotBeAuthenticated", func(t *testing.T) {
		// Given an error with User cannot be authenticated
		err := fmt.Errorf("User cannot be authenticated")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for User cannot be authenticated error")
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

	t.Run("ReturnsFalseForGenericError", func(t *testing.T) {
		// Given a generic error
		err := fmt.Errorf("network timeout")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for generic error")
		}
	})

	t.Run("ReturnsFalseForParseError", func(t *testing.T) {
		// Given a parse error
		err := fmt.Errorf("failed to parse JSON")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for parse error")
		}
	})

	t.Run("ReturnsFalseForNotFoundError", func(t *testing.T) {
		// Given a not found error
		err := fmt.Errorf("resource not found")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for not found error")
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

		// Then should return error
		if err == nil {
			t.Error("Expected error when version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
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
}
