package network

import (
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
)

func TestNewNetworkManager(t *testing.T) {
	// Given: a DI container
	diContainer := di.NewContainer()

	// When: attempting to create NetworkManager
	_, err := NewNetworkManager(diContainer)

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
