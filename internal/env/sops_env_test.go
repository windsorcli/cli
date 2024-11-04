package env

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type SopsEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSopsEnvMocks(container ...di.ContainerInterface) *SopsEnvMocks {
	var mockContainer di.ContainerInterface
	if len(container) > 0 {
		mockContainer = container[0]
	} else {
		mockContainer = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	mockShell := shell.NewMockShell()

	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)

	return &SopsEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestSopsEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSopsEnvMocks()
		sopsEnv := NewSopsEnv(mocks.Container)

		envVars := map[string]string{
			"EXISTING_VAR": "existing_value",
		}

		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Mock the file system to simulate the existence of the SOPS file
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, nil // Simulate that the file exists
		}

		// Mock the decryption process
		decryptFileFunc = func(filePath string, _ string) ([]byte, error) {
			if filePath == "" {
				return nil, fmt.Errorf("file path is empty")
			}
			return []byte("NEW_VAR: new_value"), nil
		}

		// Mock the PrintEnvVars function to capture the envVars passed to it
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call the Print function
		err := sopsEnv.Print(envVars)
		if err != nil {
			t.Fatalf("Print returned an error: %v", err)
		}

		// Validate that PrintEnvVars was called with the expected environment variables
		expectedEnvVars := map[string]string{
			"EXISTING_VAR": "existing_value",
			"NEW_VAR":      "new_value",
		}
		for key, expectedValue := range expectedEnvVars {
			if capturedEnvVars[key] != expectedValue {
				t.Errorf("Expected env var %s to be %q, got %q", key, expectedValue, capturedEnvVars[key])
			}
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a mock container with a resolution error for contextHandler
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// And a setup with mocks using the mock container
		mocks := setupSopsEnvMocks(mockContainer)

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mocks.Container)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating contextHandler resolution failure
		expectedError := "error resolving contextHandler: mock error resolving contextHandler"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Given a mock container with an invalid type for contextHandler
		mockContainer := di.NewMockContainer()
		setupSopsEnvMocks(mockContainer)
		mockContainer.Register("contextHandler", "invalidType")

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating contextHandler casting failure
		expectedError := "failed to cast contextHandler to context.ContextInterface"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a mock container with a resolve error for shell
		mockContainer := di.NewMockContainer()
		setupSopsEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating shell resolution failure
		expectedError := "error resolving shell: mock error resolving shell"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		// Given a mock container with an invalid shell type
		mockContainer := di.NewMockContainer()
		setupSopsEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating shell casting failure
		expectedError := "failed to cast shell to shell.Shell"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given a mock container with a context handler that returns an error for GetConfigRoot
		mockContainer := di.NewMockContainer()
		mocks := setupSopsEnvMocks(mockContainer)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating config root retrieval failure
		expectedError := "error retrieving configuration root directory: mock error retrieving config root"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("SopsFileDoesNotExist", func(t *testing.T) {
		// Given a mock container with a valid context handler
		mockContainer := di.NewMockContainer()
		mocks := setupSopsEnvMocks(mockContainer)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// And a mocked stat function simulating the file does not exist
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating the SOPS file does not exist
		expectedError := "SOPS encrypted secrets file does not exist"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorDecryptingSopsFile", func(t *testing.T) {
		// Given a mock container with a valid context handler
		mockContainer := di.NewMockContainer()
		mocks := setupSopsEnvMocks(mockContainer)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// And a mocked stat function simulating the file exists
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And a mocked decryptFileFunc function returning an error
		originalDecryptFile := decryptFileFunc
		defer func() { decryptFileFunc = originalDecryptFile }()
		decryptFileFunc = func(filePath string, format string) ([]byte, error) {
			return nil, fmt.Errorf("failed to decrypt file: mock error decrypting file")
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating the decryption failure
		expectedError := "mock error decrypting file"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})

	t.Run("ErrorConvertingYamlToEnvVars", func(t *testing.T) {
		// Given a mock container with a valid context handler
		mockContainer := di.NewMockContainer()
		mocks := setupSopsEnvMocks(mockContainer)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// And a mocked stat function simulating the file exists
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And a mocked decryptFileFunc function returning valid data
		originalDecryptFile := decryptFileFunc
		defer func() { decryptFileFunc = originalDecryptFile }()
		decryptFileFunc = func(filePath string, format string) ([]byte, error) {
			return []byte("valid: yaml"), nil
		}

		// And a mocked yamlUnmarshal function returning an error
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()
		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mock error converting YAML to env vars")
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnv(mockContainer)

		// Capture stdout output
		output := captureStdout(t, func() {
			// And calling Print
			envVars := map[string]string{}
			err := sopsEnv.Print(envVars)
			if err != nil {
				fmt.Println(err)
			}
		})

		// Then it should print an error message indicating the conversion failure
		expectedError := "mock error converting YAML to env vars"
		if !strings.Contains(output, expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, output)
		}
	})
}

func TestSopsEnv_decryptFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mocked decryptFileFunc returning valid data
		originalDecryptFileFunc := decryptFileFunc
		defer func() { decryptFileFunc = originalDecryptFileFunc }()
		decryptFileFunc = func(filePath string, format string) ([]byte, error) {
			return []byte("decrypted content"), nil
		}

		// And a mocked stat function simulating the file exists
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// When decryptFile is called
		result, err := decryptFile("/mock/path/to/file")

		// Then it should return the decrypted content without error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedContent := "decrypted content"
		if string(result) != expectedContent {
			t.Errorf("expected %q, got %q", expectedContent, string(result))
		}
	})

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given a mocked stat function simulating the file does not exist
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When decryptFile is called
		_, err := decryptFile("/mock/path/to/nonexistent/file")

		// Then it should return an error indicating the file does not exist
		if err == nil || !strings.Contains(err.Error(), "file does not exist") {
			t.Fatalf("expected file does not exist error, got %v", err)
		}
	})

	t.Run("DecryptError", func(t *testing.T) {
		// Given a mocked decryptFileFunc returning an error
		originalDecryptFileFunc := decryptFileFunc
		defer func() { decryptFileFunc = originalDecryptFileFunc }()
		decryptFileFunc = func(filePath string, format string) ([]byte, error) {
			return nil, fmt.Errorf("mock decryption error")
		}

		// And a mocked stat function simulating the file exists
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// When decryptFile is called
		_, err := decryptFile("/mock/path/to/file")

		// Then it should return the decryption error
		if err == nil || !strings.Contains(err.Error(), "mock decryption error") {
			t.Fatalf("expected mock decryption error, got %v", err)
		}
	})
}
