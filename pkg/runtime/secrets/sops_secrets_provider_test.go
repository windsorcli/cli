package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The SopsSecretsProviderTest is a test suite for the SopsSecretsProvider implementation
// It provides comprehensive testing of the SOPS-based secrets provider
// It serves as a validation mechanism for the provider's behavior
// It ensures the provider correctly implements the SecretsProvider interface

// =============================================================================
// Test Setup
// =============================================================================

func setupSopsSecretsMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	mocks := setupMocks(t, opts...)

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
	setup := func(t *testing.T) (*SopsSecretsProvider, *Mocks) {
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
	setup := func(t *testing.T) (*SopsSecretsProvider, *Mocks) {
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

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given a new SopsSecretsProvider with an invalid config path
		provider, mocks := setup(t)

		// Mock the stat function to return an error indicating the file does not exist
		mocks.Shims.Stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		// And the error message should indicate the file does not exist
		expectedErrorMessage := fmt.Sprintf("file does not exist: %s", filepath.Join(provider.configPath, "secrets.enc.yml"))
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
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

		// And the error message should indicate a decryption failure
		expectedErrorMessage := "failed to decrypt file: decryption error"
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

		// And the error message should indicate a YAML unmarshal error
		expectedErrorMessage := "error converting YAML to secrets map: yaml: unmarshal errors: [1:1] string was used where mapping is expected"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}
	})
}

func TestSopsSecretsProvider_GetSecret(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *Mocks) {
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
	setup := func(t *testing.T) (*SopsSecretsProvider, *Mocks) {
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

func TestSopsSecretsProvider_findSecretsFilePath(t *testing.T) {
	setup := func(t *testing.T) (*SopsSecretsProvider, *Mocks) {
		mocks := setupSopsSecretsMocks(t)
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Shell)
		provider.shims = mocks.Shims
		return provider, mocks
	}

	t.Run("ReturnsYamlPathWhenYamlExists", func(t *testing.T) {
		provider, mocks := setup(t)

		// Mock Stat to return success for yaml file
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/valid/config/path", "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		path := provider.findSecretsFilePath()
		expectedPath := filepath.Join("/valid/config/path", "secrets.enc.yaml")
		if path != expectedPath {
			t.Errorf("Expected path to be %s, got %s", expectedPath, path)
		}
	})

	t.Run("ReturnsYmlPathWhenYamlDoesNotExist", func(t *testing.T) {
		provider, mocks := setup(t)

		// Mock Stat to return error for yaml file but success for yml file
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/valid/config/path", "secrets.enc.yaml") {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		path := provider.findSecretsFilePath()
		expectedPath := filepath.Join("/valid/config/path", "secrets.enc.yml")
		if path != expectedPath {
			t.Errorf("Expected path to be %s, got %s", expectedPath, path)
		}
	})
}
