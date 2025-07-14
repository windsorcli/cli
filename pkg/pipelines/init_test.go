package pipelines

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/template"
	"github.com/windsorcli/cli/pkg/terraform"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type InitMocks struct {
	*Mocks
	BlueprintHandler  *blueprint.MockBlueprintHandler
	KubernetesManager *kubernetes.MockKubernetesManager
	ToolsManager      *tools.MockToolsManager
	Stack             *stack.MockStack
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
	mockBlueprintHandler.LoadConfigFunc = func(reset ...bool) error { return nil }
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
	mockStack := stack.NewMockStack(baseMocks.Injector)
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
}

func TestInitPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*InitPipeline, *InitMocks) {
		t.Helper()
		pipeline := NewInitPipeline()
		mocks := setupInitMocks(t, opts...)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized InitPipeline
		pipeline, _ := setup(t)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenContextIDGenerationFails", func(t *testing.T) {
		// Given a pipeline with a config handler that fails to generate context ID
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.SetContextFunc = func(contextName string) error { return nil }
		mockConfigHandler.GenerateContextIDFunc = func() error {
			return fmt.Errorf("context ID generation failed")
		}
		mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error { return nil }
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error { return nil }

		setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
		pipeline, _ := setup(t, setupOptions)

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then should return context ID generation error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to generate context ID") {
			t.Errorf("Expected error to contain 'failed to generate context ID', got %v", err)
		}
	})

	t.Run("ProcessesTemplateDataSuccessfully", func(t *testing.T) {
		// Given a pipeline with template renderer and template data
		pipeline, mocks := setup(t)

		// Add template renderer mock
		mockTemplateRenderer := template.NewMockTemplate(mocks.Injector)
		mockTemplateRenderer.ProcessFunc = func(templateData map[string][]byte, renderedData map[string]any) error {
			// Simulate template processing that creates blueprint data
			renderedData["blueprint"] = map[string]any{"test": "data"}
			return nil
		}
		mocks.Injector.Register("templateRenderer", mockTemplateRenderer)
		pipeline.templateRenderer = mockTemplateRenderer

		// Add blueprint generator mock
		mockBlueprintGenerator := generators.NewMockGenerator()
		mockBlueprintGenerator.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
			return nil
		}
		mocks.Injector.Register("blueprintGenerator", mockBlueprintGenerator)

		// Configure blueprint handler to return template data
		mocks.BlueprintHandler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return map[string][]byte{"test.yaml": []byte("test: data")}, nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ProcessesTerraformModulesSuccessfully", func(t *testing.T) {
		// Given a pipeline with terraform module resolvers
		pipeline, mocks := setup(t)

		// Add terraform resolver mock
		mockResolver := terraform.NewMockModuleResolver(mocks.Injector)
		mockResolver.ProcessModulesFunc = func() error {
			return nil
		}
		pipeline.terraformResolvers = []terraform.ModuleResolver{mockResolver}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	// Test execution failures
	execFailureTests := []struct {
		name        string
		setupMock   func(*InitMocks)
		expectedErr string
	}{
		{
			name: "ReturnsErrorWhenBlueprintLoadConfigFails",
			setupMock: func(mocks *InitMocks) {
				mocks.BlueprintHandler.LoadConfigFunc = func(reset ...bool) error {
					return fmt.Errorf("blueprint load config failed")
				}
			},
			expectedErr: "Error reloading blueprint config after generation: blueprint load config failed",
		},
		{
			name: "ReturnsErrorWhenSaveConfigFails",
			setupMock: func(mocks *InitMocks) {
				// This test case requires a mock config handler to inject the error
				// It will be handled separately below
			},
			expectedErr: "save config failed",
		},
		{
			name: "ReturnsErrorWhenToolsWriteManifestFails",
			setupMock: func(mocks *InitMocks) {
				mocks.ToolsManager.WriteManifestFunc = func() error {
					return fmt.Errorf("tools write manifest failed")
				}
			},
			expectedErr: "error writing tools manifest: tools write manifest failed",
		},
		{
			name: "ReturnsErrorWhenAddCurrentDirToTrustedFileFails",
			setupMock: func(mocks *InitMocks) {
				mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
					return fmt.Errorf("add current dir failed")
				}
			},
			expectedErr: "Error adding current directory to trusted file: add current dir failed",
		},
		{
			name: "ReturnsErrorWhenWriteResetTokenFails",
			setupMock: func(mocks *InitMocks) {
				mocks.Shell.WriteResetTokenFunc = func() (string, error) {
					return "", fmt.Errorf("write reset token failed")
				}
			},
			expectedErr: "Error writing reset token: write reset token failed",
		},
	}

	for _, tt := range execFailureTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "ReturnsErrorWhenSaveConfigFails" {
				// This test requires a mock config handler to inject the error
				mockConfigHandler := config.NewMockConfigHandler()
				mockConfigHandler.InitializeFunc = func() error { return nil }
				mockConfigHandler.SetContextFunc = func(contextName string) error { return nil }
				mockConfigHandler.GenerateContextIDFunc = func() error { return nil }
				mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error {
					return fmt.Errorf("save config failed")
				}
				mockConfigHandler.IsLoadedFunc = func() bool { return true }
				mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error { return nil }
				mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error { return nil }

				setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
				pipeline, _ := setup(t, setupOptions)

				// When executing the pipeline
				err := pipeline.Execute(context.Background())

				// Then an error should be returned
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
				}
			} else {
				// Given an init pipeline with failing component
				pipeline, mocks := setup(t)
				tt.setupMock(mocks)

				// When executing the pipeline
				err := pipeline.Execute(context.Background())

				// Then an error should be returned
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
				}
			}
		})
	}
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
			case "platform":
				return platform
			default:
				return ""
			}
		}
		mockConfigHandler.SetDefaultFunc = func(defaultConfig v1alpha1.Context) error {
			return nil
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
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

	t.Run("ReturnsErrorWhenSetContextValueFails", func(t *testing.T) {
		// Given a pipeline with config handler that fails on SetContextValue
		pipeline, mockConfigHandler := setup(t, "docker", "")
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return fmt.Errorf("set context value failed")
		}

		// When setDefaultConfiguration is called
		err := pipeline.setDefaultConfiguration(context.Background(), "test")

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
	setup := func(t *testing.T, platform string) (*InitPipeline, *config.MockConfigHandler) {
		t.Helper()
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return platform
			}
			return ""
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		return pipeline, mockConfigHandler
	}

	platformTests := []struct {
		name     string
		platform string
	}{
		{name: "HandlesAWSPlatform", platform: "aws"},
		{name: "HandlesAzurePlatform", platform: "azure"},
		{name: "HandlesMetalPlatform", platform: "metal"},
		{name: "HandlesLocalPlatform", platform: "local"},
		{name: "HandlesEmptyPlatform", platform: ""},
	}

	for _, tt := range platformTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a pipeline with specific platform configuration
			pipeline, _ := setup(t, tt.platform)

			// When processPlatformConfiguration is called
			err := pipeline.processPlatformConfiguration(context.Background())

			// Then should complete successfully
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}

	t.Run("ReturnsErrorWhenSetContextValueFails", func(t *testing.T) {
		// Given a pipeline with platform configuration that fails
		pipeline, mockConfigHandler := setup(t, "aws")
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
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

func TestInitPipeline_saveConfiguration(t *testing.T) {
	setup := func(t *testing.T, yamlExists, ymlExists bool) (*InitPipeline, *config.MockConfigHandler) {
		t.Helper()
		pipeline := &InitPipeline{}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test", nil
		}
		pipeline.shell = mockShell

		mockShims := NewShims()
		mockShims.Stat = func(path string) (os.FileInfo, error) {
			if path == "/test/windsor.yaml" && yamlExists {
				return &mockInitFileInfo{name: "windsor.yaml", isDir: false}, nil
			}
			if path == "/test/windsor.yml" && ymlExists {
				return &mockInitFileInfo{name: "windsor.yml", isDir: false}, nil
			}
			return nil, fmt.Errorf("not found")
		}
		pipeline.shims = mockShims

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SaveConfigFunc = func(path string, overwrite ...bool) error {
			return nil
		}
		pipeline.configHandler = mockConfigHandler

		return pipeline, mockConfigHandler
	}

	t.Run("SavesWithYamlFile", func(t *testing.T) {
		// Given a pipeline with existing windsor.yaml file
		pipeline, _ := setup(t, true, false)

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(false)

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SavesWithYmlFile", func(t *testing.T) {
		// Given a pipeline with existing windsor.yml file
		pipeline, _ := setup(t, false, true)

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(false)

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SavesWithDefaultYamlWhenNeitherExists", func(t *testing.T) {
		// Given a pipeline with no existing config files
		pipeline, _ := setup(t, false, false)

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(false)

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		// Given a pipeline with shell that fails to get project root
		pipeline := &InitPipeline{}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root failed")
		}
		pipeline.shell = mockShell

		// When saveConfiguration is called
		err := pipeline.saveConfiguration(false)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error retrieving project root") {
			t.Errorf("Expected error to contain 'Error retrieving project root', got %v", err)
		}
	})
}

