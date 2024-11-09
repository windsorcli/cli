package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type WindsorEnvMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeWindsorEnvMocks(injector ...di.Injector) *WindsorEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/project/root"), nil
	}

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &WindsorEnvMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestWindsorEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		windsorEnv := NewWindsorEnv(mocks.Injector)

		envVars, err := windsorEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %v, want %v", envVars["WINDSOR_CONTEXT"], "mock-context")
		}

		expectedProjectRoot := filepath.FromSlash("/mock/project/root")
		if envVars["WINDSOR_PROJECT_ROOT"] != expectedProjectRoot {
			t.Errorf("WINDSOR_PROJECT_ROOT = %v, want %v", envVars["WINDSOR_PROJECT_ROOT"], expectedProjectRoot)
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeWindsorEnvMocks(mockInjector)
		mockInjector.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		windsorEnv := NewWindsorEnv(mockInjector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeWindsorEnvMocks(mockInjector)
		mockInjector.Register("contextHandler", "invalidType")

		windsorEnv := NewWindsorEnv(mockInjector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "failed to cast contextHandler to context.ContextInterface"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeWindsorEnvMocks(mockInjector)
		mockInjector.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		windsorEnv := NewWindsorEnv(mockInjector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error resolving shell: mock resolve error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeWindsorEnvMocks(mockInjector)
		mockInjector.Register("shell", "invalidType")

		windsorEnv := NewWindsorEnv(mockInjector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "failed to cast shell to shell.Shell"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context error")
		}

		windsorEnv := NewWindsorEnv(mocks.Injector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error retrieving current context: mock context error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock shell error")
		}

		windsorEnv := NewWindsorEnv(mocks.Injector)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error retrieving project root: mock shell error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})
}

func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("TestPostEnvHookNoError", func(t *testing.T) {
		windsorEnv := &WindsorEnvPrinter{}

		err := windsorEnv.PostEnvHook()
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

func TestWindsorEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeWindsorEnvMocks to create mocks
		mocks := setupSafeWindsorEnvMocks()
		mockInjector := mocks.Injector
		windsorEnv := NewWindsorEnv(mockInjector)

		// Mock the stat function to simulate the existence of the Windsor config file
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.FromSlash("/mock/config/root/.windsor/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := windsorEnv.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"WINDSOR_CONTEXT":      "mock-context",
			"WINDSOR_PROJECT_ROOT": filepath.FromSlash("/mock/project/root"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Use setupSafeWindsorEnvMocks to create mocks
		mocks := setupSafeWindsorEnvMocks()

		// Override the GetProjectRootFunc to simulate an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		mockInjector := mocks.Injector

		windsorEnv := NewWindsorEnv(mockInjector)

		// Call Print and check for errors
		err := windsorEnv.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock project root error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
