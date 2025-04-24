package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// TestMocks holds common mock objects used across tests
type TestMocks struct {
	Controller      *ctrl.MockController
	Shell           *shell.MockShell
	ConfigHandler   *config.MockConfigHandler
	EnvPrinter      *env.MockEnvPrinter
	SecretsProvider *secrets.MockSecretsProvider
	Injector        di.Injector
}

func setupSafeMocks() *TestMocks {
	injector := di.NewInjector()
	mockController := ctrl.NewMockController(injector)

	// Set up mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.CheckTrustedDirectoryFunc = func() error {
		return nil // Directory is trusted by default
	}
	mockShell.CheckResetFlagsFunc = func() (bool, error) {
		return false, nil // No reset needed by default
	}
	mockController.ResolveShellFunc = func() shell.Shell {
		return mockShell
	}

	// Set up mock configuration handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "vm.driver" {
			return "mock-driver" // Return a value to trigger virtualization
		}
		return ""
	}
	mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
		return mockConfigHandler
	}

	// Set up mock env printer
	mockEnvPrinter := env.NewMockEnvPrinter()
	mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
		return []env.EnvPrinter{mockEnvPrinter}
	}

	// Set up mock secrets provider
	mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
	mockController.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
		return []secrets.SecretsProvider{mockSecretsProvider}
	}

	// Set up successful component mocks
	mockController.CreateEnvComponentsFunc = func() error {
		return nil
	}
	mockController.InitializeComponentsFunc = func() error {
		return nil
	}
	mockController.CreateVirtualizationComponentsFunc = func() error {
		return nil
	}
	mockController.CreateServiceComponentsFunc = func() error {
		return nil
	}

	return &TestMocks{
		Controller:      mockController,
		Shell:           mockShell,
		ConfigHandler:   mockConfigHandler,
		EnvPrinter:      mockEnvPrinter,
		SecretsProvider: mockSecretsProvider,
		Injector:        injector,
	}
}

// Creates a command for direct execution with the RunE function
func createCommand(controller ctrl.Controller, setDecrypt bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("decrypt", setDecrypt, "")
	cmd.SetContext(context.WithValue(context.Background(), controllerKey, controller))
	return cmd
}

