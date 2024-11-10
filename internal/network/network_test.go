package network

import (
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
)

func TestNetworkManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if nm == nil {
			t.Fatalf("expected a valid NetworkManager instance, got nil")
		}
	})

	t.Run("ErrorResolvingSSHClient", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Mock the injector to return an error when resolving sshClient
		injector.SetResolveError("sshClient", fmt.Errorf("mock error resolving ssh client"))

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve ssh client instance: mock error resolving ssh client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingSSHClient", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register the sshClient as "invalid"
		injector.Register("sshClient", "invalid")

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved ssh client instance is not of type ssh.Client" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Mock the injector to return an error when resolving shell
		injector.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve shell instance: mock error resolving shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Register the shell as "invalid"
		injector.Register("shell", "invalid")

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Register a mock shell
		mockShell := shell.NewMockShell()
		injector.Register("shell", mockShell)

		// Register a mock secureShell
		mockSecureShell := shell.NewMockShell()
		injector.Register("secureShell", mockSecureShell)

		// Mock the injector to return an error when resolving configHandler
		injector.SetResolveError("configHandler", fmt.Errorf("mock error resolving configHandler"))

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve CLI config handler: mock error resolving configHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Register a mock shell
		mockShell := shell.NewMockShell()
		injector.Register("shell", mockShell)

		// Mock the injector to return an error when resolving secureShell
		injector.SetResolveError("secureShell", fmt.Errorf("mock error resolving secureShell"))

		// Register a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		injector.Register("configHandler", mockConfigHandler)

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "failed to resolve secure shell instance: mock error resolving secureShell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingSecureShell", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Register a mock shell
		mockShell := shell.NewMockShell()
		injector.Register("shell", mockShell)

		// Register the secureShell as "invalid"
		injector.Register("secureShell", "invalid")

		// Register a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		injector.Register("configHandler", mockConfigHandler)

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved secure shell instance is not of type shell.Shell" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		injector := di.NewMockInjector()

		// Register a mock sshClient
		mockSSHClient := &ssh.MockClient{}
		injector.Register("sshClient", mockSSHClient)

		// Register a mock shell
		mockShell := shell.NewMockShell()
		injector.Register("shell", mockShell)

		// Register a mock secureShell
		mockSecureShell := shell.NewMockShell()
		injector.Register("secureShell", mockSecureShell)

		// Register the configHandler as "invalid"
		injector.Register("configHandler", "invalid")

		// Create a new NetworkManager
		nm, err := NewNetworkManager(injector)
		if err != nil {
			t.Fatalf("expected no error when creating NetworkManager, got %v", err)
		}

		// Run Initialize on the NetworkManager
		err = nm.Initialize()
		if err == nil {
			t.Fatalf("expected an error during Initialize, got nil")
		} else if err.Error() != "resolved CLI config handler instance is not of type config.ConfigHandler" {
			t.Fatalf("unexpected error message: got %v", err)
		}
	})
}

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	// Given: a DI container
	injector := di.NewInjector()

	// When: attempting to create NetworkManager
	_, err := NewNetworkManager(injector)

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	// Given: a DI container
	injector := di.NewInjector()

	// When: creating a NetworkManager and configuring the guest
	nm, err := NewNetworkManager(injector)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err = nm.ConfigureGuest()

	// Then: no error should be returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
