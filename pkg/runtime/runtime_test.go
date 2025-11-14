package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupEnvironmentMocks creates mock components for testing the Runtime
func setupEnvironmentMocks(t *testing.T) *Mocks {
	t.Helper()

	injector := di.NewInjector()
	configHandler := config.NewMockConfigHandler()
	shell := shell.NewMockShell()

	// Set up basic configuration
	configHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "docker.enabled", "cluster.enabled", "terraform.enabled":
			return true
		case "aws.enabled", "azure.enabled":
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

	// Set up shell mock to return output
	shell.RenderEnvVarsFunc = func(envVars map[string]string, export bool) string {
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

	shell.RenderAliasesFunc = func(aliases map[string]string) string {
		var result string
		for key, value := range aliases {
			result += "alias " + key + "='" + value + "'\n"
		}
		return result
	}

	// Set up session token mock
	shell.GetSessionTokenFunc = func() (string, error) {
		return "mock-session-token", nil
	}

	// Set up GetProjectRoot mock
	shell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	// Set up GetContext mock
	configHandler.GetContextFunc = func() string {
		return "test-context"
	}

	// Register dependencies in injector
	injector.Register("shell", shell)
	injector.Register("configHandler", configHandler)

	// Register additional dependencies that WindsorEnv printer needs
	injector.Register("projectRoot", "/test/project")
	injector.Register("contextName", "test-context")

	// Create execution context - paths will be set automatically by NewRuntime
	rt := &Runtime{
		Injector: injector,
	}

	ctx, err := NewRuntime(rt)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         shell,
		Runtime:       ctx,
	}
}

// Mocks contains all the mock dependencies for testing
type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Runtime       *Runtime
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewRuntime(t *testing.T) {
	t.Run("CreatesContextWithDependencies", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)

		ctx := mocks.Runtime

		if ctx == nil {
			t.Fatal("Expected context to be created")
		}

		if ctx.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if ctx.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if ctx.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if ctx.envVars == nil {
			t.Error("Expected envVars map to be initialized")
		}

		if ctx.aliases == nil {
			t.Error("Expected aliases map to be initialized")
		}

		if ctx.ContextName != "test-context" {
			t.Errorf("Expected ContextName to be 'test-context', got: %s", ctx.ContextName)
		}

		if ctx.ProjectRoot != "/test/project" {
			t.Errorf("Expected ProjectRoot to be '/test/project', got: %s", ctx.ProjectRoot)
		}

		expectedConfigRoot := filepath.Join("/test/project", "contexts", "test-context")
		if ctx.ConfigRoot != expectedConfigRoot {
			t.Errorf("Expected ConfigRoot to be %q, got: %s", expectedConfigRoot, ctx.ConfigRoot)
		}

		expectedTemplateRoot := filepath.Join("/test/project", "contexts", "_template")
		if ctx.TemplateRoot != expectedTemplateRoot {
			t.Errorf("Expected TemplateRoot to be %q, got: %s", expectedTemplateRoot, ctx.TemplateRoot)
		}
	})

	t.Run("ErrorWhenContextIsNil", func(t *testing.T) {
		_, err := NewRuntime(nil)

		if err == nil {
			t.Error("Expected error when context is nil")
		}

		if !strings.Contains(err.Error(), "execution context is required") {
			t.Errorf("Expected error about execution context required, got: %v", err)
		}
	})

	t.Run("ErrorWhenInjectorIsNil", func(t *testing.T) {
		ctx := &Runtime{}

		_, err := NewRuntime(ctx)

		if err == nil {
			t.Error("Expected error when injector is nil")
		}

		if !strings.Contains(err.Error(), "injector is required") {
			t.Errorf("Expected error about injector required, got: %v", err)
		}
	})

	t.Run("CreatesShellWhenNotProvided", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "test"
		}
		injector.Register("configHandler", mockConfigHandler)

		ctx := &Runtime{
			Injector: injector,
		}

		result, err := NewRuntime(ctx)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if result.Shell == nil {
			t.Error("Expected shell to be created")
		}
	})

	t.Run("CreatesConfigHandlerWhenNotProvided", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test", nil
		}
		injector.Register("shell", mockShell)

		ctx := &Runtime{
			Injector: injector,
			Shell:    mockShell,
		}

		result, err := NewRuntime(ctx)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if result.ConfigHandler == nil {
			t.Error("Expected config handler to be created")
		}
	})
}

