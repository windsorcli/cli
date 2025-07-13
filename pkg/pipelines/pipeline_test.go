package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Centralized Mock Types
// =============================================================================

type mockInitFileInfo struct {
	name  string
	isDir bool
}

func (m *mockInitFileInfo) Name() string       { return m.name }
func (m *mockInitFileInfo) Size() int64        { return 0 }
func (m *mockInitFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockInitFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockInitFileInfo) IsDir() bool        { return m.isDir }
func (m *mockInitFileInfo) Sys() any           { return nil }

type mockInitDirEntry struct {
	name  string
	isDir bool
}

func (m *mockInitDirEntry) Name() string               { return m.name }
func (m *mockInitDirEntry) IsDir() bool                { return m.isDir }
func (m *mockInitDirEntry) Type() os.FileMode          { return 0 }
func (m *mockInitDirEntry) Info() (os.FileInfo, error) { return nil, nil }

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
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
		return nil, os.ErrNotExist // Default to file not found
	}

	shims.Getenv = func(key string) string {
		return "" // Default to empty string
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
	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create injector if not provided
	var injector di.Injector
	if len(opts) > 0 && opts[0].Injector != nil {
		injector = opts[0].Injector
	} else {
		injector = di.NewInjector()
	}

	// Create and register mock shell
	mockShell := shell.NewMockShell()
	mockShell.InitializeFunc = func() error { return nil }
	mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
	injector.Register("shell", mockShell)

	// Create config handler if not provided
	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		configHandler = config.NewYamlConfigHandler(injector)
	}
	injector.Register("configHandler", configHandler)

	// Initialize config handler
	if err := configHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
	}

	configHandler.SetContext("mock-context")

	// Load base config
	configYAML := `
apiVersion: v1alpha1
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
    network:
      cidr_block: 10.0.0.0/24`

	if err := configHandler.LoadConfigString(configYAML); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Load optional config if provided
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := configHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Register shims
	shims := setupShims(t)
	injector.Register("shims", shims)

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Shims:         shims,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBasePipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		pipeline := NewBasePipeline()

		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBasePipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("InitializeReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

func TestBasePipeline_Execute(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("ExecuteReturnsNilByDefault", func(t *testing.T) {
		// Given a base pipeline
		pipeline, _ := setup(t)

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

func TestWithPipeline(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		testCases := []struct {
			name         string
			pipelineType string
		}{
			{"EnvPipeline", "envPipeline"},
			{"InitPipeline", "initPipeline"},
			{"ExecPipeline", "execPipeline"},
			{"ContextPipeline", "contextPipeline"},
			{"HookPipeline", "hookPipeline"},
			{"CheckPipeline", "checkPipeline"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given an injector and context
				injector := di.NewInjector()
				ctx := context.Background()

				// When creating a pipeline with WithPipeline
				pipeline, err := WithPipeline(injector, ctx, tc.pipelineType)

				// Then no error should be returned
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}

				// And a pipeline should be returned
				if pipeline == nil {
					t.Error("Expected non-nil pipeline")
				}
			})
		}
	})

	t.Run("UnknownPipelineType", func(t *testing.T) {
		// Given an injector and context
		injector := di.NewInjector()
		ctx := context.Background()

		// When creating a pipeline with unknown type
		pipeline, err := WithPipeline(injector, ctx, "unknownPipeline")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unknown pipeline type, got nil")
		}
		if !strings.Contains(err.Error(), "unknown pipeline") {
			t.Errorf("Expected 'unknown pipeline' error, got: %v", err)
		}

		// And no pipeline should be returned
		if pipeline != nil {
			t.Error("Expected nil pipeline for unknown type")
		}
	})

	t.Run("ExistingPipelineInInjector", func(t *testing.T) {
		// Given an injector with existing pipeline
		injector := di.NewInjector()
		existingPipeline := NewMockBasePipeline()
		injector.Register("envPipeline", existingPipeline)
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "envPipeline")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the existing pipeline should be returned
		if pipeline != existingPipeline {
			t.Error("Expected existing pipeline to be returned")
		}
	})

	t.Run("ExistingNonPipelineInInjector", func(t *testing.T) {
		// Given an injector with non-pipeline object
		injector := di.NewInjector()
		nonPipeline := "not a pipeline"
		injector.Register("envPipeline", nonPipeline)
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "envPipeline")

		// Then no error should be returned (it creates a new pipeline)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And a new pipeline should be returned
		if pipeline == nil {
			t.Error("Expected non-nil pipeline")
		}
	})

	t.Run("ContextPropagation", func(t *testing.T) {
		// Given an injector and context with values
		injector := di.NewInjector()
		ctx := context.WithValue(context.Background(), "testKey", "testValue")

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "envPipeline")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And pipeline should be created successfully
		if pipeline == nil {
			t.Error("Expected non-nil pipeline")
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		// Given a nil injector
		ctx := context.Background()

		// When creating a pipeline with nil injector, it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil injector, but no panic occurred")
			}
		}()

		// This should panic due to nil pointer dereference
		WithPipeline(nil, ctx, "envPipeline")

		// If we reach here, the test should fail
		t.Error("Expected panic for nil injector, but function returned normally")
	})

	t.Run("NilContext", func(t *testing.T) {
		// Given an injector with nil context
		injector := di.NewInjector()

		// When creating a pipeline with nil context, it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil context, but no panic occurred")
			}
		}()

		// This should panic due to nil pointer dereference during initialization
		WithPipeline(injector, nil, "envPipeline")

		// If we reach here, the test should fail
		t.Error("Expected panic for nil context, but function returned normally")
	})

	t.Run("EmptyPipelineType", func(t *testing.T) {
		// Given an injector and context
		injector := di.NewInjector()
		ctx := context.Background()

		// When creating a pipeline with empty type
		pipeline, err := WithPipeline(injector, ctx, "")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for empty pipeline type, got nil")
		}
		if !strings.Contains(err.Error(), "unknown pipeline") {
			t.Errorf("Expected 'unknown pipeline' error, got: %v", err)
		}

		// And no pipeline should be returned
		if pipeline != nil {
			t.Error("Expected nil pipeline for empty type")
		}
	})

	t.Run("AllSupportedTypes", func(t *testing.T) {
		supportedTypes := []string{
			"envPipeline",
			"initPipeline",
			"execPipeline",
			"contextPipeline",
			"hookPipeline",
			"checkPipeline",
		}

		for _, pipelineType := range supportedTypes {
			t.Run(pipelineType, func(t *testing.T) {
				// Given an injector and context
				injector := di.NewInjector()
				ctx := context.Background()

				// When creating the pipeline
				pipeline, err := WithPipeline(injector, ctx, pipelineType)

				// Then no error should be returned
				if err != nil {
					t.Errorf("Expected no error for %s, got: %v", pipelineType, err)
				}

				// And a pipeline should be returned
				if pipeline == nil {
					t.Errorf("Expected non-nil pipeline for %s", pipelineType)
				}
			})
		}
	})

	t.Run("PipelineRegistration", func(t *testing.T) {
		// Given an injector and context
		injector := di.NewInjector()
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "envPipeline")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the pipeline should be registered in the injector
		registered := injector.Resolve("envPipeline")
		if registered == nil {
			t.Error("Expected pipeline to be registered in injector")
		}
		if registered != pipeline {
			t.Error("Expected registered pipeline to match returned pipeline")
		}
	})

	t.Run("FactoryFunctionality", func(t *testing.T) {
		// Given an injector and context
		injector := di.NewInjector()
		ctx := context.Background()

		// When creating multiple pipelines of the same type
		pipeline1, err1 := WithPipeline(injector, ctx, "envPipeline")
		pipeline2, err2 := WithPipeline(injector, ctx, "envPipeline")

		// Then both calls should succeed
		if err1 != nil {
			t.Errorf("Expected no error for first call, got: %v", err1)
		}
		if err2 != nil {
			t.Errorf("Expected no error for second call, got: %v", err2)
		}

		// And the same pipeline instance should be returned
		if pipeline1 != pipeline2 {
			t.Error("Expected same pipeline instance to be returned from factory")
		}
	})
}

