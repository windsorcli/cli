package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	v1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	awsv1alpha1 "github.com/windsorcli/cli/api/v1alpha1/aws"
	azurev1alpha1 "github.com/windsorcli/cli/api/v1alpha1/azure"
	gcpv1alpha1 "github.com/windsorcli/cli/api/v1alpha1/gcp"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
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

type MockToolsManager struct {
	WriteManifestFunc       func() error
	InstallFunc             func() error
	CheckFunc               func() error
	CheckAuthFunc           func() error
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

func (m *MockToolsManager) CheckAuth() error {
	if m.CheckAuthFunc != nil {
		return m.CheckAuthFunc()
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

func TestRuntime_LoadEnvironment(t *testing.T) {
	t.Run("LoadsEnvironmentSuccessfully", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}

		if len(rt.envVars) == 0 {
			t.Error("Expected environment variables to be loaded")
		}
	})

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When LoadEnvironment is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config handler is nil")
			}
		}()
		_ = rt.LoadEnvironment(false)
	})

	t.Run("HandlesEnvPrinterInitializationError", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv printer to be initialized")
		}
	})

	t.Run("ErrorWhenGetEnvVarsFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to get env vars
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get env vars")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when GetEnvVars fails")
		}

		if !strings.Contains(err.Error(), "error getting environment variables") {
			t.Errorf("Expected error about getting environment variables, got: %v", err)
		}
	})

	t.Run("ErrorWhenGetAliasFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to get aliases
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get aliases")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when GetAlias fails")
		}

		if !strings.Contains(err.Error(), "error getting aliases") {
			t.Errorf("Expected error about getting aliases, got: %v", err)
		}
	})

	t.Run("ErrorWhenPostEnvHookFails", func(t *testing.T) {
		// Given a runtime with an env printer that fails to execute post env hook
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("failed to execute post env hook")
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when PostEnvHook fails")
		}

		if !strings.Contains(err.Error(), "failed to execute post env hooks") {
			t.Errorf("Expected error about executing post env hooks, got: %v", err)
		}
	})

	t.Run("LoadsEnvironmentWithSecretsSuccessfully", func(t *testing.T) {
		// Given a runtime with secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mocks.Shell)

		// When LoadEnvironment is called with secrets enabled
		err := rt.LoadEnvironment(true)

		// Then environment should load successfully
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSecretsLoadError", func(t *testing.T) {
		// Given a runtime with a secrets provider that fails to load
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockProvider := secrets.NewMockProvider()
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockProvider}, mocks.Shell)

		// When LoadEnvironment is called with secrets enabled
		err := rt.LoadEnvironment(true)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "secrets load failed") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})
}

