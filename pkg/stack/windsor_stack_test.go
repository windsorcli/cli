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

		// Track the commands executed
		var executedCommands []string

		// Mock the ExecProgress function to capture the commands and arguments
		mocks.Shell.ExecProgressFunc = func(message, command string, args ...string) (string, int, error) {
			executedCommands = append(executedCommands, fmt.Sprintf("%s %s", command, strings.Join(args, " ")))
			return "", 0, nil
		}

		// When the stack is initialized and brought up
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}
		if err := stack.Up(); err != nil {
			t.Fatalf("Expected no error during Up, got %v", err)
		}

		// Validate that the expected commands were executed
		expectedCommands := []string{
			"terraform init -migrate-state -upgrade",
			"terraform plan",
			"terraform apply",
		}

		for _, expected := range expectedCommands {
			found := false
			for _, executed := range executedCommands {
				if executed == expected {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Expected command %v to be executed, but it was not", expected)
			}
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

	t.Run("ErrorCheckingDirectoryExists", func(t *testing.T) {
		// Given osStat is mocked to return an error
		mocks := setupSafeMocks()
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When a new WindsorStack is created and Up is called
		stack := NewWindsorStack(mocks.Injector)
		err := stack.Initialize()
		if err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		err = stack.Up()
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
		// Given shell.Exec is mocked to return an error for 'terraform init'
		mocks := setupSafeMocks()
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", 0, fmt.Errorf("mock error running terraform init")
			}
			return "", 0, nil
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
		expectedError := "error initializing Terraform in"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error for 'terraform plan'
		mocks := setupSafeMocks()
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, int, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "plan" {
				return "", 0, fmt.Errorf("mock error running terraform plan")
			}
			return "", 0, nil
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
		expectedError := "error planning Terraform changes in"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		// Given shell.Exec is mocked to return an error for 'terraform apply'
		mocks := setupSafeMocks()
		mocks.Shell.ExecProgressFunc = func(message, command string, args ...string) (string, int, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "apply" {
				return "", 0, fmt.Errorf("mock error running terraform apply")
			}
			return "", 0, nil
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
		expectedError := "error applying Terraform changes in"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})
}