// =============================================================================
// Test LoadEnvironment
// =============================================================================

func TestRuntime_LoadEnvironment(t *testing.T) {
	t.Run("LoadsEnvironmentSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		err := ctx.LoadEnvironment(false)

		// The context should load successfully with the default WindsorEnv printer
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Check that the WindsorEnv printer was initialized
		if ctx.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		// Check that environment variables were loaded
		if len(ctx.envVars) == 0 {
			t.Error("Expected environment variables to be loaded")
		}
	})

	t.Run("HandlesConfigHandlerNotLoaded", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set config handler to nil to test error handling
		ctx.ConfigHandler = nil

		err := ctx.LoadEnvironment(false)

		if err == nil {
			t.Error("Expected error when config handler is not loaded")
		}
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		err := ctx.LoadEnvironment(false)

		// This should not error since the default WindsorEnv printer has working initialization
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// The WindsorEnv printer should be initialized after LoadEnvironment
		if ctx.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}
	})
}

// =============================================================================

// =============================================================================
// Test Getter Methods
// =============================================================================

func TestRuntime_GetEnvVars(t *testing.T) {
	t.Run("ReturnsCopyOfEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		original := map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}
		ctx.envVars = original

		copy := ctx.GetEnvVars()

		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		// Modify the copy
		copy["NEW_VAR"] = "new_value"

		// Original should be unchanged
		if len(ctx.envVars) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

func TestRuntime_GetAliases(t *testing.T) {
	t.Run("ReturnsCopyOfAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		original := map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}
		ctx.aliases = original

		copy := ctx.GetAliases()

		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		// Modify the copy
		copy["new"] = "echo new"

		// Original should be unchanged
		if len(ctx.aliases) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestRuntime_loadSecrets(t *testing.T) {
	t.Run("LoadsSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up mock secrets providers
		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		ctx.SecretsProviders.Sops = mockSopsProvider
		ctx.SecretsProviders.Onepassword = mockOnepasswordProvider

		err := ctx.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsProviderError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up mock secrets provider that returns an error
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		ctx.SecretsProviders.Sops = mockProvider

		err := ctx.loadSecrets()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})

	t.Run("HandlesNilProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Leave providers as nil
		ctx.SecretsProviders.Sops = nil
		ctx.SecretsProviders.Onepassword = nil

		err := ctx.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error with nil providers, got: %v", err)
		}
	})

	t.Run("HandlesMixedProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up one provider that works and one that's nil
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		ctx.SecretsProviders.Sops = mockProvider
		ctx.SecretsProviders.Onepassword = nil

		err := ctx.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error with mixed providers, got: %v", err)
		}
	})
}

