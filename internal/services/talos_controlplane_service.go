package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type TalosControlPlaneService struct {
	BaseService
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
}

// NewTalosControlPlaneService is a constructor for TalosControlPlaneService
func NewTalosControlPlaneService(injector di.Injector) *TalosControlPlaneService {
	return &TalosControlPlaneService{injector: injector}
}

// Initialize resolves dependencies and initializes the TalosControlPlaneService
func (s *TalosControlPlaneService) Initialize() error {
	// Resolve the configHandler from the injector
	configHandlerInstance, err := s.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}
	s.configHandler = configHandlerInstance.(config.ConfigHandler)

	// Resolve the shell from the injector
	shellInstance, err := s.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	s.shell = shellInstance.(shell.Shell)

	return nil
}

// GetComposeConfig returns a list of container data for docker-compose.
func (s *TalosControlPlaneService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context configuration
	clusterDriver := s.configHandler.GetString("cluster.driver")

	// Check if the cluster driver is Talos
	if clusterDriver == "" || clusterDriver != "talos" {
		return nil, nil
	}

	// Retrieve CPU and RAM settings for control planes from the configuration
	controlPlaneCPU := s.configHandler.GetInt("cluster.controlplanes.cpu", constants.DEFAULT_TALOS_CONTROL_PLANE_CPU)
	controlPlaneRAM := s.configHandler.GetInt("cluster.controlplanes.memory", constants.DEFAULT_TALOS_CONTROL_PLANE_RAM)

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

	// Create a single control plane service
	controlPlaneConfig := commonConfig
	if s.GetName() == "" {
		controlPlaneConfig.Name = "controlplane.test"
	} else {
		controlPlaneConfig.Name = s.GetName()
	}
	controlPlaneConfig.Environment = map[string]*string{
		"PLATFORM": strPtr("container"),
		"TALOSSKU": strPtr(fmt.Sprintf("%dCPU-%dRAM", controlPlaneCPU, controlPlaneRAM*1024)),
	}

	// Define volumes
	volumes := map[string]types.VolumeConfig{
		"system_state":           {},
		"var":                    {},
		"etc_cni":                {},
		"etc_kubernetes":         {},
		"usr_libexec_kubernetes": {},
		"usr_etc_udev":           {},
		"opt":                    {},
	}

	return &types.Config{Services: []types.ServiceConfig{controlPlaneConfig}, Volumes: volumes}, nil
}
