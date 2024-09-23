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

func TestKubeHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		kubeConfigPath := filepath.Join(contextPath, ".kube", "config")

		// Ensure the kube config file exists
		err := os.MkdirAll(filepath.Dir(kubeConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create kube config directory: %v", err)
		}
		_, err = os.Create(kubeConfigPath)
		if err != nil {
			t.Fatalf("Failed to create kube config file: %v", err)
		}

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create KubeHelper
		kubeHelper := NewKubeHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"KUBECONFIG": kubeConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		kubeConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create KubeHelper
		kubeHelper := NewKubeHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty KUBECONFIG
		expectedEnvVars := map[string]string{
			"KUBECONFIG": kubeConfigPath,
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

		// Create KubeHelper
		kubeHelper := NewKubeHelper(nil, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := kubeHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
