package env

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type TalosEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeTalosEnvMocks(container ...di.ContainerInterface) *TalosEnvMocks {
	var mockContainer di.ContainerInterface
	if len(container) > 0 {
		mockContainer = container[0]
	} else {
		mockContainer = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	mockShell := shell.NewMockShell()

	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)

	return &TalosEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestTalosEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.talos/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		talosEnv := NewTalosEnv(mocks.Container)

		envVars, err := talosEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["TALOSCONFIG"] != "/mock/config/root/.talos/config" {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], "/mock/config/root/.talos/config")
		}
	})

	t.Run("NoTalosConfig", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		talosEnv := NewTalosEnv(mocks.Container)

		envVars, err := talosEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["TALOSCONFIG"] != "" {
			t.Errorf("TALOSCONFIG = %v, want empty", envVars["TALOSCONFIG"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeTalosEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		talosEnv := NewTalosEnv(mockContainer)

		_, err := talosEnv.GetEnvVars()
		if err == nil || err.Error() != "error resolving contextHandler: mock resolve error" {
			t.Errorf("expected error resolving contextHandler, got %v", err)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeTalosEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		talosEnv := NewTalosEnv(container)

		_, err := talosEnv.GetEnvVars()
		if err == nil || err.Error() != "failed to cast contextHandler to context.ContextInterface" {
			t.Errorf("expected failed to cast contextHandler error, got %v", err)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		talosEnv := NewTalosEnv(mocks.Container)

		_, err := talosEnv.GetEnvVars()
		if err == nil || err.Error() != "error retrieving configuration root directory: mock context error" {
			t.Errorf("expected error retrieving configuration root directory, got %v", err)
		}
	})
}
