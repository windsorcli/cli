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
		pipeline := NewInitPipeline()
		injector := di.NewInjector()

		err := pipeline.Initialize(injector)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfigHandlerInitializationError", func(t *testing.T) {
		mocks := NewInitMocks()
		mocks.ConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler init error")
		}

		// Don't call setupInitPipeline since we want to test the error
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

		err := pipeline.Initialize(injector)
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

		err := pipeline.Initialize(injector)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "failed to initialize shell: shell init error" {
			t.Errorf("Expected 'failed to initialize shell: shell init error', got %v", err)
		}
	})
}

func TestInitPipeline_Execute(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector)
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

		// Override the mock to inject error
		mocks.ConfigHandler.SetDefaultFunc = func(v1alpha1.Context) error {
			return fmt.Errorf("set default error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting default config: set default error" {
			t.Errorf("Expected set default error, got %v", err)
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
		if err.Error() != "Error processing context templates: process templates error" {
			t.Errorf("Expected process templates error, got %v", err)
		}
	})
}

func TestInitPipeline_ConfigureSettings(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector)
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

		// Override the mock to inject error
		mocks.ConfigHandler.SetDefaultFunc = func(v1alpha1.Context) error {
			return fmt.Errorf("set default error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting default config: set default error" {
			t.Errorf("Expected set default error, got %v", err)
		}
	})

	t.Run("SetContextValueError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "cluster.platform" {
				return fmt.Errorf("set context value error")
			}
			return nil
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting platform: set context value error" {
			t.Errorf("Expected set context value error, got %v", err)
		}
	})

	t.Run("SetBlueprintError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "blueprint" {
				return fmt.Errorf("set blueprint error")
			}
			return nil
		}

		flagValues := map[string]any{"blueprint": "test-blueprint"}
		changedFlags := map[string]bool{"blueprint": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting blueprint: set blueprint error" {
			t.Errorf("Expected set blueprint error, got %v", err)
		}
	})

	t.Run("SetTerraformError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "terraform.enabled" {
				return fmt.Errorf("set terraform error")
			}
			return nil
		}

		flagValues := map[string]any{"terraform": false}
		changedFlags := map[string]bool{"terraform": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting terraform: set terraform error" {
			t.Errorf("Expected set terraform error, got %v", err)
		}
	})

	t.Run("SetK8sError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "cluster.enabled" {
				return fmt.Errorf("set k8s error")
			}
			return nil
		}

		flagValues := map[string]any{"k8s": false}
		changedFlags := map[string]bool{"k8s": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting k8s: set k8s error" {
			t.Errorf("Expected set k8s error, got %v", err)
		}
	})

	t.Run("SetDockerComposeError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "docker.enabled" {
				return fmt.Errorf("set docker error")
			}
			return nil
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting docker-compose: set docker error" {
			t.Errorf("Expected set docker error, got %v", err)
		}
	})

	t.Run("SetAWSEnabledError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "aws.enabled" {
				return fmt.Errorf("set aws enabled error")
			}
			return nil
		}

		flagValues := map[string]any{"aws": true}
		changedFlags := map[string]bool{"aws": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting aws.enabled: set aws enabled error" {
			t.Errorf("Expected set aws enabled error, got %v", err)
		}
	})

	t.Run("SetAzureEnabledError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "azure.enabled" {
				return fmt.Errorf("set azure enabled error")
			}
			return nil
		}

		flagValues := map[string]any{"azure": true}
		changedFlags := map[string]bool{"azure": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting azure.enabled: set azure enabled error" {
			t.Errorf("Expected set azure enabled error, got %v", err)
		}
	})

	t.Run("SetClusterDriverError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "cluster.driver" {
				return fmt.Errorf("set cluster driver error")
			}
			return nil
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: set cluster driver error" {
			t.Errorf("Expected set cluster driver error, got %v", err)
		}
	})

	t.Run("SetFlagsInvalidFormat", func(t *testing.T) {
		_, pipeline, _ := setup()

		setFlags := []string{"invalid-format"}
		ctx := context.WithValue(context.Background(), "setFlags", setFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Invalid format for --set flag. Expected key=value" {
			t.Errorf("Expected invalid format error, got %v", err)
		}
	})

	t.Run("SetFlagsSetContextValueError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "test.key" {
				return fmt.Errorf("set flag error")
			}
			return nil
		}

		setFlags := []string{"test.key=value"}
		ctx := context.WithValue(context.Background(), "setFlags", setFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting config override test.key: set flag error" {
			t.Errorf("Expected set flag error, got %v", err)
		}
	})

	t.Run("TalosOverrideClusterDriver", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.ConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "cluster.driver" && value == "talos" {
				return fmt.Errorf("talos override error")
			}
			return nil
		}

		flagValues := map[string]any{"talos": true}
		changedFlags := map[string]bool{"talos": true}
		ctx := context.WithValue(context.Background(), "flagValues", flagValues)
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		err := pipeline.Execute(ctx)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error setting cluster.driver: talos override error" {
			t.Errorf("Expected talos override error, got %v", err)
		}
	})
}

func TestInitPipeline_DetermineAndSetContext(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector)
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

		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return mocks, pipeline, injector
	}

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error retrieving project root: project root error" {
			t.Errorf("Expected project root error, got %v", err)
		}
	})

	t.Run("LoadConfigError", func(t *testing.T) {
		mocks, pipeline, _ := setup()

		// Override the mock to inject error
		mocks.BlueprintHandler.LoadConfigFunc = func(...bool) error {
			return fmt.Errorf("load config error")
		}

		err := pipeline.Execute(context.Background())
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "Error reloading blueprint config: load config error" {
			t.Errorf("Expected load config error, got %v", err)
		}
	})
}

func TestInitPipeline_OutputSuccess(t *testing.T) {
	setup := func() (*InitMocks, *InitPipeline, di.Injector) {
		mocks := NewInitMocks()
		pipeline := setupInitPipeline(t, mocks)
		injector := di.NewInjector()

		err := pipeline.Initialize(injector)
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
