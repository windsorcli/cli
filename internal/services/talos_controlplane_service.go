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
			{Type: "volume", Target: "/system/state"},
			{Type: "volume", Target: "/var"},
			{Type: "volume", Target: "/etc/cni"},
			{Type: "volume", Target: "/etc/kubernetes"},
			{Type: "volume", Target: "/usr/libexec/kubernetes"},
			{Type: "volume", Target: "/opt"},
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

	return &types.Config{Services: []types.ServiceConfig{controlPlaneConfig}}, nil
}
