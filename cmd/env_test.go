package cmd

import (
	"fmt"
	"strings"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeEnvCmdMocks(optionalInjector ...di.Injector) (*MockObjects, di.Injector) {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}
	mockController := ctrl.NewMockController(injector)

	// Set up default mock shell to allow GetProjectRoot to succeed
	defaultShell := shell.NewMockShell(injector)
	defaultShell.ResetFunc = func() error { return nil }
	defaultShell.CheckTrustedDirectoryFunc = func() error { return nil }
	defaultShell.GetProjectRootFunc = func() (string, error) { return "project-root", nil }
	mockController.ResolveShellFunc = func(name ...string) shell.Shell {
		return defaultShell
	}

	osExit = func(code int) {}

	return &MockObjects{
		Controller: mockController,
	}, injector
}

func TestEnvCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks and set the injector
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller

		// Mock the GetEnvPrinters method to return the mockEnv
		mockEnv := env.NewMockEnvPrinter()
		mockEnv.PrintFunc = func() error {
			fmt.Println("export VAR=value")
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Capture the output using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := Execute(mockController)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Verify the output
		expectedOutput := "export VAR=value\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock shell that returns an error when checking trusted directory
		mocks, injector := setupSafeEnvCmdMocks()
		mockShell := shell.NewMockShell(injector)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// Set the shell in the controller to the mock shell
		mockController := mocks.Controller
		mockController.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error checking trusted directory: error checking trusted directory"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingTrustedDirectoryWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock shell that returns an error when checking trusted directory
		mocks, injector := setupSafeEnvCmdMocks()
		mockShell := shell.NewMockShell(injector)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// Set the shell in the controller to the mock shell
		mockController := mocks.Controller
		mockController.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	for _, verbose := range []bool{true, false} {
		t.Run(fmt.Sprintf("ErrorCreatingVirtualizationComponentsWithVerbose=%v", verbose), func(t *testing.T) {
			defer resetRootCmd()

			// Given a mock controller that returns an error when creating virtualization components
			mocks, _ := setupSafeEnvCmdMocks()
			mockController := mocks.Controller
			mockController.CreateVirtualizationComponentsFunc = func() error {
				return fmt.Errorf("error creating virtualization components")
			}

			// When the env command is executed with or without verbose flag
			if verbose {
				rootCmd.SetArgs([]string{"env", "--verbose"})
			} else {
				rootCmd.SetArgs([]string{"env"})
			}
			err := Execute(mockController)

			// Then check the error contents
			if verbose {
				if err == nil {
					t.Fatalf("Expected an error, got nil")
				}
				expectedError := "Error creating virtualization components: error creating virtualization components"
				if err.Error() != expectedError {
					t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}

	for _, verbose := range []bool{true, false} {
		t.Run(fmt.Sprintf("ErrorCreatingServiceComponentsWithVerbose=%v", verbose), func(t *testing.T) {
			defer resetRootCmd()

			// Given a mock controller that returns an error when creating service components
			mocks, _ := setupSafeEnvCmdMocks()
			mockController := mocks.Controller
			mockController.CreateServiceComponentsFunc = func() error {
				return fmt.Errorf("error creating service components")
			}

			// When the env command is executed with or without verbose flag
			if verbose {
				rootCmd.SetArgs([]string{"env", "--verbose"})
			} else {
				rootCmd.SetArgs([]string{"env"})
			}
			err := Execute(mockController)

			// Then check the error contents
			if verbose {
				if err == nil {
					t.Fatalf("Expected an error, got nil")
				}
				expectedError := "Error creating service components: error creating service components"
				if err.Error() != expectedError {
					t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when creating environment components
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error creating environment components: error creating environment components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingEnvComponentsWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when creating environment components
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when initializing components
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error initializing components: error initializing components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponentsWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when initializing components
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ResolveAllEnvPrintersErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when resolving all environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return nil
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingAllEnvPrinters", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an empty list of environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{}
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error resolving environment printers: no printers returned"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("PrintError", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error executing Print: print error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PrintErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error executing PostEnvHook: post env hook error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PostEnvHookErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("DecryptFlag", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller with a mock secrets provider
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		loadCalled := false
		mockSecretsProvider.LoadSecretsFunc = func() error {
			loadCalled = true
			return nil // or return an error if needed for testing
		}
		mockController.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// When the env command is executed with the --decrypt flag
		rootCmd.SetArgs([]string{"env", "--decrypt"})
		err := Execute(mockController)

		// Then the secrets provider's LoadSecrets function should be called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !loadCalled {
			t.Fatalf("Expected secrets provider's LoadSecrets function to be called")
		}
	})

	t.Run("ErrorLoadingSecretsProvider", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller with a mock secrets provider that returns an error on load
		mocks, _ := setupSafeEnvCmdMocks()
		mockController := mocks.Controller
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("load error")
		}
		mockController.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// When the env command is executed with the --decrypt flag and verbose mode
		rootCmd.SetArgs([]string{"env", "--decrypt", "--verbose"})
		err := Execute(mockController)

		// Then the error should indicate the load error
		if err == nil || err.Error() != "Error loading secrets provider: load error" {
			t.Fatalf("Expected load error, got %v", err)
		}
	})

	t.Run("ResetErrorWithVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an error for Reset
		mockShell := shell.NewMockShell(injector)
		mockShell.ResetFunc = func() error { return fmt.Errorf("reset error") }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)
		expected := "Error executing Reset: reset error"
		if err == nil || err.Error() != expected {
			t.Fatalf("Expected error %q, got %v", expected, err)
		}
	})

	t.Run("ResetErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an error for Reset
		mockShell := shell.NewMockShell(injector)
		mockShell.ResetFunc = func() error { return fmt.Errorf("reset error") }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("GetProjectRootErrorWithVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an error for GetProjectRoot
		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) { return "", fmt.Errorf("get root error") }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)
		expected := "error retrieving project root: get root error"
		if err == nil || err.Error() != expected {
			t.Fatalf("Expected error %q, got %v", expected, err)
		}
	})

	t.Run("GetProjectRootErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an error for GetProjectRoot
		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) { return "", fmt.Errorf("get root error") }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		// Set args and explicitly force verbose false
		rootCmd.SetArgs([]string{"env", "--verbose=false"})
		rootCmd.Flags().Set("verbose", "false")
		err := Execute(mocks.Controller)
		if err == nil || !strings.Contains(err.Error(), "get root error") {
			t.Fatalf("Expected error containing 'get root error', got %v", err)
		}
	})

	t.Run("EmptyProjectRootWithVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an empty project root and no error
		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) { return "", nil }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)
		expected := "Error finding windsor.yaml in directory ancestry: %!w(<nil>)"
		if err == nil || err.Error() != expected {
			t.Fatalf("Expected error %q, got %v", expected, err)
		}
	})

	t.Run("EmptyProjectRootWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		mocks, injector := setupSafeEnvCmdMocks()
		// Create a mock shell that returns an empty project root and no error
		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) { return "", nil }
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}
		rootCmd.SetArgs([]string{"env", "--verbose=false"})
		rootCmd.Flags().Set("verbose", "false")
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Expected no error in non-verbose mode with empty project root, got %v", err)
		}
	})
}
