package secrets

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/1password/onepassword-sdk-go"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

func TestNewOnePasswordSDKSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Verify the provider was created correctly
		if provider == nil {
			t.Fatalf("Expected provider to be created, got nil")
		}

		// Verify the vault properties were set correctly
		if provider.vault.Name != vault.Name {
			t.Errorf("Expected vault name to be %s, got %s", vault.Name, provider.vault.Name)
		}

		if provider.vault.ID != vault.ID {
			t.Errorf("Expected vault ID to be %s, got %s", vault.ID, provider.vault.ID)
		}
	})
}

func TestOnePasswordSDKSecretsProvider_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Initialize the provider
		err := provider.Initialize()

		// Verify the result
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Ensure environment variable is not set
		os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Initialize the provider
		err := provider.Initialize()

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})
}

func TestOnePasswordSDKSecretsProvider_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		originalResolveSecret := resolveSecret
		defer func() {
			newOnePasswordClient = originalNewClient
			resolveSecret = originalResolveSecret
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		resolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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

	t.Run("MissingToken", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Ensure environment variable is not set
		os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("ClientCreationError", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Reset global client
		globalClient = nil

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		defer func() {
			newOnePasswordClient = originalNewClient
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return nil, errors.New("client creation error")
		}

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "failed to create 1Password client: client creation error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("SecretResolutionError", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		originalResolveSecret := resolveSecret
		defer func() {
			newOnePasswordClient = originalNewClient
			resolveSecret = originalResolveSecret
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		resolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			return "", errors.New("secret resolution error")
		}

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "failed to resolve secret: secret resolution error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})

	t.Run("NilClient", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Reset global client
		globalClient = nil

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		defer func() {
			newOnePasswordClient = originalNewClient
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return nil, nil
		}

		// Call GetSecret
		value, err := provider.GetSecret("test-secret.password")

		// Verify the result
		if err == nil {
			t.Error("Expected an error, got nil")
		}

		expectedError := "failed to create 1Password client: client is nil"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be '%s', got '%s'", expectedError, err.Error())
		}

		if value != "" {
			t.Errorf("Expected value to be empty, got %s", value)
		}
	})
}

func TestOnePasswordSDKSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupSafeMocks()

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		originalResolveSecret := resolveSecret
		defer func() {
			newOnePasswordClient = originalNewClient
			resolveSecret = originalResolveSecret
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		resolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

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
			ID:   "test-id",
		}

		// Create the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Set the provider to unlocked state
		provider.unlocked = true

		// Override the shims for testing
		originalNewClient := newOnePasswordClient
		originalResolveSecret := resolveSecret
		defer func() {
			newOnePasswordClient = originalNewClient
			resolveSecret = originalResolveSecret
		}()

		// Set up the shims to use our mock
		newOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		resolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			return "", errors.New("secret not found")
		}

		// Test with a secret that doesn't exist
		input := "This is a secret: ${{ op.test-id.nonexistent-secret.password }}"
		expectedOutput := "This is a secret: <ERROR: failed to resolve: nonexistent-secret.password: failed to resolve secret: secret not found>"

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
