package virt

import (
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
		injector.Register("configHandler", mockConfigHandler)
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
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("shell", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("Expected error containing 'error resolving shell', got %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", mockShell)
		injector.Register("configHandler", mockConfigHandler)
		injector.Register("contextHandler", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving context handler") {
			t.Fatalf("Expected error containing 'error resolving context handler', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockContextHandler := context.NewMockContext()

		injector.Register("shell", mockShell)
		injector.Register("contextHandler", mockContextHandler)
		injector.Register("configHandler", "invalid")
		v := NewBaseVirt(injector)

		// When calling Initialize
		err := v.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("Expected error containing 'error resolving configHandler', got %v", err)
		}
	})
}
