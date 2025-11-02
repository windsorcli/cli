package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/tools"
	"github.com/windsorcli/cli/pkg/infrastructure/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/infrastructure/terraform"
	"github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// patchMockFileInfo implements os.FileInfo for testing
type patchMockFileInfo struct {
	isDir bool
}

func (m *patchMockFileInfo) Name() string       { return "mock" }
func (m *patchMockFileInfo) Size() int64        { return 0 }
func (m *patchMockFileInfo) Mode() os.FileMode  { return 0 }
func (m *patchMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *patchMockFileInfo) IsDir() bool        { return m.isDir }
func (m *patchMockFileInfo) Sys() interface{}   { return nil }

type InitMocks struct {
	*Mocks
	BlueprintHandler  *blueprint.MockBlueprintHandler
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Stack             *terraforminfra.MockStack
	VirtualMachine    *virt.MockVirt
	ContainerRuntime  *virt.MockVirt
	ArtifactBuilder   *artifact.MockArtifact
}

func setupInitMocks(t *testing.T, opts ...*SetupOptions) *InitMocks {
	t.Helper()

	// Create setup options, preserving any provided options
	setupOptions := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		setupOptions = opts[0]
	}

	baseMocks := setupMocks(t, setupOptions)

	// Initialize the config handler if it's a real one
	if setupOptions.ConfigHandler == nil {
		configHandler := baseMocks.ConfigHandler
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
	}

	// Add init-specific shell mock behaviors
	baseMocks.Shell.WriteResetTokenFunc = func() (string, error) { return "mock-token", nil }
	baseMocks.Shell.AddCurrentDirToTrustedFileFunc = func() error { return nil }

	// Setup blueprint handler mock
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(baseMocks.Injector)
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.LoadConfigFunc = func() error { return nil }
	mockBlueprintHandler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
		return make(map[string][]byte), nil
	}
	baseMocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

	// Setup kubernetes manager mock
	mockKubernetesManager := kubernetes.NewMockKubernetesManager(nil)
	mockKubernetesManager.InitializeFunc = func() error { return nil }
	baseMocks.Injector.Register("kubernetesManager", mockKubernetesManager)

	// Setup tools manager mock
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockToolsManager.WriteManifestFunc = func() error { return nil }
	baseMocks.Injector.Register("toolsManager", mockToolsManager)

	// Setup stack mock
	mockStack := terraforminfra.NewMockStack(baseMocks.Injector)
	mockStack.InitializeFunc = func() error { return nil }
	baseMocks.Injector.Register("stack", mockStack)

	// Setup virtual machine mock
	mockVirtualMachine := virt.NewMockVirt()
	mockVirtualMachine.WriteConfigFunc = func() error { return nil }
	baseMocks.Injector.Register("virtualMachine", mockVirtualMachine)

	// Setup container runtime mock
	mockContainerRuntime := virt.NewMockVirt()
	mockContainerRuntime.WriteConfigFunc = func() error { return nil }
	baseMocks.Injector.Register("containerRuntime", mockContainerRuntime)

	// Setup artifact builder mock
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.InitializeFunc = func(injector di.Injector) error { return nil }
	baseMocks.Injector.Register("artifactBuilder", mockArtifactBuilder)

	return &InitMocks{
		Mocks:             baseMocks,
		BlueprintHandler:  mockBlueprintHandler,
		KubernetesManager: mockKubernetesManager,
		ToolsManager:      mockToolsManager,
		Stack:             mockStack,
		VirtualMachine:    mockVirtualMachine,
		ContainerRuntime:  mockContainerRuntime,
		ArtifactBuilder:   mockArtifactBuilder,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewInitPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new init pipeline
		pipeline := NewInitPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestInitPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given an init pipeline with mock dependencies
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	// Test initialization failures
	initFailureTests := []struct {
		name        string
		setupMock   func(*InitMocks)
		expectedErr string
	}{
		{
			name: "ReturnsErrorWhenShellInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mockShell := shell.NewMockShell()
				mockShell.InitializeFunc = func() error {
					return fmt.Errorf("shell initialization failed")
				}
				mocks.Injector.Register("shell", mockShell)
			},
			expectedErr: "failed to initialize shell: shell initialization failed",
		},
		{
			name: "ReturnsErrorWhenBlueprintHandlerInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mocks.BlueprintHandler.InitializeFunc = func() error {
					return fmt.Errorf("blueprint handler failed")
				}
			},
			expectedErr: "failed to initialize blueprint handler: blueprint handler failed",
		},
		{
			name: "ReturnsErrorWhenKubernetesManagerInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mocks.KubernetesManager.InitializeFunc = func() error {
					return fmt.Errorf("kubernetes manager failed")
				}
			},
			expectedErr: "failed to initialize kubernetes manager: kubernetes manager failed",
		},
		{
			name: "ReturnsErrorWhenToolsManagerInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mocks.ToolsManager.InitializeFunc = func() error {
					return fmt.Errorf("tools manager failed")
				}
			},
			expectedErr: "failed to initialize tools manager: tools manager failed",
		},
		{
			name: "ReturnsErrorWhenStackInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mocks.Stack.InitializeFunc = func() error {
					return fmt.Errorf("stack failed")
				}
			},
			expectedErr: "failed to initialize stack: stack failed",
		},
		{
			name: "ReturnsErrorWhenArtifactBuilderInitializeFails",
			setupMock: func(mocks *InitMocks) {
				mocks.ArtifactBuilder.InitializeFunc = func(injector di.Injector) error {
					return fmt.Errorf("artifact builder failed")
				}
			},
			expectedErr: "failed to initialize artifact builder: artifact builder failed",
		},
	}

	for _, tt := range initFailureTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an init pipeline with failing component
			pipeline, mocks := setup(t)
			tt.setupMock(mocks)

			// When initializing the pipeline
			err := pipeline.Initialize(mocks.Injector, context.Background())

			// Then an error should be returned
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if err.Error() != tt.expectedErr {
				t.Errorf("Expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}

	t.Run("InitializesSecureShellWhenRegistered", func(t *testing.T) {
		// Given an init pipeline with secure shell registered
		pipeline, mocks := setup(t)

		// Create mock secure shell
		mockSecureShell := shell.NewMockShell()
		secureShellInitialized := false
		mockSecureShell.InitializeFunc = func() error {
			secureShellInitialized = true
			return nil
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And secure shell should be initialized
		if !secureShellInitialized {
			t.Error("Expected secure shell to be initialized")
		}
	})

	t.Run("ReturnsErrorWhenSecureShellInitializeFails", func(t *testing.T) {
		// Given an init pipeline with failing secure shell
		pipeline, mocks := setup(t)

		// Create mock secure shell that fails to initialize
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("secure shell failed")
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "failed to initialize secure shell: secure shell failed" {
			t.Errorf("Expected secure shell error, got %q", err.Error())
		}
	})

	t.Run("SkipsSecureShellWhenNotRegistered", func(t *testing.T) {
		// Given an init pipeline without secure shell registered
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsSecureShellWhenRegisteredTypeIsIncorrect", func(t *testing.T) {
		// Given an init pipeline with incorrectly typed secure shell
		pipeline, mocks := setup(t)

		// Register something that's not a shell.Shell
		mocks.Injector.Register("secureShell", "not-a-shell")

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestInitPipeline_Execute(t *testing.T) {
	// Given a pipeline with mocks
	mocks := setupInitMocks(t)
	pipeline := NewInitPipeline()
	pipeline.blueprintHandler = mocks.BlueprintHandler
	pipeline.shell = mocks.Shell
	pipeline.toolsManager = mocks.ToolsManager
	pipeline.configHandler = mocks.ConfigHandler

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given successful mocks
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "token", nil
		}
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{"test.jsonnet": []byte("test")}, nil
		}
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return nil
		}
		mocks.BlueprintHandler.LoadConfigFunc = func() error {
			return nil
		}

		// Initialize the pipeline properly
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWriteResetTokenFails", func(t *testing.T) {
		// Given shell write reset token fails
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("reset token error")
		}

		// Initialize the pipeline properly
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error writing reset token") {
			t.Errorf("Expected reset token error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintLoadConfigFails", func(t *testing.T) {
		// Given successful reset token
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "token", nil
		}
		// And blueprint handler returns error on GetLocalTemplateData (template loading path)
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return nil, fmt.Errorf("template data error")
		}

		// Initialize the pipeline properly
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get template data") {
			t.Errorf("Expected template data error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenBlueprintWriteFails", func(t *testing.T) {
		// Given successful reset token
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "token", nil
		}
		// And successful template data loading
		mocks.BlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{"blueprint": []byte("test")}, nil
		}
		// And blueprint write fails
		mocks.BlueprintHandler.WriteFunc = func(overwrite ...bool) error {
			return fmt.Errorf("blueprint write error")
		}

		// Initialize the pipeline properly
		if err := pipeline.Initialize(mocks.Injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write blueprint file") {
			t.Errorf("Expected blueprint write error, got %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestInitPipeline_setDefaultConfiguration(t *testing.T) {
	setup := func(t *testing.T, vmDriver, platform string) (*InitPipeline, *config.MockConfigHandler) {
		t.Helper()
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return vmDriver
			case "provider":
				return platform
			default:
				return ""
			}
		}
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			return nil
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		return pipeline, mockConfigHandler
	}

	configurationTests := []struct {
		name        string
		vmDriver    string
		contextName string
		expectError bool
	}{
		{name: "HandlesDockerDesktopDriver", vmDriver: "docker-desktop", contextName: "test"},
		{name: "HandlesColimaDriver", vmDriver: "colima", contextName: "test"},
		{name: "HandlesDockerDriver", vmDriver: "docker", contextName: "test"},
		{name: "HandlesLocalContextWithoutDriver", vmDriver: "", contextName: "local"},
		{name: "HandlesLocalPrefixContextWithoutDriver", vmDriver: "", contextName: "local-dev"},
		{name: "HandlesUnknownDriver", vmDriver: "unknown", contextName: "test"},
	}

	for _, tt := range configurationTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a pipeline with specific configuration
			pipeline, _ := setup(t, tt.vmDriver, "")

			// When setDefaultConfiguration is called
			err := pipeline.setDefaultConfiguration(context.Background(), tt.contextName)

			// Then should complete successfully
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}

	t.Run("UsesContextNameAsProviderWhenNoProviderSet", func(t *testing.T) {
		// Given a pipeline with no provider set and "local" context name
		pipeline, mockConfigHandler := setup(t, "", "")
		providerSet := false
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				providerSet = true
			}
			return nil
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should set provider to "generic" and complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !providerSet {
			t.Error("Expected provider to be set from context name")
		}
	})

	t.Run("DoesNotSetProviderForNonLocalContexts", func(t *testing.T) {
		// Given a pipeline with no provider set and "aws" context name
		pipeline, mockConfigHandler := setup(t, "", "")
		providerSet := false
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				providerSet = true
			}
			return nil
		}

		// When setDefaultConfiguration is called with "aws" context
		err := pipeline.setDefaultConfiguration(context.Background(), "aws")

		// Then should not set provider automatically
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if providerSet {
			t.Error("Expected provider not to be set automatically for non-local contexts")
		}
	})

	t.Run("PrioritizesExplicitProviderOverContextName", func(t *testing.T) {
		// Given a pipeline with explicit provider "aws" and "local" context name
		pipeline, mockConfigHandler := setup(t, "", "aws")
		var appliedDefaults v1alpha1.Context
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			appliedDefaults = defaultConfig
			return nil
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should complete successfully and apply minimal defaults (not local defaults)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Should apply DefaultConfig (minimal) instead of DefaultConfig_Localhost (full local)
		// We can't directly compare the structs, but we can check that it's not the localhost config
		// by verifying that local-specific fields are not present
		if appliedDefaults.Docker != nil {
			t.Error("Expected no docker config when using explicit AWS provider")
		}
		if appliedDefaults.Git != nil {
			t.Error("Expected no git config when using explicit AWS provider")
		}
	})

	t.Run("UsesContextNameWhenNoProviderSpecified", func(t *testing.T) {
		// Given a pipeline with no provider set and "local" context name
		pipeline, mockConfigHandler := setup(t, "", "")
		var appliedDefaults v1alpha1.Context
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			appliedDefaults = defaultConfig
			return nil
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should complete successfully and apply local defaults
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Should apply local defaults (DefaultConfig_Localhost) which includes docker and git
		if appliedDefaults.Docker == nil {
			t.Error("Expected docker config when using local context name")
		}
		if appliedDefaults.Git == nil {
			t.Error("Expected git config when using local context name")
		}
	})

	t.Run("UsesContextNameAsProviderForLocal", func(t *testing.T) {
		// Given a pipeline with no provider set and "local" context name
		pipeline, mockConfigHandler := setup(t, "", "")
		var setProvider string
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				setProvider = value.(string)
			}
			return nil
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should set provider to "generic" and complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if setProvider != "generic" {
			t.Errorf("Expected provider to be set to 'generic', got %q", setProvider)
		}
	})

	t.Run("DoesNotUseContextNameAsProviderWhenProviderAlreadySet", func(t *testing.T) {
		// Given a pipeline with provider already set to "aws"
		pipeline, mockConfigHandler := setup(t, "", "aws")
		providerSetCount := 0
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				providerSetCount++
			}
			return nil
		}

		// When setDefaultConfiguration is called with "azure" context
		err := pipeline.setDefaultConfiguration(context.Background(), "azure")

		// Then should not set provider again and complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if providerSetCount > 0 {
			t.Errorf("Expected provider not to be set again, but it was set %d times", providerSetCount)
		}
	})

	t.Run("DoesNotUseContextNameAsProviderForUnknownProvider", func(t *testing.T) {
		// Given a pipeline with no provider set and unknown context name
		pipeline, mockConfigHandler := setup(t, "", "")
		providerSet := false
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				providerSet = true
			}
			return nil
		}

		// When setDefaultConfiguration is called with "unknown" context
		err := pipeline.setDefaultConfiguration(context.Background(), "unknown")

		// Then should not set provider and complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if providerSet {
			t.Error("Expected provider not to be set for unknown context name")
		}
	})

	t.Run("AlwaysAppliesDefaultConfigThenOverridesWithProviderSpecificSettings", func(t *testing.T) {
		// Given a pipeline with provider already set
		pipeline, mockConfigHandler := setup(t, "docker-desktop", "aws")
		defaultConfigSet := false
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			defaultConfigSet = true
			return nil
		}

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

		// Then should always set default config first, then override with provider-specific settings
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !defaultConfigSet {
			t.Error("Expected default config to be set even when provider is already set")
		}
	})

	t.Run("ReturnsErrorWhenSetProviderFromContextNameFails", func(t *testing.T) {
		// Given a pipeline with config handler that fails on Set for provider
		pipeline, mockConfigHandler := setup(t, "", "")
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "provider" {
				return fmt.Errorf("set provider failed")
			}
			return nil
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error setting provider from context name") {
			t.Errorf("Expected error to contain 'Error setting provider from context name', got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetDefaultFails", func(t *testing.T) {
		// Given a pipeline with config handler that fails on SetDefault
		pipeline, mockConfigHandler := setup(t, "docker-desktop", "")
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			return fmt.Errorf("set default failed")
		}

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error setting default config") {
			t.Errorf("Expected error to contain 'Error setting default config', got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSetFails", func(t *testing.T) {
		// Given a pipeline with config handler that fails on Set
		pipeline, mockConfigHandler := setup(t, "", "")
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return fmt.Errorf("set context value failed")
		}

		// When setDefaultConfiguration is called with "local" context
		err := pipeline.setDefaultConfiguration(context.Background(), "local")

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error setting vm.driver") {
			t.Errorf("Expected error to contain 'Error setting vm.driver', got %v", err)
		}
	})
}

