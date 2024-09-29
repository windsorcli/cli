package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

func TestOmniHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		omniConfigPath := filepath.Join(contextPath, ".omni", "config")

		// Ensure the omni config file exists
		err := os.MkdirAll(filepath.Dir(omniConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create omni config directory: %v", err)
		}
		_, err = os.Create(omniConfigPath)
		if err != nil {
			t.Fatalf("Failed to create omni config file: %v", err)
		}

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create OmniHelper
		omniHelper := NewOmniHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := omniHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"OMNICONFIG": omniConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		omniConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create OmniHelper
		omniHelper := NewOmniHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := omniHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty OMNICONFIG
		expectedEnvVars := map[string]string{
			"OMNICONFIG": omniConfigPath,
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

		// Create OmniHelper
		omniHelper := NewOmniHelper(nil, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := omniHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
