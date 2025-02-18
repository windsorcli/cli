package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeSopsSecretsProviderMocks(injector ...di.Injector) *MockSafeComponents {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

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

	return &MockSafeComponents{
		Injector: mockInjector,
		Shell:    shell.NewMockShell(),
	}
}

func TestNewSopsSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)

		// When NewSopsSecretsProvider is called
		expectedPath := filepath.Join("/valid/config/path", "secrets.enc.yaml")
		if provider.secretsFilePath != expectedPath {
			t.Fatalf("expected config path to be %v, got %v", expectedPath, provider.secretsFilePath)
		}
	})
}

func TestSopsSecretsProvider_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new SopsSecretsProvider with a valid config path
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)

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
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/invalid/config/path", mocks.Injector)

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
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)

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
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)

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

func TestSopsSecretsProvider_GetSecret(t *testing.T) {
	t.Run("ReturnsMaskedValueWhenLocked", func(t *testing.T) {
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)
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
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)
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
}

func TestSopsSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("ReplacesSecretSuccessfully", func(t *testing.T) {
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)
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

	t.Run("ReturnsErrorWhenSecretNotFound", func(t *testing.T) {
		mocks := setupSafeSopsSecretsProviderMocks()
		provider := NewSopsSecretsProvider("/valid/config/path", mocks.Injector)
		provider.unlocked = true // Simulate that secrets have been unlocked

		// Test with standard notation
		input1 := "This is a secret: ${{ sops.non_existent_key }}"
		expectedOutput1 := "This is a secret: <ERROR: secret not found: non_existent_key>"

		output1, err := provider.ParseSecrets(input1)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output1 != expectedOutput1 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output1, expectedOutput1)
		}

		// Test with spaces in the notation
		input2 := "This is a secret: ${{  sops.non_existent_key  }}"
		expectedOutput2 := "This is a secret: <ERROR: secret not found: non_existent_key>"

		output2, err := provider.ParseSecrets(input2)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output2 != expectedOutput2 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output2, expectedOutput2)
		}
	})
}