func TestInitPipeline_processPlatformConfiguration(t *testing.T) {
	setup := func(t *testing.T, provider string) (*InitPipeline, *config.MockConfigHandler) {
		t.Helper()
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return provider
			}
			return ""
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		return pipeline, mockConfigHandler
	}

	providerTests := []struct {
		name     string
		provider string
	}{
		{name: "HandlesAWSProvider", provider: "aws"},
		{name: "HandlesAzureProvider", provider: "azure"},
		{name: "HandlesGenericProvider", provider: "generic"},
		{name: "HandlesEmptyProvider", provider: ""},
	}

	for _, tt := range providerTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a pipeline with specific provider configuration
			pipeline, _ := setup(t, tt.provider)

			// When processPlatformConfiguration is called
			err := pipeline.processPlatformConfiguration(context.Background())

			// Then should complete successfully
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}

	t.Run("ReturnsErrorWhenSetFails", func(t *testing.T) {
		// Given a pipeline with platform configuration that fails
		pipeline, mockConfigHandler := setup(t, "aws")
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return fmt.Errorf("config error")
		}

		// When processPlatformConfiguration is called
		err := pipeline.processPlatformConfiguration(context.Background())

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error setting aws.enabled") {
			t.Errorf("Expected error to contain 'Error setting aws.enabled', got %v", err)
		}
	})
}

