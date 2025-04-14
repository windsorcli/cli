package stack

// The WindsorStackTest provides comprehensive test coverage for the WindsorStack implementation.
// It provides validation of stack initialization, component management, and infrastructure operations,
// The WindsorStackTest ensures proper dependency injection and component lifecycle management,
// verifying error handling, mock interactions, and infrastructure state management.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindsorStack_NewWindsorStack(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// When a new WindsorStack is created
		stack := NewWindsorStack(injector)

		// Then the stack should be non-nil
		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestWindsorStack_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		mocks := setupMocks(t)

		// When a new WindsorStack is initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Initialize to return nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given safe mock components
		mocks := setupMocks(t)

		// And the shell is unregistered to simulate an error
		mocks.Injector.Register("shell", nil)

		// When a new WindsorStack is initialized
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving shell"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		// Given safe mock components
		mocks := setupMocks(t)

		// And the blueprintHandler is unregistered to simulate an error
		mocks.Injector.Register("blueprintHandler", nil)

		// When a new WindsorStack is initialized
		stack := NewWindsorStack(mocks.Injector)

		// Then an error should occur
		if err := stack.Initialize(); err == nil {
			t.Errorf("Expected Initialize to return an error")
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		// Given safe mock components with a resolve all error
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError((*env.EnvPrinter)(nil), fmt.Errorf("mock error resolving envPrinters"))
		opts := &SetupOptions{
			Injector: mockInjector,
		}
		mocks := setupMocks(t, opts)

		// When a new WindsorStack is initialized
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving envPrinters"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})
}

func TestWindsorStack_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		mocks := setupMocks(t)

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when the stack is brought up
		if err := stack.Up(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given osGetwd is mocked to return an error
		mocks := setupMocks(t)
		originalOsGetwd := osGetwd
		defer func() { osGetwd = originalOsGetwd }()
		osGetwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()

		// Then the expected error is contained in err
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingDirectoryExists", func(t *testing.T) {
		// Given osStat is mocked to return an error
		mocks := setupMocks(t)
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		if err == nil {
			t.Fatalf("Expected an error, but got nil")
		}

		// Then the expected error is contained in err
		expectedError := "directory /mock/project/root/.windsor/.tf_modules/remote/path does not exist"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorChangingDirectory", func(t *testing.T) {
		// Given osChdir is mocked to return an error
		mocks := setupMocks(t)
		originalOsChdir := osChdir
		defer func() { osChdir = originalOsChdir }()
		osChdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error changing to directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		// Given envPrinter is mocked to return an error
		mocks := setupMocks(t)
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting environment variables")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error getting environment variables"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		// Given osSetenv is mocked to return an error
		mocks := setupMocks(t)
		originalOsSetenv := osSetenv
		defer func() { osSetenv = originalOsSetenv }()
		osSetenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error setting environment variable"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningPostEnvHook", func(t *testing.T) {
		// Given envPrinter is mocked to return an error
		mocks := setupMocks(t)
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("mock error running post environment hook")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running post environment hook"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupMocks(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error initializing Terraform"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupMocks(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error planning Terraform changes"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupMocks(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "apply" {
				return "", fmt.Errorf("mock error running terraform apply")
			}
			return "", nil
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error applying Terraform changes"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRemovingBackendOverride", func(t *testing.T) {
		// Given osStat is mocked to return nil (indicating the file exists)
		mocks := setupMocks(t)

		// And osRemove is mocked to return an error
		originalOsRemove := osRemove
		defer func() { osRemove = originalOsRemove }()
		osRemove = func(_ string) error {
			return fmt.Errorf("mock error removing backend override")
		}

		// When a new WindsorStack is created and initialized
		stack := NewWindsorStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error removing backend_override.tf"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}
