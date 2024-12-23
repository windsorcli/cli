package network

import (
	"testing"
)

func TestMockNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockManager := NewMockNetworkManager()
		mockManager.InitializeFunc = func() error {
			return nil
		}

		err := mockManager.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		mockManager := NewMockNetworkManager()

		err := mockManager.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureHostRoute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureHostRouteFunc = func() error {
			return nil
		}

		err := mockManager.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		mockManager := NewMockNetworkManager()

		err := mockManager.ConfigureHostRoute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureGuest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureGuestFunc = func() error {
			return nil
		}

		err := mockManager.ConfigureGuest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		mockManager := NewMockNetworkManager()

		err := mockManager.ConfigureGuest()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockNetworkManager_ConfigureDNS(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockManager := NewMockNetworkManager()
		mockManager.ConfigureDNSFunc = func() error {
			return nil
		}

		err := mockManager.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		mockManager := NewMockNetworkManager()

		err := mockManager.ConfigureDNS()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
