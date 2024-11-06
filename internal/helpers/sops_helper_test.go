package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		diContainer.Register("contextHandler", mockContext)

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
		container.Register("contextHandler", mockContext)

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
		diContainer.Register("contextHandler", mockContext)

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
		diContainer.Register("contextHandler", mockContext)

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

func TestSopsHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of SopsHelper
		sopsHelper, err := NewSopsHelper(diContainer)
		if err != nil {
			t.Fatalf("NewSopsHelper() error = %v", err)
		}

		// When: Info is called
		info, err := sopsHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
