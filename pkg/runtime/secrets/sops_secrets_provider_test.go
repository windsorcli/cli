package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The SopsSecretsProviderTest is a test suite for the SopsSecretsProvider implementation
// It provides comprehensive testing of the SOPS-based secrets provider
// It serves as a validation mechanism for the provider's behavior
// It ensures the provider correctly implements the SecretsProvider interface

// =============================================================================
// Test Setup
// =============================================================================

func setupSopsSecretsMocks(t *testing.T) *SecretsTestMocks {
	mocks := setupSecretsMocks(t)

	// Mock the stat function to simulate the file exists
	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	// Mock the decryptFileFunc to return valid decrypted content with nested keys
	mocks.Shims.DecryptFile = func(filePath string, format string) ([]byte, error) {
		return []byte(`
nested:
  key: value
  another:
    deep: secret
`), nil
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewSopsSecretsProvider(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *SecretsTestMocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("Success", func(t *testing.T) {
		provider, _ := setup(t)

		// When NewSopsSecretsProvider is called
		expectedPath := "/valid/config/path"
		if provider.configPath != expectedPath {
			t.Fatalf("expected config path to be %v, got %v", expectedPath, provider.configPath)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSopsSecretsProvider_LoadSecrets(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *SecretsTestMocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		provider, _ := setup(t)

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the secrets should be loaded correctly
		if provider.secrets["nested.key"] != "value" {
			t.Fatalf("expected secret 'nested.key' to be 'value', got %v", provider.secrets["nested.key"])
		}
		if provider.secrets["nested.another.deep"] != "secret" {
			t.Fatalf("expected secret 'nested.another.deep' to be 'secret', got %v", provider.secrets["nested.another.deep"])
		}
	})

	t.Run("LoadsAndMergesMultipleFiles", func(t *testing.T) {
		// Given a SopsSecretsProvider with multiple secrets files
		provider, mocks := setup(t)

		var fileCallCount int
		var fileCallCountMu sync.Mutex
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/valid/config/path", "secrets.yaml") ||
				name == filepath.Join("/valid/config/path", "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shims.DecryptFile = func(filePath string, format string) ([]byte, error) {
			fileCallCountMu.Lock()
			fileCallCount++
			fileCallCountMu.Unlock()
			if filePath == filepath.Join("/valid/config/path", "secrets.yaml") {
				return []byte(`
key1: value1
key2: value2
`), nil
			}
			if filePath == filepath.Join("/valid/config/path", "secrets.enc.yaml") {
				return []byte(`
key2: value2_override
key3: value3
`), nil
			}
			return nil, fmt.Errorf("unexpected file: %s", filePath)
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And both files should have been decrypted
		fileCallCountMu.Lock()
		actualCount := fileCallCount
		fileCallCountMu.Unlock()
		if actualCount != 2 {
			t.Fatalf("expected DecryptFile to be called 2 times, got %d", actualCount)
		}

		// And secrets from both files should be merged
		if provider.secrets["key1"] != "value1" {
			t.Fatalf("expected secret 'key1' to be 'value1', got %v", provider.secrets["key1"])
		}

		// And later file should override earlier file
		if provider.secrets["key2"] != "value2_override" {
			t.Fatalf("expected secret 'key2' to be 'value2_override' (overridden), got %v", provider.secrets["key2"])
		}

		if provider.secrets["key3"] != "value3" {
			t.Fatalf("expected secret 'key3' to be 'value3', got %v", provider.secrets["key3"])
		}
	})

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given a new SopsSecretsProvider with no secrets file
		provider, mocks := setup(t)

		// Mock the stat function to return an error indicating the file does not exist
		mocks.Shims.Stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then no error should be returned (secrets are optional)
		if err != nil {
			t.Fatalf("expected no error when file does not exist, got %v", err)
		}

		// And the provider should remain locked
		if provider.unlocked {
			t.Fatalf("expected provider to remain locked when no file exists")
		}
	})

	t.Run("DoesNotReDecryptWhenAlreadyUnlocked", func(t *testing.T) {
		// Given a SopsSecretsProvider that has already loaded secrets
		provider, mocks := setup(t)
		provider.secrets = map[string]string{"test_key": "test_value"}
		provider.unlocked = true

		decryptCallCount := 0
		mocks.Shims.DecryptFile = func(_ string, _ string) ([]byte, error) {
			decryptCallCount++
			return nil, fmt.Errorf("should not be called")
		}

		// When LoadSecrets is called again
		err := provider.LoadSecrets()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And DecryptFile should not have been called
		if decryptCallCount != 0 {
			t.Fatalf("expected DecryptFile to not be called when already unlocked, but it was called %d times", decryptCallCount)
		}

		// And secrets should remain unchanged
		if provider.secrets["test_key"] != "test_value" {
			t.Fatalf("expected secrets to remain unchanged, got %v", provider.secrets)
		}
	})

	t.Run("DecryptionFailure", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		provider, mocks := setup(t)

		// Mock the decryptFileFunc to return an error
		mocks.Shims.DecryptFile = func(_ string, _ string) ([]byte, error) {
			return nil, fmt.Errorf("decryption error")
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		// And the error message should indicate a decryption failure with file path
		expectedFilePath := filepath.Join("/valid/config/path", "secrets.yaml")
		expectedErrorMessage := fmt.Sprintf("failed to decrypt file %s: decryption error", expectedFilePath)
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}
	})

	t.Run("YAMLUnmarshalError", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		provider, mocks := setup(t)

		// Mock the yamlUnmarshal function to return an error
		mocks.Shims.YAMLUnmarshal = func(_ []byte, _ any) error {
			return fmt.Errorf("yaml: unmarshal errors: [1:1] string was used where mapping is expected")
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		// And the error message should indicate a YAML unmarshal error with file path
		expectedFilePath := filepath.Join("/valid/config/path", "secrets.yaml")
		expectedErrorMessage := fmt.Sprintf("error converting YAML to secrets map from %s: yaml: unmarshal errors: [1:1] string was used where mapping is expected", expectedFilePath)
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}
	})
}

func TestSopsSecretsProvider_GetSecret(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *SecretsTestMocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("ReturnsMaskedValueWhenLocked", func(t *testing.T) {
		provider, _ := setup(t)
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = false // Simulate that secrets are locked

		value, err := provider.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "********" {
			t.Errorf("Expected GetSecret to return '********', but got: %s", value)
		}
	})

	t.Run("ReturnsActualValueWhenUnlocked", func(t *testing.T) {
		provider, _ := setup(t)
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = true // Simulate that secrets have been unlocked

		value, err := provider.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})

	t.Run("ReturnsErrorWhenSecretNotFound", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true // Simulate that secrets have been unlocked

		value, err := provider.GetSecret("non_existent_key")

		if err == nil {
			t.Errorf("Expected GetSecret to fail, but got no error")
		}

		if value != "" {
			t.Errorf("Expected GetSecret to return empty string, but got: %s", value)
		}

		expectedError := "secret not found: non_existent_key"
		if err.Error() != expectedError {
			t.Errorf("Expected error message to be '%s', but got: %s", expectedError, err.Error())
		}
	})
}

