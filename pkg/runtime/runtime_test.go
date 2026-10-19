package runtime

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
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

type MockToolsManager struct {
	WriteManifestFunc       func() error
	InstallFunc             func() error
	CheckFunc               func() error
	CheckRequirementsFunc   func(reqs tools.Requirements) error
	CheckAuthFunc           func() error
	CheckTerraformFunc      func() error
	GetTerraformCommandFunc func() string
}

func (m *MockToolsManager) WriteManifest() error {
	if m.WriteManifestFunc != nil {
		return m.WriteManifestFunc()
	}
	return nil
}

func (m *MockToolsManager) Install() error {
	if m.InstallFunc != nil {
		return m.InstallFunc()
	}
	return nil
}

func (m *MockToolsManager) Check() error {
	if m.CheckFunc != nil {
		return m.CheckFunc()
	}
	return nil
}

func (m *MockToolsManager) CheckRequirements(reqs tools.Requirements) error {
	if m.CheckRequirementsFunc != nil {
		return m.CheckRequirementsFunc(reqs)
	}
	if m.CheckFunc != nil {
		return m.CheckFunc()
	}
	return nil
}

func (m *MockToolsManager) CheckAuth() error {
	if m.CheckAuthFunc != nil {
		return m.CheckAuthFunc()
	}
	return nil
}

func (m *MockToolsManager) CheckTerraform() error {
	if m.CheckTerraformFunc != nil {
		return m.CheckTerraformFunc()
	}
	return nil
}

