package artifact

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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

		cacheDir, err := builder.GetCacheDir("registry.example.com", "my-repo", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, artifactTarFilename), testTarData, 0644); err != nil {
			t.Fatalf("Failed to write artifact.tar: %v", err)
		}
		mocks.Shims.Stat = os.Stat

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "NO_CACHE" {
				return "true", true
			}
			return "", false
		}

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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestArtifactBuilder_validateOCIDiskCache(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWhenCacheDirAndArtifactTarExist", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(artifactTarPath, []byte("test data"), 0644); err != nil {
			t.Fatalf("Failed to create artifact.tar: %v", err)
		}

		mocks.Shims.Stat = os.Stat
		mocks.Shims.ReadFile = os.ReadFile

		err := builder.validateOCIDiskCache(cacheDir)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorWhenCacheDirDoesNotExist", func(t *testing.T) {
		builder, mocks := setup(t)
		cacheDir := "/nonexistent/cache"

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		err := builder.validateOCIDiskCache(cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Expected os.ErrNotExist, got %v", err)
		}
	})

	t.Run("ErrorWhenArtifactTarDoesNotExist", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == cacheDir {
				return &mockFileInfo{name: filepath.Base(name), isDir: true}, nil
			}
			if name == artifactTarPath {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		err := builder.validateOCIDiskCache(cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Expected os.ErrNotExist, got %v", err)
		}
	})

	t.Run("ErrorWhenArtifactTarIsEmpty", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)

		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("Failed to create cache dir: %v", err)
		}
		if err := os.WriteFile(artifactTarPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create empty artifact.tar: %v", err)
		}

		mocks.Shims.Stat = os.Stat

		err := builder.validateOCIDiskCache(cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "artifact.tar is empty") {
			t.Errorf("Expected 'artifact.tar is empty' error, got %v", err)
		}
	})
}

func TestArtifactBuilder_validateAndSanitizePath(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("SuccessWithValidRelativePath", func(t *testing.T) {
		builder, _ := setup(t)

		result, err := builder.validateAndSanitizePath("terraform/module/main.tf")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expected := filepath.ToSlash("terraform/module/main.tf")
		actual := filepath.ToSlash(result)
		if actual != expected {
			t.Errorf("Expected '%s', got %s", expected, actual)
		}
	})

	t.Run("ErrorWithPathTraversal", func(t *testing.T) {
		builder, _ := setup(t)

		_, err := builder.validateAndSanitizePath("../../etc/passwd")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "directory traversal") {
			t.Errorf("Expected directory traversal error, got %v", err)
		}
	})

	t.Run("ErrorWithAbsoluteUnixPath", func(t *testing.T) {
		builder, _ := setup(t)

		_, err := builder.validateAndSanitizePath("/etc/passwd")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "absolute paths are not allowed") {
			t.Errorf("Expected absolute path error, got %v", err)
		}
	})

	t.Run("ErrorWithAbsoluteWindowsPath", func(t *testing.T) {
		builder, _ := setup(t)

		_, err := builder.validateAndSanitizePath("C:\\Windows\\System32")

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "absolute paths are not allowed") {
			t.Errorf("Expected absolute path error, got %v", err)
		}
	})

	t.Run("RejectsUnixAbsolutePathsOnAllPlatforms", func(t *testing.T) {
		builder, _ := setup(t)

		unixAbsolutePaths := []string{
			"/etc/passwd",
			"/root/.ssh/id_rsa",
			"/var/log/syslog",
			"/usr/bin/bash",
		}

		for _, path := range unixAbsolutePaths {
			_, err := builder.validateAndSanitizePath(path)
			if err == nil {
				t.Errorf("Expected error for Unix absolute path %s, got nil", path)
			}
			if !strings.Contains(err.Error(), "absolute paths are not allowed") {
				t.Errorf("Expected absolute path error for %s, got %v", path, err)
			}
		}
	})
}

