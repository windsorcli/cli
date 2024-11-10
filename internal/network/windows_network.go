//go:build windows
// +build windows

package network

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/shell"
)

// ConfigureHost sets up the local development network
func (n *networkManager) ConfigureHost() error {
	// Retrieve the entire configuration object
	contextConfig := n.configHandler.GetConfig()

	// Access the Docker configuration
	if contextConfig.Docker == nil || contextConfig.Docker.NetworkCIDR == nil {
		return fmt.Errorf("network CIDR is not configured")
	}
	networkCIDR := *contextConfig.Docker.NetworkCIDR

	// Access the VM configuration
	if contextConfig.VM == nil || contextConfig.VM.Driver == nil {
		return fmt.Errorf("guest IP is not configured")
	}
	guestIP := *contextConfig.VM.Driver

	// Get the shell from the injector
	shellInstance, err := n.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("failed to resolve shell instance: %w", err)
	}
	shell := shellInstance.(shell.Shell)

	// Add route on the host to VM guest
	output, err := shell.Exec(
		true,
		"Adding route on the host to VM guest",
		"powershell",
		"-Command",
		fmt.Sprintf("New-NetRoute -DestinationPrefix %s -NextHop %s -RouteMetric 1", networkCIDR, guestIP),
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	return nil
}
