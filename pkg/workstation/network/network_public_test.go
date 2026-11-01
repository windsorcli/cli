package network_test

import (
	"testing"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/workstation/network"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBaseNetworkManager(t *testing.T) {
	f := newFixture(t)
	cases := map[string]*runtime.Runtime{
		"PanicsWhenRuntimeIsNil":       nil,
		"PanicsWhenShellIsNil":         {ConfigHandler: f.config},
		"PanicsWhenConfigHandlerIsNil": {Shell: f.shell},
	}
	for name, rt := range cases {
		t.Run(name, func(t *testing.T) {
			// Given an incomplete runtime
			defer func() {
				// Then the constructor panics
				if recover() == nil {
					t.Error("Expected a panic")
				}
			}()

			// When creating the manager
			network.NewBaseNetworkManager(rt)
		})
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseNetworkManager_Guest(t *testing.T) {
	t.Run("ConfigureAndRevertDoNothing", func(t *testing.T) {
		// Given a base manager
		f := newFixture(t)
		manager := network.NewBaseNetworkManager(f.runtime)

		// When configuring and reverting the guest
		configureErr := manager.ConfigureGuest()
		revertErr := manager.RevertGuest()

		// Then nothing runs and DNS has not changed
		if configureErr != nil || revertErr != nil {
			t.Errorf("Expected no errors, got %v and %v", configureErr, revertErr)
		}
		if len(f.commands) != 0 || manager.DNSChanged() {
			t.Errorf("Expected no commands and no DNS change, got %v and %v", f.commands, manager.DNSChanged())
		}
	})
}

func TestBaseNetworkManager_Probes(t *testing.T) {
	t.Run("ClusterNeedsNoPrivilegeOffColima", func(t *testing.T) {
		// Given workstation runtime docker-desktop with a guest address
		f := newFixture(t)
		f.values["workstation.runtime"] = "docker-desktop"
		f.values["workstation.address"] = "192.168.5.2"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When probing the host route
		needs, installed := manager.NeedsPrivilegeForCluster(), manager.IsHostRouteInstalled()

		// Then neither probe reports a route and no command runs
		if needs || installed {
			t.Errorf("Expected false and false, got %v and %v", needs, installed)
		}
		if len(f.commands) != 0 {
			t.Errorf("Expected no commands, got %v", f.commands)
		}
	})

	t.Run("ClusterNeedsNoPrivilegeWithoutGuestAddress", func(t *testing.T) {
		// Given workstation runtime colima without a guest address
		f := newFixture(t)
		f.values["workstation.runtime"] = "colima"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When probing the host route
		needs, installed := manager.NeedsPrivilegeForCluster(), manager.IsHostRouteInstalled()

		// Then neither probe reports a route
		if needs || installed {
			t.Errorf("Expected false and false, got %v and %v", needs, installed)
		}
	})

	t.Run("DNSNeedsNoPrivilegeWithoutDomain", func(t *testing.T) {
		// Given a DNS address without a DNS domain
		f := newFixture(t)
		f.values["workstation.dns.address"] = "10.5.0.53"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When probing the resolver
		needs, installed := manager.NeedsPrivilegeForDNS(), manager.IsResolverInstalled()

		// Then neither probe reports a resolver
		if needs || installed {
			t.Errorf("Expected false and false, got %v and %v", needs, installed)
		}
	})

	t.Run("DNSNeedsNoPrivilegeWithoutResolverIP", func(t *testing.T) {
		// Given a DNS domain on colima without a DNS address
		f := newFixture(t)
		f.values["dns.domain"] = "local.test"
		f.values["workstation.runtime"] = "colima"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When probing the resolver
		needs, installed := manager.NeedsPrivilegeForDNS(), manager.IsResolverInstalled()

		// Then neither probe reports a resolver
		if needs || installed {
			t.Errorf("Expected false and false, got %v and %v", needs, installed)
		}
	})
}

func TestBaseNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("ReturnsHintWhenDomainIsNotSet", func(t *testing.T) {
		// Given no DNS domain
		f := newFixture(t)
		manager := network.NewBaseNetworkManager(f.runtime)

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then the error has a remediation and nothing runs
		we := werror.Find(err)
		if we == nil || we.Message != "DNS domain is not configured" || we.Remediation == "" {
			t.Fatalf("Expected a DNS domain error with a remediation, got %v", err)
		}
		if len(f.commands) != 0 || manager.DNSChanged() {
			t.Errorf("Expected no commands and no DNS change, got %v", f.commands)
		}
	})

	t.Run("ReturnsHintWhenAddressIsNotSet", func(t *testing.T) {
		// Given a DNS domain on colima without a DNS address
		f := newFixture(t)
		f.values["dns.domain"] = "local.test"
		f.values["workstation.runtime"] = "colima"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then the error has a remediation
		we := werror.Find(err)
		if we == nil || we.Message != "DNS address is not configured" || we.Remediation == "" {
			t.Fatalf("Expected a DNS address error with a remediation, got %v", err)
		}
	})

	t.Run("RejectsADomainThatEscapesItsDirectory", func(t *testing.T) {
		// Given a DNS domain with an empty label
		f := newFixture(t)
		f.values["dns.domain"] = ".."
		f.values["workstation.dns.address"] = "10.5.0.53"
		manager := network.NewBaseNetworkManager(f.runtime)

		// When configuring the DNS
		err := manager.ConfigureDNS(f.ctx)

		// Then the domain is rejected and nothing runs
		if err == nil {
			t.Fatal("Expected an invalid domain error")
		}
		if len(f.commands) != 0 {
			t.Errorf("Expected no commands, got %v", f.commands)
		}
	})
}