func TestRuntime_initializeSecretsProviders(t *testing.T) {
	t.Run("InitializesSopsProviderWhenEnabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Enable SOPS in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		ctx.initializeSecretsProviders()

		if ctx.SecretsProviders.Sops == nil {
			t.Error("Expected SOPS provider to be initialized")
		}

		// Verify it's registered in the injector
		if _, ok := mocks.Injector.Resolve("sopsSecretsProvider").(secrets.SecretsProvider); !ok {
			t.Error("Expected SOPS provider to be registered in injector")
		}
	})

	t.Run("InitializesOnepasswordProviderWhenEnabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Enable 1Password in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.onepassword.enabled" {
				return true
			}
			return false
		}

		ctx.initializeSecretsProviders()

		if ctx.SecretsProviders.Onepassword == nil {
			t.Error("Expected 1Password provider to be initialized")
		}

		// Verify it's registered in the injector
		if _, ok := mocks.Injector.Resolve("onepasswordSecretsProvider").(secrets.SecretsProvider); !ok {
			t.Error("Expected 1Password provider to be registered in injector")
		}
	})

	t.Run("SkipsProvidersWhenDisabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Disable both providers
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		ctx.initializeSecretsProviders()

		if ctx.SecretsProviders.Sops != nil {
			t.Error("Expected SOPS provider to be nil when disabled")
		}

		if ctx.SecretsProviders.Onepassword != nil {
			t.Error("Expected 1Password provider to be nil when disabled")
		}
	})

	t.Run("DoesNotOverrideExistingProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Pre-set a provider
		existingProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		ctx.SecretsProviders.Sops = existingProvider

		// Enable SOPS in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		ctx.initializeSecretsProviders()

		// Should still be the same provider
		if ctx.SecretsProviders.Sops != existingProvider {
			t.Error("Expected existing provider to be preserved")
		}
	})
}

func TestRuntime_LoadEnvironment_WithSecrets(t *testing.T) {
	t.Run("LoadsEnvironmentWithSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up mock secrets providers
		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		ctx.SecretsProviders.Sops = mockSopsProvider
		ctx.SecretsProviders.Onepassword = mockOnepasswordProvider

		err := ctx.LoadEnvironment(true) // Enable secrets loading
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsLoadError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up mock secrets provider that returns an error
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		ctx.SecretsProviders.Sops = mockProvider

		err := ctx.LoadEnvironment(true) // Enable secrets loading
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})
}

