package artifact

import (
	"fmt"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockArtifact_NewMockArtifact(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given no preconditions
		// When creating a new mock artifact
		mock := NewMockArtifact()

		// Then it should not be nil
		if mock == nil {
			t.Fatal("Expected non-nil mock artifact")
		}
	})
}

func TestMockArtifact_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom initialize function
		mock := NewMockArtifact()
		called := false
		mock.InitializeFunc = func(injector di.Injector) error {
			called = true
			return nil
		}

		// When calling Initialize
		err := mock.Initialize(di.NewInjector())

		// Then the mock function should be called
		if !called {
			t.Error("Expected InitializeFunc to be called")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom initialize function
		mock := NewMockArtifact()

		// When calling Initialize
		err := mock.Initialize(di.NewInjector())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestMockArtifact_AddFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom add file function
		mock := NewMockArtifact()
		called := false
		mock.AddFileFunc = func(path string, content []byte, mode os.FileMode) error {
			called = true
			return nil
		}

		// When calling AddFile
		err := mock.AddFile("test/path", []byte("content"), 0644)

		// Then the mock function should be called
		if !called {
			t.Error("Expected AddFileFunc to be called")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom add file function
		mock := NewMockArtifact()

		// When calling AddFile
		err := mock.AddFile("test/path", []byte("content"), 0644)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestMockArtifact_Create(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom create function
		mock := NewMockArtifact()
		called := false
		expectedPath := "expected/path.tar.gz"
		mock.CreateFunc = func(outputPath string, tag string) (string, error) {
			called = true
			return expectedPath, nil
		}

		// When calling Create
		actualPath, err := mock.Create("test/output", "test:v1.0.0")

		// Then the mock function should be called
		if !called {
			t.Error("Expected CreateFunc to be called")
		}
		if actualPath != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom create function
		mock := NewMockArtifact()

		// When calling Create
		actualPath, err := mock.Create("test/output", "test:v1.0.0")

		// Then empty string and no error should be returned
		if actualPath != "" {
			t.Errorf("Expected empty string, got %s", actualPath)
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestMockArtifact_Push(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom push function
		mock := NewMockArtifact()
		called := false
		var capturedRegistryBase, capturedRepoName, capturedTag string
		mock.PushFunc = func(registryBase string, repoName string, tag string) error {
			called = true
			capturedRegistryBase = registryBase
			capturedRepoName = repoName
			capturedTag = tag
			return nil
		}

		// When calling Push
		err := mock.Push("registry.example.com", "myapp", "v1.0.0")

		// Then the mock function should be called
		if !called {
			t.Error("Expected PushFunc to be called")
		}
		if capturedRegistryBase != "registry.example.com" {
			t.Errorf("Expected registryBase 'registry.example.com', got '%s'", capturedRegistryBase)
		}
		if capturedRepoName != "myapp" {
			t.Errorf("Expected repoName 'myapp', got '%s'", capturedRepoName)
		}
		if capturedTag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", capturedTag)
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom push function
		mock := NewMockArtifact()

		// When calling Push
		err := mock.Push("registry.example.com", "myapp", "v1.0.0")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestMockArtifact_Pull(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a MockArtifact with PullFunc set
		mock := NewMockArtifact()
		expectedArtifacts := map[string][]byte{
			"registry.example.com/repo:v1.0.0": []byte("test artifact data"),
		}
		mock.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return expectedArtifacts, nil
		}

		// When Pull is called
		ociRefs := []string{"oci://registry.example.com/repo:v1.0.0"}
		artifacts, err := mock.Pull(ociRefs)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the expected artifacts should be returned
		if len(artifacts) != len(expectedArtifacts) {
			t.Errorf("expected %d artifacts, got %d", len(expectedArtifacts), len(artifacts))
		}

		for key, expectedData := range expectedArtifacts {
			if actualData, exists := artifacts[key]; !exists {
				t.Errorf("expected artifact %s to exist", key)
			} else if string(actualData) != string(expectedData) {
				t.Errorf("expected artifact data %s, got %s", expectedData, actualData)
			}
		}
	})

	t.Run("ErrorFromPullFunc", func(t *testing.T) {
		// Given a MockArtifact with PullFunc that returns an error
		mock := NewMockArtifact()
		expectedError := fmt.Errorf("mock pull error")
		mock.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, expectedError
		}

		// When Pull is called
		ociRefs := []string{"oci://registry.example.com/repo:v1.0.0"}
		artifacts, err := mock.Pull(ociRefs)

		// Then the expected error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("expected error %v, got %v", expectedError, err)
		}

		// And artifacts should be nil
		if artifacts != nil {
			t.Errorf("expected nil artifacts, got %v", artifacts)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a MockArtifact with no PullFunc set
		mock := NewMockArtifact()

		// When Pull is called
		ociRefs := []string{"oci://registry.example.com/repo:v1.0.0"}
		artifacts, err := mock.Pull(ociRefs)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And an empty map should be returned
		if artifacts == nil {
			t.Errorf("expected empty map, got nil")
		}
		if len(artifacts) != 0 {
			t.Errorf("expected empty map, got %d items", len(artifacts))
		}
	})

	t.Run("VerifyPullFuncParameters", func(t *testing.T) {
		// Given a MockArtifact with PullFunc that verifies parameters
		mock := NewMockArtifact()
		var receivedOCIRefs []string
		mock.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			receivedOCIRefs = ociRefs
			return make(map[string][]byte), nil
		}

		// When Pull is called with specific parameters
		expectedOCIRefs := []string{
			"oci://registry.example.com/repo1:v1.0.0",
			"oci://registry.example.com/repo2:v2.0.0",
		}
		_, err := mock.Pull(expectedOCIRefs)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the PullFunc should receive the correct parameters
		if len(receivedOCIRefs) != len(expectedOCIRefs) {
			t.Errorf("expected %d OCI refs, got %d", len(expectedOCIRefs), len(receivedOCIRefs))
		}

		for i, expected := range expectedOCIRefs {
			if i >= len(receivedOCIRefs) || receivedOCIRefs[i] != expected {
				t.Errorf("expected OCI ref %s at index %d, got %s", expected, i, receivedOCIRefs[i])
			}
		}
	})

	t.Run("MultipleCallsWithDifferentBehavior", func(t *testing.T) {
		// Given a MockArtifact with PullFunc that changes behavior
		mock := NewMockArtifact()
		callCount := 0
		mock.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			callCount++
			if callCount == 1 {
				return map[string][]byte{
					"registry.example.com/repo:v1.0.0": []byte("first call data"),
				}, nil
			}
			return map[string][]byte{
				"registry.example.com/repo:v1.0.0": []byte("second call data"),
			}, nil
		}

		// When Pull is called multiple times
		ociRefs := []string{"oci://registry.example.com/repo:v1.0.0"}

		artifacts1, err1 := mock.Pull(ociRefs)
		if err1 != nil {
			t.Errorf("expected no error on first call, got %v", err1)
		}

		artifacts2, err2 := mock.Pull(ociRefs)
		if err2 != nil {
			t.Errorf("expected no error on second call, got %v", err2)
		}

		// Then each call should return different data
		key := "registry.example.com/repo:v1.0.0"
		if string(artifacts1[key]) != "first call data" {
			t.Errorf("expected first call data, got %s", artifacts1[key])
		}
		if string(artifacts2[key]) != "second call data" {
			t.Errorf("expected second call data, got %s", artifacts2[key])
		}

		// And both calls should have been made
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
	})
}
