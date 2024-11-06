package helpers

import (
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function for error assertion
func assertError(t *testing.T, err error, shouldError bool) {
	if shouldError && err == nil {
		t.Errorf("Expected error, got nil")
	} else if !shouldError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestBaseHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When: Initialize is called
		err = baseHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBaseHelper_NewBaseHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Given a DI container without cliConfigHandler registered
		mockShell := shell.NewMockShell()
		mockContext := context.NewMockContext()
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		// When creating a new BaseHelper
		_, err := NewBaseHelper(diContainer)

		// Then an error should be returned
		assertError(t, err, true)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Given a DI container with cliConfigHandler registered but without shell registered
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// When creating a new BaseHelper
		_, err := NewBaseHelper(diContainer)

		// Then an error should be returned
		assertError(t, err, true)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Given a DI container with cliConfigHandler and shell registered but without context registered
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// When creating a new BaseHelper
		_, err := NewBaseHelper(diContainer)

		// Then an error should be returned
		assertError(t, err, true)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestBaseHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetComposeConfig
		composeConfig, err := baseHelper.GetComposeConfig()
		assertError(t, err, false)

		// Then the result should be nil as per the stub implementation
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})
}

func TestBaseHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/path/to/config", nil
		}
		mockShell := shell.NewMockShell()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = baseHelper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBaseHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When: Up is called
		err = baseHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestBaseHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When: Info is called
		info, err := baseHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
