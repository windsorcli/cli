package helpers

import (
	"errors"
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
		mockShell, _ := shell.NewMockShell("unix")

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
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
		mockShell, _ := shell.NewMockShell("unix")
		mockContext := context.NewMockContext()
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

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
		diContainer.Register("context", mockContext)

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
		mockShell, _ := shell.NewMockShell("unix")
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

func TestBaseHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		}
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		}

		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/project/root/contexts/test-context", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedResult := map[string]string{
			"VAR1":                 "value1",
			"VAR2":                 "value2",
			"WINDSOR_CONTEXT":      "test-context",
			"WINDSOR_PROJECT_ROOT": "/mock/project/root",
		}

		result, err := baseHelper.GetEnvVars()
		assertError(t, err, false)

		// Then the result should match the expected result
		if len(result) != len(expectedResult) {
			t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
		}

		for k, v := range expectedResult {
			if result[k] != v {
				t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
			}
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given a mock config handler that returns an error for context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", errors.New("error retrieving context")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("error retrieving context")
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/project/root/contexts/test-context", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedError := errors.New("error retrieving context: error retrieving context")

		_, err = baseHelper.GetEnvVars()
		// Then an error should be returned
		assertError(t, err, true)
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NonStringEnvVar", func(t *testing.T) {
		// Given a mock config handler with a non-string environment variable
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": 123, // Non-string value
				}, nil
			}
			return nil, errors.New("context not found")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		diContainer.Register("shell", mockShell)
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedError := errors.New("expected map[string]string for environment variables, got map[string]interface {}")

		_, err = baseHelper.GetEnvVars()
		// Then an error should be returned
		assertError(t, err, true)
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("EmptyEnvVars", func(t *testing.T) {
		// Given a mock config handler with empty environment variables
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		}
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]string{}, nil
			}
			return nil, errors.New("context not found")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		diContainer.Register("shell", mockShell)
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/project/root/contexts/test-context", nil
		}
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedResult := map[string]string{
			"WINDSOR_CONTEXT":      "test-context",
			"WINDSOR_PROJECT_ROOT": "/mock/project/root",
		}

		result, err := baseHelper.GetEnvVars()
		assertError(t, err, false)

		// Then the result should match the expected result
		if len(result) != len(expectedResult) {
			t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
		}

		for k, v := range expectedResult {
			if result[k] != v {
				t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
			}
		}
	})

	t.Run("GetError", func(t *testing.T) {
		// Given a mock config handler that returns an error for Get
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		}
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.environment" {
				return nil, errors.New("error getting value")
			}
			return nil, errors.New("key not found")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		diContainer.Register("shell", mockShell)
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/project/root/contexts/test-context", nil
		}
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedError := errors.New("error retrieving environment variables: error getting value")

		_, err = baseHelper.GetEnvVars()
		// Then an error should be returned
		assertError(t, err, true)
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ProjectRootError", func(t *testing.T) {
		// Given a mock shell that returns an error for GetProjectRoot
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("failed to get project root")
		}

		// Given a mock config handler that returns a proper interface representing env vars
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("key not found")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling GetEnvVars
		_, err = baseHelper.GetEnvVars()
		// Then an error should be returned
		expectedError := "error retrieving project root: failed to get project root"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %s, got %v", expectedError, err)
		}
	})
}

func TestBaseHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BaseHelper instance
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) { return "", nil }
		mockContext.GetConfigRootFunc = func() (string, error) { return "", nil }

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		// When creating a new BaseHelper
		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// And calling PostEnvExec
		err = baseHelper.PostEnvExec()

		// Then no error should be returned
		assertError(t, err, false)
	})
}

func TestBaseHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell, _ := shell.NewMockShell("unix")
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

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
		mockShell, _ := shell.NewMockShell("unix")

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
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
