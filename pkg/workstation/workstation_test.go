package workstation

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	ctxpkg "github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type WorkstationTestMocks struct {
	Runtime          *ctxpkg.Runtime
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	NetworkManager   *network.MockNetworkManager
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
}

func setupWorkstationMocks(t *testing.T, opts ...func(*WorkstationTestMocks)) *WorkstationTestMocks {
	t.Helper()

	// Create mock config handler
	mockConfigHandler := config.NewMockConfigHandler()

	// Create mock shell
	mockShell := shell.NewMockShell()

	// Create mock network manager
	mockNetworkManager := network.NewMockNetworkManager()

	// Create mock virtual machine
	mockVirtualMachine := virt.NewMockVirt()

	// Create mock container runtime
	mockContainerRuntime := virt.NewMockVirt()

	// Store values set via Set() for GetString() to retrieve
	configValues := make(map[string]any)

	// Set up mock behaviors
	mockConfigHandler.SetFunc = func(key string, value any) error {
		configValues[key] = value
		return nil
	}

	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if val, ok := configValues[key]; ok {
			if strVal, ok := val.(string); ok {
				return strVal
			}
		}
		switch key {
		case "vm.driver", "workstation.runtime":
			return "colima"
		case "docker.enabled":
			return "true"
		case "dns.enabled":
			return "true"
		case "git.livereload.enabled":
			return "true"
		case "aws.localstack.enabled":
			return "true"
		case "cluster.driver":
			return "talos"
		case "cluster.controlplanes.count":
			return "2"
		case "cluster.workers.count":
			return "1"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
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
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}
	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
		switch key {
		case "cluster.controlplanes.count":
			return 2
		case "cluster.workers.count":
			return 1
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
	}

	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return []string{}
	}

	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Registries: map[string]docker.RegistryConfig{
					"test-registry": {
						HostPort: 5000,
						Remote:   "https://registry.example.com",
					},
				},
			},
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	// Set up mock network manager behaviors
	mockNetworkManager.ConfigureHostRouteFunc = func() error { return nil }
	mockNetworkManager.ConfigureGuestFunc = func() error { return nil }
	mockNetworkManager.ConfigureDNSFunc = func() error { return nil }

	// Set up mock virtual machine behaviors
	mockVirtualMachine.UpFunc = func(verbose ...bool) error {
		if err := mockConfigHandler.Set("workstation.address", "192.168.1.10"); err != nil {
			return err
		}
		return nil
	}
	mockVirtualMachine.DownFunc = func() error { return nil }

	// Set up mock container runtime behaviors
	mockContainerRuntime.UpFunc = func(verbose ...bool) error { return nil }
	mockContainerRuntime.DownFunc = func() error { return nil }

	rt := &ctxpkg.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     evaluator.NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/project/contexts/_template"),
	}

	mocks := &WorkstationTestMocks{
		Runtime:          rt,
		ConfigHandler:    mockConfigHandler,
		Shell:            mockShell,
		NetworkManager:   mockNetworkManager,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewWorkstation(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured runtime with all required dependencies
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation with the runtime
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully without errors
		// And the workstation should not be nil
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
		// And the ConfigHandler should be set
		if workstation.configHandler == nil {
			t.Error("Expected ConfigHandler to be set")
		}
		// And the Shell should be set
		if workstation.shell == nil {
			t.Error("Expected Shell to be set")
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		// Given a nil runtime is provided
		_ = setupWorkstationMocks(t)

		// When creating a new workstation with nil runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil runtime")
			}
		}()
		_ = NewWorkstation(nil)
	})

	t.Run("NilConfigHandler", func(t *testing.T) {
		// Given a runtime with nil ConfigHandler
		mocks := setupWorkstationMocks(t)
		rt := &ctxpkg.Runtime{
			Shell: mocks.Shell,
		}

		// When creating a new workstation with the incomplete runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil ConfigHandler")
			}
		}()
		_ = NewWorkstation(rt)
	})

	t.Run("NilShell", func(t *testing.T) {
		// Given a runtime with nil Shell
		mocks := setupWorkstationMocks(t)
		rt := &ctxpkg.Runtime{
			ConfigHandler: mocks.ConfigHandler,
		}

		// When creating a new workstation with the incomplete runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil Shell")
			}
		}()
		_ = NewWorkstation(rt)
	})

	t.Run("NoErrorWhenShellIsProvided", func(t *testing.T) {
		// Given a runtime with Shell
		mocks := setupWorkstationMocks(t)
		rt := mocks.Runtime

		// When creating a new workstation
		workstation := NewWorkstation(rt)

		// Then the workstation should be created successfully
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
	})

	t.Run("NilRuntime", func(t *testing.T) {
		// Given a nil runtime is provided
		_ = setupWorkstationMocks(t)

		// When creating a new workstation with nil runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil runtime")
			}
		}()
		_ = NewWorkstation(nil)
	})

	t.Run("NoErrorWhenRuntimeIsProvided", func(t *testing.T) {
		// Given a valid runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
	})

	t.Run("CreatesDependencies", func(t *testing.T) {
		// Given a properly configured runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		// And NetworkManager should not be created yet (created in Prepare)
		if workstation.NetworkManager != nil {
			t.Error("Expected NetworkManager not to be created in NewWorkstation (created in Prepare)")
		}
		// And VirtualMachine should not be created yet (created in Prepare)
		if workstation.VirtualMachine != nil {
			t.Error("Expected VirtualMachine not to be created in NewWorkstation (created in Prepare)")
		}
		// And ContainerRuntime should not be created yet (created in Prepare)
		if workstation.ContainerRuntime != nil {
			t.Error("Expected ContainerRuntime not to be created in NewWorkstation (created in Prepare)")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		opts := &Workstation{
			NetworkManager:   mocks.NetworkManager,
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
		}

		workstation := NewWorkstation(mocks.Runtime, opts)

		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
		if workstation.NetworkManager != mocks.NetworkManager {
			t.Error("Expected existing NetworkManager to be used")
		}
		if workstation.VirtualMachine != mocks.VirtualMachine {
			t.Error("Expected existing VirtualMachine to be used")
		}
		if workstation.ContainerRuntime != mocks.ContainerRuntime {
			t.Error("Expected existing ContainerRuntime to be used")
		}
	})

	t.Run("SetsWorkstationConfigDefaultsWhenEmpty", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		recorded := make(map[string]any)
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value any) error {
			recorded[key] = value
			return nil
		}
		mockHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "colima"
			case "vm.address":
				return "192.168.1.1"
			case "workstation.arch", "workstation.runtime", "workstation.address":
				return ""
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		rt := &ctxpkg.Runtime{
			ContextName:   "test",
			ProjectRoot:   "/test/project",
			ConfigRoot:    "/test/project/contexts/test",
			TemplateRoot:  "/test/project/contexts/_template",
			ConfigHandler: mockHandler,
			Shell:         mocks.Shell,
			Evaluator:     evaluator.NewExpressionEvaluator(mockHandler, "/test/project", "/test/project/contexts/_template"),
		}

		_ = NewWorkstation(rt)

		expectedArch := runtime.GOARCH
		if expectedArch == "arm" {
			expectedArch = "arm64"
		}
		if got, ok := recorded["workstation.arch"]; !ok || got != expectedArch {
			t.Errorf("Expected workstation.arch to be set to %q, got recorded %v", expectedArch, recorded["workstation.arch"])
		}
		if got, ok := recorded["workstation.runtime"]; !ok || got != "colima" {
			t.Errorf("Expected workstation.runtime to be set from vm.driver (colima), got recorded %v", recorded["workstation.runtime"])
		}
		if got, ok := recorded["workstation.address"]; !ok || got != "192.168.1.1" {
			t.Errorf("Expected workstation.address to be set from vm.address (192.168.1.1), got recorded %v", recorded["workstation.address"])
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWorkstation_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a workstation with all dependencies configured
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully without errors
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SetsNoCacheEnvironmentVariable", func(t *testing.T) {
		// Given a workstation with all dependencies configured
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And the NO_CACHE environment variable should be set to "true"
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE environment variable to be set")
		}
	})

	t.Run("StartsVirtualMachine", func(t *testing.T) {
		// Given a workstation with a virtual machine configured and a tracking flag for Up() calls
		mocks := setupWorkstationMocks(t)
		vmUpCalled := false
		vmWriteConfigCalled := false
		callOrder := []string{}
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			vmWriteConfigCalled = true
			callOrder = append(callOrder, "WriteConfig")
			return nil
		}
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			vmUpCalled = true
			callOrder = append(callOrder, "Up")
			return nil
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And VirtualMachine.WriteConfig() should be called
		if !vmWriteConfigCalled {
			t.Error("Expected VirtualMachine.WriteConfig to be called")
		}
		// And VirtualMachine.Up() should be called
		if !vmUpCalled {
			t.Error("Expected VirtualMachine.Up to be called")
		}
		// And WriteConfig should be called before Up
		if len(callOrder) != 2 || callOrder[0] != "WriteConfig" || callOrder[1] != "Up" {
			t.Errorf("Expected WriteConfig to be called before Up, got call order: %v", callOrder)
		}
	})

	t.Run("StartsContainerRuntime", func(t *testing.T) {
		// Given a workstation with a container runtime configured and a tracking flag for Up() calls
		mocks := setupWorkstationMocks(t)
		containerUpCalled := false
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			containerUpCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And ContainerRuntime.Up() should be called
		if !containerUpCalled {
			t.Error("Expected ContainerRuntime.Up to be called")
		}
	})

	t.Run("DeferNetworkConfigToHook", func(t *testing.T) {
		// Given a workstation with network manager; host/guest/DNS are not run during Up()
		// but via the apply hook or ConfigureNetwork() after the workstation Terraform component.
		mocks := setupWorkstationMocks(t)
		hostRouteCalled := false
		guestCalled := false
		dnsCalled := false

		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			hostRouteCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			guestCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureDNSFunc = func() error {
			dnsCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		workstation.DeferHostGuestSetup = true

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And ConfigureHostRoute/ConfigureGuest/ConfigureDNS are not called during Up() (deferred to hook)
		if hostRouteCalled {
			t.Error("Expected ConfigureHostRoute not to be called during Up() (deferred to apply hook)")
		}
		if guestCalled {
			t.Error("Expected ConfigureGuest not to be called during Up() (deferred to apply hook)")
		}
		if dnsCalled {
			t.Error("Expected ConfigureDNS not to be called during Up() (deferred to apply hook)")
		}
	})

	t.Run("VirtualMachineWriteConfigError", func(t *testing.T) {
		// Given a workstation with a virtual machine that will fail when writing config
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("VM config write failed")
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Errorf("Expected error about writing VM config, got: %v", err)
		}
	})

	t.Run("VirtualMachineUpError", func(t *testing.T) {
		// Given a workstation with a virtual machine that will fail when starting
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return nil
		}
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("VM start failed")
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for VM start failure")
		}
		// And the error message should indicate virtual machine Up command failure
		if !strings.Contains(err.Error(), "error running virtual machine Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ContainerRuntimeUpError", func(t *testing.T) {
		// Given a workstation with a container runtime that will fail when starting
		mocks := setupWorkstationMocks(t)
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("container start failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for container start failure")
		}
		// And the error message should indicate container runtime Up command failure
		if !strings.Contains(err.Error(), "error running container runtime Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ConfigureNetworkPropagatesHostRouteError", func(t *testing.T) {
		// Given a workstation with network manager where ConfigureHostRoute fails
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return fmt.Errorf("network config failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When ConfigureNetwork is called (e.g. from apply hook)
		err := workstation.ConfigureNetwork("")

		// Then the error is propagated
		if err == nil {
			t.Error("Expected error for network configuration failure")
			return
		}
		if !strings.Contains(err.Error(), "error configuring host route") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

}

func TestWorkstation_PrepareForUp(t *testing.T) {
	t.Run("ClearsDeferHostGuestSetupWhenBlueprintNil", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime)
		ws.DeferHostGuestSetup = true

		ws.PrepareForUp(nil)

		if ws.DeferHostGuestSetup {
			t.Error("Expected DeferHostGuestSetup false when blueprint is nil")
		}
	})

	t.Run("LeavesDeferHostGuestSetupFalseWhenTerraformDisabled", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		ws := NewWorkstation(mocks.Runtime)
		blueprint := &v1alpha1.Blueprint{
			TerraformComponents: []v1alpha1.TerraformComponent{{Name: "workstation", Path: "workstation"}},
		}

		ws.PrepareForUp(blueprint)

		if ws.DeferHostGuestSetup {
			t.Error("Expected DeferHostGuestSetup false when terraform.enabled is false")
		}
	})

	t.Run("SetsDeferHostGuestSetupWhenBlueprintHasWorkstationComponentAndTerraformEnabled", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		ws := NewWorkstation(mocks.Runtime)
		blueprint := &v1alpha1.Blueprint{
			TerraformComponents: []v1alpha1.TerraformComponent{{Name: "workstation", Path: "workstation"}},
		}

		ws.PrepareForUp(blueprint)

		if !ws.DeferHostGuestSetup {
			t.Error("Expected DeferHostGuestSetup true when blueprint has workstation component and terraform enabled")
		}
	})

	t.Run("LeavesDeferHostGuestSetupFalseWhenBlueprintHasNoWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		ws := NewWorkstation(mocks.Runtime)
		blueprint := &v1alpha1.Blueprint{
			TerraformComponents: []v1alpha1.TerraformComponent{{Name: "other", Path: "other"}},
		}

		ws.PrepareForUp(blueprint)

		if ws.DeferHostGuestSetup {
			t.Error("Expected DeferHostGuestSetup false when blueprint has no workstation component")
		}
	})
}

