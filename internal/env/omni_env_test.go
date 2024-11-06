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

func TestOmniEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.omni/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		omniEnv := NewOmniEnv(mocks.Container)

		envVars, err := omniEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

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

		envVars, err := omniEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["OMNICONFIG"] != "" {
			t.Errorf("OMNICONFIG = %v, want empty", envVars["OMNICONFIG"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeOmniEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		omniEnv := NewOmniEnv(mockContainer)

		_, err := omniEnv.GetEnvVars()
		expectedError := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeOmniEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		omniEnv := NewOmniEnv(container)

		_, err := omniEnv.GetEnvVars()
		expectedError := "failed to cast contextHandler to context.ContextInterface"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		omniEnv := NewOmniEnv(mocks.Container)

		_, err := omniEnv.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}
