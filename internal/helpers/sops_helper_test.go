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

	"github.com/stretchr/testify/assert"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
)

func setupTestContext(t *testing.T, contextName string) (string, string, string) {
	contextPath := filepath.Join(os.TempDir(), "contexts", contextName)
	plaintextSecretsFile := filepath.Join(contextPath, "secrets.yaml")
	encryptedSecretsFile := filepath.Join(contextPath, "secrets.enc.yaml")

	err := os.MkdirAll(filepath.Dir(plaintextSecretsFile), 0755)
	if err != nil {
		t.Fatalf("Failed to create secrets directory: %v", err)
	}
	_, err = os.Create(plaintextSecretsFile)
	if err != nil {
		t.Fatalf("Failed to create secrets file: %v", err)
	}

	return contextPath, plaintextSecretsFile, encryptedSecretsFile
}

func TestSopsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		contextPath, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+encryptedSecretsFile), 0644)

		// Encrypt the secrets file using SOPS
		err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		envVars, err := sopsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		expectedEnvVars := map[string]string{
			"SOPS_TEST": encryptedSecretsFile,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		_, err := sopsHelper.GetEnvVars()
		if err != nil {
			expectedError := "file does not exist"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving project root")
			},
		}

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
		contextPath, plaintextSecretsFile, _ := setupTestContext(t, "test-context")

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+plaintextSecretsFile), 0644)

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		_, err := sopsHelper.GetEnvVars()
		if err != nil {
			expectedError := "file does not exist"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("ErrorUnmarshallingYaml", func(t *testing.T) {
		contextPath, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS-TEST\": "+encryptedSecretsFile), 0644)

		// Encrypt the secrets file using SOPS
		err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		// Append "breaking-code" to the encrypted secrets file
		err = os.WriteFile(encryptedSecretsFile, []byte("breaking-code\n"), 0644) // Overwrites the file
		if err != nil {
			t.Fatalf("Failed to write to encrypted secrets file: %v", err)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		sopsHelper := NewSopsHelper(nil, nil, mockContext)

		_, err = sopsHelper.GetEnvVars()
		if err != nil {
			expectedError := "Error unmarshalling input yaml"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})
}

func TestSopsHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
		sopsHelper := NewSopsHelper(mockConfigHandler, mockShell, mockContext)

		err := sopsHelper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RunCommand", func(t *testing.T) {
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockShell := createMockShell(func() (string, error) { return "echo 'PostEnvExec'", nil })
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
		sopsHelper := NewSopsHelper(mockConfigHandler, mockShell, mockContext)

		err := sopsHelper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_SetConfig(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	mockContext := &context.MockContext{}
	helper := NewSopsHelper(mockConfigHandler, nil, mockContext)

	t.Run("SetConfigStub", func(t *testing.T) {
		err := helper.SetConfig("some_key", "some_value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetConfigActual", func(t *testing.T) {
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
		sopsHelper := NewSopsHelper(mockConfigHandler, nil, mockContext)

		err := sopsHelper.SetConfig("some_key", "some_value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_DecryptFile(t *testing.T) {
	t.Run("FileNotExist", func(t *testing.T) {
		_, err := DecryptFile("non-existent-file.yaml")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "file does not exist"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("DecryptError", func(t *testing.T) {
		// Create a dummy file
		filePath := "dummy-file.yaml"
		err := os.WriteFile(filePath, []byte("dummy content"), 0644)
		if err != nil {
			t.Fatalf("failed to create dummy file: %v", err)
		}
		defer os.Remove(filePath)

		_, err = DecryptFile(filePath)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "failed to decrypt file"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

// GenerateAgeKeys generates age.key and age.public.key
func generateAgeKeys() (string, error) {
	if _, err := os.Stat("age.key"); err == nil {
		if err := os.Remove("age.key"); err != nil {
			return "", fmt.Errorf("failed to remove existing age.key: %w", err)
		}
	}

	cmdKeygen := exec.Command("age-keygen", "-o", "age.key")
	if err := cmdKeygen.Run(); err != nil {
		return "", fmt.Errorf("failed to generate age key: %w", err)
	}

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

	os.Setenv("SOPS_AGE_KEY_FILE", "age.key")

	publicKey, err := os.ReadFile("age.public.key")
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	return string(publicKey), nil
}

// encryptFile encrypts the specified file using SOPS.
func encryptFile(t *testing.T, filePath string, dstPath string) error {

	// Generate AGE keys
	_, err := generateAgeKeys()
	if err != nil {
		t.Fatalf("Failed to generate AGE keys: %v", err)
	}

	ageKeyFile := os.Getenv("SOPS_AGE_KEY_FILE")
	var cmdEncrypt *exec.Cmd

	if ageKeyFile != "" {
		publicKey, err := os.ReadFile("age.public.key")
		if err != nil {
			return fmt.Errorf("failed to read public key: %w", err)
		}
		cmdEncrypt = exec.Command("sops", "--output", dstPath, "--age", string(publicKey), "-e", filePath)
	} else {
		cmdEncrypt = exec.Command("sops", "--output", dstPath, "-e", filePath)
	}

	output, err := cmdEncrypt.CombinedOutput()
	if err != nil {
		t.Logf("SOPS encrypt output: %s", string(output))
	}
	return err
}

func TestSopsHelper_EncryptFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		_, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

		// Create and initialize the secrets file
		os.WriteFile(plaintextSecretsFile, []byte("dummy: content"), 0644)

		// Encrypt the secrets file using SOPS
		err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}
	})
}

func TestSopsHelper_DecryptFile_FileDoesNotExist(t *testing.T) {
	_, err := DecryptFile("nonexistent_file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file does not exist")
}

func TestSopsHelper_DecryptFile_Success(t *testing.T) {
	_, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

	// Create and initialize the secrets file
	os.WriteFile(plaintextSecretsFile, []byte("dummy: content"), 0644)

	// Encrypt the secrets file using SOPS
	err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
	if err != nil {
		t.Fatalf("Failed to encrypt secrets file: %v", err)
	}

	// Test decryption
	plaintextBytes, err := DecryptFile(encryptedSecretsFile)
	assert.NoError(t, err)
	assert.NotNil(t, plaintextBytes)
}

func TestSopsHelper_DecryptFile_DecryptionFailure(t *testing.T) {
	// Create a temporary invalid encrypted file for testing
	invalidEncryptedFilePath := "invalid_secrets.enc.yaml"
	invalidContent := `invalid content`
	err := os.WriteFile(invalidEncryptedFilePath, []byte(invalidContent), 0644)
	assert.NoError(t, err)
	defer os.Remove(invalidEncryptedFilePath)

	// Test decryption failure
	_, err = DecryptFile(invalidEncryptedFilePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt file")
}

func TestSopsHelper_YamlToEnvVars(t *testing.T) {
	tests := []struct {
		name      string
		yamlData  string
		want      map[string]string
		expectErr bool
	}{
		{
			name: "valid yaml",
			yamlData: `
key1: value1
key2: value2
`,
			want: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectErr: false,
		},
		{
			name:      "invalid yaml",
			yamlData:  `: invalid yaml`,
			want:      nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := YamlToEnvVars([]byte(tt.yamlData)) // Convert string to []byte
			if (err != nil) != tt.expectErr {
				t.Errorf("yamlToEnvVars() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("yamlToEnvVars() = %v, want %v", got, tt.want)
			}
		})
	}
}
