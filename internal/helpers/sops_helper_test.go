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

func TestSopsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, "sops.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := sopsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"SOPSCONFIG": sopsConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		sopsConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := sopsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty SOPSCONFIG
		expectedEnvVars := map[string]string{
			"SOPSCONFIG": sopsConfigPath,
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

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := sopsHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
