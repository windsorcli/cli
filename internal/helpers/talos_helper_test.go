package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestTalosHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		talosConfigPath := filepath.Join(contextPath, ".talos", "config")

		// Ensure the talos config file exists
		err := os.MkdirAll(filepath.Dir(talosConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create talos config directory: %v", err)
		}
		_, err = os.Create(talosConfigPath)
		if err != nil {
			t.Fatalf("Failed to create talos config file: %v", err)
		}

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create TalosHelper
		container := di.NewContainer()
		container.Register("context", mockContext)
		talosHelper, err := NewTalosHelper(container)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := talosHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
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

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create TalosHelper
		container := di.NewContainer()
		container.Register("context", mockContext)
		talosHelper, err := NewTalosHelper(container)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := talosHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty TALOSCONFIG
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": talosConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock shell and context that returns an error for config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		// Create TalosHelper
		container := di.NewContainer()
		container.Register("context", mockContext)
		talosHelper, err := NewTalosHelper(container)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = talosHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestTalosHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a TalosHelper instance
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
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
}

func TestTalosHelper_SetConfig(t *testing.T) {
	mockContext := &context.MockContext{}
	container := di.NewContainer()
	container.Register("context", mockContext)
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
}

func TestNewTalosHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container without registering context
		diContainer := di.NewContainer()

		// Attempt to create TalosHelper
		_, err := NewTalosHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestTalosHelper_GetContainerConfig(t *testing.T) {
	// Given a mock context
	mockContext := &context.MockContext{}
	container := di.NewContainer()
	container.Register("context", mockContext)

	// Create TalosHelper
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
}
