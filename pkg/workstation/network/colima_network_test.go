package network

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaNetworkManager_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewColimaNetworkManager(mocks.Injector)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a network manager with invalid secure shell
		manager, mocks := setup(t)
		mocks.Injector.Register("secureShell", "invalid")

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about secure shell type
		if err.Error() != "resolved secure shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		// Given a network manager with invalid SSH client
		manager, mocks := setup(t)
		mocks.Injector.Register("sshClient", "invalid")

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about SSH client type
		if err.Error() != "resolved ssh client instance is not of type ssh.Client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkInterfaceProvider", func(t *testing.T) {
		// Given a network manager with invalid network interface provider
		manager, mocks := setup(t)
		mocks.Injector.Register("networkInterfaceProvider", "invalid")

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		}

		// And the error should be about network interface provider type
		if err.Error() != "failed to resolve network interface provider" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})
}

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewColimaNetworkManager(mocks.Injector)
		manager.shims = mocks.Shims
		manager.Initialize([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// And configuring the guest
		err := manager.ConfigureGuest()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "")

		// And configuring the guest
		err := manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Given a network manager with no guest IP configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.address", "")

		// And configuring the guest
		err := manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingSSHConfig", func(t *testing.T) {
		// Given a network manager with SSH config error
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "ssh-config" {
				return "", fmt.Errorf("mock error getting SSH config")
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing VM SSH config command: mock error getting SSH config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingSSHClient", func(t *testing.T) {
		// Given a network manager with SSH client error
		manager, mocks := setup(t)
		mocks.SSHClient.SetClientConfigFileFunc = func(config string, contextName string) error {
			return fmt.Errorf("mock error setting SSH client config")
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error setting SSH client config: mock error setting SSH client config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Given a network manager with interface listing error
		manager, mocks := setup(t)
		mocks.SecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing command to list network interfaces: mock error listing interfaces"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		// Given a network manager with no docker bridge interface
		manager, mocks := setup(t)
		mocks.SecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo\nwlan0", nil // No "br-" interface
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error: no docker bridge interface found"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingIptablesRule", func(t *testing.T) {
		// Given a network manager with iptables rule error
		manager, mocks := setup(t)
		mocks.SecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-1234\neth0\nlo\nwlan0", nil // Include a "br-" interface
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("Bad rule") // Simulate that the rule doesn't exist
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-A" {
				return "", fmt.Errorf("mock error setting iptables rule")
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error setting iptables rule") {
			t.Fatalf("expected error to contain 'error setting iptables rule', got %q", err.Error())
		}
	})

	t.Run("ErrorFindingHostIP", func(t *testing.T) {
		// Given a network manager with host IP error
		manager, mocks := setup(t)
		mocks.NetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			// Return an empty list of addresses to simulate no matching subnet
			return []net.Addr{}, nil
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to find host IP in the same subnet as guest IP") {
			t.Fatalf("expected error to contain 'failed to find host IP in the same subnet as guest IP', got %q", err.Error())
		}
	})

	t.Run("ErrorCheckingIptablesRule", func(t *testing.T) {
		// Given a network manager with iptables rule check error
		manager, mocks := setup(t)
		originalExecSilentFunc := mocks.SecureShell.ExecSilentFunc
		mocks.SecureShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("unexpected error checking iptables rule")
			}
			return originalExecSilentFunc(command, args...)
		}

		// When initializing the network manager
		err := manager.Initialize([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking iptables rule") {
			t.Fatalf("expected error to contain 'error checking iptables rule', got %q", err.Error())
		}
	})
}

func TestColimaNetworkManager_getHostIP(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		manager := NewColimaNetworkManager(mocks.Injector)
		manager.Initialize([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// And the host IP should be correct
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("SuccessWithIpAddr", func(t *testing.T) {
		// Given a network manager with IPAddr type
		manager, mocks := setup(t)

		// And mocking networkInterfaceProvider to return IPAddr
		mocks.NetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{
				&net.IPAddr{IP: net.ParseIP("192.168.1.1")},
			}, nil
		}

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during getHostIP, got %v", err)
		}

		// And the host IP should be correct
		expectedHostIP := "192.168.1.1"
		if hostIP != expectedHostIP {
			t.Fatalf("expected host IP %v, got %v", expectedHostIP, hostIP)
		}
	})

	t.Run("NoGuestAddressSet", func(t *testing.T) {
		// Given a network manager with no guest address
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.address", "")

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "guest IP is not configured"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorParsingGuestIP", func(t *testing.T) {
		// Given a network manager with invalid guest IP
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.address", "invalid_ip_address")

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "invalid guest IP address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaces", func(t *testing.T) {
		// Given a network manager with network interfaces error
		manager, mocks := setup(t)

		mocks.NetworkInterfaceProvider.InterfacesFunc = func() ([]net.Interface, error) {
			return nil, fmt.Errorf("mock error getting network interfaces")
		}

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "failed to get network interfaces: mock error getting network interfaces"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorGettingNetworkInterfaceAddresses", func(t *testing.T) {
		// Given a network manager with interface addresses error
		manager, mocks := setup(t)

		mocks.NetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			return nil, fmt.Errorf("mock error getting network interface addresses")
		}

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should contain the expected text
		if !strings.Contains(err.Error(), "mock error getting network interface addresses") {
			t.Fatalf("expected error message to contain %q, got %q", "mock error getting network interface addresses", err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})

	t.Run("ErrorFindingHostIPInSameSubnet", func(t *testing.T) {
		// Given a network manager with no matching subnet
		manager, mocks := setup(t)

		// And mocking network interface provider to return no matching subnet
		mocks.NetworkInterfaceProvider.InterfacesFunc = func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0"},
			}, nil
		}
		mocks.NetworkInterfaceProvider.InterfaceAddrsFunc = func(iface net.Interface) ([]net.Addr, error) {
			if iface.Name == "eth0" {
				return []net.Addr{
					&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(24, 32)},
				}, nil
			}
			return nil, fmt.Errorf("no addresses found for interface %s", iface.Name)
		}

		// And getting the host IP
		hostIP, err := manager.getHostIP()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error during getHostIP, got none")
		}

		// And the error message should be correct
		expectedErrorMessage := "failed to find host IP in the same subnet as guest IP"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message %q, got %q", expectedErrorMessage, err.Error())
		}

		// And the host IP should be empty
		if hostIP != "" {
			t.Fatalf("expected empty host IP, got %v", hostIP)
		}
	})
}
