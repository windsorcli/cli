package workstation_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/workstation"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// recorder is a presenter that stores each event it receives.
type recorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWorkstation_Prepare(t *testing.T) {
	t.Run("CreatesColimaComponentsForDockerPlatform", func(t *testing.T) {
		// Given workstation runtime colima and platform docker
		mocks := setupWorkstationMocks(t)
		_ = mocks.ConfigHandler.Set("platform", "docker")
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When preparing the components
		err := ws.Prepare()

		// Then the network manager and the VM are Colima, and the container runtime is Docker
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := ws.NetworkManager.(*network.ColimaNetworkManager); !ok {
			t.Errorf("Expected a ColimaNetworkManager, got %T", ws.NetworkManager)
		}
		if _, ok := ws.VirtualMachine.(*virt.ColimaVirt); !ok {
			t.Errorf("Expected a ColimaVirt, got %T", ws.VirtualMachine)
		}
		if _, ok := ws.ContainerRuntime.(*virt.DockerVirt); !ok {
			t.Errorf("Expected a DockerVirt, got %T", ws.ContainerRuntime)
		}
	})

	t.Run("UsesTheColimaVMOfIncus", func(t *testing.T) {
		// Given workstation runtime colima and platform incus
		mocks := setupWorkstationMocks(t)
		_ = mocks.ConfigHandler.Set("platform", "incus")
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When preparing the components
		err := ws.Prepare()

		// Then the container runtime is Incus and the VM is its embedded ColimaVirt
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		incusVirt, ok := ws.ContainerRuntime.(*virt.IncusVirt)
		if !ok {
			t.Fatalf("Expected an IncusVirt, got %T", ws.ContainerRuntime)
		}
		if ws.VirtualMachine != virt.VirtualMachine(incusVirt.ColimaVirt) {
			t.Errorf("Expected the VM to be the ColimaVirt of the IncusVirt, got %T", ws.VirtualMachine)
		}
	})

	t.Run("CreatesOnlyABaseNetworkManagerWithoutColima", func(t *testing.T) {
		// Given workstation runtime docker-desktop and no platform
		mocks := setupWorkstationMocks(t)
		_ = mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When preparing the components
		err := ws.Prepare()

		// Then only a base network manager exists
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := ws.NetworkManager.(*network.BaseNetworkManager); !ok {
			t.Errorf("Expected a BaseNetworkManager, got %T", ws.NetworkManager)
		}
		if ws.VirtualMachine != nil || ws.ContainerRuntime != nil {
			t.Errorf("Expected no VM and no container runtime, got %T and %T", ws.VirtualMachine, ws.ContainerRuntime)
		}
	})

	t.Run("KeepsComponentsFromOptions", func(t *testing.T) {
		// Given components passed to the constructor
		mocks := setupWorkstationMocks(t)
		_ = mocks.ConfigHandler.Set("platform", "docker")
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager:   mocks.NetworkManager,
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When preparing the components
		err := ws.Prepare()

		// Then the passed components stay
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ws.NetworkManager != network.NetworkManager(mocks.NetworkManager) ||
			ws.VirtualMachine != virt.VirtualMachine(mocks.VirtualMachine) ||
			ws.ContainerRuntime != virt.ContainerRuntime(mocks.ContainerRuntime) {
			t.Error("Expected the passed components to stay")
		}
	})
}