// =============================================================================
// Test Protected Methods
// =============================================================================

func TestBasePipeline_handleSessionReset(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *shell.MockShell) {
		t.Helper()
		pipeline := NewBasePipeline()
		mockShell := shell.NewMockShell()
		pipeline.shell = mockShell

		// Clean up any existing environment variables
		t.Cleanup(func() {
			os.Unsetenv("WINDSOR_SESSION_TOKEN")
			os.Unsetenv("NO_CACHE")
		})

		return pipeline, mockShell
	}

	t.Run("ReturnsNilWhenShellIsNil", func(t *testing.T) {
		// Given a pipeline with nil shell
		pipeline := &BasePipeline{}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ResetsWhenNoSessionToken", func(t *testing.T) {
		// Given a pipeline with no session token
		pipeline, mockShell := setup(t)

		// Ensure no session token is set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("ResetsWhenResetFlagsTrue", func(t *testing.T) {
		// Given a pipeline with reset flags true
		pipeline, mockShell := setup(t)

		// Set a session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !resetCalled {
			t.Error("Expected shell reset to be called")
		}
	})

	t.Run("DoesNotResetWhenSessionTokenExistsAndResetFlagsFalse", func(t *testing.T) {
		// Given a pipeline with session token and reset flags false
		pipeline, mockShell := setup(t)

		// Set a session token
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		resetCalled := false
		mockShell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then reset should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if resetCalled {
			t.Error("Expected shell reset to not be called")
		}
	})

	t.Run("ReturnsErrorWhenCheckResetFlagsFails", func(t *testing.T) {
		// Given a pipeline where check reset flags fails
		pipeline, mockShell := setup(t)

		// Ensure no session token is set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("check reset flags error")
		}

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "check reset flags error" {
			t.Errorf("Expected check reset flags error, got: %v", err)
		}
	})

	t.Run("HandlesSessionResetWithNoSessionToken", func(t *testing.T) {
		// Given a base pipeline with no session token
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.ResetFunc = func(args ...bool) {
			// Reset called
		}
		pipeline.shell = mockShell

		// Ensure no session token is set
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Unsetenv("WINDSOR_SESSION_TOKEN")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			}
		}()

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSessionResetWithSessionToken", func(t *testing.T) {
		// Given a base pipeline with session token
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		pipeline.shell = mockShell

		// Set session token
		originalToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")
		defer func() {
			if originalToken != "" {
				os.Setenv("WINDSOR_SESSION_TOKEN", originalToken)
			} else {
				os.Unsetenv("WINDSOR_SESSION_TOKEN")
			}
		}()

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesSessionResetWithNilShell", func(t *testing.T) {
		// Given a base pipeline with nil shell
		pipeline := NewBasePipeline()
		pipeline.shell = nil

		// When handling session reset
		err := pipeline.handleSessionReset()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBasePipeline_loadConfig(t *testing.T) {
	t.Run("ReturnsErrorWhenShellIsNil", func(t *testing.T) {
		// Given a BasePipeline with nil shell
		pipeline := NewBasePipeline()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shell is nil")
		}
		if err.Error() != "shell not initialized" {
			t.Errorf("Expected 'shell not initialized' error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a BasePipeline with shell but nil config handler
		pipeline := NewBasePipeline()
		pipeline.shell = shell.NewMockShell()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when config handler is nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized' error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShimsIsNil", func(t *testing.T) {
		// Given a BasePipeline with shell and config handler but nil shims
		pipeline := NewBasePipeline()
		pipeline.shell = shell.NewMockShell()
		pipeline.configHandler = config.NewMockConfigHandler()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shims is nil")
		}
		if err.Error() != "shims not initialized" {
			t.Errorf("Expected 'shims not initialized' error, got %v", err)
		}
	})

	t.Run("LoadsConfigSuccessfully", func(t *testing.T) {
		// Given a BasePipeline with shell, config handler, and shims
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			expectedPath := filepath.Join(projectRoot, "windsor.yaml")
			if path != expectedPath {
				t.Errorf("Expected config path %q, got %q", expectedPath, path)
			}
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then no error should be returned and config should be loaded
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !loadConfigCalled {
			t.Error("Expected loadConfig to be called on config handler")
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a BasePipeline with failing shell
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected 'error retrieving project root' in error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given a BasePipeline with config handler that fails to load
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("load config error")
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// Create a test config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When loadConfig is called
		err := pipeline.loadConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when loadConfig fails")
		}
		if !strings.Contains(err.Error(), "error loading config file") {
			t.Errorf("Expected 'error loading config file' in error, got %v", err)
		}
	})

	t.Run("SkipsLoadingWhenNoConfigFileExists", func(t *testing.T) {
		// Given a BasePipeline with no config file
		pipeline := NewBasePipeline()

		mockShell := shell.NewMockShell()
		projectRoot := t.TempDir()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		pipeline.shell = mockShell

		mockConfigHandler := config.NewMockConfigHandler()
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.shims = NewShims()

		// When loadConfig is called (no config file exists)
		err := pipeline.loadConfig()

		// Then no error should be returned and loadConfig should not be called
		if err != nil {
			t.Errorf("Expected no error when no config file exists, got %v", err)
		}
		if loadConfigCalled {
			t.Error("Expected loadConfig not to be called when no config file exists")
		}
	})
}

