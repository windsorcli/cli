package secrets

import (
	"fmt"
	"os"
	"testing"
)

func setupSafeSopsSecretsProviderMocks() *SopsSecretsProvider {
	provider := NewSopsSecretsProvider("/valid/config/path")

	// Mock the stat function to simulate the file exists
	stat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	// Mock the decryptFileFunc to return valid decrypted content with nested keys
	decryptFileFunc = func(filePath string, format string) ([]byte, error) {
		return []byte(`
nested:
  key: value
  another:
    deep: secret
`), nil
	}

	return provider
}

func TestNewSopsSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		provider := setupSafeSopsSecretsProviderMocks()

		// When NewSopsSecretsProvider is called
		expectedPath := "/valid/config/path/secrets.enc.yml"
		if provider.secretsFilePath != expectedPath {
			t.Fatalf("expected config path to be %v, got %v", expectedPath, provider.secretsFilePath)
		}
	})
}

func TestSopsSecretsProvider_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		provider := setupSafeSopsSecretsProviderMocks()

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
		provider := setupSafeSopsSecretsProviderMocks()

		// Mock the stat function to return an error indicating the file does not exist
		stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When LoadSecrets is called
		err := provider.LoadSecrets()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		// And the error message should indicate the file does not exist
		expectedErrorMessage := fmt.Sprintf("file does not exist: %s", provider.secretsFilePath)
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}
	})

	t.Run("DecryptionFailure", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		provider := setupSafeSopsSecretsProviderMocks()

		// Mock the decryptFileFunc to return an error
		decryptFileFunc = func(_ string, _ string) ([]byte, error) {
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
		provider := setupSafeSopsSecretsProviderMocks()

		// Mock the yamlUnmarshal function to return an error
		yamlUnmarshal = func(_ []byte, _ interface{}) error {
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
