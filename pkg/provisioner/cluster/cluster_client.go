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

	// CaptureNodeBootIDs reads each node's current kernel boot ID, for later comparison
	// by WaitForNodesReboot. A node whose boot ID can't be read (e.g. permission denied,
	// unreachable) is simply omitted from the result; WaitForNodesReboot skips the
	// per-node check for any node missing from preActionBootIDs.
	CaptureNodeBootIDs(ctx context.Context, nodeAddresses []string) map[string]string

	// WaitForNodesReboot waits for nodes to reboot (confirmed by each node's kernel boot ID
	// changing from preActionBootIDs) then come back healthy. Phase 1 polls each node's
	// boot ID until it differs from before or offlineTimeout elapses; a node missing from
	// preActionBootIDs is treated as already satisfied. Phase 2 polls until all nodes are
	// healthy again within the remaining context deadline.
	WaitForNodesReboot(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error

	// UpgradeNodes upgrades the specified nodes to the specified image. powercycle requests
	// a full ACPI reboot instead of the default kexec.
	UpgradeNodes(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error

	// WaitForControlPlaneAPIReady waits for the kube-apiserver on a control-plane node
	// to accept TCP connections on port 6443. Returns nil immediately when the node is
	// not a control-plane (i.e. no etcd service present). outputFunc, when non-nil,
	// receives status messages during the polling loop. Returns an error if the node
	// role cannot be determined or the apiserver does not become reachable before the
	// context deadline.
	WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string, outputFunc func(string)) error

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

// CaptureNodeBootIDs is a stub that returns an empty map.
// Provider-specific implementations should override this.
func (c *BaseClusterClient) CaptureNodeBootIDs(ctx context.Context, nodeAddresses []string) map[string]string {
	return nil
}

// WaitForNodesReboot implements the default reboot-wait behavior
func (c *BaseClusterClient) WaitForNodesReboot(ctx context.Context, nodeAddresses []string, preActionBootIDs map[string]string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
	return fmt.Errorf("WaitForNodesReboot not implemented")
}

// UpgradeNodes is a stub that returns an error indicating the method is not implemented.
// Provider-specific implementations should override this to perform node upgrades.
func (c *BaseClusterClient) UpgradeNodes(ctx context.Context, nodeAddresses []string, image string, powercycle bool) error {
	return fmt.Errorf("UpgradeNodes not implemented")
}

// =============================================================================
// Private Methods
// =============================================================================

// WaitForControlPlaneAPIReady is a stub that returns an error indicating the method
// is not implemented. Provider-specific implementations should override this.
func (c *BaseClusterClient) WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string, outputFunc func(string)) error {
	return fmt.Errorf("WaitForControlPlaneAPIReady not implemented")
}
