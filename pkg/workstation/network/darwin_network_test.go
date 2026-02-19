//go:build darwin
// +build darwin

package network

import (
	"fmt"
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDarwinNetworkManager_ConfigureHostRoute(t *testing.T) {
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
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

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
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

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
		// Given a network manager with no guest address in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest address is required"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RouteAlreadyExists", func(t *testing.T) {
		// Given a network manager with existing route
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		originalExecSilentFunc := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "route" && len(args) >= 3 && args[0] == "-n" && args[1] == "get" {
				return "   route to: 192.168.1.0\ndestination: 192.168.1.0\n    gateway: 192.168.1.10\n  interface: en0", nil
			}
			if originalExecSilentFunc != nil {
				return originalExecSilentFunc(command, args...)
			}
			return "", fmt.Errorf("mock error")
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("CheckRouteExistsError", func(t *testing.T) {
		// Given a network manager with route check error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		originalExecSilentFunc := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "route" && len(args) >= 3 && args[0] == "-n" && args[1] == "get" {
				return "", fmt.Errorf("mock error")
			}
			if originalExecSilentFunc != nil {
				return originalExecSilentFunc(command, args...)
			}
			return "", nil
		}

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to check if route exists: mock error"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Given a network manager with route addition error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")
		originalExecSudoFunc := mocks.Shell.ExecSudoFunc
		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "route" && args[0] == "-nv" && args[1] == "add" {
				return "mock output", fmt.Errorf("mock error")
			}
			if originalExecSudoFunc != nil {
				return originalExecSudoFunc(message, command, args...)
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
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestDarwinNetworkManager_ConfigureDNS(t *testing.T) {
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
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

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
		mocks.ConfigHandler.Set("dns.address", "")

		// And capturing resolver file content
		var capturedContent []byte
		mocks.Shims.WriteFile = func(_ string, content []byte, _ os.FileMode) error {
			capturedContent = content
			return nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolver file should contain localhost
		expectedContent := "nameserver 127.0.0.1\n"
		if string(capturedContent) != expectedContent {
			t.Errorf("expected resolver file content to be %q, got %q", expectedContent, string(capturedContent))
		}
	})

	t.Run("NoDNSDomainConfigured", func(t *testing.T) {
		// Given a network manager with no DNS domain
		manager, mocks := setup(t)

		// And mocking empty DNS domain
		mocks.ConfigHandler.Set("dns.domain", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS domain is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDNSAddressConfigured", func(t *testing.T) {
		// Given a network manager with no DNS address
		manager, mocks := setup(t)

		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "DNS address is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ResolverFileAlreadyExists", func(t *testing.T) {
		// Given a network manager with existing resolver file
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking existing resolver file
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == fmt.Sprintf("/etc/resolver/%s", mocks.ConfigHandler.GetString("dns.domain")) {
				return []byte(fmt.Sprintf("nameserver %s\n", mocks.ConfigHandler.GetString("dns.address"))), nil
			}
			return nil, nil // Return nil error to simulate file existing
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("CreateResolverDirectoryError", func(t *testing.T) {
		// Given a network manager with resolver directory error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking resolver directory error
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == "/etc/resolver" {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		mocks.Shell.ExecSudoFunc = func(message, command string, args ...string) (string, error) {
			if command == "mkdir" {
				return "", fmt.Errorf("mock error creating resolver directory")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error creating resolver directory: mock error creating resolver directory"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given a network manager with file write error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking file write error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("mock error writing to temporary resolver file")
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error writing to temporary resolver file: mock error writing to temporary resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MoveResolverFileError", func(t *testing.T) {
		// Given a network manager with file move error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		// And mocking successful write but failed move
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil // Mock successful write to temporary resolver file
		}

		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "mv" {
				return "", fmt.Errorf("mock error moving resolver file")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error moving resolver file: mock error moving resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("FlushDNSCacheError", func(t *testing.T) {
		// Given a network manager with DNS cache flush error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "dscacheutil" && args[0] == "-flushcache" {
				return "", fmt.Errorf("mock error flushing DNS cache")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error flushing DNS cache: mock error flushing DNS cache"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RestartMDNSResponderError", func(t *testing.T) {
		// Given a network manager with mDNSResponder restart error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "1.2.3.4")

		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "killall" && args[0] == "-HUP" {
				return "", fmt.Errorf("mock error restarting mDNSResponder")
			}
			return "", nil
		}

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error restarting mDNSResponder: mock error restarting mDNSResponder"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("IsLocalhostScenario", func(t *testing.T) {
		// Given a network manager in localhost mode
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("dns.address", "127.0.0.1")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
