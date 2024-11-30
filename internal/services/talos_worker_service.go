package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type TalosWorkerService struct {
	BaseService
	injector      di.Injector
	configHandler config.ConfigHandler
	shell         shell.Shell
}

// NewTalosWorkerService is a constructor for TalosWorkerService
func NewTalosWorkerService(injector di.Injector) *TalosWorkerService {
	return &TalosWorkerService{injector: injector}
}

// Initialize resolves dependencies and initializes the TalosWorkerService
func (s *TalosWorkerService) Initialize() error {
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
func (s *TalosWorkerService) GetComposeConfig() (*types.Config, error) {
	// Retrieve the context configuration
	clusterDriver := s.configHandler.GetString("cluster.driver")

	// Check if the cluster driver is Talos
	if clusterDriver == "" || clusterDriver != "talos" {
		return nil, nil
	}

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
			{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.volumes", Target: "/var/local"},
		},
	}

	// Create a single worker service
	workerConfig := commonConfig
	if s.GetName() == "" {
		workerConfig.Name = "worker.test"
	} else {
		workerConfig.Name = s.GetName()
	}
	workerConfig.Environment = map[string]*string{
		"PLATFORM": strPtr("container"),
		"TALOSSKU": strPtr(fmt.Sprintf("%dCPU-%dRAM", workerCPU, workerRAM*1024)),
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

	return &types.Config{Services: []types.ServiceConfig{workerConfig}, Volumes: volumes}, nil
}
