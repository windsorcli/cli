//go:build darwin

package network_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/network"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupDarwinDNS returns a fixture for the domain local.test and the DNS address 10.5.0.53, and a
// BaseNetworkManager whose resolver file holds resolver. An empty resolver means the file is absent.
func setupDarwinDNS(t *testing.T, resolver string) (*fixture, *network.BaseNetworkManager) {
	t.Helper()
	f := newFixture(t)
	f.values["dns.domain"] = "local.test"
	f.values["workstation.dns.address"] = "10.5.0.53"
	f.shims.ReadFile = func(name string) ([]byte, error) {
		if name == "/etc/resolver/local.test" && resolver != "" {
			return []byte(resolver), nil
		}
		return nil, os.ErrNotExist
	}
	f.shims.Stat = func(string) (os.FileInfo, error) { return nil, nil }
	f.shims.MkdirTemp = func(string, string) (string, error) { return t.TempDir(), nil }
	manager := network.NewBaseNetworkManager(f.runtime)
	manager.SetShims(f.shims)
	return f, manager
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseNetworkManager_ConfigureDNS_Darwin(t *testing.T) {
	t.Run("InstallsTheResolverFile", func(t *testing.T) {
		// Given no resolver file for the domain
		f, manager := setupDarwinDNS(t, "")

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then the resolver file moves into place in one step and the DNS changed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		moved := f.commandsContaining("mv ")
		if len(moved) != 1 || !strings.HasSuffix(moved[0], " /etc/resolver/local.test") {
			t.Errorf("Expected one move to /etc/resolver/local.test, got %v", f.commands)
		}
		if len(f.commandsContaining("chmod 0644 /etc/resolver/local.test")) != 1 {
			t.Errorf("Expected the resolver file mode to be set, got %v", f.commands)
		}
		if !manager.DNSChanged() {
			t.Error("Expected DNSChanged to be true")
		}
		wantSteps := []string{"applying:Configuring DNS", "applied:Configuring DNS"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})

	t.Run("SkipsAMatchingResolverFile", func(t *testing.T) {
		// Given a resolver file with the DNS address
		f, manager := setupDarwinDNS(t, "nameserver 10.5.0.53\n")

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then nothing runs and the DNS did not change
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(f.commands) != 0 || manager.DNSChanged() || len(f.recorder.steps()) != 0 {
			t.Errorf("Expected no work, got commands %v and steps %v", f.commands, f.recorder.steps())
		}
	})

	t.Run("ReturnsErrorWhenTheMoveFails", func(t *testing.T) {
		// Given a sudo move that fails
		f, manager := setupDarwinDNS(t, "nameserver 10.0.0.1\n")
		f.shell.ExecSudoFunc = func(_ string, command string, _ ...string) (string, error) {
			if command == "mv" {
				return "", errors.New("permission denied")
			}
			return "", nil
		}

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then the error names the resolver file and the step fails
		if err == nil || !strings.Contains(err.Error(), "Error installing resolver file") {
			t.Fatalf("Expected a resolver file error, got %v", err)
		}
		wantSteps := []string{"applying:Configuring DNS", "failed:Configuring DNS"}
		if !reflect.DeepEqual(f.recorder.steps(), wantSteps) {
			t.Errorf("Expected steps %v, got %v", wantSteps, f.recorder.steps())
		}
	})
}

func TestBaseNetworkManager_ResolverProbes_Darwin(t *testing.T) {
	t.Run("ReportsAMatchingResolverAsInstalled", func(t *testing.T) {
		// Given a resolver file with the DNS address
		_, manager := setupDarwinDNS(t, "nameserver 10.5.0.53\n")

		// When probing the resolver
		needs, installed := manager.NeedsPrivilegeForDNS(), manager.IsResolverInstalled()

		// Then the resolver needs no change and counts as installed
		if needs || !installed {
			t.Errorf("Expected false and true, got %v and %v", needs, installed)
		}
	})

	t.Run("ReportsAStaleResolverAsNotInstalled", func(t *testing.T) {
		// Given a resolver file with a different address
		_, manager := setupDarwinDNS(t, "nameserver 10.0.0.1\n")

		// When probing the resolver
		needs, installed := manager.NeedsPrivilegeForDNS(), manager.IsResolverInstalled()

		// Then the resolver needs a change and does not count as installed
		if !needs || installed {
			t.Errorf("Expected true and false, got %v and %v", needs, installed)
		}
	})
}

func TestBaseNetworkManager_RevertDNS_Darwin(t *testing.T) {
	t.Run("RemovesTheResolverFile", func(t *testing.T) {
		// Given a DNS domain
		f, manager := setupDarwinDNS(t, "")

		// When reverting the DNS
		err := manager.RevertDNS()

		// Then the resolver file is removed with rm -f
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(f.commands, []string{"rm -f /etc/resolver/local.test"}) {
			t.Errorf("Expected rm -f /etc/resolver/local.test, got %v", f.commands)
		}
	})

	t.Run("DoesNothingWithoutDomain", func(t *testing.T) {
		// Given no DNS domain
		f, manager := setupDarwinDNS(t, "")
		delete(f.values, "dns.domain")

		// When reverting the DNS
		err := manager.RevertDNS()

		// Then nothing runs
		if err != nil || len(f.commands) != 0 {
			t.Errorf("Expected no error and no commands, got %v and %v", err, f.commands)
		}
	})
}