func (m *MockToolsManager) GetTerraformCommand() string {
	if m.GetTerraformCommandFunc != nil {
		return m.GetTerraformCommandFunc()
	}
	return "terraform"
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestRuntime_NewRuntime(t *testing.T) {
	t.Run("CreatesContextWithDependencies", func(t *testing.T) {
		// Given a runtime with dependencies
		mocks := setupRuntimeMocks(t)

		// When the runtime is created
		rt := mocks.Runtime

		// Then all dependencies should be set correctly

		if rt == nil {
			t.Fatal("Expected context to be created")
		}

		if rt.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if rt.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if rt.envVars == nil {
			t.Error("Expected envVars map to be initialized")
		}

		if rt.aliases == nil {
			t.Error("Expected aliases map to be initialized")
		}

		if rt.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", rt.ContextName)
		}

		if rt.ProjectRoot != "/test/project" {
			t.Errorf("Expected ProjectRoot to be '/test/project', got: %s", rt.ProjectRoot)
		}

		expectedConfigRoot := filepath.Join("/test/project", "contexts", "test-context")
		if rt.ConfigRoot != expectedConfigRoot {
			t.Errorf("Expected ConfigRoot to be %q, got: %s", expectedConfigRoot, rt.ConfigRoot)
		}

		expectedTemplateRoot := filepath.Join("/test/project", "contexts", "_template")
		if rt.TemplateRoot != expectedTemplateRoot {
			t.Errorf("Expected TemplateRoot to be %q, got: %s", expectedTemplateRoot, rt.TemplateRoot)
		}
	})

	t.Run("ErrorWhenContextIsNil", func(t *testing.T) {
		// Given nil options
		// When NewRuntime is called
		rt := NewRuntime(nil)

		// Then runtime should be created

		if rt == nil {
			t.Error("Expected runtime to be created when opts is nil")
		}
	})

	t.Run("NoPanicWhenOptsIsNil", func(t *testing.T) {
		// Given nil options
		// When NewRuntime is called
		// Then it should create a runtime with defaults
		rt := NewRuntime(nil)
		if rt == nil {
			t.Error("Expected runtime to be created")
		}
	})

	t.Run("NoErrorWhenValidRuntime", func(t *testing.T) {
		// Given valid runtime options
		// When NewRuntime is called
		rt := NewRuntime(&Runtime{
			Shell:         shell.NewMockShell(),
			ConfigHandler: config.NewMockConfigHandler(),
		})

		// Then runtime should be created

		if rt == nil {
			t.Error("Expected runtime to be created when opts is nil")
		}
	})

	t.Run("CreatesShellWhenNotProvided", func(t *testing.T) {
		// Given a runtime with config handler but no shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "test"
		}

		rtOpts := []*Runtime{
			{
				ConfigHandler: mockConfigHandler,
			},
		}

		// When NewRuntime is called
		result := NewRuntime(rtOpts...)

		// Then shell should be created

		if result == nil {
			t.Error("Expected runtime to be created")
		}

		if result.Shell == nil {
			t.Error("Expected shell to be created")
		}
	})

	t.Run("CreatesConfigHandlerWhenNotProvided", func(t *testing.T) {
		// Given a runtime with shell but no config handler
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test", nil
		}

		rtOpts := []*Runtime{
			{
				Shell: mockShell,
			},
		}

		// When NewRuntime is called
		result := NewRuntime(rtOpts...)

		// Then config handler should be created

		if result == nil {
			t.Error("Expected runtime to be created")
		}

		if result.ConfigHandler == nil {
			t.Error("Expected config handler to be created")
		}
	})

	t.Run("ErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a shell that fails to get project root
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		rtOpts := []*Runtime{
			{
				Shell: mockShell,
			},
		}

		// When NewRuntime is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when GetProjectRoot fails")
			}
		}()
		_ = NewRuntime(rtOpts...)
	})

	t.Run("ErrorWhenGetProjectRootFailsOnSecondCall", func(t *testing.T) {
		// Given a shell that fails to get project root
		// Note: With the optimization, GetProjectRoot is only called once when ProjectRoot is empty
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return ""
		}

		rtOpts := []*Runtime{
			{
				Shell:         mockShell,
				ConfigHandler: mockConfigHandler,
				ProjectRoot:   "",
			},
		}

		// When NewRuntime is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when GetProjectRoot fails")
			}
		}()
		_ = NewRuntime(rtOpts...)
	})

	t.Run("DefaultsContextNameToLocalWhenEmpty", func(t *testing.T) {
		// Given a config handler with empty context name
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return ""
		}

		rtOpts := []*Runtime{
			{
				Shell:         mockShell,
				ConfigHandler: mockConfigHandler,
			},
		}

		// When NewRuntime is called
		rt := NewRuntime(rtOpts...)

		// Then context name should default to "local"

		if rt.ContextName != "local" {
			t.Errorf("Expected ContextName to be 'local', got: %s", rt.ContextName)
		}
	})

	t.Run("HandlesAllOverridePaths", func(t *testing.T) {
		// Given runtime options with all paths and dependencies overridden
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}

		mockToolsManager := &MockToolsManager{}
		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		mockResolver := secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mockShell)
		mockAwsEnv := env.NewMockEnvPrinter()
		mockAzureEnv := env.NewMockEnvPrinter()
		mockDockerEnv := env.NewMockEnvPrinter()
		mockKubeEnv := env.NewMockEnvPrinter()
		mockTalosEnv := env.NewMockEnvPrinter()
		mockTerraformEnv := env.NewMockEnvPrinter()
		mockWindsorEnv := env.NewMockEnvPrinter()

		rtOpts := []*Runtime{
			{
				Shell:         mockShell,
				ConfigHandler: mockConfigHandler,
				ContextName:   "custom-context",
				ProjectRoot:   "/custom/project",
				ConfigRoot:    "/custom/config",
				TemplateRoot:  "/custom/template",
				ToolsManager:  mockToolsManager,
				Resolver:      mockResolver,
			},
		}
		rtOpts[0].EnvPrinters.AwsEnv = mockAwsEnv
		rtOpts[0].EnvPrinters.AzureEnv = mockAzureEnv
		rtOpts[0].EnvPrinters.DockerEnv = mockDockerEnv
		rtOpts[0].EnvPrinters.KubeEnv = mockKubeEnv
		rtOpts[0].EnvPrinters.TalosEnv = mockTalosEnv
		rtOpts[0].EnvPrinters.TerraformEnv = mockTerraformEnv
		rtOpts[0].EnvPrinters.WindsorEnv = mockWindsorEnv

		// When NewRuntime is called
		rt := NewRuntime(rtOpts...)

		// Then all overrides should be applied correctly

		if rt.ContextName != "custom-context" {
			t.Errorf("Expected ContextName to be 'custom-context', got: %s", rt.ContextName)
		}

		if rt.ProjectRoot != "/custom/project" {
			t.Errorf("Expected ProjectRoot to be '/custom/project' (from override), got: %s", rt.ProjectRoot)
		}

		if rt.ConfigRoot != "/custom/config" {
			t.Errorf("Expected ConfigRoot to be '/custom/config', got: %s", rt.ConfigRoot)
		}

		if rt.TemplateRoot != "/custom/template" {
			t.Errorf("Expected TemplateRoot to be '/custom/template', got: %s", rt.TemplateRoot)
		}

		if rt.ToolsManager != mockToolsManager {
			t.Error("Expected ToolsManager to be set")
		}

		if rt.Resolver != mockResolver {
			t.Error("Expected Resolver to be set")
		}

		if rt.EnvPrinters.AwsEnv != mockAwsEnv {
			t.Error("Expected AwsEnv to be set")
		}

		if rt.EnvPrinters.AzureEnv != mockAzureEnv {
			t.Error("Expected AzureEnv to be set")
		}

		if rt.EnvPrinters.DockerEnv != mockDockerEnv {
			t.Error("Expected DockerEnv to be set")
		}

		if rt.EnvPrinters.KubeEnv != mockKubeEnv {
			t.Error("Expected KubeEnv to be set")
		}

		if rt.EnvPrinters.TalosEnv != mockTalosEnv {
			t.Error("Expected TalosEnv to be set")
		}

		if rt.EnvPrinters.TerraformEnv != mockTerraformEnv {
			t.Error("Expected TerraformEnv to be set")
		}

		if rt.EnvPrinters.WindsorEnv != mockWindsorEnv {
			t.Error("Expected WindsorEnv to be set")
		}
	})

	t.Run("PropagatesGlobalFromShell", func(t *testing.T) {
		// Given a shell reporting global mode
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/home/user/.config/windsor", nil
		}
		mockShell.IsGlobalFunc = func() bool {
			return true
		}
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextFunc = func() string {
			return "local"
		}

		// When NewRuntime is called
		rt := NewRuntime(&Runtime{
			Shell:         mockShell,
			ConfigHandler: configHandler,
		})

		// Then Global should be true on the runtime
		if !rt.Global {
			t.Error("Expected rt.Global to be true when shell.IsGlobal() is true")
		}
	})

	t.Run("GlobalFalseWhenShellIsNotGlobal", func(t *testing.T) {
		// Given a shell not reporting global mode
		mocks := setupRuntimeMocks(t)

		// When NewRuntime has been called via setup
		rt := mocks.Runtime

		// Then Global should be false
		if rt.Global {
			t.Error("Expected rt.Global to be false when shell.IsGlobal() is false")
		}
	})

	t.Run("GlobalOverrideIsHonored", func(t *testing.T) {
		// Given an override setting Global to true even though shell is not global
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/some/project", nil
		}
		mockShell.IsGlobalFunc = func() bool {
			return false
		}
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextFunc = func() string {
			return "local"
		}

		// When NewRuntime is called with Global override
		rt := NewRuntime(&Runtime{
			Shell:         mockShell,
			ConfigHandler: configHandler,
			Global:        true,
		})

		// Then Global should be true
		if !rt.Global {
			t.Error("Expected rt.Global to be true when override is set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_CheckTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a runtime with a tools manager
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		rt.ToolsManager = mockToolsManager

		// When CheckTools is called
		err := rt.CheckTools()

		// Then no error should be returned

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("InitializesToolsManagerWhenNil", func(t *testing.T) {
		// Given a runtime with nil tools manager
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetFunc = func(key string) interface{} {
			return nil
		}

		rt.ToolsManager = nil

		// When CheckTools is called
		err := rt.CheckTools()

		// Then tools manager should be initialized

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if rt.ToolsManager == nil {
			t.Error("Expected ToolsManager to be initialized")
		}
	})

	t.Run("HandlesToolsManagerUnavailable", func(t *testing.T) {
		// Given a runtime with nil tools manager and config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ToolsManager = nil
		rt.ConfigHandler = nil

		// When CheckTools is called
		err := rt.CheckTools()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when ToolsManager cannot be initialized")
		}

		if !strings.Contains(err.Error(), "tools manager not available") {
			t.Errorf("Expected error about tools manager not available, got: %v", err)
		}
	})

	t.Run("HandlesToolsManagerCheckError", func(t *testing.T) {
		// Given a runtime with a tools manager that fails to check
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return errors.New("tools check failed")
		}
		rt.ToolsManager = mockToolsManager

		// When CheckTools is called
		err := rt.CheckTools()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when ToolsManager.Check fails")
		}

		if !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected error about tools check, got: %v", err)
		}

		if !strings.Contains(err.Error(), "tools check failed") {
			t.Errorf("Expected error to contain original error, got: %v", err)
		}
	})
}

