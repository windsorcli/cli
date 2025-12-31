package services

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// The TalosService is a service component that manages Talos Linux node configuration
// It provides containerized Talos Linux nodes for Kubernetes cluster management
// The TalosService enables both control plane and worker node deployment
// with configurable resources, networking, and storage options

// =============================================================================
// Types
// =============================================================================

// defaultAPIPort is the default API port for Talos services
const defaultAPIPort = constants.DefaultTalosAPIPort

// controlPlaneLeader tracks the first controlplane service to be created as the leader
var (
	controlPlaneLeader *TalosService
	leaderLock         sync.Mutex
)

type TalosService struct {
	BaseService
	mode     string
	isLeader bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewTalosService is a constructor for TalosService.
func NewTalosService(rt *runtime.Runtime, mode string) *TalosService {
	service := &TalosService{
		BaseService: *NewBaseService(rt),
		mode:        mode,
	}

	// Elect a "leader" for the first controlplane
	if mode == "controlplane" {
		leaderLock.Lock()
		defer leaderLock.Unlock()
		if controlPlaneLeader == nil {
			controlPlaneLeader = service
			service.isLeader = true
		}
	}

	return service
}

// =============================================================================
// Public Methods
// =============================================================================

// SetAddress configures the Talos service's hostname and endpoint using the
// provided address. It assigns the default API port to the leader controlplane
// or a unique port if the address is not local. For other nodes, it assigns
// unique API ports starting from 50001, incrementing for each node. The portAllocator
// is used for port allocation if provided; otherwise falls back to global state.
func (s *TalosService) SetAddress(address string, portAllocator *PortAllocator) error {
	if err := s.BaseService.SetAddress(address, portAllocator); err != nil {
		return err
	}

	nodeType := "workers"
	if s.mode == "controlplane" {
		nodeType = "controlplanes"
	}

	if err := s.configHandler.Set(fmt.Sprintf("cluster.%s.nodes.%s.hostname", nodeType, s.name), s.GetHostname()); err != nil {
		return err
	}
	if err := s.configHandler.Set(fmt.Sprintf("cluster.%s.nodes.%s.node", nodeType, s.name), s.GetHostname()); err != nil {
		return err
	}

	var port int
	if portAllocator != nil && s.isLocalhostMode() && !s.isLeader {
		port = portAllocator.NextAvailablePort(defaultAPIPort + 1)
	} else {
		port = defaultAPIPort
	}

	endpointAddress := address
	if s.isLocalhostMode() {
		endpointAddress = "127.0.0.1"
	}

	endpoint := fmt.Sprintf("%s:%d", endpointAddress, port)
	nodePath := fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, s.name)
	if err := s.configHandler.Set(nodePath, endpoint); err != nil {
		return err
	}

	if s.isLocalhostMode() {
		hostPorts := s.configHandler.GetStringSlice(fmt.Sprintf("cluster.%s.hostports", nodeType), []string{})

		hostPortsCopy := make([]string, len(hostPorts))
		copy(hostPortsCopy, hostPorts)

		for i, hostPortStr := range hostPortsCopy {
			hostPort, nodePort, protocol, err := validateHostPort(hostPortStr)
			if err != nil {
				return err
			}

			hostPortsCopy[i] = fmt.Sprintf("%d:%d/%s", hostPort, nodePort, protocol)
		}

		if err := s.configHandler.Set(fmt.Sprintf("cluster.%s.nodes.%s.hostports", nodeType, s.name), hostPortsCopy); err != nil {
			return err
		}
	}

	return nil
}

