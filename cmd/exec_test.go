package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
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
	mockShell.ExecFunc = func(command string, args ...string) (string, int, error) {
		return "hello", 0, nil
	}
	mockController.ResolveShellFunc = func(name ...string) shell.Shell {
		return mockShell
	}

	mockSecretsProvider := &secrets.MockSecretsProvider{}
	mockController.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
		return []secrets.SecretsProvider{mockSecretsProvider}
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.IsLoadedFunc = func() bool {
		return true
	}
	mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
		return mockConfigHandler
	}

	osExit = func(code int) {}

	return &MockObjects{
		Controller:      mockController,
		Shell:           mockShell,
		EnvPrinter:      mockEnvPrinter,
		SecretsProvider: mockSecretsProvider,
	}
}

func TestExecCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		execCalled := false
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, int, error) {
			execCalled = true
			return "hello", 0, nil
		}

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "--", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check if Exec was called
		if !execCalled {
			t.Errorf("Expected Exec to be called, but it was not")
		}
	})

	t.Run("ContainerMode", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		execCalled := false
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, int, error) {
			execCalled = true
			return "container execution", 0, nil
		}

		// Set environment variable to simulate container mode
		os.Setenv("WINDSOR_EXEC_MODE", "container")
		defer os.Unsetenv("WINDSOR_EXEC_MODE")

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "--", "echo", "container"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check if Exec was called
		if !execCalled {
			t.Errorf("Expected Exec to be called, but it was not")
		}
	})

	t.Run("NoProjectNameSet", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		originalResolveConfigHandler := mocks.Controller.ResolveConfigHandlerFunc

		// Override config handler to return empty projectName
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			mockConfig := config.NewMockConfigHandler()
			mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
				return ""
			}
			return mockConfig
		}

		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			_ = Execute(mocks.Controller)
		})

		// Then the output should contain the new message
		expectedOutput := "Cannot execute commands. Please run `windsor init` to set up your project first."
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}

		// Restore original function if needed
		mocks.Controller.ResolveConfigHandlerFunc = originalResolveConfigHandler
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()

		// Capture stderr
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Verify output
		expectedOutput := "no command provided"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock components
		mocks := setupSafeExecCmdMocks()
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the exec command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the error should indicate the environment components creation error
		expectedError := "Error creating environment components: error creating environment components"
		if !strings.Contains(output, expectedError) {
			t.Errorf("Expected output to contain %q, got %q", expectedError, output)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.Controller.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("error creating service components")
		}

		// Set verbose flag to true
		verbose = true
		defer func() { verbose = false }() // Reset verbose flag after test

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the service components creation error
		expectedError := "Error creating service components: error creating service components"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing env printer: initialize error")
		}

		// Capture stderr
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error: Error initializing components: error initializing env printer: initialize error\n"
		if output != expectedOutput {
			t.Errorf("Expected output to be %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorLoadingSecrets", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeExecCmdMocks()
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("mock error loading secrets")
		}

		// Capture stderr
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error: Error loading secrets: mock error loading secrets\n"
		if output != expectedOutput {
			t.Errorf("Expected output to be %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorResolvingAllEnvPrinters", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller using setupSafeExecCmdMocks
		mocks := setupSafeExecCmdMocks()
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mocked error resolving env printers")
		}

		// Capture stderr
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error: Error getting environment variables: mocked error resolving env printers\n"
		if output != expectedOutput {
			t.Errorf("Expected output to be %q, got %q", expectedOutput, output)
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
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
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
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

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
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

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
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

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
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, int, error) {
			return "", 1, fmt.Errorf("command execution error")
		}

		// Capture stderr
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			_ = Execute(mocks.Controller)
		})

		// Then the output should indicate the error
		expectedOutput := "command execution error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
