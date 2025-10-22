package runtime

import (
	"errors"
	"os"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The RuntimeTest is a test suite for the Runtime struct and its chaining methods.
// It provides comprehensive test coverage for dependency loading, error propagation,
// and command execution in the Windsor CLI runtime system.
// The RuntimeTest acts as a validation framework for runtime functionality,
// ensuring reliable dependency management, proper error handling, and method chaining.

// =============================================================================
// Test Setup
// =============================================================================

// setupMocks creates a new set of mocks for testing
func setupMocks(t *testing.T) *Dependencies {
	t.Helper()

	return &Dependencies{
		Injector:      di.NewInjector(),
		Shell:         shell.NewMockShell(),
		ConfigHandler: config.NewMockConfigHandler(),
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_NewRuntime(t *testing.T) {
	t.Run("CreatesRuntimeWithDependencies", func(t *testing.T) {
		// Given dependencies
		mocks := setupMocks(t)

		// When creating a new runtime
		runtime := NewRuntime(mocks)

		// Then runtime should be created successfully
		if runtime == nil {
			t.Error("Expected runtime to be created")
		}

		if runtime.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}
	})
}

func TestRuntime_LoadShell(t *testing.T) {
	t.Run("LoadsShellSuccessfully", func(t *testing.T) {
		// Given a runtime with dependencies
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should be loaded
		if runtime.Shell == nil {
			t.Error("Expected shell to be loaded")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("CreatesNewShellWhenNoneExists", func(t *testing.T) {
		// Given a runtime without pre-loaded shell
		runtime := NewRuntime()

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should be loaded
		if runtime.Shell == nil {
			t.Error("Expected shell to be loaded")
		}

		// And shell should be registered in injector
		resolvedShell := runtime.Injector.Resolve("shell")
		if resolvedShell == nil {
			t.Error("Expected shell to be registered in injector")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading shell
		result := runtime.LoadShell()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadShell to return the same runtime instance")
		}

		// And shell should not be loaded
		if runtime.Shell != nil {
			t.Error("Expected shell to not be loaded when error exists")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})
}

func TestRuntime_LoadConfigHandler(t *testing.T) {
	t.Run("LoadsConfigHandlerSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// When loading config handler
		result := runtime.LoadConfigHandler()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadConfigHandler to return the same runtime instance")
		}

		// And config handler should be loaded
		if runtime.ConfigHandler == nil {
			t.Error("Expected config handler to be loaded")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When loading config handler
		result := runtime.LoadConfigHandler()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadConfigHandler to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when shell not loaded")
		}

		expectedError := "shell not loaded - call LoadShell() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading config handler
		result := runtime.LoadConfigHandler()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadConfigHandler to return the same runtime instance")
		}

		// And config handler should not be loaded
		if runtime.ConfigHandler != nil {
			t.Error("Expected config handler to not be loaded when error exists")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PropagatesConfigHandlerInitializationError", func(t *testing.T) {
		// Given a runtime with an injector that cannot resolve the shell
		runtime := NewRuntime()
		runtime.Shell = shell.NewMockShell()
		// Don't register the shell in the injector - this will cause initialization to fail

		// When loading config handler
		result := runtime.LoadConfigHandler()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadConfigHandler to return the same runtime instance")
		}

		// And error should be set from initialization failure
		if runtime.err == nil {
			t.Error("Expected error from config handler initialization failure")
		} else {
			expectedError := "failed to initialize config handler"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_LoadEnvPrinters(t *testing.T) {
	t.Run("LoadsEnvPrintersSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And WindsorEnv should always be loaded
		if runtime.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be loaded")
		}

		// And WindsorEnv should be registered in injector
		resolvedWindsorEnv := runtime.Injector.Resolve("windsorEnv")
		if resolvedWindsorEnv == nil {
			t.Error("Expected WindsorEnv to be registered in injector")
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfigHandler() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}

		// And no env printers should be loaded
		if runtime.EnvPrinters.WindsorEnv != nil {
			t.Error("Expected no env printers to be loaded when error exists")
		}
	})

	t.Run("LoadsOnlyEnabledEnvPrinters", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler with specific enabled features
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "aws.enabled":
				return true
			case "azure.enabled":
				return false
			case "docker.enabled":
				return true
			case "cluster.enabled":
				return false
			case "terraform.enabled":
				return true
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And enabled env printers should be loaded
		if runtime.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be loaded when enabled")
		}
		if runtime.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be loaded when enabled")
		}
		if runtime.EnvPrinters.TerraformEnv == nil {
			t.Error("Expected TerraformEnv to be loaded when enabled")
		}

		// And disabled env printers should not be loaded
		if runtime.EnvPrinters.AzureEnv != nil {
			t.Error("Expected AzureEnv to not be loaded when disabled")
		}
		if runtime.EnvPrinters.KubeEnv != nil {
			t.Error("Expected KubeEnv to not be loaded when disabled")
		}
		if runtime.EnvPrinters.TalosEnv != nil {
			t.Error("Expected TalosEnv to not be loaded when cluster driver is not talos/omni")
		}

		// And WindsorEnv should always be loaded
		if runtime.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be loaded")
		}
	})

	t.Run("LoadsWindsorEnvPrinterAlways", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And WindsorEnv should always be loaded regardless of config
		if runtime.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be loaded")
		}

		// And WindsorEnv should be registered in injector
		resolvedWindsorEnv := runtime.Injector.Resolve("windsorEnv")
		if resolvedWindsorEnv == nil {
			t.Error("Expected WindsorEnv to be registered in injector")
		}
	})

	t.Run("LoadsTalosEnvPrinterForTalosDriver", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler with talos driver
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And TalosEnv should be loaded for talos driver
		if runtime.EnvPrinters.TalosEnv == nil {
			t.Error("Expected TalosEnv to be loaded for talos driver")
		}

		// And TalosEnv should be registered in injector
		resolvedTalosEnv := runtime.Injector.Resolve("talosEnv")
		if resolvedTalosEnv == nil {
			t.Error("Expected TalosEnv to be registered in injector")
		}
	})

	t.Run("LoadsTalosEnvPrinterForOmniDriver", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler with omni driver
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And TalosEnv should be loaded for omni driver
		if runtime.EnvPrinters.TalosEnv == nil {
			t.Error("Expected TalosEnv to be loaded for omni driver")
		}

		// And TalosEnv should be registered in injector
		resolvedTalosEnv := runtime.Injector.Resolve("talosEnv")
		if resolvedTalosEnv == nil {
			t.Error("Expected TalosEnv to be registered in injector")
		}
	})

	t.Run("LoadsAzureEnvPrinterWhenEnabled", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler with azure enabled
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "azure.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And AzureEnv should be loaded when enabled
		if runtime.EnvPrinters.AzureEnv == nil {
			t.Error("Expected AzureEnv to be loaded when enabled")
		}

		// And AzureEnv should be registered in injector
		resolvedAzureEnv := runtime.Injector.Resolve("azureEnv")
		if resolvedAzureEnv == nil {
			t.Error("Expected AzureEnv to be registered in injector")
		}

		// And WindsorEnv should always be loaded
		if runtime.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be loaded")
		}
	})

	t.Run("LoadsKubeEnvPrinterWhenEnabled", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler with cluster enabled
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "cluster.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading env printers
		result := runtime.LoadEnvPrinters()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadEnvPrinters to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And KubeEnv should be loaded when cluster is enabled
		if runtime.EnvPrinters.KubeEnv == nil {
			t.Error("Expected KubeEnv to be loaded when cluster is enabled")
		}

		// And KubeEnv should be registered in injector
		resolvedKubeEnv := runtime.Injector.Resolve("kubeEnv")
		if resolvedKubeEnv == nil {
			t.Error("Expected KubeEnv to be registered in injector")
		}

		// And WindsorEnv should always be loaded
		if runtime.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be loaded")
		}
	})
}

