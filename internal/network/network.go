package network

import (
	"github.com/windsor-hotel/cli/internal/di"
)

type NetworkConfig struct {
	HostRouteCIDR     string // CIDR block for the host route
	GuestIP           string // IP address assigned to the guest VM
	GuestInInterface  string // Interface name from the host to the guest VM
	GuestOutInterface string // Interface name for the cluster bridge network
	DestinationCIDR   string // CIDR block for the cluster network
	DNSDomain         string // Domain name used for DNS configuration
	DNSIP             string // IP address for the custom DNS server
}

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	// Configure sets up the local development network
	Configure(*NetworkConfig) (*NetworkConfig, error)
}

// networkManager is a concrete implementation of NetworkManager
type networkManager struct {
	diContainer di.ContainerInterface
}

// NewNetworkManager creates a new NetworkManager
func NewNetworkManager(container di.ContainerInterface) (NetworkManager, error) {
	return &networkManager{
		diContainer: container,
	}, nil
}
