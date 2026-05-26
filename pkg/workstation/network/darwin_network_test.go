//go:build darwin
// +build darwin

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

func TestDarwinNetworkManager_ConfigureHostRoute(t *testing.T) {
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

	t.Run("PersistsRouteViaNetworksetupWhenServiceAvailable", func(t *testing.T) {
		// Given a primary network service is available and no persistent route exists yet
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.1")
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "networksetup" && len(args) > 0 && args[0] == "-listallnetworkservices" {
				return "An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n*USB 10/100\n", nil
			}
			if command == "networksetup" && len(args) > 0 && args[0] == "-getadditionalroutes" {
				return "There aren't any.\n", nil
			}
			return "", nil
		}
		var setAdditionalArgs []string
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			if command == "networksetup" && len(args) > 0 && args[0] == "-setadditionalroutes" {
				setAdditionalArgs = append([]string{}, args...)
			}
			return "", nil
		}

		if err := manager.ConfigureHostRoute(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then setadditionalroutes was invoked with our triple after the service name
		if len(setAdditionalArgs) < 5 {
			t.Fatalf("expected setadditionalroutes to be invoked, got args=%v", setAdditionalArgs)
		}
		for i, want := range []string{"-setadditionalroutes", "Wi-Fi", "192.168.5.0", "255.255.255.0", "192.168.5.1"} {
			if setAdditionalArgs[i] != want {
				t.Errorf("arg[%d] = %q, want %q (full args=%v)", i, setAdditionalArgs[i], want, setAdditionalArgs)
			}
		}
	})

	t.Run("EphemeralFallbackWhenNoServiceDetected", func(t *testing.T) {
		// Given no enabled network service is available
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.1")
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "networksetup" {
				return "An asterisk (*) denotes that a network service is disabled.\n", nil
			}
			return "", nil
		}
		var routeAdded bool
		var setAdditionalCalled bool
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			if command == "route" && len(args) > 1 && args[1] == "add" {
				routeAdded = true
			}
			if command == "networksetup" && len(args) > 0 && args[0] == "-setadditionalroutes" {
				setAdditionalCalled = true
			}
			return "", nil
		}

		if err := manager.ConfigureHostRoute(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the live route was added but no persistence call fired
		if !routeAdded {
			t.Error("expected ephemeral route to be added")
		}
		if setAdditionalCalled {
			t.Error("expected no networksetup -setadditionalroutes call when no service is available")
		}
	})

	t.Run("SkipsPersistenceWhenAlreadyPersistent", func(t *testing.T) {
		// Given the route is already in the kernel and already persistent
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.1")
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "route" && len(args) >= 2 && args[0] == "-n" && args[1] == "get" {
				return "   route to: 192.168.5.0\ndestination: 192.168.5.0\n    gateway: 192.168.5.1\n", nil
			}
			if command == "networksetup" && len(args) > 0 && args[0] == "-listallnetworkservices" {
				return "Wi-Fi\n", nil
			}
			if command == "networksetup" && len(args) > 0 && args[0] == "-getadditionalroutes" {
				return "192.168.5.0 255.255.255.0 192.168.5.1\n", nil
			}
			return "", nil
		}
		var anySudoCall bool
		mocks.Shell.ExecSudoFunc = func(_, _ string, _ ...string) (string, error) {
			anySudoCall = true
			return "", nil
		}

		if err := manager.ConfigureHostRoute(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then no sudo invocation occurred (route + persistence both already in place)
		if anySudoCall {
			t.Error("expected zero sudo calls when route is both live and persistent")
		}
	})
}

