package virt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestVirt_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockContext := context.NewMockContext()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", mockShell)
		injector.Register("contextHandler", mockContext)
		injector.Register("cliConfigHandler", mockConfigHandler)
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockContext := context.NewMockContext()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("contextHandler", mockContext)
		injector.Register("cliConfigHandler", mockConfigHandler)
		injector.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || err.Error() != "error resolving shell: mock error resolving shell" {
			t.Fatalf("Expected 'error resolving shell: mock error resolving shell', got %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockContext := context.NewMockContext()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("contextHandler", mockContext)
		injector.Register("cliConfigHandler", mockConfigHandler)
		injector.Register("shell", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "not of type Shell") {
			t.Fatalf("Expected error containing 'not of type Shell', got %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", mockShell)
		injector.Register("cliConfigHandler", mockConfigHandler)
		injector.SetResolveError("contextHandler", fmt.Errorf("mock error resolving context handler"))
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving context handler") {
			t.Fatalf("Expected error containing 'error resolving context handler', got %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", mockShell)
		injector.Register("cliConfigHandler", mockConfigHandler)
		injector.Register("contextHandler", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "not of type ContextInterface") {
			t.Fatalf("Expected error containing 'not of type ContextInterface', got %v", err)
		}
	})

	t.Run("ErrorResolvingCliConfigHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockContextHandler := context.NewMockContext()

		injector.Register("shell", mockShell)
		injector.Register("contextHandler", mockContextHandler)
		injector.SetResolveError("cliConfigHandler", fmt.Errorf("mock error resolving cliConfigHandler"))
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("Expected error containing 'error resolving cliConfigHandler', got %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockContextHandler := context.NewMockContext()

		injector.Register("shell", mockShell)
		injector.Register("contextHandler", mockContextHandler)
		injector.Register("cliConfigHandler", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "not of type ConfigHandler") {
			t.Fatalf("Expected error containing 'not of type ConfigHandler', got %v", err)
		}
	})
}
