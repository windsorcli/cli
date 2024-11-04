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

func TestTalosEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			t.Log("PrintEnvVarsFunc called successfully with envVars:", envVars)
			return nil
		}

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.talos/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		talosEnv := NewTalosEnv(mocks.Container)

		envVars := make(map[string]string)
		talosEnv.Print(envVars)

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

		envVars := make(map[string]string)
		talosEnv.Print(envVars)

		if envVars["TALOSCONFIG"] != "" {
			t.Errorf("TALOSCONFIG = %v, want empty", envVars["TALOSCONFIG"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeTalosEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		talosEnv := NewTalosEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			talosEnv.Print(envVars)
		})

		expectedOutput := "Error resolving contextHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeTalosEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		talosEnv := NewTalosEnv(container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			talosEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast contextHandler to context.ContextInterface\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeTalosEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		talosEnv := NewTalosEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			talosEnv.Print(envVars)
		})

		expectedOutput := "Error resolving shell: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeTalosEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		talosEnv := NewTalosEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			talosEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast shell to shell.Shell\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		talosEnv := NewTalosEnv(mocks.Container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			talosEnv.Print(envVars)
		})

		expectedOutput := "Error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}
