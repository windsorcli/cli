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
)

func TestKubeHelper_GetEnvVars(t *testing.T) {
	t.Run("ValidConfigRoot", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		kubeConfigPath := filepath.Join(contextPath, ".kube", "config")

		// Create the directory and kubeconfig file
		err := os.MkdirAll(filepath.Join(contextPath, ".kube"), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(kubeConfigPath)
		if err != nil {
			t.Fatalf("Failed to create kubeconfig file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts"))

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

func TestKubeHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a KubeHelper instance
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := kubeHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestKubeHelper_SetConfig(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	mockContext := &context.MockContext{}
	helper := NewKubeHelper(mockConfigHandler, nil, mockContext)

	t.Run("SetConfigStub", func(t *testing.T) {
		// When: SetConfig is called
		err := helper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