func TestEnvCmd(t *testing.T) {
	// Save original exit function
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	// Save and restore all environment variables that might affect the tests
	originalEnvVars := map[string]string{
		"WINDSOR_SESSION_TOKEN": os.Getenv("WINDSOR_SESSION_TOKEN"),
	}

	// Unset the session token to start with a clean environment
	os.Unsetenv("WINDSOR_SESSION_TOKEN")

	// Restore the environment variables after the test
	t.Cleanup(func() {
		for name, value := range originalEnvVars {
			if value != "" {
				os.Setenv(name, value)
			} else {
				os.Unsetenv(name)
			}
		}
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up specific mock for this test
		printCalled := false
		mocks.EnvPrinter.PrintFunc = func() error {
			printCalled = true
			fmt.Println("export VAR=value")
			return nil
		}

		// Force verbosity on the shell directly
		mocks.Shell.SetVerbosity(true)

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set up verbose mode for this test
		verbose = true
		defer func() { verbose = false }()

		// Capture stdout and execute command directly
		output := captureStdout(func() {
			cmd := createCommand(mocks.Controller, false)
			err := envCmd.RunE(cmd, []string{})
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Check if Print method was called
		if !printCalled {
			t.Fatalf("Print method was not called")
		}

		// Verify the output contains the expected string
		if !strings.Contains(output, "export VAR=value") {
			t.Errorf("Expected output to contain %q, but got %q", "export VAR=value", output)
		}
	})

	t.Run("ResetWhenInUntrustedDirectoryWithSession", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Mock the shell to return an untrusted directory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// Make sure CheckResetFlags behaves as expected
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			// Confirm the environment variable is set
			if os.Getenv("WINDSOR_SESSION_TOKEN") != "test-session" {
				t.Fatalf("Expected WINDSOR_SESSION_TOKEN to be 'test-session', got '%s'", os.Getenv("WINDSOR_SESSION_TOKEN"))
			}
			return false, nil
		}

		// Track calls to shell.Reset
		resetCalled := false
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
		}

		// Set WINDSOR_SESSION_TOKEN to simulate being in a Windsor session
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-session")
		// Clean up after the test
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)

		// Then no error should be returned, and Reset should NOT be called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if resetCalled {
			t.Fatalf("Expected Reset NOT to be called, but it was")
		}
	})

	t.Run("NoActionWhenInUntrustedDirectoryWithoutSession", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Mock the shell to return an untrusted directory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("error checking trusted directory")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	for _, verboseFlag := range []bool{true, false} {
		t.Run(fmt.Sprintf("ErrorCreatingVirtualizationComponentsWithVerbose=%v", verboseFlag), func(t *testing.T) {
			defer resetRootCmd()

			// Setup safe mocks
			mocks := setupSafeMocks()

			// Force verbosity on the shell directly
			mocks.Shell.SetVerbosity(verboseFlag)

			// Set up failing virtualization component
			mocks.Controller.CreateVirtualizationComponentsFunc = func() error {
				return fmt.Errorf("error creating virtualization components")
			}

			// Set a session token to avoid reset logic
			os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
			defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

			// Set verbose flag for this test
			verbose = verboseFlag
			defer func() { verbose = false }()

			// Create command and execute directly
			cmd := createCommand(mocks.Controller, false)
			err := envCmd.RunE(cmd, []string{})

			// Then check for the expected result based on verbosity
			if verboseFlag {
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

	for _, verboseFlag := range []bool{true, false} {
		t.Run(fmt.Sprintf("ErrorCreatingServiceComponentsWithVerbose=%v", verboseFlag), func(t *testing.T) {
			defer resetRootCmd()

			// Setup safe mocks
			mocks := setupSafeMocks()

			// Force verbosity on the shell directly
			mocks.Shell.SetVerbosity(verboseFlag)

			// Set up failing service component
			mocks.Controller.CreateServiceComponentsFunc = func() error {
				return fmt.Errorf("error creating service components")
			}

			// Set a session token to avoid reset logic
			os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
			defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

			// Set verbose flag for this test
			verbose = verboseFlag
			defer func() { verbose = false }()

			// Create command and execute directly
			cmd := createCommand(mocks.Controller, false)
			err := envCmd.RunE(cmd, []string{})

			// Then check for the expected result based on verbosity
			if verboseFlag {
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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set failing env components creation
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)

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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set failing env components creation
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Setup that a reset is needed
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// Set failing component initialization
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)

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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set failing component initialization
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ResolveAllEnvPrintersErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Return nil for env printers
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return nil
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("PrintError", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock environment printer that returns an error on Print
		mocks.EnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set verbose flag for this test
		verbose = true
		defer func() { verbose = false }()

		// Create command and execute directly
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock environment printer that returns an error on Print
		mocks.EnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock environment printer that returns an error on PostEnvHook
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set verbose flag for this test
		verbose = true
		defer func() { verbose = false }()

		// Create command and execute directly
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock environment printer that returns an error on PostEnvHook
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("DecryptFlag", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock secrets provider to track when it's loaded
		loadCalled := false
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			loadCalled = true
			return nil
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command with decrypt flag set to true and execute directly
		cmd := createCommand(mocks.Controller, true)
		err := envCmd.RunE(cmd, []string{})

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

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up mock secrets provider that returns an error
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("load error")
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set verbose flag for this test
		verbose = true
		defer func() { verbose = false }()

		// Create command with decrypt flag set to true and execute directly
		cmd := createCommand(mocks.Controller, true)
		err := envCmd.RunE(cmd, []string{})

		// Then the error should indicate the load error
		if err == nil || err.Error() != "Error loading secrets provider: load error" {
			t.Fatalf("Expected load error, got %v", err)
		}
	})

	t.Run("ErrorCheckingResetFlags", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Mock the shell to return an error when checking reset flags
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("error checking reset flags")
		}

		// Set a session token to avoid early exit from missing token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set verbose flag for this test
		verbose = true
		defer func() { verbose = false }()

		// When env command is executed with verbose flag
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error checking reset signal: error checking reset flags"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingResetFlagsWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Mock the shell to return an error when checking reset flags
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("error checking reset flags")
		}

		// Set a session token to avoid early exit from missing token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Ensure verbose flag is not set for this test
		verbose = false

		// When env command is executed without verbose flag
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ResetWithDecryptFlagStillLoadsSecrets", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up tracking variables to check execution order
		resetCalled := false
		loadSecretsCalled := false

		// Mock shell.CheckResetFlags to return shouldReset=true (simulating post-context change)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil // Indicate reset is needed
		}

		// Mock shell.Reset to track when it's called
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
		}

		// Mock secrets provider LoadSecrets to track when it's called
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			// Ensure Reset was called before LoadSecrets
			if !resetCalled {
				t.Fatalf("Expected Reset to be called before LoadSecrets")
			}
			loadSecretsCalled = true
			return nil
		}

		// Set a session token to avoid early exit from missing token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command with decrypt flag enabled
		cmd := createCommand(mocks.Controller, true) // Set decrypt flag to true
		err := envCmd.RunE(cmd, []string{})

		// Verify no errors occurred
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify Reset was called
		if !resetCalled {
			t.Fatalf("Expected Reset to be called")
		}

		// Verify LoadSecrets was called after Reset
		if !loadSecretsCalled {
			t.Fatalf("Expected LoadSecrets to be called after Reset")
		}
	})

	t.Run("ContextSwitchSimulation", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Track all method calls to observe execution flow
		var callOrder []string

		// Set up reset condition (simulating post-context change)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			callOrder = append(callOrder, "CheckResetFlags")
			return true, nil // Indicate reset is needed
		}

		mocks.Shell.ResetFunc = func() {
			callOrder = append(callOrder, "Reset")
		}

		// Track component initialization
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			callOrder = append(callOrder, "CreateEnvComponents")
			return nil
		}

		mocks.Controller.InitializeComponentsFunc = func() error {
			callOrder = append(callOrder, "InitializeComponents")
			return nil
		}

		// Track service and virtualization components
		mocks.Controller.CreateServiceComponentsFunc = func() error {
			callOrder = append(callOrder, "CreateServiceComponents")
			return nil
		}

		mocks.Controller.CreateVirtualizationComponentsFunc = func() error {
			callOrder = append(callOrder, "CreateVirtualizationComponents")
			return nil
		}

		// Track secrets provider loading
		secretsLoaded := false
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			callOrder = append(callOrder, "LoadSecrets")
			secretsLoaded = true
			return nil
		}

		// Track environment printer methods
		mocks.EnvPrinter.PrintFunc = func() error {
			callOrder = append(callOrder, "EnvPrinter.Print")
			return nil
		}

		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			callOrder = append(callOrder, "EnvPrinter.PostEnvHook")
			return nil
		}

		// Set session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command with decrypt flag enabled
		cmd := createCommand(mocks.Controller, true) // Set decrypt flag to true
		err := envCmd.RunE(cmd, []string{})

		// Verify no errors occurred
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check if all expected methods were called
		expectedCalls := []string{
			"CheckResetFlags",
			"Reset",
			"CreateEnvComponents",
			"InitializeComponents",
			"CreateVirtualizationComponents",
			"CreateServiceComponents",
			"LoadSecrets",
			"EnvPrinter.Print",
			"EnvPrinter.PostEnvHook",
		}

		// Verify all expected calls are in callOrder (though not necessarily in this exact order)
		for _, expectedCall := range expectedCalls {
			found := false
			for _, actualCall := range callOrder {
				if actualCall == expectedCall {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Expected method '%s' to be called, but it wasn't. Call order: %v", expectedCall, callOrder)
			}
		}

		// Verify secrets were loaded
		if !secretsLoaded {
			t.Fatalf("Expected secrets to be loaded")
		}

		// Check specific order of key operations
		resetIndex := -1
		loadSecretsIndex := -1

		for i, call := range callOrder {
			if call == "Reset" {
				resetIndex = i
			}
			if call == "LoadSecrets" {
				loadSecretsIndex = i
			}
		}

		if resetIndex >= loadSecretsIndex {
			t.Fatalf("Expected LoadSecrets (%d) to be called after Reset (%d), but it wasn't", loadSecretsIndex, resetIndex)
		}
	})

	t.Run("ResetWithNoSecretsProviders", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up reset condition (simulating post-context change)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil // Indicate reset is needed
		}

		// Track what happens during Reset
		var resolveAllSecretProvidersCalled bool
		var secretsProvidersAfterReset []secrets.SecretsProvider

		// Override the controller's ResolveAllSecretsProviders function to track when it's called
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			resolveAllSecretProvidersCalled = true

			// For testing, return empty list first time after reset, then return a provider on subsequent calls
			if secretsProvidersAfterReset == nil {
				secretsProvidersAfterReset = []secrets.SecretsProvider{}
				return secretsProvidersAfterReset
			}

			// On any subsequent calls, add a provider
			mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
			return []secrets.SecretsProvider{mockProvider}
		}

		// Set session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command with decrypt flag enabled
		cmd := createCommand(mocks.Controller, true) // Set decrypt flag to true
		err := envCmd.RunE(cmd, []string{})

		// Check for errors
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify the function was called
		if !resolveAllSecretProvidersCalled {
			t.Fatalf("Expected ResolveAllSecretsProviders to be called")
		}

		// Verify the list returned no providers after reset
		if len(secretsProvidersAfterReset) != 0 {
			t.Fatalf("Expected empty list of secrets providers after reset, but got %d providers", len(secretsProvidersAfterReset))
		}

		// Simulate a second call - the controller should now return a provider
		providers := mocks.Controller.ResolveAllSecretsProviders()
		if len(providers) == 0 {
			t.Fatalf("Expected at least one secrets provider on second call")
		}
	})

	t.Run("ContinuesThroughPrinterErrors", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up multiple printers - first one fails, second one succeeds
		failingPrinter := env.NewMockEnvPrinter()
		failingPrinter.PrintFunc = func() error {
			return fmt.Errorf("first printer error")
		}

		secondPrinterCalled := false
		successPrinter := env.NewMockEnvPrinter()
		successPrinter.PrintFunc = func() error {
			secondPrinterCalled = true
			return nil
		}

		// Override the printer list with our custom printers
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{failingPrinter, successPrinter}
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command and execute directly
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the second printer should be called
		if !secondPrinterCalled {
			t.Fatalf("Second printer was not called, command stopped at first error")
		}
	})

	t.Run("ContinuesThroughPrinterErrorsInVerboseMode", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up multiple printers - first one fails, second one succeeds
		failingPrinter := env.NewMockEnvPrinter()
		failingPrinter.PrintFunc = func() error {
			return fmt.Errorf("first printer error")
		}

		secondPrinterCalled := false
		successPrinter := env.NewMockEnvPrinter()
		successPrinter.PrintFunc = func() error {
			secondPrinterCalled = true
			return nil
		}

		// Override the printer list with our custom printers
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{failingPrinter, successPrinter}
		}

		// Set a session token to avoid reset logic
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Set verbose flag for this test
		verbose = true
		defer func() { verbose = false }()

		// Create command and execute directly
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then an error should be returned for the first printer
		if err == nil {
			t.Fatalf("Expected error from first printer, got nil")
		}
		expectedError := "Error executing Print: first printer error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}

		// With our implementation, the second printer still gets processed because
		// we collect all errors first, then return the first one if we're in verbose mode
		if !secondPrinterCalled {
			t.Fatalf("Second printer was not called, but all printers should be processed")
		}
	})

	t.Run("ResetWithPreRunECall", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up reset condition
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil // Indicate reset is needed
		}

		// Track function calls
		var callOrder []string
		resetCalled := false
		initializeControllerCalled := false
		createCommonComponentsCalled := false
		createSecretsProvidersCalled := false

		// Set up mocks to track calls
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
			callOrder = append(callOrder, "Reset")
		}

		mocks.Controller.InitializeFunc = func() error {
			initializeControllerCalled = true
			callOrder = append(callOrder, "InitializeController")
			return nil
		}

		mocks.Controller.CreateCommonComponentsFunc = func() error {
			createCommonComponentsCalled = true
			callOrder = append(callOrder, "CreateCommonComponents")
			return nil
		}

		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			createSecretsProvidersCalled = true
			callOrder = append(callOrder, "CreateSecretsProviders")
			return nil
		}

		// Set session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command and execute
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// No error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify the appropriate functions were called
		if !resetCalled {
			t.Error("Expected Reset to be called, but it wasn't")
		}
		if !initializeControllerCalled {
			t.Error("Expected InitializeController to be called during preRunE, but it wasn't")
		}
		if !createCommonComponentsCalled {
			t.Error("Expected CreateCommonComponents to be called during preRunE, but it wasn't")
		}
		if !createSecretsProvidersCalled {
			t.Error("Expected CreateSecretsProviders to be called during preRunE, but it wasn't")
		}

		// Verify order - Reset should happen before all preRunE steps
		resetIndex := -1
		for i, call := range callOrder {
			if call == "Reset" {
				resetIndex = i
				break
			}
		}

		if resetIndex == -1 {
			t.Fatal("Reset was not called at all")
		}

		// Verify all these happen after Reset
		for _, call := range []string{"InitializeController", "CreateCommonComponents", "CreateSecretsProviders"} {
			found := false
			for i, actualCall := range callOrder {
				if actualCall == call && i > resetIndex {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected %s to be called after Reset, but it wasn't in the correct order", call)
			}
		}
	})

	t.Run("ErrorCreatingSecretsAfterReset", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Set up reset condition
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil // Indicate reset is needed
		}

		// Mock CreateSecretsProviders to return an error
		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			return fmt.Errorf("error creating secrets providers")
		}

		// Set session token and verbose mode
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Create command and execute with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mocks.Controller)

		// We should get an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}

		expectedError := "Error creating secrets provider: error creating secrets providers"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ResetSetsNoCacheEnvironmentVariable", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Setup reset condition
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// Track if NO_CACHE is set during reset
		var noCacheValue string
		noCacheWasSet := false

		// Original osSetenv value
		originalOsSetenv := osSetenv
		// Mock osSetenv to track NO_CACHE setting
		osSetenv = func(key, value string) error {
			if key == "NO_CACHE" {
				noCacheWasSet = true
				noCacheValue = value
			}
			return nil
		}

		// Restore original function after test
		defer func() {
			osSetenv = originalOsSetenv
		}()

		// Mock reset function to implement setting NO_CACHE
		mocks.Shell.ResetFunc = func() {
			osSetenv("NO_CACHE", "true")
		}

		// Set session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Run the env command
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Verify NO_CACHE was set to "true"
		if !noCacheWasSet {
			t.Error("Expected NO_CACHE to be set during reset, but it wasn't")
		}

		if noCacheValue != "true" {
			t.Errorf("Expected NO_CACHE to be set to 'true', got %q", noCacheValue)
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Mock SetEnvironmentVariables to return an error
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("error setting environment variables")
		}

		// Run the env command without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mocks.Controller)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("SetNoCacheAfterReset", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Setup that a reset is needed
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			t.Log("CheckResetFlags called")
			return true, nil
		}

		// Mock the Reset function to verify it's called
		resetCalled := false
		mocks.Shell.ResetFunc = func() {
			t.Log("Reset called")
			resetCalled = true
		}

		// Track whether we're in a trusted directory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			t.Log("CheckTrustedDirectory called")
			return nil // Return nil to indicate we're in a trusted directory
		}

		// Set a session token to avoid early exit
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Check if NO_CACHE is set after reset
		noCacheSet := false
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			t.Logf("osSetenv called with key=%s, value=%s", key, value)
			if key == "NO_CACHE" && value == "true" {
				noCacheSet = true
			}
			return originalOsSetenv(key, value)
		}
		defer func() { osSetenv = originalOsSetenv }()

		// Enable preRunEInitializeCommonComponents to work
		mocks.Controller.InitializeFunc = func() error {
			t.Log("Controller.Initialize called")
			return nil
		}
		mocks.Controller.CreateCommonComponentsFunc = func() error {
			t.Log("Controller.CreateCommonComponents called")
			return nil
		}
		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			t.Log("Controller.CreateSecretsProviders called")
			return nil
		}

		// Set up verbose mode
		verbose = true
		defer func() { verbose = false }()

		// When env command is executed
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then
		t.Logf("Test result: err=%v, resetCalled=%v, noCacheSet=%v", err, resetCalled, noCacheSet)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Errorf("Expected Reset to be called, but it wasn't")
		}
		if !noCacheSet {
			t.Errorf("Expected NO_CACHE environment variable to be set, but it wasn't")
		}
	})

	t.Run("SetNoCacheErrorWithVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Setup that a reset is needed
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// Set a session token to avoid early exit
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Track whether we're in a trusted directory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil // Return nil to indicate we're in a trusted directory
		}

		// Mock osSetenv to return an error for NO_CACHE
		noCacheAttempted := false
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			if key == "NO_CACHE" && value == "true" {
				noCacheAttempted = true
				return fmt.Errorf("mock error setting NO_CACHE")
			}
			return originalOsSetenv(key, value)
		}
		defer func() { osSetenv = originalOsSetenv }()

		// Enable preRunEInitializeCommonComponents to work
		mocks.Controller.InitializeFunc = func() error {
			return nil
		}
		mocks.Controller.CreateCommonComponentsFunc = func() error {
			return nil
		}
		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			return nil
		}

		// Set up verbose mode
		verbose = true
		defer func() { verbose = false }()

		// When env command is executed
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		if !noCacheAttempted {
			t.Errorf("Expected attempt to set NO_CACHE, but no attempt was made")
		}

		expectedError := "Error setting NO_CACHE: mock error setting NO_CACHE"
		if err.Error() != expectedError {
			t.Errorf("Expected error: %q, got: %q", expectedError, err.Error())
		}
	})

	t.Run("SetNoCacheErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Setup safe mocks
		mocks := setupSafeMocks()

		// Setup that a reset is needed
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// Set a session token to avoid early exit
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// Track whether we're in a trusted directory
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil // Return nil to indicate we're in a trusted directory
		}

		// Mock osSetenv to return an error for NO_CACHE
		noCacheAttempted := false
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			if key == "NO_CACHE" && value == "true" {
				noCacheAttempted = true
				return fmt.Errorf("mock error setting NO_CACHE")
			}
			return originalOsSetenv(key, value)
		}
		defer func() { osSetenv = originalOsSetenv }()

		// Enable preRunEInitializeCommonComponents to work
		mocks.Controller.InitializeFunc = func() error {
			return nil
		}
		mocks.Controller.CreateCommonComponentsFunc = func() error {
			return nil
		}
		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			return nil
		}

		// Set verbose mode to false
		verbose = false

		// When env command is executed
		cmd := createCommand(mocks.Controller, false)
		err := envCmd.RunE(cmd, []string{})

		// Then no error should be returned when not in verbose mode
		if err != nil {
			t.Fatalf("Expected no error in non-verbose mode, got: %v", err)
		}

		if !noCacheAttempted {
			t.Errorf("Expected attempt to set NO_CACHE, but no attempt was made")
		}
	})
}
