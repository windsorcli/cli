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
		// Given: a mock shell and context with valid context and project root
		mockShell := createMockShell(func() (string, error) {
			return os.TempDir(), nil
		})
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			},
		}

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

		kubeHelper := NewKubeHelper(nil, mockShell, mockContext)

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
		// Given a mock shell and context with valid context and project root
		mockShell := createMockShell(func() (string, error) {
			return os.TempDir(), nil
		})
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			},
		}
		kubeHelper := NewKubeHelper(nil, mockShell, mockContext)

		// When calling GetEnvVars
		expectedResult := map[string]string{
			"KUBECONFIG": filepath.Join(os.TempDir(), "contexts", "test-context", ".kube", "config"),
		}

		result, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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

	// t.Run("ErrorRetrievingContext", func(t *testing.T) {
	// 	// Given a mock shell and context that returns an error for context
	// 	mockShell := createMockShell(func() (string, error) {
	// 		return os.TempDir(), nil
	// 	})
	// 	mockContext := &context.MockContext{
	// 		GetContextFunc: func() (string, error) {
	// 			return "", errors.New("error retrieving context")
	// 		},
	// 		GetConfigRootFunc: func() (string, error) {
	// 			return "", errors.New("error retrieving config root")
	// 		},
	// 	}
	// 	kubeHelper := NewKubeHelper(nil, mockShell, mockContext)

	// 	// When calling GetEnvVars
	// 	expectedError := "error retrieving context"

	// 	_, err := kubeHelper.GetEnvVars()
	// 	if err == nil || !strings.Contains(err.Error(), expectedError) {
	// 		t.Fatalf("expected error containing %v, got %v", expectedError, err)
	// 	}
	// })

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock shell and context that returns an error for config root
		mockShell := createMockShell(func() (string, error) {
			return "", errors.New("error retrieving project root")
		})
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}
		kubeHelper := NewKubeHelper(nil, mockShell, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := kubeHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
