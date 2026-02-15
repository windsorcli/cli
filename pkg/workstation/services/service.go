package services

import (
	"errors"
	"net"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The Service is a core interface that defines the contract for service implementations
// It provides methods for managing service configuration, addressing, and DNS capabilities
// The Service interface serves as the foundation for all Windsor service implementations
// enabling consistent service management across different providers and environments

// =============================================================================
// Interfaces
// =============================================================================

// Service is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Service interface {
	GetComposeConfig() (*types.Config, error)
	WriteConfig() error
	SetAddress(address string, portAllocator *PortAllocator) error
	GetAddress() string
	SetName(name string)
	GetName() string
	SupportsWildcard() bool
	GetHostname() string
	GetIncusConfig() (*IncusConfig, error)
}

// =============================================================================
// Types
// =============================================================================

// IncusConfig represents the configuration for creating an Incus instance.
// It contains all necessary parameters including instance type, image, network settings,
// device configurations, profiles, and resource limits required for instance creation.
type IncusConfig struct {
	Type      string
	Image     string
	Config    map[string]string
	Devices   map[string]map[string]string
	Profiles  []string
	Resources map[string]string
}

// BaseService is a base implementation of the Service interface
type BaseService struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	projectRoot   string
	address       string
	name          string
	shims         *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseService is a constructor for BaseService
func NewBaseService(rt *runtime.Runtime) *BaseService {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}

	return &BaseService{
		configHandler: rt.ConfigHandler,
		shell:         rt.Shell,
		projectRoot:   rt.ProjectRoot,
		shims:         NewShims(),
	}
}

// WriteConfig is a no-op for the Service interface
func (s *BaseService) WriteConfig() error {
	// No operation performed
	return nil
}

// SetAddress sets the address if it is a valid IPv4 address.
// portAllocator is provided for services that need port allocation (e.g., TalosService).
func (s *BaseService) SetAddress(address string, portAllocator *PortAllocator) error {
	if address != "localhost" && (net.ParseIP(address) == nil || net.ParseIP(address).To4() == nil) {
		return errors.New("invalid IPv4 address")
	}
	s.address = address
	return nil
}

// GetAddress returns the current address of the service
func (s *BaseService) GetAddress() string {
	return s.address
}

// SetName sets the name of the service
func (s *BaseService) SetName(name string) {
	s.name = name
}

// GetName returns the current name of the service
func (s *BaseService) GetName() string {
	return s.name
}

// GetContainerName returns the container name with the DNS domain
func (s *BaseService) GetContainerName() string {
	return s.GetHostname()
}

// =============================================================================
// Private Methods
// =============================================================================

// isLocalhostMode checks if we are in localhost mode (workstation runtime == "docker-desktop")
func (s *BaseService) isLocalhostMode() bool {
	return s.configHandler.GetString("workstation.runtime") == "docker-desktop"
}

// SupportsWildcard returns whether the service supports wildcard DNS entries
func (s *BaseService) SupportsWildcard() bool {
	return false
}

// GetHostname returns the hostname for the service, handling domain names specially
func (s *BaseService) GetHostname() string {
	if s.name == "" {
		return ""
	}
	tld := s.configHandler.GetString("dns.domain", "test")

	if strings.Contains(s.name, ".") {
		parts := strings.Split(s.name, ".")
		return strings.Join(parts[:len(parts)-1], ".") + "." + tld
	}

	return s.name + "." + tld
}

// GetIncusConfig returns the Incus configuration for the service.
// The default implementation returns nil, nil indicating the service does not support Incus.
// Services that support Incus should override this method to return their Incus configuration.
func (s *BaseService) GetIncusConfig() (*IncusConfig, error) {
	return nil, nil
}
