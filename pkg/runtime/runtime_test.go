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
		case "docker.enabled", "cluster.enabled", "terraform.enabled":
			return true
		case "aws.enabled", "azure.enabled", "gcp.enabled":
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

	rt, err := NewRuntime(rtOpts...)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

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
		_, err := NewRuntime(nil)

		// Then no error should be returned

		if err != nil {
			t.Errorf("Expected no error when opts is nil, got: %v", err)
		}
	})

	t.Run("ErrorWhenRuntimeIsNil", func(t *testing.T) {
		// Given nil options
		// When NewRuntime is called
		_, err := NewRuntime(nil)

		// Then no error should be returned

		if err != nil {
			t.Errorf("Expected no error when opts is nil, got: %v", err)
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
		result, err := NewRuntime(rtOpts...)

		// Then shell should be created

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
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
		result, err := NewRuntime(rtOpts...)

		// Then config handler should be created

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
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
		_, err := NewRuntime(rtOpts...)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}

		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about getting project root, got: %v", err)
		}
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
		_, err := NewRuntime(rtOpts...)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}

		if err != nil && !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("Expected error about getting project root, got: %v", err)
		}
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
		rt, err := NewRuntime(rtOpts...)

		// Then context name should default to "local"

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

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
		mockSopsProvider := secrets.NewMockSecretsProvider(mockShell)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mockShell)
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
			},
		}
		rtOpts[0].SecretsProviders.Sops = mockSopsProvider
		rtOpts[0].SecretsProviders.Onepassword = []secrets.SecretsProvider{mockOnepasswordProvider}
		rtOpts[0].EnvPrinters.AwsEnv = mockAwsEnv
		rtOpts[0].EnvPrinters.AzureEnv = mockAzureEnv
		rtOpts[0].EnvPrinters.DockerEnv = mockDockerEnv
		rtOpts[0].EnvPrinters.KubeEnv = mockKubeEnv
		rtOpts[0].EnvPrinters.TalosEnv = mockTalosEnv
		rtOpts[0].EnvPrinters.TerraformEnv = mockTerraformEnv
		rtOpts[0].EnvPrinters.WindsorEnv = mockWindsorEnv

		// When NewRuntime is called
		rt, err := NewRuntime(rtOpts...)

		// Then all overrides should be applied correctly

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

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

		if rt.SecretsProviders.Sops != mockSopsProvider {
			t.Error("Expected Sops provider to be set")
		}

		if len(rt.SecretsProviders.Onepassword) != 1 || rt.SecretsProviders.Onepassword[0] != mockOnepasswordProvider {
			t.Error("Expected Onepassword provider to be set")
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

	t.Run("HandlesConfigHandlerNotLoaded", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when config handler is not loaded")
		}
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

		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Shell)

		rt.SecretsProviders.Sops = mockSopsProvider
		rt.SecretsProviders.Onepassword = []secrets.SecretsProvider{mockOnepasswordProvider}

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

		mockProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		rt.SecretsProviders.Sops = mockProvider

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

	t.Run("ErrorWhenShellNotInitialized", func(t *testing.T) {
		// Given a runtime with nil shell
		rt := &Runtime{}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Shell is nil")
		}

		if !strings.Contains(err.Error(), "shell not initialized") {
			t.Errorf("Expected error about shell not initialized, got: %v", err)
		}
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

		if setCalls["dev"] != true {
			t.Error("Expected dev to be set to true")
		}

		if setCalls["provider"] != "generic" {
			t.Error("Expected provider to be set to generic")
		}
	})

	t.Run("ErrorWhenConfigHandlerNotAvailable", func(t *testing.T) {
		// Given a runtime with nil config handler
		rt := &Runtime{}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when ConfigHandler is nil")
		}

		if !strings.Contains(err.Error(), "config handler not available") {
			t.Errorf("Expected error about config handler not available, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetDevFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set dev
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
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "dev" {
				return fmt.Errorf("set dev failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Set dev fails")
		}

		if !strings.Contains(err.Error(), "failed to set dev mode") {
			t.Errorf("Expected error about set dev mode, got: %v", err)
		}
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

	t.Run("SetsVMDriverForDockerDesktop", func(t *testing.T) {
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

		// Then VM driver should be set
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
			if key == "vm.driver" {
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

	t.Run("ErrorWhenSetVMDriverFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set VM driver
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
			if key == "vm.driver" {
				return fmt.Errorf("set vm.driver failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set vm.driver fails")
		}

		if !strings.Contains(err.Error(), "failed to set vm.driver") {
			t.Errorf("Expected error about set vm.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetProviderFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set provider
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
			if key == "provider" {
				return fmt.Errorf("set provider failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set provider fails")
		}

		if !strings.Contains(err.Error(), "failed to set provider") {
			t.Errorf("Expected error about set provider, got: %v", err)
		}
	})

	t.Run("UsesFullConfigWhenColimaInFlagOverrides", func(t *testing.T) {
		// Given a runtime in dev mode with colima in flag overrides
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

		var setDefaultConfig v1alpha1.Context
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultConfig = cfg
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{
			"vm.driver": "colima",
		}

		// When ApplyConfigDefaults is called with flag overrides
		err := rt.ApplyConfigDefaults(flagOverrides)

		// Then full config should be set (not localhost config)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setDefaultConfig.Network == nil || setDefaultConfig.Network.LoadBalancerIPs == nil {
			t.Error("Expected DefaultConfig_Full with LoadBalancerIPs to be set")
		}

		if setDefaultConfig.Cluster != nil && len(setDefaultConfig.Cluster.Workers.HostPorts) > 0 {
			t.Error("Expected DefaultConfig_Full without hostports to be set")
		}
	})

	t.Run("UsesLocalhostConfigWhenDockerDesktopInFlagOverrides", func(t *testing.T) {
		// Given a runtime in dev mode with docker-desktop in flag overrides
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

		var setDefaultConfig v1alpha1.Context
		mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error {
			setDefaultConfig = cfg
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{
			"vm.driver": "docker-desktop",
		}

		// When ApplyConfigDefaults is called with flag overrides
		err := rt.ApplyConfigDefaults(flagOverrides)

		// Then localhost config should be set (with hostports)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setDefaultConfig.Cluster == nil || len(setDefaultConfig.Cluster.Workers.HostPorts) == 0 {
			t.Error("Expected DefaultConfig_Localhost with hostports to be set")
		}
	})

	t.Run("IgnoresFlagOverridesWhenVMDriverAlreadySet", func(t *testing.T) {
		// Given a runtime in dev mode with vm.driver already set in config
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
			if key == "vm.driver" {
				return "colima"
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

		flagOverrides := map[string]any{
			"vm.driver": "docker-desktop",
		}

		// When ApplyConfigDefaults is called with flag overrides
		err := rt.ApplyConfigDefaults(flagOverrides)

		// Then config should use colima from config (not docker-desktop from overrides)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setDefaultConfig.Cluster != nil && len(setDefaultConfig.Cluster.Workers.HostPorts) > 0 {
			t.Error("Expected DefaultConfig_Full without hostports to be set (colima), not localhost config")
		}
	})

	t.Run("UsesDefaultConfigNoneWhenProviderIsNone", func(t *testing.T) {
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
			if key == "provider" {
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

		if setDefaultConfig.Provider == nil || *setDefaultConfig.Provider != "none" {
			t.Error("Expected DefaultConfig to be set with provider 'none'")
		}
		if setDefaultConfig.Cluster == nil || setDefaultConfig.Cluster.Enabled == nil || !*setDefaultConfig.Cluster.Enabled {
			t.Error("Expected DefaultConfig to have cluster.enabled=true")
		}
		if setDefaultConfig.DNS != nil {
			t.Error("Expected DefaultConfig to have no DNS config")
		}
		if setDefaultConfig.Terraform == nil || setDefaultConfig.Terraform.Enabled == nil || !*setDefaultConfig.Terraform.Enabled {
			t.Error("Expected DefaultConfig to have terraform enabled")
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
}

func TestRuntime_ApplyProviderDefaults(t *testing.T) {
	t.Run("SetsAWSDefaults", func(t *testing.T) {
		// Given a runtime with AWS provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyProviderDefaults is called with "aws"
		err := rt.ApplyProviderDefaults("aws")

		// Then AWS defaults should be set
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
		// Given a runtime with Azure provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyProviderDefaults is called with "azure"
		err := rt.ApplyProviderDefaults("azure")

		// Then Azure defaults should be set
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

	t.Run("SetsGCPDefaults", func(t *testing.T) {
		// Given a runtime with GCP provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyProviderDefaults is called with "gcp"
		err := rt.ApplyProviderDefaults("gcp")

		// Then GCP defaults should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["gcp.enabled"] != true {
			t.Error("Expected gcp.enabled to be set to true")
		}

		if setCalls["cluster.driver"] != "gke" {
			t.Error("Expected cluster.driver to be set to gke")
		}
	})

	t.Run("SetsGenericDefaults", func(t *testing.T) {
		// Given a runtime with generic provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyProviderDefaults is called with "generic"
		err := rt.ApplyProviderDefaults("generic")

		// Then generic defaults should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["cluster.driver"] != "talos" {
			t.Error("Expected cluster.driver to be set to talos")
		}
	})

	t.Run("ErrorWhenConfigHandlerNotAvailable", func(t *testing.T) {
		// Given a runtime with nil config handler
		rt := &Runtime{}

		// When ApplyProviderDefaults is called
		err := rt.ApplyProviderDefaults("aws")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when ConfigHandler is nil")
		}

		if !strings.Contains(err.Error(), "config handler not available") {
			t.Errorf("Expected error about config handler not available, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.enabled" {
				return fmt.Errorf("set aws.enabled failed")
			}
			return nil
		}

		// When ApplyProviderDefaults is called
		err := rt.ApplyProviderDefaults("aws")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Set fails")
		}

		if !strings.Contains(err.Error(), "failed to set aws.enabled") {
			t.Errorf("Expected error about set aws.enabled, got: %v", err)
		}
	})

	t.Run("SetsDefaultsForDevModeWithNoProvider", func(t *testing.T) {
		// Given a runtime in dev mode with no provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		// When ApplyProviderDefaults is called with empty provider
		err := rt.ApplyProviderDefaults("")

		// Then dev mode defaults should be set

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["cluster.driver"] != "talos" {
			t.Error("Expected cluster.driver to be set to talos for dev mode")
		}
	})

	t.Run("GetsProviderFromConfig", func(t *testing.T) {
		// Given a runtime with provider in config
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

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

		// When ApplyProviderDefaults is called with empty provider
		err := rt.ApplyProviderDefaults("")

		// Then provider defaults should be set from config

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["aws.enabled"] != true {
			t.Error("Expected aws.enabled to be set to true")
		}
	})

	t.Run("ErrorWhenSetClusterDriverFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

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

		// When ApplyProviderDefaults is called
		err := rt.ApplyProviderDefaults("generic")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Set cluster.driver fails")
		}

		if !strings.Contains(err.Error(), "failed to set cluster.driver") {
			t.Errorf("Expected error about set cluster.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetAzureDriverFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set Azure driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster.driver failed")
			}
			return nil
		}

		// When ApplyProviderDefaults is called with "azure"
		err := rt.ApplyProviderDefaults("azure")

		// Then an error should be returned

		if err == nil {
			t.Error("Expected error when Set cluster.driver fails for azure")
		}

		if !strings.Contains(err.Error(), "failed to set cluster.driver") {
			t.Errorf("Expected error about set cluster.driver, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetDevModeClusterDriverFails", func(t *testing.T) {
		// Given a runtime in dev mode with a config handler that fails to set cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

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

		// When ApplyProviderDefaults is called
		err := rt.ApplyProviderDefaults("")

		// Then an error should be returned

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
	t.Run("CreatesNewBuildIDWhenNoneExists", func(t *testing.T) {
		// Given a runtime with no existing build ID
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		tmpDir := t.TempDir()
		rt.ProjectRoot = tmpDir

		// When GetBuildID is called
		buildID, err := rt.GetBuildID()

		// Then a new build ID should be created

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if buildID == "" {
			t.Error("Expected build ID to be generated")
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

		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Shell)

		rt.SecretsProviders.Sops = mockSopsProvider
		rt.SecretsProviders.Onepassword = []secrets.SecretsProvider{mockOnepasswordProvider}

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

		mockProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockProvider.LoadSecretsFunc = func() error {
			return errors.New("secrets load failed")
		}

		rt.SecretsProviders.Sops = mockProvider

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

	t.Run("HandlesNilProviders", func(t *testing.T) {
		// Given a runtime with nil secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.SecretsProviders.Sops = nil
		rt.SecretsProviders.Onepassword = nil

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error with nil providers, got: %v", err)
		}
	})

	t.Run("HandlesMixedProviders", func(t *testing.T) {
		// Given a runtime with mixed secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		rt.SecretsProviders.Sops = mockProvider
		rt.SecretsProviders.Onepassword = nil

		// When loadSecrets is called
		err := rt.loadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error with mixed providers, got: %v", err)
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

		// Then SOPS provider should be initialized

		if rt.SecretsProviders.Sops == nil {
			t.Error("Expected SOPS provider to be initialized")
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

		// Then 1Password provider should be initialized for each vault

		if len(rt.SecretsProviders.Onepassword) == 0 {
			t.Error("Expected 1Password provider to be initialized")
		}
		if len(rt.SecretsProviders.Onepassword) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(rt.SecretsProviders.Onepassword))
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

		// Then providers should not be initialized

		if rt.SecretsProviders.Sops != nil {
			t.Error("Expected SOPS provider to be nil when disabled")
		}

		if len(rt.SecretsProviders.Onepassword) > 0 {
			t.Error("Expected 1Password provider to be nil when disabled")
		}
	})

	t.Run("DoesNotOverrideExistingProviders", func(t *testing.T) {
		// Given a runtime with an existing secrets provider
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		rt.SecretsProviders.Sops = existingProvider

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "secrets.sops.enabled" {
				return true
			}
			return false
		}

		// When initializeSecretsProviders is called
		rt.initializeSecretsProviders()

		// Then existing provider should be preserved
		if rt.SecretsProviders.Sops != existingProvider {
			t.Error("Expected existing provider to be preserved")
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

		// Then multiple providers should be initialized
		if len(rt.SecretsProviders.Onepassword) == 0 {
			t.Error("Expected 1Password providers to be initialized")
		}
		if len(rt.SecretsProviders.Onepassword) != 2 {
			t.Errorf("Expected 2 providers, got %d", len(rt.SecretsProviders.Onepassword))
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

		// Then no providers should be initialized
		if len(rt.SecretsProviders.Onepassword) > 0 {
			t.Error("Expected no providers to be initialized for empty vaults map")
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

		// Then no providers should be initialized for invalid data
		if len(rt.SecretsProviders.Onepassword) > 0 {
			t.Error("Expected no providers to be initialized for invalid vault data")
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

		// Then provider should be initialized with explicit ID
		if len(rt.SecretsProviders.Onepassword) == 0 {
			t.Error("Expected 1Password provider to be initialized")
		}
		if len(rt.SecretsProviders.Onepassword) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(rt.SecretsProviders.Onepassword))
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

		// And providers should be initialized
		if len(rt.SecretsProviders.Onepassword) == 0 {
			t.Error("Expected 1Password providers to be initialized")
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

		// When initializeComponents is called
		err := rt.initializeComponents()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil, got: %v", err)
		}
	})
}

func TestRuntime_initializeEnvPrinters(t *testing.T) {
	t.Run("InitializesAwsEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with AWS enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "aws.enabled" {
				return true
			}
			return false
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then AWS env printer should be initialized

		if rt.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be initialized")
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

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized

		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized")
		}
	})

	t.Run("InitializesDockerEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with Docker enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Docker env printer should be initialized

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized")
		}
	})

	t.Run("InitializesKubeEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with cluster enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "cluster.enabled" {
				return true
			}
			return false
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

	t.Run("InitializesTalosEnvWhenDriverIsOmni", func(t *testing.T) {
		// Given a runtime with Omni cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
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

	t.Run("SkipsInitializationWhenShellIsNil", func(t *testing.T) {
		// Given a runtime with nil shell
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.Shell = nil

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then env printers should not be initialized

		if rt.EnvPrinters.AwsEnv != nil {
			t.Error("Expected AwsEnv not to be initialized when Shell is nil")
		}
	})

	t.Run("SkipsInitializationWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then env printers should not be initialized

		if rt.EnvPrinters.AwsEnv != nil {
			t.Error("Expected AwsEnv not to be initialized when ConfigHandler is nil")
		}
	})

	t.Run("DoesNotOverrideExistingPrinters", func(t *testing.T) {
		// Given a runtime with an existing env printer
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingAwsEnv := env.NewMockEnvPrinter()
		rt.EnvPrinters.AwsEnv = existingAwsEnv

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "aws.enabled" {
				return true
			}
			return false
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then existing printer should be preserved

		if rt.EnvPrinters.AwsEnv != existingAwsEnv {
			t.Error("Expected existing AwsEnv to be preserved")
		}
	})

	t.Run("InitializesWindsorEnvWithSecretsProviders", func(t *testing.T) {
		// Given a runtime with secrets providers
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		mockOnepasswordProvider := secrets.NewMockSecretsProvider(mocks.Shell)
		rt.SecretsProviders.Sops = mockSopsProvider
		rt.SecretsProviders.Onepassword = []secrets.SecretsProvider{mockOnepasswordProvider}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Windsor env printer should be initialized with secrets providers

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized with secrets providers")
		}
	})
}
