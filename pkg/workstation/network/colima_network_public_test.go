package network_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/workstation/network"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupColimaNetwork returns a fixture with the guest address 192.168.5.2 and a ColimaNetworkManager.
// The ls /sys/class/net command in the VM returns bridges.
func setupColimaNetwork(t *testing.T, bridges string) (*fixture, *network.ColimaNetworkManager) {
	t.Helper()
	f := newFixture(t)
	f.values["workstation.address"] = "192.168.5.2"
	record := f.shell.ExecSilentWithTimeoutFunc
	f.shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		out, err := record(command, args, timeout)
		if strings.HasSuffix(strings.Join(args, " "), "ls /sys/class/net") {
			return bridges, nil
		}
		return out, err
	}
	return f, network.NewColimaNetworkManager(f.runtime, f.provider)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewColimaNetworkManager(t *testing.T) {
	t.Run("PanicsWhenRuntimeIsNil", func(t *testing.T) {
		// Given no runtime
		f := newFixture(t)
		defer func() {
			// Then the constructor panics
			if recover() == nil {
				t.Error("Expected a panic")
			}
		}()

		// When creating the manager
		network.NewColimaNetworkManager(nil, f.provider)
	})

	t.Run("PanicsWhenProviderIsNil", func(t *testing.T) {
		// Given no interface provider
		f := newFixture(t)
		defer func() {
			// Then the constructor panics
			if recover() == nil {
				t.Error("Expected a panic")
			}
		}()

		// When creating the manager
		network.NewColimaNetworkManager(f.runtime, nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("DoesNothingWithoutGuestAddress", func(t *testing.T) {
		// Given no guest address
		f, manager := setupColimaNetwork(t, "br-abc\n")
		delete(f.values, "workstation.address")

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then no command runs
		if err != nil || len(f.commands) != 0 {
			t.Errorf("Expected no error and no commands, got %v and %v", err, f.commands)
		}
	})

	t.Run("ForwardsHostTrafficToTheDockerBridge", func(t *testing.T) {
		// Given a VM with the Docker bridge br-abc and no forwarding rule
		f, manager := setupColimaNetwork(t, "eth0\ncol0\nbr-abc\n")
		record := f.shell.ExecSilentWithTimeoutFunc
		f.shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			out, err := record(command, args, timeout)
			if strings.Contains(strings.Join(args, " "), "iptables -t filter -C") {
				return "", errors.New("exit status 1")
			}
			return out, err
		}

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then IP forwarding is on and a rule forwards the host IP to the default CIDR on br-abc
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(f.commandsContaining("net.ipv4.ip_forward=1")) != 1 {
			t.Errorf("Expected IP forwarding to be enabled, got %v", f.commands)
		}
		added := f.commandsContaining("iptables -t filter -A FORWARD -i col0 -o br-abc -s 192.168.5.1 -d 10.5.0.0/16 -j ACCEPT")
		if len(added) != 1 || !strings.HasPrefix(added[0], "colima ssh --profile windsor-local --") {
			t.Errorf("Expected one rule added through colima ssh, got %v", f.commands)
		}
	})

	t.Run("KeepsAnExistingRule", func(t *testing.T) {
		// Given a VM where the forwarding rule exists
		f, manager := setupColimaNetwork(t, "br-abc\n")

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then no rule is added
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if added := f.commandsContaining("iptables -t filter -A"); len(added) != 0 {
			t.Errorf("Expected no rule added, got %v", added)
		}
	})

	t.Run("ReturnsErrorWithoutDockerBridge", func(t *testing.T) {
		// Given a VM without a Docker bridge
		_, manager := setupColimaNetwork(t, "eth0\ncol0\n")

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then the error names the missing bridge
		if err == nil || !strings.Contains(err.Error(), "no docker bridge interface found") {
			t.Errorf("Expected a missing bridge error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenHostIPIsNotInTheGuestSubnet", func(t *testing.T) {
		// Given a guest address outside every host subnet
		f, manager := setupColimaNetwork(t, "br-abc\n")
		f.values["workstation.address"] = "172.30.0.2"

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then the error names the host IP
		if err == nil || !strings.Contains(err.Error(), "error getting host IP") {
			t.Errorf("Expected a host IP error, got %v", err)
		}
	})

	t.Run("SetsTheIncusGatewayOnIncus", func(t *testing.T) {
		// Given platform incus and the CIDR 10.20.0.0/24
		f, manager := setupColimaNetwork(t, "")
		f.values["platform"] = "incus"
		f.values["network.cidr_block"] = "10.20.0.0/24"

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then incusbr0 gets the first address of the CIDR, and the rule targets incusbr0
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(f.commandsContaining("incus network set incusbr0 ipv4.address=10.20.0.1/24")) != 1 {
			t.Errorf("Expected the incus gateway to be set, got %v", f.commands)
		}
		if len(f.commandsContaining("-o incusbr0 -s 192.168.5.1 -d 10.20.0.0/24")) == 0 {
			t.Errorf("Expected a rule for incusbr0, got %v", f.commands)
		}
	})
}

func TestColimaNetworkManager_RevertGuest(t *testing.T) {
	t.Run("ReturnsNilWhenTheVMIsGone", func(t *testing.T) {
		// Given a VM that does not answer
		f, manager := setupColimaNetwork(t, "")
		f.shell.ExecSilentWithTimeoutFunc = func(string, []string, time.Duration) (string, error) {
			return "", errors.New("profile not found")
		}

		// When reverting the guest
		err := manager.RevertGuest()

		// Then nothing fails
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DeletesTheRuleForTheDockerBridge", func(t *testing.T) {
		// Given a VM with the Docker bridge br-abc
		f, manager := setupColimaNetwork(t, "br-abc\n")

		// When reverting the guest
		err := manager.RevertGuest()

		// Then the forwarding rule for br-abc is deleted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(f.commandsContaining("iptables -t filter -D FORWARD -i col0 -o br-abc -s 192.168.5.1 -d 10.5.0.0/16 -j ACCEPT")) != 1 {
			t.Errorf("Expected one rule deleted, got %v", f.commands)
		}
	})

	t.Run("ReturnsErrorWhenDeleteFails", func(t *testing.T) {
		// Given a rule delete that fails with an error that is not exit code 1
		f, manager := setupColimaNetwork(t, "br-abc\n")
		record := f.shell.ExecSilentWithTimeoutFunc
		f.shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			out, err := record(command, args, timeout)
			if strings.Contains(strings.Join(args, " "), "iptables -t filter -D") {
				return "", errors.New("ssh failed")
			}
			return out, err
		}

		// When reverting the guest
		err := manager.RevertGuest()

		// Then the error names the rule
		if err == nil || !strings.Contains(err.Error(), "error removing iptables rule") {
			t.Errorf("Expected a rule removal error, got %v", err)
		}
	})
}
