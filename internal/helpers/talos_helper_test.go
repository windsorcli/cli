package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestTalosHelper(t *testing.T) {
	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a valid context path
			contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
			talosConfigPath := filepath.Join(contextPath, ".talos", "config")

			// And the talos config file is created
			err := os.MkdirAll(filepath.Dir(talosConfigPath), 0755)
			if err != nil {
				t.Fatalf("Failed to create talos config directory: %v", err)
			}
			_, err = os.Create(talosConfigPath)
			if err != nil {
				t.Fatalf("Failed to create talos config file: %v", err)
			}

			// And a mock context is set up
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}

			// And a DI container with the mock context is created
			container := di.NewContainer()
			container.Register("context", mockContext)

			// When creating TalosHelper
			talosHelper, err := NewTalosHelper(container)
			if err != nil {
				t.Fatalf("failed to create talos helper: %v", err)
			}

			// And calling GetEnvVars
			envVars, err := talosHelper.GetEnvVars()
			if err != nil {
				t.Fatalf("GetEnvVars() error = %v", err)
			}

			// Then the environment variables should be set correctly
			expectedEnvVars := map[string]string{
				"TALOSCONFIG": talosConfigPath,
			}
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
			}
		})

		t.Run("FileNotExist", func(t *testing.T) {
			// Given: a non-existent context path
			contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
			talosConfigPath := ""

			// And a mock context is set up
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}

			// And a DI container with the mock context is created
			container := di.NewContainer()
			container.Register("context", mockContext)

			// When creating TalosHelper
			talosHelper, err := NewTalosHelper(container)
			if err != nil {
				t.Fatalf("failed to create talos helper: %v", err)
			}

			// And calling GetEnvVars
			envVars, err := talosHelper.GetEnvVars()
			if err != nil {
				t.Fatalf("GetEnvVars() error = %v", err)
			}

			// Then the environment variables should be set correctly with an empty TALOSCONFIG
			expectedEnvVars := map[string]string{
				"TALOSCONFIG": talosConfigPath,
			}
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
			}
		})

		t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
			// Given a mock context that returns an error for config root
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", errors.New("error retrieving config root")
			}

			// And a DI container with the mock context is created
			container := di.NewContainer()
			container.Register("context", mockContext)

			// When creating TalosHelper
			talosHelper, err := NewTalosHelper(container)
			if err != nil {
				t.Fatalf("failed to create talos helper: %v", err)
			}

			// And calling GetEnvVars
			expectedError := "error retrieving config root"

			_, err = talosHelper.GetEnvVars()

			// Then it should return an error indicating config root retrieval failure
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a TalosHelper instance
			mockContext := context.NewMockContext()
			container := di.NewContainer()
			container.Register("context", mockContext)
			talosHelper, err := NewTalosHelper(container)
			if err != nil {
				t.Fatalf("failed to create talos helper: %v", err)
			}

			// When calling PostEnvExec
			err = talosHelper.PostEnvExec()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("SetConfig", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating TalosHelper
		helper, err := NewTalosHelper(container)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		t.Run("SetConfigStub", func(t *testing.T) {
			// When: SetConfig is called
			err := helper.SetConfig("some_key", "some_value")

			// Then: it should return no error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("NewTalosHelper", func(t *testing.T) {
		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Given a DI container without registering context
			diContainer := di.NewContainer()

			// When attempting to create TalosHelper
			_, err := NewTalosHelper(diContainer)

			// Then it should return an error indicating context resolution failure
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(container)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		t.Run("Success", func(t *testing.T) {
			// When: GetContainerConfig is called
			containerConfig, err := talosHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: the result should be nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/path/to/config", nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create an instance of TalosHelper
			talosHelper, err := NewTalosHelper(diContainer)
			if err != nil {
				t.Fatalf("NewTalosHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = talosHelper.WriteConfig()
			if err != nil {
				t.Fatalf("WriteConfig() error = %v", err)
			}

			// Then: no error should be returned
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	})
}
