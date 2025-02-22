package env

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

type CustomEnvMocks struct {
	Injector        di.Injector
	ConfigHandler   *config.MockConfigHandler
	Shell           *shell.MockShell
	SecretsProvider *secrets.MockSecretsProvider
}

func setupSafeCustomEnvMocks(injector ...di.Injector) *CustomEnvMocks {
	// Use the provided injector or create a new one if not provided
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock ConfigHandler using its constructor
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
		if key == "environment" {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}
		}
		return nil
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Create a mock SecretsProvider using its constructor
	mockSecretsProvider := secrets.NewMockSecretsProvider()
	mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
		if input == "${{ secrets.mySecret }}" {
			return "resolvedSecretValue", nil
		}
		return input, nil
	}

	// Register the mocks in the DI injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)
	mockInjector.Register("secretsProvider", mockSecretsProvider)

	return &CustomEnvMocks{
		Injector:        mockInjector,
		ConfigHandler:   mockConfigHandler,
		Shell:           mockShell,
		SecretsProvider: mockSecretsProvider,
	}
}

func TestCustomEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()

		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()

		// Override the GetStringMapFunc to return nil for environment configuration
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			return nil
		}

		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})

	t.Run("WithSecretResolution", func(t *testing.T) {
		// Given a handler with a context set and a secret placeholder
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Mock the environment variables to include a secret placeholder
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1":       "value1",
					"VAR2":       "value2",
					"SECRET_KEY": "${{ secrets.mySecret }}",
				}
			}
			return nil
		}

		// Correctly set up the mock function to resolve the secret placeholder
		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.mySecret }}" {
				return "resolvedSecret", nil
			} else if input == "${{ secrets.unknownSecret }}" {
				return input, fmt.Errorf("failed to parse: secrets.unknownSecret")
			}
			return input, nil
		}

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the returned map should have the resolved secret value
		expectedEnvVars := map[string]string{"VAR1": "value1", "VAR2": "value2", "SECRET_KEY": "resolvedSecret"}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected GetEnvVars to return %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("WithSecretResolutionError", func(t *testing.T) {
		// Given a handler with a context set and a secret placeholder
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Use the mock secrets provider from setupSafeCustomEnvMocks and set the mock function
		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.unknownSecret }}" {
				return input, fmt.Errorf("failed to parse: secrets.unknownSecret")
			}
			return input, nil
		}

		// Mock the environment variables to include a secret placeholder
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1":       "value1",
					"VAR2":       "value2",
					"SECRET_KEY": "${{ secrets.unknownSecret }}",
				}
			}
			return nil
		}

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the returned map should have the unresolved secret placeholder with the correct error format
		expectedEnvVars := map[string]string{
			"VAR1":       "value1",
			"VAR2":       "value2",
			"SECRET_KEY": "<ERROR: failed to parse: secrets.unknownSecret>",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected GetEnvVars to return %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("NoCacheScenario", func(t *testing.T) {
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Set environment variables
		os.Setenv("VAR1", "cachedValue")
		os.Setenv("NO_CACHE", "true")

		// Mock the environment variables to include a secret placeholder
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1": "${{ secrets.someSecret }}",
					"VAR2": "value2",
				}
			}
			return nil
		}

		// Mock the secrets provider to resolve the secret placeholder
		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.someSecret }}" {
				return "resolvedSecretValue", nil
			}
			return input, nil
		}

		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Check if NO_CACHE is set to true, then the secret should be resolved
		expectedEnvVars := map[string]string{
			"VAR1": "resolvedSecretValue",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}

		// Unset environment variables
		os.Unsetenv("VAR1")
		os.Unsetenv("NO_CACHE")
	})

	t.Run("CachedEnvVarScenario", func(t *testing.T) {
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Set environment variables
		os.Setenv("VAR1", "cachedValue")
		os.Setenv("NO_CACHE", "false")

		// Mock the environment variables to include a secret placeholder
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1": "${{ secrets.someSecret }}",
					"VAR2": "value2",
				}
			}
			return nil
		}

		// Mock the secrets provider to resolve the secret placeholder
		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.someSecret }}" {
				return "resolvedSecretValue", nil
			}
			return input, nil
		}

		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Check if the environment variable is cached, it should not resolve the secret
		expectedEnvVars := map[string]string{
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}

		// Unset environment variables
		os.Unsetenv("VAR1")
		os.Unsetenv("NO_CACHE")
	})

	t.Run("DifferentWindsorContext", func(t *testing.T) {
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Set the WINDSOR_CONTEXT to a different value than the current context
		os.Setenv("WINDSOR_CONTEXT", "differentContext")

		// Mock the current context to be different
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "currentContext"
		}

		// Mock the environment variables
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1": "${{ secrets.someSecret }}",
					"VAR2": "value2",
				}
			}
			return nil
		}

		// Mock the secrets provider to resolve the secret placeholder
		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.someSecret }}" {
				return "resolvedSecretValue", nil
			}
			return input, nil
		}

		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify that the secret is resolved since caching should be disabled
		expectedEnvVars := map[string]string{
			"VAR1": "resolvedSecretValue",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}

		// Unset the WINDSOR_CONTEXT environment variable
		os.Unsetenv("WINDSOR_CONTEXT")
	})

	t.Run("NoSecretsProviders", func(t *testing.T) {
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Mock the environment variables
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1": "${{ secrets.someSecret }}",
					"VAR2": "value2",
				}
			}
			return nil
		}

		// Ensure no secrets providers are configured
		customEnvPrinter.secretsProviders = nil

		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Verify that the secret is not resolved and an error message is returned
		expectedEnvVars := map[string]string{
			"VAR1": "<ERROR: No secrets providers configured>",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})

}

func TestCustomEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := customEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}

func TestCustomEnv_Initialize(t *testing.T) {
	t.Run("InitializeWithSecretsProviders", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()

		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)

		// Call Initialize and check for errors
		err := customEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Initialize returned an error: %v", err)
		}

		// Verify that secretsProviders are initialized
		if len(customEnvPrinter.secretsProviders) == 0 {
			t.Errorf("Expected secretsProviders to be initialized, got %v", customEnvPrinter.secretsProviders)
		}
	})
}