// =============================================================================
// Test Private Methods - withEnvPrinters
// =============================================================================

func TestBasePipeline_withEnvPrinters(t *testing.T) {
	t.Run("CreatesWindsorEnvPrinterByDefault", func(t *testing.T) {
		// Given a base pipeline with minimal configuration
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and Windsor env printer should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(envPrinters) != 1 {
			t.Errorf("Expected 1 env printer, got %d", len(envPrinters))
		}
	})

	t.Run("CreatesMultipleEnvPrintersWhenEnabled", func(t *testing.T) {
		// Given a base pipeline with multiple services enabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "aws.enabled":
				return true
			case "azure.enabled":
				return true
			case "docker.enabled":
				return true
			case "cluster.enabled":
				return true
			case "terraform.enabled":
				return true
			default:
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.driver":
				return "talos"
			default:
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and multiple env printers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have AWS, Azure, Docker, Kube, Talos, Terraform, and Windsor
		if len(envPrinters) != 7 {
			t.Errorf("Expected 7 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("CreatesOmniAndTalosEnvPrintersWhenOmniProvider", func(t *testing.T) {
		// Given a base pipeline with omni cluster provider
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.driver":
				return "omni"
			default:
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned and Omni, Talos, and Windsor env printers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have Omni, Talos, and Windsor
		if len(envPrinters) != 3 {
			t.Errorf("Expected 3 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When creating env printers
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

	t.Run("RegistersAllEnvPrintersInDIContainer", func(t *testing.T) {
		// Given a base pipeline with all services enabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "aws.enabled":
				return true
			case "azure.enabled":
				return true
			case "docker.enabled":
				return true
			case "cluster.enabled":
				return true
			case "terraform.enabled":
				return true
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.driver":
				return "talos"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And all expected env printers should be registered in the DI container
		expectedRegistrations := []string{
			"awsEnv",
			"azureEnv",
			"dockerEnv",
			"kubeEnv",
			"talosEnv",
			"terraformEnv", // Always registered even if not in returned slice
			"windsorEnv",
		}

		for _, expectedKey := range expectedRegistrations {
			resolved := pipeline.injector.Resolve(expectedKey)
			if resolved == nil {
				t.Errorf("Expected %s to be registered in DI container, but it was not found", expectedKey)
			}
		}

		// And the correct number of env printers should be returned
		// Should have AWS, Azure, Docker, Kube, Talos, Terraform, and Windsor
		if len(envPrinters) != 7 {
			t.Errorf("Expected 7 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("RegistersOmniAndTalosEnvPrintersInDIContainer", func(t *testing.T) {
		// Given a base pipeline with omni cluster provider
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "cluster.driver":
				return "omni"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And omni, talos, terraform, and windsor env printers should be registered
		expectedRegistrations := []string{
			"omniEnv",
			"talosEnv",
			"terraformEnv", // Always registered
			"windsorEnv",
		}

		for _, expectedKey := range expectedRegistrations {
			resolved := pipeline.injector.Resolve(expectedKey)
			if resolved == nil {
				t.Errorf("Expected %s to be registered in DI container, but it was not found", expectedKey)
			}
		}

		// And the correct number of env printers should be returned
		// Should have Omni, Talos, and Windsor (terraform not included in slice when disabled)
		if len(envPrinters) != 3 {
			t.Errorf("Expected 3 env printers, got %d", len(envPrinters))
		}
	})

	t.Run("AlwaysRegistersTerraformEnvEvenWhenDisabled", func(t *testing.T) {
		// Given a base pipeline with terraform disabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "terraform.enabled":
				return false // Explicitly disabled
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating env printers
		envPrinters, err := pipeline.withEnvPrinters()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And terraform env should still be registered in DI container
		terraformEnv := pipeline.injector.Resolve("terraformEnv")
		if terraformEnv == nil {
			t.Error("Expected terraformEnv to be registered in DI container even when disabled")
		}

		// But terraform should not be included in the returned slice
		// Should only have Windsor
		if len(envPrinters) != 1 {
			t.Errorf("Expected 1 env printer, got %d", len(envPrinters))
		}
	})
}

// =============================================================================
// Test Private Methods - withSecretsProviders
// =============================================================================

func TestBasePipeline_withSecretsProviders(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks, string) {
		pipeline := NewBasePipeline()

		// Create temp directory for secrets files
		tmpDir := t.TempDir()

		// Create mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			return nil
		}

		// Create setup options with mock config handler
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		pipeline.configHandler = mockConfigHandler
		pipeline.shims = mocks.Shims
		pipeline.injector = mocks.Injector
		return pipeline, mocks, tmpDir
	}

	t.Run("ReturnsEmptyWhenNoSecretsConfigured", func(t *testing.T) {
		// Given a base pipeline with no secrets configuration
		pipeline, _, _ := setup(t)

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and no providers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 0 {
			t.Errorf("Expected 0 secrets providers, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesSopsProviderWhenSecretsFileExists", func(t *testing.T) {
		// Given a base pipeline with secrets file
		pipeline, mocks, tmpDir := setup(t)

		// Create secrets file
		secretsFile := filepath.Join(tmpDir, "secrets.enc.yaml")
		if err := os.WriteFile(secretsFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		// Configure shims to return file exists for secrets file
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == secretsFile {
				return os.Stat(secretsFile) // Return actual file info
			}
			return nil, os.ErrNotExist
		}

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and SOPS provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline, _, _ := setup(t)
		pipeline.configHandler = nil

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if secretsProviders != nil {
			t.Error("Expected nil secrets providers")
		}
	})

	t.Run("ReturnsErrorWhenGetConfigRootFails", func(t *testing.T) {
		// Given a base pipeline with failing config root
		pipeline, mocks, _ := setup(t)

		// Configure mock to fail on GetConfigRoot
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected 'error getting config root' in error, got: %v", err)
		}
		if secretsProviders != nil {
			t.Error("Expected nil secrets providers")
		}
	})

	t.Run("CreatesOnePasswordSDKProviderWhenServiceAccountTokenSet", func(t *testing.T) {
		// Given a base pipeline with OnePassword vaults and service account token
		pipeline, mocks, _ := setup(t)

		// Configure OnePassword vaults
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "contexts.test-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {
						Name: "test-vault",
					},
				}
			}
			return nil
		}

		// Configure service account token
		mocks.Shims.Getenv = func(key string) string {
			if key == "OP_SERVICE_ACCOUNT_TOKEN" {
				return "test-token"
			}
			return ""
		}

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and OnePassword SDK provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesOnePasswordCLIProviderWhenNoServiceAccountToken", func(t *testing.T) {
		// Given a base pipeline with OnePassword vaults and no service account token
		pipeline, mocks, _ := setup(t)

		// Configure OnePassword vaults
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "contexts.test-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {
						Name: "test-vault",
					},
				}
			}
			return nil
		}

		// Configure no service account token (already set to "" in setup)

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and OnePassword CLI provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})

	t.Run("CreatesSecretsProviderForSecretsDotEncDotYmlFile", func(t *testing.T) {
		// Given a base pipeline with secrets.enc.yml file
		pipeline, mocks, tmpDir := setup(t)

		// Create secrets.enc.yml file
		secretsFile := filepath.Join(tmpDir, "secrets.enc.yml")
		if err := os.WriteFile(secretsFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		// Configure shims to return file exists for secrets file
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == secretsFile {
				return os.Stat(secretsFile) // Return actual file info
			}
			return nil, os.ErrNotExist
		}

		// When creating secrets providers
		secretsProviders, err := pipeline.withSecretsProviders()

		// Then no error should be returned and SOPS provider should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(secretsProviders))
		}
	})
}

