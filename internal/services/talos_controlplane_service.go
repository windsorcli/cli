package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
)

type TalosControlPlaneService struct {
	BaseService
}

// NewTalosControlPlaneService is a constructor for TalosControlPlaneService
func NewTalosControlPlaneService(injector di.Injector) *TalosControlPlaneService {
	return &TalosControlPlaneService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// GetComposeConfig returns a list of container data for docker-compose.
func (s *TalosControlPlaneService) GetComposeConfig() (*types.Config, error) {
	// Retrieve CPU and RAM settings for control planes from the configuration
	controlPlaneCPU := s.configHandler.GetInt("cluster.controlplanes.cpu", constants.DEFAULT_TALOS_CONTROL_PLANE_CPU)
	controlPlaneRAM := s.configHandler.GetInt("cluster.controlplanes.memory", constants.DEFAULT_TALOS_CONTROL_PLANE_RAM)

	// Common configuration for Talos containers
	commonConfig := types.ServiceConfig{
		Image:       constants.DEFAULT_TALOS_IMAGE,
		Environment: map[string]*string{"PLATFORM": ptrString("container")},
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
		controlPlaneConfig.Name = "controlplane"
	} else {
		controlPlaneConfig.Name = s.GetName()
	}
	controlPlaneConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", controlPlaneCPU, controlPlaneRAM*1024)),
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
