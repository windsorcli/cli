// The ClusterClient is a base interface for cluster node operations.
// It provides a common interface for health checks and management operations,
// serving as the foundation for provider-specific cluster clients,
// and enabling consistent cluster management across different providers.

package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
)

// ClusterClient defines the interface for cluster operations
type ClusterClient interface {
	// WaitForNodesHealthy waits for nodes to be healthy and optionally match a specific version
	// Polls until all nodes are healthy (and correct version if specified) or timeout
	// skipServices is a list of service names to ignore during health checks
	WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error

	// WaitForNodesReboot waits for nodes to go offline (reboot started) then come back healthy.
	// Phase 1 polls the Talos version endpoint until all nodes are unreachable (offlineTimeout caps this phase).
	// Phase 2 polls until all nodes are healthy again within the remaining context deadline.
	WaitForNodesReboot(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error

	// UpgradeNodes upgrades the specified nodes to the specified image
	UpgradeNodes(ctx context.Context, nodeAddresses []string, image string) error

	// WaitForControlPlaneAPIReady waits for the kube-apiserver on a control-plane node
	// to accept TCP connections on port 6443. Returns nil immediately when the node is
	// not a control-plane (i.e. no etcd service present). Returns an error if the node
	// role cannot be determined or the apiserver does not become reachable before the
	// context deadline.
	WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string) error

	// Close closes any open connections.
	Close()
}

// BaseClusterClient provides a base implementation of ClusterClient.
type BaseClusterClient struct {
	// Configurable timeouts
	healthCheckTimeout      time.Duration
	healthCheckPollInterval time.Duration
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseClusterClient creates a new BaseClusterClient with default timeouts.
func NewBaseClusterClient() *BaseClusterClient {
	return &BaseClusterClient{
		healthCheckTimeout:      constants.DefaultNodeHealthCheckTimeout,
		healthCheckPollInterval: constants.DefaultNodeHealthCheckPollInterval,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Close is a no-op in the base implementation.
// Provider-specific implementations should override this to close their connections.
func (c *BaseClusterClient) Close() {
	// Base implementation does nothing
}

// WaitForNodesHealthy implements the default polling behavior for node health and version checks
func (c *BaseClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
	return fmt.Errorf("WaitForNodesHealthy not implemented")
}

// WaitForNodesReboot implements the default reboot-wait behavior
func (c *BaseClusterClient) WaitForNodesReboot(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
	return fmt.Errorf("WaitForNodesReboot not implemented")
}

// UpgradeNodes is a stub that returns an error indicating the method is not implemented.
// Provider-specific implementations should override this to perform node upgrades.
func (c *BaseClusterClient) UpgradeNodes(ctx context.Context, nodeAddresses []string, image string) error {
	return fmt.Errorf("UpgradeNodes not implemented")
}

// =============================================================================
// Private Methods
// =============================================================================

// WaitForControlPlaneAPIReady is a stub that returns an error indicating the method
// is not implemented. Provider-specific implementations should override this.
func (c *BaseClusterClient) WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string) error {
	return fmt.Errorf("WaitForControlPlaneAPIReady not implemented")
}