// =============================================================================
// Test Private Methods - withBlueprintHandler
// =============================================================================

func TestBasePipeline_withBlueprintHandler(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		pipeline.injector = mocks.Injector
		return pipeline, mocks
	}

	t.Run("CreatesNewBlueprintHandlerWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without blueprint handler
		pipeline, _ := setup(t)

		// When getting blueprint handler
		handler := pipeline.withBlueprintHandler()

		// Then a new handler should be created and registered
		if handler == nil {
			t.Error("Expected blueprint handler to be created")
		}

		registered := pipeline.injector.Resolve("blueprintHandler")
		if registered == nil {
			t.Error("Expected blueprint handler to be registered")
		}
	})

	t.Run("ReusesExistingBlueprintHandlerWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing blueprint handler
		pipeline, mocks := setup(t)
		existingHandler := blueprint.NewBlueprintHandler(mocks.Injector)
		pipeline.injector.Register("blueprintHandler", existingHandler)

		// When getting blueprint handler
		handler := pipeline.withBlueprintHandler()

		// Then the existing handler should be returned
		if handler != existingHandler {
			t.Error("Expected existing blueprint handler to be reused")
		}
	})

	t.Run("CreatesNewHandlerWhenRegisteredValueIsNotBlueprintHandler", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		pipeline.injector.Register("blueprintHandler", "not-a-handler")

		// When getting blueprint handler
		handler := pipeline.withBlueprintHandler()

		// Then a new handler should be created
		if handler == nil {
			t.Error("Expected blueprint handler to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withStack
// =============================================================================

func TestBasePipeline_withStack(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		pipeline.injector = mocks.Injector
		return pipeline, mocks
	}

	t.Run("CreatesNewStackWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without stack
		pipeline, _ := setup(t)

		// When getting stack
		stackInstance := pipeline.withStack()

		// Then a new stack should be created and registered
		if stackInstance == nil {
			t.Error("Expected stack to be created")
		}

		registered := pipeline.injector.Resolve("stack")
		if registered == nil {
			t.Error("Expected stack to be registered")
		}
	})

	t.Run("ReusesExistingStackWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing stack
		pipeline, mocks := setup(t)
		existingStack := stack.NewWindsorStack(mocks.Injector)
		pipeline.injector.Register("stack", existingStack)

		// When getting stack
		stackInstance := pipeline.withStack()

		// Then the existing stack should be returned
		if stackInstance != existingStack {
			t.Error("Expected existing stack to be reused")
		}
	})

	t.Run("CreatesNewStackWhenRegisteredValueIsNotStack", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		pipeline.injector.Register("stack", "not-a-stack")

		// When getting stack
		stackInstance := pipeline.withStack()

		// Then a new stack should be created
		if stackInstance == nil {
			t.Error("Expected stack to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withArtifactBuilder
// =============================================================================

func TestBasePipeline_withArtifactBuilder(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		pipeline.injector = mocks.Injector
		return pipeline, mocks
	}

	t.Run("CreatesNewArtifactBuilderWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without artifact builder
		pipeline, _ := setup(t)

		// When getting artifact builder
		builder := pipeline.withArtifactBuilder()

		// Then a new builder should be created and registered
		if builder == nil {
			t.Error("Expected artifact builder to be created")
		}

		registered := pipeline.injector.Resolve("artifactBuilder")
		if registered == nil {
			t.Error("Expected artifact builder to be registered")
		}
	})

	t.Run("ReusesExistingArtifactBuilderWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing artifact builder
		pipeline, _ := setup(t)
		existingBuilder := bundler.NewArtifactBuilder()
		pipeline.injector.Register("artifactBuilder", existingBuilder)

		// When getting artifact builder
		builder := pipeline.withArtifactBuilder()

		// Then the existing builder should be returned
		if builder != existingBuilder {
			t.Error("Expected existing artifact builder to be reused")
		}
	})

	t.Run("CreatesNewBuilderWhenRegisteredValueIsNotArtifactBuilder", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		pipeline.injector.Register("artifactBuilder", "not-a-builder")

		// When getting artifact builder
		builder := pipeline.withArtifactBuilder()

		// Then a new builder should be created
		if builder == nil {
			t.Error("Expected artifact builder to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withVirtualMachine
// =============================================================================

func TestBasePipeline_withVirtualMachine(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()

		// Create mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler
		return pipeline, mocks
	}

	t.Run("ReturnsNilWhenNoVMDriverConfigured", func(t *testing.T) {
		// Given a pipeline with no VM driver configured
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return ""
			}
			return ""
		}

		// When getting virtual machine
		vm := pipeline.withVirtualMachine()

		// Then nil should be returned
		if vm != nil {
			t.Error("Expected nil virtual machine when no driver configured")
		}
	})

	t.Run("CreatesColimaVMWhenColimaDriverConfigured", func(t *testing.T) {
		// Given a pipeline with colima driver configured
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		// When getting virtual machine
		vm := pipeline.withVirtualMachine()

		// Then colima VM should be created and registered
		if vm == nil {
			t.Error("Expected colima virtual machine to be created")
		}

		registered := pipeline.injector.Resolve("virtualMachine")
		if registered == nil {
			t.Error("Expected virtual machine to be registered")
		}
	})

	t.Run("ReturnsNilWhenUnsupportedVMDriverConfigured", func(t *testing.T) {
		// Given a pipeline with unsupported VM driver configured
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "unsupported"
			}
			return ""
		}

		// When getting virtual machine
		vm := pipeline.withVirtualMachine()

		// Then nil should be returned
		if vm != nil {
			t.Error("Expected nil virtual machine for unsupported driver")
		}
	})

	t.Run("ReusesExistingVMWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing VM
		pipeline, mocks := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		existingVM := virt.NewColimaVirt(mocks.Injector)
		pipeline.injector.Register("virtualMachine", existingVM)

		// When getting virtual machine
		vm := pipeline.withVirtualMachine()

		// Then the existing VM should be returned
		if vm != existingVM {
			t.Error("Expected existing virtual machine to be reused")
		}
	})

	t.Run("CreatesNewVMWhenRegisteredValueIsNotVirtualMachine", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		pipeline.injector.Register("virtualMachine", "not-a-vm")

		// When getting virtual machine
		vm := pipeline.withVirtualMachine()

		// Then a new VM should be created
		if vm == nil {
			t.Error("Expected virtual machine to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withContainerRuntime
// =============================================================================

func TestBasePipeline_withContainerRuntime(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()

		// Create mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		pipeline.injector = mocks.Injector
		pipeline.configHandler = mockConfigHandler
		return pipeline, mocks
	}

	t.Run("ReturnsNilWhenDockerDisabled", func(t *testing.T) {
		// Given a pipeline with docker disabled
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}

		// When getting container runtime
		runtime := pipeline.withContainerRuntime()

		// Then nil should be returned
		if runtime != nil {
			t.Error("Expected nil container runtime when docker disabled")
		}
	})

	t.Run("CreatesDockerRuntimeWhenDockerEnabled", func(t *testing.T) {
		// Given a pipeline with docker enabled
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		// When getting container runtime
		runtime := pipeline.withContainerRuntime()

		// Then docker runtime should be created and registered
		if runtime == nil {
			t.Error("Expected docker container runtime to be created")
		}

		registered := pipeline.injector.Resolve("containerRuntime")
		if registered == nil {
			t.Error("Expected container runtime to be registered")
		}
	})

	t.Run("ReusesExistingContainerRuntimeWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing container runtime
		pipeline, mocks := setup(t)
		existingRuntime := virt.NewDockerVirt(mocks.Injector)
		pipeline.injector.Register("containerRuntime", existingRuntime)

		// When getting container runtime
		runtime := pipeline.withContainerRuntime()

		// Then the existing runtime should be returned
		if runtime != existingRuntime {
			t.Error("Expected existing container runtime to be reused")
		}
	})

	t.Run("CreatesNewRuntimeWhenRegisteredValueIsNotContainerRuntime", func(t *testing.T) {
		// Given a pipeline with wrong type registered and docker enabled
		pipeline, _ := setup(t)
		mockConfigHandler := pipeline.configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}

		pipeline.injector.Register("containerRuntime", "not-a-runtime")

		// When getting container runtime
		runtime := pipeline.withContainerRuntime()

		// Then a new runtime should be created
		if runtime == nil {
			t.Error("Expected container runtime to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withKubernetesClient
// =============================================================================

func TestBasePipeline_withKubernetesClient(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		pipeline.injector = mocks.Injector
		return pipeline, mocks
	}

	t.Run("CreatesNewKubernetesClientWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without kubernetes client
		pipeline, _ := setup(t)

		// When getting kubernetes client
		client := pipeline.withKubernetesClient()

		// Then a new client should be created and registered
		if client == nil {
			t.Error("Expected kubernetes client to be created")
		}

		registered := pipeline.injector.Resolve("kubernetesClient")
		if registered == nil {
			t.Error("Expected kubernetes client to be registered")
		}
	})

	t.Run("ReusesExistingKubernetesClientWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing kubernetes client
		pipeline, _ := setup(t)
		existingClient := kubernetes.NewDynamicKubernetesClient()
		pipeline.injector.Register("kubernetesClient", existingClient)

		// When getting kubernetes client
		client := pipeline.withKubernetesClient()

		// Then the existing client should be returned
		if client != existingClient {
			t.Error("Expected existing kubernetes client to be reused")
		}
	})

	t.Run("CreatesNewClientWhenRegisteredValueIsNotKubernetesClient", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		pipeline.injector.Register("kubernetesClient", "not-a-client")

		// When getting kubernetes client
		client := pipeline.withKubernetesClient()

		// Then a new client should be created
		if client == nil {
			t.Error("Expected kubernetes client to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withKubernetesManager
// =============================================================================

func TestBasePipeline_withKubernetesManager(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		pipeline.injector = mocks.Injector
		return pipeline, mocks
	}

	t.Run("CreatesNewKubernetesManagerWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without kubernetes manager
		pipeline, _ := setup(t)

		// When getting kubernetes manager
		manager := pipeline.withKubernetesManager()

		// Then a new manager should be created and registered
		if manager == nil {
			t.Error("Expected kubernetes manager to be created")
		}

		registered := pipeline.injector.Resolve("kubernetesManager")
		if registered == nil {
			t.Error("Expected kubernetes manager to be registered")
		}
	})

	t.Run("ReusesExistingKubernetesManagerWhenRegistered", func(t *testing.T) {
		// Given a pipeline with existing kubernetes manager
		pipeline, mocks := setup(t)
		existingManager := kubernetes.NewKubernetesManager(mocks.Injector)
		pipeline.injector.Register("kubernetesManager", existingManager)

		// When getting kubernetes manager
		manager := pipeline.withKubernetesManager()

		// Then the existing manager should be returned
		if manager != existingManager {
			t.Error("Expected existing kubernetes manager to be reused")
		}
	})

	t.Run("CreatesNewManagerWhenRegisteredValueIsNotKubernetesManager", func(t *testing.T) {
		// Given a pipeline with wrong type registered
		pipeline, _ := setup(t)
		pipeline.injector.Register("kubernetesManager", "not-a-manager")

		// When getting kubernetes manager
		manager := pipeline.withKubernetesManager()

		// Then a new manager should be created
		if manager == nil {
			t.Error("Expected kubernetes manager to be created")
		}
	})
}

// =============================================================================
// Test Private Methods - withServices
// =============================================================================

func TestBasePipeline_withServices(t *testing.T) {
	t.Run("ReturnsEmptyWhenDockerDisabled", func(t *testing.T) {
		// Given a base pipeline with Docker disabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}
		pipeline.configHandler = mockConfigHandler

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and no services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(services) != 0 {
			t.Errorf("Expected 0 services, got %d", len(services))
		}
	})

	t.Run("CreatesMultipleServicesWhenDockerEnabled", func(t *testing.T) {
		// Given a base pipeline with Docker and multiple services enabled
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			switch key {
			case "cluster.controlplanes.count":
				return 2
			case "cluster.workers.count":
				return 3
			default:
				return 1
			}
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return true
			case "git.livereload.enabled":
				return true
			case "aws.localstack.enabled":
				return true
			default:
				return false
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and multiple services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have DNS, Git, AWS, 2 control plane, and 3 worker services
		if len(services) != 8 {
			t.Errorf("Expected 8 services, got %d", len(services))
		}
	})

	t.Run("CreatesRegistryServicesWhenDockerRegistriesConfigured", func(t *testing.T) {
		// Given a base pipeline with Docker registries configured
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry1": {},
						"registry2": {},
					},
				},
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and registry services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Errorf("Expected 2 services, got %d", len(services))
		}
	})

	t.Run("CreatesOmniClusterServices", func(t *testing.T) {
		// Given a base pipeline with Omni cluster provider
		pipeline := NewBasePipeline()

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "omni"
			}
			return ""
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			switch key {
			case "cluster.controlplanes.count":
				return 1
			case "cluster.workers.count":
				return 2
			default:
				return 1
			}
		}
		pipeline.configHandler = mockConfigHandler
		pipeline.injector = di.NewInjector()

		// When creating services
		services, err := pipeline.withServices()

		// Then no error should be returned and cluster services should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have 1 control plane and 2 worker services
		if len(services) != 3 {
			t.Errorf("Expected 3 services, got %d", len(services))
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a base pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When creating services
		services, err := pipeline.withServices()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "config handler not initialized" {
			t.Errorf("Expected 'config handler not initialized', got: %v", err)
		}
		if services != nil {
			t.Error("Expected nil services")
		}
	})
}

