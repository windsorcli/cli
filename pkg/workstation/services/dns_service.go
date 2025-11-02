package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// The DNSService is a core component that manages DNS configuration and resolution
// It provides DNS management capabilities for Windsor services and applications
// The DNSService handles CoreDNS configuration, service discovery, and DNS forwarding
// enabling seamless DNS resolution across different environments and contexts

// =============================================================================
// Types
// =============================================================================

// DNSService handles DNS configuration
type DNSService struct {
	BaseService
	services []Service
}

// =============================================================================
// Constructor
// =============================================================================

// NewDNSService creates a new DNSService
func NewDNSService(injector di.Injector) *DNSService {
	return &DNSService{
		BaseService: *NewBaseService(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

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
	err := s.configHandler.Set("dns.address", address)
	if err != nil {
		return fmt.Errorf("error setting DNS address: %w", err)
	}
	return s.BaseService.SetAddress(address)
}

// GetComposeConfig sets up CoreDNS with context and domain, configures ports if localhost.
func (s *DNSService) GetComposeConfig() (*types.Config, error) {
	contextName := s.configHandler.GetContext()
	serviceName := s.GetName()
	containerName := s.GetContainerName()

	corednsConfig := types.ServiceConfig{
		Name:          serviceName,
		ContainerName: containerName,
		Image:         constants.DefaultDNSImage,
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

	if s.isLocalhostMode() {
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

	services := types.Services{
		serviceName: corednsConfig,
	}

	return &types.Config{Services: services}, nil
}

// WriteConfig generates and writes a CoreDNS Corefile for the Windsor project.
// It collects the project root directory, top-level domain (TLD), and service IP addresses.
// For each service, it adds DNS entries mapping hostnames to IP addresses, and includes wildcard DNS entries if supported.
// In localhost mode, it uses a template for local DNS resolution and sets up forwarding rules for DNS queries.
// The generated Corefile is saved in the .windsor directory for CoreDNS to manage project DNS queries.
func (s *DNSService) WriteConfig() error {
	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	tld := s.configHandler.GetString("dns.domain", "test")

	var (
		hostEntries              string
		localhostHostEntries     string
		wildcardEntries          string
		localhostWildcardEntries string
	)

	wildcardTemplate := `
    template IN A {
        match ^(.*)\.%s\.$
        answer "{{ .Name }} 60 IN A %s"
        fallthrough
    }
`
	localhostTemplate := `
    template IN A {
        match ^(.*)\.%s\.$
        answer "{{ .Name }} 60 IN A 127.0.0.1"
        fallthrough
    }
`

	for _, service := range s.services {
		composeConfig, err := service.GetComposeConfig()
		if err != nil || composeConfig == nil {
			continue
		}
		for _, svc := range composeConfig.Services {
			if svc.Name == "" {
				continue
			}
			address := service.GetAddress()
			if address == "" {
				continue
			}

			hostname := service.GetHostname()
			escapedHostname := strings.ReplaceAll(hostname, ".", "\\.")

			hostEntries += fmt.Sprintf("        %s %s\n", address, hostname)
			if s.isLocalhostMode() {
				localhostHostEntries += fmt.Sprintf("        127.0.0.1 %s\n", hostname)
			}
			if service.SupportsWildcard() {
				wildcardEntries += fmt.Sprintf(wildcardTemplate, escapedHostname, address)
				if s.isLocalhostMode() {
					localhostWildcardEntries += fmt.Sprintf(localhostTemplate, escapedHostname)
				}
			}
		}
	}

	for _, record := range s.configHandler.GetStringSlice("dns.records", nil) {
		hostEntries += fmt.Sprintf("        %s\n", record)
		if s.isLocalhostMode() {
			localhostHostEntries += fmt.Sprintf("        %s\n", record)
		}
	}

	forwardAddresses := s.configHandler.GetStringSlice("dns.forward", nil)
	if len(forwardAddresses) == 0 {
		forwardAddresses = []string{"1.1.1.1", "8.8.8.8"}
	}
	forwardAddressesStr := strings.Join(forwardAddresses, " ")

	serverBlockTemplate := `%s:53 {
%s    hosts {
%s        fallthrough
    }
%s
    reload
    loop
    forward . %s
}
`

	var corefileContent string
	if s.isLocalhostMode() {
		corefileContent = fmt.Sprintf(serverBlockTemplate, tld, "", localhostHostEntries, localhostWildcardEntries, forwardAddressesStr)
	} else {
		corefileContent = fmt.Sprintf(serverBlockTemplate, tld, "", hostEntries, wildcardEntries, forwardAddressesStr)
	}

	corefileContent += `.:53 {
    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
`

	corefilePath := filepath.Join(projectRoot, ".windsor", "Corefile")
	if err := s.shims.MkdirAll(filepath.Dir(corefilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent folders: %w", err)
	}

	if stat, err := s.shims.Stat(corefilePath); err == nil && stat.IsDir() {
		if err := s.shims.RemoveAll(corefilePath); err != nil {
			return fmt.Errorf("error removing Corefile directory: %w", err)
		}
	}

	if err := s.shims.WriteFile(corefilePath, []byte(corefileContent), 0644); err != nil {
		return fmt.Errorf("error writing Corefile: %w", err)
	}

	return nil
}

// Ensure DNSService implements Service interface
var _ Service = (*DNSService)(nil)
