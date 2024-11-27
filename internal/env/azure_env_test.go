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

type AzureEnvMocks struct {
	Injector       di.Injector
	ConfigHandler  *config.MockConfigHandler
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeAzureEnvMocks(injector ...di.Injector) *AzureEnvMocks {
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
			Azure: &config.AzureConfig{
				AzureProfile:        stringPtr("default"),
				AzureEndpointURL:    stringPtr("https://azure.endpoint"),
				StorageAccountName:  stringPtr("storageaccount"),
				FunctionAppEndpoint: stringPtr("https://functionapp.endpoint"),
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

	return &AzureEnvMocks{
		Injector:       mockInjector,
		ConfigHandler:  mockConfigHandler,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestAzureEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()

		// Mock the stat function to simulate the existence of the Azure config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.azure/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		azureEnvPrinter := NewAzureEnvPrinter(mocks.Injector)
		azureEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := azureEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be set correctly
		expectedConfigFile := filepath.FromSlash("/mock/config/root/.azure/config")
		if envVars["AZURE_CONFIG_FILE"] != expectedConfigFile {
			t.Errorf("AZURE_CONFIG_FILE = %v, want %v", envVars["AZURE_CONFIG_FILE"], expectedConfigFile)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()

		// Override the GetConfigFunc to return nil for Azure configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{Azure: nil}
		}

		mockInjector := mocks.Injector

		azureEnvPrinter := NewAzureEnvPrinter(mockInjector)
		azureEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := azureEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should indicate the missing configuration
		expectedOutput := "context configuration or Azure configuration is missing\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NoAzureConfigFile", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()

		// Override the GetConfigFunc to return a valid Azure configuration
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Azure: &config.AzureConfig{
					AzureProfile:        stringPtr("default"),
					AzureEndpointURL:    stringPtr("https://example.com"),
					StorageAccountName:  stringPtr("storageaccount"),
					FunctionAppEndpoint: stringPtr("https://functionapp.example.com"),
				},
			}
		}

		// Override the GetConfigRootFunc to return a valid path
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/non/existent/path", nil
		}

		mockInjector := mocks.Injector

		azureEnvPrinter := NewAzureEnvPrinter(mockInjector)
		azureEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := azureEnvPrinter.GetEnvVars()
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then the output should not include AZURE_CONFIG_FILE and should not indicate an error
		if output != "" {
			t.Errorf("output = %v, want empty output", output)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()

		// Override the GetConfigRootFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		mockInjector := mocks.Injector

		azureEnvPrinter := NewAzureEnvPrinter(mockInjector)
		azureEnvPrinter.Initialize()

		// Capture stdout
		output := captureStdout(t, func() {
			// When calling GetEnvVars
			_, err := azureEnvPrinter.GetEnvVars()
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

func TestAzureEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()
		mockInjector := mocks.Injector
		azureEnvPrinter := NewAzureEnvPrinter(mockInjector)
		azureEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the Azure config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.azure/config") {
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
		err := azureEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"AZURE_CONFIG_FILE":     filepath.FromSlash("/mock/config/root/.azure/config"),
			"AZURE_PROFILE":         "default",
			"AZURE_ENDPOINT_URL":    "https://azure.endpoint",
			"STORAGE_ACCOUNT_NAME":  "storageaccount",
			"FUNCTION_APP_ENDPOINT": "https://functionapp.endpoint",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Use setupSafeAzureEnvMocks to create mocks
		mocks := setupSafeAzureEnvMocks()

		// Set Azure configuration to nil to simulate the error condition
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Azure: nil,
			}
		}

		mockInjector := mocks.Injector
		azureEnvPrinter := NewAzureEnvPrinter(mockInjector)
		azureEnvPrinter.Initialize()

		// Call Print and expect an error
		err := azureEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		}

		// Verify the error message
		expectedError := "error getting environment variables: context configuration or Azure configuration is missing"
		if err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err.Error(), expectedError)
		}
	})
}