func TestBasePipeline_withTerraformResolvers(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("ReturnsEmptyWhenTerraformDisabled", func(t *testing.T) {
		// Given a pipeline with terraform disabled
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting terraform resolvers
		resolvers, err := pipeline.withTerraformResolvers()

		// Then no error should occur and resolvers should be empty
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(resolvers) != 0 {
			t.Errorf("Expected 0 resolvers, got %d", len(resolvers))
		}
	})

	t.Run("CreatesTerraformResolversWhenTerraformEnabled", func(t *testing.T) {
		// Given a pipeline with terraform enabled
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.LoadConfigString(`
apiVersion: v1alpha1
contexts:
  mock-context:
    terraform:
      enabled: true`)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting terraform resolvers
		resolvers, err := pipeline.withTerraformResolvers()

		// Then no error should occur and both resolvers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(resolvers) != 2 {
			t.Errorf("Expected 2 resolvers, got %d", len(resolvers))
		}

		// And resolvers should be registered in the DI container
		if mocks.Injector.Resolve("standardModuleResolver") == nil {
			t.Error("Expected standardModuleResolver to be registered")
		}
		if mocks.Injector.Resolve("ociModuleResolver") == nil {
			t.Error("Expected ociModuleResolver to be registered")
		}
	})

	t.Run("ReturnsErrorWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a pipeline with nil config handler
		pipeline := NewBasePipeline()
		pipeline.configHandler = nil

		// When getting terraform resolvers
		_, err := pipeline.withTerraformResolvers()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "config handler not initialized") {
			t.Errorf("Expected config handler error, got %v", err)
		}
	})
}

