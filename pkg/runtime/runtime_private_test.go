package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// RuntimeTestMocks contains all the mock dependencies for testing the Runtime
type RuntimeTestMocks struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Runtime       *Runtime
}

// setupRuntimeMocks creates mock components for testing the Runtime with optional overrides
func setupRuntimeMocks(t *testing.T) *RuntimeTestMocks {
	t.Helper()

	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()

	configHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "docker.enabled", "terraform.enabled":
			return true
		case "azure.enabled", "gcp.enabled":
			return false
		default:
			return false
		}
	}

	configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "cluster.driver":
			return "talos"
		default:
			return ""
		}
	}

	mockShell.RenderEnvVarsFunc = func(envVars map[string]string, export bool) string {
		var result string
		for key, value := range envVars {
			if export {
				result += "export " + key + "=" + value + "\n"
			} else {
				result += key + "=" + value + "\n"
			}
		}
		return result
	}

	mockShell.RenderAliasesFunc = func(aliases map[string]string) string {
		var result string
		for key, value := range aliases {
			result += "alias " + key + "='" + value + "'\n"
		}
		return result
	}

	mockShell.GetSessionTokenFunc = func() (string, error) {
		return "mock-session-token", nil
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	configHandler.GetContextFunc = func() string {
		return "test-context"
	}

	rtOpts := []*Runtime{
		{
			Shell:         mockShell,
			ConfigHandler: configHandler,
		},
	}

	rt := NewRuntime(rtOpts...)

	mocks := &RuntimeTestMocks{
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Runtime:       rt,
	}

	return mocks
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestRuntime_initializeSecretsProviders(t *testing.T) {
	t.Run("InitializesSopsProviderWhenEnabled", func(t *testing.T) {
		// Given a runtime with SOPS enabled in config
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should be initialized
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized")
		}
	})

	t.Run("InitializesOnepasswordProviderWhenVaultsConfigured", func(t *testing.T) {
		// Given a runtime with 1Password vaults configured
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{
					"personal": map[string]any{
						"url":  "my.1password.com",
						"name": "Personal",
					},
				}
			}
			return nil
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should be initialized
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized")
		}
	})

	t.Run("SkipsProvidersWhenDisabled", func(t *testing.T) {
		// Given a runtime with secrets providers disabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should still be created (with empty providers)
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized even with no providers")
		}
	})

	t.Run("DoesNotOverrideExistingResolver", func(t *testing.T) {
		// Given a runtime with an existing Resolver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingProvider := secrets.NewMockProvider()
		existingResolver := secrets.NewResolver([]secrets.Provider{existingProvider}, mocks.Shell)
		rt.Resolver = existingResolver

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then existing Resolver should be preserved
		if rt.Resolver != existingResolver {
			t.Error("Expected existing Resolver to be preserved")
		}
	})

	t.Run("InitializesMultipleVaults", func(t *testing.T) {
		// Given a runtime with multiple 1Password vaults configured
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{
					"personal": map[string]any{
						"url":  "my.1password.com",
						"name": "Personal",
					},
					"work": map[string]any{
						"url":  "company.1password.com",
						"name": "Work",
					},
				}
			}
			return nil
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should be initialized
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized")
		}
	})

	t.Run("HandlesEmptyVaultsMap", func(t *testing.T) {
		// Given a runtime with empty vaults map
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{}
			}
			return nil
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should still be created (with empty providers)
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized even with empty vaults map")
		}
	})

	t.Run("HandlesInvalidVaultData", func(t *testing.T) {
		// Given a runtime with invalid vault data structure
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{
					"personal": "not-a-map",
				}
			}
			return nil
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should still be created (invalid vault data is skipped)
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized even with invalid vault data")
		}
	})

	t.Run("HandlesVaultWithExplicitID", func(t *testing.T) {
		// Given a runtime with vault that has explicit ID field
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{
					"personal": map[string]any{
						"id":   "custom-vault-id",
						"url":  "my.1password.com",
						"name": "Personal",
					},
				}
			}
			return nil
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then Resolver should be initialized
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized")
		}
	})

	t.Run("InitializesProvidersBeforeWindsorEnv", func(t *testing.T) {
		// Given a runtime with vaults configured
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "secrets.onepassword.vaults" {
				return map[string]any{
					"personal": map[string]any{
						"url":  "my.1password.com",
						"name": "Personal",
					},
				}
			}
			return nil
		}

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And Resolver should be initialized
		if rt.Resolver == nil {
			t.Error("Expected Resolver to be initialized")
		}

		// And WindsorEnv should be initialized with providers
		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized")
		}
	})
}

func TestRuntime_loadSecrets(t *testing.T) {
	t.Run("LoadsSecretsSuccessfully", func(t *testing.T) {
		// Given a runtime with secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mocks.Shell)

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then secrets should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsProviderError", func(t *testing.T) {
		// Given a runtime with a secrets provider that fails to load
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockProvider := secrets.NewMockProvider()
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockProvider}, mocks.Shell)

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})

	t.Run("HandlesNilResolver", func(t *testing.T) {
		// Given a runtime with nil Resolver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.Resolver = nil

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error with nil Resolver, got: %v", err)
		}
	})

	t.Run("HandlesSingleProvider", func(t *testing.T) {
		// Given a runtime with a single provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockProvider}, mocks.Shell)

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error with single provider, got: %v", err)
		}
	})
}
