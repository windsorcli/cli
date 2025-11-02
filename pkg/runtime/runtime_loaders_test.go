package runtime

import (
	"errors"
	"os"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/context/shell"
)

// The RuntimeLoadersTest is a test suite for the Runtime loader methods.
// It provides comprehensive test coverage for dependency loading, error propagation,
// and method chaining in the Windsor CLI runtime system.
// The RuntimeLoadersTest acts as a validation framework for loader functionality,
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
// Test Loader Methods
// =============================================================================

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

func TestRuntime_LoadConfig(t *testing.T) {
	t.Run("LoadsConfigSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell()

		// When loading config
		result := runtime.LoadConfig()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadConfigHandler to return the same runtime instance")
		}

		// And config should be loaded
		if runtime.ConfigHandler == nil {
			t.Error("Expected config to be loaded")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenShellNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded shell (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When loading config
		result := runtime.LoadConfig()

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

		// When loading config
		result := runtime.LoadConfig()

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

		// When loading config
		result := runtime.LoadConfig()

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

func TestRuntime_LoadSecretsProviders(t *testing.T) {
	t.Run("LoadsSecretsProvidersSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded shell and config handler
		mocks := setupMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/test/config/root", nil
		}
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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

		expectedError := "config handler not loaded - call LoadConfig() first"
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
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

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

func TestRuntime_LoadKubernetes(t *testing.T) {
	t.Run("LoadsKubernetesSuccessfully", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns "talos" for cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And kubernetes client should be registered in injector
		kubernetesClient := runtime.Injector.Resolve("kubernetesClient")
		if kubernetesClient == nil {
			t.Error("Expected kubernetes client to be registered in injector")
		}

		// And cluster client should be loaded
		if runtime.ClusterClient == nil {
			t.Error("Expected cluster client to be loaded")
		}

		// And cluster client should be registered in injector
		clusterClient := runtime.Injector.Resolve("clusterClient")
		if clusterClient == nil {
			t.Error("Expected cluster client to be registered in injector")
		}

		// And kubernetes manager should be loaded
		if runtime.K8sManager == nil {
			t.Error("Expected kubernetes manager to be loaded")
		}

		// And kubernetes manager should be registered in injector
		kubernetesManager := runtime.Injector.Resolve("kubernetesManager")
		if kubernetesManager == nil {
			t.Error("Expected kubernetes manager to be registered in injector")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime without loaded config handler (no pre-loaded dependencies)
		runtime := NewRuntime()

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when config handler not loaded")
		}

		expectedError := "config handler not loaded - call LoadConfig() first"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}

		// And no kubernetes components should be loaded
		if runtime.ClusterClient != nil {
			t.Error("Expected cluster client to not be loaded when error occurs")
		}
		if runtime.K8sManager != nil {
			t.Error("Expected kubernetes manager to not be loaded when error occurs")
		}
	})

	t.Run("ReturnsErrorWhenUnsupportedClusterDriver", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns unsupported cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "unsupported-driver"
			}
			return "mock-string"
		}

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when unsupported cluster driver")
		}

		expectedError := "unsupported cluster driver: unsupported-driver"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}

		// And no kubernetes components should be loaded
		if runtime.ClusterClient != nil {
			t.Error("Expected cluster client to not be loaded when error occurs")
		}
		if runtime.K8sManager != nil {
			t.Error("Expected kubernetes manager to not be loaded when error occurs")
		}
	})

	t.Run("ReturnsEarlyOnExistingError", func(t *testing.T) {
		// Given a runtime with an existing error (no pre-loaded dependencies)
		runtime := NewRuntime()
		runtime.err = errors.New("existing error")

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And kubernetes components should not be loaded
		if runtime.ClusterClient != nil {
			t.Error("Expected cluster client to not be loaded when error exists")
		}
		if runtime.K8sManager != nil {
			t.Error("Expected kubernetes manager to not be loaded when error exists")
		}

		// And original error should be preserved
		if runtime.err.Error() != "existing error" {
			t.Errorf("Expected original error to be preserved, got %v", runtime.err)
		}
	})

	t.Run("ReusesExistingKubernetesClient", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns "talos" for cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}

		// And an existing kubernetes client registered
		existingClient := k8sclient.NewMockKubernetesClient()
		runtime.Injector.Register("kubernetesClient", existingClient)

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And the same kubernetes client should be reused
		currentClient := runtime.Injector.Resolve("kubernetesClient")
		if currentClient != existingClient {
			t.Error("Expected to reuse existing kubernetes client")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReusesExistingClusterClient", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns "talos" for cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}

		// And an existing cluster client
		existingClusterClient := cluster.NewMockClusterClient()
		runtime.ClusterClient = existingClusterClient

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And the same cluster client should be reused
		if runtime.ClusterClient != existingClusterClient {
			t.Error("Expected to reuse existing cluster client")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("ReusesExistingKubernetesManager", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns "talos" for cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}

		// And an existing kubernetes manager
		existingManager := kubernetes.NewMockKubernetesManager(nil)
		runtime.K8sManager = existingManager

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And the same kubernetes manager should be reused
		if runtime.K8sManager != existingManager {
			t.Error("Expected to reuse existing kubernetes manager")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("LoadsKubernetesWithEmptyClusterDriver", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns empty string for cluster driver (generic k8s)
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return ""
			}
			return "mock-string"
		}

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And kubernetes client should be registered in injector
		kubernetesClient := runtime.Injector.Resolve("kubernetesClient")
		if kubernetesClient == nil {
			t.Error("Expected kubernetes client to be registered in injector")
		}

		// And kubernetes manager should be loaded
		if runtime.K8sManager == nil {
			t.Error("Expected kubernetes manager to be loaded")
		}

		// And kubernetes manager should be registered in injector
		kubernetesManager := runtime.Injector.Resolve("kubernetesManager")
		if kubernetesManager == nil {
			t.Error("Expected kubernetes manager to be registered in injector")
		}

		// And cluster client should NOT be loaded (no specific cluster driver)
		if runtime.ClusterClient != nil {
			t.Error("Expected cluster client to not be loaded when no cluster driver specified")
		}

		// And no error should be set
		if runtime.err != nil {
			t.Errorf("Expected no error, got %v", runtime.err)
		}
	})

	t.Run("PropagatesKubernetesManagerInitializationError", func(t *testing.T) {
		// Given a runtime with loaded config handler
		mocks := setupMocks(t)
		runtime := NewRuntime(mocks).LoadShell().LoadConfig()

		// And mock config handler returns "talos" for cluster driver
		mockConfigHandler := runtime.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return "mock-string"
		}

		// And a mock kubernetes manager that fails initialization
		mockManager := kubernetes.NewMockKubernetesManager(nil)
		mockManager.InitializeFunc = func() error {
			return errors.New("initialization failed")
		}
		runtime.K8sManager = mockManager

		// When loading kubernetes
		result := runtime.LoadKubernetes()

		// Then should return the same runtime instance
		if result != runtime {
			t.Error("Expected LoadKubernetes to return the same runtime instance")
		}

		// And error should be set
		if runtime.err == nil {
			t.Error("Expected error when kubernetes manager initialization fails")
		}

		expectedError := "failed to initialize kubernetes manager: initialization failed"
		if runtime.err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, runtime.err.Error())
		}
	})
}