func TestBasePipeline_withTemplateRenderer(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("CreatesNewTemplateRendererWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without template renderer registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting template renderer
		renderer := pipeline.withTemplateRenderer()

		// Then a new template renderer should be created
		if renderer == nil {
			t.Error("Expected template renderer to not be nil")
		}

		// And it should be registered in the injector
		registered := mocks.Injector.Resolve("templateRenderer")
		if registered == nil {
			t.Error("Expected template renderer to be registered")
		}
	})

	t.Run("ReusesExistingTemplateRendererWhenRegistered", func(t *testing.T) {
		// Given a pipeline with template renderer already registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// And an existing template renderer
		existingRenderer := pipeline.withTemplateRenderer()

		// When getting template renderer again
		renderer := pipeline.withTemplateRenderer()

		// Then the same template renderer should be returned
		if renderer != existingRenderer {
			t.Error("Expected to reuse existing template renderer")
		}
	})
}

func TestBasePipeline_withGenerators(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("CreatesGenerators", func(t *testing.T) {
		// Given a pipeline
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting generators
		generators, err := pipeline.withGenerators()

		// Then no error should occur and generators should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(generators) == 0 {
			t.Error("Expected generators to be created")
		}

		// And git generator should be registered
		registered := mocks.Injector.Resolve("gitGenerator")
		if registered == nil {
			t.Error("Expected git generator to be registered")
		}
	})

	t.Run("CreatesTerraformGeneratorWhenTerraformEnabled", func(t *testing.T) {
		// Given a pipeline with terraform enabled
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.LoadConfigString(`
apiVersion: v1alpha1
contexts:
  mock-context:
    terraform:
      enabled: true`)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting generators
		generators, err := pipeline.withGenerators()

		// Then no error should occur and terraform generator should be included
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(generators) < 2 {
			t.Error("Expected at least 2 generators (git + terraform)")
		}

		// And terraform generator should be registered
		registered := mocks.Injector.Resolve("terraformGenerator")
		if registered == nil {
			t.Error("Expected terraform generator to be registered")
		}
	})
}

