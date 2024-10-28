//go:build darwin
// +build darwin

package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func stringPtr(s string) *string {
	return &s
}

func TestDarwinNetworkManager_Configure(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a DI container with mock handlers
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		colimaStatus := []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Address string `json:"address"`
		}{
			{
				Name:    "windsor-test",
				Status:  "Running",
				Address: "192.168.5.5",
			},
		}
		colimaStatusJSON, _ := json.Marshal(colimaStatus)
		shellInstance := shell.NewMockShell("unix")
		shellInstance.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "list" && args[1] == "--json" {
				return string(colimaStatusJSON), nil
			}
			return "", nil
		}

		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)
		diContainer.Register("shell", shellInstance)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ConfigHandlerResolveError", func(t *testing.T) {
		// Given: a mock DI container that fails to resolve the config handler
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("resolve error"))

		networkManager, _ := NewNetworkManager(mockContainer.DIContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("ConfigHandlerError", func(t *testing.T) {
		// Given: a DI container with a failing config handler
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("config handler error")
		}
		diContainer.Register("cliConfigHandler", configHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("NoDockerConfig", func(t *testing.T) {
		// Given: a DI container with no Docker configuration
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: nil,
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		diContainer.Register("cliConfigHandler", configHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoNetworkCIDR", func(t *testing.T) {
		// Given: a DI container with Docker configuration but no NetworkCIDR
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: nil,
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		diContainer.Register("cliConfigHandler", configHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoVMConfig", func(t *testing.T) {
		// Given: a DI container with Docker configuration but no VM configuration
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: nil,
			}, nil
		}
		diContainer.Register("cliConfigHandler", configHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("VMDriverNotColima", func(t *testing.T) {
		// Given: a DI container with Docker and VM configuration but VM driver is not "colima"
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			driver := "other"
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: &driver,
				},
			}, nil
		}
		diContainer.Register("cliConfigHandler", configHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		// Given: a mock DI container that returns an error when resolving context
		mockContainer := di.NewMockContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		mockContainer.Register("cliConfigHandler", configHandler)
		mockContainer.SetResolveError("context", errors.New("error resolving context handler: no instance registered with name context"))

		networkManager, _ := NewNetworkManager(mockContainer.DIContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving context handler: no instance registered with name context") {
			t.Fatalf("expected error message to contain 'error resolving context handler: no instance registered with name context', got %v", err)
		}
	})

	t.Run("ContextHandlerError", func(t *testing.T) {
		// Given: a DI container with a failing context handler
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "", errors.New("context handler error")
		}
		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("ShellResolveError", func(t *testing.T) {
		// Given: a mock DI container that fails to resolve the shell instance
		mockContainer := di.NewMockContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		mockContainer.Register("cliConfigHandler", configHandler)
		mockContainer.Register("context", contextHandler)
		mockContainer.SetResolveError("shell", errors.New("no instance registered with name shell"))

		networkManager, _ := NewNetworkManager(mockContainer.DIContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving shell: no instance registered with name shell") {
			t.Fatalf("expected error message to contain 'error resolving shell: no instance registered with name shell', got %v", err)
		}
	})

	t.Run("ShellError", func(t *testing.T) {
		// Given: a DI container with a failing shell instance
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		colimaStatus := []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Address string `json:"address"`
		}{
			{
				Name:    "windsor-test",
				Status:  "Running",
				Address: "192.168.5.5",
			},
		}
		colimaStatusJSON, _ := json.Marshal(colimaStatus)
		shellInstance := shell.NewMockShell("unix")
		shellInstance.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "list" && args[1] == "--json" {
				return string(colimaStatusJSON), nil
			}
			return "", errors.New("shell error")
		}

		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)
		diContainer.Register("shell", shellInstance)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("ShellExecError", func(t *testing.T) {
		// Given: a DI container with a shell instance that fails to execute the command
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		shellInstance := shell.NewMockShell("unix")
		shellInstance.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "list" && args[1] == "--json" {
				return "", errors.New("failed to check Colima status")
			}
			return "", nil
		}

		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)
		diContainer.Register("shell", shellInstance)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to check Colima status") {
			t.Fatalf("expected error message to contain 'failed to check Colima status', got %v", err)
		}
	})

	t.Run("UnmarshalError", func(t *testing.T) {
		// Given: a DI container with a shell instance that returns invalid JSON
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		shellInstance := shell.NewMockShell("unix")
		shellInstance.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "list" && args[1] == "--json" {
				return "invalid json", nil
			}
			return "", nil
		}

		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)
		diContainer.Register("shell", shellInstance)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse Colima status") {
			t.Fatalf("expected error message to contain 'failed to parse Colima status', got %v", err)
		}
	})

	t.Run("ColimaVMIPNotFound", func(t *testing.T) {
		// Given: a DI container with a shell instance that returns valid JSON but no running Colima VM
		diContainer := di.NewContainer()
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: stringPtr("192.168.0.0/16"),
				},
				VM: &config.VMConfig{
					Driver: stringPtr("colima"),
				},
			}, nil
		}
		contextHandler := context.NewMockContext()
		contextHandler.GetContextFunc = func() (string, error) {
			return "test", nil
		}
		shellInstance := shell.NewMockShell("unix")
		shellInstance.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "list" && args[1] == "--json" {
				return `[{"name": "windsor-test", "status": "Stopped", "address": ""}]`, nil
			}
			return "", nil
		}

		diContainer.Register("cliConfigHandler", configHandler)
		diContainer.Register("context", contextHandler)
		diContainer.Register("shell", shellInstance)

		networkManager, _ := NewNetworkManager(diContainer)

		// When: configuring the network
		err := networkManager.Configure()

		// Then: an error should be returned
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "Colima VM IP not found or Colima is not running") {
			t.Fatalf("expected error message to contain 'Colima VM IP not found or Colima is not running', got %v", err)
		}
	})
}
