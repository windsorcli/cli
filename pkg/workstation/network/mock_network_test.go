package network

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock network manager with successful initialization
		mockManager := NewMockNetworkManager()
		mockManager.InitializeFunc = func() error {
			return nil
		}

		// When initializing the manager
		err := mockManager.Initialize()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		// Given a mock network manager with no initialization function
		mockManager := NewMockNetworkManager()

		// When initializing the manager
		err := mockManager.Initialize()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock network manager with successful host route configuration
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureHostRouteFunc = func() error {
			return nil
		}

		// When configuring the host route
		err := mockManager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		// Given a mock network manager with no host route configuration function
		mockManager := NewMockNetworkManager()

		// When configuring the host route
		err := mockManager.ConfigureHostRoute()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock network manager with successful guest configuration
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureGuestFunc = func() error {
			return nil
		}

		// When configuring the guest
		err := mockManager.ConfigureGuest()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		// Given a mock network manager with no guest configuration function
		mockManager := NewMockNetworkManager()

		// When configuring the guest
		err := mockManager.ConfigureGuest()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock network manager with successful DNS configuration
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureDNSFunc = func() error {
			return nil
		}

		// When configuring DNS
		err := mockManager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		// Given a mock network manager with no DNS configuration function
		mockManager := NewMockNetworkManager()

		// When configuring DNS
		err := mockManager.ConfigureDNS()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