func TestBasePipeline_withBundlers(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("CreatesBundlers", func(t *testing.T) {
		// Given a pipeline
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting bundlers
		bundlers, err := pipeline.withBundlers()

		// Then no error should occur and bundlers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(bundlers) == 0 {
			t.Error("Expected bundlers to be created")
		}

		// And bundlers should be registered
		kustomizeBundler := mocks.Injector.Resolve("kustomizeBundler")
		if kustomizeBundler == nil {
			t.Error("Expected kustomize bundler to be registered")
		}
		templateBundler := mocks.Injector.Resolve("templateBundler")
		if templateBundler == nil {
			t.Error("Expected template bundler to be registered")
		}
	})

	t.Run("CreatesTerraformBundlerWhenTerraformEnabled", func(t *testing.T) {
		// Given a pipeline with terraform enabled
		pipeline, mocks := setup(t)
		mocks.ConfigHandler.LoadConfigString(`
apiVersion: v1alpha1
contexts:
  mock-context:
    terraform:
      enabled: true`)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting bundlers
		bundlers, err := pipeline.withBundlers()

		// Then no error should occur and terraform bundler should be included
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(bundlers) < 3 {
			t.Error("Expected at least 3 bundlers (kustomize + template + terraform)")
		}

		// And terraform bundler should be registered
		registered := mocks.Injector.Resolve("terraformBundler")
		if registered == nil {
			t.Error("Expected terraform bundler to be registered")
		}
	})
}

