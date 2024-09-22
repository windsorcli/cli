package helpers

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

type KubeHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

func NewKubeHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *KubeHelper {
	return &KubeHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

func (h *KubeHelper) GetEnvVars() (map[string]string, error) {
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	kubeConfigPath := fmt.Sprintf("%s/.kube/config", configRoot)
	envVars := map[string]string{
		"KUBECONFIG": kubeConfigPath,
	}

	return envVars, nil
}

// Ensure KubeHelper implements Helper interface
var _ Helper = (*KubeHelper)(nil)
