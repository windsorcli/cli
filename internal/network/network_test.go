package network

import (
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
)

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	// Given: a DI container
	diContainer := di.NewContainer()

	// When: attempting to create NetworkManager
	_, err := NewNetworkManager(diContainer)

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	// Given: a DI container
	diContainer := di.NewContainer()

	// When: creating a NetworkManager and configuring the guest
	nm, err := NewNetworkManager(diContainer)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = nm.ConfigureGuest()

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