func TestRuntime_initializeComponents_EdgeCases(t *testing.T) {
	t.Run("HandlesToolsManagerInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up a mock tools manager that returns an error
		mockToolsManager := &MockToolsManager{}
		mockToolsManager.InitializeFunc = func() error {
			return errors.New("tools manager init failed")
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.initializeComponents()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "tools manager init failed") {
			t.Errorf("Expected tools manager init error, got: %v", err)
		}
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		// Set up a mock env printer that returns an error
		mockPrinter := &MockEnvPrinter{}
		mockPrinter.InitializeFunc = func() error {
			return errors.New("env printer init failed")
		}
		ctx.EnvPrinters.WindsorEnv = mockPrinter

		err := ctx.initializeComponents()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "env printer init failed") {
			t.Errorf("Expected env printer init error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type MockToolsManager struct {
	InitializeFunc    func() error
	WriteManifestFunc func() error
	InstallFunc       func() error
	CheckFunc         func() error
}

func (m *MockToolsManager) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
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

type MockEnvPrinter struct {
	InitializeFunc      func() error
	GetEnvVarsFunc      func() (map[string]string, error)
	GetAliasFunc        func() (map[string]string, error)
	PostEnvHookFunc     func(directory ...string) error
	GetManagedEnvFunc   func() []string
	GetManagedAliasFunc func() []string
	SetManagedEnvFunc   func(env string)
	SetManagedAliasFunc func(alias string)
	ResetFunc           func()
}

func (m *MockEnvPrinter) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

func (m *MockEnvPrinter) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return make(map[string]string), nil
}

func (m *MockEnvPrinter) GetAlias() (map[string]string, error) {
	if m.GetAliasFunc != nil {
		return m.GetAliasFunc()
	}
	return make(map[string]string), nil
}

func (m *MockEnvPrinter) PostEnvHook(directory ...string) error {
	if m.PostEnvHookFunc != nil {
		return m.PostEnvHookFunc(directory...)
	}
	return nil
}

func (m *MockEnvPrinter) GetManagedEnv() []string {
	if m.GetManagedEnvFunc != nil {
		return m.GetManagedEnvFunc()
	}
	return []string{}
}

func (m *MockEnvPrinter) GetManagedAlias() []string {
	if m.GetManagedAliasFunc != nil {
		return m.GetManagedAliasFunc()
	}
	return []string{}
}

func (m *MockEnvPrinter) SetManagedEnv(env string) {
	if m.SetManagedEnvFunc != nil {
		m.SetManagedEnvFunc(env)
	}
}

func (m *MockEnvPrinter) SetManagedAlias(alias string) {
	if m.SetManagedAliasFunc != nil {
		m.SetManagedAliasFunc(alias)
	}
}

func (m *MockEnvPrinter) Reset() {
	if m.ResetFunc != nil {
		m.ResetFunc()
	}
}

// =============================================================================
// Test CheckTools
// =============================================================================

func TestRuntime_CheckTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.InitializeFunc = func() error {
			return nil
		}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.CheckTools()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("InitializesToolsManagerWhenNil", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetFunc = func(key string) interface{} {
			return nil
		}

		ctx.ToolsManager = nil

		err := ctx.CheckTools()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if ctx.ToolsManager == nil {
			t.Error("Expected ToolsManager to be initialized")
		}
	})

	t.Run("HandlesToolsManagerInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		ctx.ToolsManager = nil

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.InitializeFunc = func() error {
			return errors.New("initialization failed")
		}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		err := ctx.CheckTools()

		if err == nil {
			t.Error("Expected error when ToolsManager initialization fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize tools manager") {
			t.Errorf("Expected error about tools manager initialization, got: %v", err)
		}
	})

	t.Run("HandlesToolsManagerCheckError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.InitializeFunc = func() error {
			return nil
		}
		mockToolsManager.CheckFunc = func() error {
			return errors.New("tools check failed")
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.CheckTools()

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

func TestRuntime_HandleSessionReset(t *testing.T) {
	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		err := ctx.HandleSessionReset()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when no session token")
		}
	})

	t.Run("ResetsWhenResetFlagSet", func(t *testing.T) {
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
		})

		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		err := ctx.HandleSessionReset()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when reset flag set")
		}
	})

	t.Run("SkipsResetWhenSessionTokenAndNoResetFlag", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		t.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		err := ctx.HandleSessionReset()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if resetCalled {
			t.Error("Expected Reset not to be called when session token exists and no reset flag")
		}
	})

	t.Run("ErrorWhenShellNotInitialized", func(t *testing.T) {
		ctx := &Runtime{}

		err := ctx.HandleSessionReset()

		if err == nil {
			t.Error("Expected error when Shell is nil")
		}

		if !strings.Contains(err.Error(), "shell not initialized") {
			t.Errorf("Expected error about shell not initialized, got: %v", err)
		}
	})

	t.Run("ErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags failed")
		}

		err := ctx.HandleSessionReset()

		if err == nil {
			t.Error("Expected error when CheckResetFlags fails")
		}

		if !strings.Contains(err.Error(), "failed to check reset flags") {
			t.Errorf("Expected error about check reset flags, got: %v", err)
		}
	})
}

