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
	"github.com/windsorcli/cli/internal/werror"
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

func (m *mockFileInfo) Name() string { return m.name }

func (m *mockFileInfo) Size() int64 { return 0 }

func (m *mockFileInfo) Mode() os.FileMode { return 0 }

func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }

func (m *mockFileInfo) IsDir() bool { return m.isDir }

func (m *mockFileInfo) Sys() any { return nil }

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

func (m *mockImage) Layers() ([]v1.Layer, error) { return nil, nil }

func (m *mockImage) MediaType() (types.MediaType, error) { return "", nil }

func (m *mockImage) Size() (int64, error) { return 0, nil }

func (m *mockImage) ConfigName() (v1.Hash, error) { return v1.Hash{}, nil }

func (m *mockImage) ConfigFile() (*v1.ConfigFile, error) { return nil, nil }

func (m *mockImage) RawConfigFile() ([]byte, error) { return nil, nil }

func (m *mockImage) Digest() (v1.Hash, error) { return v1.Hash{}, nil }

func (m *mockImage) Manifest() (*v1.Manifest, error) { return nil, nil }

func (m *mockImage) RawManifest() ([]byte, error) { return nil, nil }

func (m *mockImage) LayerByDigest(v1.Hash) (v1.Layer, error) { return nil, nil }

func (m *mockImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { return nil, nil }

// Enhanced mock image with configurable behavior for testing different scenarios
type mockImageWithManifest struct {
	manifestFunc      func() (*v1.Manifest, error)
	layerByDigestFunc func(v1.Hash) (v1.Layer, error)
	configNameFunc    func() (v1.Hash, error)
	rawConfigFileFunc func() ([]byte, error)
}

func (m *mockImageWithManifest) Layers() ([]v1.Layer, error) { return nil, nil }

func (m *mockImageWithManifest) MediaType() (types.MediaType, error) { return "", nil }

func (m *mockImageWithManifest) Size() (int64, error) { return 0, nil }

func (m *mockImageWithManifest) ConfigFile() (*v1.ConfigFile, error) { return nil, nil }

func (m *mockImageWithManifest) Digest() (v1.Hash, error) { return v1.Hash{}, nil }

func (m *mockImageWithManifest) RawManifest() ([]byte, error) { return nil, nil }

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

func (m *mockLayer) Digest() (v1.Hash, error) { return v1.Hash{}, nil }

func (m *mockLayer) DiffID() (v1.Hash, error) { return v1.Hash{}, nil }

func (m *mockLayer) Compressed() (io.ReadCloser, error) { return nil, nil }

func (m *mockLayer) Uncompressed() (io.ReadCloser, error) { return nil, nil }

func (m *mockLayer) Size() (int64, error) { return 0, nil }

func (m *mockLayer) MediaType() (types.MediaType, error) { return "", nil }

// mockReference provides a mock implementation of name.Reference for testing
type mockReference struct{}

func (m *mockReference) Context() name.Repository { return name.Repository{} }

func (m *mockReference) Identifier() string { return "" }

func (m *mockReference) Name() string { return "" }

func (m *mockReference) String() string { return "" }

func (m *mockReference) Scope(action string) string { return "" }

type mockTarReader struct {
	nextFunc func() (*tar.Header, error)
	readFunc func([]byte) (int, error)
}

func (m *mockTarReader) Next() (*tar.Header, error) {
	if m.nextFunc != nil {
		return m.nextFunc()
	}
	return nil, io.EOF
}

func (m *mockTarReader) Read(p []byte) (int, error) {
	if m.readFunc != nil {
		return m.readFunc(p)
	}
	return 0, io.EOF
}

type mockWriteCloser struct{}

func (m *mockWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	return nil
}

type errorWriteCloser struct{}

func (e *errorWriteCloser) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("write error")
}

func (e *errorWriteCloser) Close() error {
	return nil
}

type errorCloseWriteCloser struct{}

