package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// TalosHelper is a helper struct that provides Talos-specific utility functions
type TalosHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
}

// NewTalosHelper is a constructor for TalosHelper
func NewTalosHelper(di *di.DIContainer) (*TalosHelper, error) {
	resolvedConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving config handler: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &TalosHelper{
		ConfigHandler: resolvedConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// GetEnvVars retrieves Talos-specific environment variables for the current context
func (h *TalosHelper) GetEnvVars() (map[string]string, error) {
	// Retrieve the current context
	currentContext, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving current context: %w", err)
	}

	// Check if the cluster driver is Talos
	clusterDriver, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.cluster.driver", currentContext))
	if err != nil {
		return nil, fmt.Errorf("error retrieving cluster driver: %w", err)
	}
	if clusterDriver != "talos" {
		return nil, nil
	}

	// Get the context config path
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context config path: %w", err)
	}

	// Construct the path to the talosconfig file
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")
	if _, err := os.Stat(talosConfigPath); os.IsNotExist(err) {
		talosConfigPath = ""
	}

	envVars := map[string]string{
		"TALOSCONFIG": talosConfigPath,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *TalosHelper) PostEnvExec() error {
	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *TalosHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Retrieve the current context
	currentContext, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving current context: %w", err)
	}

	// Check if the cluster driver is Talos
	clusterDriver, err := h.ConfigHandler.GetString(fmt.Sprintf("contexts.%s.cluster.driver", currentContext))
	if err != nil {
		return nil, fmt.Errorf("error retrieving cluster driver: %w", err)
	}
	if clusterDriver != "talos" {
		return nil, nil
	}

	// Retrieve the number of control planes and workers from the configuration
	numControlPlanes, err := h.ConfigHandler.GetInt(fmt.Sprintf("contexts.%s.cluster.controlplanes.count", currentContext), 1)
	if err != nil {
		return nil, fmt.Errorf("error retrieving number of control planes: %w", err)
	}

	numWorkers, err := h.ConfigHandler.GetInt(fmt.Sprintf("contexts.%s.cluster.workers.count", currentContext), 1)
	if err != nil {
		return nil, fmt.Errorf("error retrieving number of workers: %w", err)
	}

	// Retrieve CPU and RAM settings for control planes from the configuration
	controlPlaneCPU, err := h.ConfigHandler.GetInt(
		fmt.Sprintf("contexts.%s.cluster.controlplanes.cpu", currentContext),
		constants.DEFAULT_TALOS_CONTROL_PLANE_CPU,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving control plane CPU setting: %w", err)
	}

	controlPlaneRAM, err := h.ConfigHandler.GetInt(
		fmt.Sprintf("contexts.%s.cluster.controlplanes.memory", currentContext),
		constants.DEFAULT_TALOS_CONTROL_PLANE_RAM,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving control plane RAM setting: %w", err)
	}

	// Retrieve CPU and RAM settings for workers from the configuration
	workerCPU, err := h.ConfigHandler.GetInt(fmt.Sprintf("contexts.%s.cluster.workers.cpu", currentContext), 4)
	if err != nil {
		return nil, fmt.Errorf("error retrieving worker CPU setting: %w", err)
	}

	workerRAM, err := h.ConfigHandler.GetInt(fmt.Sprintf("contexts.%s.cluster.workers.memory", currentContext), 4)
	if err != nil {
		return nil, fmt.Errorf("error retrieving worker RAM setting: %w", err)
	}

	// Common configuration for Talos containers
	commonConfig := types.ServiceConfig{
		Image:       constants.DEFAULT_TALOS_IMAGE,
		Environment: map[string]*string{"PLATFORM": strPtr("container")},
		Restart:     "always",
		ReadOnly:    true,
		Privileged:  true,
		SecurityOpt: []string{"seccomp=unconfined"},
		Tmpfs:       []string{"/run", "/system", "/tmp"},
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "/run/udev", Target: "/run/udev"},
			{Type: "volume", Source: "system_state", Target: "/system/state"},
			{Type: "volume", Source: "var", Target: "/var"},
			{Type: "volume", Source: "etc_cni", Target: "/etc/cni"},
			{Type: "volume", Source: "etc_kubernetes", Target: "/etc/kubernetes"},
			{Type: "volume", Source: "usr_libexec_kubernetes", Target: "/usr/libexec/kubernetes"},
			{Type: "volume", Source: "usr_etc_udev", Target: "/usr/etc/udev"},
			{Type: "volume", Source: "opt", Target: "/opt"},
		},
	}

	var services []types.ServiceConfig

	// Create control plane services
	for i := 0; i < numControlPlanes; i++ {
		controlPlaneConfig := commonConfig
		controlPlaneConfig.Name = fmt.Sprintf("controlplane-%d.test", i+1)
		controlPlaneConfig.Environment["TALOSSKU"] = strPtr(fmt.Sprintf("%dCPU-%dRAM", controlPlaneCPU, controlPlaneRAM*1024))
		services = append(services, controlPlaneConfig)
	}

	// Create worker services
	for i := 0; i < numWorkers; i++ {
		workerConfig := commonConfig
		workerConfig.Name = fmt.Sprintf("worker-%d.test", i+1)
		workerConfig.Environment["TALOSSKU"] = strPtr(fmt.Sprintf("%dCPU-%dRAM", workerCPU, workerRAM*1024))
		workerConfig.Volumes = append(workerConfig.Volumes, types.ServiceVolumeConfig{
			Type:   "bind",
			Source: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".volumes"),
			Target: "/var/local",
		})
		services = append(services, workerConfig)
	}

	return services, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *TalosHelper) WriteConfig() error {
	return nil
}

// Ensure TalosHelper implements Helper interface
var _ Helper = (*TalosHelper)(nil)