func TestRuntime_ApplyConfigDefaults(t *testing.T) {
	t.Run("SkipsWhenConfigAlreadyLoaded", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}

		err := ctx.ApplyConfigDefaults()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SetsDefaultsForDevMode", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultCalled = true
			return nil
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}

		if setCalls["dev"] != true {
			t.Error("Expected dev to be set to true")
		}

		if setCalls["provider"] != "generic" {
			t.Error("Expected provider to be set to generic")
		}
	})

	t.Run("ErrorWhenConfigHandlerNotAvailable", func(t *testing.T) {
		ctx := &Runtime{}

		err := ctx.ApplyConfigDefaults()

		if err == nil {
			t.Error("Expected error when ConfigHandler is nil")
		}

		if !strings.Contains(err.Error(), "config handler not available") {
			t.Errorf("Expected error about config handler not available, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetDevFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "dev" {
				return fmt.Errorf("set dev failed")
			}
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err == nil {
			t.Error("Expected error when Set dev fails")
		}

		if !strings.Contains(err.Error(), "failed to set dev mode") {
			t.Errorf("Expected error about set dev mode, got: %v", err)
		}
	})

	t.Run("SetsDefaultsForNonDevMode", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultCalled = true
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("SetsVMDriverForDockerDesktop", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return nil
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		vmDriverSet := false
		for key := range setCalls {
			if key == "vm.driver" {
				vmDriverSet = true
				break
			}
		}

		if !vmDriverSet {
			t.Error("Expected vm.driver to be set")
		}
	})

	t.Run("ErrorWhenSetDefaultFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err == nil {
			t.Error("Expected error when SetDefault fails")
		}

		if !strings.Contains(err.Error(), "failed to set default config") {
			t.Errorf("Expected error about set default config, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetVMDriverFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.driver" {
				return fmt.Errorf("set vm.driver failed")
			}
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err == nil {
			t.Error("Expected error when Set vm.driver fails")
		}

		if !strings.Contains(err.Error(), "failed to set vm.driver") {
			t.Errorf("Expected error about set vm.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetProviderFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				return fmt.Errorf("set provider failed")
			}
			return nil
		}

		err := ctx.ApplyConfigDefaults()

		if err == nil {
			t.Error("Expected error when Set provider fails")
		}

		if !strings.Contains(err.Error(), "failed to set provider") {
			t.Errorf("Expected error about set provider, got: %v", err)
		}
	})
}

func TestRuntime_ApplyProviderDefaults(t *testing.T) {
	t.Run("SetsAWSDefaults", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyProviderDefaults("aws")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["aws.enabled"] != true {
			t.Error("Expected aws.enabled to be set to true")
		}

		if setCalls["cluster.driver"] != "eks" {
			t.Error("Expected cluster.driver to be set to eks")
		}
	})

	t.Run("SetsAzureDefaults", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyProviderDefaults("azure")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["azure.enabled"] != true {
			t.Error("Expected azure.enabled to be set to true")
		}

		if setCalls["cluster.driver"] != "aks" {
			t.Error("Expected cluster.driver to be set to aks")
		}
	})

	t.Run("SetsGenericDefaults", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyProviderDefaults("generic")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["cluster.driver"] != "talos" {
			t.Error("Expected cluster.driver to be set to talos")
		}
	})

	t.Run("ErrorWhenConfigHandlerNotAvailable", func(t *testing.T) {
		ctx := &Runtime{}

		err := ctx.ApplyProviderDefaults("aws")

		if err == nil {
			t.Error("Expected error when ConfigHandler is nil")
		}

		if !strings.Contains(err.Error(), "config handler not available") {
			t.Errorf("Expected error about config handler not available, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.enabled" {
				return fmt.Errorf("set aws.enabled failed")
			}
			return nil
		}

		err := ctx.ApplyProviderDefaults("aws")

		if err == nil {
			t.Error("Expected error when Set fails")
		}

		if !strings.Contains(err.Error(), "failed to set aws.enabled") {
			t.Errorf("Expected error about set aws.enabled, got: %v", err)
		}
	})

	t.Run("SetsDefaultsForDevModeWithNoProvider", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return ""
			}
			if key == "cluster.driver" {
				return ""
			}
			return ""
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyProviderDefaults("")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["cluster.driver"] != "talos" {
			t.Error("Expected cluster.driver to be set to talos for dev mode")
		}
	})

	t.Run("GetsProviderFromConfig", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return "aws"
			}
			return ""
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		err := ctx.ApplyProviderDefaults("")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["aws.enabled"] != true {
			t.Error("Expected aws.enabled to be set to true")
		}
	})

	t.Run("ErrorWhenSetClusterDriverFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		err := ctx.ApplyProviderDefaults("generic")

		if err == nil {
			t.Error("Expected error when Set cluster.driver fails")
		}

		if !strings.Contains(err.Error(), "failed to set cluster.driver") {
			t.Errorf("Expected error about set cluster.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetAzureDriverFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		err := ctx.ApplyProviderDefaults("azure")

		if err == nil {
			t.Error("Expected error when Set cluster.driver fails for azure")
		}

		if !strings.Contains(err.Error(), "failed to set cluster.driver") {
			t.Errorf("Expected error about set cluster.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetDevModeClusterDriverFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime
		ctx.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return ""
			}
			if key == "cluster.driver" {
				return ""
			}
			return ""
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		err := ctx.ApplyProviderDefaults("")

		if err == nil {
			t.Error("Expected error when Set cluster.driver fails for dev mode")
		}

		if !strings.Contains(err.Error(), "failed to set cluster.driver") {
			t.Errorf("Expected error about set cluster.driver, got: %v", err)
		}
	})
}

func TestRuntime_PrepareTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mockToolsManager.InstallFunc = func() error {
			return nil
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.PrepareTools()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("ErrorWhenCheckFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("tools check failed")
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.PrepareTools()

		if err == nil {
			t.Error("Expected error when Check fails")
		}

		if !strings.Contains(err.Error(), "error checking tools") {
			t.Errorf("Expected error about checking tools, got: %v", err)
		}
	})

	t.Run("ErrorWhenInstallFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		mockToolsManager := &MockToolsManager{}
		mockToolsManager.CheckFunc = func() error {
			return nil
		}
		mockToolsManager.InstallFunc = func() error {
			return fmt.Errorf("tools install failed")
		}
		ctx.ToolsManager = mockToolsManager

		err := ctx.PrepareTools()

		if err == nil {
			t.Error("Expected error when Install fails")
		}

		if !strings.Contains(err.Error(), "error installing tools") {
			t.Errorf("Expected error about installing tools, got: %v", err)
		}
	})
}

