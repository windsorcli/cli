//go:build darwin
// +build darwin

package network

import (
	"encoding/json"
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Configure sets up the local development network
func (n *networkManager) Configure() error {
	configHandler, err := n.diContainer.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving config handler: %w", err)
	}
	config, err := configHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving config: %w", err)
	}

	// Check if Docker and NetworkCIDR is defined
	if config.Docker == nil || config.Docker.NetworkCIDR == nil {
		return nil
	}

	// Only proceed with Colima if VM.Driver is "colima"
	if config.VM == nil || config.VM.Driver == nil || *config.VM.Driver != "colima" {
		return nil
	}

	// Check Colima status
	contextHandler, err := n.diContainer.Resolve("contextInstance")
	if err != nil {
		return fmt.Errorf("error resolving context handler: %w", err)
	}
	contextName, err := contextHandler.(context.ContextInterface).GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}
	colimaProfile := fmt.Sprintf("windsor-%s", contextName)

	shellInstance, err := n.diContainer.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	output, err := shellInstance.(shell.Shell).Exec(false, "Checking Colima status", "colima", "list", "--json", "--profile", colimaProfile)
	if err != nil {
		return fmt.Errorf("failed to check Colima status: %w", err)
	}

	var colimaStatus struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(output), &colimaStatus); err != nil {
		return fmt.Errorf("failed to parse Colima status: %w", err)
	}

	var colimaVMIP string
	if colimaStatus.Name == colimaProfile && colimaStatus.Status == "Running" && colimaStatus.Address != "" {
		colimaVMIP = colimaStatus.Address
	}

	if colimaVMIP == "" {
		return fmt.Errorf("Colima VM IP not found or Colima is not running")
	}

	// Add route on the host to VM guest
	output, err = shellInstance.(shell.Shell).Exec(
		false,
		"",
		"sudo",
		"route",
		"-nv",
		"add",
		"-net",
		*config.Docker.NetworkCIDR,
		colimaVMIP,
	)
	if err != nil {
		return fmt.Errorf("failed to add route: %w", err)
	}

	return nil
}
