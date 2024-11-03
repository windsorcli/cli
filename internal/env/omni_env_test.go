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

type OmniEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeOmniEnvMocks(container ...di.ContainerInterface) *OmniEnvMocks {
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

	return &OmniEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestOmniEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			t.Log("PrintEnvVarsFunc called successfully with envVars:", envVars)
		}

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.omni/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		omniEnv := NewOmniEnv(mocks.Container)

		envVars := make(map[string]string)
		omniEnv.Print(envVars)

		if envVars["OMNICONFIG"] != "/mock/config/root/.omni/config" {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], "/mock/config/root/.omni/config")
		}
	})

	t.Run("NoOmniConfig", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		omniEnv := NewOmniEnv(mocks.Container)

		envVars := make(map[string]string)
		omniEnv.Print(envVars)

		if envVars["OMNICONFIG"] != "" {
			t.Errorf("OMNICONFIG = %v, want empty", envVars["OMNICONFIG"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeOmniEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		omniEnv := NewOmniEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			omniEnv.Print(envVars)
		})

		expectedOutput := "Error resolving contextHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeOmniEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		omniEnv := NewOmniEnv(container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			omniEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast contextHandler to context.ContextInterface\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeOmniEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		omniEnv := NewOmniEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			omniEnv.Print(envVars)
		})

		expectedOutput := "Error resolving shell: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeOmniEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		omniEnv := NewOmniEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			omniEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast shell to shell.Shell\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		omniEnv := NewOmniEnv(mocks.Container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			omniEnv.Print(envVars)
		})

		expectedOutput := "Error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})
}