func TestRuntime_GetBuildID(t *testing.T) {
	t.Run("CreatesNewBuildIDWhenNoneExists", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID, err := ctx.GetBuildID()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID == "" {
			t.Error("Expected build ID to be generated")
		}
	})

	t.Run("ReturnsExistingBuildID", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID1, err := ctx.GetBuildID()
		if err != nil {
			t.Fatalf("Failed to get initial build ID: %v", err)
		}

		buildID2, err := ctx.GetBuildID()
		if err != nil {
			t.Fatalf("Failed to get second build ID: %v", err)
		}

		if buildID1 != buildID2 {
			t.Errorf("Expected build IDs to match, got %s and %s", buildID1, buildID2)
		}
	})
}

func TestRuntime_GenerateBuildID(t *testing.T) {
	t.Run("GeneratesAndSavesBuildID", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID, err := ctx.GenerateBuildID()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID == "" {
			t.Error("Expected build ID to be generated")
		}
	})

	t.Run("IncrementsBuildIDOnSubsequentCalls", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID1, err := ctx.GenerateBuildID()
		if err != nil {
			t.Fatalf("Failed to generate first build ID: %v", err)
		}

		buildID2, err := ctx.GenerateBuildID()
		if err != nil {
			t.Fatalf("Failed to generate second build ID: %v", err)
		}

		if buildID1 == buildID2 {
			t.Error("Expected build IDs to be different")
		}
	})

	t.Run("ErrorOnInvalidFormat", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID, err := ctx.incrementBuildID("invalid", "251112")

		if err == nil {
			t.Error("Expected error for invalid format")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ErrorOnInvalidCounter", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID, err := ctx.incrementBuildID("251112.123.abc", "251112")

		if err == nil {
			t.Error("Expected error for invalid counter")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ResetsCounterOnDateChange", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.Runtime

		tmpDir := t.TempDir()
		ctx.ProjectRoot = tmpDir

		buildID, err := ctx.incrementBuildID("251111.123.5", "251112")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !strings.HasPrefix(buildID, "251112.") {
			t.Error("Expected new date in build ID")
		}

		if !strings.HasSuffix(buildID, ".1") {
			t.Error("Expected counter to reset to 1")
		}
	})
}
