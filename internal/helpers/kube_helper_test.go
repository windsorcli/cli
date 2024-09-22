package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

func TestKubeHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and context with valid context and project root
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("unknown key")
			},
			nil, // getNestedMapFunc is not needed for this test
		)
		mockShell := createMockShell(func() (string, error) {
			return os.TempDir(), nil
		})
		mockContext := context.NewContext(mockConfigHandler, mockShell)

		// Create a temporary kube config file
		projectRoot := os.TempDir()
		kubeConfigPath := filepath.Join(projectRoot, "contexts", "test-context", ".kube", "config")
		err := os.MkdirAll(filepath.Dir(kubeConfigPath), os.ModePerm)
		if err != nil {
			t.Fatalf("failed to create directories: %v", err)
		}
		file, err := os.Create(kubeConfigPath)
		if err != nil {
			t.Fatalf("failed to create kube config file: %v", err)
		}
		file.Close()
		defer os.RemoveAll(filepath.Join(projectRoot, "contexts"))

		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When: calling GetEnvVars
		envVars, err := kubeHelper.GetEnvVars()

		// Then: the KUBECONFIG environment variable should be set correctly
		expectedEnvVars := map[string]string{
			"KUBECONFIG": kubeConfigPath,
		}
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !equalMaps(envVars, expectedEnvVars) {
			t.Fatalf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a mock config handler, shell, and context with valid context and project root
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("unknown key")
			},
			nil, // getNestedMapFunc is not needed for this test
		)
		mockShell := createMockShell(func() (string, error) {
			return os.TempDir(), nil
		})
		mockContext := context.NewContext(mockConfigHandler, mockShell)

		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When: calling GetEnvVars
		envVars, err := kubeHelper.GetEnvVars()

		// Then: the KUBECONFIG environment variable should be empty
		expectedEnvVars := map[string]string{
			"KUBECONFIG": "",
		}
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !equalMaps(envVars, expectedEnvVars) {
			t.Fatalf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given: a mock config handler that returns an error for context
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "", errors.New("error retrieving context")
			},
			nil, // getNestedMapFunc is not needed for this test
		)
		mockShell := createMockShell(func() (string, error) {
			return os.TempDir(), nil
		})
		mockContext := context.NewContext(mockConfigHandler, mockShell)

		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When: calling GetEnvVars
		_, err := kubeHelper.GetEnvVars()

		// Then: an error should be returned
		expectedError := "error retrieving context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given: a mock shell that returns an error for project root
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("unknown key")
			},
			nil, // getNestedMapFunc is not needed for this test
		)
		mockShell := createMockShell(func() (string, error) {
			return "", errors.New("error retrieving project root")
		})
		mockContext := context.NewContext(mockConfigHandler, mockShell)

		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When: calling GetEnvVars
		_, err := kubeHelper.GetEnvVars()

		// Then: an error should be returned
		expectedError := "error retrieving project root"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingContextAndProjectRoot", func(t *testing.T) {
		// Given: a mock config handler that returns an error for context and a mock shell that returns an error for project root
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "", errors.New("error retrieving context")
			},
			nil, // getNestedMapFunc is not needed for this test
		)
		mockShell := createMockShell(func() (string, error) {
			return "", errors.New("error retrieving project root")
		})
		mockContext := context.NewContext(mockConfigHandler, mockShell)

		kubeHelper := NewKubeHelper(mockConfigHandler, mockShell, mockContext)

		// When: calling GetEnvVars
		_, err := kubeHelper.GetEnvVars()

		// Then: an error should be returned for context first
		expectedError := "error retrieving context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
