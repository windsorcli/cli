package pipelines

import (
	"context"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// InitMocks provides mock implementations for testing
type InitMocks struct {
	ConfigHandler    *config.MockConfigHandler
	Shell            *shell.MockShell
	BlueprintHandler *blueprint.MockBlueprintHandler
	Shims            *Shims
}

// NewInitMocks creates a new set of mocks for testing
func NewInitMocks() *InitMocks {
	return &InitMocks{
		ConfigHandler:    &config.MockConfigHandler{},
		Shell:            &shell.MockShell{},
		BlueprintHandler: &blueprint.MockBlueprintHandler{},
		Shims:            NewShims(),
	}
}

func setupInitPipeline(t *testing.T, mocks *InitMocks) *InitPipeline {
	t.Helper()

	// Set up mock behaviors
	mocks.Shell.InitializeFunc = func() error { return nil }
	mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	mocks.Shell.WriteResetTokenFunc = func() (string, error) { return "token", nil }
	mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/tmp", nil }

	mocks.ConfigHandler.InitializeFunc = func() error { return nil }
	mocks.ConfigHandler.SetContextFunc = func(string) error { return nil }
	mocks.ConfigHandler.GetContextFunc = func() string { return "local" }
	mocks.ConfigHandler.SetDefaultFunc = func(v1alpha1.Context) error { return nil }
	mocks.ConfigHandler.SetContextValueFunc = func(string, any) error { return nil }
	mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
	mocks.ConfigHandler.GenerateContextIDFunc = func() error { return nil }
	mocks.ConfigHandler.SaveConfigFunc = func(string, ...bool) error { return nil }

	mocks.BlueprintHandler.InitializeFunc = func() error { return nil }
	mocks.BlueprintHandler.ProcessContextTemplatesFunc = func(contextName string, reset ...bool) error { return nil }
	mocks.BlueprintHandler.LoadConfigFunc = func(...bool) error { return nil }

	constructors := InitConstructors{
		NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
			return mocks.ConfigHandler
		},
		NewShell: func(injector di.Injector) shell.Shell {
			return mocks.Shell
		},
		NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
			return mocks.BlueprintHandler
		},
		NewShims: func() *Shims {
			return mocks.Shims
		},
	}

	return NewInitPipeline(constructors)
}

func TestInitPipeline_NewInitPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		pipeline := NewInitPipeline()
		if pipeline == nil {
			t.Error("Expected pipeline to be created")
		}
	})

	t.Run("CreatesWithCustomConstructors", func(t *testing.T) {
		constructors := InitConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return &config.MockConfigHandler{}
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return &shell.MockShell{}
			},
			NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
				return &blueprint.MockBlueprintHandler{}
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}
		pipeline := NewInitPipeline(constructors)
		if pipeline == nil {
			t.Error("Expected pipeline to be created")
		}
	})
}

func TestInitPipeline_Initialize(t *testing.T) {
	t.Run("InitializesSuccessfully", func(t *testing.T) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfigHandlerInitializationError", func(t *testing.T) {
		mocks := NewInitMocks()
		mocks.ConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler init error")
		}
		// Set up other mocks to avoid nil pointer dereference
		mocks.Shell.InitializeFunc = func() error { return nil }
		mocks.BlueprintHandler.InitializeFunc = func() error { return nil }

		constructors := InitConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return mocks.ConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return mocks.Shell
			},
			NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
				return mocks.BlueprintHandler
			},
			NewShims: func() *Shims {
				return mocks.Shims
			},
		}
		pipeline := NewInitPipeline(constructors)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "failed to initialize config handler: config handler init error" {
			t.Errorf("Expected 'failed to initialize config handler: config handler init error', got %v", err)
		}
	})

	t.Run("ShellInitializationError", func(t *testing.T) {
		mocks := NewInitMocks()
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell init error")
		}
		// Set up other mocks to avoid nil pointer dereference
		mocks.ConfigHandler.InitializeFunc = func() error { return nil }
		mocks.BlueprintHandler.InitializeFunc = func() error { return nil }

		constructors := InitConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return mocks.ConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return mocks.Shell
			},
			NewBlueprintHandler: func(injector di.Injector) blueprint.BlueprintHandler {
				return mocks.BlueprintHandler
			},
			NewShims: func() *Shims {
				return mocks.Shims
			},
		}
		pipeline := NewInitPipeline(constructors)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "error initializing shell: shell init error" {
			t.Errorf("Expected 'error initializing shell: shell init error', got %v", err)
		}
	})
}

