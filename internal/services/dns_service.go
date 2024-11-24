package services

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// DNSService handles DNS configuration
type DNSService struct {
	injector di.Injector
}

// NewDNSService creates a new DNSService
func NewDNSService(injector di.Injector) (*DNSService, error) {
	return &DNSService{
		injector: injector,
	}, nil
}

// GetComposeConfig returns the compose configuration
func (s *DNSService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context name
	contextHandler, err := s.injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}
	contextName, err := contextHandler.(context.ContextInterface).GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context name: %w", err)
	}

	// Retrieve the context configuration
	configHandler, err := s.injector.Resolve("configHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving configHandler: %w", err)
	}
	contextConfig := configHandler.(config.ConfigHandler).GetConfig()

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

// WriteConfig writes any necessary configuration files needed by the service
func (s *DNSService) WriteConfig() error {
	// Retrieve the context configuration
	configHandler, err := s.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}
	contextConfig := configHandler.(config.ConfigHandler).GetConfig()

	// Check if DNS is defined and DNS Create is enabled
	if contextConfig.DNS == nil || contextConfig.DNS.Create == nil || !*contextConfig.DNS.Create {
		return nil
	}

	// Check if Docker is enabled
	if contextConfig.Docker == nil || contextConfig.Docker.Enabled == nil || !*contextConfig.Docker.Enabled {
		return nil
	}

	// Retrieve the configuration directory for the current context
	resolvedContext, err := s.injector.Resolve("contextHandler")
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

	// Retrieve the compose configuration from DockerService
	dockerService, err := s.injector.Resolve("dockerService")
	if err != nil {
		return fmt.Errorf("error resolving dockerService: %w", err)
	}
	dockerServiceInstance, ok := dockerService.(DockerService)
	if !ok {
		return fmt.Errorf("error casting to DockerService")
	}
	composeConfig, err := dockerServiceInstance.GetFullComposeConfig()
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

// Ensure DNSService implements Service interface
var _ Service = (*DNSService)(nil)