func TestWorkstation_EnsureNetworkPrivilege(t *testing.T) {
	t.Run("NoOpWhenNetworkManagerNil", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   nil,
		})

		err := ws.EnsureNetworkPrivilege()

		if err != nil {
			t.Errorf("Expected no error when NetworkManager is nil, got: %v", err)
		}
	})

	t.Run("NoOpWhenNeedsPrivilegeFalse", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.NeedsPrivilegeFunc = func() bool {
			return false
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		err := ws.EnsureNetworkPrivilege()

		if err != nil {
			t.Errorf("Expected no error when NeedsPrivilege false, got: %v", err)
		}
	})

	t.Run("ErrorWhenPrivilegeCheckFails", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.NeedsPrivilegeFunc = func() bool {
			return true
		}
		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("sudo required")
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("passwordless sudo required")
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		err := ws.EnsureNetworkPrivilege()

		if err == nil {
			t.Error("Expected error when privilege check fails")
			return
		}
		if !strings.Contains(err.Error(), "privileged access required") && !strings.Contains(err.Error(), "network configuration may require sudo") {
			t.Errorf("Expected error about privilege/sudo, got: %v", err)
		}
	})
}

func TestWorkstation_MakeApplyHook(t *testing.T) {
	t.Run("ReturnsNilWhenDeferHostGuestSetupFalse", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime)
		ws.DeferHostGuestSetup = false

		hook := ws.MakeApplyHook()

		if hook != nil {
			t.Error("Expected nil hook when DeferHostGuestSetup is false")
		}
	})

	t.Run("CallbackIgnoresNonWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true
		configureNetworkCalled := false
		mocks.NetworkManager.ConfigureDNSFunc = func() error {
			configureNetworkCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureGuestFunc = func() error { return nil }
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { return nil }

		hook := ws.MakeApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook("other-component")

		if err != nil {
			t.Errorf("Expected no error for non-workstation component, got: %v", err)
		}
		if configureNetworkCalled {
			t.Error("Expected ConfigureNetwork not to be called for non-workstation component")
		}
	})

	t.Run("CallbackCallsConfigureNetworkForWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		configureNetworkCalled := false
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			configureNetworkCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			configureNetworkCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureDNSFunc = func() error {
			configureNetworkCalled = true
			return nil
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakeApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook("workstation")

		if err != nil {
			t.Errorf("Expected no error for workstation component, got: %v", err)
		}
		if !configureNetworkCalled {
			t.Error("Expected ConfigureNetwork to be called for workstation component")
		}
	})
}

