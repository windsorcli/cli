package cluster

import (
	"context"
	"fmt"
	"testing"
)

// =============================================================================
// Public Methods
// =============================================================================

func TestMockClusterClient_WaitForNodesHealthy(t *testing.T) {
	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with configured function
		client := NewMockClusterClient()
		errVal := fmt.Errorf("err")
		client.WaitForNodesHealthyFunc = func(ctx context.Context, addresses []string, version string) error {
			return errVal
		}

		// When calling WaitForNodesHealthy
		err := client.WaitForNodesHealthy(context.Background(), []string{"10.0.0.1"}, "v1.0.0")

		// Then it should return the expected error
		if err != errVal {
			t.Errorf("Expected err, got %v", err)
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock without configured function
		client := NewMockClusterClient()

		// When calling WaitForNodesHealthy
		err := client.WaitForNodesHealthy(context.Background(), []string{"10.0.0.1"}, "v1.0.0")

		// Then it should return nil
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestMockClusterClient_Close(t *testing.T) {
	t.Run("FuncSet", func(t *testing.T) {
		// Given a mock with configured function
		client := NewMockClusterClient()
		called := false
		client.CloseFunc = func() {
			called = true
		}

		// When calling Close
		client.Close()

		// Then the function should be called
		if !called {
			t.Error("Expected CloseFunc to be called")
		}
	})

	t.Run("FuncNotSet", func(t *testing.T) {
		// Given a mock without configured function
		client := NewMockClusterClient()

		// When calling Close
		// Then it should not panic
		client.Close()
	})
}
