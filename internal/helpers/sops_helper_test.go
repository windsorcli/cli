package helpers

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

// EncryptFile encrypts the specified file using SOPS.
func EncryptFile(filePath string, dstPath string) error {
	cmd := exec.Command("sops", "-e", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// Write the encrypted output back to the file
	return os.WriteFile(dstPath, output, 0644)
}

func TestSopsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")
		sopsEncConfigPath := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPSCONFIG\": "+sopsConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(sopsConfigPath, sopsEncConfigPath)
		if err != nil {
			t.Fatalf("Failed to encrypt sops config file: %v", err)
			t.Fatalf("sopsConfigPath: %v", sopsConfigPath)
			t.Fatalf("sopsEncConfigPath: %v", sopsEncConfigPath)
		}

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		defer func() {
			if err := os.Remove(sopsEncConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

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
			"SOPSCONFIG": sopsEncConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err := sopsHelper.GetEnvVars()

		if err != nil {
			expectedError := "file does not exist"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
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

		_, err := sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "error retrieving config root"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}
	})

	t.Run("SopsMetaDataNotFound", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPSCONFIG\": "+sopsConfigPath), 0644)

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err = sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "sops metadata not found"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}
	})

	t.Run("ErrorUnmarshallingYaml", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		sopsConfigPath := filepath.Join(contextPath, ".sops/secrets.yaml")
		sopsEncConfigPath := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the sops config file exists
		err := os.MkdirAll(filepath.Dir(sopsConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create sops config directory: %v", err)
		}
		_, err = os.Create(sopsConfigPath)
		if err != nil {
			t.Fatalf("Failed to create sops config file: %v", err)
		}

		// Create and initialize the sops config file
		os.WriteFile(sopsConfigPath, []byte("\"SOPS-CONFIG\": "+sopsConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(sopsConfigPath, sopsEncConfigPath)
		if err != nil {
			t.Fatalf("Failed to encrypt sops config file: %v", err)
		}

		// Append "breaking-code" to the sops config file
		err = os.WriteFile(sopsEncConfigPath, []byte("breaking-code\n"), 0644) // Overwrites the file
		if err != nil {
			t.Fatalf("Failed to write to sops config file: %v", err)
		}

		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()
		// Defer removal of the sops config file
		defer func() {
			if err := os.Remove(sopsEncConfigPath); err != nil {
				t.Fatalf("Failed to remove sops config file: %v", err)
			}
		}()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		_, err = sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "Error unmarshalling input yaml"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
				// } else {
				// 	t.Logf("err: %v", err)
			}
		}

	})
}
