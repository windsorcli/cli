package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockComponents struct {
	Injector  di.Injector
	MockShell *shell.MockShell
}

func setSafeContextMocks(mockInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell()
	injector.Register("shell", mockShell)

	return &MockComponents{
		Injector:  injector,
		MockShell: mockShell,
	}
}

func TestContext_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe context mocks
		mocks := setSafeContextMocks()

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolvedInstanceNotShell", func(t *testing.T) {
		// Given a mock injector that resolves to an incorrect type for shell
		mocks := setSafeContextMocks()
		mocks.Injector.Register("shell", "not a shell")

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then an error should be returned
		if err == nil || err.Error() != "error resolving shell" {
			t.Fatalf("expected error for incorrect shell type, got %v", err)
		}
	})
}

func TestContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns a valid project root and context file
		mocks := setSafeContextMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osReadFile to return a specific context
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// Mock osMkdirAll to simulate successful directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetContext
		contextValue := contextHandler.GetContext()

		// Then the context should be returned without error
		if contextValue != "test-context" {
			t.Errorf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a mock shell that returns a valid project root but no context file
		mocks := setSafeContextMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osReadFile to simulate file not found
		osReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		// Create a new Context instance
		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := context.GetContext()

		// Then the context should default to "local"
		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})
}

func TestContext_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns a valid project root
		mocks := setSafeContextMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osWriteFile to simulate successful write
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") && string(data) == "new-context" {
				return nil
			}
			return fmt.Errorf("error writing file")
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = context.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a mock shell that returns a valid project root
		mocks := setSafeContextMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osMkdirAll to simulate successful directory creation
		osMkdirAll = func(path string, _ os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		// Mock osWriteFile to simulate an error
		osWriteFile = func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("error writing context to file")
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = context.SetContext("new-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error writing context to file") {
			t.Fatalf("expected error to contain 'error writing context to file', got %s", err.Error())
		}
	})
}
