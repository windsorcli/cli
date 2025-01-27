package services

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// DNSService handles DNS configuration
type DNSService struct {
	BaseService
	services []Service
}

// NewDNSService creates a new DNSService
func NewDNSService(injector di.Injector) *DNSService {
	return &DNSService{
		BaseService: BaseService{
			injector: injector,
			name:     "dns",
		},
	}
}

// Initialize resolves and sets all the things resolved from the DI
func (s *DNSService) Initialize() error {
	// Call the base Initialize method
	if err := s.BaseService.Initialize(); err != nil {
		return err
	}

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

// SetAddress sets the address for the DNS service
func (s *DNSService) SetAddress(address string) error {
	// Set the value of the DNS address in the configuration
	err := s.configHandler.SetContextValue("dns.address", address)
	if err != nil {
		return fmt.Errorf("error setting DNS address: %w", err)
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig returns the compose configuration
func (s *DNSService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context name
	contextName := s.configHandler.GetContext()

	// Get the domain from the configuration
	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld

	// Common configuration for CoreDNS container
	corednsConfig := types.ServiceConfig{
		Name:          fullName,
		ContainerName: fullName,
		Image:         constants.DEFAULT_DNS_IMAGE,
		Restart:       "always",
		Command:       []string{"-conf", "/etc/coredns/Corefile"},
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.windsor/Corefile", Target: "/etc/coredns/Corefile"},
		},
		Labels: map[string]string{
			"managed_by": "windsor",
			"context":    contextName,
			"role":       "dns",
		},
	}

	// Forward port 53 to the host
	if isLocalhost(s.address) {
		corednsConfig.Ports = []types.ServicePortConfig{
			{
				Target:    53,
				Published: "53",
				Protocol:  "tcp",
			},
			{
				Target:    53,
				Published: "53",
				Protocol:  "udp",
			},
		}
	}

	services := []types.ServiceConfig{corednsConfig}

	return &types.Config{Services: services}, nil
}

// WriteConfig writes any necessary configuration files needed by the service
func (s *DNSService) WriteConfig() error {
	// Retrieve the project root directory using shell.GetProjectRoot
	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	// Get the domain from the configuration
	tld := s.configHandler.GetString("dns.domain", "test")

	// Gather the IP address of each service using the GetHostname method
	var hostEntries string
	for _, service := range s.services {
		composeConfig, err := service.GetComposeConfig()
		if err != nil || composeConfig == nil {
			continue
		}
		for _, svc := range composeConfig.Services {
			if svc.Name != "" {
				address := service.GetAddress()
				if address != "" {
					hostname := service.GetHostname()
					hostEntries += fmt.Sprintf("        %s %s\n", address, hostname)
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

	corefilePath := filepath.Join(projectRoot, ".windsor", "Corefile")

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