func TestSopsSecretsProvider_ParseSecrets(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *SecretsTestMocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("ReplacesSecretSuccessfully", func(t *testing.T) {
		provider, _ := setup(t)
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = true // Simulate that secrets have been unlocked

		// Test with standard notation
		input1 := "This is a secret: ${{ sops.test_key }}"
		expectedOutput1 := "This is a secret: test_value"

		output1, err := provider.ParseSecrets(input1)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output1 != expectedOutput1 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output1, expectedOutput1)
		}

		// Test with spaces in the notation
		input2 := "This is a secret: ${{  sops.test_key  }}"
		expectedOutput2 := "This is a secret: test_value"

		output2, err := provider.ParseSecrets(input2)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output2 != expectedOutput2 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output2, expectedOutput2)
		}
	})

	t.Run("HandlesEmptyInput", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true

		output, err := provider.ParseSecrets("")
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != "" {
			t.Errorf("Expected empty output for empty input, got '%s'", output)
		}
	})

	t.Run("HandlesInputWithNoSecrets", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true

		input := "This is a string with no secrets"
		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != input {
			t.Errorf("Expected unchanged input, got '%s'", output)
		}
	})

	t.Run("HandlesMultipleSecrets", func(t *testing.T) {
		provider, _ := setup(t)
		provider.secrets["key1"] = "value1"
		provider.secrets["key2"] = "value2"
		provider.unlocked = true

		input := "First: ${{ sops.key1 }}, Second: ${{ sops.key2 }}"
		expectedOutput := "First: value1, Second: value2"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("HandlesInvalidSecretFormat", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true

		input := "This is invalid: ${{ sops. }}"
		expectedOutput := "This is invalid: <ERROR: invalid secret format>"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("HandlesConsecutiveDots", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true

		input := "This is invalid: ${{ sops.key..path }}"
		expectedOutput := "This is invalid: <ERROR: invalid key path: key..path>"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("HandlesSecretNotFound", func(t *testing.T) {
		provider, _ := setup(t)
		provider.unlocked = true

		input := "Missing secret: ${{ sops.missing_key }}"
		expectedOutput := "Missing secret: <ERROR: secret not found: missing_key>"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected '%s', got '%s'", expectedOutput, output)
		}
	})
}

func TestSopsSecretsProvider_findSecretsFilePaths(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *SecretsTestMocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("ReturnsAllExistingFiles", func(t *testing.T) {
		provider, mocks := setup(t)

		// Mock Stat to return success for all files
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/valid/config/path", "secrets.yaml") ||
				name == filepath.Join("/valid/config/path", "secrets.yml") ||
				name == filepath.Join("/valid/config/path", "secrets.enc.yaml") ||
				name == filepath.Join("/valid/config/path", "secrets.enc.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		paths := provider.findSecretsFilePaths()
		expectedCount := 4
		if len(paths) != expectedCount {
			t.Errorf("Expected %d paths, got %d", expectedCount, len(paths))
		}

		expectedPaths := map[string]bool{
			filepath.Join("/valid/config/path", "secrets.yaml"):     true,
			filepath.Join("/valid/config/path", "secrets.yml"):      true,
			filepath.Join("/valid/config/path", "secrets.enc.yaml"): true,
			filepath.Join("/valid/config/path", "secrets.enc.yml"):  true,
		}

		for _, path := range paths {
			if !expectedPaths[path] {
				t.Errorf("Unexpected path: %s", path)
			}
		}
	})

	t.Run("ReturnsOnlyExistingFiles", func(t *testing.T) {
		provider, mocks := setup(t)

		// Mock Stat to return success only for secrets.yaml and secrets.enc.yaml
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/valid/config/path", "secrets.yaml") ||
				name == filepath.Join("/valid/config/path", "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		paths := provider.findSecretsFilePaths()
		expectedCount := 2
		if len(paths) != expectedCount {
			t.Errorf("Expected %d paths, got %d", expectedCount, len(paths))
		}

		expectedPaths := map[string]bool{
			filepath.Join("/valid/config/path", "secrets.yaml"):     true,
			filepath.Join("/valid/config/path", "secrets.enc.yaml"): true,
		}

		for _, path := range paths {
			if !expectedPaths[path] {
				t.Errorf("Unexpected path: %s", path)
			}
		}
	})

	t.Run("ReturnsEmptySliceWhenNoFilesExist", func(t *testing.T) {
		provider, mocks := setup(t)

		// Mock Stat to return error for all files
		mocks.Shims.Stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		paths := provider.findSecretsFilePaths()
		if len(paths) != 0 {
			t.Errorf("Expected empty slice when no files exist, got %v", paths)
		}
	})
}
