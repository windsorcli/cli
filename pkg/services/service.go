package services

import (
	"errors"
	"fmt"
	"net"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// Service is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Service interface {
	// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
	GetComposeConfig() (*types.Config, error)

	// WriteConfig writes any necessary configuration files needed by the service
	WriteConfig() error

	// SetAddress sets the address if it is a valid IPv4 address
	SetAddress(address string) error

	// GetAddress returns the current address of the service
	GetAddress() string

	// SetName sets the name of the service
	SetName(name string)

	// GetName returns the current name of the service
	GetName() string

	// Initialize performs any necessary initialization for the service.
	Initialize() error

	// SupportsWildcard returns whether the service supports wildcard DNS entries
	SupportsWildcard() bool

	// GetHostname returns the hostname for the service, which may include domain processing
	GetHostname() string
}

// BaseService is a base implementation of the Service interface
type BaseService struct {
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
	address       string
	name          string
}

// Initialize resolves and assigns configHandler and shell dependencies using the injector.
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

// GetContainerName returns the container name with the "windsor-" prefix and without the DNS domain
func (s *BaseService) GetContainerName() string {
	contextName := s.configHandler.GetContext()
	return fmt.Sprintf("windsor-%s-%s", contextName, s.name)
}

// IsLocalhostMode checks if we are in localhost mode (vm.driver == "docker-desktop")
func (s *BaseService) isLocalhostMode() bool {
	vmDriver := s.configHandler.GetString("vm.driver")
	return vmDriver == "docker-desktop"
}

// SupportsWildcard returns whether the service supports wildcard DNS entries
func (s *BaseService) SupportsWildcard() bool {
	return false
}

// GetHostname returns the hostname for the service with the configured TLD
func (s *BaseService) GetHostname() string {
	tld := s.configHandler.GetString("dns.domain", "test")
	return s.name + "." + tld
}
