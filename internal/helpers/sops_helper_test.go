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
	"github.com/windsor-hotel/cli/internal/di"
)

// setupTestContext sets paths and names for secrets
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

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

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

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		_, err = sopsHelper.GetEnvVars()
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

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		_, err = sopsHelper.GetEnvVars()
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

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		_, err = sopsHelper.GetEnvVars()
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

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

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
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		err = sopsHelper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RunCommand", func(t *testing.T) {
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}

		container := di.NewContainer()
		container.Register("context", mockContext)

		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		err = sopsHelper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_SetConfig(t *testing.T) {
	mockContext := &context.MockContext{
		GetContextFunc:    func() (string, error) { return "", nil },
		GetConfigRootFunc: func() (string, error) { return "", nil },
	}

	container := di.NewContainer()
	container.Register("context", mockContext)

	t.Run("SetConfigStub", func(t *testing.T) {
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		err = sopsHelper.SetConfig("some_key", "some_value")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_DecryptFile_FileDoesNotExist(t *testing.T) {
	_, err := DecryptFile("nonexistent_file.yaml")
	if err == nil || !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("expected error containing 'file does not exist', got %v", err)
	}
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
				t.Errorf("YamlToEnvVars() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("YamlToEnvVars() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSopsHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container without registering context
		diContainer := di.NewContainer()

		// Attempt to create SopsHelper
		_, err := NewSopsHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}
