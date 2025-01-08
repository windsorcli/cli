package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockConfigComponents struct {
	Injector    di.Injector
	MockShell   *shell.MockShell
	MockContext *context.MockContext
}

func setSafeConfigMocks(mockInjector ...di.Injector) *MockConfigComponents {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell()
	mockContext := context.NewMockContext()
	injector.Register("shell", mockShell)
	injector.Register("context", mockContext)

	return &MockConfigComponents{
		Injector:    injector,
		MockShell:   mockShell,
		MockContext: mockContext,
	}
}

func TestBaseConfigHandler_Initialize(t *testing.T) {

	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		injector.Register("shell", shell.NewMockShell())
		injector.Register("context", context.NewMockContext())

		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		injector := di.NewInjector()

		// Do not register the shell to simulate error in resolving shell
		injector.Register("context", context.NewMockContext())

		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err == nil {
			t.Errorf("Expected error when resolving shell, got nil")
		}
	})
}

func TestContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns valid values
		mocks := setSafeConfigMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Set the mock context to return "test-context"
		mocks.MockContext.GetContextFunc = func() string {
			return "test-context"
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetConfigRoot
		configRoot, err := configHandler.GetConfigRoot()

		// Then the config root should be returned without error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedConfigRoot := filepath.Join("/mock/project/root", "contexts", "test-context")
		if configRoot != expectedConfigRoot {
			t.Fatalf("expected config root %s, got %s", expectedConfigRoot, configRoot)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that returns an error
		mocks := setSafeConfigMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetConfigRoot
		_, err = configHandler.GetConfigRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error getting project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestContext_Clean(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context handler
		mocks := setSafeConfigMocks()

		// When calling Clean
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osStat to simulate the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osRemoveAll to simulate successful deletion
		osRemoveAll = func(path string) error {
			return nil
		}

		err = configHandler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock context handler that returns an error when getting the config root
		mocks := setSafeConfigMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling Clean
		err = configHandler.Clean()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error getting config root: error getting project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorDeletingDirectory", func(t *testing.T) {
		// Given a mock context handler
		mocks := setSafeConfigMocks()

		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osStat to simulate the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osRemoveAll to return an error
		osRemoveAll = func(path string) error {
			return fmt.Errorf("error deleting %s", path)
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling Clean
		err = configHandler.Clean()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error deleting") {
			t.Fatalf("expected error containing 'error deleting', got %s", err.Error())
		}
	})
}
