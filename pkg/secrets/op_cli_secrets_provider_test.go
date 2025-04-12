package secrets

import (
	"errors"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

func TestNewOnePasswordCLISecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Verify the provider was created correctly
		if provider == nil {
			t.Fatalf("Expected provider to be created, got nil")
		}

		// Verify the vault properties were set correctly
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
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to unlocked state
		provider.unlocked = true

		// Mock the shell.ExecSilent function to return a successful result
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			// Verify the command and arguments
			if command != "op" {
				t.Errorf("Expected command to be 'op', got %s", command)
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

			// Return a successful result
			return "secret-value", nil
		}

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if value != "secret-value" {
			t.Errorf("Expected value to be 'secret-value', got %s", value)
		}
	})

	t.Run("NotUnlocked", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to locked state
		provider.unlocked = false

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if value != "********" {
			t.Errorf("Expected value to be '********', got %s", value)
		}
	})

	t.Run("InvalidKeyFormat", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to unlocked state
		provider.unlocked = true

		// Call GetSecret with an invalid key format
		value, err := provider.GetSecret("invalid-key")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "invalid key notation: invalid-key. Expected format is 'secret.field'"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("CommandExecutionError", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to unlocked state
		provider.unlocked = true

		// Mock the shell.ExecSilent function to return an error
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", errors.New("command execution error")
		}

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "failed to retrieve secret from 1Password: command execution error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})
}

func TestParseSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to unlocked state
		provider.unlocked = true

		// Mock the shell.ExecSilent function to return a successful result
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "secret-value", nil
		}

		// Test with standard notation
		input := "This is a secret: ${{ op.test-id.test-secret.password }}"
		expectedOutput := "This is a secret: secret-value"

		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Test with empty input
		input := ""
		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != input {
			t.Errorf("Expected output to be '%s', got '%s'", input, output)
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Test with invalid format (missing field)
		input := "This is a secret: ${{ op.test-id.test-secret }}"
		expectedOutput := "This is a secret: <ERROR: invalid key path: test-id.test-secret>"

		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Test with malformed JSON (missing closing brace)
		input := "This is a secret: ${{ op.test-id.test-secret.password"
		expectedOutput := "This is a secret: ${{ op.test-id.test-secret.password"

		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MismatchedVaultID", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Test with wrong vault ID
		input := "This is a secret: ${{ op.wrong-id.test-secret.password }}"
		expectedOutput := "This is a secret: ${{ op.wrong-id.test-secret.password }}"

		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("SecretNotFound", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			URL:  "test-url",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		// Set the provider to unlocked state
		provider.unlocked = true

		// Mock the shell.ExecSilent function to return an error
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", errors.New("secret not found")
		}

		// Test with a secret that doesn't exist
		input := "This is a secret: ${{ op.test-id.nonexistent-secret.password }}"
		expectedOutput := "This is a secret: <ERROR: secret not found>"

		output, err := provider.ParseSecrets(input)

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})
}
