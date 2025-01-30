package services

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// Initialize the global port settings
var (
	nextAPIPort        = 50001
	defaultAPIPort     = 50000
	portLock           sync.Mutex
	extraPortIndex     = 0
	nextNodePorts      = []string{}
	controlPlaneLeader *TalosService
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

	// Elect a leader for the first controlplane
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
// provided address. It assigns unique API ports starting from 50001, incrementing
// for each node. A mutex is used to safely manage concurrent access to the port allocation.
// The leader controlplane is assigned the default API port. Node ports are configured
// based on the cluster configuration, ensuring no conflicts.
func (s *TalosService) SetAddress(address string) error {
	tld := s.configHandler.GetString("dns.domain", "test")
	nodeType := "workers"
	if s.mode == "controlplane" {
		nodeType = "controlplanes"
	}

	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.hostname", nodeType, s.name), s.name+"."+tld); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.node", nodeType, s.name), address); err != nil {
		return err
	}

	portLock.Lock()
	defer portLock.Unlock()

	var port int
	if s.isLeader {
		port = defaultAPIPort // Reserve 50000 for the leader controlplane
	} else {
		port = nextAPIPort
		nextAPIPort++
	}

	if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, s.name), fmt.Sprintf("%s:%d", address, port)); err != nil {
		return err
	}

	config := s.configHandler.GetConfig()
	if config.Cluster != nil {
		var nodePorts []string
		if s.mode == "controlplane" {
			nodePorts = config.Cluster.ControlPlanes.NodePorts
		} else {
			nodePorts = config.Cluster.Workers.NodePorts
		}

		if nodePorts != nil && (nextNodePorts == nil || len(nextNodePorts) == 0) {
			nextNodePorts = make([]string, len(nodePorts))
			copy(nextNodePorts, nodePorts)
		}

		currentNodePorts := make([]string, len(nextNodePorts))
		copy(currentNodePorts, nextNodePorts)

		if nextNodePorts != nil {
			for i := 0; i < len(nextNodePorts); i++ {
				parts := strings.Split(nextNodePorts[i], ":")
				var hostPort, nodePort int
				protocol := "tcp"

				switch len(parts) {
				case 1: // nodePort only
					var err error
					nodePort, err = strconv.Atoi(parts[0])
					if err != nil {
						return fmt.Errorf("invalid nodePort value: %s", parts[0])
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
						return fmt.Errorf("invalid nodePort value: %s", nodePortProtocol[0])
					}
					if len(nodePortProtocol) == 2 {
						if nodePortProtocol[1] == "tcp" || nodePortProtocol[1] == "udp" {
							protocol = nodePortProtocol[1]
						} else {
							return fmt.Errorf("invalid protocol value: %s", nodePortProtocol[1])
						}
					}
				default:
					return fmt.Errorf("invalid nodePort format: %s", nextNodePorts[i])
				}

				nextNodePorts[i] = fmt.Sprintf("%d:%d/%s", hostPort, nodePort, protocol)
			}
		}

		if err := s.configHandler.SetContextValue(fmt.Sprintf("cluster.%s.nodes.%s.nodeports", nodeType, s.name), currentNodePorts); err != nil {
			return err
		}
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig generates a docker-compose configuration for Talos services. It retrieves CPU and RAM
// settings based on the node type (worker or control plane) and identifies the endpoint ports for service
// communication. The function ensures necessary volume directories are defined and configures the container
// with the appropriate image, environment variables, security options, and volume mounts. It constructs the
// service name using the node name and sets up port mappings, including both default and node-specific ports.
// The resulting configuration includes detailed service and volume specifications for deployment.
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

	// Define a default name if s.name is not set
	nodeName := s.name
	if nodeName == "" {
		nodeName = nodeType[:len(nodeType)-1] // remove 's' from nodeType
	}

	endpoint := s.configHandler.GetString(fmt.Sprintf("cluster.%s.nodes.%s.endpoint", nodeType, nodeName), fmt.Sprintf("%d", defaultAPIPort))
	publishedPort := fmt.Sprintf("%d", defaultAPIPort)
	if parts := strings.Split(endpoint, ":"); len(parts) == 2 {
		publishedPort = parts[1]
	}

	commonConfig := types.ServiceConfig{
		Image:       constants.DEFAULT_TALOS_IMAGE,
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

	// Add bind mount for workers
	if s.mode != "controlplane" {
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
		commonConfig.Volumes = append(commonConfig.Volumes, types.ServiceVolumeConfig{
			Type:   "bind",
			Source: "${WINDSOR_PROJECT_ROOT}/.volumes",
			Target: "/var/local",
		})
	}

	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := nodeName + "." + tld
	if s.name == "" {
		fullName = nodeType[:len(nodeType)-1] + "." + tld
	}

	serviceConfig := commonConfig
	serviceConfig.Name = fullName
	serviceConfig.ContainerName = fullName
	serviceConfig.Hostname = fullName
	serviceConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", cpu, ram*1024)),
	}

	var ports []types.ServicePortConfig

	// Ensure defaultAPIPort is within the valid range for uint32
	if defaultAPIPort < 0 || defaultAPIPort > math.MaxUint32 {
		return nil, fmt.Errorf("defaultAPIPort value out of range: %d", defaultAPIPort)
	}

	ports = append(ports, types.ServicePortConfig{
		Target:    uint32(defaultAPIPort),
		Published: publishedPort,
		Protocol:  "tcp",
	})

	// Add port 6443 forwarding for the leader controlplane
	if s.isLeader {
		ports = append(ports, types.ServicePortConfig{
			Target:    6443,
			Published: "6443",
			Protocol:  "tcp",
		})
	}

	nodePortsKey := fmt.Sprintf("cluster.%s.nodes.%s.nodeports", nodeType, nodeName)
	nodePorts := s.configHandler.GetStringSlice(nodePortsKey)
	for _, nodePortStr := range nodePorts {
		parts := strings.Split(nodePortStr, ":")
		hostPort, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || hostPort > math.MaxUint32 {
			return nil, fmt.Errorf("invalid hostPort value: %s", parts[0])
		}
		nodePortProtocol := strings.Split(parts[1], "/")
		nodePort, err := strconv.ParseUint(nodePortProtocol[0], 10, 32)
		if err != nil || nodePort > math.MaxUint32 {
			return nil, fmt.Errorf("invalid nodePort value: %s", nodePortProtocol[0])
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

	volumes := map[string]types.VolumeConfig{
		strings.ReplaceAll(nodeName+"_system_state", "-", "_"):           {},
		strings.ReplaceAll(nodeName+"_var", "-", "_"):                    {},
		strings.ReplaceAll(nodeName+"_etc_cni", "-", "_"):                {},
		strings.ReplaceAll(nodeName+"_etc_kubernetes", "-", "_"):         {},
		strings.ReplaceAll(nodeName+"_usr_libexec_kubernetes", "-", "_"): {},
		strings.ReplaceAll(nodeName+"_opt", "-", "_"):                    {},
	}

	return &types.Config{
		Services: []types.ServiceConfig{serviceConfig},
		Volumes:  volumes,
	}, nil
}