func TestInitPipeline_Execute(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return mocks, pipeline, injector
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		_, pipeline, _ := setup()

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesSuccessfullyWithContextArgument", func(t *testing.T) {
		_, pipeline, _ := setup()

		ctx := context.WithValue(context.Background(), "args", []string{"test-context"})
		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesWithFlags", func(t *testing.T) {
		_, pipeline, _ := setup()

		// Set up context with flags
		flagValues := map[string]any{
			"blueprint":      "test-blueprint",
			"terraform":      false,
			"k8s":            false,
			"colima":         true,
			"aws":            true,
			"azure":          false,
			"docker-compose": false,
			"talos":          true,
		}
		changedFlags := map[string]bool{
			"blueprint":      true,
			"terraform":      true,
			"k8s":            true,
			"colima":         true,
			"aws":            true,
			"azure":          false,
			"docker-compose": true,
			"talos":          true,
		}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesWithSetFlags", func(t *testing.T) {
		_, pipeline, _ := setup()

		setFlags := []string{"custom.key=value", "another.key=another-value"}
		ctx := context.WithValue(context.Background(), "setFlags", setFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetupTrustedEnvironmentError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("trusted file error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error adding current directory to trusted file: trusted file error" {
			t.Errorf("Expected trusted file error, got %v", err)
		}
	})

	t.Run("WriteResetTokenError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("reset token error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error writing reset token: reset token error" {
			t.Errorf("Expected reset token error, got %v", err)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextFunc = func(string) error {
			return fmt.Errorf("set context error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting context value: set context error" {
			t.Errorf("Expected set context error, got %v", err)
		}
	})

	t.Run("ConfigureSettingsError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error for platform setting
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "cluster.platform" {
				return fmt.Errorf("set platform error")
			}
			return nil
		}

		// Set up context with platform value to trigger the error
		ctx := context.WithValue(context.Background(), "platform", "test-platform")

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting platform: set platform error" {
			t.Errorf("Expected set platform error, got %v", err)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SaveConfigFunc = func(string, ...bool) error {
			return fmt.Errorf("save config error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error saving config file: save config error" {
			t.Errorf("Expected save config error, got %v", err)
		}
	})

	t.Run("GenerateContextIDError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.GenerateContextIDFunc = func() error {
			return fmt.Errorf("generate context id error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "failed to generate context ID: generate context id error" {
			t.Errorf("Expected generate context id error, got %v", err)
		}
	})

	t.Run("ProcessContextTemplatesError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.BlueprintHandler.ProcessContextTemplatesFunc = func(contextName string, reset ...bool) error {
			return fmt.Errorf("process templates error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "error processing blueprint templates: process templates error" {
			t.Errorf("Expected process templates error, got %v", err)
		}
	})
}

func TestInitPipeline_ConfigureSettings(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return mocks, pipeline, injector
	}

	t.Run("ConfiguresAWSPlatform", func(t *testing.T) {
		_, pipeline, _ := setup()

		flagValues := map[string]any{"aws": true}
		changedFlags := map[string]bool{"aws": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresAzurePlatform", func(t *testing.T) {
		_, pipeline, _ := setup()

		flagValues := map[string]any{"azure": true}
		changedFlags := map[string]bool{"azure": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresTalosPlatform", func(t *testing.T) {
		_, pipeline, _ := setup()

		flagValues := map[string]any{"talos": true}
		changedFlags := map[string]bool{"talos": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresColimaVMDriver", func(t *testing.T) {
		_, pipeline, _ := setup()

		flagValues := map[string]any{"colima": true}
		changedFlags := map[string]bool{"colima": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresDockerDesktopForDarwin", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the environment variable
		mocks.Shims.Getenv = func(key string) string {
			if key == "GOOS" {
				return "darwin"
			}
			return ""
		}

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresDockerForLinux", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the environment variable
		mocks.Shims.Getenv = func(key string) string {
			if key == "GOOS" {
				return "linux"
			}
			return ""
		}

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetDefaultConfigError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error for vm.driver setting
		mocks.ConfigHandler.SetFunc = func(key string, value any) error {
			if key == "vm.driver" {
				return fmt.Errorf("set vm driver error")
			}
			return nil
		}

		// Set up context with vmDriver value to trigger the error
		ctx := context.WithValue(context.Background(), "vmDriver", "test-driver")

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting VM driver: set vm driver error" {
			t.Errorf("Expected set vm driver error, got %v", err)
		}
	})
}

func TestInitPipeline_DetermineAndSetContext(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return mocks, pipeline, injector
	}

	t.Run("UsesContextFromArgs", func(t *testing.T) {
		_, pipeline, _ := setup()

		ctx := context.WithValue(context.Background(), "args", []string{"test-context"})
		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UsesCurrentContext", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "current-context"
		}

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DefaultsToLocal", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock
		mocks.ConfigHandler.GetContextFunc = func() string {
			return ""
		}

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitPipeline_SaveConfigAndProcessTemplates(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return mocks, pipeline, injector
	}

	t.Run("OutputsSuccessMessage", func(t *testing.T) {
		_, pipeline, _ := setup()

		err := pipeline.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("OutputsSuccessMessageWithVerbose", func(t *testing.T) {
		_, pipeline, _ := setup()

		ctx := context.WithValue(context.Background(), "verbose", true)
		err := pipeline.Execute(ctx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitPipeline_SetDefaultConfiguration(t *testing.T) {
	setup := func(t *testing.T) (*InitPipeline, *Mocks) {
		mocks := setupMocks(t)
		pipeline := NewInitPipeline()
		pipeline.configHandler = mocks.ConfigHandler
		pipeline.shims = mocks.Shims
		return pipeline, mocks
	}

	t.Run("LocalContextWithDockerDesktop", func(t *testing.T) {
		// Given an init pipeline with local context and docker-desktop driver
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")
		mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the localhost configuration should be applied
		// We can verify this by checking that the SetDefault method was called
		// The actual verification would require mocking the SetDefault method
	})

	t.Run("LocalPrefixContextWithDockerDesktop", func(t *testing.T) {
		// Given an init pipeline with local-prefix context and docker-desktop driver
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local-development")
		mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the localhost configuration should be applied
	})

	t.Run("ProductionContextWithDockerDesktop", func(t *testing.T) {
		// Given an init pipeline with production context and docker-desktop driver
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.SetContext("production")
		mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the full configuration should be applied (not localhost)
	})

	t.Run("StagingContextWithDockerDesktop", func(t *testing.T) {
		// Given an init pipeline with staging context and docker-desktop driver
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.SetContext("staging")
		mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the full configuration should be applied (not localhost)
	})

	t.Run("LocalContextWithColima", func(t *testing.T) {
		// Given an init pipeline with local context and colima driver
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.SetContext("local")
		mocks.ConfigHandler.SetContextValue("vm.driver", "colima")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the full configuration should be applied (not localhost)
	})
}
