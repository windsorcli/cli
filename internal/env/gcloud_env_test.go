package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type GCloudEnvMocks struct {
	Injector       di.Injector
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeGCloudEnvMocks(injector ...di.Injector) *GCloudEnvMocks {
	// Use the provided injector or create a new one if not provided
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock ConfigHandler using its constructor
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			GCloud: &config.GCloudConfig{
				ProjectID:   stringPtr("my-gcloud-project"),
				EndpointURL: stringPtr("https://gcloud.endpoint"),
			},
		}
	}
	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Register the mocks in the DI injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &GCloudEnvMocks{
		Injector:       mockInjector,
		ConfigHandler:  mockConfigHandler,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestGCloudEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()

		// Mock the stat function to simulate the existence of the GCloud config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.gcloud/service-account-key.json") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		gcloudEnvPrinter := NewGCloudEnvPrinter(mocks.Injector)
		gcloudEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := gcloudEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be set correctly
		expectedConfigFile := filepath.FromSlash("/mock/config/root/.gcloud/service-account-key.json")
		if envVars["GOOGLE_APPLICATION_CREDENTIALS"] != expectedConfigFile {
			t.Errorf("GOOGLE_APPLICATION_CREDENTIALS = %v, want %v", envVars["GOOGLE_APPLICATION_CREDENTIALS"], expectedConfigFile)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()

		// Override the GetConfigFunc to return nil for GCloud configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{GCloud: nil}
		}

		mockInjector := mocks.Injector

		gcloudEnvPrinter := NewGCloudEnvPrinter(mockInjector)
		gcloudEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := gcloudEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the missing configuration
		expectedOutput := "context configuration or GCloud configuration is missing\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NoGCloudConfigFile", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()

		// Override the GetConfigFunc to return a valid GCloud configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				GCloud: &config.GCloudConfig{
					ProjectID:   stringPtr("my-gcloud-project"),
					EndpointURL: stringPtr("https://example.com"),
				},
			}
		}

		// Override the GetConfigRootFunc to return a valid path
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/non/existent/path", nil
		}

		mockInjector := mocks.Injector

		gcloudEnvPrinter := NewGCloudEnvPrinter(mockInjector)
		gcloudEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := gcloudEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should not include GOOGLE_APPLICATION_CREDENTIALS and should not indicate an error
		if output != "" {
			t.Errorf("output = %v, want empty output", output)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()

		// Override the GetConfigRootFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		mockInjector := mocks.Injector

		gcloudEnvPrinter := NewGCloudEnvPrinter(mockInjector)
		gcloudEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := gcloudEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the error
		expectedOutput := "error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}

func TestGCloudEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()
		mockInjector := mocks.Injector
		gcloudEnvPrinter := NewGCloudEnvPrinter(mockInjector)
		gcloudEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the GCloud config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.gcloud/service-account-key.json") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := gcloudEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"GOOGLE_APPLICATION_CREDENTIALS": filepath.FromSlash("/mock/config/root/.gcloud/service-account-key.json"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Use setupSafeGCloudEnvMocks to create mocks
		mocks := setupSafeGCloudEnvMocks()

		// Set GCloud configuration to nil to simulate the error condition
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				GCloud: nil,
			}
		}

		mockInjector := mocks.Injector
		gcloudEnvPrinter := NewGCloudEnvPrinter(mockInjector)
		gcloudEnvPrinter.Initialize()

		// Call Print and expect an error
		err := gcloudEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		}

		// Verify the error message
		expectedError := "no environment variables: context configuration or GCloud configuration is missing"
		if err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err.Error(), expectedError)
		}
	})
}
