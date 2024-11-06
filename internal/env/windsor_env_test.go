package env

import (
	"errors"
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type WindsorEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeWindsorEnvMocks(container ...di.ContainerInterface) *WindsorEnvMocks {
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
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)

	return &WindsorEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestWindsorEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		windsorEnv := NewWindsorEnv(mocks.Container)

		envVars, err := windsorEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %v, want %v", envVars["WINDSOR_CONTEXT"], "mock-context")
		}
		if envVars["WINDSOR_PROJECT_ROOT"] != "/mock/project/root" {
			t.Errorf("WINDSOR_PROJECT_ROOT = %v, want %v", envVars["WINDSOR_PROJECT_ROOT"], "/mock/project/root")
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeWindsorEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		windsorEnv := NewWindsorEnv(mockContainer)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeWindsorEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		windsorEnv := NewWindsorEnv(container)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "failed to cast contextHandler to context.ContextInterface"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeWindsorEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		windsorEnv := NewWindsorEnv(mockContainer)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error resolving shell: mock resolve error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeWindsorEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		windsorEnv := NewWindsorEnv(mockContainer)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "failed to cast shell to shell.Shell"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error retrieving current context: mock context error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("mock shell error")
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		_, err := windsorEnv.GetEnvVars()
		expectedErrorMessage := "error retrieving project root: mock shell error"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})
}

func TestWindsorEnv_PostEnvHook(t *testing.T) {
	t.Run("TestPostEnvHookNoError", func(t *testing.T) {
		windsorEnv := &Env{}

		err := windsorEnv.PostEnvHook()
		if err != nil {
			t.Errorf("PostEnvHook() returned an error: %v", err)
		}
	})
}