func TestInitPipeline_prepareTemplateData(t *testing.T) {
	t.Run("Priority1_ExplicitBlueprintOverridesLocalTemplates", func(t *testing.T) {
		// Given a pipeline with both explicit blueprint and local templates
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
		pipeline.BasePipeline.injector = di.NewInjector()

		// Mock artifact builder that succeeds
		mockArtifactBuilder := artifact.NewMockArtifact()
		expectedOCIData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ explicit: 'oci-data' }"),
		}
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return expectedOCIData, nil
		}
		pipeline.BasePipeline.artifactBuilder = mockArtifactBuilder

		// Mock blueprint handler with local templates
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return map[string][]byte{
				"blueprint.jsonnet": []byte("{ local: 'template-data' }"),
			}, nil
		}
		pipeline.BasePipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// Create context with explicit blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.BasePipeline.prepareTemplateData(ctx)

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
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
		pipeline.BasePipeline.injector = di.NewInjector()

		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return nil, fmt.Errorf("OCI pull failed")
		}
		pipeline.BasePipeline.artifactBuilder = mockArtifactBuilder

		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.BasePipeline.prepareTemplateData(ctx)

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
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
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
		if err := pipeline.BasePipeline.Initialize(injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When prepareTemplateData is called with no blueprint context
		templateData, err := pipeline.BasePipeline.prepareTemplateData(context.Background())

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
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
		pipeline.BasePipeline.injector = di.NewInjector()

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
		pipeline.BasePipeline.artifactBuilder = mockArtifactBuilder

		// Mock blueprint handler with no local templates
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil // Empty local templates
		}
		pipeline.BasePipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// When prepareTemplateData is called with no blueprint context
		templateData, err := pipeline.BasePipeline.prepareTemplateData(context.Background())

		// Then should use default OCI URL and set fallback URL
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

	t.Run("Priority4_EmbeddedDefaultWhenNoArtifactBuilder", func(t *testing.T) {
		// Given a pipeline with no artifact builder
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
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
		if err := pipeline.BasePipeline.Initialize(injector, context.Background()); err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// Set artifact builder to nil to test the "no artifact builder" scenario
		pipeline.BasePipeline.artifactBuilder = nil

		// When prepareTemplateData is called
		templateData, err := pipeline.BasePipeline.prepareTemplateData(context.Background())

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
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
		pipeline.BasePipeline.injector = di.NewInjector()
		pipeline.BasePipeline.artifactBuilder = nil

		// Mock config handler (needed for determineContextName)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		pipeline.BasePipeline.configHandler = mockConfigHandler

		// Mock blueprint handler that returns empty data
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil // Empty local templates
		}
		mockBlueprintHandler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return make(map[string][]byte), nil // Empty default templates
		}
		pipeline.BasePipeline.injector.Register("blueprintHandler", mockBlueprintHandler)

		// When prepareTemplateData is called
		templateData, err := pipeline.BasePipeline.prepareTemplateData(context.Background())

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
		pipeline := &InitPipeline{}

		// Set up BasePipeline properly
		pipeline.BasePipeline = *NewBasePipeline()
		pipeline.BasePipeline.injector = di.NewInjector()

		mockArtifactBuilder := artifact.NewMockArtifact()
		pipeline.BasePipeline.artifactBuilder = mockArtifactBuilder

		// Create context with invalid blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "invalid-oci-reference")

		// When prepareTemplateData is called
		templateData, err := pipeline.BasePipeline.prepareTemplateData(ctx)

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

