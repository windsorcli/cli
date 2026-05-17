//go:build linux
// +build linux

package network

import (
	"fmt"
	"os"
	"strings"
	"testing"
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
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, mocks := setup(t)
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
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingRouteTable", func(t *testing.T) {
		// Given a network manager with route check error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")
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
		// Given a network manager with no guest address in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("workstation.address", "")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then an error should occur
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest address is required"
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
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.2")

		// And configuring the host route
		err := manager.ConfigureHostRoute()

		// Then no error should occur
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
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.100")

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
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")
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
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And capturing the content via the temp-file write shim
		var capturedContent []byte
		mocks.Shims.WriteFile = func(_ string, data []byte, _ os.FileMode) error {
			capturedContent = data
			return nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			if capturedContent != nil {
				return capturedContent, nil
			}
			return nil, os.ErrNotExist
		}

		// And configuring DNS
		err = manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the drop-in file should contain domain-scoped localhost resolver
		expectedContent := "[Resolve]\nDomains=~example.com\nDNS=127.0.0.1\n"
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
		mocks.ConfigHandler.Set("workstation.dns.address", "")
		mocks.ConfigHandler.Set("workstation.runtime", "colima")

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
		// Given a network manager with existing drop-in file (domain-scoped)
		manager, mocks := setup(t)

		// And mocking the drop-in file content
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return []byte("[Resolve]\nDomains=~example.com\nDNS=1.2.3.4\n"), nil
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
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		// And mocking systemd-resolved being in use
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}

		// And mocking file not existing
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// And mocking DNS config writing error on the mv into place
		mocks.Shell.ExecSudoFunc = func(description, command string, args ...string) (string, error) {
			if command == "mv" {
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
		expectedError := "failed to write DNS configuration: failed to install file: mock error writing config"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRestartingSystemdResolved", func(t *testing.T) {
		// Given a network manager with systemd-resolved restart error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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

	t.Run("AbsoluteSymlinkAccepted", func(t *testing.T) {
		// Given systemd-resolved exposes the absolute stub-resolv.conf symlink form (Fedora, some Ubuntu cloud images)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "/run/systemd/resolve/stub-resolv.conf", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur — the absolute symlink form is treated as systemd-resolved in use
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorStagingTempFile", func(t *testing.T) {
		// Given the temp-file write for the DNS drop-in fails
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("mock error staging temp file")
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then the staging error surfaces and no sudo write is attempted
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to write DNS configuration: failed to stage file: mock error staging temp file"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingTempDir", func(t *testing.T) {
		// Given the private temp dir cannot be created
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.MkdirTemp = func(_, _ string) (string, error) {
			return "", fmt.Errorf("mock mkdirtemp failure")
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then the helper's temp-dir error surfaces with context
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to write DNS configuration: failed to create temp directory: mock mkdirtemp failure"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingFileMode", func(t *testing.T) {
		// Given the post-mv chmod on the drop-in fails
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			if command == "chmod" {
				return "", fmt.Errorf("mock error setting mode")
			}
			return "", nil
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then the chmod error surfaces
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to write DNS configuration: failed to set file mode: mock error setting mode"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DomainWithPathSeparatorRejected", func(t *testing.T) {
		// Given a malformed DNS domain that would let configuration escape the drop-in directory
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "evil/../etc/passwd")

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then validation rejects it before any filesystem or shell operation runs
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := `invalid DNS domain "evil/../etc/passwd": contains path separator`
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("TempDirCleanedUpOnSuccess", func(t *testing.T) {
		// Given a successful DNS configuration run
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")
		mocks.Shims.ReadLink = func(_ string) (string, error) {
			return "../run/systemd/resolve/stub-resolv.conf", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		var removedPath string
		mocks.Shims.MkdirTemp = func(_, _ string) (string, error) {
			return "/tmp/windsor-net-mock", nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			removedPath = path
			return nil
		}

		// When configuring DNS
		if err := manager.ConfigureDNS(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the deferred cleanup removed the same temp directory the helper created
		if removedPath != "/tmp/windsor-net-mock" {
			t.Fatalf("expected temp dir cleanup of %q, got %q", "/tmp/windsor-net-mock", removedPath)
		}
	})
}

func TestLinuxNetworkManager_FlushDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a network manager
		manager, _ := setup(t)

		// When flushing the DNS cache
		err := manager.FlushDNS()

		// Then no error should occur (FlushDNS is a no-op on Linux)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
