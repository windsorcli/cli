package helpers

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// DNSHelper handles DNS configuration
type DNSHelper struct {
	injector di.Injector
}

// NewDNSHelper creates a new DNSHelper
func NewDNSHelper(injector di.Injector) (*DNSHelper, error) {
	return &DNSHelper{
		injector: injector,
	}, nil
}

// GetComposeConfig returns the compose configuration
func (h *DNSHelper) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context name
	contextHandler, err := h.injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}
	contextName, err := contextHandler.(context.ContextInterface).GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context name: %w", err)
	}

	// Retrieve the context configuration
	cliConfigHandler, err := h.injector.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}
	contextConfig, err := cliConfigHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Check if the DNS is enabled
	if contextConfig.DNS == nil || contextConfig.DNS.Create == nil || !*contextConfig.DNS.Create {
		return nil, nil
	}

	// Get the Name from the configuration
	name := "test"
	if contextConfig.DNS.Name != nil && *contextConfig.DNS.Name != "" {
		name = *contextConfig.DNS.Name
	}

	// Common configuration for CoreDNS container
	corednsConfig := types.ServiceConfig{
		Name:    fmt.Sprintf("dns.%s", name),
		Image:   constants.DEFAULT_DNS_IMAGE,
		Restart: "always",
		Command: []string{"-conf", "/etc/coredns/Corefile"},
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "./Corefile", Target: "/etc/coredns/Corefile"},
		},
		Labels: map[string]string{
			"managed_by": "windsor",
			"context":    contextName,
			"role":       "dns",
		},
	}

	services := []types.ServiceConfig{corednsConfig}
	volumes := map[string]types.VolumeConfig{
		"coredns_config": {},
	}

	return &types.Config{Services: services, Volumes: volumes}, nil
}

// WriteConfig writes any necessary configuration files needed by the helper
func (h *DNSHelper) WriteConfig() error {
	// Retrieve the context configuration
	cliConfigHandler, err := h.injector.Resolve("cliConfigHandler")
	if err != nil {
		return fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}
	contextConfig, err := cliConfigHandler.(config.ConfigHandler).GetConfig()
	if err != nil {
		return fmt.Errorf("error retrieving context configuration: %w", err)
	}

	// Check if DNS is defined and DNS Create is enabled
	if contextConfig.DNS == nil || contextConfig.DNS.Create == nil || !*contextConfig.DNS.Create {
		return nil
	}

	// Check if Docker is enabled
	if contextConfig.Docker == nil || contextConfig.Docker.Enabled == nil || !*contextConfig.Docker.Enabled {
		return nil
	}

	// Retrieve the configuration directory for the current context
	resolvedContext, err := h.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving context: %w", err)
	}
	configDir, err := resolvedContext.(context.ContextInterface).GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving config root: %w", err)
	}

	// Get the TLD from the configuration
	name := "test"
	if contextConfig.DNS.Name != nil && *contextConfig.DNS.Name != "" {
		name = *contextConfig.DNS.Name
	}

	// Retrieve the compose configuration from DockerHelper
	dockerHelper, err := h.injector.Resolve("dockerHelper")
	if err != nil {
		return fmt.Errorf("error resolving dockerHelper: %w", err)
	}
	dockerHelperInstance, ok := dockerHelper.(*DockerHelper)
	if !ok {
		return fmt.Errorf("error casting to DockerHelper")
	}
	composeConfig, err := dockerHelperInstance.GetFullComposeConfig()
	if err != nil {
		return fmt.Errorf("error retrieving compose configuration: %w", err)
	}

	// Gather the IP address of each service
	var hostEntries string
	for _, service := range composeConfig.Services {
		for _, networkConfig := range service.Networks {
			if networkConfig.Ipv4Address != "" {
				hostEntries += fmt.Sprintf("        %s %s\n", networkConfig.Ipv4Address, service.Name)
			}
		}
	}

	// Template out the Corefile with information from the compose configuration
	corefileContent := fmt.Sprintf(`
%s:53 {
    hosts {
%s        fallthrough
    }

    forward . 1.1.1.1 8.8.8.8
}
`, name, hostEntries)

	corefilePath := filepath.Join(configDir, "Corefile")

	// Ensure the parent folders exist
	if err := mkdirAll(filepath.Dir(corefilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent folders: %w", err)
	}

	err = writeFile(corefilePath, []byte(corefileContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing Corefile: %w", err)
	}

	return nil
}

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DNSHelper)(nil)
