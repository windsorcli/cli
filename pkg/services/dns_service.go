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

// Initialize sets up DNSService by resolving dependencies via DI.
func (s *DNSService) Initialize() error {
	if err := s.BaseService.Initialize(); err != nil {
		return err
	}
	resolvedServices, err := s.injector.ResolveAll(new(Service))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}
	for _, serviceInterface := range resolvedServices {
		service, _ := serviceInterface.(Service)
		s.services = append(s.services, service)
	}
	return nil
}

// SetAddress updates DNS address in config and calls BaseService's SetAddress.
func (s *DNSService) SetAddress(address string) error {
	err := s.configHandler.SetContextValue("dns.address", address)
	if err != nil {
		return fmt.Errorf("error setting DNS address: %w", err)
	}
	return s.BaseService.SetAddress(address)
}

// GetComposeConfig sets up CoreDNS with context and domain, configures ports if localhost.
func (s *DNSService) GetComposeConfig() (*types.Config, error) {
	contextName := s.configHandler.GetContext()
	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld

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

	if s.IsLocalhostMode() {
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

// WriteConfig generates a Corefile for DNS configuration by gathering project root, TLD, and service IPs,
// constructing DNS host entries, and appending static DNS records. It adapts the Corefile for localhost
// by adding a template for local DNS resolution. Additionally, it configures DNS forwarding by including
// specified forward addresses, ensuring DNS queries are directed appropriately. The final Corefile is
// written to the .windsor config directory
func (s *DNSService) WriteConfig() error {
	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	tld := s.configHandler.GetString("dns.domain", "test")

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

	dnsRecords := s.configHandler.GetStringSlice("dns.records", nil)
	for _, record := range dnsRecords {
		hostEntries += fmt.Sprintf("        %s\n", record)
	}

	forwardAddresses := s.configHandler.GetStringSlice("dns.forward", nil)
	if len(forwardAddresses) == 0 {
		forwardAddresses = []string{"1.1.1.1", "8.8.8.8"}
	}
	forwardAddressesStr := fmt.Sprintf("%s", forwardAddresses[0])
	for _, addr := range forwardAddresses[1:] {
		forwardAddressesStr += fmt.Sprintf(" %s", addr)
	}

	var corefileContent string
	corefileContent = fmt.Sprintf(`
%s:53 {
    errors
    reload
    loop
    hosts {
%s        fallthrough
    }
    forward . %s
}
.:53 {
    errors
    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
`, tld, hostEntries, forwardAddressesStr)

	corefilePath := filepath.Join(projectRoot, ".windsor", "Corefile")

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
