package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestEnvCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the GetEnvPrinters method to return the mockEnv
		mockEnv := env.NewMockEnvPrinter()
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			fmt.Println("export VAR=value")
			return nil
		}
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			fmt.Println("alias test_alias='test_command'")
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
		expectedOutput := "export VAR=value\nalias test_alias='test_command'\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock shell that returns an error when checking trusted directory
		injector := di.NewInjector()
		mockShell := shell.NewMockShell(injector)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// Set the shell in the controller to the mock shell
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveShellFunc = func() shell.Shell {
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
		injector := di.NewInjector()
		mockShell := shell.NewMockShell(injector)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// Set the shell in the controller to the mock shell
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveShellFunc = func() shell.Shell {
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
			injector := di.NewInjector()
			mockController := ctrl.NewMockController(injector)
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
			injector := di.NewInjector()
			mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func(customVars ...map[string]string) error {
			return fmt.Errorf("expected error")
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
		expectedError := "Error executing Print: expected error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PrintErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func(customVars ...map[string]string) error {
			return fmt.Errorf("expected error")
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
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

	t.Run("ClearsEnvironmentWithEmptySessionToken", func(t *testing.T) {
		defer resetRootCmd()

		// Save original environment variables
		origSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		origManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origManagedAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_SESSION_TOKEN", origSessionToken)
			os.Setenv("WINDSOR_MANAGED_ENV", origManagedEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origManagedAlias)
		}()

		// Set empty session token and some managed env/alias values
		os.Setenv("WINDSOR_SESSION_TOKEN", "")
		os.Setenv("WINDSOR_MANAGED_ENV", "TEST_VAR1:TEST_VAR2")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "alias1:alias2")

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the environment printer
		mockEnv := env.NewMockEnvPrinter()
		clearCalled := false
		var clearedEnv, clearedAlias string
		mockEnv.ClearFunc = func() error {
			clearCalled = true
			// Capture the environment variables at the time Clear is called
			clearedEnv = os.Getenv("WINDSOR_MANAGED_ENV")
			clearedAlias = os.Getenv("WINDSOR_MANAGED_ALIAS")
			return nil
		}
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			return nil
		}
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Execute the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify Clear was called because session token was empty
		if !clearCalled {
			t.Errorf("Expected Clear to be called with empty session token, but it wasn't")
		}

		// Verify the managed env/alias values were preserved for clearing
		if clearedEnv != "TEST_VAR1:TEST_VAR2" {
			t.Errorf("Expected WINDSOR_MANAGED_ENV='TEST_VAR1:TEST_VAR2' during Clear, got '%s'", clearedEnv)
		}
		if clearedAlias != "alias1:alias2" {
			t.Errorf("Expected WINDSOR_MANAGED_ALIAS='alias1:alias2' during Clear, got '%s'", clearedAlias)
		}
	})

	t.Run("ClearsEnvironmentWhenSessionTokenChanges", func(t *testing.T) {
		defer resetRootCmd()

		// Save original environment variables
		origSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		origManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origManagedAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_SESSION_TOKEN", origSessionToken)
			os.Setenv("WINDSOR_MANAGED_ENV", origManagedEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origManagedAlias)
		}()

		// Set initial values
		initialToken := "initial-token"
		os.Setenv("WINDSOR_SESSION_TOKEN", initialToken)
		os.Setenv("WINDSOR_MANAGED_ENV", "INITIAL_VAR1:INITIAL_VAR2")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "initial_alias1:initial_alias2")

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the environment printer
		mockEnv := env.NewMockEnvPrinter()
		clearCalled := false
		var clearedEnv, clearedAlias string
		mockEnv.ClearFunc = func() error {
			clearCalled = true
			// Capture the environment variables at the time Clear is called
			clearedEnv = os.Getenv("WINDSOR_MANAGED_ENV")
			clearedAlias = os.Getenv("WINDSOR_MANAGED_ALIAS")
			return nil
		}
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			// Change the session token and managed env/alias when Print is called
			os.Setenv("WINDSOR_SESSION_TOKEN", "new-token")
			os.Setenv("WINDSOR_MANAGED_ENV", "NEW_VAR1:NEW_VAR2")
			os.Setenv("WINDSOR_MANAGED_ALIAS", "new_alias1:new_alias2")
			return nil
		}
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Execute the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify Clear was called because session token changed
		if !clearCalled {
			t.Errorf("Expected Clear to be called when session token changes, but it wasn't")
		}

		// Verify the original managed env/alias values were restored for clearing
		if clearedEnv != "INITIAL_VAR1:INITIAL_VAR2" {
			t.Errorf("Expected WINDSOR_MANAGED_ENV='INITIAL_VAR1:INITIAL_VAR2' during Clear, got '%s'", clearedEnv)
		}
		if clearedAlias != "initial_alias1:initial_alias2" {
			t.Errorf("Expected WINDSOR_MANAGED_ALIAS='initial_alias1:initial_alias2' during Clear, got '%s'", clearedAlias)
		}
	})

	t.Run("DoesNotClearEnvironmentWhenSessionTokenStaysTheSame", func(t *testing.T) {
		defer resetRootCmd()

		// Save original environment variables
		origSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		origManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origManagedAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_SESSION_TOKEN", origSessionToken)
			os.Setenv("WINDSOR_MANAGED_ENV", origManagedEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origManagedAlias)
		}()

		// Set a session token and other env vars
		token := "test-token"
		os.Setenv("WINDSOR_SESSION_TOKEN", token)
		os.Setenv("WINDSOR_MANAGED_ENV", "STABLE_VAR1:STABLE_VAR2")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "stable_alias1:stable_alias2")

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the environment printer
		mockEnv := env.NewMockEnvPrinter()
		clearCalled := false
		mockEnv.ClearFunc = func() error {
			clearCalled = true
			return nil
		}
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			// Session token stays the same, but we might modify other env vars
			os.Setenv("WINDSOR_MANAGED_ENV", "MODIFIED_VAR1:MODIFIED_VAR2")
			os.Setenv("WINDSOR_MANAGED_ALIAS", "modified_alias1:modified_alias2")
			return nil
		}
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Execute the command
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify Clear was not called because session token didn't change
		if clearCalled {
			t.Errorf("Expected Clear not to be called when session token doesn't change, but it was")
		}

		// Verify that the new environment values were kept (not restored to initial values)
		currentEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		currentAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		if currentEnv != "MODIFIED_VAR1:MODIFIED_VAR2" {
			t.Errorf("Expected WINDSOR_MANAGED_ENV to be 'MODIFIED_VAR1:MODIFIED_VAR2', got '%s'", currentEnv)
		}
		if currentAlias != "modified_alias1:modified_alias2" {
			t.Errorf("Expected WINDSOR_MANAGED_ALIAS to be 'modified_alias1:modified_alias2', got '%s'", currentAlias)
		}
	})

	t.Run("HandlesClearErrorsGracefully", func(t *testing.T) {
		defer resetRootCmd()

		// Save original environment variables
		origSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		origManagedEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origManagedAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_SESSION_TOKEN", origSessionToken)
			os.Setenv("WINDSOR_MANAGED_ENV", origManagedEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origManagedAlias)
		}()

		// Set empty session token to trigger clearing
		os.Setenv("WINDSOR_SESSION_TOKEN", "")
		os.Setenv("WINDSOR_MANAGED_ENV", "ERROR_VAR1:ERROR_VAR2")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "error_alias1:error_alias2")

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the environment printer
		mockEnv := env.NewMockEnvPrinter()
		mockEnv.ClearFunc = func() error {
			return fmt.Errorf("error clearing environment")
		}
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			return nil
		}
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Capture stderr to verify warning is printed
		output := captureStderr(func() {
			// Execute the command with verbose flag to see warnings
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := Execute(mockController)
			if err != nil {
				t.Fatalf("Expected no error despite Clear failing, got %v", err)
			}
		})

		// Verify warning was printed
		if !strings.Contains(output, "Warning: failed to clear previous environment variables: error clearing environment") {
			t.Errorf("Expected warning about Clear error, got: %q", output)
		}
	})
}
