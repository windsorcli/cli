package services

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/internal/constants"
	"github.com/windsorcli/cli/internal/di"
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
			{Type: "volume", Source: "system_state", Target: "/system/state"},
			{Type: "volume", Source: "var", Target: "/var"},
			{Type: "volume", Source: "etc_cni", Target: "/etc/cni"},
			{Type: "volume", Source: "etc_kubernetes", Target: "/etc/kubernetes"},
			{Type: "volume", Source: "usr_libexec_kubernetes", Target: "/usr/libexec/kubernetes"},
			{Type: "volume", Source: "opt", Target: "/opt"},
		},
	}

	// Get the TLD from the configuration
	tld := s.configHandler.GetString("dns.name", "test")
	fullName := s.name + "." + tld
	if s.name == "" {
		fullName = "controlplane" + "." + tld
	} else {
		fullName = s.name + "." + tld
	}

	// Create a single control plane service
	controlPlaneConfig := commonConfig
	controlPlaneConfig.Name = fullName
	controlPlaneConfig.ContainerName = fullName
	controlPlaneConfig.Hostname = fullName
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
		"opt":                    {},
	}

	return &types.Config{Services: []types.ServiceConfig{controlPlaneConfig}, Volumes: volumes}, nil
}