func TestWorkstation_ConfigureNetwork(t *testing.T) {
	setup := func(t *testing.T) (*WorkstationTestMocks, *workstation.Workstation, *recorder, context.Context) {
		t.Helper()
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})
		rec := &recorder{}
		return mocks, ws, rec, presenter.NewContext(context.Background(), rec)
	}

	t.Run("DoesNothingWithoutNetworkManager", func(t *testing.T) {
		// Given a workstation without a network manager
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When configuring the network
		err := ws.ConfigureNetwork(context.Background(), "", true)

		// Then nothing happens
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfiguresGuestRouteAndDNSOnColima", func(t *testing.T) {
		// Given colima with a DNS domain and a DNS address
		mocks, ws, rec, ctx := setup(t)
		_ = mocks.ConfigHandler.Set("dns.domain", "local.test")
		_ = mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.53")
		var calls []string
		mocks.NetworkManager.ConfigureGuestFunc = func() error { calls = append(calls, "guest"); return nil }
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { calls = append(calls, "route"); return nil }
		mocks.NetworkManager.ConfigureDNSFunc = func() error { calls = append(calls, "dns"); return nil }

		// When configuring the network with status messages
		err := ws.ConfigureNetwork(ctx, "", true)

		// Then each step runs in order and sends one status message
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(calls, []string{"guest", "route", "dns"}) {
			t.Errorf("Expected guest, route, dns, got %v", calls)
		}
		want := []string{"network: ready", "dns: local.test @ 10.5.0.53"}
		if got := rec.messages(slog.LevelInfo); !reflect.DeepEqual(got, want) {
			t.Errorf("Expected messages %q, got %q", want, got)
		}
	})

	t.Run("DNSAddressOverrideReplacesConfig", func(t *testing.T) {
		// Given a DNS domain and no DNS address
		mocks, ws, rec, ctx := setup(t)
		_ = mocks.ConfigHandler.Set("dns.domain", "local.test")

		// When configuring the network with an override address
		err := ws.ConfigureNetwork(ctx, "10.5.0.99", true)

		// Then the override is stored and the DNS step uses it
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got := mocks.ConfigHandler.GetString("workstation.dns.address"); got != "10.5.0.99" {
			t.Errorf("Expected workstation.dns.address 10.5.0.99, got %q", got)
		}
		if got := rec.messages(slog.LevelInfo); len(got) != 2 || got[1] != "dns: local.test @ 10.5.0.99" {
			t.Errorf("Expected the DNS message with the override, got %q", got)
		}
	})

	t.Run("SkipsRouteOffColimaAndDNSWithoutDomain", func(t *testing.T) {
		// Given workstation runtime docker-desktop and no DNS domain
		mocks, ws, rec, ctx := setup(t)
		_ = mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		called := false
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { called = true; return nil }
		mocks.NetworkManager.ConfigureDNSFunc = func() error { called = true; return nil }

		// When configuring the network with status messages
		err := ws.ConfigureNetwork(ctx, "", true)

		// Then no step runs and both skips are reported
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if called {
			t.Error("Expected no route or DNS step")
		}
		want := []string{"network: skipped (not colima)", "dns: skipped (domain or address not set)"}
		if got := rec.messages(slog.LevelInfo); !reflect.DeepEqual(got, want) {
			t.Errorf("Expected messages %q, got %q", want, got)
		}
	})

	t.Run("SendsNoMessagesWithoutStatus", func(t *testing.T) {
		// Given colima with a DNS domain and a DNS address
		mocks, ws, rec, ctx := setup(t)
		_ = mocks.ConfigHandler.Set("dns.domain", "local.test")
		_ = mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.53")

		// When configuring the network without status messages
		err := ws.ConfigureNetwork(ctx, "", false)

		// Then no message is sent
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(rec.events) != 0 {
			t.Errorf("Expected no events, got %v", rec.events)
		}
	})

	t.Run("ReturnsErrorWhenHostRouteFails", func(t *testing.T) {
		// Given a host route step that fails
		mocks, ws, _, ctx := setup(t)
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { return fmt.Errorf("route failed") }

		// When configuring the network
		err := ws.ConfigureNetwork(ctx, "", false)

		// Then the error names the host route
		if err == nil || !strings.Contains(err.Error(), "error configuring host route") {
			t.Errorf("Expected a host route error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenDNSFails", func(t *testing.T) {
		// Given a DNS step that fails
		mocks, ws, _, ctx := setup(t)
		_ = mocks.ConfigHandler.Set("dns.domain", "local.test")
		_ = mocks.ConfigHandler.Set("workstation.dns.address", "10.5.0.53")
		mocks.NetworkManager.ConfigureDNSFunc = func() error { return fmt.Errorf("dns failed") }

		// When configuring the network
		err := ws.ConfigureNetwork(ctx, "", false)

		// Then the error names the DNS
		if err == nil || !strings.Contains(err.Error(), "error configuring DNS") {
			t.Errorf("Expected a DNS error, got %v", err)
		}
	})
}

func TestWorkstation_DeferredWork(t *testing.T) {
	t.Run("ReturnsNilWhenNothingAppended", func(t *testing.T) {
		// Given a fresh workstation with no deferred work recorded
		mocks := setupWorkstationMocks(t)
		w := workstation.NewWorkstation(mocks.Runtime)

		// When reading DeferredWork before any append
		items := w.DeferredWork()

		// Then the slice should be nil
		if items != nil {
			t.Errorf("Expected nil deferred work, got %v", items)
		}
	})

	t.Run("ApplyHookAccumulatesItemsInOrder", func(t *testing.T) {
		// Given a deferred setup whose cluster and DNS both need privilege
		mocks := setupWorkstationMocks(t)
		_ = mocks.ConfigHandler.Set("dns.domain", "local.test")
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return true }
		mocks.NetworkManager.NeedsPrivilegeForDNSFunc = func() bool { return true }
		w := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})
		w.DeferHostGuestSetup = true

		// When the apply hook runs for the workstation component
		if _, err := w.MakeApplyHook()(context.Background(), "workstation"); err != nil {
			t.Fatalf("Expected hook to succeed, got %v", err)
		}

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
		w := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return false }))
		mocks.NetworkManager.IsHostRouteInstalledFunc = func() bool { return true }
		if err := w.Down(context.Background()); err != nil {
			t.Fatalf("Expected Down to succeed, got %v", err)
		}
		if len(w.DeferredWork()) == 0 {
			t.Fatal("Expected Down to defer the network revert")
		}

		// When running Up()
		if err := w.Up(context.Background()); err != nil {
			t.Fatalf("Expected Up to succeed, got %v", err)
		}

		// Then the prior deferred work should be cleared
		if items := w.DeferredWork(); items != nil {
			t.Errorf("Expected deferred work to be reset at start of Up, got %v", items)
		}
	})
}

