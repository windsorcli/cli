package env

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

type WindsorEnvMocks struct {
	Injector        di.Injector
	ConfigHandler   *config.MockConfigHandler
	Shell           *shell.MockShell
	SecretsProvider *secrets.MockSecretsProvider
}

// setupSafeWindsorEnvMocks creates a new WindsorEnvMocks instance with mock dependencies.
func setupSafeWindsorEnvMocks(injector ...di.Injector) *WindsorEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/project/root"), nil
	}

	mockSecretsProvider := secrets.NewMockSecretsProvider()
	mockSecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
		return strings.ReplaceAll(input, "${{ SECRET_NAME }}", "mock-secret-value"), nil
	}

	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)
	mockInjector.Register("secretsProvider", mockSecretsProvider)

	return &WindsorEnvMocks{
		Injector:        mockInjector,
		ConfigHandler:   mockConfigHandler,
		Shell:           mockShell,
		SecretsProvider: mockSecretsProvider,
	}
}

func TestWindsorEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		if len(windsorEnvPrinter.secretsProviders) == 0 {
			t.Error("expected secretsProviders to be initialized, but got none")
		}
	})
}

func TestWindsorEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Explicitly set printedEnvVars to ensure a clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedEnvVars := map[string]string{
			"WINDSOR_CONTEXT":      "mock-context",
			"WINDSOR_PROJECT_ROOT": filepath.FromSlash("/mock/project/root"),
			"WINDSOR_EXEC_MODE":    "container",
			"WINDSOR_MANAGED_ENV":  "WINDSOR_CONTEXT,WINDSOR_PROJECT_ROOT,WINDSOR_EXEC_MODE",
		}

		for key, expectedValue := range expectedEnvVars {
			if envVars[key] != expectedValue {
				t.Errorf("%s = %v, want %v", key, envVars[key], expectedValue)
			}
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock shell error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Explicitly set printedEnvVars to ensure a clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		_, err := windsorEnvPrinter.GetEnvVars()
		expectedErrorMessage := "error retrieving project root: mock shell error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("NoEnvironmentVariables", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return nil
			}
			return make(map[string]string)
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		windsorEnvPrinter.Initialize()

		// Explicitly set printedEnvVars to ensure a clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedEnvVars := map[string]string{
			"WINDSOR_CONTEXT":      "mock-context",
			"WINDSOR_PROJECT_ROOT": filepath.FromSlash("/mock/project/root"),
			"WINDSOR_EXEC_MODE":    "container",
			"WINDSOR_MANAGED_ENV":  "WINDSOR_CONTEXT,WINDSOR_PROJECT_ROOT,WINDSOR_EXEC_MODE",
		}

		for key, expectedValue := range expectedEnvVars {
			if envVars[key] != expectedValue {
				t.Errorf("%s = %v, want %v", key, envVars[key], expectedValue)
			}
		}
	})

	t.Run("WithSecretResolution", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

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

		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.mySecret }}" {
				return "resolvedSecret", nil
			}
			return input, nil
		}

		windsorEnvPrinter.Initialize()
		// Clear previously printed environment variables
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["VAR1"] != "value1" || envVars["VAR2"] != "value2" || envVars["SECRET_KEY"] != "resolvedSecret" {
			t.Errorf("Expected secret resolution to succeed, got VAR1=%q, VAR2=%q, SECRET_KEY=%q", envVars["VAR1"], envVars["VAR2"], envVars["SECRET_KEY"])
		}
	})

	t.Run("WithSecretResolutionError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

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

		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.unknownSecret }}" {
				return input, fmt.Errorf("failed to parse: secrets.unknownSecret")
			}
			return input, nil
		}

		windsorEnvPrinter.Initialize()
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedError := "<ERROR: failed to parse: secrets.unknownSecret>"
		if envVars["SECRET_KEY"] != expectedError {
			t.Errorf("Expected SECRET_KEY to be %q, got %q", expectedError, envVars["SECRET_KEY"])
		}
	})

	t.Run("NoCacheScenario", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

		t.Setenv("VAR1", "cachedValue")
		t.Setenv("NO_CACHE", "true")

		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1": "${{ secrets.someSecret }}",
					"VAR2": "value2",
				}
			}
			return nil
		}

		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			if input == "${{ secrets.someSecret }}" {
				return "resolvedSecretValue", nil
			}
			return input, nil
		}

		windsorEnvPrinter.Initialize()
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["VAR1"] != "resolvedSecretValue" {
			t.Errorf("Expected VAR1 to be resolved to 'resolvedSecretValue', got %q", envVars["VAR1"])
		}
		if envVars["VAR2"] != "value2" {
			t.Errorf("Expected VAR2 to be 'value2', got %q", envVars["VAR2"])
		}
	})

	t.Run("CacheHitScenario", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"VAR1":       "value1",
					"SECRET_KEY": "${{ secrets.mySecret }}",
				}
			}
			return nil
		}

		// Set a cached value for SECRET_KEY in the environment
		t.Setenv("SECRET_KEY", "cachedSecret")
		// Ensure caching is enabled by not setting NO_CACHE to "true"
		t.Setenv("NO_CACHE", "false")

		mocks.SecretsProvider.ParseSecretsFunc = func(input string) (string, error) {
			// This should not be used if a valid cached value exists
			return "resolvedSecret", nil
		}

		windsorEnvPrinter.Initialize()
		// Clear previously printed environment variables
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["SECRET_KEY"] != "cachedSecret" {
			t.Errorf("Expected SECRET_KEY to be from cache 'cachedSecret', got %q", envVars["SECRET_KEY"])
		}
	})

	t.Run("PrintedEnvVarsIncluded", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

		// Ensure that no additional environment variables override this; return nil
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			return nil
		}

		windsorEnvPrinter.Initialize()
		// Set printedEnvVars with an extra key
		mu.Lock()
		printedEnvVars = map[string]string{"EXTRA_VAR": "extraValue"}
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		managedEnv := envVars["WINDSOR_MANAGED_ENV"]
		expectedDefaults := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "WINDSOR_EXEC_MODE"}
		for _, key := range expectedDefaults {
			if !strings.Contains(managedEnv, key) {
				t.Errorf("Expected managed env to contain %q, got %q", key, managedEnv)
			}
		}
		if !strings.Contains(managedEnv, "EXTRA_VAR") {
			t.Errorf("Expected managed env to include printed key 'EXTRA_VAR', got %q", managedEnv)
		}
	})

	t.Run("NoSecretsProvider", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)

		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			if key == "environment" {
				return map[string]string{
					"SECRET_KEY": "${{ secrets.something }}",
				}
			}
			return nil
		}

		windsorEnvPrinter.Initialize()
		// Override secretsProviders to simulate no secrets providers configured
		windsorEnvPrinter.secretsProviders = []secrets.SecretsProvider{}

		// Clear printedEnvVars to ensure clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		envVars, err := windsorEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expected := "<ERROR: No secrets providers configured>"
		if envVars["SECRET_KEY"] != expected {
			t.Errorf("Expected SECRET_KEY to be %q, got %q", expected, envVars["SECRET_KEY"])
		}
	})
}

