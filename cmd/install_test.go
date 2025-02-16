package cmd

import (
	"fmt"
	"strings"
	"testing"

	bp "github.com/windsorcli/cli/pkg/blueprint"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

func TestInstallCmd(t *testing.T) {
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

		// Mock the CreateProjectComponents method to succeed
		mockController.CreateProjectComponentsFunc = func() error {
			return nil
		}

		// Use a mock blueprint handler
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return bp.NewMockBlueprintHandler(injector)
		}

		// Capture the output using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"install"})
			err := Execute(mockController)
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

		// Given a mock controller that returns an error when creating project components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateProjectComponentsFunc = func() error {
			return fmt.Errorf("error creating project components")
		}

		// Use a mock blueprint handler
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return bp.NewMockBlueprintHandler(injector)
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller that returns an error when creating service components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("error creating service components")
		}

		// Use a mock blueprint handler
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return bp.NewMockBlueprintHandler(injector)
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller that returns an error when creating virtualization components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("error creating virtualization components")
		}

		// Use a mock blueprint handler
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return bp.NewMockBlueprintHandler(injector)
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller that returns nil for the blueprint handler
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return nil
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller that returns a valid mock blueprint handler with an error on install
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			handler := bp.NewMockBlueprintHandler(injector)
			handler.InstallFunc = func() error {
				return fmt.Errorf("error installing blueprint")
			}
			return handler
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller that returns an error when initializing components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// Use a mock blueprint handler
		mockController.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return bp.NewMockBlueprintHandler(injector)
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
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

	t.Run("LoadSecretsProvider", func(t *testing.T) {
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

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

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

		// Given a mock controller with a mock secrets provider that returns an error when loading secrets
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockSecretsProvider := secrets.NewMockSecretsProvider()
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("mock error loading secrets")
		}
		mockController.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// When the install command is executed
		rootCmd.SetArgs([]string{"install"})
		err := Execute(mockController)

		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error loading secrets: mock error loading secrets") {
			t.Fatalf("Expected error containing 'Error loading secrets: mock error loading secrets', got %v", err)
		}
	})
}