func TestArtifactBuilder_extractTarEntries(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ErrorWhenTarReaderFails", func(t *testing.T) {
		builder, _ := setup(t)
		tmpDir := t.TempDir()

		mockReader := &mockTarReader{
			nextFunc: func() (*tar.Header, error) {
				return nil, fmt.Errorf("read error")
			},
		}

		err := builder.extractTarEntries(mockReader, tmpDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read tar header") {
			t.Errorf("Expected tar header error, got %v", err)
		}
	})

	t.Run("ErrorWhenPathInvalid", func(t *testing.T) {
		builder, _ := setup(t)
		tmpDir := t.TempDir()

		mockReader := &mockTarReader{
			nextFunc: func() (*tar.Header, error) {
				return &tar.Header{Name: "../../etc/passwd", Typeflag: tar.TypeReg}, nil
			},
		}

		err := builder.extractTarEntries(mockReader, tmpDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid path in tar archive") {
			t.Errorf("Expected invalid path error, got %v", err)
		}
	})

	t.Run("ExtractsKustomizeDirectory", func(t *testing.T) {
		builder, _ := setup(t)
		tmpDir := t.TempDir()
		callCount := 0
		content := []byte("resources:\n")

		mockReader := &mockTarReader{
			nextFunc: func() (*tar.Header, error) {
				callCount++
				if callCount == 1 {
					return &tar.Header{Name: "kustomize/file.yaml", Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0644}, nil
				}
				return nil, io.EOF
			},
			readFunc: func(p []byte) (int, error) {
				n := copy(p, content)
				content = content[n:]
				if len(content) == 0 {
					return n, io.EOF
				}
				return n, nil
			},
		}

		err := builder.extractTarEntries(mockReader, tmpDir)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		kustomizeFile := filepath.Join(tmpDir, "kustomize", "file.yaml")
		if _, statErr := os.Stat(kustomizeFile); statErr != nil {
			t.Errorf("Expected kustomize/file.yaml to be extracted, got stat error: %v", statErr)
		}
	})

	t.Run("RejectsPathTraversalUsingRelCheck", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		destDir := filepath.Join(tmpDir, "cache")

		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			rel, err := filepath.Rel(basepath, targpath)
			if err != nil {
				return "", err
			}
			if strings.HasPrefix(rel, "..") {
				return "", fmt.Errorf("path escapes base directory")
			}
			return rel, nil
		}

		mockReader := &mockTarReader{
			nextFunc: func() (*tar.Header, error) {
				return &tar.Header{Name: "../evil/file", Typeflag: tar.TypeReg}, nil
			},
		}

		err := builder.extractTarEntries(mockReader, destDir)

		if err == nil {
			t.Error("Expected error for path traversal, got nil")
		}
		if !strings.Contains(err.Error(), "path traversal attempt detected") && !strings.Contains(err.Error(), "invalid path") {
			t.Errorf("Expected path traversal error, got %v", err)
		}
	})

	t.Run("ReportsCopyErrorBeforeCloseError", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()

		copyError := fmt.Errorf("copy failed")

		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, copyError
		}

		mockReader := &mockTarReader{
			nextFunc: func() (*tar.Header, error) {
				return &tar.Header{Name: "terraform/test.txt", Typeflag: tar.TypeReg, Size: 10}, nil
			},
		}

		err := builder.extractTarEntries(mockReader, tmpDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write file") {
			t.Errorf("Expected write error, got %v", err)
		}
		if !strings.Contains(err.Error(), copyError.Error()) {
			t.Errorf("Expected Copy error to be reported, got %v", err)
		}
	})

}

