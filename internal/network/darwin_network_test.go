//go:build darwin
// +build darwin

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

func TestDarwinNetworkManager_ConfigureHost(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock shell
		mockShell := shell.NewMockShell(di.NewInjector())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", nil
		}

		// Create a mock DI container
		injector := di.NewInjector()
		injector.Register("shell", mockShell)

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureHost method
		err = nm.ConfigureHost()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		// Create a mock DI container that does not register the shell
		injector := di.NewInjector()
		injector.Register("shell", shell.NewMockShell(injector))

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureHost method and expect an error due to unresolved shell
		err = nm.ConfigureHost()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	t.Run("AddRouteError", func(t *testing.T) {
		// Create a mock shell
		mockShell := shell.NewMockShell(di.NewInjector())
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "mock output", fmt.Errorf("mock error")
		}

		// Create a mock DI container
		injector := di.NewInjector()
		injector.Register("shell", mockShell)

		// Create a networkManager using NewNetworkManager with the mock DI container
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Call the ConfigureHost method and expect an error due to failed route addition
		err = nm.ConfigureHost()
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to add route: mock error, output: mock output"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}
