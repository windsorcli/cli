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
	WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string) error

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
		healthCheckTimeout:      constants.DEFAULT_NODE_HEALTH_CHECK_TIMEOUT,
		healthCheckPollInterval: constants.DEFAULT_NODE_HEALTH_CHECK_POLL_INTERVAL,
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
func (c *BaseClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
	return fmt.Errorf("WaitForNodesHealthy not implemented")
}