func TestDarwinNetworkManager_ConfigureDNS(t *testing.T) {
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
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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
		mocks.ConfigHandler.Set("workstation.dns.address", "")

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
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		// And mocking existing resolver file
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == fmt.Sprintf("/etc/resolver/%s", mocks.ConfigHandler.GetString("dns.domain")) {
				return []byte(fmt.Sprintf("nameserver %s\n", mocks.ConfigHandler.GetString("workstation.dns.address"))), nil
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
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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
		expectedError := "Error installing resolver file: failed to stage file: mock error writing to temporary resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MoveResolverFileError", func(t *testing.T) {
		// Given a network manager with file move error
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

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
		expectedError := "Error installing resolver file: failed to install file: mock error moving resolver file"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("IsLocalhostScenario", func(t *testing.T) {
		// Given a network manager in localhost mode
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.dns.address", "127.0.0.1")

		// And configuring DNS
		err := manager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DomainOutsideAllowlistRejected", func(t *testing.T) {
		// Given a malformed DNS domain that would let configuration escape the resolver directory
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "evil/../etc/passwd")

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then validation rejects it before any filesystem or shell operation runs
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := `invalid DNS domain "evil/../etc/passwd": must contain only letters, digits, hyphen, and dot`
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("DotOnlyDomainRejectedBeforeMv", func(t *testing.T) {
		// Given a dot-only DNS domain. /etc/resolver/.. resolves to /etc/, so without this guard
		// a subsequent sudo mv would deposit the staged drop-in at /etc/drop-in.
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "..")
		mocks.ConfigHandler.Set("workstation.dns.address", "1.2.3.4")

		// And tracking whether a sudo mv (or any sudo) ever runs
		var sudoCommands []string
		mocks.Shell.ExecSudoFunc = func(_, command string, _ ...string) (string, error) {
			sudoCommands = append(sudoCommands, command)
			return "", nil
		}

		// When configuring DNS
		err := manager.ConfigureDNS()

		// Then validation rejects with the empty-label error before any sudo step runs
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := `invalid DNS domain "..": contains empty label`
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
		if len(sudoCommands) != 0 {
			t.Fatalf("expected zero sudo invocations before validation rejection, got %v", sudoCommands)
		}
	})
}

func TestDarwinNetworkManager_RevertHostRoute(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("NoOpWhenCIDRUnset", func(t *testing.T) {
		// Given no network CIDR in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "")
		var called bool
		mocks.Shell.ExecSudoFunc = func(_, _ string, _ ...string) (string, error) {
			called = true
			return "", nil
		}

		// When reverting the host route
		if err := manager.RevertHostRoute(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no sudo invocation occurs — there's nothing to revert
		if called {
			t.Errorf("expected no sudo invocation when CIDR is unset")
		}
	})

	t.Run("RemovesRouteWhenPresent", func(t *testing.T) {
		// Given a configured CIDR and route delete succeeds
		manager, _ := setup(t)
		var deletedCIDR string
		mocks := setupNetworkMocks(t)
		manager.shell = mocks.Shell
		manager.configHandler = mocks.ConfigHandler
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			if command == "route" && len(args) > 2 && args[1] == "delete" {
				deletedCIDR = args[len(args)-1]
				return "", nil
			}
			return "", nil
		}

		// When reverting the host route
		if err := manager.RevertHostRoute(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then route delete was called with the configured CIDR
		if deletedCIDR != "192.168.5.0/24" {
			t.Errorf("expected route delete for %q, got %q", "192.168.5.0/24", deletedCIDR)
		}
	})

	t.Run("TolerantOfNotInTable", func(t *testing.T) {
		// Given route delete returns "not in table" (route was never installed or already removed)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.Shell.ExecSudoFunc = func(_, command string, _ ...string) (string, error) {
			if command == "route" {
				return "route: writing to routing socket: not in table", fmt.Errorf("exit status 77")
			}
			return "", nil
		}

		// When reverting the host route
		err := manager.RevertHostRoute()

		// Then revert reports success — idempotent
		if err != nil {
			t.Errorf("expected nil error for idempotent revert, got %v", err)
		}
	})

	t.Run("ErrorsOnUnknownFailure", func(t *testing.T) {
		// Given route delete fails for an unrecognized reason
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.5.0/24")
		mocks.Shell.ExecSudoFunc = func(_, command string, _ ...string) (string, error) {
			if command == "route" {
				return "permission denied", fmt.Errorf("exit status 1")
			}
			return "", nil
		}

		// When reverting the host route
		err := manager.RevertHostRoute()

		// Then the error surfaces with context
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting host route") {
			t.Errorf("expected wrapped error, got %v", err)
		}
	})
}

func TestDarwinNetworkManager_RevertDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("NoOpWhenDomainUnset", func(t *testing.T) {
		// Given no DNS domain in config
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "")
		var called bool
		mocks.Shell.ExecSudoFunc = func(_, _ string, _ ...string) (string, error) {
			called = true
			return "", nil
		}

		// When reverting DNS
		if err := manager.RevertDNS(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then no sudo invocation occurs
		if called {
			t.Errorf("expected no sudo invocation when domain is unset")
		}
	})

	t.Run("RemovesResolverFileForConfiguredDomain", func(t *testing.T) {
		// Given a configured domain
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "local.test")
		var removed string
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			if command == "rm" && len(args) >= 2 {
				removed = args[len(args)-1]
			}
			return "", nil
		}

		// When reverting DNS
		if err := manager.RevertDNS(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		// Then the per-domain resolver file is removed via rm -f
		want := "/etc/resolver/local.test"
		if removed != want {
			t.Errorf("expected removal of %q, got %q", want, removed)
		}
	})

	t.Run("RejectsDomainWithPathSeparator", func(t *testing.T) {
		// Given a malformed DNS domain
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "evil/../etc/passwd")

		// When reverting DNS
		err := manager.RevertDNS()

		// Then validation rejects before any rm runs
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must contain only letters") {
			t.Errorf("expected validation error, got %v", err)
		}
	})
}

func TestDarwinNetworkManager_FlushDNS(t *testing.T) {
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

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FlushCacheError", func(t *testing.T) {
		// Given a network manager where dscacheutil fails
		manager, mocks := setup(t)
		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "dscacheutil" {
				return "", fmt.Errorf("mock error flushing DNS cache")
			}
			return "", nil
		}

		// When flushing the DNS cache
		err := manager.FlushDNS()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error flushing DNS cache: mock error flushing DNS cache"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("RestartMDNSResponderError", func(t *testing.T) {
		// Given a network manager where killall mDNSResponder fails
		manager, mocks := setup(t)
		mocks.Shell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			if command == "killall" {
				return "", fmt.Errorf("mock error restarting mDNSResponder")
			}
			return "", nil
		}

		// When flushing the DNS cache
		err := manager.FlushDNS()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error restarting mDNSResponder: mock error restarting mDNSResponder"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
