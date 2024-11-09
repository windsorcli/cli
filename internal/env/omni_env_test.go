package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type OmniEnvMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeOmniEnvMocks(injector ...di.Injector) *OmniEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &OmniEnvMocks{
		Injector:       mockInjector,
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
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		omniEnv := NewOmniEnv(mocks.Injector)

		envVars, err := omniEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["OMNICONFIG"] != filepath.FromSlash("/mock/config/root/.omni/config") {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], filepath.FromSlash("/mock/config/root/.omni/config"))
		}
	})

	t.Run("NoOmniConfig", func(t *testing.T) {
		mocks := setupSafeOmniEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		omniEnv := NewOmniEnv(mocks.Injector)

		envVars, err := omniEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["OMNICONFIG"] != "" {
			t.Errorf("OMNICONFIG = %v, want empty", envVars["OMNICONFIG"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeOmniEnvMocks(mockInjector)
		mockInjector.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		omniEnv := NewOmniEnv(mockInjector)

		_, err := omniEnv.GetEnvVars()
		expectedError := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		setupSafeOmniEnvMocks(mockInjector)
		mockInjector.Register("contextHandler", "invalidType")

		omniEnv := NewOmniEnv(mockInjector)

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

		omniEnv := NewOmniEnv(mocks.Injector)

		_, err := omniEnv.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

func TestOmniEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeOmniEnvMocks to create mocks
		mocks := setupSafeOmniEnvMocks()
		mockInjector := mocks.Injector
		omniEnv := NewOmniEnv(mockInjector)

		// Mock the stat function to simulate the existence of the omniconfig file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := omniEnv.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"OMNICONFIG": filepath.FromSlash("/mock/config/root/.omni/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeOmniEnvMocks to create mocks
		mocks := setupSafeOmniEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockInjector := mocks.Injector

		omniEnv := NewOmniEnv(mockInjector)

		// Call Print and check for errors
		err := omniEnv.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
