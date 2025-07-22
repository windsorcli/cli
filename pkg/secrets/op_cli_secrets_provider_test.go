// The OnePasswordCLISecretsProviderTest is a test suite for the OnePasswordCLISecretsProvider
// It provides comprehensive testing of the 1Password CLI integration
// It serves as a validation mechanism for the provider's behavior
// It ensures the provider correctly implements the SecretsProvider interface

package secrets

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// =============================================================================
// Test Methods
// =============================================================================

func TestNewOnePasswordCLISecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// When a new provider is created
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Then the provider should be created correctly
		if provider == nil {
			t.Fatalf("Expected provider to be created, got nil")
		}

		// And the vault properties should be set correctly
		if provider.vault.Name != vault.Name {
			t.Errorf("Expected vault name to be %s, got %s", vault.Name, provider.vault.Name)
		}

		if provider.vault.URL != vault.URL {
			t.Errorf("Expected vault URL to be %s, got %s", vault.URL, provider.vault.URL)
		}
	})
}

func TestOnePasswordCLISecretsProvider_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// And a provider initialized and unlocked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = true

		// And mock shims for command execution
		mockShims := NewShims()
		mockShims.Command = func(name string, args ...string) *exec.Cmd {
			// Verify the command and arguments
			if name != "op" {
				t.Errorf("Expected command to be 'op', got %s", name)
			}

			// Check that the arguments contain the expected values
			expectedArgs := []string{"item", "get", "test-secret", "--vault", "test-vault", "--fields", "password", "--reveal", "--account", "test-url"}
			if len(args) != len(expectedArgs) {
				t.Errorf("Expected %d arguments, got %d", len(expectedArgs), len(args))
			}

			for i, arg := range args {
				if i < len(expectedArgs) && arg != expectedArgs[i] {
					t.Errorf("Expected argument %d to be %s, got %s", i, expectedArgs[i], arg)
				}
			}

			// Return a mock command
			return &exec.Cmd{}
		}
		mockShims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("secret-value"), nil
		}
		provider.shims = mockShims

		// When GetSecret is called
		value, err := provider.GetSecret("test-secret.password")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the correct value should be returned
		if value != "secret-value" {
			t.Errorf("Expected value to be 'secret-value', got %s", value)
		}
	})

	t.Run("NotUnlocked", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// And a provider initialized but locked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = false

		// When GetSecret is called
		value, err := provider.GetSecret("test-secret.password")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And a masked value should be returned
		if value != "********" {
			t.Errorf("Expected value to be '********', got %s", value)
		}
	})

	t.Run("InvalidKeyFormat", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// And a provider initialized and unlocked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = true

		// When GetSecret is called with an invalid key format
		value, err := provider.GetSecret("invalid-key")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		// And the error message should be correct
		expectedError := "invalid key notation: invalid-key. Expected format is 'secret.field'"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		// And the value should be empty
		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("CommandExecutionError", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// And a provider initialized and unlocked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = true

		// And mock shims that return an error
		mockShims := NewShims()
		mockShims.Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		mockShims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errors.New("command execution error")
		}
		provider.shims = mockShims

		// When GetSecret is called
		value, err := provider.GetSecret("test-secret.password")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		// And the error message should contain the expected text
		if !strings.Contains(err.Error(), "failed to retrieve secret from 1Password") {
			t.Errorf("Expected error to contain 'failed to retrieve secret from 1Password', got '%s'", err.Error())
		}
		if !strings.Contains(err.Error(), "command execution error") {
			t.Errorf("Expected error to contain 'command execution error', got '%s'", err.Error())
		}

		// And the value should be empty
		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})
}

func TestParseSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized and unlocked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = true

		// And mock shims for command execution
		mockShims := NewShims()
		mockShims.Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		mockShims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("secret-value"), nil
		}
		provider.shims = mockShims

		// When ParseSecrets is called with standard notation
		input := "This is a secret: ${{ op.test-id.test-secret.password }}"
		expectedOutput := "This is a secret: secret-value"
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should be correctly replaced
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// When ParseSecrets is called with empty input
		input := ""
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should be unchanged
		if output != input {
			t.Errorf("Expected output to be '%s', got '%s'", input, output)
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// When ParseSecrets is called with invalid format (missing field)
		input := "This is a secret: ${{ op.test-id.test-secret }}"
		expectedOutput := "This is a secret: <ERROR: invalid key path: test-id.test-secret>"
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should contain an error message
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// When ParseSecrets is called with malformed JSON (missing closing brace)
		input := "This is a secret: ${{ op.test-id.test-secret.password"
		expectedOutput := "This is a secret: ${{ op.test-id.test-secret.password"
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should be unchanged
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MismatchedVaultID", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// When ParseSecrets is called with wrong vault ID
		input := "This is a secret: ${{ op.wrong-id.test-secret.password }}"
		expectedOutput := "This is a secret: ${{ op.wrong-id.test-secret.password }}"
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should be unchanged
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("SecretNotFound", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// And a test vault configuration
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// And a provider initialized and unlocked
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}
		provider.unlocked = true

		// And a mock shell that returns an error
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", errors.New("secret not found")
		}

		// When ParseSecrets is called with a secret that doesn't exist
		input := "This is a secret: ${{ op.test-id.nonexistent-secret.password }}"
		expectedOutput := "This is a secret: <ERROR: secret not found>"
		output, err := provider.ParseSecrets(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the output should contain an error message
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})
}
