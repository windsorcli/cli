package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

type TalosWorkerService struct {
	BaseService
}

// NewTalosWorkerService is a constructor for TalosWorkerService
func NewTalosWorkerService(injector di.Injector) *TalosWorkerService {
	return &TalosWorkerService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// SetAddress sets the address of the service
// This turns out to be a convenient place to set node information
func (s *TalosWorkerService) SetAddress(address string) error {
	tld := s.configHandler.GetString("dns.name", "test")

	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".hostname", s.name+"."+tld); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".node", address); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".endpoint", address+":50000"); err != nil {
		return err
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig returns a list of container data for docker-compose.
func (s *TalosWorkerService) GetComposeConfig() (*types.Config, error) {
	// Retrieve CPU and RAM settings for workers from the configuration
	workerCPU := s.configHandler.GetInt("cluster.workers.cpu", constants.DEFAULT_TALOS_WORKER_CPU)
	workerRAM := s.configHandler.GetInt("cluster.workers.memory", constants.DEFAULT_TALOS_WORKER_RAM)

	// Get the project root and create the .volumes folder if it doesn't exist
	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	volumesPath := filepath.Join(projectRoot, ".volumes")
	if _, err := stat(volumesPath); os.IsNotExist(err) {
		if err := mkdir(volumesPath, os.ModePerm); err != nil {
			return nil, fmt.Errorf("error creating .volumes directory: %w", err)
		}
	}

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
			{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.volumes", Target: "/var/local"},
		},
	}

	// Get the TLD from the configuration
	tld := s.configHandler.GetString("dns.name", "test")
	fullName := s.name + "." + tld
	if s.name == "" {
		fullName = "worker" + "." + tld
	} else {
		fullName = s.name + "." + tld
	}

	// Create a single worker service
	workerConfig := commonConfig
	workerConfig.Name = fullName
	workerConfig.ContainerName = fullName
	workerConfig.Hostname = fullName
	workerConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", workerCPU, workerRAM*1024)),
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
		Services: []types.ServiceConfig{workerConfig},
		Volumes:  volumes,
	}, nil
}
