package artifact

import (
	"errors"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
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

// TestMockArtifact_Initialize tests the Initialize method of MockArtifact
func TestMockArtifact_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)

		// When initializing
		injector := di.NewMockInjector()
		err := mockArtifact.Initialize(injector)

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInitializeFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("initialize error")
		mockArtifact.InitializeFunc = func(di.Injector) error {
			return expectedError
		}

		// When initializing
		injector := di.NewMockInjector()
		err := mockArtifact.Initialize(injector)

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
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
}

// TestMockArtifact_Pull tests the Pull method of MockArtifact
func TestMockArtifact_Pull(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedData := map[string][]byte{"test.yaml": []byte("test content")}
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
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
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, expectedError
		}

		// When pulling
		_, err := mockArtifact.Pull([]string{"registry.example.com/test-repo:v1.0.0"})

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}

// TestMockArtifact_GetTemplateData tests the GetTemplateData method of MockArtifact
func TestMockArtifact_GetTemplateData(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedData := map[string][]byte{"test.yaml": []byte("test content")}
		mockArtifact.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return expectedData, nil
		}

		// When getting template data
		actualData, err := mockArtifact.GetTemplateData("registry.example.com/test-repo:v1.0.0")

		// Then should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(actualData) != len(expectedData) {
			t.Errorf("Expected data length %d, got %d", len(expectedData), len(actualData))
		}
	})

	t.Run("ReturnsErrorWhenGetTemplateDataFuncSet", func(t *testing.T) {
		// Given
		mockArtifact := setupMockArtifactMocks(t)
		expectedError := errors.New("get template data error")
		mockArtifact.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return nil, expectedError
		}

		// When getting template data
		_, err := mockArtifact.GetTemplateData("registry.example.com/test-repo:v1.0.0")

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}
