package services

import (
	"errors"
	"net"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/di"
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
}

// BaseService is a base implementation of the Service interface
type BaseService struct {
	injector di.Injector
	address  string
	name     string
}

// Initialize is a no-op for the Service interface
func (s *BaseService) Initialize() error {
	// No operation performed
	return nil
}

// WriteConfig is a no-op for the Service interface
func (s *BaseService) WriteConfig() error {
	// No operation performed
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

// SetAddress sets the address if it is a valid IPv4 address
func (s *BaseService) SetAddress(address string) error {
	if net.ParseIP(address) == nil || net.ParseIP(address).To4() == nil {
		return errors.New("invalid IPv4 address")
	}
	s.address = address
	return nil
}
