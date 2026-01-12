package network

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaNetworkManager_AssignIPs(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewColimaNetworkManager(mocks.Runtime, mocks.NetworkInterfaceProvider)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// When assigning IPs to services
		err := manager.AssignIPs([]services.Service{})

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error during AssignIPs, got %v", err)
		}

		// And services should be set
		if manager.services == nil {
			t.Fatalf("expected services to be set")
		}
	})
}

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewColimaNetworkManager(mocks.Runtime, mocks.NetworkInterfaceProvider)
		manager.shims = mocks.Shims
		manager.AssignIPs([]services.Service{})
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

		// Then no error should occur (default CIDR is used automatically)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Given a network manager with no guest IP configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.address", "")

		// And configuring the guest
		err := manager.ConfigureGuest()

		// Then no error should occur (ConfigureGuest returns early when guest IP is empty)
		if err != nil {
			t.Fatalf("expected no error when guest IP is not configured, got %v", err)
		}
	})


	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Given a network manager with interface listing error
		manager, mocks := setup(t)
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" && args[6] == "ls /sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.AssignIPs([]services.Service{})
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
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 3 && args[0] == "ssh" && args[2] == "--" {
				cmd := args[4]
				if cmd == "sh" && len(args) >= 6 && args[5] == "-c" {
					actualCmd := args[6]
					if actualCmd == "ls /sys/class/net" {
						return "eth0\nlo\nwlan0", nil // No "br-" interface
					}
				}
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.AssignIPs([]services.Service{})
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
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if actualCmd == "ls /sys/class/net" {
					return "br-1234\neth0\nlo\nwlan0", nil // Include a "br-" interface
				}
				if strings.Contains(actualCmd, "iptables") && strings.Contains(actualCmd, "-C") {
					return "", fmt.Errorf("Bad rule") // Simulate that the rule doesn't exist
				}
				if strings.Contains(actualCmd, "iptables") && strings.Contains(actualCmd, "-A") {
					return "", fmt.Errorf("mock error setting iptables rule")
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.AssignIPs([]services.Service{})
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
		err := manager.AssignIPs([]services.Service{})
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
		originalFunc := mocks.Shell.ExecSilentWithTimeoutFunc
		checkErrorReturned := false
		mocks.Shell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			if command == "colima" && len(args) >= 7 && args[0] == "ssh" && args[3] == "--" && args[4] == "sh" && args[5] == "-c" {
				actualCmd := args[6]
				if actualCmd == "ls /sys/class/net" {
					return "br-1234\neth0\nlo\nwlan0", nil
				}
				if strings.Contains(actualCmd, "iptables") && strings.Contains(actualCmd, "-C") {
					checkErrorReturned = true
					return "", fmt.Errorf("unexpected error checking iptables rule")
				}
				if strings.Contains(actualCmd, "sysctl") {
					return "", nil
				}
			}
			if originalFunc != nil {
				return originalFunc(command, args, timeout)
			}
			return "", nil
		}

		// When initializing the network manager
		err := manager.AssignIPs([]services.Service{})
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And configuring the guest
		err = manager.ConfigureGuest()

		// Then the check error should have been encountered (even though it's handled gracefully)
		if !checkErrorReturned {
			t.Fatalf("expected iptables check error to be returned, but it was not")
		}
		// The error is handled gracefully by trying to add the rule, so no error should be returned
		if err != nil {
			t.Fatalf("expected no error (check error is handled gracefully), got %v", err)
		}
	})
}

func TestColimaNetworkManager_getHostIP(t *testing.T) {
	setup := func(t *testing.T) (*ColimaNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewColimaNetworkManager(mocks.Runtime, mocks.NetworkInterfaceProvider)
		manager.AssignIPs([]services.Service{})
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
