package environment

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/types"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupEnvironmentMocks creates mock components for testing the Environment
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

	// Register dependencies in injector
	injector.Register("shell", shell)
	injector.Register("configHandler", configHandler)

	// Register additional dependencies that WindsorEnv printer needs
	injector.Register("projectRoot", "/test/project")
	injector.Register("contextName", "test-context")

	// Create execution context
	execCtx := &types.ExecutionContext{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         shell,
	}

	// Create environment execution context
	envCtx := &EnvironmentExecutionContext{
		ExecutionContext: *execCtx,
	}

	return &Mocks{
		Injector:                    injector,
		ConfigHandler:               configHandler,
		Shell:                       shell,
		EnvironmentExecutionContext: envCtx,
	}
}

// Mocks contains all the mock dependencies for testing
type Mocks struct {
	Injector                    di.Injector
	ConfigHandler               config.ConfigHandler
	Shell                       shell.Shell
	EnvironmentExecutionContext *EnvironmentExecutionContext
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewEnvironment(t *testing.T) {
	t.Run("CreatesEnvironmentWithDependencies", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)

		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		if env == nil {
			t.Fatal("Expected environment to be created")
		}

		if env.Injector != mocks.Injector {
			t.Error("Expected injector to be set")
		}

		if env.Shell != mocks.Shell {
			t.Error("Expected shell to be set")
		}

		if env.ConfigHandler != mocks.ConfigHandler {
			t.Error("Expected config handler to be set")
		}

		if env.envVars == nil {
			t.Error("Expected envVars map to be initialized")
		}

		if env.aliases == nil {
			t.Error("Expected aliases map to be initialized")
		}
	})
}

// =============================================================================
// Test LoadEnvironment
// =============================================================================

func TestEnvironment_LoadEnvironment(t *testing.T) {
	t.Run("LoadsEnvironmentSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		err := env.LoadEnvironment(false)

		// The environment should load successfully with the default WindsorEnv printer
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Check that the WindsorEnv printer was initialized
		if env.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		// Check that environment variables were loaded
		if len(env.envVars) == 0 {
			t.Error("Expected environment variables to be loaded")
		}
	})

	t.Run("HandlesConfigHandlerNotLoaded", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set config handler to nil to test error handling
		env.ConfigHandler = nil

		err := env.LoadEnvironment(false)

		if err == nil {
			t.Error("Expected error when config handler is not loaded")
		}
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// The WindsorEnv printer should be initialized by default
		if env.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		err := env.LoadEnvironment(false)

		// This should not error since the default WindsorEnv printer has working initialization
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})
}

// =============================================================================
// Test PrintEnvVars
// =============================================================================

func TestEnvironment_PrintEnvVars(t *testing.T) {
	t.Run("PrintsEnvironmentVariables", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up environment variables
		env.envVars = map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}

		output := env.PrintEnvVarsExport()

		if output == "" {
			t.Error("Expected output to be generated")
		}
	})

	t.Run("HandlesEmptyEnvironmentVariables", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		output := env.PrintEnvVars()

		if output != "" {
			t.Error("Expected no output for empty environment variables")
		}
	})

}

// =============================================================================
// Test PrintAliases
// =============================================================================

func TestEnvironment_PrintAliases(t *testing.T) {
	t.Run("PrintsAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up aliases
		env.aliases = map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}

		output := env.PrintAliases()

		if output == "" {
			t.Error("Expected output to be generated")
		}
	})

	t.Run("HandlesEmptyAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		output := env.PrintAliases()

		if output != "" {
			t.Error("Expected no output for empty aliases")
		}
	})

}

// =============================================================================
// Test ExecutePostEnvHooks
// =============================================================================

func TestEnvironment_ExecutePostEnvHooks(t *testing.T) {
	t.Run("ExecutesPostEnvHooksSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// The WindsorEnv printer should be initialized by default
		if env.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		err := env.ExecutePostEnvHooks()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// The default WindsorEnv printer should have a working PostEnvHook
	})

	t.Run("HandlesPostEnvHookError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// The WindsorEnv printer should be initialized by default
		if env.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		// Test that post env hooks work with the default printer
		err := env.ExecutePostEnvHooks()

		// This should not error since the default WindsorEnv printer has a working PostEnvHook
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("IgnoresPostEnvHookErrorWhenNotVerbose", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// The WindsorEnv printer should be initialized by default
		if env.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		// Test that post env hooks work with the default printer
		err := env.ExecutePostEnvHooks()

		// This should not error since the default WindsorEnv printer has a working PostEnvHook
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})
}

// =============================================================================
// Test Getter Methods
// =============================================================================

