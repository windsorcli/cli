package services

import (
	"fmt"
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
	localhostAPIPort = 50001
	defaultAPIPort   = 50000
	portLock         sync.Mutex
	extraPortIndex   = 0
	nextNodePorts    = []string{}
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

// SetAddress configures the Talos worker's hostname and endpoint using the
// provided address. For localhost addresses, it assigns unique ports starting
// from 50001, incrementing for each node. A mutex is used to safely increment
// the port for each localhost node. Node ports are adjusted to avoid conflicts.
// Each node is assigned a unique host port by incrementing the port number.
func (s *TalosWorkerService) SetAddress(address string) error {
	tld := s.configHandler.GetString("dns.name", "test")

	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".hostname", s.name+"."+tld); err != nil {
		return err
	}
	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".node", address); err != nil {
		return err
	}

	port := defaultAPIPort
	if isLocalhost(address) {
		portLock.Lock()
		port = localhostAPIPort
		localhostAPIPort++
		portLock.Unlock()
	}

	if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".endpoint", fmt.Sprintf("%s:%d", address, port)); err != nil {
		return err
	}

	config := s.configHandler.GetConfig()
	if config.Cluster != nil {
		nodePorts := config.Cluster.NodePorts
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

				if isLocalhost(address) {
					nextNodePorts[i] = fmt.Sprintf("%d:%d/%s", hostPort+1, nodePort, protocol)
				} else {
					nextNodePorts[i] = fmt.Sprintf("%d:%d/%s", hostPort, nodePort, protocol)
				}
			}
		}

		if err := s.configHandler.SetContextValue("cluster.workers.nodes."+s.name+".nodeports", currentNodePorts); err != nil {
			return err
		}
	}

	return s.BaseService.SetAddress(address)
}

// GetComposeConfig creates a docker-compose setup for Talos workers. It fetches CPU/RAM settings,
// determines endpoint ports, and ensures the .volumes directory exists. The function configures
// the container with image, environment, security, and volume settings. It constructs the service
// name using the TLD and sets up port mappings for network communication, including default and
// node-specific ports. The final configuration includes service and volume details for deployment.
func (s *TalosWorkerService) GetComposeConfig() (*types.Config, error) {
	config := s.configHandler.GetConfig()
	if config.Cluster == nil {
		return &types.Config{
			Services: []types.ServiceConfig{},
			Volumes:  map[string]types.VolumeConfig{},
		}, nil
	}

	workerCPU := s.configHandler.GetInt("cluster.workers.cpu", constants.DEFAULT_TALOS_WORKER_CPU)
	workerRAM := s.configHandler.GetInt("cluster.workers.memory", constants.DEFAULT_TALOS_WORKER_RAM)

	// Define a default name if s.name is not set
	nodeName := s.name
	if nodeName == "" {
		nodeName = "worker"
	}

	endpoint := s.configHandler.GetString("cluster.workers.nodes."+nodeName+".endpoint", fmt.Sprintf("%d", defaultAPIPort))
	publishedPort := fmt.Sprintf("%d", defaultAPIPort)
	if parts := strings.Split(endpoint, ":"); len(parts) == 2 {
		publishedPort = parts[1]
	}

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
			{Type: "bind", Source: "${WINDSOR_PROJECT_ROOT}/.volumes", Target: "/var/local"},
		},
	}

	tld := s.configHandler.GetString("dns.name", "test")
	fullName := nodeName + "." + tld

	workerConfig := commonConfig
	workerConfig.Name = fullName
	workerConfig.ContainerName = fullName
	workerConfig.Hostname = fullName
	workerConfig.Environment = map[string]*string{
		"PLATFORM": ptrString("container"),
		"TALOSSKU": ptrString(fmt.Sprintf("%dCPU-%dRAM", workerCPU, workerRAM*1024)),
	}

	var ports []types.ServicePortConfig
	ports = append(ports, types.ServicePortConfig{
		Target:    uint32(defaultAPIPort),
		Published: publishedPort,
		Protocol:  "tcp",
	})

	nodePortsKey := "cluster.workers.nodes." + nodeName + ".nodeports"
	nodePorts := s.configHandler.GetStringSlice(nodePortsKey)
	for _, nodePortStr := range nodePorts {
		parts := strings.Split(nodePortStr, ":")
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid hostPort value: %s", parts[0])
		}
		nodePortProtocol := strings.Split(parts[1], "/")
		nodePort, err := strconv.Atoi(nodePortProtocol[0])
		if err != nil {
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

	workerConfig.Ports = ports

	volumes := map[string]types.VolumeConfig{
		strings.ReplaceAll(nodeName+"_system_state", "-", "_"):           {},
		strings.ReplaceAll(nodeName+"_var", "-", "_"):                    {},
		strings.ReplaceAll(nodeName+"_etc_cni", "-", "_"):                {},
		strings.ReplaceAll(nodeName+"_etc_kubernetes", "-", "_"):         {},
		strings.ReplaceAll(nodeName+"_usr_libexec_kubernetes", "-", "_"): {},
		strings.ReplaceAll(nodeName+"_opt", "-", "_"):                    {},
	}

	return &types.Config{
		Services: []types.ServiceConfig{workerConfig},
		Volumes:  volumes,
	}, nil
}
