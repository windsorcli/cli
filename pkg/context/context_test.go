package context

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/secrets"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupEnvironmentMocks creates mock components for testing the ExecutionContext
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

	// Create execution context - paths will be set automatically by NewContext
	execCtx := &ExecutionContext{
		Injector: injector,
	}

	ctx, err := NewContext(execCtx)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	return &Mocks{
		Injector:         injector,
		ConfigHandler:    configHandler,
		Shell:            shell,
		ExecutionContext: ctx,
	}
}

// Mocks contains all the mock dependencies for testing
type Mocks struct {
	Injector         di.Injector
	ConfigHandler    config.ConfigHandler
	Shell            shell.Shell
	ExecutionContext *ExecutionContext
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewContext(t *testing.T) {
	t.Run("CreatesContextWithDependencies", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)

		ctx := mocks.ExecutionContext

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
}

// =============================================================================
// Test LoadEnvironment
// =============================================================================

func TestExecutionContext_LoadEnvironment(t *testing.T) {
	t.Run("LoadsEnvironmentSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

		// Set config handler to nil to test error handling
		ctx.ConfigHandler = nil

		err := ctx.LoadEnvironment(false)

		if err == nil {
			t.Error("Expected error when config handler is not loaded")
		}
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
// Test PrintEnvVars
// =============================================================================

func TestExecutionContext_PrintEnvVars(t *testing.T) {
	t.Run("PrintsEnvironmentVariables", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		// Set up environment variables
		ctx.envVars = map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}

		output := ctx.PrintEnvVarsExport()

		if output == "" {
			t.Error("Expected output to be generated")
		}
	})

	t.Run("HandlesEmptyEnvironmentVariables", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		output := ctx.PrintEnvVars()

		if output != "" {
			t.Error("Expected no output for empty environment variables")
		}
	})

}

// =============================================================================
// Test PrintAliases
// =============================================================================

func TestExecutionContext_PrintAliases(t *testing.T) {
	t.Run("PrintsAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		// Set up aliases
		ctx.aliases = map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}

		output := ctx.PrintAliases()

		if output == "" {
			t.Error("Expected output to be generated")
		}
	})

	t.Run("HandlesEmptyAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		output := ctx.PrintAliases()

		if output != "" {
			t.Error("Expected no output for empty aliases")
		}
	})

}

// =============================================================================
// Test ExecutePostEnvHooks
// =============================================================================

func TestExecutionContext_ExecutePostEnvHooks(t *testing.T) {
	t.Run("ExecutesPostEnvHooksSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		// Initialize env printers first
		ctx.initializeEnvPrinters()

		// The WindsorEnv printer should be initialized after initializeEnvPrinters
		if ctx.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		err := ctx.ExecutePostEnvHooks()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// The default WindsorEnv printer should have a working PostEnvHook
	})

	t.Run("HandlesPostEnvHookError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		// Initialize env printers first
		ctx.initializeEnvPrinters()

		// The WindsorEnv printer should be initialized after initializeEnvPrinters
		if ctx.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		// Test that post env hooks work with the default printer
		err := ctx.ExecutePostEnvHooks()

		// This should not error since the default WindsorEnv printer has a working PostEnvHook
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("WrapsErrorWhenPostEnvHookFails", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

		// Initialize env printers first
		ctx.initializeEnvPrinters()

		// Set up a printer that returns an error
		mockPrinter := &MockEnvPrinter{}
		mockPrinter.PostEnvHookFunc = func(directory ...string) error {
			return errors.New("hook error")
		}
		ctx.EnvPrinters.WindsorEnv = mockPrinter

		err := ctx.ExecutePostEnvHooks()

		if err == nil {
			t.Fatal("Expected error when hook fails")
		}

		if !strings.Contains(err.Error(), "failed to execute post env hooks") {
			t.Errorf("Expected error to be wrapped, got: %v", err)
		}

		if !strings.Contains(err.Error(), "hook error") {
			t.Errorf("Expected error to contain original error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Getter Methods
// =============================================================================

func TestExecutionContext_GetEnvVars(t *testing.T) {
	t.Run("ReturnsCopyOfEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_GetAliases(t *testing.T) {
	t.Run("ReturnsCopyOfAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_loadSecrets(t *testing.T) {
	t.Run("LoadsSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_initializeSecretsProviders(t *testing.T) {
	t.Run("InitializesSopsProviderWhenEnabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_LoadEnvironment_WithSecrets(t *testing.T) {
	t.Run("LoadsEnvironmentWithSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_PrintEnvVars_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.Shell = nil

		// This should not panic
		result := ctx.PrintEnvVars()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.envVars = make(map[string]string)

		result := ctx.PrintEnvVars()
		if result != "" {
			t.Errorf("Expected empty string with empty env vars, got: %s", result)
		}
	})
}

func TestExecutionContext_PrintEnvVarsExport_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.Shell = nil

		// This should not panic
		result := ctx.PrintEnvVarsExport()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.envVars = make(map[string]string)

		result := ctx.PrintEnvVarsExport()
		if result != "" {
			t.Errorf("Expected empty string with empty env vars, got: %s", result)
		}
	})
}

func TestExecutionContext_PrintAliases_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.Shell = nil

		// This should not panic
		result := ctx.PrintAliases()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext
		ctx.aliases = make(map[string]string)

		result := ctx.PrintAliases()
		if result != "" {
			t.Errorf("Expected empty string with empty aliases, got: %s", result)
		}
	})
}

func TestExecutionContext_initializeComponents_EdgeCases(t *testing.T) {
	t.Run("HandlesToolsManagerInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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

func TestExecutionContext_CheckTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
		ctx := mocks.ExecutionContext

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
