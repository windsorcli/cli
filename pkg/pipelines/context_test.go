package pipelines

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ContextMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

type ContextSetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupContextShims(t *testing.T) *Shims {
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

func setupContextMocks(t *testing.T, opts ...*ContextSetupOptions) *ContextMocks {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	var options *ContextSetupOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	if options == nil {
		options = &ContextSetupOptions{}
	}

	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.InitializeFunc = func() error {
		return nil
	}
	mockShell.WriteResetTokenFunc = func() (string, error) {
		return "reset-token", nil
	}
	injector.Register("shell", mockShell)

	var configHandler *config.MockConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewMockConfigHandler()
	} else {
		if mockHandler, ok := options.ConfigHandler.(*config.MockConfigHandler); ok {
			configHandler = mockHandler
		} else {
			configHandler = config.NewMockConfigHandler()
		}
	}
	injector.Register("configHandler", configHandler)

	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}
	configHandler.Initialize()

	shims := setupContextShims(t)

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &ContextMocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Shims:         shims,
	}
}

func setupContextPipeline(t *testing.T, mocks *ContextMocks) *ContextPipeline {
	t.Helper()

	constructors := ContextConstructors{
		NewConfigHandler: func(di.Injector) config.ConfigHandler {
			return mocks.ConfigHandler
		},
		NewShell: func(di.Injector) shell.Shell {
			return mocks.Shell
		},
		NewShims: func() *Shims {
			return mocks.Shims
		},
	}

	return NewContextPipeline(constructors)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewContextPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		pipeline := NewContextPipeline()

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

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
		constructors := ContextConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
		}

		pipeline := NewContextPipeline(constructors)

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}

		if pipeline.constructors.NewConfigHandler == nil {
			t.Error("Expected NewConfigHandler constructor to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestContextPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*ContextSetupOptions) (*ContextPipeline, *ContextMocks) {
		t.Helper()
		mocks := setupContextMocks(t, opts...)
		pipeline := setupContextPipeline(t, mocks)
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		pipeline, _ := setup(t, &ContextSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    name: "Test Context"
`,
		})

		err := pipeline.Initialize(di.NewMockInjector(), context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerInitializeFails", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config initialization failed")
		}

		pipeline, mocks := setup(t, &ContextSetupOptions{
			Injector:      di.NewMockInjector(),
			ConfigHandler: mockConfigHandler,
		})

		err := pipeline.Initialize(mocks.Injector, context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contextContains(err.Error(), "failed to initialize config handler") {
			t.Errorf("Expected config handler error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellInitializeFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		err := pipeline.Initialize(mocks.Injector, context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contextContains(err.Error(), "failed to initialize shell") {
			t.Errorf("Expected shell error, got: %v", err)
		}
	})

	t.Run("ReusesExistingComponentsFromDIContainer", func(t *testing.T) {
		injector := di.NewMockInjector()

		existingShell := shell.NewMockShell()
		existingShell.InitializeFunc = func() error { return nil }
		existingShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		injector.Register("shell", existingShell)

		existingConfigHandler := config.NewMockConfigHandler()
		existingConfigHandler.InitializeFunc = func() error { return nil }
		existingConfigHandler.LoadConfigFunc = func(path string) error { return nil }
		injector.Register("configHandler", existingConfigHandler)

		constructorsCalled := false
		constructors := ContextConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				constructorsCalled = true
				return config.NewMockConfigHandler()
			},
			NewShell: func(di.Injector) shell.Shell {
				constructorsCalled = true
				return shell.NewMockShell()
			},
			NewShims: func() *Shims {
				return setupContextShims(t)
			},
		}

		pipeline := NewContextPipeline(constructors)

		err := pipeline.Initialize(injector, context.Background())

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if constructorsCalled {
			t.Error("Expected constructors not to be called when components exist in DI container")
		}

		if pipeline.shell != existingShell {
			t.Error("Expected pipeline to use existing shell from DI container")
		}
		if pipeline.configHandler != existingConfigHandler {
			t.Error("Expected pipeline to use existing config handler from DI container")
		}
	})
}

func TestContextPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*ContextSetupOptions) (*ContextPipeline, *ContextMocks) {
		t.Helper()
		mocks := setupContextMocks(t, opts...)
		pipeline := setupContextPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ReturnsErrorWhenNoOperationSpecified", func(t *testing.T) {
		pipeline, _ := setup(t)

		err := pipeline.Execute(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "no operation specified" {
			t.Errorf("Expected 'no operation specified', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInvalidOperationType", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", 123)
		err := pipeline.Execute(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "invalid operation type" {
			t.Errorf("Expected 'invalid operation type', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenUnsupportedOperation", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "operation", "unsupported")
		err := pipeline.Execute(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "unsupported operation: unsupported" {
			t.Errorf("Expected 'unsupported operation: unsupported', got: %v", err)
		}
	})

	t.Run("ExecutesGetOperationSuccessfully", func(t *testing.T) {
		pipeline, mocks := setup(t, &ContextSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    name: "Test Context"
`,
		})

		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.ConfigHandler.GetContextFunc = func() string { return "test-context" }

		var capturedOutput string
		outputFunc := func(output string) {
			capturedOutput = output
		}

		ctx := context.WithValue(context.Background(), "operation", "get")
		ctx = context.WithValue(ctx, "output", outputFunc)
		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedOutput != "test-context" {
			t.Errorf("Expected 'test-context', got: %v", capturedOutput)
		}
	})

	t.Run("ExecutesSetOperationSuccessfully", func(t *testing.T) {
		pipeline, mocks := setup(t, &ContextSetupOptions{
			ConfigStr: `
contexts:
  test-context:
    name: "Test Context"
  new-context:
    name: "New Context"
`,
		})

		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.ConfigHandler.SetContextFunc = func(context string) error { return nil }

		var capturedOutput string
		outputFunc := func(output string) {
			capturedOutput = output
		}

		ctx := context.WithValue(context.Background(), "operation", "set")
		ctx = context.WithValue(ctx, "contextName", "new-context")
		ctx = context.WithValue(ctx, "output", outputFunc)
		err := pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedOutput != "Context set to: new-context" {
			t.Errorf("Expected 'Context set to: new-context', got: %v", capturedOutput)
		}
	})
}