func TestRuntime_GetEnvVars(t *testing.T) {
	t.Run("ReturnsCopyOfEnvVars", func(t *testing.T) {
		// Given a runtime with environment variables
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		original := map[string]string{
			"TEST_VAR1": "value1",
			"TEST_VAR2": "value2",
		}
		rt.envVars = original

		// When GetEnvVars is called
		copy := rt.GetEnvVars()

		// Then a copy should be returned that doesn't affect the original
		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		copy["NEW_VAR"] = "new_value"

		if len(rt.envVars) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

func TestRuntime_GetAliases(t *testing.T) {
	t.Run("ReturnsCopyOfAliases", func(t *testing.T) {
		// Given a runtime with aliases
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		original := map[string]string{
			"test1": "echo test1",
			"test2": "echo test2",
		}
		rt.aliases = original

		// When GetAliases is called
		copy := rt.GetAliases()

		// Then a copy should be returned that doesn't affect the original
		if len(copy) != len(original) {
			t.Error("Expected copy to have same length as original")
		}

		copy["new"] = "echo new"

		if len(rt.aliases) != len(original) {
			t.Error("Expected original to be unchanged")
		}
	})
}

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

func TestRuntime_HandleSessionReset(t *testing.T) {
	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a runtime with no session token
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when no session token")
		}
	})

	t.Run("ResetsWhenResetFlagSet", func(t *testing.T) {
		// Given a runtime with reset flag set
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
		})

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !resetCalled {
			t.Error("Expected Reset to be called when reset flag set")
		}
	})

	t.Run("SkipsResetWhenSessionTokenAndNoResetFlag", func(t *testing.T) {
		// Given a runtime with session token and no reset flag
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		t.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell := mocks.Shell.(*shell.MockShell)
		resetCalled := false
		mockShell.ResetFunc = func(clearSession ...bool) {
			resetCalled = true
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then reset should not be called

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if resetCalled {
			t.Error("Expected Reset not to be called when session token exists and no reset flag")
		}
	})

	t.Run("PanicsWhenShellNotInitialized", func(t *testing.T) {
		// Given a runtime with nil shell
		rt := &Runtime{}

		// When HandleSessionReset is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when Shell is nil")
			}
		}()
		_ = rt.HandleSessionReset()
	})

	t.Run("ErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		// Given a runtime with a shell that fails to check reset flags
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags failed")
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then an error should be returned

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
		// Given a runtime with config already loaded
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then no error should be returned

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("SetsDefaultsForDevMode", func(t *testing.T) {
		// Given a runtime in dev mode
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then dev mode defaults should be set

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}

		if setCalls["platform"] != "docker" {
			t.Error("Expected platform to be set to docker")
		}
	})

	t.Run("SetsIncusPlatformInDevModeWhenColimaIncus", func(t *testing.T) {
		// Given a runtime in dev mode with colima-incus configuration
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			if key == "vm.runtime" {
				return "incus"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then platform should be set to incus
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["platform"] != "incus" {
			t.Errorf("Expected platform to be set to incus, got: %v", setCalls["platform"])
		}
	})

	t.Run("PanicsWhenConfigHandlerNotAvailable", func(t *testing.T) {
		// Given a runtime with nil config handler
		rt := &Runtime{}

		// When ApplyConfigDefaults is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when ConfigHandler is nil")
			}
		}()
		_ = rt.ApplyConfigDefaults()
	})

	t.Run("SetsDefaultsForNonDevMode", func(t *testing.T) {
		// Given a runtime not in dev mode
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then defaults should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("SetsWorkstationRuntimeForDockerDesktop", func(t *testing.T) {
		// Given a runtime in dev mode with Docker Desktop
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then workstation runtime should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["workstation.runtime"] == nil {
			t.Error("Expected workstation.runtime to be set")
		}
	})

	t.Run("UsesFullConfigForDevModeWithNonDockerDesktop", func(t *testing.T) {
		// Given a runtime in dev mode with non-Docker Desktop VM driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then full config should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("UsesStandardConfigForNonDevMode", func(t *testing.T) {
		// Given a runtime not in dev mode
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then standard config should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("ErrorWhenSetDefaultFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set default
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when SetDefault fails")
		}

		if !strings.Contains(err.Error(), "failed to set default config") {
			t.Errorf("Expected error about set default config, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetWorkstationRuntimeFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set workstation runtime
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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
			if key == "workstation.runtime" {
				return fmt.Errorf("set workstation.runtime failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set workstation.runtime fails")
		}

		if !strings.Contains(err.Error(), "failed to set workstation.runtime") {
			t.Errorf("Expected error about set workstation.runtime, got: %v", err)
		}
	})
	t.Run("ErrorWhenSetPlatformFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set platform
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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
			if key == "platform" {
				return fmt.Errorf("set platform failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set platform fails")
		}

		if !strings.Contains(err.Error(), "failed to set platform") {
			t.Errorf("Expected error about set platform, got: %v", err)
		}
	})

	t.Run("UsesDevConfigInDevMode", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		var setDefaultCalled bool
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultCalled = true
			if cfg.Platform != nil {
				t.Error("Expected DefaultConfig_Dev with no platform set")
			}
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{
			"workstation.runtime": "colima",
		}

		err := rt.ApplyConfigDefaults(flagOverrides)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called with DefaultConfig_Dev")
		}
	})

	t.Run("UsesDefaultConfigNoneWhenPlatformIsNone", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "test"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "none"
			}
			return ""
		}

		var setDefaultConfig v1alpha1.Context
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultConfig = cfg
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		err := rt.ApplyConfigDefaults(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setDefaultConfig.Platform == nil || *setDefaultConfig.Platform != "none" {
			t.Error("Expected DefaultConfig to be set with platform 'none'")
		}
		if setDefaultConfig.DNS != nil {
			t.Error("Expected DefaultConfig to have no DNS config")
		}
		if setDefaultConfig.Terraform != nil {
			t.Error("Expected DefaultConfig to have no terraform config (inline default at call site)")
		}
	})

	t.Run("IgnoresEmptyFlagOverrides", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{}

		// When ApplyConfigDefaults is called with empty flag overrides
		err := rt.ApplyConfigDefaults(flagOverrides)

		// Then defaults should still be applied (using OS default)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called even with empty flag overrides")
		}
	})

	t.Run("SetsPlatformToIncusWhenColimaIncusInFlagOverrides", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		var platformSet string
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "platform" {
				platformSet = value.(string)
			}
			return nil
		}

		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			return nil
		}

		flagOverrides := map[string]any{
			"workstation.runtime": "colima",
			"vm.runtime":          "incus",
		}

		err := rt.ApplyConfigDefaults(flagOverrides)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if platformSet != "incus" {
			t.Errorf("Expected platform to be set to 'incus', got: %s", platformSet)
		}
	})
}

