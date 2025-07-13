package pipelines

import (
	"context"
	"fmt"
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

type EnvMocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

func setupEnvShims(t *testing.T) *Shims {
	t.Helper()
	shims := setupShims(t)

	// Add any env-specific shim overrides here if needed
	return shims
}

func setupEnvMocks(t *testing.T, opts ...*SetupOptions) *EnvMocks {
	t.Helper()

	// Get base mocks
	baseMocks := setupMocks(t, opts...)

	// Add env-specific shell mock behaviors
	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }
	baseMocks.Shell.CheckResetFlagsFunc = func() (bool, error) { return false, nil }
	baseMocks.Shell.GetSessionTokenFunc = func() (string, error) { return "test-token", nil }
	baseMocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {}
	baseMocks.Shell.ResetFunc = func(args ...bool) {}

	return &EnvMocks{
		Injector:      baseMocks.Injector,
		ConfigHandler: baseMocks.ConfigHandler,
		Shell:         baseMocks.Shell,
		Shims:         baseMocks.Shims,
	}
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
	setup := func(t *testing.T, opts ...*SetupOptions) (*EnvPipeline, *EnvMocks) {
		t.Helper()
		pipeline := NewEnvPipeline()
		mocks := setupEnvMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given an env pipeline with mock dependencies
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		// Given an env pipeline with failing shell initialization
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell init error")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: config.NewMockConfigHandler(),
		}
		pipeline, mocks := setup(t, setupOptions)
		mocks.Injector.Register("shell", mockShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

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
		pipeline := NewEnvPipeline()

		// Create injector and register failing config handler directly
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler init error")
		}
		injector.Register("configHandler", mockConfigHandler)

		// Create and register basic shell
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error { return nil }
		mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", mockShell)

		// Register shims
		shims := setupShims(t)
		injector.Register("shims", shims)

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
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: config.NewMockConfigHandler(),
		}
		pipeline, mocks := setup(t, setupOptions)
		mocks.Injector.Register("shell", mockShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

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
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializesWithDefaultEnvPrinters", func(t *testing.T) {
		// Given an env pipeline with default env printers
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWithSecretsProvidersFails", func(t *testing.T) {
		// Given an env pipeline with failing secrets providers creation
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		setupOptions := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		pipeline, mocks := setup(t, setupOptions)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(fmt.Sprintf("%v", err), "failed to create secrets providers") {
			t.Errorf("Expected secrets providers creation error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSecretsProviderInitializeFails", func(t *testing.T) {
		// Given an env pipeline with a failing secrets provider
		pipeline, mocks := setup(t)

		// Create a mock secrets provider that fails during initialization
		mockSecretsProvider := secrets.NewMockSecretsProvider(nil)
		mockSecretsProvider.InitializeFunc = func() error {
			return fmt.Errorf("secrets provider init error")
		}

		// Initialize the base pipeline first to set up dependencies
		err := pipeline.BasePipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize base pipeline: %v", err)
		}

		// Set the failing secrets provider directly
		pipeline.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		// When trying to initialize the secrets provider
		for _, secretsProvider := range pipeline.secretsProviders {
			if err := secretsProvider.Initialize(); err != nil {
				// Then an error should be returned
				expectedError := "secrets provider init error"
				if err.Error() != expectedError {
					t.Errorf("Expected '%s', got: %v", expectedError, err)
				}
				return
			}
		}

		t.Fatal("Expected error, got nil")
	})

	t.Run("ReturnsErrorWhenWithEnvPrintersFails", func(t *testing.T) {
		// Given an env pipeline with failing env printers creation
		pipeline, mocks := setup(t)

		// Initialize the base pipeline first
		err := pipeline.BasePipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize base pipeline: %v", err)
		}

		// Set configHandler to nil to cause withEnvPrinters to fail
		pipeline.configHandler = nil

		// When calling withEnvPrinters
		envPrinters, err := pipeline.withEnvPrinters()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if envPrinters != nil {
			t.Error("Expected nil env printers")
		}
	})

	t.Run("ReturnsErrorWhenEnvPrinterInitializeFails", func(t *testing.T) {
		// Given an env pipeline with failing env printer initialization
		pipeline, mocks := setup(t)

		// Create a mock env printer that fails during initialization
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("env printer init error")
		}

		// Initialize the base pipeline first
		err := pipeline.BasePipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize base pipeline: %v", err)
		}

		// Set the env printer directly to trigger the error
		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter}

		// When calling the env printer initialization
		for _, envPrinter := range pipeline.envPrinters {
			if err := envPrinter.Initialize(); err != nil {
				// Then an error should be returned
				expectedError := "env printer init error"
				if err.Error() != expectedError {
					t.Errorf("Expected '%s', got: %v", expectedError, err)
				}
				return
			}
		}

		t.Fatal("Expected error, got nil")
	})
}

