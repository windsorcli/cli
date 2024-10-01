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
		plaintextSecretsFile := filepath.Join(contextPath, ".sops/secrets.yaml")
		encryptedSecretsFile := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the secrets file exists
		err := os.MkdirAll(filepath.Dir(plaintextSecretsFile), 0755)
		if err != nil {
			t.Fatalf("Failed to create secrets directory: %v", err)
		}
		_, err = os.Create(plaintextSecretsFile)
		if err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+encryptedSecretsFile), 0644)

		// Encrypt the secrets file using SOPS
		err = EncryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		// Defer removal of the secrets file
		defer func() {
			if err := os.Remove(plaintextSecretsFile); err != nil {
				t.Fatalf("Failed to remove secrets file: %v", err)
			}
		}()

		defer func() {
			if err := os.Remove(encryptedSecretsFile); err != nil {
				t.Fatalf("Failed to remove encrypted secrets file: %v", err)
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
			"SOPS_TEST": encryptedSecretsFile,
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
			}
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock shell and context that returns an error for project root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving project root")
			},
		}

		// Create SopsHelper
		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		_, err := sopsHelper.GetEnvVars()

		if err != nil {

			expectedError := "error retrieving config root"

			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("SopsMetaDataNotFound", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		plaintextSecretsFile := filepath.Join(contextPath, ".sops/secrets.yaml")

		// Ensure the secrets file exists
		err := os.MkdirAll(filepath.Dir(plaintextSecretsFile), 0755)
		if err != nil {
			t.Fatalf("Failed to create secrets directory: %v", err)
		}
		_, err = os.Create(plaintextSecretsFile)
		if err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+plaintextSecretsFile), 0644)

		// Defer removal of the secrets file
		defer func() {
			if err := os.Remove(plaintextSecretsFile); err != nil {
				t.Fatalf("Failed to remove secrets file: %v", err)
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
			}
		}
	})

	t.Run("ErrorUnmarshallingYaml", func(t *testing.T) {

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		plaintextSecretsFile := filepath.Join(contextPath, ".sops/secrets.yaml")
		encryptedSecretsFile := filepath.Join(contextPath, ".sops/secrets.enc.yaml")

		// Ensure the secrets file exists
		err := os.MkdirAll(filepath.Dir(plaintextSecretsFile), 0755)
		if err != nil {
			t.Fatalf("Failed to create secrets files directory: %v", err)
		}
		_, err = os.Create(plaintextSecretsFile)
		if err != nil {
			t.Fatalf("Failed to create secrets file file: %v", err)
		}

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS-TEST\": "+encryptedSecretsFile), 0644)

		// Encrypt the secrets file using SOPS
		err = EncryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		// Append "breaking-code" to the encrypted secrets file
		err = os.WriteFile(encryptedSecretsFile, []byte("breaking-code\n"), 0644) // Overwrites the file
		if err != nil {
			t.Fatalf("Failed to write to encrypted secrets file: %v", err)
		}

		// Defer removal of the secrets file
		defer func() {
			if err := os.Remove(plaintextSecretsFile); err != nil {
				t.Fatalf("Failed to remove secrets file: %v", err)
			}
		}()
		// Defer removal of the encrypted secrets file
		defer func() {
			if err := os.Remove(encryptedSecretsFile); err != nil {
				t.Fatalf("Failed to remove encrypted secrets file: %v", err)
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
			}
		}

	})
}
