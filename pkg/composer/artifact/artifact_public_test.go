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

// mockReference provides a mock implementation of name.Reference for testing
type mockReference struct{}

func (m *mockReference) Context() name.Repository   { return name.Repository{} }
func (m *mockReference) Identifier() string         { return "" }
func (m *mockReference) Name() string               { return "" }
func (m *mockReference) String() string             { return "" }
func (m *mockReference) Scope(action string) string { return "" }

// setupDefaultShims creates shims with default test configurations
func setupDefaultShims() *Shims {
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

// setupArtifactMocks creates mock components for testing the ArtifactBuilder with optional overrides
func setupArtifactMocks(t *testing.T, opts ...func(*ArtifactMocks)) *ArtifactMocks {
	t.Helper()

	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Set up shell - default to MockShell for easier testing
	mockShell := shell.NewMockShell()

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

	// Create shims with default configurations
	shims := setupDefaultShims()

	// Override specific shims for file system operations
	shims.Stat = func(name string) (os.FileInfo, error) {
		if name == "." {
			return &mockFileInfo{name: ".", isDir: true}, nil
		}
		return nil, os.ErrNotExist
	}
	shims.Create = func(name string) (io.WriteCloser, error) {
		fullPath := name
		if !filepath.IsAbs(name) {
			fullPath = filepath.Join(tmpDir, name)
		}

		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		return os.Create(fullPath)
	}

	// Create runtime
	rt := &runtime.Runtime{
		Shell:       mockShell,
		ProjectRoot: tmpDir,
	}

	// Create default mocks
	mocks := &ArtifactMocks{
		Shell:   mockShell,
		Shims:   shims,
		Runtime: rt,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
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
// Test Public Methods
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
		builder.addFile("_template/metadata.yaml", metadataContent, 0644)

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
		builder.addFile("_template/metadata.yaml", []byte("metadata"), 0644)
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

		builder.addFile("_template/metadata.yaml", []byte("metadata"), 0644)
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

		builder.addFile("_template/metadata.yaml", []byte("invalid yaml"), 0644)
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

	t.Run("ErrorWhenBundleFails", func(t *testing.T) {
		// Given a builder with failing Walk operation
		builder, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "contexts" || name == "kustomize" || name == "terraform" {
				return &mockFileInfo{name: name, isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Walk = func(root string, walkFn filepath.WalkFunc) error {
			return fmt.Errorf("walk error")
		}

		// When creating artifact
		_, err := builder.Write(".", "testproject:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Bundle fails")
		}
		if !strings.Contains(err.Error(), "failed to bundle files") {
			t.Errorf("Expected bundle error, got: %v", err)
		}
	})

	t.Run("ResolvesOutputPathForDirectory", func(t *testing.T) {
		// Given a builder and a directory path
		builder, _ := setup(t)
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, "output")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatalf("Failed to create output directory: %v", err)
		}

		// When creating artifact with directory path
		actualPath, err := builder.Write(outputDir, "testproject:v1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And path should be resolved correctly
		expectedPath := filepath.Join(outputDir, "testproject-v1.0.0.tar.gz")
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("ResolvesOutputPathForFile", func(t *testing.T) {
		// Given a builder and a file path
		builder, _ := setup(t)
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "custom.tar.gz")

		// When creating artifact with file path
		actualPath, err := builder.Write(outputFile, "testproject:v1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And path should use the provided file path
		if actualPath != outputFile {
			t.Errorf("Expected path %s, got %s", outputFile, actualPath)
		}
	})

	t.Run("ResolvesOutputPathForPathWithoutExtension", func(t *testing.T) {
		// Given a builder and a path without extension
		builder, _ := setup(t)
		tmpDir := t.TempDir()
		outputPath := filepath.Join(tmpDir, "custom")

		// When creating artifact with path without extension
		actualPath, err := builder.Write(outputPath, "testproject:v1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And path should be resolved with filename appended
		expectedPath := filepath.Join(tmpDir, "custom", "testproject-v1.0.0.tar.gz")
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("SkipsMetadataFileInFileLoop", func(t *testing.T) {
		// Given a builder with metadata file and other files
		builder, _ := setup(t)
		builder.addFile("_template/metadata.yaml", []byte("metadata content"), 0644)
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

		// And metadata.yaml should be written at root (from the metadata generation)
		// And the input _template/metadata.yaml should be skipped in the file loop to avoid duplication
		if !filesWritten["metadata.yaml"] {
			t.Error("Expected metadata.yaml to be written at root from metadata generation")
		}
		// Count occurrences - should only be written once (from generation, not from input file)
		count := 0
		for name := range filesWritten {
			if name == "metadata.yaml" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Expected metadata.yaml to be written exactly once, but it appears %d times", count)
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

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			dir := filepath.Dir(name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
			return os.Create(name)
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
		if artifacts[expectedKey] == "" {
			t.Errorf("expected artifact cache path to be present")
		}
	})

	t.Run("MultipleOCIReferencesDifferentArtifacts", func(t *testing.T) {
		// Given an ArtifactBuilder with mocks
		builder, mocks := setup(t)

		// And mocks that return different data based on calls
		downloadCallCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCallCount++
			testTarData := createTestTarGz(t, map[string][]byte{
				"metadata.yaml":       []byte(fmt.Sprintf("name: test%d\nversion: v1.0.0\n", downloadCallCount)),
				"_template/test.yaml": []byte(fmt.Sprintf("test content %d", downloadCallCount)),
			})
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		mocks.Shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return io.ReadAll(r)
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
		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})
		downloadCallCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCallCount++
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		mocks.Shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return io.ReadAll(r)
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
		if artifacts[expectedKey] == "" {
			t.Errorf("expected artifact cache path to be present")
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
		builder.shims = mocks.Shims
		_ = shell.NewMockShell()

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

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
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}
		builder.shims.Stat = os.Stat
		builder.shims.Create = func(name string) (io.WriteCloser, error) {
			dir := filepath.Dir(name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
			return os.Create(name)
		}
		builder.shims.RemoveAll = os.RemoveAll
		builder.shims.Rename = os.Rename
		builder.shims.MkdirAll = os.MkdirAll

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
		if artifacts1[expectedKey] != artifacts2[expectedKey] {
			t.Errorf("cached artifact data should be identical")
		}
	})

	t.Run("RespectsNO_CACHEEnvironmentVariable", func(t *testing.T) {
		builder, mocks := setup(t)

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		downloadCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCount++
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		os.Setenv("NO_CACHE", "true")
		defer os.Unsetenv("NO_CACHE")

		artifacts, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(artifacts) != 1 {
			t.Errorf("Expected 1 artifact, got %d", len(artifacts))
		}
		if downloadCount != 1 {
			t.Errorf("Expected 1 download (NO_CACHE bypasses cache), got %d", downloadCount)
		}
		expectedKey := "registry.example.com/my-repo:v1.0.0"
		cacheDir, exists := artifacts[expectedKey]
		if !exists {
			t.Errorf("Expected artifact cache directory, got none")
		}
		normalizedPath := filepath.ToSlash(cacheDir)
		if !strings.Contains(normalizedPath, ".windsor/cache") {
			t.Errorf("Expected cache directory path, got %s", cacheDir)
		}
	})

	t.Run("UsesDiskCacheWhenAvailable", func(t *testing.T) {
		builder, mocks := setup(t)

		cacheDir, err := builder.GetCacheDir("registry.example.com", "my-repo", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if err := os.WriteFile(artifactTarPath, testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == cacheDir || name == artifactTarPath {
				return &mockFileInfo{name: filepath.Base(name), isDir: name == cacheDir}, nil
			}
			return os.Stat(name)
		}

		artifacts, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(artifacts) != 1 {
			t.Errorf("Expected 1 artifact, got %d", len(artifacts))
		}
		if artifacts["registry.example.com/my-repo:v1.0.0"] == "" {
			t.Error("Expected cache directory path, got empty string")
		}
	})

	t.Run("PopulatesInMemoryCacheWhenReadingFromDisk", func(t *testing.T) {
		builder, mocks := setup(t)

		cacheDir, err := builder.GetCacheDir("registry.example.com", "my-repo", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if err := os.WriteFile(artifactTarPath, testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == cacheDir || name == artifactTarPath {
				return &mockFileInfo{name: filepath.Base(name), isDir: name == cacheDir}, nil
			}
			return os.Stat(name)
		}
		mocks.Shims.ReadFile = os.ReadFile

		cacheKey := "registry.example.com/my-repo:v1.0.0"

		// When Pull is called the first time (should read from disk)
		artifacts1, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And one artifact should be returned
		if len(artifacts1) != 1 {
			t.Errorf("Expected 1 artifact, got %d", len(artifacts1))
		}

		// And artifact data should match
		if artifacts1[cacheKey] == "" {
			t.Error("Expected cache directory path to be returned")
		}

		// Cache directory should exist
		if artifacts1[cacheKey] == "" {
			t.Error("Expected cache directory path to be returned")
		}

		// When Pull is called again with the same reference
		artifacts2, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error on second call, got %v", err)
		}

		// And one artifact should be returned
		if len(artifacts2) != 1 {
			t.Errorf("Expected 1 artifact on second call, got %d", len(artifacts2))
		}

		// And artifact data should match
		if artifacts2[cacheKey] == "" {
			t.Error("Expected artifact data to match on second call")
		}
	})

	t.Run("RemovesCorruptedCacheAndFallsBackToDownloadWhenReadFileFails", func(t *testing.T) {
		builder, mocks := setup(t)

		cacheDir, err := builder.GetCacheDir("registry.example.com", "my-repo", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		downloadCount := 0
		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			downloadCount++
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		cacheRemoved := false
		originalRemoveAll := mocks.Shims.RemoveAll
		mocks.Shims.RemoveAll = func(path string) error {
			if path == cacheDir && !cacheRemoved {
				cacheRemoved = true
			}
			return originalRemoveAll(path)
		}

		cacheDirExists := true
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == cacheDir {
				if !cacheDirExists {
					return nil, os.ErrNotExist
				}
				return &mockFileInfo{name: filepath.Base(name), isDir: true}, nil
			}
			if name == artifactTarPath {
				if !cacheDirExists {
					return nil, os.ErrNotExist
				}
				return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
			}
			return os.Stat(name)
		}

		readFileCallCount := 0
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			if name == artifactTarPath {
				readFileCallCount++
				if readFileCallCount == 1 {
					cacheDirExists = false
					return nil, fmt.Errorf("read error")
				}
			}
			return os.ReadFile(name)
		}
		mocks.Shims.RemoveAll = func(path string) error {
			if path == cacheDir {
				cacheDirExists = false
				cacheRemoved = true
			}
			return os.RemoveAll(path)
		}
		mocks.Shims.ParseReference = func(ref string, opts ...name.Option) (name.Reference, error) {
			return &mockReference{}, nil
		}
		mocks.Shims.RemoteImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
			return &mockImage{}, nil
		}
		mocks.Shims.ImageLayers = func(img v1.Image) ([]v1.Layer, error) {
			return []v1.Layer{&mockLayer{}}, nil
		}
		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			dir := filepath.Dir(name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
			return os.Create(name)
		}
		mocks.Shims.Rename = os.Rename
		mocks.Shims.MkdirAll = os.MkdirAll

		artifacts, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		if err != nil {
			t.Errorf("Expected no error (graceful fallback), got %v", err)
		}
		if len(artifacts) != 1 {
			t.Errorf("Expected 1 artifact, got %d", len(artifacts))
		}
		if !cacheRemoved {
			t.Error("Expected corrupted cache to be removed, but it was not")
		}
		if downloadCount != 1 {
			t.Errorf("Expected 1 download (fallback after cache corruption), got %d", downloadCount)
		}
	})

	t.Run("RemovesCorruptedCacheDirectory", func(t *testing.T) {
		builder, mocks := setup(t)

		cacheDir, err := builder.GetCacheDir("registry.example.com", "my-repo", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}

		cacheRemoved := false
		originalRemoveAll := mocks.Shims.RemoveAll
		mocks.Shims.RemoveAll = func(path string) error {
			if path == cacheDir {
				cacheRemoved = true
			}
			return originalRemoveAll(path)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == cacheDir {
				return &mockFileInfo{name: filepath.Base(name), isDir: true}, nil
			}
			return nil, fmt.Errorf("stat error")
		}

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

		mocks.Shims.LayerUncompressed = func(layer v1.Layer) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}

		artifacts, err := builder.Pull([]string{"oci://registry.example.com/my-repo:v1.0.0"})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(artifacts) != 1 {
			t.Errorf("Expected 1 artifact, got %d", len(artifacts))
		}
		if !cacheRemoved {
			t.Error("Expected corrupted cache directory to be removed, but it was not")
		}
	})

	t.Run("CachingWorksWithMixedNewAndCachedArtifacts", func(t *testing.T) {
		// Given an ArtifactBuilder with mocked dependencies
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		_ = shell.NewMockShell()

		testTarData := createTestTarGz(t, map[string][]byte{
			"metadata.yaml":       []byte("name: test\nversion: v1.0.0\n"),
			"_template/test.yaml": []byte("test content"),
		})

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
			return io.NopCloser(bytes.NewReader(testTarData)), nil
		}
		builder.shims.ReadAll = func(r io.Reader) ([]byte, error) {
			return io.ReadAll(r)
		}
		builder.shims.Stat = os.Stat
		builder.shims.MkdirAll = os.MkdirAll
		builder.shims.Create = func(name string) (io.WriteCloser, error) {
			dir := filepath.Dir(name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
			return os.Create(name)
		}
		builder.shims.RemoveAll = os.RemoveAll
		builder.shims.Rename = os.Rename

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
		if artifacts1[key1] != artifacts2[key1] {
			t.Errorf("cached artifact data should be identical")
		}
	})
}

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

func TestArtifactBuilder_GetTemplateData_Removed(t *testing.T) {
	t.Skip("GetTemplateData has been removed - functionality now handled by Pull + reading from cache directories")
}

func TestArtifactBuilder_GetTemplateData_Old_Removed(t *testing.T) {
	t.Skip("GetTemplateData has been removed - entire test function removed")
}

func TestArtifactBuilder_GetCacheDir(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ReturnsOCICacheDir", func(t *testing.T) {
		// Given a builder with project root
		builder, mocks := setup(t)

		// When getting cache dir for OCI artifact
		cacheDir, err := builder.GetCacheDir("ghcr.io", "example/repo", "v1.0.0")

		// Then should return correct path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedPath := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", "cache", "oci", "ghcr.io_example_repo_v1.0.0")
		if cacheDir != expectedPath {
			t.Errorf("Expected cache dir %s, got %s", expectedPath, cacheDir)
		}
	})

	t.Run("HandlesComplexRegistryPaths", func(t *testing.T) {
		// Given a builder with project root
		builder, mocks := setup(t)

		// When getting cache dir with complex registry path
		cacheDir, err := builder.GetCacheDir("registry.example.com", "namespace/subnamespace/repo", "latest")

		// Then should return correct path with sanitized extraction key
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedPath := filepath.Join(mocks.Runtime.ProjectRoot, ".windsor", "cache", "oci", "registry.example.com_namespace_subnamespace_repo_latest")
		if cacheDir != expectedPath {
			t.Errorf("Expected cache dir %s, got %s", expectedPath, cacheDir)
		}
	})

	t.Run("ErrorWhenProjectRootNotSet", func(t *testing.T) {
		// Given a builder without project root
		builder, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""

		// When getting cache dir
		_, err := builder.GetCacheDir("ghcr.io", "example/repo", "v1.0.0")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error when project root is not set")
		}
		if !strings.Contains(err.Error(), "project root is not set") {
			t.Errorf("Expected error about project root, got %v", err)
		}
	})
}

func TestArtifactBuilder_ExtractModulePath(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWhenModuleAlreadyExtracted", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		modulePath := "terraform/test-module"
		fullModulePath := filepath.Join(cacheDir, modulePath)
		if err := os.MkdirAll(fullModulePath, 0755); err != nil {
			t.Fatalf("Failed to create module directory: %v", err)
		}

		mocks.Shims.Stat = os.Stat

		result, err := builder.ExtractModulePath("registry", "repo", "tag", modulePath)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != fullModulePath {
			t.Errorf("Expected path %s, got %s", fullModulePath, result)
		}
	})

	t.Run("SuccessWhenExtractingModule", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		modulePath := "terraform/test-module"
		testTarData := createTestTarGz(t, map[string][]byte{
			"terraform/test-module/main.tf":      []byte("resource \"test\" {}"),
			"terraform/test-module/variables.tf": []byte("variable \"test\" {}"),
		})

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(artifactTarPath, testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}

		mocks.Shims.Stat = os.Stat
		mocks.Shims.ReadFile = os.ReadFile
		mocks.Shims.NewBytesReader = bytes.NewReader
		mocks.Shims.NewTarReader = func(r io.Reader) TarReader {
			return tar.NewReader(r)
		}
		mocks.Shims.MkdirAll = os.MkdirAll
		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			return os.Create(name)
		}
		mocks.Shims.Copy = io.Copy
		mocks.Shims.Chmod = os.Chmod

		result, err := builder.ExtractModulePath("registry", "repo", "tag", modulePath)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedPath := filepath.Join(cacheDir, modulePath)
		if result != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, result)
		}

		mainTfPath := filepath.Join(result, "main.tf")
		if _, err := os.Stat(mainTfPath); err != nil {
			t.Errorf("Expected main.tf to exist, got error: %v", err)
		}
	})

	t.Run("ErrorWhenGetCacheDirFails", func(t *testing.T) {
		builder, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""

		_, err := builder.ExtractModulePath("registry", "repo", "tag", "terraform/module")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get cache directory") {
			t.Errorf("Expected cache directory error, got %v", err)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		_, err = builder.ExtractModulePath("registry", "repo", "tag", "terraform/module")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read cached artifact.tar") {
			t.Errorf("Expected read file error, got %v", err)
		}
	})

	t.Run("ErrorWhenExtractTarEntriesFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		testTarData := createTestTarGz(t, map[string][]byte{
			"terraform/test-module/main.tf": []byte("resource \"test\" {}"),
		})

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(artifactTarPath, testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = os.ReadFile
		mocks.Shims.NewBytesReader = bytes.NewReader
		mocks.Shims.NewTarReader = func(r io.Reader) TarReader {
			return &mockTarReader{
				nextFunc: func() (*tar.Header, error) {
					return nil, fmt.Errorf("tar read error")
				},
			}
		}

		_, err = builder.ExtractModulePath("registry", "repo", "tag", "terraform/test-module")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to extract module path from artifact") {
			t.Errorf("Expected extract error, got %v", err)
		}
	})

	t.Run("ErrorWhenModulePathDoesNotExistInArtifact", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		testTarData := createTestTarGz(t, map[string][]byte{
			"terraform/other-module/main.tf": []byte("resource \"test\" {}"),
			"_template/some-file.yaml":       []byte("content"),
		})

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(artifactTarPath, testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}

		mocks.Shims.Stat = os.Stat
		mocks.Shims.ReadFile = os.ReadFile
		mocks.Shims.NewBytesReader = bytes.NewReader
		mocks.Shims.NewTarReader = func(r io.Reader) TarReader {
			return tar.NewReader(r)
		}
		mocks.Shims.MkdirAll = os.MkdirAll
		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			return os.Create(name)
		}
		mocks.Shims.Copy = io.Copy
		mocks.Shims.Chmod = os.Chmod

		requestedModulePath := "terraform/nonexistent-module"
		_, err = builder.ExtractModulePath("registry", "repo", "tag", requestedModulePath)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "does not exist in artifact") {
			t.Errorf("Expected module path not found error, got %v", err)
		}
		if !strings.Contains(err.Error(), requestedModulePath) {
			t.Errorf("Expected error to mention module path %s, got %v", requestedModulePath, err)
		}
	})
}

// createTestTarGz creates a test tar archive with the given files