// GetComposeConfig creates a Docker Compose configuration for Talos services.
// It dynamically retrieves CPU and RAM settings based on whether the node is a worker
// or part of the control plane. The function identifies endpoint ports for service communication and ensures
// that all necessary volume directories are defined. It configures the container with the appropriate image
// (prioritizing node-specific, then group-specific, then cluster-wide, and finally default image settings),
// environment variables, security options, and volume mounts. The service name is constructed using the node
// name, and port mappings are set up, including both default and node-specific ports. The resulting configuration
// provides comprehensive service and volume specifications for deployment.
func (s *TalosService) GetComposeConfig() (*types.Config, error) {
	config := s.configHandler.GetConfig()
	if config.Cluster == nil {
		return &types.Config{
			Services: types.Services{},
			Volumes:  map[string]types.VolumeConfig{},
		}, nil
	}

	var cpu, ram int
	nodeType := "workers"
	if s.mode == "controlplane" {
		nodeType = "controlplanes"
		cpu = s.configHandler.GetInt("cluster.controlplanes.cpu", constants.DefaultTalosControlPlaneCPU)
		ram = s.configHandler.GetInt("cluster.controlplanes.memory", constants.DefaultTalosControlPlaneRAM)
	} else {
		cpu = s.configHandler.GetInt("cluster.workers.cpu", constants.DefaultTalosWorkerCPU)
		ram = s.configHandler.GetInt("cluster.workers.memory", constants.DefaultTalosWorkerRAM)
	}

	nodeName := s.name
	if nodeName == "" {
		nodeName = nodeType[:len(nodeType)-1] // remove 's' from nodeType
	}

	endpoint := s.configHandler.GetString(fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, nodeName), fmt.Sprintf("%d", defaultAPIPort))
	publishedPort := fmt.Sprintf("%d", defaultAPIPort)
	if parts := strings.Split(endpoint, ":"); len(parts) == 2 {
		publishedPort = parts[1]
		if _, err := strconv.ParseUint(publishedPort, 10, 32); err != nil {
			return nil, fmt.Errorf("invalid port value: %s", publishedPort)
		}
	}

	var image string

	nodeImage := s.configHandler.GetString(fmt.Sprintf("cluster.%s.nodes.%s.image", nodeType, nodeName), "")
	if nodeImage != "" {
		image = nodeImage
	} else {
		groupImage := s.configHandler.GetString(fmt.Sprintf("cluster.%s.image", nodeType), "")
		if groupImage != "" {
			image = groupImage
		} else {
			clusterImage := s.configHandler.GetString("cluster.image", "")
			if clusterImage != "" {
				image = clusterImage
			} else {
				image = constants.DefaultTalosImage
			}
		}
	}

	commonConfig := types.ServiceConfig{
		Image:       image,
		Environment: map[string]*string{"PLATFORM": ptrString("container")},
		Restart:     "always",
		ReadOnly:    true,
		Privileged:  true,
		SecurityOpt: []string{"seccomp=unconfined"},
		Tmpfs:       []string{"/run", "/system", "/tmp"},
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_system_state", "-", "_"), Target: "/system/state"},
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_var", "-", "_"), Target: "/var"},
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_etc_cni", "-", "_"), Target: "/etc/cni"},
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_etc_kubernetes", "-", "_"), Target: "/etc/kubernetes"},
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_usr_libexec_kubernetes", "-", "_"), Target: "/usr/libexec/kubernetes"},
			{Type: "volume", Source: strings.ReplaceAll(nodeName+"_opt", "-", "_"), Target: "/opt"},
		},
	}

	// Use volumes from cluster configuration
	volumesKey := fmt.Sprintf("cluster.%s.volumes", nodeType)
	volumes := s.configHandler.GetStringSlice(volumesKey, []string{})
	for _, volume := range volumes {
		parts := strings.Split(volume, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid volume format: %s", volume)
		}

		// Expand environment variables in the source path for directory creation
		expandedSourcePath := os.ExpandEnv(parts[0])

		// Create the directory if it doesn't exist
		if err := s.shims.MkdirAll(expandedSourcePath, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", expandedSourcePath, err)
		}

		// Use the original, pre-expanded source path in the volume configuration
		commonConfig.Volumes = append(commonConfig.Volumes, types.ServiceVolumeConfig{
			Type:   "bind",
			Source: parts[0],
			Target: parts[1],
		})
	}

	serviceConfig := commonConfig
	serviceConfig.Name = nodeName
	s.SetName(nodeName)
	serviceConfig.ContainerName = s.GetContainerName()
	serviceConfig.Hostname = nodeName
	serviceConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", cpu, ram*1024)),
	}

	var ports []types.ServicePortConfig

	// Convert defaultAPIPort to uint32 safely
	if defaultAPIPort < 0 || defaultAPIPort > math.MaxUint32 {
		return nil, fmt.Errorf("defaultAPIPort value out of range: %d", defaultAPIPort)
	}
	defaultAPIPortUint32 := uint32(defaultAPIPort)

	if s.isLocalhostMode() {
		ports = append(ports, types.ServicePortConfig{
			Target:    defaultAPIPortUint32,
			Published: publishedPort,
			Protocol:  "tcp",
		})

		if s.isLeader {
			ports = append(ports, types.ServicePortConfig{
				Target:    6443,
				Published: "6443",
				Protocol:  "tcp",
			})
		}
	}

	hostPortsKey := fmt.Sprintf("cluster.%s.nodes.%s.hostports", nodeType, nodeName)
	hostPorts := s.configHandler.GetStringSlice(hostPortsKey)
	for _, hostPortStr := range hostPorts {
		hostPort, nodePort, protocol, err := validateHostPort(hostPortStr)
		if err != nil {
			return nil, err
		}

		ports = append(ports, types.ServicePortConfig{
			Target:    nodePort,
			Published: fmt.Sprintf("%d", hostPort),
			Protocol:  protocol,
		})
	}

	serviceConfig.Ports = ports

	volumesMap := map[string]types.VolumeConfig{
		strings.ReplaceAll(nodeName+"_system_state", "-", "_"):           {},
		strings.ReplaceAll(nodeName+"_var", "-", "_"):                    {},
		strings.ReplaceAll(nodeName+"_etc_cni", "-", "_"):                {},
		strings.ReplaceAll(nodeName+"_etc_kubernetes", "-", "_"):         {},
		strings.ReplaceAll(nodeName+"_usr_libexec_kubernetes", "-", "_"): {},
		strings.ReplaceAll(nodeName+"_opt", "-", "_"):                    {},
	}

	services := types.Services{
		nodeName: serviceConfig,
	}

	return &types.Config{
		Services: services,
		Volumes:  volumesMap,
	}, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// GetHostname returns the hostname without any TLD
