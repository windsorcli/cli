//go:build windows
// +build windows

package network

import (
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func stringPtr(s string) *string {
	return &s
}

func TestWindowsNetworkManager_Configure(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			HostRouteCIDR: "192.168.1.0/24",
			GuestIP:       "192.168.1.2",
		}

		// Create a mock shell
		mockShell := shell.NewMockShell("mock")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("shell", mockShell)

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the Configure method
		_, err = nm.Configure(networkConfig)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		// Create a mock DI container that does not register the shell
		diContainer := di.NewMockContainer()
		diContainer.SetResolveError("shell", fmt.Errorf("shell not found"))

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(diContainer.DIContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			HostRouteCIDR: "192.168.1.0/24",
			GuestIP:       "192.168.1.2",
		}

		// Call the Configure method and expect an error due to unresolved shell
		_, err = nm.Configure(networkConfig)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Create a mock shell
		mockShell := shell.NewMockShell("mock")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "mock output", fmt.Errorf("mock error")
		}

		// Create a mock DI container
		diContainer := di.NewContainer()
		diContainer.Register("shell", mockShell)

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(diContainer)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Create a mock NetworkConfig
		networkConfig := &NetworkConfig{
			HostRouteCIDR: "192.168.1.0/24",
			GuestIP:       "192.168.1.2",
		}

		// Call the Configure method and expect an error due to failed route addition
		_, err = nm.Configure(networkConfig)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add route: mock error, output: mock output"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