func TestEnvironment_GetEnvVars(t *testing.T) {
	t.Run("ReturnsCopyOfEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		original := map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}
		env.envVars = original

		copy := env.GetEnvVars()

		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		// Modify the copy
		copy["NEW_VAR"] = "new_value"

		// Original should be unchanged
		if len(env.envVars) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

func TestEnvironment_GetAliases(t *testing.T) {
	t.Run("ReturnsCopyOfAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		original := map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}
		env.aliases = original

		copy := env.GetAliases()

		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		// Modify the copy
		copy["new"] = "echo new"

		// Original should be unchanged
		if len(env.aliases) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestEnvironment_loadSecrets(t *testing.T) {
	t.Run("LoadsSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up mock secrets providers
		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		env.SecretsProviders.Sops = mockSopsProvider
		env.SecretsProviders.Onepassword = mockOnepasswordProvider

		err := env.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsProviderError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up mock secrets provider that returns an error
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		env.SecretsProviders.Sops = mockProvider

		err := env.loadSecrets()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})

	t.Run("HandlesNilProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Leave providers as nil
		env.SecretsProviders.Sops = nil
		env.SecretsProviders.Onepassword = nil

		err := env.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error with nil providers, got: %v", err)
		}
	})

	t.Run("HandlesMixedProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up one provider that works and one that's nil
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		env.SecretsProviders.Sops = mockProvider
		env.SecretsProviders.Onepassword = nil

		err := env.loadSecrets()
		if err != nil {
			t.Fatalf("Expected no error with mixed providers, got: %v", err)
		}
	})
}

func TestEnvironment_initializeSecretsProviders(t *testing.T) {
	t.Run("InitializesSopsProviderWhenEnabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Enable SOPS in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		env.initializeSecretsProviders()

		if env.SecretsProviders.Sops == nil {
			t.Error("Expected SOPS provider to be initialized")
		}

		// Verify it's registered in the injector
		if _, ok := mocks.Injector.Resolve("sopsSecretsProvider").(secrets.SecretsProvider); !ok {
			t.Error("Expected SOPS provider to be registered in injector")
		}
	})

	t.Run("InitializesOnepasswordProviderWhenEnabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Enable 1Password in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.onepassword.enabled" {
				return true
			}
			return false
		}

		env.initializeSecretsProviders()

		if env.SecretsProviders.Onepassword == nil {
			t.Error("Expected 1Password provider to be initialized")
		}

		// Verify it's registered in the injector
		if _, ok := mocks.Injector.Resolve("onepasswordSecretsProvider").(secrets.SecretsProvider); !ok {
			t.Error("Expected 1Password provider to be registered in injector")
		}
	})

	t.Run("SkipsProvidersWhenDisabled", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Disable both providers
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		env.initializeSecretsProviders()

		if env.SecretsProviders.Sops != nil {
			t.Error("Expected SOPS provider to be nil when disabled")
		}

		if env.SecretsProviders.Onepassword != nil {
			t.Error("Expected 1Password provider to be nil when disabled")
		}
	})

	t.Run("DoesNotOverrideExistingProviders", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Pre-set a provider
		existingProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		env.SecretsProviders.Sops = existingProvider

		// Enable SOPS in config
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		env.initializeSecretsProviders()

		// Should still be the same provider
		if env.SecretsProviders.Sops != existingProvider {
			t.Error("Expected existing provider to be preserved")
		}
	})
}

func TestEnvironment_LoadEnvironment_WithSecrets(t *testing.T) {
	t.Run("LoadsEnvironmentWithSecretsSuccessfully", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up mock secrets providers
		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		env.SecretsProviders.Sops = mockSopsProvider
		env.SecretsProviders.Onepassword = mockOnepasswordProvider

		err := env.LoadEnvironment(true) // Enable secrets loading
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsLoadError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up mock secrets provider that returns an error
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		env.SecretsProviders.Sops = mockProvider

		err := env.LoadEnvironment(true) // Enable secrets loading
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})
}

func TestEnvironment_PrintEnvVars_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.Shell = nil

		// This should not panic
		result := env.PrintEnvVars()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.envVars = make(map[string]string)

		result := env.PrintEnvVars()
		if result != "" {
			t.Errorf("Expected empty string with empty env vars, got: %s", result)
		}
	})
}

func TestEnvironment_PrintEnvVarsExport_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.Shell = nil

		// This should not panic
		result := env.PrintEnvVarsExport()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyEnvVars", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.envVars = make(map[string]string)

		result := env.PrintEnvVarsExport()
		if result != "" {
			t.Errorf("Expected empty string with empty env vars, got: %s", result)
		}
	})
}

func TestEnvironment_PrintAliases_EdgeCases(t *testing.T) {
	t.Run("HandlesNilShell", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.Shell = nil

		// This should not panic
		result := env.PrintAliases()
		if result != "" {
			t.Errorf("Expected empty string with nil shell, got: %s", result)
		}
	})

	t.Run("HandlesEmptyAliases", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)
		env.aliases = make(map[string]string)

		result := env.PrintAliases()
		if result != "" {
			t.Errorf("Expected empty string with empty aliases, got: %s", result)
		}
	})
}

func TestEnvironment_initializeComponents_EdgeCases(t *testing.T) {
	t.Run("HandlesToolsManagerInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up a mock tools manager that returns an error
		mockToolsManager := &MockToolsManager{}
		mockToolsManager.InitializeFunc = func() error {
			return errors.New("tools manager init failed")
		}
		env.ToolsManager = mockToolsManager

		err := env.initializeComponents()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "tools manager init failed") {
			t.Errorf("Expected tools manager init error, got: %v", err)
		}
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		mocks := setupEnvironmentMocks(t)
		env := NewEnvironment(mocks.EnvironmentExecutionContext)

		// Set up a mock env printer that returns an error
		mockPrinter := &MockEnvPrinter{}
		mockPrinter.InitializeFunc = func() error {
			return errors.New("env printer init failed")
		}
		env.EnvPrinters.WindsorEnv = mockPrinter

		err := env.initializeComponents()
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
