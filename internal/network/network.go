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
}

// networkManager is a concrete implementation of NetworkManager
type networkManager struct {
	diContainer      di.ContainerInterface
	sshClient        ssh.Client
	shell            shell.Shell
	secureShell      shell.SecureShell
	cliConfigHandler config.ConfigHandler
	colimaVirt       virt.VMInterface
	dockerVirt       virt.ContainerInterface
}

// NewNetworkManager creates a new NetworkManager
func NewNetworkManager(container di.ContainerInterface) (NetworkManager, error) {
	nm := &networkManager{
		diContainer: container,
	}
	return nm, nil
}

// Initialize the network manager
func (n *networkManager) Initialize() error {
	// Resolve the sshClient from the DI container
	sshClientInstance, err := n.diContainer.Resolve("sshClient")
	if err != nil {
		return fmt.Errorf("failed to resolve ssh client instance: %w", err)
	}
	sshClient, ok := sshClientInstance.(ssh.Client)
	if !ok {
		return fmt.Errorf("resolved ssh client instance is not of type ssh.Client")
	}
	n.sshClient = sshClient

	// Get the shell from the DI container
	shellInstance, err := n.diContainer.Resolve("shell")
	if err != nil {
		return fmt.Errorf("failed to resolve shell instance: %w", err)
	}
	shellInterface, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	// Get the secure shell from the DI container
	secureShellInstance, err := n.diContainer.Resolve("secureShell")
	if err != nil {
		return fmt.Errorf("failed to resolve secure shell instance: %w", err)
	}
	secureShellInterface, ok := secureShellInstance.(shell.SecureShell)
	if !ok {
		return fmt.Errorf("resolved secure shell instance is not of type shell.SecureShell")
	}
	n.secureShell = secureShellInterface

	// Get the CLI config handler from the DI container
	cliConfigHandlerInstance, err := n.diContainer.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("failed to resolve CLI config handler: %w", err)
	}
	cliConfigHandler, ok := cliConfigHandlerInstance.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("resolved CLI config handler instance is not of type config.ConfigHandler")
	}
	n.cliConfigHandler = cliConfigHandler

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *networkManager) ConfigureGuest() error {
	return nil
}

// Ensure networkManager implements NetworkManager
var _ NetworkManager = (*networkManager)(nil)
