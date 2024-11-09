package network

import (
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock secure shell
		mockShell := shell.NewMockShell(di.NewContainer())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-bridge0\neth0\nlo", nil
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("Bad rule")
			}
			return "", nil
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("secureShell", mockShell)

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method
		err = nm.ConfigureGuest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		// Create a mock DI container that does not register the secure shell
		diContainer := di.NewMockContainer()
		diContainer.SetResolveError("secureShell", fmt.Errorf("no instance registered with name secureShell"))

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer.DIContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to unresolved secure shell
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to resolve secure shell instance: no instance registered with name secureShell"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Create a mock shell that returns an error when listing interfaces
		mockShell := shell.NewMockShell(di.NewContainer())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			return "", nil
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("secureShell", mockShell)

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed interface listing
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error executing command to list network interfaces: mock error listing interfaces"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		// Create a mock shell that returns no interfaces starting with "br-"
		mockShell := shell.NewMockShell(di.NewContainer())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo\nwlan0", nil // No "br-" interface
			}
			return "", nil
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("secureShell", mockShell)

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to no docker bridge interface found
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error: No docker bridge interface found"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingIptablesRule", func(t *testing.T) {
		// Create a mock shell that simulates finding a docker bridge interface and an error when setting iptables rule
		mockShell := shell.NewMockShell(di.NewContainer())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
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

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("secureShell", mockShell)

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to iptables rule setting failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error setting iptables rule: mock error setting iptables rule"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingIptablesRule", func(t *testing.T) {
		// Create a mock shell that simulates finding a docker bridge interface and an error when checking iptables rule
		mockShell := shell.NewMockShell(di.NewContainer())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-1234\neth0\nlo\nwlan0", nil // Include a "br-" interface
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("mock error checking iptables rule")
			}
			return "", nil
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("secureShell", mockShell)

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock DI container
		nm, err := NewColimaNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to iptables rule check failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "Error checking iptables rule: mock error checking iptables rule"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