func TestRuntime_LoadSecretsProviders(t *testing.T) {
	t.Run("LoadsSecretsProvidersSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfigHandler() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}

		// And no secrets providers should be loaded
		if runtime.SecretsProviders.Sops != nil {
			t.Error("Expected no secrets providers to be loaded when error exists")
		}
		if runtime.SecretsProviders.Onepassword != nil {
			t.Error("Expected no secrets providers to be loaded when error exists")
		}
	})

	t.Run("PropagatesConfigRootError", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler that returns error for GetConfigRoot
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root error")
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated from config root")
		} else {
			expectedError := "error getting config root"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})

	t.Run("LoadsSopsProviderWhenSecretsFileExists", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler, and secrets file exists
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// Mock Stat to return success for secrets.enc.yaml
		runtime.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "secrets.enc.yaml") {
				return nil, nil // Success - file exists
			}
			return nil, errors.New("file not found")
		}

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And Sops provider should be loaded
		if runtime.SecretsProviders.Sops == nil {
			t.Error("Expected Sops provider to be loaded when secrets file exists")
		}

		// And Sops provider should be registered in injector
		resolvedSops := runtime.Injector.Resolve("sopsSecretsProvider")
		if resolvedSops == nil {
			t.Error("Expected Sops provider to be registered in injector")
		}
	})

	t.Run("LoadsOnePasswordSDKProviderWhenTokenExists", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler, OnePassword vaults configured, and SDK token
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {URL: "https://vault1.com", Name: "Vault 1"},
				}
			}
			return nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// Mock Getenv to return SDK token
		runtime.Shims.Getenv = func(key string) string {
			if key == "OP_SERVICE_ACCOUNT_TOKEN" {
				return "test-token"
			}
			return ""
		}

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And OnePassword provider should be loaded
		if runtime.SecretsProviders.Onepassword == nil {
			t.Error("Expected OnePassword provider to be loaded when vaults configured and SDK token exists")
		}

		// And OnePassword provider should be registered in injector
		resolvedOnePassword := runtime.Injector.Resolve("onePasswordSecretsProvider")
		if resolvedOnePassword == nil {
			t.Error("Expected OnePassword provider to be registered in injector")
		}
	})

	t.Run("LoadsOnePasswordCLIProviderWhenNoToken", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler, OnePassword vaults configured, but no SDK token
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {URL: "https://vault1.com", Name: "Vault 1"},
				}
			}
			return nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// Mock Getenv to return no SDK token
		runtime.Shims.Getenv = func(key string) string {
			return ""
		}

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And OnePassword provider should be loaded
		if runtime.SecretsProviders.Onepassword == nil {
			t.Error("Expected OnePassword provider to be loaded when vaults configured but no SDK token")
		}

		// And OnePassword provider should be registered in injector
		resolvedOnePassword := runtime.Injector.Resolve("onePasswordSecretsProvider")
		if resolvedOnePassword == nil {
			t.Error("Expected OnePassword provider to be registered in injector")
		}
	})

	t.Run("DoesNotLoadProvidersWhenNoConfig", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler, but no secrets configuration
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return nil // No secrets configuration
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// Mock Stat to return file not found for secrets files
		runtime.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, errors.New("file not found")
		}

		// Mock Getenv to return no SDK token
		runtime.Shims.Getenv = func(key string) string {
			return ""
		}

		// When loading secrets providers
		result := runtime.LoadSecretsProviders()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadSecretsProviders to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And no secrets providers should be loaded
		if runtime.SecretsProviders.Sops != nil {
			t.Error("Expected Sops provider to not be loaded when no secrets file exists")
		}
		if runtime.SecretsProviders.Onepassword != nil {
			t.Error("Expected OnePassword provider to not be loaded when no vaults configured")
		}
	})
}

