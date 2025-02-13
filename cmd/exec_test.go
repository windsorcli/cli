package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeExecCmdMocks() *MockObjects {
	injector := di.NewInjector()
	mockController := ctrl.NewMockController(injector)

	mockEnvPrinter := &env.MockEnvPrinter{}
	mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{"KEY": "VALUE"}, nil
	}
	mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
		return []env.EnvPrinter{mockEnvPrinter}
	}

	mockShell := shell.NewMockShell()
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "hello", nil
	}
	mockController.ResolveShellFunc = func() shell.Shell {
		return mockShell
	}

	return &MockObjects{
		Controller: mockController,
		Shell:      mockShell,
		EnvPrinter: mockEnvPrinter,
	}
}

func TestExecCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		cmd := exec.Command("windsor", "exec", "echo", "hello")
		outputBytes, err := cmd.Output()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		output := string(outputBytes)
		fmt.Println(output)

		// Setup mock controller
		// mocks := setupSafeExecCmdMocks()

		// Capture stdout using captureStdout
		// output = captureStdout(func() {
		// 	rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		// 	err := Execute(mocks.Controller)
		// 	if err != nil {
		// 		t.Fatalf("Expected no error, got %v", err)
		// 	}
		// })

		// Then the output should be as expected
		expectedOutput := "hello"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)
		rootCmd.SetArgs([]string{"exec"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Verify output
		expectedOutput := "no command provided"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the environment components creation error
		expectedError := "Error creating environment components: error creating environment components"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing env printer: initialize error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error: Error initializing components: error initializing env printer: initialize error\n"
		if output != expectedOutput {
			t.Errorf("Expected output to be %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorResolvingAllEnvPrinters", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return nil
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error resolving environment printers: no printers returned"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorPrinting", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("print error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		// expectedOutput := "Error executing Print: print error"
		expectedOutput := "print error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("get env vars error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error getting environment variables: get env vars error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorPostEnvHook", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error executing PostEnvHook: post env hook error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"KEY": "VALUE"}, nil
		}

		// Mock osSetenv to return an error
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			return fmt.Errorf("set env var error")
		}
		defer func() { osSetenv = originalOsSetenv }()

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error setting environment variable KEY: set env var error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("command execution error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "command execution error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
