package artifact

import (
	"errors"
	"testing"
)

// The MockArtifactTest is a test suite for the MockArtifact implementation.
// It provides comprehensive test coverage for mock artifact operations,
// ensuring reliable testing of artifact-dependent functionality.
// The MockArtifactTest validates the mock implementation's behavior.

// =============================================================================
// Test Setup
// =============================================================================

// setupMockArtifactMocks creates a new set of mocks for testing MockArtifact
func setupMockArtifactMocks(t *testing.T) *MockArtifact {
	t.Helper()

	// Create mock artifact
	mockArtifact := NewMockArtifact()

	return mockArtifact
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockArtifact_NewMockArtifact tests the constructor for MockArtifact
func TestMockArtifact_NewMockArtifact(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// Then the mock artifact should be created successfully
		if mockArtifact == nil {
			t.Errorf("Expected mockArtifact, got nil")
		}
	})
}

// TestMockArtifact_Bundle tests the Bundle method of MockArtifact
func TestMockArtifact_Bundle(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		mockArtifact.BundleFunc = func() error {
			return nil
		}

		// When bundling
		err := mockArtifact.Bundle()

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBundleFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("bundle error")
		mockArtifact.BundleFunc = func() error {
			return expectedError
		}

		// When bundling
		err := mockArtifact.Bundle()

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

// TestMockArtifact_Write tests the Write method of MockArtifact
func TestMockArtifact_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedPath := "/test/path.tar.gz"
		mockArtifact.WriteFunc = func(outputPath string, tag string) (string, error) {
			return expectedPath, nil
		}

		// When writing
		actualPath, err := mockArtifact.Write("/test", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
	})

	t.Run("ReturnsErrorWhenWriteFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("write error")
		mockArtifact.WriteFunc = func(outputPath string, tag string) (string, error) {
			return "", expectedError
		}

		// When writing
		_, err := mockArtifact.Write("/test", "v1.0.0")

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsDefaultWhenWriteFuncNotSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When writing without func set
		path, err := mockArtifact.Write("/test", "v1.0.0")

		// Then should return default values
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})
}

// TestMockArtifact_Push tests the Push method of MockArtifact
func TestMockArtifact_Push(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		mockArtifact.PushFunc = func(registryBase string, repoName string, tag string) error {
			return nil
		}

		// When pushing
		err := mockArtifact.Push("registry.example.com", "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenPushFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("push error")
		mockArtifact.PushFunc = func(registryBase string, repoName string, tag string) error {
			return expectedError
		}

		// When pushing
		err := mockArtifact.Push("registry.example.com", "test-repo", "v1.0.0")

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsErrorWhenBundleFails", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("bundle error")
		mockArtifact.BundleFunc = func() error {
			return expectedError
		}

		// When pushing
		err := mockArtifact.Push("registry.example.com", "test-repo", "v1.0.0")

		// Then should return bundle error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsDefaultWhenPushFuncNotSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When pushing without func set
		err := mockArtifact.Push("registry.example.com", "test-repo", "v1.0.0")

		// Then should return no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockArtifact_Pull tests the Pull method of MockArtifact
func TestMockArtifact_Pull(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedData := map[string]string{"test.yaml": "/test/cache/path"}
		mockArtifact.PullFunc = func(ociRefs []string) (map[string]string, error) {
			return expectedData, nil
		}

		// When pulling
		data, err := mockArtifact.Pull([]string{"registry.example.com/test-repo:v1.0.0"})

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(data) != len(expectedData) {
			t.Errorf("Expected data length %d, got %d", len(expectedData), len(data))
		}
	})

	t.Run("ReturnsErrorWhenPullFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("pull error")
		mockArtifact.PullFunc = func(ociRefs []string) (map[string]string, error) {
			return nil, expectedError
		}

		// When pulling
		_, err := mockArtifact.Pull([]string{"registry.example.com/test-repo:v1.0.0"})

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsDefaultWhenPullFuncNotSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When pulling without func set
		data, err := mockArtifact.Pull([]string{"registry.example.com/test-repo:v1.0.0"})

		// Then should return default values
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if data == nil {
			t.Error("Expected empty map, got nil")
		}
		if len(data) != 0 {
			t.Errorf("Expected empty map, got map with %d entries", len(data))
		}
	})
}

// TestMockArtifact_GetTemplateData tests the GetTemplateData method of MockArtifact
func TestMockArtifact_GetTemplateData_Removed(t *testing.T) {
	t.Skip("GetTemplateData has been removed from MockArtifact")
}

// TestMockArtifact_ParseOCIRef tests the ParseOCIRef method of MockArtifact
func TestMockArtifact_ParseOCIRef(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedRegistry := "registry.example.com"
		expectedRepository := "test-repo"
		expectedTag := "v1.0.0"
		mockArtifact.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			return expectedRegistry, expectedRepository, expectedTag, nil
		}

		// When parsing OCI reference
		registry, repository, tag, err := mockArtifact.ParseOCIRef("oci://registry.example.com/test-repo:v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if registry != expectedRegistry {
			t.Errorf("Expected registry %s, got %s", expectedRegistry, registry)
		}
		if repository != expectedRepository {
			t.Errorf("Expected repository %s, got %s", expectedRepository, repository)
		}
		if tag != expectedTag {
			t.Errorf("Expected tag %s, got %s", expectedTag, tag)
		}
	})

	t.Run("ReturnsErrorWhenParseOCIRefFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("parse error")
		mockArtifact.ParseOCIRefFunc = func(ociRef string) (registry, repository, tag string, err error) {
			return "", "", "", expectedError
		}

		// When parsing OCI reference
		_, _, _, err := mockArtifact.ParseOCIRef("oci://registry.example.com/test-repo:v1.0.0")

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsDefaultWhenParseOCIRefFuncNotSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When parsing OCI reference without func set
		registry, repository, tag, err := mockArtifact.ParseOCIRef("oci://registry.example.com/test-repo:v1.0.0")

		// Then should return default values
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if registry != "" {
			t.Errorf("Expected empty registry, got %s", registry)
		}
		if repository != "" {
			t.Errorf("Expected empty repository, got %s", repository)
		}
		if tag != "" {
			t.Errorf("Expected empty tag, got %s", tag)
		}
	})
}

// TestMockArtifact_GetCacheDir tests the GetCacheDir method of MockArtifact
func TestMockArtifact_GetCacheDir(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedCacheDir := "/test/cache/dir"
		mockArtifact.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return expectedCacheDir, nil
		}

		// When getting cache directory
		cacheDir, err := mockArtifact.GetCacheDir("registry.example.com", "test-repo", "v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if cacheDir != expectedCacheDir {
			t.Errorf("Expected cache dir %s, got %s", expectedCacheDir, cacheDir)
		}
	})

	t.Run("ReturnsErrorWhenGetCacheDirFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("cache dir error")
		mockArtifact.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return "", expectedError
		}

		// When getting cache directory
		_, err := mockArtifact.GetCacheDir("registry.example.com", "test-repo", "v1.0.0")

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ReturnsDefaultWhenGetCacheDirFuncNotSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When getting cache directory without func set
		cacheDir, err := mockArtifact.GetCacheDir("registry.example.com", "test-repo", "v1.0.0")

		// Then should return default values
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if cacheDir != "" {
			t.Errorf("Expected empty cache dir, got %s", cacheDir)
		}
	})
}
