package cmd

import (
	"fmt"
	"testing"

	blueprintpkg "github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/secrets"
)

// setupInstallMocks creates mock components specifically for testing the install command
func setupInstallMocks(t *testing.T, opts *SetupOptions) *Mocks {
	t.Helper()

	// Use the existing setupMocks function as a base
	mocks := setupMocks(t, opts)

	// Set up mock controller functions specific to install command
	mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
		return nil
	}
	mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
		return []secrets.SecretsProvider{}
	}
	mocks.Controller.SetEnvironmentVariablesFunc = func() error {
		return nil
	}
	mocks.Controller.ResolveBlueprintHandlerFunc = func() blueprintpkg.BlueprintHandler {
		return mocks.BlueprintHandler
	}

	return mocks
}

func TestInstallCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("InitializeWithRequirementsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Override controller to return error for InitializeWithRequirements
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("initialize error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain initialize error message
		expectedError := "Error initializing: initialize error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("LoadSecretsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Create a mock secrets provider that returns an error
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Controller.ResolveInjector())
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("load secrets error")
		}

		// Override controller to return the mock secrets provider
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain load secrets error message
		expectedError := "Error loading secrets: load secrets error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetEnvironmentVariablesError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Override controller to return error for SetEnvironmentVariables
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("set env vars error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain set env vars error message
		expectedError := "Error setting environment variables: set env vars error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoBlueprintHandler", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Override controller to return nil for ResolveBlueprintHandler
		mocks.Controller.ResolveBlueprintHandlerFunc = func() blueprintpkg.BlueprintHandler {
			return nil
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain no blueprint handler message
		expectedError := "No blueprint handler found"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("InstallError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInstallMocks(t, nil)

		// Override blueprint handler to return error for Install
		mocks.BlueprintHandler.InstallFunc = func() error {
			return fmt.Errorf("install error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"install"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain install error message
		expectedError := "Error installing blueprint: install error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
