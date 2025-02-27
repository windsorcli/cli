package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// resetRootCmd resets the root command to its initial state.
func resetRootCmd() {
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	verbose = false // Reset the verbose flag
}

// Helper function to capture stdout output
func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	return buf.String()
}

// Helper function to capture stderr output
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	return buf.String()
}

// Mock exit function to capture exit code
var exitCode int

func mockExit(code int) {
	exitCode = code
}

type MockObjects struct {
	Controller      *ctrl.MockController
	Shell           *shell.MockShell
	EnvPrinter      *env.MockEnvPrinter
	ConfigHandler   *config.MockConfigHandler
	SecretsProvider *secrets.MockSecretsProvider
}

func setupSafeRootMocks(optionalInjector ...di.Injector) *MockObjects {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	mockController := ctrl.NewMockController(injector)

	mockShell := &shell.MockShell{}
	mockEnvPrinter := &env.MockEnvPrinter{}
	mockConfigHandler := config.NewMockConfigHandler()
	mockSecretsProvider := &secrets.MockSecretsProvider{}

	injector.Register("configHandler", mockConfigHandler)
	injector.Register("secretsProvider", mockSecretsProvider)

	// No cleanup function is returned

	return &MockObjects{
		Controller:      mockController,
		Shell:           mockShell,
		EnvPrinter:      mockEnvPrinter,
		ConfigHandler:   mockConfigHandler,
		SecretsProvider: mockSecretsProvider,
	}
}

func TestRoot_Execute(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})
}

func TestRoot_preRunEInitializeCommonComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(cmd, nil)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock the controller to return an error on Initialize
		mocks.Controller.InitializeFunc = func() error {
			return fmt.Errorf("mocked error initializing controller")
		}

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(cmd, nil)

		// Then an error should be returned
		expectedError := "mocked error initializing controller"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorCreatingCommonComponents", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock the controller to return an error on CreateCommonComponents
		mocks.Controller.CreateCommonComponentsFunc = func() error {
			return fmt.Errorf("mocked error creating common components")
		}

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(cmd, nil)

		// Then an error should be returned
		expectedError := "mocked error creating common components"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock ResolveConfigHandler to return nil
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return nil
		}

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(cmd, nil)

		// Then an error should be returned
		expectedError := "No config handler found"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("SetVerbositySuccess", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock ResolveShell to return a mock shell
		mockShell := &shell.MockShell{}
		mocks.Controller.ResolveShellFunc = func(name ...string) shell.Shell {
			return mockShell
		}

		// Mock SetVerbosity to verify it is called with the correct argument
		var verbositySet bool
		mockShell.SetVerbosityFunc = func(v bool) {
			if v {
				verbositySet = true
			}
		}

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// Set the verbosity
		shell := mocks.Controller.ResolveShell()
		if shell != nil {
			shell.SetVerbosity(true)
		}

		// Then the verbosity should be set
		if !verbositySet {
			t.Fatalf("Expected verbosity to be set, but it was not")
		}
	})

	t.Run("ErrorCreatingSecretsProvider", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock CreateSecretsProviders to return an error
		mocks.Controller.CreateSecretsProvidersFunc = func() error {
			return fmt.Errorf("error creating secrets provider")
		}

		// Create a new command and register the controller
		cmd := &cobra.Command{}
		cmd.SetContext(context.WithValue(context.Background(), controllerKey, mocks.Controller))

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(cmd, nil)

		// Then an error should be returned
		expectedError := "Error creating secrets provider: error creating secrets provider"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %v", expectedError, err)
		}
	})

}
