package network

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	// Initialize the network manager
	Initialize() error
	// ConfigureHost sets up the local development network
	ConfigureHost() error
	// ConfigureGuest sets up the guest VM network
	ConfigureGuest() error
	// ConfigureDNS sets up the DNS configuration
	ConfigureDNS() error
}

// networkManager is a concrete implementation of NetworkManager
type networkManager struct {
	injector      di.Injector
	sshClient     ssh.Client
	shell         shell.Shell
	secureShell   shell.Shell
	configHandler config.ConfigHandler
	colimaVirt    virt.Virt
	dockerVirt    virt.Virt
}

// NewNetworkManager creates a new NetworkManager
func NewNetworkManager(injector di.Injector) (NetworkManager, error) {
	nm := &networkManager{
		injector: injector,
	}
	return nm, nil
}

// Initialize the network manager
func (n *networkManager) Initialize() error {
	// Resolve the sshClient from the injector
	sshClientInstance, err := n.injector.Resolve("sshClient")
	if err != nil {
		return fmt.Errorf("failed to resolve ssh client instance: %w", err)
	}
	sshClient, ok := sshClientInstance.(ssh.Client)
	if !ok {
		return fmt.Errorf("resolved ssh client instance is not of type ssh.Client")
	}
	n.sshClient = sshClient

	// Get the shell from the injector
	shellInstance, err := n.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("failed to resolve shell instance: %w", err)
	}
	shellInterface, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	// Get the secure shell from the injector
	secureShellInstance, err := n.injector.Resolve("secureShell")
	if err != nil {
		return fmt.Errorf("failed to resolve secure shell instance: %w", err)
	}
	secureShell, ok := secureShellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved secure shell instance is not of type shell.Shell")
	}
	n.secureShell = secureShell

	// Get the CLI config handler from the injector
	configHandlerInstance, err := n.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("failed to resolve CLI config handler: %w", err)
	}
	configHandler, ok := configHandlerInstance.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("resolved CLI config handler instance is not of type config.ConfigHandler")
	}
	n.configHandler = configHandler

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *networkManager) ConfigureGuest() error {
	return nil
}

// Ensure networkManager implements NetworkManager
var _ NetworkManager = (*networkManager)(nil)
