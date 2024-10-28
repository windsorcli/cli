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

	"github.com/windsor-hotel/cli/internal/config"
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

func TestSopsHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of SopsHelper
		sopsHelper, err := NewSopsHelper(diContainer)
		if err != nil {
			t.Fatalf("NewSopsHelper() error = %v", err)
		}

		// When: Initialize is called
		err = sopsHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a test context
		contextPath, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

		// And a secrets file is created and initialized
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+encryptedSecretsFile), 0644)

		// And the secrets file is encrypted using SOPS
		err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling GetEnvVars
		envVars, err := sopsHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"SOPS_TEST": encryptedSecretsFile,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling GetEnvVars
		_, err = sopsHelper.GetEnvVars()
		if err != nil {
			// Then it should return an error indicating the file does not exist
			expectedError := "file does not exist"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock context that returns an error for config root
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("error retrieving project root")
		}

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling GetEnvVars
		_, err = sopsHelper.GetEnvVars()
		if err != nil {
			// Then it should return an error indicating config root retrieval failure
			expectedError := "error retrieving config root"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("SopsMetaDataNotFound", func(t *testing.T) {
		// Given a test context
		contextPath, plaintextSecretsFile, _ := setupTestContext(t, "test-context")

		// And a secrets file is created and initialized
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS_TEST\": "+plaintextSecretsFile), 0644)

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling GetEnvVars
		_, err = sopsHelper.GetEnvVars()
		if err != nil {
			// Then it should return an error indicating the file does not exist
			expectedError := "file does not exist"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})

	t.Run("ErrorUnmarshallingYaml", func(t *testing.T) {
		// Given a test context
		contextPath, plaintextSecretsFile, encryptedSecretsFile := setupTestContext(t, "test-context")

		// And a secrets file is created and initialized
		os.WriteFile(plaintextSecretsFile, []byte("\"SOPS-TEST\": "+encryptedSecretsFile), 0644)

		// And the secrets file is encrypted using SOPS
		err := encryptFile(t, plaintextSecretsFile, encryptedSecretsFile)
		if err != nil {
			t.Fatalf("Failed to encrypt secrets file: %v", err)
		}

		// And "breaking-code" is appended to the encrypted secrets file
		err = os.WriteFile(encryptedSecretsFile, []byte("breaking-code\n"), 0644) // Overwrites the file
		if err != nil {
			t.Fatalf("Failed to write to encrypted secrets file: %v", err)
		}

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling GetEnvVars
		_, err = sopsHelper.GetEnvVars()
		if err != nil {
			// Then it should return an error indicating YAML unmarshalling failure
			expectedError := "Error unmarshalling input yaml"
			if !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		}
	})
}

func TestSopsHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) { return "", nil }
		mockContext.GetConfigRootFunc = func() (string, error) { return "", nil }

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling PostEnvExec
		err = sopsHelper.PostEnvExec()
		if err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("RunCommand", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) { return "", nil }
		mockContext.GetConfigRootFunc = func() (string, error) { return "", nil }

		// And a DI container with the mock context is created
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("Failed to create SopsHelper: %v", err)
		}

		// And calling PostEnvExec
		err = sopsHelper.PostEnvExec()
		if err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DecryptFile", func(t *testing.T) {
		t.Run("FileDoesNotExist", func(t *testing.T) {
			// When calling DecryptFile with a non-existent file
			_, err := DecryptFile("nonexistent_file.yaml")
			if err == nil || !strings.Contains(err.Error(), "file does not exist") {
				// Then it should return an error indicating the file does not exist
				t.Fatalf("expected error containing 'file does not exist', got %v", err)
			}
		})
	})

	t.Run("YamlToEnvVars", func(t *testing.T) {
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
				// When calling YamlToEnvVars with the provided YAML data
				got, err := YamlToEnvVars([]byte(tt.yamlData)) // Convert string to []byte
				if (err != nil) != tt.expectErr {
					// Then it should return the expected error state
					t.Errorf("YamlToEnvVars() error = %v, expectErr %v", err, tt.expectErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					// And it should return the expected environment variables
					t.Errorf("YamlToEnvVars() = %v, want %v", got, tt.want)
				}
			})
		}
	})
}

func TestSopsHelper_NewSopsHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Given a DI container without registering context
		diContainer := di.NewContainer()

		// When attempting to create SopsHelper
		_, err := NewSopsHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			// Then it should return an error indicating context resolution failure
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestSopsHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		container := di.NewContainer()
		container.Register("context", mockContext)

		// When creating SopsHelper
		sopsHelper, err := NewSopsHelper(container)
		if err != nil {
			t.Fatalf("NewSopsHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		composeConfig, err := sopsHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then the result should be nil as per the stub implementation
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})
}

func TestSopsHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/path/to/config", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of SopsHelper
		sopsHelper, err := NewSopsHelper(diContainer)
		if err != nil {
			t.Fatalf("NewSopsHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = sopsHelper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestSopsHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of SopsHelper
		sopsHelper, err := NewSopsHelper(diContainer)
		if err != nil {
			t.Fatalf("NewSopsHelper() error = %v", err)
		}

		// When: Up is called
		err = sopsHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}