func TestRuntime_SaveConfig(t *testing.T) {
	t.Run("MigratesProviderToPlatformAndClearsProvider", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		saveCount := 0
		platformValue := ""
		providerCleared := false
		mockConfig := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfig.GetStringFunc = func(key string, _ ...string) string {
			switch key {
			case "provider":
				if saveCount == 0 {
					return "docker"
				}
				return "none"
			case "platform":
				return platformValue
			case "workstation.runtime":
				return ""
			default:
				return ""
			}
		}
		mockConfig.SetFunc = func(key string, value any) error {
			if key == "platform" && value != nil {
				platformValue = fmt.Sprint(value)
			}
			if key == "provider" && value == nil {
				providerCleared = true
			}
			return nil
		}
		mockConfig.SaveConfigFunc = func(_ ...bool) error {
			saveCount++
			return nil
		}

		if err := rt.SaveConfig(); err != nil {
			t.Fatalf("first SaveConfig: %v", err)
		}
		if err := rt.SaveConfig(); err != nil {
			t.Fatalf("second SaveConfig: %v", err)
		}
		if platformValue != "docker" {
			t.Errorf("platform must remain docker after second SaveConfig (schema default would overwrite), got %q", platformValue)
		}
		if !providerCleared {
			t.Error("expected provider to be cleared before persistence")
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

func TestRuntime_GetBuildID(t *testing.T) {
	t.Run("ReturnsEmptyStringWhenNoBuildIDExists", func(t *testing.T) {
		// Given a runtime with no existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When GetBuildID is called
		buildID, err := rt.GetBuildID()

		// Then an empty string should be returned (read-only behavior)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID != "" {
			t.Errorf("Expected empty string when no build ID exists, got: %s", buildID)
		}
	})

	t.Run("ReturnsExistingBuildID", func(t *testing.T) {
		// Given a runtime with an existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildID1, err := rt.GetBuildID()
		if err != nil {
			t.Fatalf("Failed to get initial build ID: %v", err)
		}

		// When GetBuildID is called again
		buildID2, err := rt.GetBuildID()

		// Then the same build ID should be returned
		if err != nil {
			t.Fatalf("Failed to get second build ID: %v", err)
		}

		if buildID1 != buildID2 {
			t.Errorf("Expected build IDs to match, got %s and %s", buildID1, buildID2)
		}
	})

	t.Run("ErrorWhenReadFileFails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent file operations")
		}

		// Given a runtime with a build ID file that cannot be read
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildIDDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}

		buildIDFile := filepath.Join(buildIDDir, ".build-id")
		if err := os.WriteFile(buildIDFile, []byte("test-build-id"), 0600); err != nil {
			t.Fatalf("Failed to write build ID file: %v", err)
		}

		if err := os.Chmod(buildIDDir, 0000); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when ReadFile fails")
		}

		if !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about reading build ID file, got: %v", err)
		}
	})

	t.Run("ErrorWhenWriteBuildIDToFileFails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent file operations")
		}

		// Given a runtime with a build ID directory that cannot be written to
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildIDDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}

		if err := os.Chmod(buildIDDir, 0000); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GetBuildID is called
		_, err := rt.GetBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when writeBuildIDToFile fails")
		}

		if !strings.Contains(err.Error(), "failed to set build ID") && !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about setting or reading build ID, got: %v", err)
		}
	})
}