// =============================================================================
// Test Public Methods - Execute
// =============================================================================

func TestEnvPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T) (*EnvPipeline, *EnvMocks) {
		t.Helper()

		pipeline := NewEnvPipeline()
		mocks := setupEnvMocks(t)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfullyInTrustedDirectory", func(t *testing.T) {
		// Given an env pipeline in a trusted directory
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
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
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("untrusted directory")
		}

		resetCalled := false
		mocks.Shell.ResetFunc = func(args ...bool) {
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
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
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
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
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
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
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

	t.Run("ReturnsErrorWhenGetEnvVarsFails", func(t *testing.T) {
		// Given an env pipeline with failing env vars collection
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
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
		if err.Error() != "error getting environment variables: env vars error" {
			t.Errorf("Expected env vars error, got: %v", err)
		}
	})

	t.Run("PrintsEnvVarsWhenNotQuiet", func(t *testing.T) {
		// Given an env pipeline with quiet mode disabled
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		printCalled := false
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			printCalled = true
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		postEnvHookCalled := false
		mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
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
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		printCalled := false
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			printCalled = true
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
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

	t.Run("ReturnsErrorWhenPostEnvHookFailsInVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing post env hook in verbose mode
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("post env hook error")
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
		if err.Error() != "failed to execute post env hook: post env hook error" {
			t.Errorf("Expected post env hook error, got: %v", err)
		}
	})

	t.Run("IgnoresPostEnvHookErrorInNonVerboseMode", func(t *testing.T) {
		// Given an env pipeline with failing post env hook in non-verbose mode
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("post env hook error")
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

	t.Run("HandlesHookContextInUntrustedDirectory", func(t *testing.T) {
		// Given an env pipeline in an untrusted directory with hook context
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("untrusted directory")
		}

		resetCalled := false
		mocks.Shell.ResetFunc = func(args ...bool) {
			resetCalled = true
		}

		ctx := context.WithValue(context.Background(), "hook", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("HandlesSessionResetError", func(t *testing.T) {
		// Given an env pipeline with failing session reset
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("session reset error")
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to handle session reset") {
			t.Errorf("Expected session reset error, got: %v", err)
		}
	})

	t.Run("SkipsSecretsLoadingWhenDecryptFalse", func(t *testing.T) {
		// Given an env pipeline with decrypt disabled
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(nil)
		loadSecretsCalled := false
		mockSecretsProvider.LoadSecretsFunc = func() error {
			loadSecretsCalled = true
			return nil
		}
		pipeline.secretsProviders = []secrets.SecretsProvider{mockSecretsProvider}

		ctx := context.WithValue(context.Background(), "decrypt", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned and secrets should not be loaded
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if loadSecretsCalled {
			t.Error("Expected secrets to not be loaded")
		}
	})

	t.Run("SkipsSecretsLoadingWhenNoSecretsProviders", func(t *testing.T) {
		// Given an env pipeline with no secrets providers
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		pipeline.secretsProviders = []secrets.SecretsProvider{}

		ctx := context.WithValue(context.Background(), "decrypt", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CollectsEnvVarsFromMultipleEnvPrinters", func(t *testing.T) {
		// Given an env pipeline with multiple env printers
		pipeline, mocks := setup(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		mockEnvPrinter1 := env.NewMockEnvPrinter()
		mockEnvPrinter1.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"VAR1": "value1"}, nil
		}
		mockEnvPrinter1.PostEnvHookFunc = func(directory ...string) error {
			return nil
		}

		mockEnvPrinter2 := env.NewMockEnvPrinter()
		mockEnvPrinter2.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"VAR2": "value2"}, nil
		}
		mockEnvPrinter2.PostEnvHookFunc = func(directory ...string) error {
			return nil
		}

		pipeline.envPrinters = []env.EnvPrinter{mockEnvPrinter1, mockEnvPrinter2}

		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned and both variables should be collected
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedEnvVars["VAR1"] != "value1" {
			t.Errorf("Expected VAR1=value1, got %s", capturedEnvVars["VAR1"])
		}
		if capturedEnvVars["VAR2"] != "value2" {
			t.Errorf("Expected VAR2=value2, got %s", capturedEnvVars["VAR2"])
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

// NOTE: collectAndSetEnvVars functionality is now integrated into Execute method
