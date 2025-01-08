package config

import (
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestBaseConfigHandler_Initialize(t *testing.T) {

	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		injector.Register("shell", shell.NewMockShell())

		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		injector := di.NewInjector()

		// Do not register the shell to simulate error in resolving shell
		// injector.Register("shell", shell.NewMockShell())

		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err == nil {
			t.Errorf("Expected error when resolving shell, got nil")
		}
	})
}
