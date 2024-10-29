//go:build darwin
// +build darwin

package network

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/shell"
)

// Configure sets up the local development network
func (n *networkManager) Configure(networkConfig *NetworkConfig) (*NetworkConfig, error) {
	// Get the shell from the DI container
	shellInstance, err := n.diContainer.Resolve("shell")
	if err != nil {
		return networkConfig, fmt.Errorf("failed to resolve shell instance: %w", err)
	}
	shell := shellInstance.(shell.Shell)

	// Add route on the host to VM guest
	output, err := shell.Exec(
		false,
		"",
		"sudo",
		"route",
		"-nv",
		"add",
		"-net",
		networkConfig.HostRouteCIDR,
		networkConfig.GuestIP,
	)
	if err != nil {
		return networkConfig, fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	return networkConfig, nil
}
