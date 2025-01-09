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
	nextPort int
}

// NewTalosWorkerService is a constructor for TalosWorkerService
func NewTalosWorkerService(injector di.Injector) *TalosWorkerService {
	return &TalosWorkerService{
		BaseService: BaseService{
			injector: injector,
		},
		nextPort: 50001, // Initialize the next available port
	}
}

// SetAddress configures the network address for the Talos worker service.
// It also sets node-specific information such as hostname and endpoint in the configuration.
// If the address is localhost (127.0.0.1), it assigns a unique port starting from 50001.
func (s *TalosWorkerService) SetAddress(address string) error {
	tld := s.configHandler.GetString("dns.name", "test")

	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".hostname", s.name+"."+tld); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".node", address); err != nil {
		return err
	}

	port := 50000
	if address == "127.0.0.1" {
		port = s.nextPort
		s.nextPort++
	}

	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".endpoint", fmt.Sprintf("%s:%d", address, port)); err != nil {
		return err
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig returns a list of container data for docker-compose. It retrieves CPU and
// RAM settings for workers from the configuration and determines the port for the endpoint.
// The function ensures the project root's .volumes folder exists and sets up a common
// configuration for Talos containers. Finally, it creates a single worker service and includes
// volume specifications in the compose config.
func (s *TalosWorkerService) GetComposeConfig() (*types.Config, error) {
	// Retrieve configuration settings and endpoint details
	workerCPU := s.configHandler.GetInt("cluster.workers.cpu", constants.DEFAULT_TALOS_WORKER_CPU)
	workerRAM := s.configHandler.GetInt("cluster.workers.memory", constants.DEFAULT_TALOS_WORKER_RAM)
	endpoint := s.configHandler.GetString("cluster.workers.nodes."+s.name+".endpoint", "50000")
	publishedPort := "50000"
	if parts := strings.Split(endpoint, ":"); len(parts) == 2 {
		publishedPort = parts[1]
	}

	// Ensure necessary directories exist
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

	// Construct the common configuration for the Talos worker service
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

	// Check if the address is localhost and assign ports if it is
	if isLocalhost(s.address) {
		commonConfig.Ports = []types.ServicePortConfig{
			{
				Target:    50000,
				Published: publishedPort,
				Protocol:  "tcp",
			},
		}
	}

	// Finalize the worker configuration with specific settings
	tld := s.configHandler.GetString("dns.name", "test")
	fullName := s.name + "." + tld
	if s.name == "" {
		fullName = "worker" + "." + tld
	} else {
		fullName = s.name + "." + tld
	}

	workerConfig := commonConfig
	workerConfig.Name = fullName
	workerConfig.ContainerName = fullName
	workerConfig.Hostname = fullName
	workerConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", workerCPU, workerRAM*1024)),
	}

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
