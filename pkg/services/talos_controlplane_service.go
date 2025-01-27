package services

import (
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
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

// SetAddress sets the address of the service
// This turns out to be a convenient place to set node information
func (s *TalosControlPlaneService) SetAddress(address string) error {
	tld := s.configHandler.GetString("dns.domain")

	if err := s.configHandler.SetContextValue("cluster.controlplanes.nodes."+s.name+".hostname", s.name+"."+tld); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.controlplanes.nodes."+s.name+".node", address); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.controlplanes.nodes."+s.name+".endpoint", address+":50000"); err != nil {
		return err
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig returns a list of container data for docker-compose.
func (s *TalosControlPlaneService) GetComposeConfig() (*types.Config, error) {
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
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_system_state", "-", "_"), Target: "/system/state"},
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_var", "-", "_"), Target: "/var"},
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_etc_cni", "-", "_"), Target: "/etc/cni"},
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_etc_kubernetes", "-", "_"), Target: "/etc/kubernetes"},
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_usr_libexec_kubernetes", "-", "_"), Target: "/usr/libexec/kubernetes"},
			{Type: "volume", Source: strings.ReplaceAll(s.name+"_opt", "-", "_"), Target: "/opt"},
		},
	}

	// Check if the address is localhost and assign ports if it is
	if isLocalhost(s.address) {
		commonConfig.Ports = []types.ServicePortConfig{
			{
				Target:    50000,
				Published: "50000",
				Protocol:  "tcp",
			},
			{
				Target:    6443,
				Published: "6443",
				Protocol:  "tcp",
			},
		}
	}

	// Get the domain from the configuration
	tld := s.configHandler.GetString("dns.domain", "test")
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

	// Include volume specifications in the compose config
	volumes := map[string]types.VolumeConfig{
		strings.ReplaceAll(s.name+"_system_state", "-", "_"):           {},
		strings.ReplaceAll(s.name+"_var", "-", "_"):                    {},
		strings.ReplaceAll(s.name+"_etc_cni", "-", "_"):                {},
		strings.ReplaceAll(s.name+"_etc_kubernetes", "-", "_"):         {},
		strings.ReplaceAll(s.name+"_usr_libexec_kubernetes", "-", "_"): {},
		strings.ReplaceAll(s.name+"_opt", "-", "_"):                    {},
	}

	return &types.Config{
		Services: []types.ServiceConfig{controlPlaneConfig},
		Volumes:  volumes,
	}, nil
}
