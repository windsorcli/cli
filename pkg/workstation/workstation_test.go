package workstation

import (
	"bytes"
	"fmt"
	"io"
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

		ws := NewWorkstation(rt, &Workstation{NetworkManager: mocks.NetworkManager, VirtualMachine: mocks.VirtualMachine})
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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
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
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
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
		err := workstation.ConfigureNetwork("", false)

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
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		workstation.DeferHostGuestSetup = false

		// When calling Up()
		err := workstation.Up()

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
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		workstation.DeferHostGuestSetup = false

		// When calling Up()
		err := workstation.Up()

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
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		workstation.DeferHostGuestSetup = false

		// When calling Up()
		err := workstation.Up()

		// Then the error is propagated
		if err == nil {
			t.Error("Expected error for FlushDNS failure")
		}
		if !strings.Contains(err.Error(), "error flushing DNS cache") {
			t.Errorf("Expected flush DNS error, got: %v", err)
		}
	})

}

func TestWorkstation_DeferredWork(t *testing.T) {
	t.Run("ReturnsNilWhenNothingAppended", func(t *testing.T) {
		// Given a fresh workstation with no deferred work recorded
		mocks := setupWorkstationMocks(t)
		w := NewWorkstation(mocks.Runtime)

		// When reading DeferredWork before any append
		items := w.DeferredWork()

		// Then the slice should be nil
		if items != nil {
			t.Errorf("Expected nil deferred work, got %v", items)
		}
	})

	t.Run("AppendDeferredWorkAccumulatesInOrder", func(t *testing.T) {
		// Given a workstation
		mocks := setupWorkstationMocks(t)
		w := NewWorkstation(mocks.Runtime)

		// When appending two items
		w.appendDeferredWork(DeferredWorkItem{Required: true, Command: "windsor configure network"})
		w.appendDeferredWork(DeferredWorkItem{Required: false, Command: "windsor configure network", Outcome: "use *.local.test in your browser"})

		// Then DeferredWork should return both in insertion order
		items := w.DeferredWork()
		if len(items) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(items))
		}
		if !items[0].Required || items[0].Command != "windsor configure network" {
			t.Errorf("Expected first item required+command, got %+v", items[0])
		}
		if items[1].Required || items[1].Outcome != "use *.local.test in your browser" {
			t.Errorf("Expected second item optional+outcome, got %+v", items[1])
		}
	})

	t.Run("UpResetsDeferredWorkAtStart", func(t *testing.T) {
		// Given a workstation that previously accumulated deferred work
		mocks := setupWorkstationMocks(t)
		w := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		w.appendDeferredWork(DeferredWorkItem{Required: true, Command: "windsor configure network"})

		// When running Up()
		if err := w.Up(); err != nil {
			t.Fatalf("Expected Up to succeed, got %v", err)
		}

		// Then the prior deferred work should be cleared
		if items := w.DeferredWork(); items != nil {
			t.Errorf("Expected deferred work to be reset at start of Up, got %v", items)
		}
	})
}

