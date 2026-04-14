// The MockClusterClient is a mock implementation of the ClusterClient interface.
// It provides configurable function fields for testing cluster operations,
// enabling controlled testing scenarios for health checks and node management,
// and allowing test cases to simulate various cluster states and behaviors.

package cluster

import (
	"context"
	"time"
)

// =============================================================================
// Types
// =============================================================================

// MockClusterClient is a mock implementation of the ClusterClient interface
type MockClusterClient struct {
	BaseClusterClient
	WaitForNodesHealthyFunc func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error
	WaitForNodesRebootFunc  func(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error
	UpgradeNodesFunc        func(ctx context.Context, nodeAddresses []string, image string) error
	WaitForControlPlaneAPIReadyFunc func(ctx context.Context, nodeAddress string) error
	CloseFunc               func()
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockClusterClient is a constructor for MockClusterClient
func NewMockClusterClient() *MockClusterClient {
	return &MockClusterClient{}
}

// =============================================================================
// Public Methods
// =============================================================================

// WaitForNodesHealthy calls the mock WaitForNodesHealthyFunc if set, otherwise returns nil
func (m *MockClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string) error {
	if m.WaitForNodesHealthyFunc != nil {
		return m.WaitForNodesHealthyFunc(ctx, nodeAddresses, expectedVersion, skipServices)
	}
	return nil
}

// WaitForNodesReboot calls the mock WaitForNodesRebootFunc if set, otherwise returns nil
func (m *MockClusterClient) WaitForNodesReboot(ctx context.Context, nodeAddresses []string, expectedVersion string, skipServices []string, offlineTimeout time.Duration) error {
	if m.WaitForNodesRebootFunc != nil {
		return m.WaitForNodesRebootFunc(ctx, nodeAddresses, expectedVersion, skipServices, offlineTimeout)
	}
	return nil
}

// UpgradeNodes calls the mock UpgradeNodesFunc if set, otherwise returns nil
func (m *MockClusterClient) UpgradeNodes(ctx context.Context, nodeAddresses []string, image string) error {
	if m.UpgradeNodesFunc != nil {
		return m.UpgradeNodesFunc(ctx, nodeAddresses, image)
	}
	return nil
}

// WaitForControlPlaneAPIReady calls the mock WaitForControlPlaneAPIReadyFunc if set, otherwise returns nil
func (m *MockClusterClient) WaitForControlPlaneAPIReady(ctx context.Context, nodeAddress string) error {
	if m.WaitForControlPlaneAPIReadyFunc != nil {
		return m.WaitForControlPlaneAPIReadyFunc(ctx, nodeAddress)
	}
	return nil
}

// Close calls the mock CloseFunc if set
func (m *MockClusterClient) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockClusterClient implements ClusterClient
var _ ClusterClient = (*MockClusterClient)(nil)
