package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

type SopsEnvMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSopsEnvMocks(injector ...di.Injector) *SopsEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &SopsEnvMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestSopsEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSopsEnvMocks()
		sopsEnv := NewSopsEnvPrinter(mocks.Injector)
		sopsEnv.Initialize()

		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
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

		// Call the GetEnvVars function
		envVars, err := sopsEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Validate that the environment variables are as expected
		expectedEnvVars := map[string]string{
			"NEW_VAR": "new_value",
		}
		for key, expectedValue := range expectedEnvVars {
			if envVars[key] != expectedValue {
				t.Errorf("Expected env var %s to be %q, got %q", key, expectedValue, envVars[key])
			}
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given a mock injector with a context handler that returns an error for GetConfigRoot
		mockInjector := di.NewMockInjector()
		mocks := setupSopsEnvMocks(mockInjector)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()

		// Call the GetEnvVars function
		_, err := sopsEnv.GetEnvVars()

		// Then it should return an error indicating config root retrieval failure
		expectedError := "error retrieving configuration root directory: mock error retrieving config root"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("SopsFileDoesNotExist", func(t *testing.T) {
		// Given a mock injector with a valid context handler
		mockInjector := di.NewMockInjector()
		mocks := setupSopsEnvMocks(mockInjector)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}

		// And a mocked stat function simulating the file does not exist
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When creating SopsEnv
		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()
		// Call the GetEnvVars function
		envVars, err := sopsEnv.GetEnvVars()

		// Then it should return nil without an error
		if envVars != nil {
			t.Errorf("expected nil, got %v", envVars)
		}
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorDecryptingSopsFile", func(t *testing.T) {
		// Given a mock injector with a valid context handler
		mockInjector := di.NewMockInjector()
		mocks := setupSopsEnvMocks(mockInjector)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
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
		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()
		// Call the GetEnvVars function
		_, err := sopsEnv.GetEnvVars()

		// Then it should return an error indicating the decryption failure
		expectedError := "mock error decrypting file"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorConvertingYamlToEnvVars", func(t *testing.T) {
		// Given a mock injector with a valid context handler
		mockInjector := di.NewMockInjector()
		mocks := setupSopsEnvMocks(mockInjector)
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
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
		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()
		// Call the GetEnvVars function
		_, err := sopsEnv.GetEnvVars()

		// Then it should return an error indicating the conversion failure
		expectedError := "mock error converting YAML to env vars"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
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

func TestSopsEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSopsEnvMocks to create mocks
		mocks := setupSopsEnvMocks()
		mockInjector := mocks.Injector
		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()
		// Mock the stat function to simulate the existence of the sops encrypted secrets file
		stat = func(name string) (os.FileInfo, error) {
			if filepath.Clean(name) == filepath.FromSlash("/mock/config/root/secrets.enc.yaml") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the decryption process to ensure the environment variable is set
		decryptFileFunc = func(filePath string, _ string) ([]byte, error) {
			return []byte("NEW_VAR: new_value"), nil
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := sopsEnv.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"NEW_VAR": "new_value", // Updated expected env var
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSopsEnvMocks to create mocks
		mocks := setupSopsEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock config error")
		}

		mockInjector := mocks.Injector

		sopsEnv := NewSopsEnvPrinter(mockInjector)
		sopsEnv.Initialize()
		// Call Print and check for errors
		err := sopsEnv.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