func TestWorkstation_FlushDNS(t *testing.T) {
	t.Run("CallsFlushDNSWhenDNSConfigured", func(t *testing.T) {
		// Given a workstation with DNS domain and address configured
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return "mock-value"
		}
		flushCalled := false
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := workstation.FlushDNS()

		// Then no error occurs and FlushDNS was called on the network manager
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !flushCalled {
			t.Error("expected NetworkManager.FlushDNS to be called")
		}
	})

	t.Run("SkipsFlushWhenDNSDomainNotSet", func(t *testing.T) {
		// Given a workstation with no DNS domain
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		flushCalled := false
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := workstation.FlushDNS()

		// Then no error occurs and FlushDNS was not called
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if flushCalled {
			t.Error("expected NetworkManager.FlushDNS not to be called when domain is empty")
		}
	})

	t.Run("PropagatesNetworkManagerError", func(t *testing.T) {
		// Given a workstation where NetworkManager.FlushDNS returns an error
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return "mock-value"
		}
		mocks.NetworkManager.FlushDNSFunc = func() error {
			return fmt.Errorf("flush failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := workstation.FlushDNS()

		// Then the error is propagated
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("NoopWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation with no network manager
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When FlushDNS is called
		err := workstation.FlushDNS()

		// Then no error occurs
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

func TestWorkstation_RevertNetwork(t *testing.T) {
	t.Run("NoOpWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation without a NetworkManager
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: nil})

		// When reverting the network
		err := ws.RevertNetwork(false)

		// Then no error, no calls
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("SkipsClusterRevertOnNonColima", func(t *testing.T) {
		// Given a docker-desktop runtime
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		var calls []string
		mocks.NetworkManager.RevertGuestFunc = func() error { calls = append(calls, "guest"); return nil }
		mocks.NetworkManager.RevertHostRouteFunc = func() error { calls = append(calls, "route"); return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { calls = append(calls, "dns"); return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		if err := ws.RevertNetwork(false); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then only DNS revert runs; cluster reverts are skipped (no host route or in-VM forwarding on docker-desktop)
		if len(calls) != 1 || calls[0] != "dns" {
			t.Errorf("expected [dns], got %v", calls)
		}
	})

	t.Run("RevertsGuestThenRouteThenDNSOnColima", func(t *testing.T) {
		// Given a colima runtime
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		var calls []string
		mocks.NetworkManager.RevertGuestFunc = func() error { calls = append(calls, "guest"); return nil }
		mocks.NetworkManager.RevertHostRouteFunc = func() error { calls = append(calls, "route"); return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { calls = append(calls, "dns"); return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		if err := ws.RevertNetwork(false); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then all three reverts run in the expected order: guest (in-VM forwarding) before route
		// so the iptables rule is gone before the route stops carrying traffic
		want := []string{"guest", "route", "dns"}
		if len(calls) != len(want) {
			t.Fatalf("expected %v, got %v", want, calls)
		}
		for i := range want {
			if calls[i] != want[i] {
				t.Errorf("position %d: expected %q, got %q", i, want[i], calls[i])
			}
		}
	})

	t.Run("BubblesGuestRevertError", func(t *testing.T) {
		// Given colima with RevertGuest failing
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.NetworkManager.RevertGuestFunc = func() error { return fmt.Errorf("guest boom") }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		err := ws.RevertNetwork(false)

		// Then the guest error surfaces with context
		if err == nil || !strings.Contains(err.Error(), "error reverting guest: guest boom") {
			t.Errorf("expected guest revert error, got %v", err)
		}
	})

	t.Run("BubblesDNSRevertErrorOnDockerDesktop", func(t *testing.T) {
		// Given docker-desktop with RevertDNS failing
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.NetworkManager.RevertDNSFunc = func() error { return fmt.Errorf("dns boom") }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		err := ws.RevertNetwork(false)

		// Then the DNS error surfaces with context
		if err == nil || !strings.Contains(err.Error(), "error reverting DNS: dns boom") {
			t.Errorf("expected DNS revert error, got %v", err)
		}
	})
}

func TestWorkstation_PendingNetworkChanges(t *testing.T) {
	t.Run("EmptyWhenNothingPending", func(t *testing.T) {
		// Given a workstation whose network manager reports neither cluster nor DNS work pending
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return false }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return false }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then the list is empty (callers print "nothing pending")
		if len(got) != 0 {
			t.Errorf("expected empty pending list, got %v", got)
		}
	})

	t.Run("ListsHostRouteAndVMForwardWhenClusterPending", func(t *testing.T) {
		// Given cluster privilege is pending with concrete CIDR + guest address in config
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.10")
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return true }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return false }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then both cluster-privilege rows appear with config values interpolated into Detail
		want := []NetworkChange{
			{Kind: "host-route", Detail: "192.168.5.0/24 via 192.168.5.10"},
			{Kind: "vm-forward", Detail: "col0 -> docker bridge"},
		}
		if len(got) != len(want) {
			t.Fatalf("expected %d entries, got %d (%+v)", len(want), len(got), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d: expected %+v, got %+v", i, want[i], got[i])
			}
		}
	})

	t.Run("ListsResolverEntryWhenDNSPending", func(t *testing.T) {
		// Given DNS privilege is pending with concrete domain + resolver address in config
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return false }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return true }
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: mocks.NetworkManager})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then the resolver entry appears with domain + address interpolated into Detail
		want := NetworkChange{Kind: "dns-resolver", Detail: "*.local.test -> 10.5.0.2"}
		if len(got) != 1 || got[0] != want {
			t.Errorf("expected [%+v], got %+v", want, got)
		}
	})

	t.Run("ReturnsNilWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation with no NetworkManager (atypical, but the helper must not panic)
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime, &Workstation{NetworkManager: nil})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then a nil slice is returned
		if got != nil {
			t.Errorf("expected nil, got %v", got)
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

		_, err := hook("other-component")

		if err != nil {
			t.Errorf("Expected no error for non-workstation component, got: %v", err)
		}
		if configureNetworkCalled {
			t.Error("Expected ConfigureNetwork not to be called for non-workstation component")
		}
	})

	t.Run("CallbackReturnsNilWhenClusterPrivilegeNotNeeded", func(t *testing.T) {
		// Given a docker-desktop runtime (no cluster privilege ever needed) with no other privileged work
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		var calls []string
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return false }
		mocks.NetworkManager.ConfigureGuestFunc = func() error { calls = append(calls, "guest"); return nil }
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { calls = append(calls, "route"); return nil }
		mocks.NetworkManager.ConfigureDNSFunc = func() error { calls = append(calls, "dns"); return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires for the workstation component
		_, err := ws.MakeApplyHook()("workstation")

		// Then the hook returns nil and never invokes any network configuration
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
		if len(calls) != 0 {
			t.Errorf("expected no network configuration calls, got: %v", calls)
		}
	})

	t.Run("CallbackHaltsAndAppendsRequiredDeferredWorkWhenClusterPrivilegeNeeded", func(t *testing.T) {
		// Given cluster privilege is needed (host route + in-VM forwarding missing)
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return true }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return false }
		var configureCalls []string
		mocks.NetworkManager.ConfigureGuestFunc = func() error { configureCalls = append(configureCalls, "guest"); return nil }
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { configureCalls = append(configureCalls, "route"); return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires for the workstation component
		halt, err := ws.MakeApplyHook()("workstation")

		// Then the hook signals halt with no error, appends a required deferred-work item,
		// and never invokes inline ConfigureGuest/ConfigureHostRoute (windsor up must not prompt for sudo)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if !halt {
			t.Error("expected halt=true when cluster privilege is needed")
		}
		if len(configureCalls) != 0 {
			t.Errorf("expected no inline configure calls, got: %v", configureCalls)
		}
		items := ws.DeferredWork()
		if len(items) != 1 {
			t.Fatalf("expected 1 deferred-work item, got %d (%v)", len(items), items)
		}
		if !items[0].Required || items[0].Command != "windsor configure network" {
			t.Errorf("expected required item for 'windsor configure network', got %+v", items[0])
		}
	})

	t.Run("CallbackAppendsOptionalDeferredWorkWhenOnlyDNSPrivilegeNeeded", func(t *testing.T) {
		// Given only DNS privilege is needed (cluster reachable; host resolver not pointed at cluster DNS)
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return false }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return true }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()("workstation")

		// Then no halt is signaled and an optional deferred-work item describing the DNS outcome is appended
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if halt {
			t.Error("expected halt=false when only DNS privilege is needed (DNS is not load-bearing for subsequent components)")
		}
		items := ws.DeferredWork()
		if len(items) != 1 {
			t.Fatalf("expected 1 deferred-work item, got %d (%v)", len(items), items)
		}
		if items[0].Required {
			t.Error("expected optional item for DNS, got required")
		}
		if items[0].Outcome != "use *.local.test in your browser" {
			t.Errorf("expected outcome to embed dns.domain, got %q", items[0].Outcome)
		}
		if items[0].Command != "windsor configure network" {
			t.Errorf("expected command 'windsor configure network', got %q", items[0].Command)
		}
	})

	t.Run("CallbackAppendsBothItemsAndHaltsWhenClusterAndDNSPrivilegeNeeded", func(t *testing.T) {
		// Given both cluster and DNS privilege are needed (typical first-run colima case)
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return true }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return true }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()("workstation")

		// Then the hook halts and records both items; the renderer collapses to the halt sentence
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if !halt {
			t.Error("expected halt=true when cluster privilege is needed (regardless of DNS state)")
		}
		items := ws.DeferredWork()
		if len(items) != 2 {
			t.Fatalf("expected 2 deferred-work items, got %d (%v)", len(items), items)
		}
		if !items[0].Required {
			t.Errorf("expected first item to be required (cluster), got %+v", items[0])
		}
		if items[1].Required {
			t.Errorf("expected second item to be optional (DNS), got %+v", items[1])
		}
	})
}

