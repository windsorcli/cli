package workstation_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/shims"
	ctxpkg "github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation"
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
		case "workstation.runtime":
			return "colima"
		case "docker.enabled":
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

	tmpDir := t.TempDir()
	rt := &ctxpkg.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir + "/contexts/test-context",
		TemplateRoot:  tmpDir + "/contexts/_template",
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir+"/contexts/_template"),
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
		ws := workstation.NewWorkstation(mocks.Runtime)

		// Then the workstation is created without components
		if ws == nil {
			t.Fatal("Expected workstation to be created")
		}
		if ws.NetworkManager != nil || ws.VirtualMachine != nil || ws.ContainerRuntime != nil {
			t.Error("Expected no components before Prepare")
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
		_ = workstation.NewWorkstation(nil)
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
		_ = workstation.NewWorkstation(rt)
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
		_ = workstation.NewWorkstation(rt)
	})

	t.Run("NoErrorWhenShellIsProvided", func(t *testing.T) {
		// Given a runtime with Shell
		mocks := setupWorkstationMocks(t)
		rt := mocks.Runtime

		// When creating a new workstation
		ws := workstation.NewWorkstation(rt)

		// Then the workstation should be created successfully
		if ws == nil {
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
		_ = workstation.NewWorkstation(nil)
	})

	t.Run("NoErrorWhenRuntimeIsProvided", func(t *testing.T) {
		// Given a valid runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		ws := workstation.NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		if ws == nil {
			t.Error("Expected workstation to be created")
		}
	})

	t.Run("CreatesDependencies", func(t *testing.T) {
		// Given a properly configured runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		ws := workstation.NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		// And NetworkManager should not be created yet (created in Prepare)
		if ws.NetworkManager != nil {
			t.Error("Expected NetworkManager not to be created in NewWorkstation (created in Prepare)")
		}
		// And VirtualMachine should not be created yet (created in Prepare)
		if ws.VirtualMachine != nil {
			t.Error("Expected VirtualMachine not to be created in NewWorkstation (created in Prepare)")
		}
		// And ContainerRuntime should not be created yet (created in Prepare)
		if ws.ContainerRuntime != nil {
			t.Error("Expected ContainerRuntime not to be created in NewWorkstation (created in Prepare)")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		opts := &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
		}

		ws := workstation.NewWorkstation(mocks.Runtime, opts)

		if ws == nil {
			t.Error("Expected workstation to be created")
		}
		if ws.NetworkManager != mocks.NetworkManager {
			t.Error("Expected existing NetworkManager to be used")
		}
		if ws.VirtualMachine != mocks.VirtualMachine {
			t.Error("Expected existing VirtualMachine to be used")
		}
		if ws.ContainerRuntime != mocks.ContainerRuntime {
			t.Error("Expected existing ContainerRuntime to be used")
		}
	})

	t.Run("DoesNotBackfillWorkstationAddressFromVmAddress", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		recorded := make(map[string]any)
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value any) error {
			recorded[key] = value
			return nil
		}
		mockHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if v, ok := recorded[key]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
			switch key {
			case "workstation.arch":
				return ""
			case "workstation.runtime":
				return "colima"
			case "workstation.address":
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

		ws := workstation.NewWorkstation(rt, &workstation.Workstation{NetworkManager: mocks.NetworkManager, VirtualMachine: mocks.VirtualMachine})
		if err := ws.Prepare(); err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}

		if _, ok := recorded["workstation.address"]; ok {
			t.Errorf("Expected workstation.address to remain unset, got recorded %v", recorded["workstation.address"])
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

		// Then the workstation should start successfully without errors
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SetsNoCacheEnvironmentVariable", func(t *testing.T) {
		// Given a workstation with all dependencies configured
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Errorf("Expected error about writing VM config, got: %v", err)
		}
	})

	t.Run("ErrorSettingNoCache", func(t *testing.T) {
		// Given a workstation whose Setenv shim fails
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		s := shims.NewShims()
		s.Setenv = func(key, value string) error {
			return fmt.Errorf("setenv failed")
		}
		ws.SetShims(s)

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

		// Then the Setenv error should be returned with context
		if err == nil || !strings.Contains(err.Error(), "setenv failed") {
			t.Fatalf("Expected wrapped setenv error, got: %v", err)
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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When calling Up() to start the workstation
		err := ws.Up(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When ConfigureNetwork is called (e.g. from apply hook)
		err := ws.ConfigureNetwork(context.Background(), "", false)

		// Then the error is propagated
		if err == nil {
			t.Error("Expected error for network configuration failure")
			return
		}
		if !strings.Contains(err.Error(), "error configuring host route") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("FlushesAfterConfigureDNSWhenChanged", func(t *testing.T) {
		// Given a workstation without DeferHostGuestSetup where DNS is configured and changed
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return nil
		}
		flushCalled := false
		mocks.NetworkManager.DNSChangedFunc = func() bool { return true }
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = false

		// When calling Up()
		err := ws.Up(context.Background())

		// Then no error occurs and FlushDNS was called
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !flushCalled {
			t.Error("Expected FlushDNS to be called after ConfigureDNS changed the resolver")
		}
	})

	t.Run("SkipsFlushAfterConfigureDNSWhenUnchanged", func(t *testing.T) {
		// Given a workstation without DeferHostGuestSetup where DNS is configured but unchanged
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return nil
		}
		flushCalled := false
		mocks.NetworkManager.DNSChangedFunc = func() bool { return false }
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = false

		// When calling Up()
		err := ws.Up(context.Background())

		// Then no error occurs and FlushDNS was not called
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if flushCalled {
			t.Error("Expected FlushDNS not to be called when DNS was not changed")
		}
	})

	t.Run("PropagatesFlushDNSError", func(t *testing.T) {
		// Given a workstation without DeferHostGuestSetup where FlushDNS fails
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return nil
		}
		mocks.NetworkManager.DNSChangedFunc = func() bool { return true }
		mocks.NetworkManager.FlushDNSFunc = func() error {
			return fmt.Errorf("flush failed")
		}
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = false

		// When calling Up()
		err := ws.Up(context.Background())

		// Then the error is propagated
		if err == nil {
			t.Error("Expected error for FlushDNS failure")
		}
		if !strings.Contains(err.Error(), "error flushing DNS cache") {
			t.Errorf("Expected flush DNS error, got: %v", err)
		}
	})

}