func (s *TalosService) GetHostname() string {
	if parts := strings.Split(s.name, "."); len(parts) > 1 {
		return parts[0]
	}
	return s.name
}

// validateHostPort parses and validates a host port string in the format "hostPort:nodePort/protocol"
// Returns the parsed hostPort, nodePort, and protocol, or an error if validation fails
func validateHostPort(hostPortStr string) (uint32, uint32, string, error) {
	parts := strings.Split(hostPortStr, ":")
	var hostPort, nodePort uint32
	protocol := "tcp"

	switch len(parts) {
	case 1: // hostPort only
		port, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid hostPort value: %s", parts[0])
		}
		nodePort = uint32(port)
		hostPort = nodePort
	case 2: // hostPort and nodePort/protocol
		port, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid hostPort value: %s", parts[0])
		}
		hostPort = uint32(port)
		nodePortProtocol := strings.Split(parts[1], "/")
		port, err = strconv.ParseUint(nodePortProtocol[0], 10, 32)
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid hostPort value: %s", nodePortProtocol[0])
		}
		nodePort = uint32(port)
		if len(nodePortProtocol) == 2 {
			if nodePortProtocol[1] == "tcp" || nodePortProtocol[1] == "udp" {
				protocol = nodePortProtocol[1]
			} else {
				return 0, 0, "", fmt.Errorf("invalid protocol value: %s", nodePortProtocol[1])
			}
		}
	default:
		return 0, 0, "", fmt.Errorf("invalid hostPort format: %s", hostPortStr)
	}

	return hostPort, nodePort, protocol, nil
}

// Ensure TalosService implements Service interface
var _ Service = (*TalosService)(nil)