func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("TestPostEnvHookNoError", func(t *testing.T) {
		windsorEnvPrinter := &WindsorEnvPrinter{}

		err := windsorEnvPrinter.PostEnvHook()
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}

func TestWindsorEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string {
			return "darwin"
		}

		// Explicitly set printedEnvVars to ensure a clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		expectedEnvVars := map[string]string{
			"WINDSOR_CONTEXT":      "mock-context",
			"WINDSOR_PROJECT_ROOT": filepath.FromSlash("/mock/project/root"),
			"WINDSOR_EXEC_MODE":    "container",
			"WINDSOR_MANAGED_ENV":  "WINDSOR_CONTEXT,WINDSOR_PROJECT_ROOT,WINDSOR_EXEC_MODE",
		}

		capturedEnvVars := make(map[string]string)
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			for k, v := range envVars {
				capturedEnvVars[k] = v
			}
			return nil
		}

		err = windsorEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		for key, expectedValue := range expectedEnvVars {
			if capturedEnvVars[key] != expectedValue {
				t.Errorf("%s = %v, want %v", key, capturedEnvVars[key], expectedValue)
			}
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock project root error")
		}

		windsorEnvPrinter := NewWindsorEnvPrinter(mocks.Injector)
		err := windsorEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Explicitly set printedEnvVars to ensure a clean state
		mu.Lock()
		printedEnvVars = make(map[string]string)
		mu.Unlock()

		err = windsorEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock project root error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
