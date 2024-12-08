package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	ctrl "github.com/windsorcli/cli/internal/controller"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/env"
	"github.com/windsorcli/cli/internal/shell"
)

// Helper functions to create pointers for basic types
func ptrInt(i int) *int {
	return &i
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
	Controller     *ctrl.MockController
	Shell          *shell.MockShell
	EnvPrinter     *env.MockEnvPrinter
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
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
		// Mock the injector
		injector := di.NewInjector()

		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController

		// Create mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		injector.Register("configHandler", mockConfigHandler)

		// Mock getCliConfigPath to return a consistent path format across OS
		originalGetCliConfigPath := getCliConfigPath
		getCliConfigPath = func() (string, error) {
			return filepath.ToSlash("/mock/home/.config/windsor/config.yaml"), nil
		}
		defer func() { getCliConfigPath = originalGetCliConfigPath }()

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Mock the injector
		injector := di.NewInjector()

		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the controller to return an error on Initialize
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeFunc = func() error {
			return fmt.Errorf("mocked error initializing controller")
		}
		controller = mockController

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error initializing controller"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorCreatingCommonComponents", func(t *testing.T) {
		// Mock the injector
		injector := di.NewInjector()

		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the controller to return an error on CreateCommonComponents
		mockController := ctrl.NewMockController(injector)
		mockController.CreateCommonComponentsFunc = func() error {
			return fmt.Errorf("mocked error creating common components")
		}
		controller = mockController

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error creating common components"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorGettingCliConfigPath", func(t *testing.T) {
		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the injector
		injector := di.NewInjector()

		// Mock the controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController

		// Mock getCliConfigPath to return an error
		originalGetCliConfigPath := getCliConfigPath
		getCliConfigPath = func() (string, error) {
			return "", fmt.Errorf("mocked error getting cli configuration path")
		}
		defer func() { getCliConfigPath = originalGetCliConfigPath }()

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error getting cli configuration path"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the injector
		injector := di.NewInjector()

		// Mock the controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController

		// Mock ResolveConfigHandler to return nil
		mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
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

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
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
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("mocked error loading config")
		}
		mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
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

	t.Run("ErrorSettingDefaultLocalConfig", func(t *testing.T) {
		// Mock the global controller
		originalController := controller
		defer func() { controller = originalController }()

		// Mock the injector
		injector := di.NewInjector()

		// Mock the controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController

		// Mock ResolveContextHandler to return a mock context handler
		mockContextHandler := &context.MockContext{}
		mockContextHandler.GetContextFunc = func() string {
			return "local"
		}
		mockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			return mockContextHandler
		}

		// Mock ResolveConfigHandler to return a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(cfg config.Context) error {
			if reflect.DeepEqual(cfg, config.DefaultLocalConfig) {
				return fmt.Errorf("mocked error setting default local config")
			}
			return nil
		}
		mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mockConfigHandler
		}

		// When preRunEInitializeCommonComponents is called
		err := preRunEInitializeCommonComponents(nil, nil)

		// Then an error should be returned
		expectedError := "mocked error setting default local config"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})
}

func TestRoot_getCliConfigPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Unset the WINDSORCONFIG environment variable
		os.Unsetenv("WINDSORCONFIG")

		// Mock osUserHomeDir to return a specific home directory
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "/mock/home", nil
		}

		// When getCliConfigPath is called
		cliConfigPath, err := getCliConfigPath()

		// Then the path should be as expected and no error should be returned
		expectedPath := filepath.ToSlash("/mock/home/.config/windsor/config.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if filepath.ToSlash(cliConfigPath) != expectedPath {
			t.Errorf("Expected path to be %q, got %q", expectedPath, filepath.ToSlash(cliConfigPath))
		}
	})

	t.Run("EnvVarSet", func(t *testing.T) {
		// Set the WINDSORCONFIG environment variable
		os.Setenv("WINDSORCONFIG", "/mock/env/config.yaml")

		// When getCliConfigPath is called
		cliConfigPath, err := getCliConfigPath()

		// Then the path should be as expected and no error should be returned
		expectedPath := "/mock/env/config.yaml"
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if cliConfigPath != expectedPath {
			t.Errorf("Expected path to be %q, got %q", expectedPath, cliConfigPath)
		}
	})

	t.Run("ErrorGettingHomeDir", func(t *testing.T) {
		// Unset the WINDSORCONFIG environment variable
		os.Unsetenv("WINDSORCONFIG")

		// Mock osUserHomeDir to return an error
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving home directory")
		}

		// When getCliConfigPath is called
		cliConfigPath, err := getCliConfigPath()

		// Then an error should be returned and the path should be empty
		expectedError := "mocked error retrieving home directory"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
		if cliConfigPath != "" {
			t.Errorf("Expected path to be empty, got %q", cliConfigPath)
		}
	})
}
