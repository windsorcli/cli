package helpers

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

// GenerateAgeKey generates age.key and age.public.key
func GenerateAgeKey() (string, error) {
	// Check if age.key already exists and remove it
	if _, err := os.Stat("age.key"); err == nil {
		if err := os.Remove("age.key"); err != nil {
			return "", fmt.Errorf("failed to remove existing age.key: %w", err)
		}
	}

	// Generate the private key
	cmdKeygen := exec.Command("age-keygen", "-o", "age.key")
	if err := cmdKeygen.Run(); err != nil {
		return "", fmt.Errorf("failed to generate age key: %w", err)
	}

	// Generate the public key
	cmdPublicKey := exec.Command("age-keygen", "-y", "age.key")
	publicKeyFile, err := os.Create("age.public.key")
	if err != nil {
		return "", fmt.Errorf("failed to create public key file: %w", err)
	}
	defer publicKeyFile.Close()

	cmdPublicKey.Stdout = publicKeyFile
	if err := cmdPublicKey.Run(); err != nil {
		return "", fmt.Errorf("failed to generate public key: %w", err)
	}

	// Set the environment variable
	os.Setenv("SOPS_AGE_KEY_FILE", "age.key")

	// Read the public key from the file
	publicKey, err := os.ReadFile("age.public.key")
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	return string(publicKey), nil
}

// EncryptFile encrypts the specified file using SOPS.
func EncryptFile(t *testing.T, filePath string, dstPath string) error {

	publicKey, err := GenerateAgeKey()

	if err != nil {
		return err
	}

	cmdEncrypt := exec.Command("sops", "--output", dstPath, "--age", publicKey, "-e", filePath)
	_, err = cmdEncrypt.CombinedOutput()

	return err
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
		os.WriteFile(sopsConfigPath, []byte("\"SOPSCONFIG\": "+sopsEncConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(t, sopsConfigPath, sopsEncConfigPath)
		if err != nil {
			t.Fatalf("Failed to encrypt sops config file: %v", err)
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

			expectedError := "file does not exist"

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
		os.WriteFile(sopsConfigPath, []byte("\"SOPS-CONFIG\": "+sopsEncConfigPath), 0644)

		// Encrypt the sops config file using SOPS
		err = EncryptFile(t, sopsConfigPath, sopsEncConfigPath)
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