func TestWorkstation_MakePostApplyHook(t *testing.T) {
	t.Run("ReturnsNilWhenDeferHostGuestSetupFalse", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := NewWorkstation(mocks.Runtime)
		ws.DeferHostGuestSetup = false

		hook := ws.MakePostApplyHook()

		if hook != nil {
			t.Error("Expected nil hook when DeferHostGuestSetup is false")
		}
	})

	t.Run("CallbackIgnoresNonWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		flushCalled := false
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook("other-component")

		if err != nil {
			t.Errorf("Expected no error for non-workstation component, got: %v", err)
		}
		if flushCalled {
			t.Error("Expected FlushDNS not to be called for non-workstation component")
		}
	})

	t.Run("CallbackFlushDNSForWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		mocks.ConfigHandler.(*config.MockConfigHandler).GetFunc = func(key string) any {
			return "mock-value"
		}
		flushCalled := false
		mocks.NetworkManager.DNSChangedFunc = func() bool { return true }
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook("workstation")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !flushCalled {
			t.Error("Expected FlushDNS to be called for workstation component")
		}
	})

	t.Run("CallbackSkipsFlushDNSWhenDNSUnchanged", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.Set("dns.domain", "test.example")
		mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.2")
		flushCalled := false
		mocks.NetworkManager.DNSChangedFunc = func() bool { return false }
		mocks.NetworkManager.FlushDNSFunc = func() error {
			flushCalled = true
			return nil
		}
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook("workstation")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if flushCalled {
			t.Error("Expected FlushDNS not to be called when DNS was not changed")
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

	t.Run("NoRevertOrHintWhenNothingInstalled", func(t *testing.T) {
		// Given a NetworkManager that reports no installed state
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsHostRouteInstalledFunc = func() bool { return false }
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return false }
		var revertCalled bool
		mocks.NetworkManager.RevertHostRouteFunc = func() error { revertCalled = true; return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { revertCalled = true; return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
			VirtualMachine:   mocks.VirtualMachine,
		})

		// When tearing down
		if err := ws.Down(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no revert is attempted — silent teardown
		if revertCalled {
			t.Errorf("expected no revert call when nothing installed, but one fired")
		}
	})

	t.Run("RevertsBeforeTeardownWhenInstalledAndCanElevate", func(t *testing.T) {
		// Given installed network state AND non-interactive elevation available (CI / root)
		t.Cleanup(SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))

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
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
			VirtualMachine:   mocks.VirtualMachine,
		})

		// When tearing down
		if err := ws.Down(); err != nil {
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
		t.Cleanup(SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return false }))

		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return true }
		var revertCalled bool
		mocks.NetworkManager.RevertHostRouteFunc = func() error { revertCalled = true; return nil }
		mocks.NetworkManager.RevertDNSFunc = func() error { revertCalled = true; return nil }
		mocks.ContainerRuntime.DownFunc = func() error { return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When tearing down
		if err := ws.Down(); err != nil {
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
		t.Cleanup(SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))

		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.IsResolverInstalledFunc = func() bool { return true }
		mocks.NetworkManager.RevertDNSFunc = func() error { return fmt.Errorf("dns boom") }
		var teardownCalled bool
		mocks.ContainerRuntime.DownFunc = func() error { teardownCalled = true; return nil }
		ws := NewWorkstation(mocks.Runtime, &Workstation{
			NetworkManager:   mocks.NetworkManager,
			ContainerRuntime: mocks.ContainerRuntime,
		})

		stderrBuf, restore := captureProcessStderrForWorkstationTest(t)
		t.Cleanup(restore)

		// When tearing down
		if err := ws.Down(); err != nil {
			t.Fatalf("expected teardown to succeed despite revert failure, got %v", err)
		}
		restore()

		// Then a warning was emitted but teardown proceeded — operator's primary intent honored
		if !teardownCalled {
			t.Errorf("expected teardown to proceed despite revert failure")
		}
		if !strings.Contains(stderrBuf.String(), "warning: failed to revert host network configuration") {
			t.Errorf("expected warning in stderr, got %q", stderrBuf.String())
		}
	})
}

// captureProcessStderrForWorkstationTest is a local pipe-based stderr capture for tests that
// assert on lines written via fmt.Fprintln(os.Stderr, ...) from production code. Mirrors the
// equivalent helper in pkg/cmd/root_test.go (kept package-local here to avoid a cross-package
// test-helper dependency).
func captureProcessStderrForWorkstationTest(t *testing.T) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	buf = new(bytes.Buffer)
	restore = func() {
		w.Close()
		_, _ = io.Copy(buf, r)
		os.Stderr = orig
	}
	return buf, restore
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
