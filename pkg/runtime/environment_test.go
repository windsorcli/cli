package runtime

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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
		if os.Getenv("NO_CACHE") != "true" {
			t.Errorf("Expected NO_CACHE to default to true, got %q", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("PreservesExplicitNoCacheFalseOnReset", func(t *testing.T) {
		// Given a caller that explicitly opted back into caching (e.g. `NO_CACHE=false windsor
		// exec -- ...`), and no session token — so a reset is still triggered
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		t.Setenv("NO_CACHE", "false")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then the reset still runs, but the caller's explicit NO_CACHE=false is not
		// force-overwritten to true
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if os.Getenv("NO_CACHE") != "false" {
			t.Errorf("Expected explicit NO_CACHE=false to be preserved, got %q", os.Getenv("NO_CACHE"))
		}
	})

	t.Run("PreservesExplicitNoCacheTrueOnReset", func(t *testing.T) {
		// Given a caller that already set NO_CACHE=true itself, and no session token
		t.Cleanup(func() {
			os.Unsetenv("NO_CACHE")
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
		})
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		t.Setenv("NO_CACHE", "true")

		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockShell := mocks.Shell.(*shell.MockShell)
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then the pre-existing value is left alone (not re-set, though the observable result
		// is the same either way)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if os.Getenv("NO_CACHE") != "true" {
			t.Errorf("Expected NO_CACHE to remain true, got %q", os.Getenv("NO_CACHE"))
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

	t.Run("ErrorWhenSetNoCacheFails", func(t *testing.T) {
		// Given a reset with no NO_CACHE value and a Setenv that fails
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.Shims.Getenv = func(string) string { return "" }
		rt.Shims.LookupEnv = func(string) (string, bool) { return "", false }
		rt.Shims.Setenv = func(string, string) error {
			return fmt.Errorf("setenv failed")
		}

		// When HandleSessionReset is called
		err := rt.HandleSessionReset()

		// Then an error about NO_CACHE is returned
		if err == nil || !strings.Contains(err.Error(), "failed to set NO_CACHE") {
			t.Errorf("Expected NO_CACHE error, got: %v", err)
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

	t.Run("ErrorWhenSetenvFails", func(t *testing.T) {
		// Given an env printer with a variable and a Setenv that fails
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "value"}, nil
		}
		rt.EnvPrinters.WindsorEnv = mockEnvPrinter
		rt.Shims.Setenv = func(string, string) error {
			return fmt.Errorf("setenv failed")
		}

		// When LoadEnvironment is called
		err := rt.LoadEnvironment(false)

		// Then an error about setting the variable is returned
		if err == nil || !strings.Contains(err.Error(), "error setting environment variable") {
			t.Errorf("Expected set environment variable error, got: %v", err)
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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestRuntime_initializeEnvPrinters(t *testing.T) {
	t.Run("InitializesAwsEnvWhenAWSConfigPresent", func(t *testing.T) {
		// Given a runtime with an AWS config block (aws.enabled flag is not required)
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				AWS: &cliconfig.AWS{},
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
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				Azure: &cliconfig.Azure{},
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
		// Given a runtime with a gcp: config block present
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				GCP: &cliconfig.GCP{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized

		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized")
		}
	})

	t.Run("InitializesGcpEnvWhenPlatformIsGcp", func(t *testing.T) {
		// Given a runtime with platform=gcp and no gcp block
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "gcp"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized from the platform signal alone
		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized when platform=gcp")
		}
	})

	t.Run("InitializesDockerEnvWhenPlatformDockerAndWorkstationRuntimeDockerDesktop", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
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
			t.Error("Expected DockerEnv to be initialized when platform=docker and workstation.runtime=docker-desktop")
		}
	})

	t.Run("InitializesDockerEnvWhenPlatformDockerAndWorkstationRuntimeColima", func(t *testing.T) {
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
			if key == "platform" {
				return "docker"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized when platform=docker and workstation.runtime=colima even if docker.enabled=false")
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
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				AWS: &cliconfig.AWS{},
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

	t.Run("AlwaysInitializesDotEnvEnv", func(t *testing.T) {
		// Given a runtime with no platform-specific configuration at all
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then DotEnvEnv should be initialized unconditionally
		if rt.EnvPrinters.DotEnvEnv == nil {
			t.Error("Expected DotEnvEnv to always be initialized")
		}
	})

	t.Run("DoesNotOverrideExistingDotEnvEnv", func(t *testing.T) {
		// Given a runtime with an existing DotEnvEnv printer
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingDotEnvEnv := env.NewMockEnvPrinter()
		rt.EnvPrinters.DotEnvEnv = existingDotEnvEnv

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then the existing printer should be preserved
		if rt.EnvPrinters.DotEnvEnv != existingDotEnvEnv {
			t.Error("Expected existing DotEnvEnv to be preserved")
		}
	})
}

// TestRuntime_getAllEnvPrinters tests the ordering returned by getAllEnvPrinters.
func TestRuntime_getAllEnvPrinters(t *testing.T) {
	t.Run("DotEnvEnvIsFirst", func(t *testing.T) {
		// Given a runtime with all env printers initialized
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.initializeEnvPrinters()

		// When getAllEnvPrinters is called
		printers := rt.getAllEnvPrinters()

		// Then DotEnvEnv should be first (lowest precedence) and WindsorEnv last
		if len(printers) == 0 {
			t.Fatal("Expected at least one printer")
		}
		if printers[0] != rt.EnvPrinters.DotEnvEnv {
			t.Error("Expected DotEnvEnv to be first in getAllEnvPrinters")
		}
		if printers[len(printers)-1] != rt.EnvPrinters.WindsorEnv {
			t.Error("Expected WindsorEnv to be last in getAllEnvPrinters")
		}
	})
}
