package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// OmniHelper is a helper struct that provides Omnirnetes-specific utility functions
type OmniHelper struct {
	Context context.ContextInterface
}

// NewOmniHelper is a constructor for OmniHelper
func NewOmniHelper(di *di.DIContainer) (*OmniHelper, error) {
	resolvedContext, err := di.Resolve("contextInstance")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	contextInterface, ok := resolvedContext.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("resolved context is not of type ContextInterface")
	}

	return &OmniHelper{
		Context: contextInterface,
	}, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *OmniHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetEnvVars retrieves Omnirnetes-specific environment variables for the current context
func (h *OmniHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the omniconfig file
	omniConfigPath := filepath.Join(configRoot, ".omni", "config")
	if _, err := os.Stat(omniConfigPath); os.IsNotExist(err) {
		omniConfigPath = ""
	}

	envVars := map[string]string{
		"OMNICONFIG": omniConfigPath,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *OmniHelper) PostEnvExec() error {
	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *OmniHelper) GetComposeConfig() (*types.Config, error) {
	// Stub implementation
	return nil, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *OmniHelper) WriteConfig() error {
	return nil
}

// Up executes necessary commands to instantiate the tool or environment.
func (h *OmniHelper) Up(verbose ...bool) error {
	return nil
}

// Info returns information about the helper.
func (h *OmniHelper) Info() (interface{}, error) {
	return nil, nil
}

// Ensure OmniHelper implements Helper interface
var _ Helper = (*OmniHelper)(nil)