func TestWorkstation_IsProvisioned(t *testing.T) {
	t.Run("ReturnsFalseWhenStateFileMissing", func(t *testing.T) {
		// Given a project root with no .windsor/contexts/<context>/workstation.yaml
		tmpDir := t.TempDir()
		mocks := setupWorkstationMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string { return "test-context" }
		w := workstation.NewWorkstation(mocks.Runtime)

		// When checking the precondition
		provisioned, err := w.IsProvisioned()

		// Then no error and provisioned reports false — operator hasn't run 'windsor up' yet
		if err != nil {
			t.Fatalf("expected nil error when state file is missing, got %v", err)
		}
		if provisioned {
			t.Error("expected provisioned=false when workstation.yaml is missing")
		}
	})

	t.Run("ReturnsTrueWhenStateFilePresent", func(t *testing.T) {
		// Given a project root with the workstation state file present (typical post-up state,
		// preserved across windsor down by PerformCleanup so up→down→up cycles can resume)
		tmpDir := t.TempDir()
		contextDir := filepath.Join(tmpDir, ".windsor", "contexts", "test-context")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "workstation.yaml"), []byte("dns:\n  address: 10.5.0.2\n"), 0644); err != nil {
			t.Fatalf("setup write workstation.yaml: %v", err)
		}
		mocks := setupWorkstationMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextFunc = func() string { return "test-context" }
		w := workstation.NewWorkstation(mocks.Runtime)

		// When checking the precondition
		provisioned, err := w.IsProvisioned()

		// Then provisioned reports true
		if err != nil {
			t.Errorf("expected nil error when state file exists, got %v", err)
		}
		if !provisioned {
			t.Error("expected provisioned=true when workstation.yaml exists")
		}
	})

	t.Run("ReturnsErrorWhenProjectRootCannotBeResolved", func(t *testing.T) {
		// Given a shell that fails to resolve project root (e.g. operator outside any project)
		mocks := setupWorkstationMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("not in a project")
		}
		w := workstation.NewWorkstation(mocks.Runtime)

		// When checking the precondition
		_, err := w.IsProvisioned()

		// Then the IO error surfaces with context
		if err == nil || !strings.Contains(err.Error(), "not in a project") {
			t.Errorf("expected wrapped project-root error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStateFileCannotBeChecked", func(t *testing.T) {
		// Given a Stat shim that fails with an error other than not-exist
		mocks := setupWorkstationMocks(t)
		w := workstation.NewWorkstation(mocks.Runtime)
		s := shims.NewShims()
		s.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		}
		w.SetShims(s)

		// When checking the precondition
		provisioned, err := w.IsProvisioned()

		// Then the error surfaces with context and the workstation is not provisioned
		if err == nil || !strings.Contains(err.Error(), "error checking workstation state") {
			t.Errorf("expected wrapped state check error, got %v", err)
		}
		if provisioned {
			t.Error("expected provisioned to be false")
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := ws.FlushDNS(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := ws.FlushDNS(context.Background())

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})

		// When FlushDNS is called
		err := ws.FlushDNS(context.Background())

		// Then the error is propagated
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("NoopWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation with no network manager
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime)

		// When FlushDNS is called
		err := ws.FlushDNS(context.Background())

		// Then no error occurs
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestWorkstation_PrepareForUp(t *testing.T) {
	t.Run("ClearsDeferHostGuestSetupWhenBlueprintNil", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime)
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
		ws := workstation.NewWorkstation(mocks.Runtime)
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
		ws := workstation.NewWorkstation(mocks.Runtime)
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
		ws := workstation.NewWorkstation(mocks.Runtime)
		blueprint := &v1alpha1.Blueprint{
			TerraformComponents: []v1alpha1.TerraformComponent{{Name: "other", Path: "other"}},
		}

		ws.PrepareForUp(blueprint)

		if ws.DeferHostGuestSetup {
			t.Error("Expected DeferHostGuestSetup false when blueprint has no workstation component")
		}
	})
}

func TestWorkstation_RestoreClusterRoute(t *testing.T) {
	setup := func(t *testing.T, backend string, routeMissing bool) (*WorkstationTestMocks, *workstation.Workstation, *[]string) {
		t.Helper()
		mocks := setupWorkstationMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetTerraformBackendTypeFunc = func() string { return backend }
		var calls []string
		mocks.NetworkManager.NeedsPrivilegeForClusterFunc = func() bool { return routeMissing }
		mocks.NetworkManager.ConfigureGuestFunc = func() error { calls = append(calls, "guest"); return nil }
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { calls = append(calls, "route"); return nil }
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})
		ws.DeferHostGuestSetup = true
		return mocks, ws, &calls
	}

	t.Run("DoesNothingWhenBackendIsNotKubernetes", func(t *testing.T) {
		// Given a local backend and a missing host route
		_, ws, calls := setup(t, "local", true)

		// When the route check runs
		reachable, err := ws.RestoreClusterRoute(context.Background())

		// Then it continues without network calls or deferred work
		if err != nil || !reachable {
			t.Fatalf("Expected reachable and no error, got %v, %v", reachable, err)
		}
		if len(*calls) != 0 || ws.DeferredWork() != nil {
			t.Errorf("Expected no calls and no deferred work, got %v, %v", *calls, ws.DeferredWork())
		}
	})

	t.Run("DoesNothingWhenHostGuestSetupIsNotDeferred", func(t *testing.T) {
		// Given a kubernetes backend and a workstation that configures the network in Up
		_, ws, calls := setup(t, "kubernetes", true)
		ws.DeferHostGuestSetup = false

		// When the route check runs
		reachable, err := ws.RestoreClusterRoute(context.Background())

		// Then it continues without network calls
		if err != nil || !reachable {
			t.Fatalf("Expected reachable and no error, got %v, %v", reachable, err)
		}
		if len(*calls) != 0 {
			t.Errorf("Expected no calls, got %v", *calls)
		}
	})

	t.Run("DoesNothingWhenRouteIsPresent", func(t *testing.T) {
		// Given a kubernetes backend and a host route that is present
		_, ws, calls := setup(t, "kubernetes", false)

		// When the route check runs
		reachable, err := ws.RestoreClusterRoute(context.Background())

		// Then it continues without network calls
		if err != nil || !reachable {
			t.Fatalf("Expected reachable and no error, got %v, %v", reachable, err)
		}
		if len(*calls) != 0 {
			t.Errorf("Expected no calls, got %v", *calls)
		}
	})

	t.Run("AddsRouteWhenElevationNeedsNoPrompt", func(t *testing.T) {
		// Given a missing host route and cached sudo credentials
		_, ws, calls := setup(t, "kubernetes", true)
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))

		// When the route check runs
		reachable, err := ws.RestoreClusterRoute(context.Background())

		// Then it configures the guest and the host route and continues
		if err != nil || !reachable {
			t.Fatalf("Expected reachable and no error, got %v, %v", reachable, err)
		}
		if !reflect.DeepEqual(*calls, []string{"guest", "route"}) {
			t.Errorf("Expected guest then route, got %v", *calls)
		}
		if ws.DeferredWork() != nil {
			t.Errorf("Expected no deferred work, got %v", ws.DeferredWork())
		}
	})

	t.Run("StopsWithConfigureNetworkWhenElevationNeedsPrompt", func(t *testing.T) {
		// Given a missing host route and no cached sudo credentials
		_, ws, calls := setup(t, "kubernetes", true)
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return false }))

		// When the route check runs
		reachable, err := ws.RestoreClusterRoute(context.Background())

		// Then it stops, makes no network calls, and defers 'windsor configure network' as required work
		if err != nil || reachable {
			t.Fatalf("Expected not reachable and no error, got %v, %v", reachable, err)
		}
		if len(*calls) != 0 {
			t.Errorf("Expected no calls, got %v", *calls)
		}
		want := []workstation.DeferredWorkItem{{Required: true, Command: "windsor configure network"}}
		if !reflect.DeepEqual(ws.DeferredWork(), want) {
			t.Errorf("Expected %v, got %v", want, ws.DeferredWork())
		}
	})

	t.Run("ReturnsErrorWhenHostRouteFails", func(t *testing.T) {
		// Given cached sudo credentials and a host route step that fails
		mocks, ws, _ := setup(t, "kubernetes", true)
		t.Cleanup(workstation.SetCanElevateNonInteractivelyForTest(func(_ shell.Shell) bool { return true }))
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error { return fmt.Errorf("route add failed") }

		// When the route check runs
		_, err := ws.RestoreClusterRoute(context.Background())

		// Then the error names the host route step
		if err == nil || !strings.Contains(err.Error(), "error configuring host route") {
			t.Errorf("Expected host route error, got %v", err)
		}
	})
}

