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
	BaseService
	injector       di.Injector
	configHandler  config.ConfigHandler
	contextHandler context.ContextHandler
	services       []Service
}

// NewDNSService creates a new DNSService
func NewDNSService(injector di.Injector) *DNSService {
	return &DNSService{
		injector: injector,
	}
}

// Initialize resolves and sets all the things resolved from the DI
func (s *DNSService) Initialize() error {
	// Resolve the configHandler from the injector
	configHandler, err := s.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}
	s.configHandler = configHandler.(config.ConfigHandler)

	// Resolve the contextHandler from the injector
	resolvedContext, err := s.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving context: %w", err)
	}
	s.contextHandler = resolvedContext.(context.ContextHandler)

	// Resolve all services from the injector
	resolvedServices, err := s.injector.ResolveAll(new(Service))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	// Set each service on the class
	for _, serviceInterface := range resolvedServices {
		service, _ := serviceInterface.(Service)
		s.services = append(s.services, service)
	}

	return nil
}

// GetComposeConfig returns the compose configuration
func (s *DNSService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context name
	contextName, err := s.contextHandler.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context name: %w", err)
	}

	// Retrieve the context configuration
	contextConfig := s.configHandler.GetConfig()

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
	contextConfig := s.configHandler.GetConfig()

	// Check if DNS is defined and DNS Create is enabled
	if contextConfig.DNS == nil || contextConfig.DNS.Create == nil || !*contextConfig.DNS.Create {
		return nil
	}

	// Check if Docker is enabled
	if contextConfig.Docker == nil || contextConfig.Docker.Enabled == nil || !*contextConfig.Docker.Enabled {
		return nil
	}

	// Retrieve the configuration directory for the current context
	configDir, err := s.contextHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving config root: %w", err)
	}

	// Get the TLD from the configuration
	tld := "test"
	if contextConfig.DNS.Name != nil && *contextConfig.DNS.Name != "" {
		tld = *contextConfig.DNS.Name
	}

	// Gather the IP address of each service using the Address field
	var hostEntries string
	for _, service := range s.services {
		composeConfig, err := service.GetComposeConfig()
		if err != nil || composeConfig == nil {
			continue
		}
		for _, svc := range composeConfig.Services {
			if svc.Name != "" {
				if addressService, ok := service.(interface{ GetAddress() string }); ok {
					address := addressService.GetAddress()
					if address != "" {
						fullName := fmt.Sprintf("%s.%s", svc.Name, tld)
						hostEntries += fmt.Sprintf("        %s %s\n", fullName, address)
					}
				}
			}
		}
	}

	// Template out the Corefile with information from the services
	corefileContent := fmt.Sprintf(`
%s:53 {
    hosts {
%s        fallthrough
    }

    forward . 1.1.1.1 8.8.8.8
}
`, tld, hostEntries)

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
