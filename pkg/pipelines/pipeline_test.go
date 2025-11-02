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
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/di"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/context/tools"
	"github.com/windsorcli/cli/pkg/provisioner/cluster"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	k8sclient "github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/workstation/virt"
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
		configHandler = config.NewConfigHandler(injector)
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

	// Create context directory and config file to ensure loaded flag is set
	contextDir := filepath.Join(tmpDir, "contexts", "mock-context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatalf("Failed to create context directory: %v", err)
	}
	contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
	contextConfigYAML := `
dns:
  domain: mock.domain.com
network:
  cidr_block: 10.0.0.0/24`
	if err := os.WriteFile(contextConfigPath, []byte(contextConfigYAML), 0644); err != nil {
		t.Fatalf("Failed to write context config: %v", err)
	}

	// Register shims
	shims := setupShims(t)
	injector.Register("shims", shims)

	// Create and register mock kubernetes manager for complex pipelines
	mockKubernetesManager := kubernetes.NewMockKubernetesManager(nil)
	injector.Register("kubernetesManager", mockKubernetesManager)

	// Create and register mock blueprint handler for stack dependency
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
	injector.Register("blueprintHandler", mockBlueprintHandler)

	// Create and register mock artifact builder for install pipeline dependency
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
	injector.Register("artifactBuilder", mockArtifactBuilder)

	// Create and register terraformEnv for stack dependency
	terraformEnv := envvars.NewTerraformEnvPrinter(injector)
	injector.Register("terraformEnv", terraformEnv)

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

	t.Run("SetsWindsorContextWhenContextNameProvided", func(t *testing.T) {
		// Given a base pipeline and mock config handler
		pipeline, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }

		contextSetCalled := false
		var capturedContextName string
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			contextSetCalled = true
			capturedContextName = contextName
			return nil
		}

		mocks.Injector.Register("configHandler", mockConfigHandler)

		// And a context with contextName specified
		ctx := context.WithValue(context.Background(), "contextName", "test-context")

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And SetContext should be called with the correct context name
		if !contextSetCalled {
			t.Error("Expected SetContext to be called")
		}
		if capturedContextName != "test-context" {
			t.Errorf("Expected SetContext to be called with 'test-context', got %s", capturedContextName)
		}
	})

	t.Run("DoesNotSetContextWhenContextNameIsEmpty", func(t *testing.T) {
		// Given a base pipeline and mock config handler
		pipeline, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }

		contextSetCalled := false
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			contextSetCalled = true
			return nil
		}

		mocks.Injector.Register("configHandler", mockConfigHandler)

		// And a context with empty contextName
		ctx := context.WithValue(context.Background(), "contextName", "")

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And SetContext should not be called
		if contextSetCalled {
			t.Error("Expected SetContext not to be called for empty context name")
		}
	})

	t.Run("DoesNotSetContextWhenContextNameNotProvided", func(t *testing.T) {
		// Given a base pipeline and mock config handler
		pipeline, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }

		contextSetCalled := false
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			contextSetCalled = true
			return nil
		}

		mocks.Injector.Register("configHandler", mockConfigHandler)

		// When initializing the pipeline without contextName in context
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And SetContext should not be called
		if contextSetCalled {
			t.Error("Expected SetContext not to be called when contextName not provided")
		}
	})

	t.Run("ReturnsErrorWhenSetContextFails", func(t *testing.T) {
		// Given a base pipeline and mock config handler that fails on SetContext
		pipeline, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			return fmt.Errorf("failed to set context")
		}

		mocks.Injector.Register("configHandler", mockConfigHandler)

		// And a context with contextName specified
		ctx := context.WithValue(context.Background(), "contextName", "test-context")

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedErr := "failed to set Windsor context: failed to set context"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("HandlesDifferentContextNameTypes", func(t *testing.T) {
		// Given a base pipeline and mock config handler
		pipeline, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }

		contextSetCalled := false
		mockConfigHandler.SetContextFunc = func(contextName string) error {
			contextSetCalled = true
			return nil
		}

		mocks.Injector.Register("configHandler", mockConfigHandler)

		// And a context with non-string contextName
		ctx := context.WithValue(context.Background(), "contextName", 123)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And SetContext should not be called for non-string values
		if contextSetCalled {
			t.Error("Expected SetContext not to be called for non-string contextName")
		}
	})

	t.Run("AddsTrustedDirectoryWhenTrustContextIsTrue", func(t *testing.T) {
		// Given a base pipeline and mocks
		pipeline, mocks := setup(t)

		trustFuncCalled := false
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			trustFuncCalled = true
			return nil
		}

		// And a context with trust set to true
		ctx := context.WithValue(context.Background(), "trust", true)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And AddCurrentDirToTrustedFile should be called
		if !trustFuncCalled {
			t.Error("Expected AddCurrentDirToTrustedFile to be called")
		}
	})

	t.Run("DoesNotAddTrustedDirectoryWhenTrustContextIsFalse", func(t *testing.T) {
		// Given a base pipeline and mocks
		pipeline, mocks := setup(t)

		trustFuncCalled := false
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			trustFuncCalled = true
			return nil
		}

		// And a context with trust set to false
		ctx := context.WithValue(context.Background(), "trust", false)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And AddCurrentDirToTrustedFile should not be called
		if trustFuncCalled {
			t.Error("Expected AddCurrentDirToTrustedFile not to be called when trust is false")
		}
	})

	t.Run("DoesNotAddTrustedDirectoryWhenTrustContextNotSet", func(t *testing.T) {
		// Given a base pipeline and mocks
		pipeline, mocks := setup(t)

		trustFuncCalled := false
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			trustFuncCalled = true
			return nil
		}

		// When initializing the pipeline without trust context
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And AddCurrentDirToTrustedFile should not be called
		if trustFuncCalled {
			t.Error("Expected AddCurrentDirToTrustedFile not to be called when trust context not set")
		}
	})

	t.Run("ReturnsErrorWhenAddCurrentDirToTrustedFileFails", func(t *testing.T) {
		// Given a base pipeline and mocks
		pipeline, mocks := setup(t)

		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("failed to add trusted directory")
		}

		// And a context with trust set to true
		ctx := context.WithValue(context.Background(), "trust", true)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, ctx)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		expectedErr := "failed to add current directory to trusted file: failed to add trusted directory"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
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
			{"InitPipeline", "initPipeline"},
			{"ExecPipeline", "execPipeline"},
			{"CheckPipeline", "checkPipeline"},
			{"UpPipeline", "upPipeline"},
			{"DownPipeline", "downPipeline"},
			{"InstallPipeline", "installPipeline"},
			{"BasePipeline", "basePipeline"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given an injector with proper mocks
				mocks := setupMocks(t)

				// When creating a pipeline with NewPipeline (without initialization)
				pipeline, err := WithPipeline(mocks.Injector, context.Background(), tc.pipelineType)

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
		// Given an injector with proper mocks
		mocks := setupMocks(t)

		// When creating a pipeline with unknown type
		pipeline, err := WithPipeline(mocks.Injector, context.Background(), "unknownPipeline")

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

	t.Run("WithPipelineInitialization", func(t *testing.T) {
		// Given an injector with proper mocks
		mocks := setupMocks(t)

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(mocks.Injector, context.Background(), "basePipeline")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And a pipeline should be returned
		if pipeline == nil {
			t.Error("Expected non-nil pipeline")
		}

		// And the pipeline should be registered in the injector
		registered := mocks.Injector.Resolve("basePipeline")
		if registered == nil {
			t.Error("Expected pipeline to be registered in injector")
		}
	})

	t.Run("ExistingPipelineInInjector", func(t *testing.T) {
		// Given an injector with existing pipeline
		injector := di.NewInjector()
		existingPipeline := NewMockBasePipeline()
		injector.Register("initPipeline", existingPipeline)
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "initPipeline")

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
		injector.Register("initPipeline", nonPipeline)
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "initPipeline")

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
		pipeline, err := WithPipeline(injector, ctx, "initPipeline")

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
		WithPipeline(nil, ctx, "initPipeline")

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
		WithPipeline(injector, nil, "initPipeline")

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
			"initPipeline",
			"execPipeline",
			"checkPipeline",
			"basePipeline",
		}

		for _, pipelineType := range supportedTypes {
			t.Run(pipelineType, func(t *testing.T) {
				// Given an injector and context
				injector := di.NewInjector()
				ctx := context.Background()

				// Set up required dependencies for check pipeline
				if pipelineType == "checkPipeline" {
					// Set up tools manager
					mockToolsManager := tools.NewMockToolsManager()
					mockToolsManager.InitializeFunc = func() error { return nil }
					mockToolsManager.CheckFunc = func() error { return nil }
					injector.Register("toolsManager", mockToolsManager)

					// Set up cluster client
					mockClusterClient := cluster.NewMockClusterClient()
					mockClusterClient.WaitForNodesHealthyFunc = func(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
						return nil
					}
					injector.Register("clusterClient", mockClusterClient)

					// Set up kubernetes manager
					mockKubernetesManager := kubernetes.NewMockKubernetesManager(injector)
					mockKubernetesManager.InitializeFunc = func() error { return nil }
					mockKubernetesManager.WaitForKubernetesHealthyFunc = func(ctx context.Context, endpoint string, outputFunc func(string), nodeNames ...string) error {
						return nil
					}
					injector.Register("kubernetesManager", mockKubernetesManager)
				}

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

		// Special test for upPipeline which requires more complex setup
		t.Run("upPipeline", func(t *testing.T) {
			// Given an injector with proper mocks for up pipeline
			mocks := setupMocks(t)

			// When creating the up pipeline
			pipeline, err := WithPipeline(mocks.Injector, context.Background(), "upPipeline")

			// Then no error should be returned
			if err != nil {
				t.Errorf("Expected no error for upPipeline, got: %v", err)
			}

			// And a pipeline should be returned
			if pipeline == nil {
				t.Error("Expected non-nil pipeline for upPipeline")
			}
		})
	})

	t.Run("PipelineRegistration", func(t *testing.T) {
		// Given an injector and context
		injector := di.NewInjector()
		ctx := context.Background()

		// When creating a pipeline with WithPipeline
		pipeline, err := WithPipeline(injector, ctx, "initPipeline")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the pipeline should be registered in the injector
		registered := injector.Resolve("initPipeline")
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
		pipeline1, err1 := WithPipeline(injector, ctx, "initPipeline")
		pipeline2, err2 := WithPipeline(injector, ctx, "initPipeline")

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

// =============================================================================
// Test Private Methods
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

	t.Run("CreatesTalosEnvPrinterWhenOmniProvider", func(t *testing.T) {
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

		// Then no error should be returned and Talos and Windsor env printers should be created
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// Should have Talos and Windsor
		if len(envPrinters) != 2 {
			t.Errorf("Expected 2 env printers, got %d", len(envPrinters))
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

	t.Run("RegistersTalosEnvPrinterInDIContainer", func(t *testing.T) {
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

		// And talos, terraform, and windsor env printers should be registered
		expectedRegistrations := []string{
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
		// Should have Talos and Windsor (terraform not included in slice when disabled)
		if len(envPrinters) != 2 {
			t.Errorf("Expected 2 env printers, got %d", len(envPrinters))
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
			if key == "secrets.onepassword.vaults" {
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
			if key == "secrets.onepassword.vaults" {
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
		existingStack := terraforminfra.NewWindsorStack(mocks.Injector)
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
		existingBuilder := artifact.NewArtifactBuilder()
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
		existingClient := k8sclient.NewDynamicKubernetesClient()
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

// =============================================================================
// Template Processing Tests
// =============================================================================

func TestBasePipeline_prepareTemplateData(t *testing.T) {
	t.Run("Priority1_ExplicitBlueprintOverridesLocalTemplates", func(t *testing.T) {
		// Given a pipeline with both explicit blueprint and local templates
		pipeline := NewBasePipeline()
		pipeline.injector = di.NewInjector()

		// Mock artifact builder that succeeds
		mockArtifactBuilder := artifact.NewMockArtifact()
		expectedOCIData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ explicit: 'oci-data' }"),
		}
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return expectedOCIData, nil
		}
		pipeline.artifactBuilder = mockArtifactBuilder

		// Mock blueprint handler with local templates
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"blueprint.jsonnet": []byte("{ local: 'template-data' }"),
			}, nil
		}
		pipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// Create context with explicit blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should use explicit blueprint, not local templates
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "{ explicit: 'oci-data' }" {
			t.Error("Expected explicit blueprint data to override local templates")
		}
	})

	t.Run("Priority1_ExplicitBlueprintFailsWithError", func(t *testing.T) {
		// Given a pipeline with explicit blueprint that fails
		pipeline := NewBasePipeline()
		pipeline.injector = di.NewInjector()

		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return nil, fmt.Errorf("OCI pull failed")
		}
		pipeline.artifactBuilder = mockArtifactBuilder

		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get template data from blueprint") {
			t.Errorf("Expected error to contain 'failed to get template data from blueprint', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("Priority2_LocalTemplatesWhenNoExplicitBlueprint", func(t *testing.T) {
		// Given a pipeline with local templates but no explicit blueprint
		pipeline := NewBasePipeline()
		injector := di.NewInjector()

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		expectedLocalData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ local: 'template-data' }"),
		}
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return expectedLocalData, nil
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Initialize the pipeline to set up all components
		if err := pipeline.Initialize(injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When prepareTemplateData is called with no blueprint context
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use local template data
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "{ local: 'template-data' }" {
			t.Error("Expected local template data")
		}
	})

	t.Run("Priority3_DefaultOCIURLWhenNoLocalTemplates", func(t *testing.T) {
		// Given a pipeline with no local templates and artifact builder
		pipeline := NewBasePipeline()
		pipeline.injector = di.NewInjector()

		// Mock artifact builder for default OCI URL
		mockArtifactBuilder := artifact.NewMockArtifact()
		expectedDefaultOCIData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ default: 'oci-data' }"),
		}
		var receivedOCIRef string
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			receivedOCIRef = ociRef
			return expectedDefaultOCIData, nil
		}
		pipeline.artifactBuilder = mockArtifactBuilder

		// Mock blueprint handler with no local templates
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil // Empty local templates
		}
		pipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// When prepareTemplateData is called with no blueprint context
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use default OCI URL
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "{ default: 'oci-data' }" {
			t.Error("Expected default OCI blueprint data")
		}
		// Verify the correct default OCI URL was used
		if !strings.Contains(receivedOCIRef, "ghcr.io/windsorcli/core") {
			t.Errorf("Expected default OCI URL to be used, got %s", receivedOCIRef)
		}
	})

	t.Run("Priority3_LocalTemplateDirectoryExistsUsesLocalEvenIfEmpty", func(t *testing.T) {
		// Given a pipeline with contexts/_template directory that exists but has no .jsonnet files
		pipeline := NewBasePipeline()
		injector := di.NewInjector()

		// Mock shell to return project root
		mockShell := shell.NewMockShell(nil)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}
		injector.Register("shell", mockShell)

		// Mock shims to simulate contexts/_template directory exists
		shims := &Shims{
			Stat: func(path string) (os.FileInfo, error) {
				if path == "/test/project/contexts/_template" {
					return &mockInitFileInfo{name: "_template", isDir: true}, nil
				}
				return nil, os.ErrNotExist
			},
		}
		injector.Register("shims", shims)

		// Mock blueprint handler with empty local templates (no .jsonnet files)
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			// Return empty map but with values.yaml data merged in
			return map[string][]byte{
				"values": []byte("external_domain: local.test"),
			}, nil
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Mock artifact builder (should NOT be called)
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			t.Error("Artifact builder should not be called when local template directory exists")
			return nil, fmt.Errorf("should not be called")
		}
		injector.Register("artifactBuilder", mockArtifactBuilder)

		// Initialize the pipeline to set up all components
		if err := pipeline.Initialize(injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When prepareTemplateData is called with no blueprint context
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use local template data even if it only contains values.yaml
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file (values), got %d", len(templateData))
		}
		if string(templateData["values"]) != "external_domain: local.test" {
			t.Error("Expected local values data")
		}
	})

	t.Run("Priority4_EmbeddedDefaultWhenNoArtifactBuilder", func(t *testing.T) {
		// Given a pipeline with no artifact builder
		pipeline := NewBasePipeline()
		injector := di.NewInjector()

		// Mock config handler (needed for determineContextName)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		injector.Register("configHandler", mockConfigHandler)

		// Mock blueprint handler with no local templates but default template
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil // Empty local templates
		}
		expectedDefaultData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ embedded: 'default-template' }"),
		}
		mockBlueprintHandler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return expectedDefaultData, nil
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// Initialize the pipeline to set up all components
		if err := pipeline.Initialize(injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Set artifact builder to nil to test the "no artifact builder" scenario
		pipeline.artifactBuilder = nil

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use embedded default template
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "{ embedded: 'default-template' }" {
			t.Error("Expected embedded default template data")
		}
	})

	t.Run("ReturnsEmptyMapWhenNothingAvailable", func(t *testing.T) {
		// Given a pipeline with no blueprint handler and no artifact builder
		pipeline := NewBasePipeline()
		pipeline.injector = di.NewInjector()

		// Set up config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		pipeline.configHandler = mockConfigHandler

		// Register a mock blueprint handler that returns empty data
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil
		}
		pipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should return empty map
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if templateData == nil {
			t.Error("Expected non-nil template data")
		}
		if len(templateData) != 0 {
			t.Error("Expected empty template data")
		}
	})

	t.Run("HandlesInvalidOCIReference", func(t *testing.T) {
		// Given a pipeline with invalid OCI reference
		pipeline := NewBasePipeline()
		pipeline.injector = di.NewInjector()

		mockArtifactBuilder := artifact.NewMockArtifact()
		pipeline.artifactBuilder = mockArtifactBuilder

		// Create context with invalid blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "invalid-oci-reference")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should return error for invalid reference
		if err == nil {
			t.Fatal("Expected error for invalid OCI reference, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse blueprint reference") {
			t.Errorf("Expected error to contain 'failed to parse blueprint reference', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})
}

func TestBasePipeline_determineContextName(t *testing.T) {
	t.Run("ReturnsContextNameFromContext", func(t *testing.T) {
		// Given a pipeline
		pipeline := NewBasePipeline()

		// And context with contextName
		ctx := context.WithValue(context.Background(), "contextName", "test-context")

		// When determineContextName is called
		result := pipeline.determineContextName(ctx)

		// Then should return context name from context
		if result != "test-context" {
			t.Errorf("Expected 'test-context', got %s", result)
		}
	})

	t.Run("ReturnsContextFromConfigHandler", func(t *testing.T) {
		// Given a pipeline with config handler
		pipeline := NewBasePipeline()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "config-context"
		}
		pipeline.configHandler = mockConfigHandler

		// When determineContextName is called
		result := pipeline.determineContextName(context.Background())

		// Then should return context from config handler
		if result != "config-context" {
			t.Errorf("Expected 'config-context', got %s", result)
		}
	})

	t.Run("ReturnsLocalWhenNoContextSet", func(t *testing.T) {
		// Given a pipeline with no context set
		pipeline := NewBasePipeline()

		// When determineContextName is called
		result := pipeline.determineContextName(context.Background())

		// Then should return "local"
		if result != "local" {
			t.Errorf("Expected 'local', got %s", result)
		}
	})

	t.Run("ReturnsLocalWhenContextIsLocal", func(t *testing.T) {
		// Given a pipeline with config handler returning "local"
		pipeline := NewBasePipeline()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		pipeline.configHandler = mockConfigHandler

		// When determineContextName is called
		result := pipeline.determineContextName(context.Background())

		// Then should return "local"
		if result != "local" {
			t.Errorf("Expected 'local', got %s", result)
		}
	})
}
