package network

import (
	"github.com/windsor-hotel/cli/internal/di"
)

type NetworkConfig struct {
	HostRouteCIDR          string // CIDR for the host route
	HostRouteIP            string // IP address for the host route
	ColimaNetworkInterface string // Interface name for Colima
	DockerBridgeInterface  string // Interface name for Docker bridge
	ColimaHostIP           string // IP address of the Colima host
	ClusterIPv4CIDR        string // CIDR for the cluster network
	DNSDomain              string // Domain name for DNS configuration
	DNSIP                  string // IP address for custom DNS
}

// NetworkManager handles configuring the local development network
type NetworkManager interface {
	// Configure sets up the local development network
	Configure() error
}

// networkManager is a concrete implementation of NetworkManager
type networkManager struct {
	diContainer *di.DIContainer
}

// NewNetworkManager creates a new NetworkManager
func NewNetworkManager(di *di.DIContainer) (NetworkManager, error) {
	return &networkManager{
		diContainer: di,
	}, nil
}