func TestInitPipeline_writeConfigurationFiles(t *testing.T) {
	t.Run("WritesAllConfigurationsSuccessfully", func(t *testing.T) {
		// Given a pipeline with all components
		pipeline := &InitPipeline{}

		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.WriteManifestFunc = func() error { return nil }
		pipeline.toolsManager = mockToolsManager

		mockService := services.NewMockService()
		mockService.WriteConfigFunc = func() error { return nil }
		pipeline.services = []services.Service{mockService}

		mockVirtualMachine := virt.NewMockVirt()
		mockVirtualMachine.WriteConfigFunc = func() error { return nil }
		pipeline.virtualMachine = mockVirtualMachine

		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error { return nil }
		pipeline.containerRuntime = mockContainerRuntime

		// When writeConfigurationFiles is called
		err := pipeline.writeConfigurationFiles()

		// Then should complete successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	configWriteErrorTests := []struct {
		name        string
		setupMock   func(*InitPipeline)
		expectedErr string
	}{
		{
			name: "ReturnsErrorWhenServiceWriteConfigFails",
			setupMock: func(pipeline *InitPipeline) {
				mockService := services.NewMockService()
				mockService.WriteConfigFunc = func() error {
					return fmt.Errorf("service write config failed")
				}
				pipeline.services = []services.Service{mockService}
			},
			expectedErr: "error writing service config",
		},
		{
			name: "ReturnsErrorWhenVirtualMachineWriteConfigFails",
			setupMock: func(pipeline *InitPipeline) {
				mockVirtualMachine := virt.NewMockVirt()
				mockVirtualMachine.WriteConfigFunc = func() error {
					return fmt.Errorf("vm write config failed")
				}
				pipeline.virtualMachine = mockVirtualMachine
			},
			expectedErr: "error writing virtual machine config",
		},
		{
			name: "ReturnsErrorWhenContainerRuntimeWriteConfigFails",
			setupMock: func(pipeline *InitPipeline) {
				mockContainerRuntime := virt.NewMockVirt()
				mockContainerRuntime.WriteConfigFunc = func() error {
					return fmt.Errorf("container runtime write config failed")
				}
				pipeline.containerRuntime = mockContainerRuntime
			},
			expectedErr: "error writing container runtime config",
		},
	}

	for _, tt := range configWriteErrorTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a pipeline with failing component
			pipeline := &InitPipeline{}
			tt.setupMock(pipeline)

			// When writeConfigurationFiles is called
			err := pipeline.writeConfigurationFiles()

			// Then should return error
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestInitPipeline_prepareTemplateData(t *testing.T) {
	t.Run("ReturnsEmptyMapWhenNoBlueprintHandler", func(t *testing.T) {
		// Given a pipeline with no blueprint handler
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return "" // No blueprint flag
		}
		pipeline.configHandler = mockConfigHandler

		pipeline.blueprintHandler = nil

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

	t.Run("ReturnsErrorWhenOCIFails", func(t *testing.T) {
		// Given a pipeline with OCI blueprint that fails
		pipeline := &InitPipeline{}

		// Mock artifact builder that fails
		mockArtifactBuilder := artifact.NewMockArtifact()
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return nil, fmt.Errorf("OCI pull failed")
		}
		pipeline.artifactBuilder = mockArtifactBuilder

		// Create context with blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get OCI template data") {
			t.Errorf("Expected error to contain 'failed to get OCI template data', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("ReturnsErrorWhenArtifactBuilderMissing", func(t *testing.T) {
		// Given a pipeline with OCI blueprint but no artifact builder
		pipeline := &InitPipeline{}
		pipeline.artifactBuilder = nil

		// Create context with blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should return error
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "artifact builder not available") {
			t.Errorf("Expected error to contain 'artifact builder not available', got %v", err)
		}
		if templateData != nil {
			t.Error("Expected nil template data on error")
		}
	})

	t.Run("UsesArtifactBuilderForOCIBlueprint", func(t *testing.T) {
		// Given a pipeline with OCI blueprint and artifact builder
		pipeline := &InitPipeline{}

		mockArtifactBuilder := artifact.NewMockArtifact()
		expectedTemplateData := map[string][]byte{
			"blueprint.jsonnet": []byte("{ test: 'data' }"),
		}
		mockArtifactBuilder.GetTemplateDataFunc = func(ociRef string) (map[string][]byte, error) {
			return expectedTemplateData, nil
		}
		pipeline.artifactBuilder = mockArtifactBuilder

		// Create context with blueprint value
		ctx := context.WithValue(context.Background(), "blueprint", "oci://registry.example.com/blueprint:latest")

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(ctx)

		// Then should use artifact builder
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "{ test: 'data' }" {
			t.Error("Expected correct template data from artifact builder")
		}
	})

	t.Run("UsesLocalTemplateDataWhenAvailable", func(t *testing.T) {
		// Given a pipeline with local template data
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return "" // No blueprint flag
		}
		pipeline.configHandler = mockConfigHandler

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		expectedLocalData := map[string][]byte{
			"blueprint.jsonnet": []byte("local template data"),
		}
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return expectedLocalData, nil
		}
		pipeline.blueprintHandler = mockBlueprintHandler

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use local template data
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "local template data" {
			t.Error("Expected local template data")
		}
	})

	t.Run("FallsBackToDefaultTemplateData", func(t *testing.T) {
		// Given a pipeline with no local templates but blueprint handler
		pipeline := &InitPipeline{}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return "" // No blueprint flag
		}
		pipeline.configHandler = mockConfigHandler

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(nil)
		// Return empty local template data
		mockBlueprintHandler.GetLocalTemplateDataFunc = func() (map[string][]byte, error) {
			return make(map[string][]byte), nil
		}
		// Return default template data
		expectedDefaultData := map[string][]byte{
			"blueprint.jsonnet": []byte("default template data"),
		}
		mockBlueprintHandler.GetDefaultTemplateDataFunc = func(contextName string) (map[string][]byte, error) {
			return expectedDefaultData, nil
		}
		pipeline.blueprintHandler = mockBlueprintHandler

		// When prepareTemplateData is called
		templateData, err := pipeline.prepareTemplateData(context.Background())

		// Then should use default template data
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(templateData) != 1 {
			t.Errorf("Expected 1 template file, got %d", len(templateData))
		}
		if string(templateData["blueprint.jsonnet"]) != "default template data" {
			t.Error("Expected default template data")
		}
	})
}