func TestContextPipeline_executeGet(t *testing.T) {
	setup := func(t *testing.T, opts ...*ContextSetupOptions) (*ContextPipeline, *ContextMocks) {
		t.Helper()
		mocks := setupContextMocks(t, opts...)
		pipeline := setupContextPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		err := pipeline.executeGet(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No context is available. Have you run `windsor init`?" {
			t.Errorf("Expected context not available error, got: %v", err)
		}
	})

	t.Run("ReturnsCurrentContextSuccessfully", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.ConfigHandler.GetContextFunc = func() string { return "current-context" }

		var capturedOutput string
		outputFunc := func(output string) {
			capturedOutput = output
		}

		ctx := context.WithValue(context.Background(), "output", outputFunc)
		err := pipeline.executeGet(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedOutput != "current-context" {
			t.Errorf("Expected 'current-context', got: %v", capturedOutput)
		}
	})
}

func TestContextPipeline_executeSet(t *testing.T) {
	setup := func(t *testing.T, opts ...*ContextSetupOptions) (*ContextPipeline, *ContextMocks) {
		t.Helper()
		mocks := setupContextMocks(t, opts...)
		pipeline := setupContextPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ReturnsErrorWhenNoContextNameProvided", func(t *testing.T) {
		pipeline, _ := setup(t)

		err := pipeline.executeSet(context.Background())

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "no context name provided" {
			t.Errorf("Expected 'no context name provided', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInvalidContextNameType", func(t *testing.T) {
		pipeline, _ := setup(t)

		ctx := context.WithValue(context.Background(), "contextName", 123)
		err := pipeline.executeSet(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "invalid context name type" {
			t.Errorf("Expected 'invalid context name type', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigNotLoaded", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		ctx := context.WithValue(context.Background(), "contextName", "test-context")
		err := pipeline.executeSet(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No context is available. Have you run `windsor init`?" {
			t.Errorf("Expected context not available error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWriteResetTokenFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("write reset token failed")
		}

		ctx := context.WithValue(context.Background(), "contextName", "test-context")
		err := pipeline.executeSet(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contextContains(err.Error(), "Error writing reset token") {
			t.Errorf("Expected reset token error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetContextFails", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.ConfigHandler.SetContextFunc = func(context string) error {
			return fmt.Errorf("set context failed")
		}

		ctx := context.WithValue(context.Background(), "contextName", "test-context")
		err := pipeline.executeSet(ctx)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !contextContains(err.Error(), "Error setting context") {
			t.Errorf("Expected set context error, got: %v", err)
		}
	})

	t.Run("SetsContextSuccessfully", func(t *testing.T) {
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return true }
		mocks.ConfigHandler.SetContextFunc = func(context string) error { return nil }

		var capturedOutput string
		outputFunc := func(output string) {
			capturedOutput = output
		}

		ctx := context.WithValue(context.Background(), "contextName", "new-context")
		ctx = context.WithValue(ctx, "output", outputFunc)
		err := pipeline.executeSet(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if capturedOutput != "Context set to: new-context" {
			t.Errorf("Expected 'Context set to: new-context', got: %v", capturedOutput)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func contextContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			contextContains(s[1:len(s)-1], substr))))
}
