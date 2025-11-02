package services

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/shell"
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
	SetAddress(address string) error
	GetAddress() string
	SetName(name string)
	GetName() string
	Initialize() error
	SupportsWildcard() bool
	GetHostname() string
}

// =============================================================================
// Types
// =============================================================================

// BaseService is a base implementation of the Service interface
type BaseService struct {
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
	address       string
	name          string
	shims         *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseService is a constructor for BaseService
func NewBaseService(injector di.Injector) *BaseService {
	return &BaseService{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

func (s *BaseService) Initialize() error {
	configHandler, ok := s.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	s.configHandler = configHandler

	shell, ok := s.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	s.shell = shell

	return nil
}

// WriteConfig is a no-op for the Service interface
func (s *BaseService) WriteConfig() error {
	// No operation performed
	return nil
}

// SetAddress sets the address if it is a valid IPv4 address
func (s *BaseService) SetAddress(address string) error {
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

// isLocalhostMode checks if we are in localhost mode (vm.driver == "docker-desktop")
func (s *BaseService) isLocalhostMode() bool {
	vmDriver := s.configHandler.GetString("vm.driver")
	return vmDriver == "docker-desktop"
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