func TestRuntime_InstallHook(t *testing.T) {
	t.Run("InstallsHookSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when shell not loaded")
		}

		expectedError := "shell not loaded - call LoadShell() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When installing hook
		result := runtime.InstallHook("bash")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected InstallHook to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})
}

func TestRuntime_SetContext(t *testing.T) {
	t.Run("SetsContextSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And SetContext should have been called on the config handler
		// (We can't easily track this without modifying the mock, so we just verify no error occurred)
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfigHandler() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PropagatesConfigHandlerError", func(t *testing.T) {
		// Given a runtime with a mock shell that returns an error
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		// Create runtime with only the mock shell, no mock config handler
		runtime := NewRuntime()
		runtime.Shell = mockShell
		runtime.Injector.Register("shell", mockShell)
		runtime.LoadConfigHandler()

		// When setting context
		result := runtime.SetContext("test-context")

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected SetContext to return the same runtime instance")
		}

		// And error should be propagated from config handler
		if runtime.err == nil {
			t.Error("Expected error to be propagated from config handler")
		} else {
			expectedError := "error getting project root"
			if !strings.Contains(runtime.err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_PrintContext(t *testing.T) {
	t.Run("PrintsContextSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string {
			return "test-context"
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfigHandler()

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And output should be correct
		if output != "test-context" {
			t.Errorf("Expected output 'test-context', got %q", output)
		}

		// And GetContext should have been called on the config handler
		// (We can't easily track this without modifying the mock, so we just verify the output is correct)
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfigHandler() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}

		// And output should not be set
		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		var output string
		outputFunc := func(s string) {
			output = s
		}

		// When printing context
		result := runtime.PrintContext(outputFunc)

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected PrintContext to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}

		// And output should not be set
		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})
}

func TestRuntime_WriteResetToken(t *testing.T) {
	t.Run("WritesResetTokenSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		mocks.Shell.(*shell.MockShell).WriteResetTokenFunc = func() (string, error) {
			return "/tmp/reset-token", nil
		}
		runtime := NewRuntime(mocks).LoadShell()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}

		// And WriteResetToken should have been called on the shell
		// (We can't easily track this without modifying the mock, so we just verify no error occurred)
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when shell not loaded")
		}

		expectedError := "shell not loaded - call LoadShell() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("PropagatesShellError", func(t *testing.T) {
		// Given a runtime with loaded shell that returns an error
		mocks := setupMocks(t)
		mocks.Shell.(*shell.MockShell).WriteResetTokenFunc = func() (string, error) {
			return "", errors.New("shell error")
		}
		runtime := NewRuntime(mocks).LoadShell()

		// When writing reset token
		result := runtime.WriteResetToken()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected WriteResetToken to return the same runtime instance")
		}

		// And error should be propagated
		if runtime.err == nil {
			t.Error("Expected error to be propagated from shell")
		} else {
			expectedError := "shell error"
			if runtime.err.Error() != expectedError {
				t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
			}
		}
	})
}

func TestRuntime_Do(t *testing.T) {
	t.Run("ReturnsNilWhenNoError", func(t *testing.T) {
		// Given a runtime with no error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)

		// When calling Do
		err := runtime.Do()

		// Then should return nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenErrorSet", func(t *testing.T) {
		// Given a runtime with an error
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks)
		expectedError := errors.New("test error")
		runtime.err = expectedError

		// When calling Do
		err := runtime.Do()

		// Then should return the error
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})
}
