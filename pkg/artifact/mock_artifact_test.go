package artifact

import (
	"errors"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// The MockArtifact tests provide comprehensive test coverage for mock artifact functionality.
// It provides test utilities and configurations for validating mock artifact operations.
// The test suite verifies mock behavior, interface compliance, and helper methods.
// It ensures proper adherence to Windsor CLI style guidelines and testing patterns.

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewMockArtifact(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given no preconditions

		// When creating a new mock artifact
		mock := NewMockArtifact()

		// Then it should be properly initialized
		if mock == nil {
			t.Error("Expected mock to be non-nil")
		}
	})
}

// =============================================================================
// Test Initialize Method
// =============================================================================

func TestMockArtifact_Initialize(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock artifact with no custom function
		mock := NewMockArtifact()
		injector := di.NewMockInjector()

		// When calling Initialize
		err := mock.Initialize(injector)

		// Then it should succeed with default behavior
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CustomFunction", func(t *testing.T) {
		// Given a mock artifact with custom initialize function
		mock := NewMockArtifact()
		injector := di.NewMockInjector()
		called := false

		mock.InitializeFunc = func(inj di.Injector) error {
			called = true
			if inj != injector {
				return errors.New("wrong injector")
			}
			return nil
		}

		// When calling Initialize
		err := mock.Initialize(injector)

		// Then it should call the custom function
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !called {
			t.Error("Expected custom function to be called")
		}
	})

	t.Run("CustomFunctionError", func(t *testing.T) {
		// Given a mock artifact with custom function that returns error
		mock := NewMockArtifact()
		injector := di.NewMockInjector()

		mock.InitializeFunc = func(inj di.Injector) error {
			return errors.New("initialize error")
		}

		// When calling Initialize
		err := mock.Initialize(injector)

		// Then it should return the custom error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "initialize error" {
			t.Errorf("Expected 'initialize error', got %v", err)
		}
	})
}

// =============================================================================
// Test AddFile Method
// =============================================================================

func TestMockArtifact_AddFile(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock artifact with no custom function
		mock := NewMockArtifact()

		// When calling AddFile
		err := mock.AddFile("test.txt", []byte("test content"))

		// Then it should succeed with default behavior
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CustomFunction", func(t *testing.T) {
		// Given a mock artifact with AddFile function
		mock := NewMockArtifact()
		called := false

		mock.AddFileFunc = func(path string, content []byte) error {
			called = true
			if path != "test.txt" {
				return errors.New("wrong path")
			}
			if string(content) != "test content" {
				return errors.New("wrong content")
			}
			return nil
		}

		// When calling AddFile
		err := mock.AddFile("test.txt", []byte("test content"))

		// Then it should call the custom function
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !called {
			t.Error("Expected custom function to be called")
		}
	})

	t.Run("CustomFunctionError", func(t *testing.T) {
		// Given a mock artifact with custom function that returns error
		mock := NewMockArtifact()

		mock.AddFileFunc = func(path string, content []byte) error {
			return errors.New("add file error")
		}

		// When calling AddFile
		err := mock.AddFile("test.txt", []byte("test content"))

		// Then it should return the custom error
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "add file error" {
			t.Errorf("Expected 'add file error', got %v", err)
		}
	})
}

// =============================================================================
// Test Create Method
// =============================================================================

func TestMockArtifact_Create(t *testing.T) {
	t.Run("Create - DefaultBehavior", func(t *testing.T) {
		// Given a mock artifact with no custom function
		mock := NewMockArtifact()

		// When calling Create with default behavior
		outputPath, err := mock.Create("test.tar.gz", "test:1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And output path should be returned
		if outputPath != "test.tar.gz" {
			t.Errorf("Expected output path 'test.tar.gz', got %q", outputPath)
		}
	})

	t.Run("Create - Success", func(t *testing.T) {
		// Given a mock artifact with Create function
		mock := NewMockArtifact()
		mock.CreateFunc = func(outputPath string, tag string) (string, error) {
			return outputPath, nil
		}

		// When calling Create
		outputPath, err := mock.Create("test.tar.gz", "test:1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if outputPath != "test.tar.gz" {
			t.Errorf("Expected output path 'test.tar.gz', got %q", outputPath)
		}
	})

	t.Run("Create - Error", func(t *testing.T) {
		// Given a mock artifact with Create function that returns error
		mock := NewMockArtifact()
		mock.CreateFunc = func(outputPath string, tag string) (string, error) {
			return "", fmt.Errorf("create failed")
		}

		// When calling Create
		_, err := mock.Create("test.tar.gz", "test:1.0.0")

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error message should match
		expectedError := "create failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