func TestWorkstation_RevertNetwork(t *testing.T) {
	t.Run("NoOpWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation without a NetworkManager
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: nil})

		// When reverting the network
		err := ws.RevertNetwork(context.Background(), false)

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		if err := ws.RevertNetwork(context.Background(), false); err != nil {
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		if err := ws.RevertNetwork(context.Background(), false); err != nil {
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		err := ws.RevertNetwork(context.Background(), false)

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When reverting the network
		err := ws.RevertNetwork(context.Background(), false)

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then both cluster-privilege rows appear with config values interpolated into Detail
		want := []workstation.NetworkChange{
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: mocks.NetworkManager})

		// When inspecting pending changes
		got := ws.PendingNetworkChanges()

		// Then the resolver entry appears with domain + address interpolated into Detail
		want := workstation.NetworkChange{Kind: "dns-resolver", Detail: "*.local.test -> 10.5.0.2"}
		if len(got) != 1 || got[0] != want {
			t.Errorf("expected [%+v], got %+v", want, got)
		}
	})

	t.Run("ReturnsNilWhenNetworkManagerNil", func(t *testing.T) {
		// Given a workstation with no NetworkManager (atypical, but the helper must not panic)
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{NetworkManager: nil})

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
		ws := workstation.NewWorkstation(mocks.Runtime)
		ws.DeferHostGuestSetup = false

		hook := ws.MakeApplyHook()

		if hook != nil {
			t.Error("Expected nil hook when DeferHostGuestSetup is false")
		}
	})

	t.Run("CallbackIgnoresNonWorkstationComponent", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
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

		_, err := hook(context.Background(), "other-component")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires for the workstation component
		_, err := ws.MakeApplyHook()(context.Background(), "workstation")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires for the workstation component
		halt, err := ws.MakeApplyHook()(context.Background(), "workstation")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()(context.Background(), "workstation")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()(context.Background(), "workstation")

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

	t.Run("CallbackSurfacesGetTerraformOutputsError", func(t *testing.T) {
		// Given a terraform provider that fails to read workstation outputs (e.g. corrupted
		// state file). The hook cannot reason about DNS state without the outputs and must
		// surface the failure rather than silently proceeding with stale config.
		mocks := setupWorkstationMocks(t)
		mockTF := &terraform.MockTerraformProvider{}
		mockTF.GetTerraformOutputsFunc = func(_ string) (map[string]any, error) {
			return nil, fmt.Errorf("tf state unreadable")
		}
		mocks.Runtime.TerraformProvider = mockTF
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()(context.Background(), "workstation")

		// Then the error bubbles with context, no halt is signaled, and no deferred work was
		// recorded (the hook bailed before the privilege probes)
		if err == nil || !strings.Contains(err.Error(), "tf state unreadable") {
			t.Errorf("expected wrapped tf-outputs error, got: %v", err)
		}
		if halt {
			t.Error("expected halt=false on error path")
		}
		if items := ws.DeferredWork(); len(items) != 0 {
			t.Errorf("expected no deferred work when hook errors early, got %v", items)
		}
	})

	t.Run("CallbackSurfacesConfigSetError", func(t *testing.T) {
		// Given a terraform provider that returns a dns_address, but the config handler refuses
		// to persist it (read-only state, IO error, etc.). The hook must not call WriteState
		// with a stale value masquerading as fresh — surface the error instead.
		mocks := setupWorkstationMocks(t)
		mockTF := &terraform.MockTerraformProvider{}
		mockTF.GetTerraformOutputsFunc = func(_ string) (map[string]any, error) {
			return map[string]any{"dns_address": "10.5.0.2"}, nil
		}
		mocks.Runtime.TerraformProvider = mockTF
		mocks.ConfigHandler.(*config.MockConfigHandler).SetFunc = func(key string, _ any) error {
			if key == "workstation.dns.address" {
				return fmt.Errorf("config locked")
			}
			return nil
		}
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		// When the hook fires
		halt, err := ws.MakeApplyHook()(context.Background(), "workstation")

		// Then the Set failure bubbles with context and halt is not signaled
		if err == nil || !strings.Contains(err.Error(), "config locked") {
			t.Errorf("expected wrapped Set error, got: %v", err)
		}
		if halt {
			t.Error("expected halt=false on error path")
		}
	})
}

func TestWorkstation_MakePostApplyHook(t *testing.T) {
	t.Run("ReturnsNilWhenDeferHostGuestSetupFalse", func(t *testing.T) {
		mocks := setupWorkstationMocks(t)
		ws := workstation.NewWorkstation(mocks.Runtime)
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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook(context.Background(), "other-component")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook(context.Background(), "workstation")

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
		ws := workstation.NewWorkstation(mocks.Runtime, &workstation.Workstation{
			NetworkManager: mocks.NetworkManager,
		})
		ws.DeferHostGuestSetup = true

		hook := ws.MakePostApplyHook()
		if hook == nil {
			t.Fatal("Expected non-nil hook when DeferHostGuestSetup is true")
		}

		err := hook(context.Background(), "workstation")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if flushCalled {
			t.Error("Expected FlushDNS not to be called when DNS was not changed")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// Emit stores e.
func (r *recorder) Emit(_ context.Context, e presenter.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// messages returns the text of each message event at level, in order.
func (r *recorder) messages(level slog.Level) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == presenter.KindMessage && e.Level == level {
			out = append(out, e.Message)
		}
	}
	return out
}
