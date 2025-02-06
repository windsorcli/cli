package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/shell"
)

// resetRootCmd resets the root command to its initial state.
func resetRootCmd() {
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	controller = nil
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
	Controller    *ctrl.MockController
	Shell         *shell.MockShell
	EnvPrinter    *env.MockEnvPrinter
	ConfigHandler *config.MockConfigHandler
}

func setupSafeRootMocks(optionalInjector ...di.Injector) *MockObjects {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	mockController := ctrl.NewMockController(injector)
	controller = mockController

	mockShell := &shell.MockShell{}
	mockEnvPrinter := &env.MockEnvPrinter{}
	mockConfigHandler := config.NewMockConfigHandler()

	injector.Register("configHandler", mockConfigHandler)

	// No cleanup function is returned

	return &MockObjects{
		Controller:    mockController,
		Shell:         mockShell,
		EnvPrinter:    mockEnvPrinter,
		ConfigHandler: mockConfigHandler,
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
		setupSafeRootMocks()

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

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

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

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

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

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

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

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
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Mock SetVerbosity to verify it is called with the correct argument
		var verbositySet bool
		mockShell.SetVerbosityFunc = func(v bool) {
			if v {
				verbositySet = true
			}
		}

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

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		mocks := setupSafeRootMocks()

		// Mock LoadConfig to return an error
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.LoadConfigFunc = func(path string) error {
				return fmt.Errorf("mocked error loading config")
			}
			return mockConfigHandler
		}

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error loading config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorSettingDefaultConfig", func(t *testing.T) {
		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the injector
		injector := di.NewInjector()

		// Mock the controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController

		// Mock ResolveConfigHandler to return a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			if reflect.DeepEqual(cfg, config.DefaultConfig) {
				return fmt.Errorf("mocked error setting default config")
			}
			return nil
		}
		mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mockConfigHandler
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "production"
		}

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error setting default config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})
}
