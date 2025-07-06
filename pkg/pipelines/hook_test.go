package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type HookMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

type HookSetupOptions struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

func setupHookMocks(t *testing.T, opts ...*HookSetupOptions) *HookMocks {
	t.Helper()

	var options *HookSetupOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = &HookSetupOptions{}
	}

	var injector di.Injector
	if options.Injector != nil {
		injector = options.Injector
	} else {
		injector = di.NewMockInjector()
	}

	var configHandler *config.MockConfigHandler
	if options.ConfigHandler != nil {
		configHandler = options.ConfigHandler
	} else {
		configHandler = config.NewMockConfigHandler()
		configHandler.InitializeFunc = func() error { return nil }
		configHandler.GetContextFunc = func() string { return "test-context" }
		configHandler.GetConfigRootFunc = func() (string, error) { return t.TempDir(), nil }
	}

	var mockShell *shell.MockShell
	if options.Shell != nil {
		mockShell = options.Shell
	} else {
		mockShell = shell.NewMockShell()
		mockShell.InitializeFunc = func() error { return nil }
		mockShell.GetProjectRootFunc = func() (string, error) { return t.TempDir(), nil }
		mockShell.InstallHookFunc = func(shellName string) error { return nil }
	}

	shims := setupHookShims(t)

	return &HookMocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Shims:         shims,
	}
}

func setupHookShims(t *testing.T) *Shims {
	t.Helper()
	return &Shims{
		Stat: func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		Getenv: func(key string) string {
			return ""
		},
		Setenv: func(key, value string) error {
			return nil
		},
	}
}

func setupHookPipeline(t *testing.T, mocks *HookMocks) *HookPipeline {
	t.Helper()

	constructors := HookConstructors{
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

	return NewHookPipeline(constructors)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewHookPipeline(t *testing.T) {
	t.Run("CreatesWithDefaultConstructors", func(t *testing.T) {
		// Given no constructors provided
		// When creating a new hook pipeline
		pipeline := NewHookPipeline()

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
		constructors := HookConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
		}

		// When creating a new hook pipeline
		pipeline := NewHookPipeline(constructors)

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

func TestHookPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*HookSetupOptions) (*HookPipeline, *HookMocks) {
		t.Helper()
		mocks := setupHookMocks(t, opts...)
		pipeline := setupHookPipeline(t, mocks)
		return pipeline, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured pipeline
		pipeline, _ := setup(t)

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

		pipeline, mocks := setup(t, &HookSetupOptions{
			Injector:      di.NewMockInjector(),
			ConfigHandler: mockConfigHandler,
		})

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize config handler") {
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
		if !strings.Contains(err.Error(), "failed to initialize shell") {
			t.Errorf("Expected shell error, got: %v", err)
		}
	})

	t.Run("ReusesExistingComponentsFromDIContainer", func(t *testing.T) {
		// Given a DI container with pre-existing shell and config handler
		injector := di.NewMockInjector()

		existingShell := shell.NewMockShell()
		existingShell.InitializeFunc = func() error { return nil }
		injector.Register("shell", existingShell)

		existingConfigHandler := config.NewMockConfigHandler()
		existingConfigHandler.InitializeFunc = func() error { return nil }
		injector.Register("configHandler", existingConfigHandler)

		// And a pipeline with custom constructors that should NOT be called
		constructorsCalled := false
		constructors := HookConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				constructorsCalled = true
				return config.NewMockConfigHandler()
			},
			NewShell: func(di.Injector) shell.Shell {
				constructorsCalled = true
				return shell.NewMockShell()
			},
			NewShims: func() *Shims {
				return setupHookShims(t)
			},
		}

		pipeline := NewHookPipeline(constructors)

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

func TestHookPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*HookSetupOptions) (*HookPipeline, *HookMocks) {
		t.Helper()
		mocks := setupHookMocks(t, opts...)
		pipeline := setupHookPipeline(t, mocks)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}
		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized pipeline
		pipeline, mocks := setup(t)

		// And a shell that successfully installs hooks
		hookInstalled := false
		mocks.Shell.InstallHookFunc = func(shellName string) error {
			hookInstalled = true
			if shellName != "zsh" {
				t.Errorf("Expected shell name 'zsh', got '%s'", shellName)
			}
			return nil
		}

		// When executing the pipeline with a shell type
		ctx := context.WithValue(context.Background(), "shellType", "zsh")
		err := pipeline.Execute(ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the hook should be installed
		if !hookInstalled {
			t.Error("Expected hook to be installed")
		}
	})

	t.Run("ReturnsErrorWhenNoShellTypeProvided", func(t *testing.T) {
		// Given a properly initialized pipeline
		pipeline, _ := setup(t)

		// When executing the pipeline without a shell type
		ctx := context.Background()
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "No shell name provided") {
			t.Errorf("Expected shell name error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellTypeIsInvalidType", func(t *testing.T) {
		// Given a properly initialized pipeline
		pipeline, _ := setup(t)

		// When executing the pipeline with an invalid shell type
		ctx := context.WithValue(context.Background(), "shellType", 123)
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Invalid shell name type") {
			t.Errorf("Expected shell name type error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInstallHookFails", func(t *testing.T) {
		// Given a properly initialized pipeline
		pipeline, mocks := setup(t)

		// And a shell that fails to install hooks
		mocks.Shell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("hook installation failed")
		}

		// When executing the pipeline with a shell type
		ctx := context.WithValue(context.Background(), "shellType", "bash")
		err := pipeline.Execute(ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "hook installation failed") {
			t.Errorf("Expected hook installation error, got: %v", err)
		}
	})

	t.Run("HandlesAllSupportedShellTypes", func(t *testing.T) {
		// Given a properly initialized pipeline
		pipeline, mocks := setup(t)

		// And a list of supported shell types
		supportedShells := []string{"zsh", "bash", "fish", "tcsh", "elvish", "powershell"}

		for _, shellType := range supportedShells {
			t.Run(fmt.Sprintf("Shell_%s", shellType), func(t *testing.T) {
				// And a shell that tracks the installed shell type
				var installedShellType string
				mocks.Shell.InstallHookFunc = func(shellName string) error {
					installedShellType = shellName
					return nil
				}

				// When executing the pipeline with the shell type
				ctx := context.WithValue(context.Background(), "shellType", shellType)
				err := pipeline.Execute(ctx)

				// Then no error should be returned
				if err != nil {
					t.Errorf("Expected no error for shell %s, got %v", shellType, err)
				}

				// And the correct shell type should be passed to InstallHook
				if installedShellType != shellType {
					t.Errorf("Expected shell type '%s', got '%s'", shellType, installedShellType)
				}
			})
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================
