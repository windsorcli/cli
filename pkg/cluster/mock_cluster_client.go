// The MockClusterClient is a mock implementation of the ClusterClient interface.
// It provides configurable function fields for testing cluster operations,
// enabling controlled testing scenarios for health checks and node management,
// and allowing test cases to simulate various cluster states and behaviors.

package cluster

import (
	"context"
)

// =============================================================================
// Types
// =============================================================================

// MockClusterClient is a mock implementation of the ClusterClient interface
type MockClusterClient struct {
	BaseClusterClient
	WaitForNodesHealthyFunc func(ctx context.Context, nodeAddresses []string, expectedVersion string) error
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
func (m *MockClusterClient) WaitForNodesHealthy(ctx context.Context, nodeAddresses []string, expectedVersion string) error {
	if m.WaitForNodesHealthyFunc != nil {
		return m.WaitForNodesHealthyFunc(ctx, nodeAddresses, expectedVersion)
	}
	return nil
}

// Close calls the mock CloseFunc if set
func (m *MockClusterClient) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// Ensure MockClusterClient implements ClusterClient
var _ ClusterClient = (*MockClusterClient)(nil)