func TestRuntime_PrepareTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a runtime with a tools manager
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mockToolsManager.InstallFunc = func() error {
			return nil
		}
		rt.ToolsManager = mockToolsManager

		// When PrepareTools is called
		err := rt.PrepareTools()

		// Then no error should be returned

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorWhenCheckFails", func(t *testing.T) {
		// Given a runtime with a tools manager that fails to check
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}
		rt.ToolsManager = mockToolsManager

		// When PrepareTools is called
		err := rt.PrepareTools()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Check fails")
		}

		if !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected error about checking tools, got: %v", err)
		}
	})

	t.Run("ErrorWhenInstallFails", func(t *testing.T) {
		// Given a runtime with a tools manager that fails to install
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mockToolsManager.InstallFunc = func() error {
			return fmt.Errorf("tools install failed")
		}
		rt.ToolsManager = mockToolsManager

		// When PrepareTools is called
		err := rt.PrepareTools()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Install fails")
		}

		if !strings.Contains(err.Error(), "error installing tools") {
			t.Errorf("Expected error about installing tools, got: %v", err)
		}
	})

	t.Run("ErrorWhenToolsManagerCannotBeInitialized", func(t *testing.T) {
		// Given a runtime with nil tools manager and config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ToolsManager = nil
		rt.ConfigHandler = nil

		// When PrepareTools is called
		err := rt.PrepareTools()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when ToolsManager cannot be initialized")
		}

		if !strings.Contains(err.Error(), "tools manager not available") {
			t.Errorf("Expected error about tools manager not available, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestRuntime_initializeComponents_EdgeCases(t *testing.T) {
	t.Run("ReturnsNil", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When InitializeComponents is called
		err := rt.InitializeComponents()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil, got: %v", err)
		}
	})
}

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
