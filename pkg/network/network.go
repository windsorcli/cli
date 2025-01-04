package network

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/ssh"
)

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	// Initialize the network manager
	Initialize() error
	// ConfigureHostRoute sets up the local development network
	ConfigureHostRoute() error
	// ConfigureGuest sets up the guest VM network
	ConfigureGuest() error
	// ConfigureDNS sets up the DNS configuration
	ConfigureDNS() error
}

// BaseNetworkManager is a concrete implementation of NetworkManager
type BaseNetworkManager struct {
	injector                 di.Injector
	sshClient                ssh.Client
	shell                    shell.Shell
	secureShell              shell.Shell
	configHandler            config.ConfigHandler
	contextHandler           context.ContextHandler
	networkInterfaceProvider NetworkInterfaceProvider
	services                 []services.Service
	isLocalhost              bool
}

// NewNetworkManager creates a new NetworkManager
func NewBaseNetworkManager(injector di.Injector) (*BaseNetworkManager, error) {
	nm := &BaseNetworkManager{
		injector: injector,
	}
	return nm, nil
}

// Initialize resolves dependencies, sorts services, and assigns IPs based on network CIDR
func (n *BaseNetworkManager) Initialize() error {
	shellInterface, ok := n.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}
	n.shell = shellInterface

	configHandler, ok := n.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	n.configHandler = configHandler

	contextHandler, ok := n.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("failed to resolve context handler")
	}
	n.contextHandler = contextHandler

	resolvedServices, err := n.injector.ResolveAll(new(services.Service))
	if err != nil {
		return fmt.Errorf("error resolving services: %w", err)
	}

	var serviceList []services.Service
	for _, serviceInterface := range resolvedServices {
		service, _ := serviceInterface.(services.Service)
		serviceList = append(serviceList, service)
	}

	sort.Slice(serviceList, func(i, j int) bool {
		return serviceList[i].GetName() < serviceList[j].GetName()
	})

	// Determine if the VM driver is docker-desktop to set localhost mode
	vmDriver := n.configHandler.GetString("vm.driver")
	n.isLocalhost = vmDriver == "docker-desktop"

	if n.isLocalhost {
		for _, service := range serviceList {
			if err := service.SetAddress("127.0.0.1"); err != nil {
				return fmt.Errorf("error setting address for service: %w", err)
			}
		}
	} else {
		// Ensure network CIDR is set, defaulting if necessary
		networkCIDR := n.configHandler.GetString("docker.network_cidr")
		if networkCIDR == "" {
			networkCIDR = constants.DEFAULT_NETWORK_CIDR
			return n.configHandler.SetContextValue("docker.network_cidr", networkCIDR)
		}
		if err := assignIPAddresses(serviceList, &networkCIDR); err != nil {
			return fmt.Errorf("error assigning IP addresses: %w", err)
		}
	}

	n.services = serviceList

	return nil
}

// ConfigureGuest sets up the guest VM network
func (n *BaseNetworkManager) ConfigureGuest() error {
	// no-op
	return nil
}

// updateHostsFile manages DNS entries in the hosts file. It ensures the file reflects the current
// isLocalhost state by adding or removing the DNS entry for the given domain. If isLocalhost is
// true, it adds an entry mapping the domain to 127.0.0.1. If false, it removes the entry. The
// function checks for changes and only updates the file if necessary. It handles both Windows
// and Unix-like systems, using appropriate file paths and commands for each.
func (n *BaseNetworkManager) updateHostsFile(dnsDomain string) error {
	var hostsFile, tempHostsFile string

	switch goos() {
	case "windows":
		hostsFile = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		tempHostsFile = "C:\\Windows\\Temp\\hosts"
	default:
		hostsFile = "/etc/hosts"
		tempHostsFile = "/tmp/hosts"
	}

	existingContent, err := readFile(hostsFile)
	if err != nil {
		return fmt.Errorf("Error reading hosts file: %w", err)
	}

	hostsEntry := fmt.Sprintf("127.0.0.1 %s", dnsDomain)
	lines := strings.Split(string(existingContent), "\n")
	entryExists, changed := false, false

	for i, line := range lines {
		if strings.TrimSpace(line) == hostsEntry {
			entryExists = true
			if !n.isLocalhost {
				lines = append(lines[:i], lines[i+1:]...)
				changed = true
			}
			break
		}
	}

	if n.isLocalhost && !entryExists {
		lines = append(lines, hostsEntry)
		changed = true
	}

	if !changed {
		return nil
	}

	updatedContent := strings.Join(lines, "\n")
	if err := writeFile(tempHostsFile, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("Error writing to temporary hosts file: %w", err)
	}

	switch goos() {
	case "windows":
		if _, err := n.shell.ExecSudo(
			"🔐 Updating hosts file",
			"cmd",
			"/C",
			fmt.Sprintf("copy /Y %s %s", tempHostsFile, hostsFile),
		); err != nil {
			return fmt.Errorf("Error updating hosts file: %w", err)
		}
	default:
		if _, err := n.shell.ExecSudo(
			"🔐 Updating /etc/hosts",
			"mv",
			tempHostsFile,
			hostsFile,
		); err != nil {
			return fmt.Errorf("Error updating hosts file: %w", err)
		}
	}

	return nil
}

// Ensure BaseNetworkManager implements NetworkManager
var _ NetworkManager = (*BaseNetworkManager)(nil)

// assignIPAddresses assigns IP addresses to services based on the network CIDR.
var assignIPAddresses = func(services []services.Service, networkCIDR *string) error {
	if networkCIDR == nil || *networkCIDR == "" {
		return fmt.Errorf("network CIDR is not defined")
	}

	ip, ipNet, err := net.ParseCIDR(*networkCIDR)
	if err != nil {
		return fmt.Errorf("error parsing network CIDR: %w", err)
	}

	// Skip the network address
	ip = incrementIP(ip)

	// Skip the first IP address
	ip = incrementIP(ip)

	for i := range services {
		if err := services[i].SetAddress(ip.String()); err != nil {
			return fmt.Errorf("error setting address for service: %w", err)
		}
		ip = incrementIP(ip)
		if !ipNet.Contains(ip) {
			return fmt.Errorf("not enough IP addresses in the CIDR range")
		}
	}

	return nil
}

// incrementIP increments an IP address by one
func incrementIP(ip net.IP) net.IP {
	ip = ip.To4()
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
	return ip
}
