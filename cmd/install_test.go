package cmd

import (
	"fmt"
	"strings"
	"testing"

	bp "github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

type InstallCmdComponents struct {
	Injector         di.Injector
	Controller       *ctrl.MockController
	ConfigHandler    *config.MockConfigHandler
	SecretsProvider  *secrets.MockSecretsProvider
	BlueprintHandler *bp.MockBlueprintHandler
}

// setupMockInstallCmdComponents creates mock components for testing the install command
func setupMockInstallCmdComponents(optionalInjector ...di.Injector) InstallCmdComponents {
	var controller *ctrl.MockController
	var injector di.Injector

	// Use the provided injector if passed, otherwise create a new one
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	// Use the injector to create a mock controller
	controller = ctrl.NewMockController(injector)

	// Manually override and set up components
	controller.CreateProjectComponentsFunc = func() error {
		return nil
	}

	// Setup mock config handler
	configHandler := config.NewMockConfigHandler()
	configHandler.IsLoadedFunc = func() bool {
		return true
	}
	controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
		return configHandler
	}
	injector.Register("configHandler", configHandler)

	// Setup mock secrets provider
	secretsProvider := secrets.NewMockSecretsProvider(injector)
	controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
		return []secrets.SecretsProvider{secretsProvider}
	}
	injector.Register("secretsProvider", secretsProvider)

	// Setup mock blueprint handler
	blueprintHandler := bp.NewMockBlueprintHandler(injector)
	controller.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
		return blueprintHandler
	}
	injector.Register("blueprintHandler", blueprintHandler)

	return InstallCmdComponents{
		Injector:         injector,
		Controller:       controller,
		ConfigHandler:    configHandler,
		SecretsProvider:  secretsProvider,
		BlueprintHandler: blueprintHandler,
	}
}

func TestInstallCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()

		// Capture the output using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"install"})
			err := Execute(mocks.Controller)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Verify the output
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.CreateProjectComponentsFunc = func() error {
			return fmt.Errorf("error creating project components")
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error creating project components: error creating project components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("error creating service components")
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error creating service components: error creating service components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("error creating virtualization components")
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error creating virtualization components: error creating virtualization components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoBlueprintHandlerFound", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return nil
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "No blueprint handler found"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInstallingBlueprint", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			handler := bp.NewMockBlueprintHandler(mocks.Injector)
			handler.InstallFunc = func() error {
				return fmt.Errorf("error installing blueprint")
			}
			return handler
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error installing blueprint: error installing blueprint"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
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

	t.Run("LoadSecretsProvider", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		loadCalled := false
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			loadCalled = true
			return nil // or return an error if needed for testing
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mocks.SecretsProvider}
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then the secrets provider's LoadSecrets function should be called
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !loadCalled {
			t.Fatalf("Expected secrets provider's LoadSecrets function to be called")
		}
	})

	t.Run("ErrorLoadingSecrets", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("mock error loading secrets")
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mocks.SecretsProvider}
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error loading secrets: mock error loading secrets") {
			t.Fatalf("Expected error containing 'Error loading secrets: mock error loading secrets', got %v", err)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		defer resetRootCmd()

		// Create mock components
		mocks := setupMockInstallCmdComponents()
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		// When: the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then: it should return an error indicating the configuration is not loaded
		if err == nil {
			t.Fatalf("Expected error about configuration not loaded, got nil")
		}
		if !strings.Contains(err.Error(), "Cannot install blueprint. Please run `windsor init` to set up your project first.") {
			t.Fatalf("Expected error about configuration not loaded, got %v", err.Error())
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("error setting environment variables")
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error setting environment variables: error setting environment variables"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SuccessSettingEnvironmentVariables", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks
		mocks := setupMockInstallCmdComponents()
		setEnvVarsCalled := false
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			setEnvVarsCalled = true
			return nil
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then SetEnvironmentVariables should have been called
		if !setEnvVarsCalled {
			t.Fatal("Expected SetEnvironmentVariables to be called, but it wasn't")
		}
	})
}
