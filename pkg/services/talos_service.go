package services

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// Initialize the global port settings
var (
	nextAPIPort        = constants.DEFAULT_TALOS_API_PORT + 1
	defaultAPIPort     = constants.DEFAULT_TALOS_API_PORT
	portLock           sync.Mutex
	extraPortIndex     = 0
	controlPlaneLeader *TalosService
	usedHostPorts      = make(map[int]bool)
)

type TalosService struct {
	BaseService
	mode     string
	isLeader bool
}

// NewTalosService is a constructor for TalosService
func NewTalosService(injector di.Injector, mode string) *TalosService {
	service := &TalosService{
		BaseService: BaseService{
			injector: injector,
		},
		mode: mode,
	}

	// Elect a "leader" for the first controlplane
	if mode == "controlplane" {
		portLock.Lock()
		defer portLock.Unlock()
		if controlPlaneLeader == nil {
			controlPlaneLeader = service
			service.isLeader = true
		}
	}

	return service
}

// SetAddress configures the Talos service's hostname and endpoint using the
// provided address. It assigns the default API port to the leader controlplane
// or a unique port if the address is not local. For other nodes, it assigns
// unique API ports starting from 50001, incrementing for each node. A mutex
// is used to safely manage concurrent access to the port allocation. Node ports
// are configured based on the cluster configuration, ensuring no conflicts.
func (s *TalosService) SetAddress(address string) error {
	if err := s.BaseService.SetAddress(address); err != nil {
		return err
	}

	tld := s.configHandler.GetString("dns.domain", "test")
	nodeType := "workers"
	if s.mode == "controlplane" {
		nodeType = "controlplanes"
	}

	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.hostname", nodeType, s.name), s.name); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.node", nodeType, s.name), s.name); err != nil {
		return err
	}

	portLock.Lock()
	defer portLock.Unlock()

	var port int
	if s.isLeader || !s.isLocalhostMode() {
		port = defaultAPIPort
	} else {
		port = nextAPIPort
		nextAPIPort++
	}

	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, s.name), fmt.Sprintf("%s.%s:%d", s.name, tld, port)); err != nil {
		return err
	}

	hostPorts := s.configHandler.GetStringSlice(fmt.Sprintf("cluster.%s.hostports", nodeType), []string{})

	hostPortsCopy := make([]string, len(hostPorts))
	copy(hostPortsCopy, hostPorts)

	for i, hostPortStr := range hostPortsCopy {
		parts := strings.Split(hostPortStr, ":")
		var hostPort, nodePort int
		protocol := "tcp"

		switch len(parts) {
		case 1: // hostPort only
			var err error
			nodePort, err = strconv.Atoi(parts[0])
			if err != nil {
				return fmt.Errorf("invalid hostPort value: %s", parts[0])
			}
			hostPort = nodePort
		case 2: // hostPort and nodePort/protocol
			var err error
			hostPort, err = strconv.Atoi(parts[0])
			if err != nil {
				return fmt.Errorf("invalid hostPort value: %s", parts[0])
			}
			nodePortProtocol := strings.Split(parts[1], "/")
			nodePort, err = strconv.Atoi(nodePortProtocol[0])
			if err != nil {
				return fmt.Errorf("invalid hostPort value: %s", nodePortProtocol[0])
			}
			if len(nodePortProtocol) == 2 {
				if nodePortProtocol[1] == "tcp" || nodePortProtocol[1] == "udp" {
					protocol = nodePortProtocol[1]
				} else {
					return fmt.Errorf("invalid protocol value: %s", nodePortProtocol[1])
				}
			}
		default:
			return fmt.Errorf("invalid hostPort format: %s", hostPortStr)
		}

		// Check for conflicts in hostPort
		for usedHostPorts[hostPort] {
			hostPort++
		}
		usedHostPorts[hostPort] = true

		hostPortsCopy[i] = fmt.Sprintf("%d:%d/%s", hostPort, nodePort, protocol)
	}

	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.hostports", nodeType, s.name), hostPortsCopy); err != nil {
		return err
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
			Services: []types.ServiceConfig{},
			Volumes:  map[string]types.VolumeConfig{},
		}, nil
	}

	var cpu, ram int
	nodeType := "workers"
	if s.mode == "controlplane" {
		nodeType = "controlplanes"
		cpu = s.configHandler.GetInt("cluster.controlplanes.cpu", constants.DEFAULT_TALOS_CONTROL_PLANE_CPU)
		ram = s.configHandler.GetInt("cluster.controlplanes.memory", constants.DEFAULT_TALOS_CONTROL_PLANE_RAM)
	} else {
		cpu = s.configHandler.GetInt("cluster.workers.cpu", constants.DEFAULT_TALOS_WORKER_CPU)
		ram = s.configHandler.GetInt("cluster.workers.memory", constants.DEFAULT_TALOS_WORKER_RAM)
	}

	nodeName := s.name
	if nodeName == "" {
		nodeName = nodeType[:len(nodeType)-1] // remove 's' from nodeType
	}

	endpoint := s.configHandler.GetString(fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, nodeName), fmt.Sprintf("%d", defaultAPIPort))
	publishedPort := fmt.Sprintf("%d", defaultAPIPort)
	if parts := strings.Split(endpoint, ":"); len(parts) == 2 {
		publishedPort = parts[1]
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
				image = constants.DEFAULT_TALOS_IMAGE
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
		if err := mkdirAll(expandedSourcePath, os.ModePerm); err != nil {
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
		parts := strings.Split(hostPortStr, ":")
		hostPort, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || hostPort > math.MaxUint32 {
			return nil, fmt.Errorf("invalid hostPort value: %s", parts[0])
		}
		nodePortProtocol := strings.Split(parts[1], "/")
		nodePort, err := strconv.ParseUint(nodePortProtocol[0], 10, 32)
		if err != nil || nodePort > math.MaxUint32 {
			return nil, fmt.Errorf("invalid hostPort value: %s", nodePortProtocol[0])
		}
		protocol := "tcp"
		if len(nodePortProtocol) == 2 {
			protocol = nodePortProtocol[1]
		}
		ports = append(ports, types.ServicePortConfig{
			Target:    uint32(nodePort),
			Published: fmt.Sprintf("%d", hostPort),
			Protocol:  protocol,
		})
	}

	serviceConfig.Ports = ports

	dnsAddress := s.configHandler.GetString("dns.address")
	if dnsAddress != "" {
		if serviceConfig.DNS == nil {
			serviceConfig.DNS = []string{}
		}

		dnsExists := slices.Contains(serviceConfig.DNS, dnsAddress)

		if !dnsExists {
			serviceConfig.DNS = append(serviceConfig.DNS, dnsAddress)
		}
	}

	volumesMap := map[string]types.VolumeConfig{
		strings.ReplaceAll(nodeName+"_system_state", "-", "_"):           {},
		strings.ReplaceAll(nodeName+"_var", "-", "_"):                    {},
		strings.ReplaceAll(nodeName+"_etc_cni", "-", "_"):                {},
		strings.ReplaceAll(nodeName+"_etc_kubernetes", "-", "_"):         {},
		strings.ReplaceAll(nodeName+"_usr_libexec_kubernetes", "-", "_"): {},
		strings.ReplaceAll(nodeName+"_opt", "-", "_"):                    {},
	}

	return &types.Config{
		Services: []types.ServiceConfig{serviceConfig},
		Volumes:  volumesMap,
	}, nil
}
