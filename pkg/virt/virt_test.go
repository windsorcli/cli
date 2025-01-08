package virt

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestVirt_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", mockShell)
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
		mockConfigHandler := config.NewMockConfigHandler()

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

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a Virt with a mock injector
		injector := di.NewMockInjector()
		mockShell := shell.NewMockShell()

		injector.Register("shell", mockShell)
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