func TestArtifactBuilder_extractArtifactToCache(t *testing.T) {
	setup := func(t *testing.T) (*ArtifactBuilder, *ArtifactMocks) {
		t.Helper()
		mocks := setupArtifactMocks(t)
		builder := NewArtifactBuilder(mocks.Runtime)
		builder.shims = mocks.Shims
		return builder, mocks
	}

	t.Run("ErrorWhenProjectRootNotSet", func(t *testing.T) {
		builder, mocks := setup(t)
		builder.runtime = nil

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		tmpDir := t.TempDir()
		testExtractionDir := filepath.Join(tmpDir, "test-extraction")
		if err := os.RemoveAll(testExtractionDir); err != nil {
			t.Fatalf("Failed to clean up test directory: %v", err)
		}

		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		err := builder.extractArtifactToCache(testTar, testExtractionDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to create temporary extraction directory") && !strings.Contains(err.Error(), "failed to create parent directory") {
			t.Errorf("Expected extraction directory error, got %v", err)
		}
	})

	t.Run("ErrorWhenExtractTarEntriesFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.Shims.NewTarReader = func(r io.Reader) TarReader {
			return &mockTarReader{
				nextFunc: func() (*tar.Header, error) {
					return nil, fmt.Errorf("tar read error")
				},
			}
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read tar header") {
			t.Errorf("Expected tar read error, got %v", err)
		}
	})

	t.Run("ErrorWhenCreateArtifactTarFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			if strings.Contains(name, artifactTarFilename) {
				return nil, fmt.Errorf("create error")
			}
			dir := filepath.Dir(name)
			os.MkdirAll(dir, 0755)
			return os.Create(name)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create artifact.tar file") {
			t.Errorf("Expected create error, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
			"metadata.yaml":            []byte("name: test\nversion: v1.0.0\n"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
		if _, err := os.Stat(artifactTarPath); err != nil {
			t.Errorf("Expected artifact.tar to exist, got error: %v", err)
		}
	})

	t.Run("ErrorWhenWriteArtifactTarFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			if strings.Contains(name, artifactTarFilename) {
				return &errorWriteCloser{}, nil
			}
			dir := filepath.Dir(name)
			os.MkdirAll(dir, 0755)
			return os.Create(name)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write artifact.tar file") {
			t.Errorf("Expected write error, got %v", err)
		}
	})

	t.Run("ErrorWhenCloseArtifactTarFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.Shims.Create = func(name string) (io.WriteCloser, error) {
			if strings.Contains(name, artifactTarFilename) {
				return &errorCloseWriteCloser{}, nil
			}
			dir := filepath.Dir(name)
			os.MkdirAll(dir, 0755)
			return os.Create(name)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to close artifact.tar file") {
			t.Errorf("Expected close error, got %v", err)
		}
	})

	t.Run("ErrorWhenCreateParentDirFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		parentDir := filepath.Dir(cacheDir)

		callCount := 0
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			callCount++
			normalizedPath := filepath.ToSlash(path)
			normalizedParentDir := filepath.ToSlash(parentDir)
			if callCount > 1 && normalizedPath == normalizedParentDir {
				return fmt.Errorf("mkdir error")
			}
			return os.MkdirAll(path, perm)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to create parent directory") {
			t.Errorf("Expected parent dir error, got %v", err)
		}
	})

	t.Run("ErrorWhenRemoveExistingExtractionDirFails", func(t *testing.T) {
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
			if name == cacheDir {
				return &mockFileInfo{name: filepath.Base(name), isDir: true}, nil
			}
			return os.Stat(name)
		}

		mocks.Shims.RemoveAll = func(path string) error {
			if path == cacheDir {
				return fmt.Errorf("remove error")
			}
			return os.RemoveAll(path)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to remove existing extraction directory") {
			t.Errorf("Expected remove error, got %v", err)
		}
	})

	t.Run("ErrorWhenRenameFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		mocks.Shims.Rename = func(oldpath, newpath string) error {
			return fmt.Errorf("rename error")
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}
		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to rename temporary extraction directory") {
			t.Errorf("Expected rename error, got %v", err)
		}
	})

	t.Run("ErrorWhenCleanupExistingTempDirFails", func(t *testing.T) {
		builder, mocks := setup(t)
		tmpDir := t.TempDir()
		mocks.Runtime.ProjectRoot = tmpDir

		cacheDir, err := builder.GetCacheDir("registry", "repo", "tag")
		if err != nil {
			t.Fatalf("Failed to get cache dir: %v", err)
		}

		tmpExtractionDir := cacheDir + ".tmp"
		if err := os.MkdirAll(tmpExtractionDir, 0755); err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == tmpExtractionDir {
				return &mockFileInfo{name: filepath.Base(name), isDir: true}, nil
			}
			return os.Stat(name)
		}

		mocks.Shims.RemoveAll = func(path string) error {
			if path == tmpExtractionDir {
				return fmt.Errorf("remove error")
			}
			return os.RemoveAll(path)
		}

		testTar := createTestTarGzForPrivate(t, map[string][]byte{
			"terraform/module/main.tf": []byte("resource \"test\" {}"),
		})

		err = builder.extractArtifactToCache(testTar, cacheDir)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to clean up existing temporary extraction directory") {
			t.Errorf("Expected cleanup error, got %v", err)
		}
	})
}
