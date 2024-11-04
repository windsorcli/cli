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

func TestWindsorEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			t.Log("PrintEnvVarsFunc called successfully with envVars:", envVars)
			return nil
		}

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.windsor/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		envVars := make(map[string]string)
		err := windsorEnv.Print(envVars)
		if err != nil {
			t.Fatalf("Print returned an error: %v", err)
		}

		if envVars["WINDSOR_CONTEXT"] != "mock-context" {
			t.Errorf("WINDSOR_CONTEXT = %v, want %v", envVars["WINDSOR_CONTEXT"], "mock-context")
		}
		if envVars["WINDSOR_PROJECT_ROOT"] != "/mock/project/root" {
			t.Errorf("WINDSOR_PROJECT_ROOT = %v, want %v", envVars["WINDSOR_PROJECT_ROOT"], "/mock/project/root")
		}
	})

	t.Run("NoWindsorConfig", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		envVars := make(map[string]string)
		err := windsorEnv.Print(envVars)
		if err != nil {
			t.Fatalf("Print returned an error: %v", err)
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

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "error resolving contextHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeWindsorEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		windsorEnv := NewWindsorEnv(container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "failed to cast contextHandler to context.ContextInterface\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeWindsorEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		windsorEnv := NewWindsorEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "error resolving shell: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeWindsorEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		windsorEnv := NewWindsorEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "failed to cast shell to shell.Shell\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "error retrieving current context: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		mocks := setupSafeWindsorEnvMocks()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("mock shell error")
		}

		windsorEnv := NewWindsorEnv(mocks.Container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			err := windsorEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		expectedOutput := "error retrieving project root: mock shell error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
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
