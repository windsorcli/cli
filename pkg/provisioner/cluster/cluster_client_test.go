package cluster

import (
	"context"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBaseClusterClient(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// When creating a new base cluster client
		client := NewBaseClusterClient()

		// Then it should not be nil
		if client == nil {
			t.Error("Expected non-nil BaseClusterClient")
		}

		// Then it should have default timeout values
		if client.healthCheckTimeout != constants.DefaultNodeHealthCheckTimeout {
			t.Errorf("Expected healthCheckTimeout %v, got %v", constants.DefaultNodeHealthCheckTimeout, client.healthCheckTimeout)
		}

		if client.healthCheckPollInterval != constants.DefaultNodeHealthCheckPollInterval {
			t.Errorf("Expected healthCheckPollInterval %v, got %v", constants.DefaultNodeHealthCheckPollInterval, client.healthCheckPollInterval)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseClusterClient_Close(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		client := NewBaseClusterClient()

		client.Close()

		if client == nil {
			t.Error("Expected client to remain non-nil after Close")
		}
	})

	t.Run("CanBeCalledMultipleTimes", func(t *testing.T) {
		client := NewBaseClusterClient()

		client.Close()
		client.Close()
		client.Close()

		if client == nil {
			t.Error("Expected client to remain non-nil after multiple Close calls")
		}
	})
}

func TestBaseClusterClient_WaitForNodesHealthy(t *testing.T) {
	t.Run("NotImplementedError", func(t *testing.T) {
		// Given a base cluster client
		client := NewBaseClusterClient()
		ctx := context.Background()
		nodeAddresses := []string{"10.0.0.1", "10.0.0.2"}
		expectedVersion := "v1.0.0"

		// When calling WaitForNodesHealthy
		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)

		// Then it should return not implemented error
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedMsg := "WaitForNodesHealthy not implemented"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}
	})

	t.Run("NotImplementedErrorEmptyNodes", func(t *testing.T) {
		// Given a base cluster client
		client := NewBaseClusterClient()
		ctx := context.Background()
		nodeAddresses := []string{}
		expectedVersion := ""

		// When calling WaitForNodesHealthy with empty parameters
		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)

		// Then it should still return not implemented error
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedMsg := "WaitForNodesHealthy not implemented"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}
	})

	t.Run("NotImplementedErrorCancelledContext", func(t *testing.T) {
		// Given a base cluster client and cancelled context
		client := NewBaseClusterClient()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		nodeAddresses := []string{"10.0.0.1"}
		expectedVersion := "v1.0.0"

		// When calling WaitForNodesHealthy with cancelled context
		err := client.WaitForNodesHealthy(ctx, nodeAddresses, expectedVersion)

		// Then it should return not implemented error (not context error)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedMsg := "WaitForNodesHealthy not implemented"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}
	})
}
