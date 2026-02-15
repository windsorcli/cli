//go:build linux
// +build linux

package network

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLinuxNetworkManager_ConfigureHostRoute(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		manager.AssignIPs([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Given a network manager with no CIDR configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingRouteTable", func(t *testing.T) {
		// Given a network manager with route check error
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" {
				return "", fmt.Errorf("mock error checking route table")
			}
			return "", nil
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to check if route exists: mock error checking route table"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Given a network manager with no guest IP configured
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("vm.address", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RouteExists", func(t *testing.T) {
		// Given a network manager with existing route
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				return "192.168.5.0/24 via 192.168.1.2 dev eth0", nil
			}
			return "", nil
		}
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("vm.address", "192.168.1.2")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// T	hen no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RouteExistsWithGuestIP", func(t *testing.T) {
		// Given a network manager with existing route matching guest IP
		manager, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "show" && args[2] == "192.168.5.0/24" {
				return "192.168.5.0/24 via 192.168.5.100 dev eth0", nil
			}
			return "", nil
		}
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("vm.address", "192.168.5.100")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Given a network manager with route addition error
		manager, mocks := setup(t)
		mocks.Shell.ExecSudoFunc = func(message, command string, args ...string) (string, error) {
			if command == "ip" && args[0] == "route" && args[1] == "add" {
				return "mock output", fmt.Errorf("mock error")
			}
			return "", nil
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add route: mock error, output: mock output"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestLinuxNetworkManager_ConfigureDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		manager.AssignIPs([]services.Service{})
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		// Given a network manager in localhost mode
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("dns.domain", "example.com")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And capturing the content
		var capturedContent []byte
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			if capturedContent != nil {
				return capturedContent, nil
			}
			return nil, os.ErrNotExist
		}

		// And capturing the drop-in file content
		mocks.Shell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				cmdStr := args[1]
				if strings.Contains(cmdStr, "echo '") && strings.Contains(cmdStr, "' | sudo tee") {
					start := strings.Index(cmdStr, "echo '") + 6
					end := strings.Index(cmdStr, "' | sudo tee")
					if start < end {
						content := cmdStr[start:end]
						capturedContent = []byte(content)
					}
				}
				return "", nil
			}
			return "", nil
		}

		// And configuring DNS
		err = manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the drop-in file should contain localhost
		expectedContent := "[Resolve]\nDNS=127.0.0.1\n"
		if string(capturedContent) != expectedContent {
			t.Errorf("expected drop-in file content to be %q, got %q", expectedContent, string(capturedContent))
		}
	})

	t.Run("DomainNotConfigured", func(t *testing.T) {
		// Given a network manager with no DNS domain
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		// Given a network manager with no DNS address
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "")
		mocks.ConfigHandler.Set("vm.driver", "colima")

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SystemdResolvedNotInUse", func(t *testing.T) {
		// Given a network manager with systemd-resolved not in use
		manager, mocks := setup(t)

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "/etc/resolv.conf", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "systemd-resolved is not in use. Please configure DNS manually or use a compatible system"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DropInFileAlreadyExistsWithCorrectContent", func(t *testing.T) {
		// Given a network manager with existing drop-in file
		manager, mocks := setup(t)

		// And mocking the drop-in file content
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return []byte("[Resolve]\nDNS=1.2.3.4\n"), nil
		}

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorCreatingDropInDirectory", func(t *testing.T) {
		// Given a network manager with drop-in directory creation error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And mocking drop-in directory creation error
		mocks.Shell.ExecSudoFunc = func(message, command string, args ...string) (string, error) {
			if command == "mkdir" {
				return "", fmt.Errorf("mock error creating directory")
			}
			return "", nil
		}

		// And mocking file not existing
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to create drop-in directory: mock error creating directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingDNSConfig", func(t *testing.T) {
		// Given a network manager with DNS config writing error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And mocking file not existing
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And mocking DNS config writing error
		mocks.Shell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "bash" && args[0] == "-c" {
				return "", fmt.Errorf("mock error writing config")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to write DNS configuration: mock error writing config"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRestartingSystemdResolved", func(t *testing.T) {
		// Given a network manager with systemd-resolved restart error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And mocking file not existing
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And mocking systemd-resolved restart error
		mocks.Shell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "systemctl" && args[0] == "restart" {
				return "", fmt.Errorf("mock error restarting service")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to restart systemd-resolved: mock error restarting service"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
