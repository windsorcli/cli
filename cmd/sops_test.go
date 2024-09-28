package cmd

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to set up the container with mock handlers
func setupMockSopsContainer(mockCliConfigHandler, mockProjectConfigHandler config.ConfigHandler, mockShell shell.Shell) di.ContainerInterface {
	container := di.NewContainer()
	container.Register("cliConfigHandler", mockCliConfigHandler)
	container.Register("projectConfigHandler", mockProjectConfigHandler)
	container.Register("shell", mockShell)
	Initialize(container)
	return container
}

// TestDecryptSopsCmd tests the decrypt functionality of the sops command
func TestDecryptSopsCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("decryptSopsFileSuccess", func(t *testing.T) {
		// Given: a valid encrypted file (use a mock or a temporary encrypted file)
		mockHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// Create a temporary file for testing
		tmpFile, err := ioutil.TempFile("", "test-sops-decrypt.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// Write mock encrypted content to the file (this would normally be valid SOPS-encrypted data)
		_, err = tmpFile.WriteString("test-key: ENC[AES256_GCM,data]")
		if err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		// When: the decrypt command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"sops", "decrypt", tmpFile.Name()})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate successful decryption
		expectedOutput := "test-key: ENC[AES256_GCM,data]"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("decryptSopsFileError", func(t *testing.T) {
		// Given: a config handler that returns an error on GetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "", errors.New("accepts 1 arg(s), received 0") },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the decrypt command is executed with missing argument
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"sops", "decrypt"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "accepts 1 arg(s), received 0"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

// TestEncryptSopsCmd tests the encrypt functionality of the sops command
func TestEncryptSopsCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("encryptSopsFileSuccess", func(t *testing.T) {
		// Given: a valid file to encrypt
		mockHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// Create a temporary file to encrypt
		tmpFile, err := ioutil.TempFile("", "test-sops-encrypt.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// Write some content to the file
		_, err = tmpFile.WriteString("test-key: test-value\n")
		if err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		// When: the encrypt command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"sops", "encrypt", tmpFile.Name()})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "File successfully encrypted"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}

		// Check if the file was encrypted (note: SOPS encrypted files have an "sops" metadata block)
		encryptedContent, err := ioutil.ReadFile(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to read encrypted file: %v", err)
		}
		if !strings.Contains(string(encryptedContent), "sops") {
			t.Errorf("Expected encrypted content to contain SOPS metadata, got %q", string(encryptedContent))
		}
	})

	t.Run("encryptSopsFileError", func(t *testing.T) {
		// Given: a config handler and shell that simulate an encryption error
		mockHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: encryption is attempted on a non-existent file
		nonExistentFile := "/tmp/non-existent-file.yaml"

		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"sops", "encrypt", nonExistentFile})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate a file not found error
		expectedOutput := "file does not exist"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	// Edge case: Test empty input file
	t.Run("encryptEmptyFile", func(t *testing.T) {
		// Given: a valid but empty file
		mockHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// Create an empty temporary file
		tmpFile, err := ioutil.TempFile("", "test-sops-empty-encrypt.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// When: the encrypt command is executed on the empty file
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"sops", "encrypt", tmpFile.Name()})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "File successfully encrypted"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}

		// Check if the empty file was still encrypted (should have "sops" metadata)
		encryptedContent, err := ioutil.ReadFile(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to read encrypted file: %v", err)
		}
		if !strings.Contains(string(encryptedContent), "sops") {
			t.Errorf("Expected encrypted content to contain SOPS metadata, got %q", string(encryptedContent))
		}
	})
}
