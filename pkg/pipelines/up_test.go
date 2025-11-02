package pipelines

import (
	"context"
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/context/config"
	envvars "github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/context/tools"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type UpMocks struct {
	*Mocks
	ToolsManager     *tools.MockToolsManager
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
	NetworkManager   *network.MockNetworkManager
	Stack            *terraforminfra.MockStack
}

func setupUpMocks(t *testing.T, opts ...*SetupOptions) *UpMocks {
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

		// Load base config with up-specific settings
		configYAML := `
apiVersion: v1alpha1
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
      enabled: true
    network:
      cidr_block: 10.0.0.0/24
    docker:
      enabled: true
    vm:
      driver: colima
    tools:
      enabled: true`

		if err := configHandler.LoadConfigString(configYAML); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
	}

	// Setup tools manager mock
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockToolsManager.CheckFunc = func() error { return nil }
	mockToolsManager.InstallFunc = func() error { return nil }
	baseMocks.Injector.Register("toolsManager", mockToolsManager)

	// Setup virtual machine mock
	mockVirtualMachine := virt.NewMockVirt()
	mockVirtualMachine.InitializeFunc = func() error { return nil }
	mockVirtualMachine.UpFunc = func(verbose ...bool) error { return nil }
	baseMocks.Injector.Register("virtualMachine", mockVirtualMachine)

	// Setup container runtime mock
	mockContainerRuntime := virt.NewMockVirt()
	mockContainerRuntime.InitializeFunc = func() error { return nil }
	mockContainerRuntime.UpFunc = func(verbose ...bool) error { return nil }
	baseMocks.Injector.Register("containerRuntime", mockContainerRuntime)

	// Setup network manager mock
	mockNetworkManager := network.NewMockNetworkManager()
	mockNetworkManager.InitializeFunc = func() error { return nil }
	mockNetworkManager.ConfigureGuestFunc = func() error { return nil }
	mockNetworkManager.ConfigureHostRouteFunc = func() error { return nil }
	mockNetworkManager.ConfigureDNSFunc = func() error { return nil }
	baseMocks.Injector.Register("networkManager", mockNetworkManager)

	// Setup stack mock
	mockStack := terraforminfra.NewMockStack(baseMocks.Injector)
	mockStack.InitializeFunc = func() error { return nil }
	mockStack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
	baseMocks.Injector.Register("stack", mockStack)

	// Setup terraform env mock
	mockTerraformEnv := envvars.NewMockEnvPrinter()
	mockTerraformEnv.InitializeFunc = func() error { return nil }
	mockTerraformEnv.GetEnvVarsFunc = func() (map[string]string, error) { return map[string]string{}, nil }
	baseMocks.Injector.Register("terraformEnv", mockTerraformEnv)

	// Add GetSessionTokenFunc to the existing shell mock
	baseMocks.Shell.GetSessionTokenFunc = func() (string, error) { return "mock-session-token", nil }

	return &UpMocks{
		Mocks:            baseMocks,
		ToolsManager:     mockToolsManager,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
		NetworkManager:   mockNetworkManager,
		Stack:            mockStack,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewUpPipeline(t *testing.T) {
	t.Run("CreatesWithDefaults", func(t *testing.T) {
		// Given creating a new up pipeline
		pipeline := NewUpPipeline()

		// Then pipeline should not be nil
		if pipeline == nil {
			t.Fatal("Expected pipeline to not be nil")
		}
	})
}

// =============================================================================
// Test Public Methods - Initialize
// =============================================================================

func TestUpPipeline_Initialize(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*UpPipeline, *UpMocks) {
		t.Helper()
		pipeline := NewUpPipeline()
		mocks := setupUpMocks(t, opts...)
		return pipeline, mocks
	}

	t.Run("InitializesSuccessfully", func(t *testing.T) {
		// Given an up pipeline with mock dependencies
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
		setupMock   func(*UpMocks)
		expectedErr string
	}{
		{
			name: "ReturnsErrorWhenShellInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mockShell := shell.NewMockShell()
				mockShell.InitializeFunc = func() error {
					return fmt.Errorf("shell initialization failed")
				}
				mocks.Injector.Register("shell", mockShell)
			},
			expectedErr: "failed to initialize shell: shell initialization failed",
		},
		{
			name: "ReturnsErrorWhenToolsManagerInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mocks.ToolsManager.InitializeFunc = func() error {
					return fmt.Errorf("tools manager failed")
				}
			},
			expectedErr: "failed to initialize tools manager: tools manager failed",
		},
		{
			name: "ReturnsErrorWhenVirtualMachineInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mocks.VirtualMachine.InitializeFunc = func() error {
					return fmt.Errorf("virtual machine failed")
				}
			},
			expectedErr: "failed to initialize virtual machine: virtual machine failed",
		},
		{
			name: "ReturnsErrorWhenContainerRuntimeInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mocks.ContainerRuntime.InitializeFunc = func() error {
					return fmt.Errorf("container runtime failed")
				}
			},
			expectedErr: "failed to initialize container runtime: container runtime failed",
		},
		{
			name: "ReturnsErrorWhenNetworkManagerInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mocks.NetworkManager.InitializeFunc = func() error {
					return fmt.Errorf("network manager failed")
				}
			},
			expectedErr: "failed to initialize network manager: network manager failed",
		},
		{
			name: "ReturnsErrorWhenStackInitializeFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Stack.InitializeFunc = func() error {
					return fmt.Errorf("stack failed")
				}
			},
			expectedErr: "failed to initialize stack: stack failed",
		},
	}

	for _, tt := range initFailureTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an up pipeline with failing component
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
		// Given an up pipeline with secure shell registered
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
		// Given an up pipeline with failing secure shell
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
		// Given an up pipeline without secure shell registered
		pipeline, mocks := setup(t)

		// When initializing the pipeline
		err := pipeline.Initialize(mocks.Injector, context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsSecureShellWhenRegisteredTypeIsIncorrect", func(t *testing.T) {
		// Given an up pipeline with incorrectly typed secure shell
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

// =============================================================================
// Test Public Methods - Execute
// =============================================================================

func TestUpPipeline_Execute(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*UpPipeline, *UpMocks) {
		t.Helper()
		pipeline := NewUpPipeline()
		mocks := setupUpMocks(t, opts...)

		err := pipeline.Initialize(mocks.Injector, context.Background())
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		return pipeline, mocks
	}

	t.Run("ExecutesSuccessfully", func(t *testing.T) {
		// Given a properly initialized UpPipeline
		pipeline, mocks := setup(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ExecutesWithVerboseFlag", func(t *testing.T) {
		// Given a pipeline with verbose flag set during initialization
		pipeline := NewUpPipeline()
		mocks := setupUpMocks(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		// Initialize with verbose context
		verboseCtx := context.WithValue(context.Background(), "verbose", true)
		err := pipeline.Initialize(mocks.Injector, verboseCtx)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		// When Execute is called
		err = pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipsVirtualMachineWhenNotColima", func(t *testing.T) {
		// Given a pipeline with non-colima VM driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "docker" // Not colima
			default:
				return ""
			}
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return false
			default:
				return false
			}
		}

		setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
		pipeline, mocks := setup(t, setupOptions)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		vmUpCalled := false
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			vmUpCalled = true
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned and VM Up should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if vmUpCalled {
			t.Error("Expected virtual machine Up to not be called when driver is not colima")
		}
	})

	t.Run("SkipsContainerRuntimeWhenDisabled", func(t *testing.T) {
		// Given a pipeline with docker disabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "colima"
			default:
				return ""
			}
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return false // Docker disabled
			case "dns.enabled":
				return false
			default:
				return false
			}
		}

		setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
		pipeline, mocks := setup(t, setupOptions)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		containerUpCalled := false
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			containerUpCalled = true
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned and container runtime Up should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if containerUpCalled {
			t.Error("Expected container runtime Up to not be called when docker is disabled")
		}
	})

	t.Run("ShowsDNSNotificationWhenEnabled", func(t *testing.T) {
		// Given a pipeline with DNS enabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return true // DNS enabled
			default:
				return false
			}
		}

		setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
		pipeline, mocks := setup(t, setupOptions)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// The notification appears in stderr output (visible in test output)
	})

	t.Run("SkipsDNSNotificationWhenDisabled", func(t *testing.T) {
		// Given a pipeline with DNS disabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return false // DNS disabled
			default:
				return false
			}
		}

		setupOptions := &SetupOptions{ConfigHandler: mockConfigHandler}
		pipeline, mocks := setup(t, setupOptions)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}

		// When Execute is called
		err := pipeline.Execute(context.Background())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// No notification should appear (can verify by comparing test output)
	})

	// Test execution failures
	execFailureTests := []struct {
		name        string
		setupMock   func(*UpMocks)
		expectedErr string
	}{
		{
			name: "ReturnsErrorWhenSetenvFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error {
					return fmt.Errorf("setenv failed")
				}
			},
			expectedErr: "Error setting NO_CACHE environment variable: setenv failed",
		},
		{
			name: "ReturnsErrorWhenToolsCheckFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.ToolsManager.CheckFunc = func() error {
					return fmt.Errorf("tools check failed")
				}
			},
			expectedErr: "Error checking tools: tools check failed",
		},
		{
			name: "ReturnsErrorWhenToolsInstallFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.ToolsManager.InstallFunc = func() error {
					return fmt.Errorf("tools install failed")
				}
			},
			expectedErr: "Error installing tools: tools install failed",
		},
		{
			name: "ReturnsErrorWhenVirtualMachineUpFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
					return fmt.Errorf("vm up failed")
				}
			},
			expectedErr: "Error running virtual machine Up command: vm up failed",
		},
		{
			name: "ReturnsErrorWhenContainerRuntimeUpFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
					return fmt.Errorf("container runtime up failed")
				}
			},
			expectedErr: "Error running container runtime Up command: container runtime up failed",
		},
		{
			name: "ReturnsErrorWhenNetworkConfigureGuestFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.NetworkManager.ConfigureGuestFunc = func() error {
					return fmt.Errorf("configure guest failed")
				}
			},
			expectedErr: "Error configuring guest network: configure guest failed",
		},
		{
			name: "ReturnsErrorWhenNetworkConfigureHostRouteFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
					return fmt.Errorf("configure host route failed")
				}
			},
			expectedErr: "Error configuring host network: configure host route failed",
		},
		{
			name: "ReturnsErrorWhenNetworkConfigureDNSFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.NetworkManager.ConfigureDNSFunc = func() error {
					return fmt.Errorf("configure dns failed")
				}
			},
			expectedErr: "Error configuring DNS: configure dns failed",
		},
		{
			name: "ReturnsErrorWhenStackUpFails",
			setupMock: func(mocks *UpMocks) {
				mocks.Shims.Setenv = func(key, value string) error { return nil }
				mocks.Stack.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
					return fmt.Errorf("stack up failed")
				}
			},
			expectedErr: "Error running stack Up command: stack up failed",
		},
	}

	for _, tt := range execFailureTests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an up pipeline with failing component
			pipeline, mocks := setup(t)
			tt.setupMock(mocks)

			ctx := context.Background()

			// When executing the pipeline
			err := pipeline.Execute(ctx)

			// Then an error should be returned
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if err.Error() != tt.expectedErr {
				t.Errorf("Expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}

	t.Run("ReturnsErrorWhenNoVirtualMachineFound", func(t *testing.T) {
		// Given an up pipeline with nil virtual machine
		pipeline, mocks := setup(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error { return nil }

		// Set virtual machine to nil
		pipeline.virtualMachine = nil

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No virtual machine found" {
			t.Errorf("Expected 'No virtual machine found', got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenNoContainerRuntimeFound", func(t *testing.T) {
		// Given an up pipeline with nil container runtime
		pipeline, mocks := setup(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error { return nil }

		// Set container runtime to nil
		pipeline.containerRuntime = nil

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No container runtime found" {
			t.Errorf("Expected 'No container runtime found', got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenNoNetworkManagerFound", func(t *testing.T) {
		// Given an up pipeline with nil network manager
		pipeline, mocks := setup(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error { return nil }

		// Set network manager to nil
		pipeline.networkManager = nil

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No network manager found" {
			t.Errorf("Expected 'No network manager found', got %q", err.Error())
		}
	})

	t.Run("ReturnsErrorWhenNoStackFound", func(t *testing.T) {
		// Given an up pipeline with nil stack
		pipeline, mocks := setup(t)

		// Setup shims to allow NO_CACHE environment variable setting
		mocks.Shims.Setenv = func(key, value string) error { return nil }

		// Set stack to nil
		pipeline.stack = nil

		// When executing the pipeline
		err := pipeline.Execute(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "No stack found" {
			t.Errorf("Expected 'No stack found', got %q", err.Error())
		}
	})
}