func TestWorkstation_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("StopsContainerRuntime", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		containerDownCalled := false
		mocks.ContainerRuntime.DownFunc = func() error {
			containerDownCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !containerDownCalled {
			t.Error("Expected ContainerRuntime.Down to be called")
		}
	})

	t.Run("StopsVirtualMachine", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		vmDownCalled := false
		mocks.VirtualMachine.DownFunc = func() error {
			vmDownCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !vmDownCalled {
			t.Error("Expected VirtualMachine.Down to be called")
		}
	})

	t.Run("ContainerRuntimeDownError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mocks.ContainerRuntime.DownFunc = func() error {
			return fmt.Errorf("container stop failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := workstation.Down()

		// Then
		if err == nil {
			t.Error("Expected error for container stop failure")
		}
		if !strings.Contains(err.Error(), "Error running container runtime Down command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("VirtualMachineDownError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.DownFunc = func() error {
			return fmt.Errorf("VM stop failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := workstation.Down()

		// Then
		if err == nil {
			t.Error("Expected error for VM stop failure")
		}
		if !strings.Contains(err.Error(), "Error running virtual machine Down command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestWorkstation_Integration(t *testing.T) {
	t.Run("FullUpDownCycle", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When - Up
		err := workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected Up to succeed, got error: %v", err)
		}

		// When - Down
		err = workstation.Down()

		// Then
	})

	t.Run("MultipleUpDownCycles", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When - Multiple cycles
		for i := 0; i < 3; i++ {
			err := workstation.Up()
			if err != nil {
				t.Errorf("Expected Up cycle %d to succeed, got error: %v", i+1, err)
			}

			err = workstation.Down()
			if err != nil {
				t.Errorf("Expected Down cycle %d to succeed, got error: %v", i+1, err)
			}
		}
	})
}
