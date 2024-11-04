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
		// Test error when casting contextHandler to context.ContextInterface
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Test error when resolving shell
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		// Test error when casting shell to shell.Shell
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Test error when retrieving configuration root directory
	})

	t.Run("SopsFileDoesNotExist", func(t *testing.T) {
		// Test scenario where SOPS encrypted secrets file does not exist
	})

	t.Run("ErrorDecryptingSopsFile", func(t *testing.T) {
		// Test error when decrypting SOPS file
	})

	t.Run("ErrorConvertingYamlToEnvVars", func(t *testing.T) {
		// Test error when converting YAML to environment variables
	})
}