func TestInitPipeline_setDefaultConfiguration_HostPortsValidation(t *testing.T) {
	setup := func(t *testing.T, vmDriver string) (*InitPipeline, *config.MockConfigHandler, *v1alpha1.Context) {
		t.Helper()
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()

		// Track which default config was set
		var setDefaultConfig v1alpha1.Context

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return vmDriver
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			setDefaultConfig = defaultConfig
			return nil
		}
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		pipeline.configHandler = mockConfigHandler

		return pipeline, mockConfigHandler, &setDefaultConfig
	}

	t.Run("ColimaDriver_UsesConfigWithoutHostPorts", func(t *testing.T) {
		// Given a pipeline with colima driver
		pipeline, _, setConfigPtr := setup(t, "colima")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the default config should be DefaultConfig_Full (no hostports)
		setConfig := *setConfigPtr
		if setConfig.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// Verify no hostports for workers
		if len(setConfig.Cluster.Workers.HostPorts) != 0 {
			t.Errorf("Expected no hostports for colima driver, got %d: %v",
				len(setConfig.Cluster.Workers.HostPorts), setConfig.Cluster.Workers.HostPorts)
		}

		// Verify no hostports for controlplanes
		if len(setConfig.Cluster.ControlPlanes.HostPorts) != 0 {
			t.Errorf("Expected no hostports for colima driver controlplanes, got %d: %v",
				len(setConfig.Cluster.ControlPlanes.HostPorts), setConfig.Cluster.ControlPlanes.HostPorts)
		}
	})

	t.Run("DockerDriver_UsesConfigWithoutHostPorts", func(t *testing.T) {
		// Given a pipeline with docker driver
		pipeline, _, setConfigPtr := setup(t, "docker")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the default config should be DefaultConfig_Full (no hostports)
		setConfig := *setConfigPtr
		if setConfig.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// Verify no hostports for workers
		if len(setConfig.Cluster.Workers.HostPorts) != 0 {
			t.Errorf("Expected no hostports for docker driver, got %d: %v",
				len(setConfig.Cluster.Workers.HostPorts), setConfig.Cluster.Workers.HostPorts)
		}

		// Verify no hostports for controlplanes
		if len(setConfig.Cluster.ControlPlanes.HostPorts) != 0 {
			t.Errorf("Expected no hostports for docker driver controlplanes, got %d: %v",
				len(setConfig.Cluster.ControlPlanes.HostPorts), setConfig.Cluster.ControlPlanes.HostPorts)
		}
	})

	t.Run("DockerDesktopDriver_UsesConfigWithHostPorts", func(t *testing.T) {
		// Given a pipeline with docker-desktop driver
		pipeline, _, setConfigPtr := setup(t, "docker-desktop")

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

		// Then no error should occur
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the default config should be DefaultConfig_Localhost (with hostports)
		setConfig := *setConfigPtr
		if setConfig.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// Verify hostports are present for workers
		expectedHostPorts := []string{"8080:30080/tcp", "8443:30443/tcp", "9292:30292/tcp", "8053:30053/udp"}
		actualHostPorts := setConfig.Cluster.Workers.HostPorts

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Errorf("Expected %d hostports for docker-desktop driver, got %d",
				len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expected := range expectedHostPorts {
			if i >= len(actualHostPorts) || actualHostPorts[i] != expected {
				t.Errorf("Expected hostport %s at index %d, got %s", expected, i,
					func() string {
						if i < len(actualHostPorts) {
							return actualHostPorts[i]
						}
						return "missing"
					}())
			}
		}

		// Verify no hostports for controlplanes (only workers need them)
		if len(setConfig.Cluster.ControlPlanes.HostPorts) != 0 {
			t.Errorf("Expected no hostports for docker-desktop driver controlplanes, got %d: %v",
				len(setConfig.Cluster.ControlPlanes.HostPorts), setConfig.Cluster.ControlPlanes.HostPorts)
		}
	})
}
