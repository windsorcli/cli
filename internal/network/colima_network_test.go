package network

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
)

type ColimaNetworkManagerMocks struct {
	Injector          di.Injector
	MockShell         *shell.MockShell
	MockSecureShell   *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
	MockSSHClient     *ssh.MockClient
}

func setupColimaNetworkManagerMocks() *ColimaNetworkManagerMocks {
	// Create a mock injector
	injector := di.NewInjector()

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
		if command == "ls" && args[0] == "/sys/class/net" {
			return "br-bridge0\neth0\nlo", nil
		}
		if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
			return "", fmt.Errorf("Bad rule")
		}
		return "", nil
	}

	// Use the same mock shell for both shell and secure shell
	mockSecureShell := mockShell

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "docker.network_cidr":
			return "192.168.5.0/24"
		case "vm.driver":
			return "colima"
		case "vm.address":
			return "192.168.5.100"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "docker.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}

	// Create a mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Register mocks in the injector
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("sshClient", mockSSHClient)

	// Return a struct containing all mocks
	return &ColimaNetworkManagerMocks{
		Injector:          injector,
		MockShell:         mockShell,
		MockSecureShell:   mockSecureShell,
		MockConfigHandler: mockConfigHandler,
		MockSSHClient:     mockSSHClient,
	}
}

func TestColimaNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks using setupSafeAwsEnvMocks
		mocks := setupColimaNetworkManagerMocks()

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method
		err = nm.ConfigureGuest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDRConfigured", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the GetString method to return an empty string for "docker.network_cidr"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "docker.network_cidr" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some-value"
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to missing network CIDR
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "network CIDR is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoGuestIPConfigured", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the GetString method to return an empty string for "vm.address"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.address" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "some-value"
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to missing guest IP
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "guest IP is not configured"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingSSHConfig", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return an error when getting SSH config
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "ssh-config" {
				return "", fmt.Errorf("mock error getting SSH config")
			}
			return "", nil
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed SSH config retrieval
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing VM SSH config command: mock error getting SSH config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingSSHClient", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the SetClientConfigFileFunc to return an error
		mocks.MockSSHClient.SetClientConfigFileFunc = func(config string, contextName string) error {
			return fmt.Errorf("mock error setting SSH client config")
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed SSH client config
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error setting SSH client config: mock error setting SSH client config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorListingInterfaces", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return an error when listing interfaces
		mocks.MockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error listing interfaces")
			}
			return "", nil
		}

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(mocks.Injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to failed interface listing
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error executing command to list network interfaces: mock error listing interfaces"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to return no interfaces starting with "br-"
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo\nwlan0", nil // No "br-" interface
			}
			return "", nil
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to no docker bridge interface found
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error: no docker bridge interface found"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingIptablesRule", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to simulate finding a docker bridge interface and an error when setting iptables rule
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
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

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to iptables rule setting failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error setting iptables rule") {
			t.Fatalf("expected error to contain 'error setting iptables rule', got %q", err.Error())
		}
	})

	t.Run("ErrorCheckingIptablesRule", func(t *testing.T) {
		// Setup mocks using setupColimaNetworkManagerMocks
		mocks := setupColimaNetworkManagerMocks()

		// Override the ExecFunc to simulate finding a docker bridge interface and an error when checking iptables rule
		mocks.MockSecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-1234\neth0\nlo\nwlan0", nil // Include a "br-" interface
			}
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("mock error checking iptables rule")
			}
			return "", nil
		}

		// Use the mock injector from setupColimaNetworkManagerMocks
		injector := mocks.Injector

		// Create a colimaNetworkManager using NewColimaNetworkManager with the mock injector
		nm := NewColimaNetworkManager(injector)

		// Initialize the network manager
		err := nm.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Call the ConfigureGuest method and expect an error due to iptables rule check failure
		err = nm.ConfigureGuest()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error checking iptables rule: mock error checking iptables rule"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
