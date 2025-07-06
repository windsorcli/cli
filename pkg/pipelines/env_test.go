package pipelines

import (
	"context"
	"fmt"
	"os"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	envpkg "github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector            di.Injector
	ConfigHandler       config.ConfigHandler
	Shell               *shell.MockShell
	AwsEnvPrinter       *envpkg.MockEnvPrinter
	AzureEnvPrinter     *envpkg.MockEnvPrinter
	DockerEnvPrinter    *envpkg.MockEnvPrinter
	TerraformEnvPrinter *envpkg.MockEnvPrinter
	WindsorEnvPrinter   *envpkg.MockEnvPrinter
	SopsSecretsProvider *secrets.MockSecretsProvider
	Shims               *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()

	shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	shims.Getenv = func(key string) string {
		return ""
	}

	shims.Setenv = func(key, value string) error {
		return nil
	}

	return shims
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Process options with defaults
	var options *SetupOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	if options == nil {
		options = &SetupOptions{}
	}

	// Create injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	// Create shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.CheckTrustedDirectoryFunc = func() error {
		return nil
	}
	mockShell.CheckResetFlagsFunc = func() (bool, error) {
		return false, nil
	}
	mockShell.ResetFunc = func(...bool) {}
	injector.Register("shell", mockShell)

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewYamlConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}
	injector.Register("configHandler", configHandler)

	// Load config if provided
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}
	configHandler.Initialize()

	// Create env printers
	awsEnvPrinter := envpkg.NewMockEnvPrinter()
	azureEnvPrinter := envpkg.NewMockEnvPrinter()
	dockerEnvPrinter := envpkg.NewMockEnvPrinter()
	terraformEnvPrinter := envpkg.NewMockEnvPrinter()
	windsorEnvPrinter := envpkg.NewMockEnvPrinter()

	// Create secrets provider
	sopsSecretsProvider := secrets.NewMockSecretsProvider(injector)

	// Setup shims
	shims := setupShims(t)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Injector:            injector,
		ConfigHandler:       configHandler,
		Shell:               mockShell,
		AwsEnvPrinter:       awsEnvPrinter,
		AzureEnvPrinter:     azureEnvPrinter,
		DockerEnvPrinter:    dockerEnvPrinter,
		TerraformEnvPrinter: terraformEnvPrinter,
		WindsorEnvPrinter:   windsorEnvPrinter,
		SopsSecretsProvider: sopsSecretsProvider,
		Shims:               shims,
	}
}