func TestWorkstation_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When
		err := ws.Down(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := ws.Down(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := ws.Down(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := ws.Down(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := ws.Down(context.Background())

		// Then
		if err == nil {
			t.Error("Expected error for VM stop failure")
		}
		if !strings.Contains(err.Error(), "Error running virtual machine Down command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NoRevertOrHintWhenNothingInstalled", func(t *testing.T) {
		// Given a NetworkManager that reports no installed state
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsHostRouteInstalledFunc = func() bool { return false }
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return false }
		var revertCalled bool
		mocks.NetworkManager.RevertHostRouteFunc = func() error { revertCalled = true; return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { revertCalled = true; return nil }
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
			VirtualMachine:   mocks.VirtualMachine,
		})

		// When tearing down
		if err := ws.Down(context.Background()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no revert is attempted — silent teardown
		if revertCalled {
			t.Errorf("expected no revert call when nothing installed, but one fired")
		}
	})

	t.Run("RevertsBeforeTeardownWhenInstalledAndCanElevate", func(t *testing.T) {
		// Given installed network state AND non-interactive elevation available (CI / root)
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))

		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.NetworkManager.IsHostRouteInstalledFunc = func() bool { return true }
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return true }
		var order []string
		mocks.NetworkManager.RevertGuestFunc = func() error { order = append(order, "revert-guest"); return nil }
		mocks.NetworkManager.RevertHostRouteFunc = func() error { order = append(order, "revert-route"); return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { order = append(order, "revert-dns"); return nil }
		mocks.ContainerRuntime.DownFunc = func() error { order = append(order, "runtime-down"); return nil }
		mocks.VirtualMachine.DownFunc = func() error { order = append(order, "vm-down"); return nil }
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
			VirtualMachine:   mocks.VirtualMachine,
		})

		// When tearing down
		if err := ws.Down(context.Background()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then revert runs BEFORE teardown so RevertGuest can still SSH into the live VM
		want := []string{"revert-guest", "revert-route", "revert-dns", "runtime-down", "vm-down"}
		if len(order) != len(want) {
			t.Fatalf("expected order %v, got %v", want, order)
		}
		for i := range want {
			if order[i] != want[i] {
				t.Errorf("position %d: expected %q, got %q", i, want[i], order[i])
			}
		}
	})

	t.Run("RecordsRevertDeferredWorkWhenInstalledButCannotElevate", func(t *testing.T) {
		// Given installed network state but no non-interactive elevation
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return false }))

		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return true }
		var revertCalled bool
		mocks.NetworkManager.RevertHostRouteFunc = func() error { revertCalled = true; return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { revertCalled = true; return nil }
		mocks.ContainerRuntime.DownFunc = func() error { return nil }
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When tearing down
		if err := ws.Down(context.Background()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no revert ran inline (no surprise sudo prompt during down), and the leftover
		// configuration is recorded as a deferred-work item the cmd layer renders after teardown
		if revertCalled {
			t.Errorf("expected no revert call when cannot elevate")
		}
		items := ws.DeferredWork()
		if len(items) != 1 {
			t.Fatalf("expected 1 deferred-work item, got %d (%v)", len(items), items)
		}
		if items[0].Required {
			t.Error("expected optional item (down completed; cleanup is at operator's convenience)")
		}
		if items[0].Command != "windsor configure network --revert" {
			t.Errorf("expected --revert command, got %q", items[0].Command)
		}
		if items[0].Outcome != "remove host configuration" {
			t.Errorf("expected outcome 'remove host configuration', got %q", items[0].Outcome)
		}
	})

	t.Run("WarnsButProceedsOnRevertError", func(t *testing.T) {
		// Given installed state and elevation available, but RevertDNS fails partway
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))

		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return true }
		mocks.NetworkManager.RevertDNSFunc = func() error { return fmt.Errorf("dns boom") }
		var teardownCalled bool
		mocks.ContainerRuntime.DownFunc = func() error { teardownCalled = true; return nil }
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
		})

		rec := &recorder{}
		ctx := presenter.NewContext(context.Background(), rec)

		// When tearing down
		if err := ws.Down(ctx); err != nil {
			t.Fatalf("expected teardown to succeed despite revert failure, got %v", err)
		}

		// Then a warning was emitted but teardown proceeded — operator's primary intent honored
		if !teardownCalled {
			t.Errorf("expected teardown to proceed despite revert failure")
		}
		warnings := rec.messages(slog.LevelWarn)
		if len(warnings) != 1 || !strings.Contains(warnings[0], "failed to revert host network configuration") {
			t.Errorf("expected one revert warning, got %q", warnings)
		}
	})
}

func TestWorkstation_Integration(t *testing.T) {
	t.Run("FullUpDownCycle", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When - Up
		err := ws.Up(context.Background())

		// Then
		if err != nil {
			t.Errorf("Expected Up to succeed, got error: %v", err)
		}

		// When - Down
		err = ws.Down(context.Background())

		// Then
	})

	t.Run("MultipleUpDownCycles", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})

		// When - Multiple cycles
		for i := 0; i < 3; i++ {
			err := ws.Up(context.Background())
			if err != nil {
				t.Errorf("Expected Up cycle %d to succeed, got error: %v", i+1, err)
			}

			err = ws.Down(context.Background())
			if err != nil {
				t.Errorf("Expected Down cycle %d to succeed, got error: %v", i+1, err)
			}
		}
	})
}
