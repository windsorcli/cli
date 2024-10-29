package network

import (
	"fmt"
	"testing"
)

func TestMockNetworkManager_Configure(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			NetworkCIDR: "192.168.1.0/24",
			GuestIP:     "192.168.1.2",
		}

		// Create a MockNetworkManager and set the ConfigureFunc
		mockNetworkManager := NewMockNetworkManager()
		mockNetworkManager.SetConfigureFunc(func(config *NetworkConfig) (*NetworkConfig, error) {
			return config, nil
		})

		// Call the Configure method
		config, err := mockNetworkManager.Configure(networkConfig)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate the returned config
		if config.NetworkCIDR != networkConfig.NetworkCIDR || config.GuestIP != networkConfig.GuestIP {
			t.Fatalf("expected config to be %v, got %v", networkConfig, config)
		}
	})

	t.Run("ConfigureFuncError", func(t *testing.T) {
		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			NetworkCIDR: "192.168.1.0/24",
			GuestIP:     "192.168.1.2",
		}

		// Create a MockNetworkManager and set the ConfigureFunc to return an error
		mockNetworkManager := NewMockNetworkManager()
		mockNetworkManager.SetConfigureFunc(func(config *NetworkConfig) (*NetworkConfig, error) {
			return nil, fmt.Errorf("mock error")
		})

		// Call the Configure method and expect an error
		_, err := mockNetworkManager.Configure(networkConfig)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "mock error"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NilConfigureFunc", func(t *testing.T) {
		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			NetworkCIDR: "192.168.1.0/24",
			GuestIP:     "192.168.1.2",
		}

		// Create a MockNetworkManager without setting the ConfigureFunc
		mockNetworkManager := NewMockNetworkManager()

		// Call the Configure method
		config, err := mockNetworkManager.Configure(networkConfig)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate the returned config
		if config.NetworkCIDR != networkConfig.NetworkCIDR || config.GuestIP != networkConfig.GuestIP {
			t.Fatalf("expected config to be %v, got %v", networkConfig, config)
		}
	})
}