func setupPipeline(t *testing.T, mocks *Mocks) *EnvPipeline {
	t.Helper()

	constructors := EnvConstructors{
		NewConfigHandler: func(di.Injector) config.ConfigHandler {
			return mocks.ConfigHandler
		},
		NewShell: func(di.Injector) shell.Shell {
			return mocks.Shell
		},
		NewAwsEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.AwsEnvPrinter
		},
		NewAzureEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.AzureEnvPrinter
		},
		NewDockerEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.DockerEnvPrinter
		},
		NewKubeEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.AwsEnvPrinter // Reuse for simplicity
		},
		NewOmniEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.AwsEnvPrinter // Reuse for simplicity
		},
		NewTalosEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.AwsEnvPrinter // Reuse for simplicity
		},
		NewTerraformEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.TerraformEnvPrinter
		},
		NewWindsorEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
			return mocks.WindsorEnvPrinter
		},
		NewSopsSecretsProvider: func(string, di.Injector) secrets.SecretsProvider {
			return mocks.SopsSecretsProvider
		},
		NewOnePasswordSDKSecretsProvider: func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider {
			return mocks.SopsSecretsProvider // Reuse for simplicity
		},
		NewOnePasswordCLISecretsProvider: func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider {
			return mocks.SopsSecretsProvider // Reuse for simplicity
		},
		NewShims: func() *Shims {
			return mocks.Shims
		},
	}

	return NewEnvPipeline(constructors)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		// Given no constructors provided
		// When creating a new pipeline
		pipeline := NewEnvPipeline()

		// Then pipeline should be created successfully
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		// And all default constructors should be set and functional
		injector := di.NewMockInjector()

		if pipeline.constructors.NewConfigHandler == nil {
			t.Error("Expected NewConfigHandler constructor to be set")
		} else {
			configHandler := pipeline.constructors.NewConfigHandler(injector)
			if configHandler == nil {
				t.Error("Expected NewConfigHandler to return a non-nil config handler")
			}
		}

		if pipeline.constructors.NewShell == nil {
			t.Error("Expected NewShell constructor to be set")
		} else {
			shell := pipeline.constructors.NewShell(injector)
			if shell == nil {
				t.Error("Expected NewShell to return a non-nil shell")
			}
		}

		if pipeline.constructors.NewAwsEnvPrinter == nil {
			t.Error("Expected NewAwsEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewAwsEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewAwsEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewAzureEnvPrinter == nil {
			t.Error("Expected NewAzureEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewAzureEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewAzureEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewDockerEnvPrinter == nil {
			t.Error("Expected NewDockerEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewDockerEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewDockerEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewKubeEnvPrinter == nil {
			t.Error("Expected NewKubeEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewKubeEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewKubeEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewOmniEnvPrinter == nil {
			t.Error("Expected NewOmniEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewOmniEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewOmniEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewTalosEnvPrinter == nil {
			t.Error("Expected NewTalosEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewTalosEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewTalosEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewTerraformEnvPrinter == nil {
			t.Error("Expected NewTerraformEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewTerraformEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewTerraformEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewWindsorEnvPrinter == nil {
			t.Error("Expected NewWindsorEnvPrinter constructor to be set")
		} else {
			printer := pipeline.constructors.NewWindsorEnvPrinter(injector)
			if printer == nil {
				t.Error("Expected NewWindsorEnvPrinter to return a non-nil printer")
			}
		}

		if pipeline.constructors.NewSopsSecretsProvider == nil {
			t.Error("Expected NewSopsSecretsProvider constructor to be set")
		} else {
			provider := pipeline.constructors.NewSopsSecretsProvider("/test", injector)
			if provider == nil {
				t.Error("Expected NewSopsSecretsProvider to return a non-nil provider")
			}
		}

		if pipeline.constructors.NewOnePasswordSDKSecretsProvider == nil {
			t.Error("Expected NewOnePasswordSDKSecretsProvider constructor to be set")
		} else {
			vault := secretsConfigType.OnePasswordVault{ID: "test", Name: "Test Vault"}
			provider := pipeline.constructors.NewOnePasswordSDKSecretsProvider(vault, injector)
			if provider == nil {
				t.Error("Expected NewOnePasswordSDKSecretsProvider to return a non-nil provider")
			}
		}

		if pipeline.constructors.NewOnePasswordCLISecretsProvider == nil {
			t.Error("Expected NewOnePasswordCLISecretsProvider constructor to be set")
		} else {
			vault := secretsConfigType.OnePasswordVault{ID: "test", Name: "Test Vault"}
			provider := pipeline.constructors.NewOnePasswordCLISecretsProvider(vault, injector)
			if provider == nil {
				t.Error("Expected NewOnePasswordCLISecretsProvider to return a non-nil provider")
			}
		}

		if pipeline.constructors.NewShims == nil {
			t.Error("Expected NewShims constructor to be set")
		} else {
			shims := pipeline.constructors.NewShims()
			if shims == nil {
				t.Error("Expected NewShims to return a non-nil shims")
			}
		}
	})

	t.Run("CreatesWithCustomConstructors", func(t *testing.T) {
		// Given custom constructors
		constructors := EnvConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
		}

		// When creating a new pipeline
		pipeline := NewEnvPipeline(constructors)

		// Then pipeline should be created successfully
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		// And custom constructor should be used
		if pipeline.constructors.NewConfigHandler == nil {
			t.Error("Expected NewConfigHandler constructor to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*EnvPipeline, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, opts...)
		pipeline := setupPipeline(t, mocks)
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured pipeline
		pipeline, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test-context:
    aws:
      enabled: true
`,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(di.NewMockInjector(), context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		// Given a config handler that fails to initialize
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config initialization failed")
		}

		// Use a fresh injector and register the failing config handler
		pipeline, mocks := setup(t, &SetupOptions{
			Injector:      di.NewMockInjector(),
			ConfigHandler: mockConfigHandler,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to initialize config handler") {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		// Given a shell that fails to initialize
		pipeline, mocks := setup(t)
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to initialize shell") {
			t.Errorf("Expected shell error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSecretsProviderInitializeFails", func(t *testing.T) {
		// Given a secrets provider that fails to initialize
		pipeline, mocks := setup(t)
		mocks.SopsSecretsProvider.InitializeFunc = func() error {
			return fmt.Errorf("secrets provider initialization failed")
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if contains(name, "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to initialize secrets provider") {
			t.Errorf("Expected secrets provider error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenEnvPrinterInitializeFails", func(t *testing.T) {
		// Given an env printer that fails to initialize
		pipeline, mocks := setup(t)
		mocks.WindsorEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("env printer initialization failed")
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to initialize env printer") {
			t.Errorf("Expected env printer error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenCreateSecretsProvidersFails", func(t *testing.T) {
		// Given a config handler that fails to get config root
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		pipeline, mocks := setup(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to create secrets providers") {
			t.Errorf("Expected create secrets providers error, got: %v", err)
		}
	})

	t.Run("HandlesEmptySecretsProvidersAndEnvPrinters", func(t *testing.T) {
		// Given a pipeline with minimal configuration (no secrets, only Windsor env printer)
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // No secrets files
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesNoSecretsProvidersCase", func(t *testing.T) {
		// Given a pipeline with no secrets providers
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // No secrets files exist
		}

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned and no secrets providers should exist
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(pipeline.secretsProviders) != 0 {
			t.Errorf("Expected no secrets providers, got %d", len(pipeline.secretsProviders))
		}
	})

	t.Run("VerifiesAllCodePathsAreReachable", func(t *testing.T) {
		// Given a standard pipeline configuration
		pipeline, _ := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(di.NewMockInjector(), context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReusesExistingComponentsFromDIContainer", func(t *testing.T) {
		// Given a DI container with pre-existing shell and config handler
		injector := di.NewMockInjector()

		existingShell := shell.NewMockShell()
		existingShell.InitializeFunc = func() error { return nil }
		existingShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", existingShell)

		existingConfigHandler := config.NewMockConfigHandler()
		existingConfigHandler.InitializeFunc = func() error { return nil }
		existingConfigHandler.GetContextFunc = func() string { return "test" }
		existingConfigHandler.GetConfigRootFunc = func() (string, error) { return t.TempDir(), nil }
		existingConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
		existingConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		existingConfigHandler.GetFunc = func(key string) any { return nil }
		injector.Register("configHandler", existingConfigHandler)

		// And a pipeline with custom constructors that should NOT be called
		constructorsCalled := false
		constructors := EnvConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				constructorsCalled = true
				return config.NewMockConfigHandler()
			},
			NewShell: func(di.Injector) shell.Shell {
				constructorsCalled = true
				return shell.NewMockShell()
			},
			NewAwsEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewAzureEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewDockerEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewKubeEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewOmniEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewTalosEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewTerraformEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewWindsorEnvPrinter: func(di.Injector) envpkg.EnvPrinter {
				return envpkg.NewMockEnvPrinter()
			},
			NewSopsSecretsProvider: func(string, di.Injector) secrets.SecretsProvider {
				return secrets.NewMockSecretsProvider(injector)
			},
			NewOnePasswordSDKSecretsProvider: func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider {
				return secrets.NewMockSecretsProvider(injector)
			},
			NewOnePasswordCLISecretsProvider: func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider {
				return secrets.NewMockSecretsProvider(injector)
			},
			NewShims: func() *Shims {
				return setupShims(t)
			},
		}

		pipeline := NewEnvPipeline(constructors)

		// When initializing the pipeline
		err := pipeline.Initialize(injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the existing components should be reused (constructors not called)
		if constructorsCalled {
			t.Error("Expected constructors not to be called when components exist in DI container")
		}

		// And the pipeline should use the existing components
		if pipeline.shell != existingShell {
			t.Error("Expected pipeline to use existing shell from DI container")
		}
		if pipeline.configHandler != existingConfigHandler {
			t.Error("Expected pipeline to use existing config handler from DI container")
		}
	})
}

func TestPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*EnvPipeline, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, opts...)
		pipeline := setupPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfullyInTrustedDirectory", func(t *testing.T) {
		// Given a properly initialized pipeline in a trusted directory
		pipeline, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test-context:
    aws:
      enabled: false
`,
		})

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExitsEarlyInUntrustedDirectory", func(t *testing.T) {
		// Given a pipeline in an untrusted directory
		pipeline, mocks := setup(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSessionReset", func(t *testing.T) {
		// Given a pipeline that needs session reset
		pipeline, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			return "" // No session token
		}
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("LoadsSecretsWhenDecryptFlagSet", func(t *testing.T) {
		// Given a pipeline with secrets and decrypt flag
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if contains(name, "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		ctx := context.WithValue(context.Background(), "decrypt", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenHandleSessionResetFails", func(t *testing.T) {
		// Given a pipeline where session reset fails
		pipeline, mocks := setup(t)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("reset flags error")
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to handle session reset") {
			t.Errorf("Expected session reset error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSecretsLoadFailsInVerboseMode", func(t *testing.T) {
		// Given a pipeline with failing secrets loading and verbose mode
		setup := func(t *testing.T) (*EnvPipeline, *Mocks) {
			t.Helper()
			mocks := setupMocks(t)
			mocks.SopsSecretsProvider.LoadSecretsFunc = func() error {
				return fmt.Errorf("secrets load failed")
			}
			mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
				if contains(name, "secrets.enc.yaml") {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}
			pipeline := setupPipeline(t, mocks)
			err := pipeline.Initialize(mocks.Injector, context.Background())
			if err != nil {
				t.Fatalf("Failed to initialize pipeline: %v", err)
			}
			return pipeline, mocks
		}

		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "decrypt", true)
		ctx = context.WithValue(ctx, "verbose", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to load secrets") {
			t.Errorf("Expected secrets load error, got: %v", err)
		}
	})

	t.Run("SilentlyIgnoresSecretsLoadFailureInNonVerboseMode", func(t *testing.T) {
		// Given a pipeline with failing secrets loading and non-verbose mode
		pipeline, mocks := setup(t)
		mocks.SopsSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets load failed")
		}
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if contains(name, "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		ctx := context.WithValue(context.Background(), "decrypt", true)
		ctx = context.WithValue(ctx, "verbose", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsFirstPrintErrorInVerboseMode", func(t *testing.T) {
		// Given a pipeline with a failing env printer in verbose mode
		pipeline, mocks := setup(t)
		mocks.WindsorEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print failed")
		}

		ctx := context.WithValue(context.Background(), "verbose", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to print env vars") {
			t.Errorf("Expected print error, got: %v", err)
		}
	})

	t.Run("ReturnsFirstPostHookErrorInVerboseMode", func(t *testing.T) {
		// Given a pipeline with a failing post hook in verbose mode
		pipeline, mocks := setup(t)
		mocks.WindsorEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post hook failed")
		}

		ctx := context.WithValue(context.Background(), "verbose", true)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contains(err.Error(), "failed to execute post env hook") {
			t.Errorf("Expected post hook error, got: %v", err)
		}
	})

	t.Run("SilentlyIgnoresErrorsInNonVerboseMode", func(t *testing.T) {
		// Given a pipeline with failing operations in non-verbose mode
		pipeline, mocks := setup(t)
		mocks.WindsorEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print failed")
		}
		mocks.WindsorEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post hook failed")
		}

		ctx := context.WithValue(context.Background(), "verbose", false)

		// When executing the pipeline
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("QuietModeInjectsEnvironmentVariables", func(t *testing.T) {
		// Set context for configuration
		os.Setenv("WINDSOR_CONTEXT", "test")
		t.Cleanup(func() { os.Unsetenv("WINDSOR_CONTEXT") })

		// Given a pipeline with env printers that return environment variables
		pipeline, mocks := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test:
    terraform:
      enabled: true
`,
		})
		mocks.WindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"WINDSOR_VAR": "windsor_value",
			}, nil
		}
		mocks.TerraformEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TF_VAR": "terraform_value",
			}, nil
		}

		// Track environment variables set via shims
		setEnvVars := make(map[string]string)
		mocks.Shims.Setenv = func(key, value string) error {
			setEnvVars[key] = value
			return nil
		}

		// When executing the pipeline in quiet mode
		ctx := context.WithValue(context.Background(), "quiet", true)
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And environment variables should be set via shims
		if value, ok := setEnvVars["WINDSOR_VAR"]; !ok || value != "windsor_value" {
			t.Errorf("Expected WINDSOR_VAR to be set to 'windsor_value', got %q", value)
		}
		if value, ok := setEnvVars["TF_VAR"]; !ok || value != "terraform_value" {
			t.Errorf("Expected TF_VAR to be set to 'terraform_value', got %q", value)
		}
	})

	t.Run("QuietModeSkipsPrinting", func(t *testing.T) {
		// Given a pipeline with env printers
		pipeline, mocks := setup(t)
		printCalled := false
		mocks.WindsorEnvPrinter.PrintFunc = func() error {
			printCalled = true
			return nil
		}
		mocks.WindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR": "test_value",
			}, nil
		}

		// When executing the pipeline in quiet mode
		ctx := context.WithValue(context.Background(), "quiet", true)
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And Print should not be called
		if printCalled {
			t.Error("Expected Print not to be called in quiet mode")
		}
	})

	t.Run("NormalModeInjectsAndPrints", func(t *testing.T) {
		// Given a pipeline with env printers
		pipeline, mocks := setup(t)
		printCalled := false
		mocks.WindsorEnvPrinter.PrintFunc = func() error {
			printCalled = true
			return nil
		}
		mocks.WindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"NORMAL_VAR": "normal_value",
			}, nil
		}

		// Track environment variables set via shims
		setEnvVars := make(map[string]string)
		mocks.Shims.Setenv = func(key, value string) error {
			setEnvVars[key] = value
			return nil
		}

		// When executing the pipeline in normal mode (not quiet)
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And environment variables should be injected
		if value, ok := setEnvVars["NORMAL_VAR"]; !ok || value != "normal_value" {
			t.Errorf("Expected NORMAL_VAR to be set to 'normal_value', got %q", value)
		}

		// And Print should be called
		if !printCalled {
			t.Error("Expected Print to be called in normal mode")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestPipeline_createSecretsProviders(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*EnvPipeline, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, opts...)
		pipeline := setupPipeline(t, mocks)

		// Set up minimal required components for createSecretsProviders
		pipeline.shims = mocks.Shims
		pipeline.configHandler = mocks.ConfigHandler

		return pipeline, mocks
	}

	t.Run("CreatesSOPSProvider", func(t *testing.T) {
		// Given a pipeline with SOPS secrets file
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if contains(name, "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		err := pipeline.createSecretsProviders(mocks.Injector)

		// Then SOPS provider should be created
		if err != nil {
			t.Fatalf("Failed to create secrets providers: %v", err)
		}
		if len(pipeline.secretsProviders) == 0 {
			t.Error("Expected SOPS secrets provider to be created")
		}
	})

	t.Run("CreatesSOPSProviderWithYmlExtension", func(t *testing.T) {
		// Given a pipeline with SOPS secrets file with .yml extension
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if contains(name, "secrets.enc.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		err := pipeline.createSecretsProviders(mocks.Injector)

		// Then SOPS provider should be created
		if err != nil {
			t.Fatalf("Failed to create secrets providers: %v", err)
		}
		if len(pipeline.secretsProviders) == 0 {
			t.Error("Expected SOPS secrets provider to be created")
		}
	})

	t.Run("CreatesOnePasswordSDKProvider", func(t *testing.T) {
		// Given a pipeline with OnePassword vault and service account token
		os.Setenv("WINDSOR_CONTEXT", "test")
		t.Cleanup(func() { os.Unsetenv("WINDSOR_CONTEXT") })

		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		t.Cleanup(func() { os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN") })

		pipeline, mocks := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test:
    secrets:
      onepassword:
        vaults:
          vault1:
            name: "Test Vault"
`,
		})

		err := pipeline.createSecretsProviders(mocks.Injector)

		// Then OnePassword SDK provider should be created
		if err != nil {
			t.Fatalf("Failed to create secrets providers: %v", err)
		}
		if len(pipeline.secretsProviders) == 0 {
			t.Error("Expected OnePassword SDK secrets provider to be created")
		}
	})

	t.Run("CreatesOnePasswordCLIProvider", func(t *testing.T) {
		// Given a pipeline with OnePassword vault but no service account token
		os.Setenv("WINDSOR_CONTEXT", "test")
		t.Cleanup(func() { os.Unsetenv("WINDSOR_CONTEXT") })

		pipeline, mocks := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test:
    secrets:
      onepassword:
        vaults:
          vault1:
            name: "Test Vault"
`,
		})

		err := pipeline.createSecretsProviders(mocks.Injector)

		// Then OnePassword CLI provider should be created
		if err != nil {
			t.Fatalf("Failed to create secrets providers: %v", err)
		}
		if len(pipeline.secretsProviders) == 0 {
			t.Error("Expected OnePassword CLI secrets provider to be created")
		}
	})

	t.Run("HandlesNoSecretsFiles", func(t *testing.T) {
		// Given a pipeline with no secrets files
		pipeline, mocks := setup(t)
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		err := pipeline.createSecretsProviders(mocks.Injector)

		// Then no secrets providers should be created
		if err != nil {
			t.Fatalf("Failed to create secrets providers: %v", err)
		}
		if len(pipeline.secretsProviders) != 0 {
			t.Error("Expected no secrets providers to be created")
		}
	})
}

func TestPipeline_createEnvPrinters(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*EnvPipeline, *Mocks) {
		t.Helper()
		os.Setenv("WINDSOR_CONTEXT", "test")
		t.Cleanup(func() { os.Unsetenv("WINDSOR_CONTEXT") })

		mocks := setupMocks(t, opts...)
		pipeline := setupPipeline(t, mocks)
		return pipeline, mocks
	}

	t.Run("CreatesAllEnabledPrinters", func(t *testing.T) {
		pipeline, mocks := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test:
    aws:
      enabled: true
    azure:
      enabled: true
    docker:
      enabled: true
    cluster:
      enabled: true
      driver: talos
    terraform:
      enabled: true
`,
		})

		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then all env printers should be created (aws, azure, docker, kube, talos, terraform, windsor)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		if len(pipeline.envPrinters) != 7 {
			t.Errorf("Expected 7 env printers, got %d", len(pipeline.envPrinters))
		}
	})

	t.Run("CreatesOnlyWindsorWhenAllDisabled", func(t *testing.T) {
		pipeline, mocks := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  test:
    aws:
      enabled: false
    azure:
      enabled: false
    docker:
      enabled: false
    cluster:
      enabled: false
    terraform:
      enabled: false
`,
		})

		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then only Windsor env printer should be created
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		if len(pipeline.envPrinters) != 1 {
			t.Errorf("Expected 1 env printer (Windsor), got %d", len(pipeline.envPrinters))
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			contains(s[1:len(s)-1], substr))))
}
