package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// KubeHelper is a helper struct that provides Kubernetes-specific utility functions
type KubeHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewKubeHelper is a constructor for KubeHelper
func NewKubeHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *KubeHelper {
	return &KubeHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves Kubernetes-specific environment variables for the current context
func (h *KubeHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the kubeconfig file
	kubeConfigPath := filepath.Join(configRoot, ".kube", "config")
	if _, err := os.Stat(kubeConfigPath); os.IsNotExist(err) {
		kubeConfigPath = ""
	}

	envVars := map[string]string{
		"KUBECONFIG": kubeConfigPath,
	}

	return envVars, nil
}

// Ensure KubeHelper implements Helper interface
var _ Helper = (*KubeHelper)(nil)