func TestBasePipeline_withToolsManager(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("CreatesNewToolsManagerWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without tools manager registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting tools manager
		toolsManager := pipeline.withToolsManager()

		// Then a new tools manager should be created
		if toolsManager == nil {
			t.Error("Expected tools manager to not be nil")
		}

		// And it should be registered in the injector
		registered := mocks.Injector.Resolve("toolsManager")
		if registered == nil {
			t.Error("Expected tools manager to be registered")
		}
	})

	t.Run("ReusesExistingToolsManagerWhenRegistered", func(t *testing.T) {
		// Given a pipeline with tools manager already registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// And an existing tools manager
		existingManager := pipeline.withToolsManager()

		// When getting tools manager again
		toolsManager := pipeline.withToolsManager()

		// Then the same tools manager should be returned
		if toolsManager != existingManager {
			t.Error("Expected to reuse existing tools manager")
		}
	})
}

func TestBasePipeline_withClusterClient(t *testing.T) {
	setup := func(t *testing.T) (*BasePipeline, *Mocks) {
		pipeline := NewBasePipeline()
		mocks := setupMocks(t)
		return pipeline, mocks
	}

	t.Run("CreatesNewClusterClientWhenNotRegistered", func(t *testing.T) {
		// Given a pipeline without cluster client registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// When getting cluster client
		clusterClient := pipeline.withClusterClient()

		// Then a new cluster client should be created
		if clusterClient == nil {
			t.Error("Expected cluster client to not be nil")
		}

		// And it should be registered in the injector
		registered := mocks.Injector.Resolve("clusterClient")
		if registered == nil {
			t.Error("Expected cluster client to be registered")
		}
	})

	t.Run("ReusesExistingClusterClientWhenRegistered", func(t *testing.T) {
		// Given a pipeline with cluster client already registered
		pipeline, mocks := setup(t)
		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// And an existing cluster client
		existingClient := pipeline.withClusterClient()

		// When getting cluster client again
		clusterClient := pipeline.withClusterClient()

		// Then the same cluster client should be returned
		if clusterClient != existingClient {
			t.Error("Expected to reuse existing cluster client")
		}
	})
}
