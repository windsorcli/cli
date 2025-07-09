package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupEnvPipeline(t *testing.T) (*EnvPipeline, di.Injector) {
	t.Helper()

	injector := di.NewInjector()
	pipeline := NewEnvPipeline()

	return pipeline, injector
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewEnvPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new env pipeline
		pipeline := NewEnvPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

func TestNewDefaultEnvPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new default env pipeline
		pipeline := NewDefaultEnvPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestEnvPipeline_Initialize(t *testing.T) {
	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given an env pipeline with mock dependencies
		pipeline, injector := setupEnvPipeline(t)

		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return nil
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShims := NewShims()
		injector.Register("shims", mockShims)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		// Given an env pipeline with failing shell initialization
		pipeline, injector := setupEnvPipeline(t)

		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell init error")
		}
		injector.Register("shell", mockShell)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell init error" {
			t.Errorf("Expected shell init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		// Given an env pipeline with failing config handler initialization
		pipeline, injector := setupEnvPipeline(t)

		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler init error")
		}
		injector.Register("configHandler", mockConfigHandler)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize config handler: config handler init error" {
			t.Errorf("Expected config handler init error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given an env pipeline with failing config loading
		pipeline, injector := setupEnvPipeline(t)

		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		injector.Register("configHandler", mockConfigHandler)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to load config: error retrieving project root: project root error" {
			t.Errorf("Expected load config error, got: %v", err)
		}
	})

	t.Run("InitializesWithoutSecretsProviders", func(t *testing.T) {
		// Given an env pipeline with no secrets providers configured
		pipeline, injector := setupEnvPipeline(t)

		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return nil
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShims := NewShims()
		injector.Register("shims", mockShims)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializesWithDefaultEnvPrinters", func(t *testing.T) {
		// Given an env pipeline with default env printers
		pipeline, injector := setupEnvPipeline(t)

		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "test-token", nil
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return nil
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShims := NewShims()
		injector.Register("shims", mockShims)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Public Methods - Execute
// =============================================================================

func TestEnvPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T) (*EnvPipeline, *shell.MockShell, *config.MockConfigHandler) {
		t.Helper()

		pipeline, injector := setupEnvPipeline(t)

		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "test-token", nil
		}
		injector.Register("shell", mockShell)

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return nil
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShims := NewShims()
		injector.Register("shims", mockShims)

		pipeline.Initialize(injector, context.Background())

		return pipeline, mockShell, mockConfigHandler
	}

	t.Run("ExecutesSuccessfullyInTrustedDirectory", func(t *testing.T) {
		// Given an env pipeline in a trusted directory
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ResetsShellInUntrustedDirectory", func(t *testing.T) {
		// Given an env pipeline in an untrusted directory
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("untrusted directory")
		}

		resetCalled := false
		mockShell.ResetFunc = func(args ...bool) {
			resetCalled = true
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned and reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("LoadsSecretsWhenDecryptIsTrue", func(t *testing.T) {
		// Given an env pipeline with decrypt enabled
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(nil)
		loadSecretsCalled := false
		mockSecretsProvider.LoadSecretsFunc = func() error {
			loadSecretsCalled = true
			return nil
		}
		pipeline.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		ctx := context.WithValue(context.Background(), "decrypt", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and secrets should be loaded
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !loadSecretsCalled {
			t.Error("Expected secrets to be loaded")
		}
	})

	t.Run("HandlesSecretsLoadingErrorInVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing secrets loading in verbose mode
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(nil)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets load error")
		}
		pipeline.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		ctx := context.WithValue(context.Background(), "decrypt", true)
		ctx = context.WithValue(ctx, "verbose", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to load secrets: secrets load error" {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})

	t.Run("IgnoresSecretsLoadingErrorInNonVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing secrets loading in non-verbose mode
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(nil)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets load error")
		}
		pipeline.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		ctx := context.WithValue(context.Background(), "decrypt", true)
		ctx = context.WithValue(ctx, "verbose", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenCollectAndSetEnvVarsFails", func(t *testing.T) {
		// Given an env pipeline with failing env vars collection
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("env vars error")
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to collect and set environment variables: error getting environment variables: env vars error" {
			t.Errorf("Expected env vars error, got: %v", err)
		}
	})

	t.Run("PrintsEnvVarsWhenNotQuiet", func(t *testing.T) {
		// Given an env pipeline with quiet mode disabled
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		printCalled := false
		mockEnvPrinter.PrintFunc = func() error {
			printCalled = true
			return nil
		}
		postEnvHookCalled := false
		mockEnvPrinter.PostEnvHookFunc = func() error {
			postEnvHookCalled = true
			return nil
		}
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		ctx := context.WithValue(context.Background(), "quiet", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and print methods should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !printCalled {
			t.Error("Expected print to be called")
		}
		if !postEnvHookCalled {
			t.Error("Expected post env hook to be called")
		}
	})

	t.Run("SkipsPrintingWhenQuiet", func(t *testing.T) {
		// Given an env pipeline with quiet mode enabled
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		printCalled := false
		mockEnvPrinter.PrintFunc = func() error {
			printCalled = true
			return nil
		}
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		ctx := context.WithValue(context.Background(), "quiet", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and print should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if printCalled {
			t.Error("Expected print to not be called")
		}
	})

	t.Run("ReturnsErrorWhenPrintFailsInVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing print in verbose mode
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		ctx := context.WithValue(context.Background(), "quiet", false)
		ctx = context.WithValue(ctx, "verbose", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to print env vars: print error" {
			t.Errorf("Expected print error, got: %v", err)
		}
	})

	t.Run("IgnoresPrintErrorInNonVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing print in non-verbose mode
		pipeline, mockShell, _ := setup(t)

		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		ctx := context.WithValue(context.Background(), "quiet", false)
		ctx = context.WithValue(ctx, "verbose", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestEnvPipeline_collectAndSetEnvVars(t *testing.T) {
	t.Run("CollectsAndSetsEnvVarsSuccessfully", func(t *testing.T) {
		// Given an env pipeline with mock env printers
		pipeline := NewEnvPipeline()

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR": "test_value",
			}, nil
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		// When collecting and setting env vars
		err := pipeline.collectAndSetEnvVars()

		// Then no error should be returned and env var should be set
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if os.Getenv("TEST_VAR") != "test_value" {
			t.Errorf("Expected TEST_VAR to be 'test_value', got '%s'", os.Getenv("TEST_VAR"))
		}

		// Cleanup
		os.Unsetenv("TEST_VAR")
	})

	t.Run("ReturnsErrorWhenGetEnvVarsFails", func(t *testing.T) {
		// Given an env pipeline with failing env printer
		pipeline := NewEnvPipeline()

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("get env vars failed")
		}
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		// When collecting and setting env vars
		err := pipeline.collectAndSetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting environment variables") {
			t.Errorf("Expected 'error getting environment variables' in error, got: %v", err)
		}
	})
}