func (e *errorCloseWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (e *errorCloseWriteCloser) Close() error {
	return fmt.Errorf("close error")
}

// useRealYAMLShims replaces the canned YAML marshal/unmarshal mocks with the
// real implementations so tests whose assertions depend on faithful YAML
// round-tripping (such as artifact-manifest generation) can operate on actual
// serialized output.
func useRealYAMLShims(shims *Shims) {
	shims.YamlMarshal = yaml.Marshal
	shims.YamlUnmarshal = yaml.Unmarshal
}

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

func createTestTarGzForPrivate(t *testing.T, files map[string][]byte) []byte {
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

// =============================================================================
// Test Public Methods
// =============================================================================

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
		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("Expected name required error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "set name in metadata.yaml") {
			t.Errorf("Expected a remediation that names metadata.yaml, got %v", we)
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
		if !strings.Contains(err.Error(), "version is required") {
			t.Errorf("Expected version required error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "set version in metadata.yaml") {
			t.Errorf("Expected a remediation that names metadata.yaml, got %v", we)
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
			if _, ok := data.(*ArtifactManifest); ok {
				return []byte("version: v1alpha1\nartifacts: []\n"), nil
			}
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
		if err == nil || !strings.Contains(err.Error(), "failed to write content for") {
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

func TestArtifactBuilder_Bundle_EmitsArtifactManifest(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("BundleWithKustomizeAndTerraformAnnotations_WritesManifestFile", func(t *testing.T) {
		// Given a builder whose kustomize and terraform sources contain
		// Renovate annotations pinning both a helm chart and a docker image
		builder, mocks := setup(t)
		useRealYAMLShims(mocks.Shims)

		kustomizeYAML := []byte(`  chart:
    spec:
      chart: longhorn
      # renovate: datasource=helm depName=longhorn helmRepo=https://charts.longhorn.io
      version: 1.11.1
`)
		terraformTF := []byte(`variable "cilium_version" {
  # renovate: datasource=helm depName=cilium helmRepo=https://helm.cilium.io
  default = "1.16.3"
}
`)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "kustomize" || name == "terraform" {
				return &mockFileInfo{name: name, isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			switch root {
			case "kustomize":
				fn("kustomize", &mockFileInfo{name: "kustomize", isDir: true}, nil)
				fn("kustomize/helm-release.yaml", &mockFileInfo{name: "helm-release.yaml", isDir: false}, nil)
			case "terraform":
				fn("terraform", &mockFileInfo{name: "terraform", isDir: true}, nil)
				fn("terraform/variables.tf", &mockFileInfo{name: "variables.tf", isDir: false}, nil)
			}
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			switch {
			case strings.HasSuffix(name, "helm-release.yaml"):
				return kustomizeYAML, nil
			case strings.HasSuffix(name, "variables.tf"):
				return terraformTF, nil
			}
			return nil, fmt.Errorf("unexpected read: %s", name)
		}
		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return filepath.Base(targpath), nil
		}

		// When bundling
		if err := builder.Bundle(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then artifact-manifest.yaml is present in the bundle
		info, ok := builder.files[ManifestFileName]
		if !ok {
			t.Fatalf("expected %q in bundle files", ManifestFileName)
		}

		// And its content parses back to a manifest containing both annotations
		var manifest ArtifactManifest
		if err := mocks.Shims.YamlUnmarshal(info.Content, &manifest); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}
		if manifest.Version != ManifestVersion {
			t.Errorf("manifest.Version: got %q", manifest.Version)
		}
		if len(manifest.Artifacts) != 2 {
			t.Fatalf("artifacts: got %d, want 2", len(manifest.Artifacts))
		}

		// And each annotation produced the expected helm entry
		foundCilium, foundLonghorn := false, false
		for _, e := range manifest.Artifacts {
			if e.Type != ArtifactTypeHelm {
				t.Errorf("unexpected type %q for reference %q", e.Type, e.Reference)
			}
			switch e.Reference {
			case "cilium":
				foundCilium = true
				if e.Version != "1.16.3" {
					t.Errorf("cilium.Version: got %q", e.Version)
				}
			case "longhorn":
				foundLonghorn = true
				if e.Version != "1.11.1" {
					t.Errorf("longhorn.Version: got %q", e.Version)
				}
			}
		}
		if !foundCilium || !foundLonghorn {
			t.Errorf("expected both cilium and longhorn entries, got cilium=%v longhorn=%v", foundCilium, foundLonghorn)
		}
	})

	t.Run("BundleWithNoAnnotations_WritesEmptyManifest", func(t *testing.T) {
		// Given a builder whose sources contain no Renovate annotations
		builder, mocks := setup(t)
		useRealYAMLShims(mocks.Shims)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "kustomize" {
				return &mockFileInfo{name: "kustomize", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.Walk = func(root string, fn filepath.WalkFunc) error {
			if root == "kustomize" {
				fn("kustomize", &mockFileInfo{name: "kustomize", isDir: true}, nil)
				fn("kustomize/plain.yaml", &mockFileInfo{name: "plain.yaml", isDir: false}, nil)
			}
			return nil
		}
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("apiVersion: v1\nkind: ConfigMap\n"), nil
		}
		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return filepath.Base(targpath), nil
		}

		// When bundling
		if err := builder.Bundle(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the manifest is written with an empty artifacts list
		info, ok := builder.files[ManifestFileName]
		if !ok {
			t.Fatalf("expected %q in bundle files", ManifestFileName)
		}
		var manifest ArtifactManifest
		if err := mocks.Shims.YamlUnmarshal(info.Content, &manifest); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}
		if len(manifest.Artifacts) != 0 {
			t.Errorf("expected no artifacts, got %d", len(manifest.Artifacts))
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

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

func TestArtifactBuilder_generateArtifactManifest(t *testing.T) {
	setup := func(t *testing.T) *ArtifactBuilder {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		builder.shims.YamlMarshal = yaml.Marshal
		builder.shims.YamlUnmarshal = yaml.Unmarshal
		return builder
	}

	t.Run("EmptyFileSet_ProducesManifestWithNoArtifacts", func(t *testing.T) {
		// Given a builder whose bundle has accumulated no files
		builder := setup(t)

		// When the manifest is generated
		data, err := builder.generateArtifactManifest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the manifest carries the current schema version and an empty
		// artifact list rather than omitting the key
		var m ArtifactManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m.Version != ManifestVersion {
			t.Errorf("Version: got %q, want %q", m.Version, ManifestVersion)
		}
		if len(m.Artifacts) != 0 {
			t.Errorf("Artifacts: got %d, want 0", len(m.Artifacts))
		}
	})

	t.Run("MixedPaths_OnlyScansKustomizeAndTerraform", func(t *testing.T) {
		// Given a bundle containing annotated YAML under kustomize/, a
		// plain terraform file under terraform/, and a YAML sitting under
		// _template/ that also carries a Renovate annotation
		builder := setup(t)
		builder.addFile(
			"kustomize/helm-release.yaml",
			[]byte("# renovate: datasource=helm depName=cilium helmRepo=https://helm.cilium.io\nversion: 1.16.3\n"),
			0644,
		)
		builder.addFile(
			"terraform/main.tf",
			[]byte("# renovate: datasource=github-releases depName=flux package=fluxcd/flux2\ndefault = \"v2.3.0\"\n"),
			0644,
		)
		builder.addFile(
			"_template/metadata.yaml",
			[]byte("# renovate: datasource=docker depName=shouldnotappear\ntag: v1.0.0\n"),
			0644,
		)

		// When the manifest is generated
		data, err := builder.generateArtifactManifest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the _template path is excluded and only kustomize + terraform
		// entries appear in the manifest
		var m ArtifactManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(m.Artifacts) != 2 {
			t.Fatalf("Artifacts: got %d, want 2 (templates must be excluded)", len(m.Artifacts))
		}
		for _, a := range m.Artifacts {
			if strings.HasPrefix(a.Source.File, "_template/") {
				t.Errorf("unexpected _template entry in manifest: %+v", a)
			}
		}
	})

	t.Run("YamlMarshalFailure_ReturnsError", func(t *testing.T) {
		// Given a builder whose YAML marshal shim fails
		builder := setup(t)
		builder.shims.YamlMarshal = func(any) ([]byte, error) {
			return nil, errors.New("boom")
		}

		// When the manifest is generated
		_, err := builder.generateArtifactManifest()

		// Then the marshal error is propagated
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("expected marshal error, got %v", err)
		}
	})

	t.Run("MetadataProvenance_PopulatesBlueprintFields", func(t *testing.T) {
		// Given a builder whose metadata carries a blueprint name and version
		builder := setup(t)
		builder.metadata.Name = "testbp"
		builder.metadata.Version = "v1.2.3"

		// When the manifest is generated
		data, err := builder.generateArtifactManifest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the Provenance block reflects the builder's metadata
		var m ArtifactManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m.Blueprint.Name != "testbp" || m.Blueprint.Version != "v1.2.3" {
			t.Errorf("Blueprint: got %+v, want {testbp v1.2.3}", m.Blueprint)
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

func TestArtifactBuilder_generateMetadata(t *testing.T) {
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
		metadata, err := builder.generateMetadata(input, "testapp", "1.0.0")

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
		metadata, err := builder.generateMetadata(input, "testapp", "1.0.0")

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
		_, err := builder.generateMetadata(input, "testapp", "1.0.0")

		// Then should get marshal error
		if err == nil || !strings.Contains(err.Error(), "yaml marshal failed") {
			t.Errorf("Expected yaml marshal error, got: %v", err)
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
		// Given a builder with _template/metadata.yaml file
		builder, _ := setup(t)
		builder.addFile("_template/metadata.yaml", []byte("original metadata"), 0644)
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
		// Given a builder where user name is not configured in git
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

		// Then should succeed (may have user from env vars if present, or empty)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if builderInfo.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got '%s'", builderInfo.Email)
		}
		// User may be empty or from environment variables - both are valid
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
		// Given a builder where both user and email are not configured in git
		builder, mocks := setup(t)

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("git config not found")
		}

		// When getting builder info
		builderInfo, err := builder.getBuilderInfo()

		// Then should succeed (values may be empty or from environment variables)
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// User and email may be empty or populated from environment variables - both are valid
		// The function should not error regardless
		_ = builderInfo.User
		_ = builderInfo.Email
	})
}
