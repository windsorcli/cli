package services

import (
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
}

// BaseService is a base implementation of the Service interface
type BaseService struct {
	injector di.Injector
}

// WriteConfig is a no-op for the Service interface
func (s *BaseService) WriteConfig() error {
	// No operation performed
	return nil
}
