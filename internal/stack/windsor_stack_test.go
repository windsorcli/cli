package stack

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestWindsorStack_NewWindsorStack(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new WindsorStack is created
		stack := NewWindsorStack(mocks.Injector)

		// Then the stack should be non-nil
		if stack == nil {
			t.Fatalf("Expected stack to be non-nil")
		}
	})
}

func TestWindsorStack_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new WindsorStack with safe mocks
		mocks := setupSafeMocks()
		stack := NewWindsorStack(mocks.Injector)

		// When the stack is initialized
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when the stack is brought up
		err = stack.Up()
		// Then no error should occur during Up
		if err != nil {
			t.Fatalf("Expected no error during Up, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given osGetwd is mocked to return an error
		mocks := setupSafeMocks()
		originalOsGetwd := osGetwd
		defer func() { osGetwd = originalOsGetwd }()
		osGetwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When a new WindsorStack is created and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Up()

		// Then the expected error is contained in err
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingIfDirectoryExists", func(t *testing.T) {
		// Given osStat is mocked to return an error
		mocks := setupSafeMocks()
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()

		// Then the expected error is contained in err
		expectedError := "directory /mock/project/root/.tf_modules/remote/path does not exist"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorChangingDirectory", func(t *testing.T) {
		// Given osChdir is mocked to return an error
		mocks := setupSafeMocks()
		originalOsChdir := osChdir
		defer func() { osChdir = originalOsChdir }()
		osChdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error changing to directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		// Given envPrinter is mocked to return an error
		mocks := setupSafeMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting environment variables")
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error getting environment variables"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		// Given osSetenv is mocked to return an error
		mocks := setupSafeMocks()
		originalOsSetenv := osSetenv
		defer func() { osSetenv = originalOsSetenv }()
		osSetenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error setting environment variable"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningPostEnvHook", func(t *testing.T) {
		// Given envPrinter is mocked to return an error
		mocks := setupSafeMocks()
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("mock error running post environment hook")
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running post environment hook"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupSafeMocks()
		mocks.Shell.ExecFunc = func(_ string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running 'terraform init'"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupSafeMocks()
		mocks.Shell.ExecFunc = func(_ string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running 'terraform plan'"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error
		mocks := setupSafeMocks()
		mocks.Shell.ExecFunc = func(_ string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "apply" {
				return "", fmt.Errorf("mock error running terraform apply")
			}
			return "", nil
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running 'terraform apply'"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRemovingBackendOverride", func(t *testing.T) {
		// Given osStat is mocked to return nil (indicating the file exists)
		mocks := setupSafeMocks()

		// And osRemove is mocked to return an error
		originalOsRemove := osRemove
		defer func() { osRemove = originalOsRemove }()
		osRemove = func(_ string) error {
			return fmt.Errorf("mock error removing backend_override.tf")
		}

		// When a new WindsorStack is created, initialized, and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		// Then no error should occur during initialization
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
		// Then the expected error is contained in err
		expectedError := "error removing backend_override.tf"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}