func TestRuntime_GenerateBuildID(t *testing.T) {
	t.Run("GeneratesAndSavesBuildID", func(t *testing.T) {
		// Given a runtime with no existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When GenerateBuildID is called
		buildID, err := rt.GenerateBuildID()

		// Then a new build ID should be generated and saved

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID == "" {
			t.Error("Expected build ID to be generated")
		}
	})

	t.Run("IncrementsBuildIDOnSubsequentCalls", func(t *testing.T) {
		// Given a runtime with an existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildID1, err := rt.GenerateBuildID()
		if err != nil {
			t.Fatalf("Failed to generate first build ID: %v", err)
		}

		// When GenerateBuildID is called again
		buildID2, err := rt.GenerateBuildID()

		// Then the build ID should be incremented
		if err != nil {
			t.Fatalf("Failed to generate second build ID: %v", err)
		}

		if buildID1 == buildID2 {
			t.Error("Expected build IDs to be different")
		}
	})

	t.Run("ErrorOnInvalidFormat", func(t *testing.T) {
		// Given a runtime with an invalid build ID format
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When incrementBuildID is called with invalid format
		buildID, err := rt.incrementBuildID("invalid", "251112")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error for invalid format")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ErrorOnInvalidCounter", func(t *testing.T) {
		// Given a runtime with an invalid build ID counter
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When incrementBuildID is called with invalid counter
		buildID, err := rt.incrementBuildID("251112.123.abc", "251112")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error for invalid counter")
		}

		if buildID != "" {
			t.Error("Expected empty build ID on error")
		}
	})

	t.Run("ResetsCounterOnDateChange", func(t *testing.T) {
		// Given a runtime with a build ID from a previous date
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		oldDate := "251111"
		newDate := "251112"
		existingBuildID := fmt.Sprintf("%s.123.5", oldDate)

		// When incrementBuildID is called with a new date
		buildID, err := rt.incrementBuildID(existingBuildID, newDate)

		// Then the counter should be reset

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.HasPrefix(buildID, newDate) {
			t.Errorf("Expected build ID to start with new date %s, got: %s", newDate, buildID)
		}

		if !strings.HasSuffix(buildID, ".1") {
			t.Errorf("Expected build ID to end with .1, got: %s", buildID)
		}
	})

	t.Run("IncrementsCounterOnSameDate", func(t *testing.T) {
		// Given a runtime with a build ID from the same date
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		date := "251112"
		existingBuildID := fmt.Sprintf("%s.123.5", date)

		// When incrementBuildID is called with the same date
		buildID, err := rt.incrementBuildID(existingBuildID, date)

		// Then the counter should be incremented

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.HasPrefix(buildID, date) {
			t.Errorf("Expected build ID to start with date %s, got: %s", date, buildID)
		}

		if !strings.Contains(buildID, ".123.6") {
			t.Errorf("Expected build ID to contain incremented counter .123.6, got: %s", buildID)
		}
	})

	t.Run("ErrorWhenWriteBuildIDToFileFails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent file operations")
		}

		// Given a runtime with a build ID directory that cannot be written to
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		buildIDDir := filepath.Join(tmpDir, ".windsor")
		if err := os.MkdirAll(buildIDDir, 0750); err != nil {
			t.Fatalf("Failed to create build ID directory: %v", err)
		}

		if err := os.Chmod(buildIDDir, 0000); err != nil {
			t.Fatalf("Failed to set directory permissions: %v", err)
		}
		defer os.Chmod(buildIDDir, 0750)

		// When GenerateBuildID is called
		_, err := rt.GenerateBuildID()

		// Then an error should be returned

		if err == nil {
			t.Fatal("Expected error when writeBuildIDToFile fails")
		}

		if !strings.Contains(err.Error(), "failed to set build ID") && !strings.Contains(err.Error(), "failed to read build ID file") {
			t.Errorf("Expected error about setting or reading build ID, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

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

func TestRuntime_initializeEnvPrinters(t *testing.T) {
	t.Run("InitializesAwsEnvWhenAWSConfigPresent", func(t *testing.T) {
		// Given a runtime with an AWS config block (aws.enabled flag is not required)
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &awsv1alpha1.AWSConfig{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then AWS env printer should be initialized

		if rt.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be initialized")
		}
	})

	t.Run("InitializesAwsEnvWhenPlatformIsAws", func(t *testing.T) {
		// Given a runtime with platform=aws and no aws block
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "aws"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then AWS env printer should be initialized from the platform signal alone
		if rt.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be initialized when platform=aws")
		}
	})

	t.Run("InitializesAzureEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with Azure enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "azure.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Azure: &azurev1alpha1.AzureConfig{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Azure env printer should be initialized

		if rt.EnvPrinters.AzureEnv == nil {
			t.Error("Expected AzureEnv to be initialized")
		}
	})

	t.Run("InitializesGcpEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with GCP enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "gcp.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				GCP: &gcpv1alpha1.GCPConfig{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized

		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized")
		}
	})

	t.Run("InitializesDockerEnvWhenProviderDockerAndWorkstationRuntimeDockerDesktop", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return "docker"
			}
			if key == "workstation.runtime" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized when provider=docker and workstation.runtime=docker-desktop")
		}
	})

	t.Run("InitializesDockerEnvWhenProviderDockerAndWorkstationRuntimeColima", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			if key == "provider" {
				return "docker"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized when provider=docker and workstation.runtime=colima even if docker.enabled=false")
		}
	})

	t.Run("InitializesKubeEnvWhenCloudDriverSet", func(t *testing.T) {
		// Given a runtime with cluster driver set
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Kube env printer should be initialized

		if rt.EnvPrinters.KubeEnv == nil {
			t.Error("Expected KubeEnv to be initialized")
		}
	})

	t.Run("InitializesTalosEnvWhenDriverIsTalos", func(t *testing.T) {
		// Given a runtime with Talos cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Talos env printer should be initialized

		if rt.EnvPrinters.TalosEnv == nil {
			t.Error("Expected TalosEnv to be initialized")
		}
	})

	t.Run("SkipsTalosEnvWhenDriverIsNotTalos", func(t *testing.T) {
		// Given a runtime with non-talos cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "eks"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Talos env printer should not be initialized
		if rt.EnvPrinters.TalosEnv != nil {
			t.Error("Expected TalosEnv to be nil for non-talos cluster driver")
		}
	})

	t.Run("InitializesKubeEnvWhenDriverSet", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "gke"
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.KubeEnv == nil {
			t.Error("Expected KubeEnv to be initialized when cluster.driver is set")
		}
	})

	t.Run("InitializesTerraformEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with Terraform enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Terraform env printer should be initialized

		if rt.EnvPrinters.TerraformEnv == nil {
			t.Error("Expected TerraformEnv to be initialized")
		}
	})

	t.Run("InitializesWindsorEnvAlways", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Windsor env printer should always be initialized

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized")
		}
	})

	t.Run("PanicsWhenShellIsNil", func(t *testing.T) {
		// Given a runtime with nil shell
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.Shell = nil

		// When initializeEnvPrinters is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when Shell is nil")
			}
		}()
		rt.initializeEnvPrinters()
	})

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When initializeEnvPrinters is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when ConfigHandler is nil")
			}
		}()
		rt.initializeEnvPrinters()
	})

	t.Run("DoesNotOverrideExistingPrinters", func(t *testing.T) {
		// Given a runtime with an existing env printer and an AWS config block that would
		// otherwise trigger the gate (presence of an aws: block activates AWS injection now
		// that aws.enabled has been removed).
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingAwsEnv := env.NewMockEnvPrinter()
		rt.EnvPrinters.AwsEnv = existingAwsEnv

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &awsv1alpha1.AWSConfig{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then existing printer should be preserved

		if rt.EnvPrinters.AwsEnv != existingAwsEnv {
			t.Error("Expected existing AwsEnv to be preserved")
		}
	})

	t.Run("InitializesWindsorEnvWithResolver", func(t *testing.T) {
		// Given a runtime with a Resolver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mocks.Shell)

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Windsor env printer should be initialized with Resolver

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized with Resolver")
		}
	})
}
