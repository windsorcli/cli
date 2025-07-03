package bundler

import (
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
