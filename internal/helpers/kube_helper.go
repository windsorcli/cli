package helpers

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

type KubeHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
}

func NewKubeHelper(configHandler config.ConfigHandler, shell shell.Shell) *KubeHelper {
	return &KubeHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
	}
}

func (h *KubeHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	projectRoot, err := h.Shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}

	kubeConfigPath := filepath.Join(projectRoot, "contexts", context, ".kube", "config")
	envVars := map[string]string{
		"KUBECONFIG": kubeConfigPath,
	}

	return envVars, nil
}

// Ensure KubeHelper implements Helper interface
var _ Helper = (*KubeHelper)(nil)
