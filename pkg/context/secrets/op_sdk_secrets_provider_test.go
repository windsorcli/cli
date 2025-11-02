// The OnePasswordSDKSecretsProviderTest is a test suite for the OnePasswordSDKSecretsProvider
// It provides comprehensive testing of the 1Password SDK integration
// It serves as a validation mechanism for the provider's behavior
// It ensures the provider correctly implements the SecretsProvider interface

package secrets

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/1password/onepassword-sdk-go"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewOnePasswordSDKSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupMocks(t)

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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestOnePasswordSDKSecretsProvider_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupMocks(t)

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
		mocks := setupMocks(t)

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
			t.Error("Expected error, got nil")
		}

		expectedError := "OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK"
		if err.Error() != expectedError {
			t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("BaseInitializationFails", func(t *testing.T) {
		// Setup mocks
		mocks := setupMocks(t)

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

		// Remove shell from injector to cause base initialization to fail
		mocks.Injector.Register("shell", nil)

		// Initialize the provider
		err := provider.Initialize()

		// Verify the result
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to resolve shell instance from injector"
		if err.Error() != expectedError {
			t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
		}
	})
}

func TestOnePasswordSDKSecretsProvider_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mocks
		mocks := setupMocks(t)

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

		// Set up the shims to use our mock
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		provider.shims.ResolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			return "secret-value", nil
		}

		// Initialize the global client
		client, err := provider.shims.NewOnePasswordClient(context.Background(), onepassword.WithServiceAccountToken("test-token"))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		globalClient = client

		// Initialize the provider
		if err := provider.Initialize(); err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
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
		mocks := setupMocks(t)

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
		mocks := setupMocks(t)

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
		mocks := setupMocks(t)

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
		mocks := setupMocks(t)

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

		// Set up the shims to use our mock
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
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
		mocks := setupMocks(t)

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

		// Set up the shims to use our mock
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		provider.shims.ResolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
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
		mocks := setupMocks(t)

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

		// Set up the shims to use our mock
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
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
	// setup creates and initializes a OnePasswordSDKSecretsProvider for testing
	setup := func(t *testing.T) (*Mocks, *OnePasswordSDKSecretsProvider) {
		t.Helper()

		// Set environment variable
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		t.Cleanup(func() { os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN") })

		// Setup mocks
		mocks := setupMocks(t)

		// Create a test vault
		vault := secretsConfigType.OnePasswordVault{
			Name: "test-vault",
			ID:   "test-id",
		}

		// Create and initialize the provider
		provider := NewOnePasswordSDKSecretsProvider(vault, mocks.Injector)
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize provider: %v", err)
		}

		return mocks, provider
	}

	t.Run("Success", func(t *testing.T) {
		// Given a provider with unlocked state and mock 1Password client configured
		_, provider := setup(t)
		provider.unlocked = true
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		provider.shims.ResolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			return "secret-value", nil
		}

		// When parsing input containing a valid 1Password secret reference
		input := "This is a secret: ${{ op.test-id.test-secret.password }}"
		output, err := provider.ParseSecrets(input)

		// Then the secret reference should be replaced with the actual secret value
		expectedOutput := "This is a secret: secret-value"
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		// Given a provider with no special configuration
		_, provider := setup(t)

		// When parsing an empty input string
		input := ""
		output, err := provider.ParseSecrets(input)

		// Then the output should remain empty and unchanged
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != input {
			t.Errorf("Expected output to be '%s', got '%s'", input, output)
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		// Given a provider with no special configuration
		_, provider := setup(t)

		// When parsing input with an invalid secret reference format (missing field)
		input := "This is a secret: ${{ op.test-id.test-secret }}"
		output, err := provider.ParseSecrets(input)

		// Then the invalid reference should be replaced with an error message
		expectedOutput := "This is a secret: <ERROR: invalid key path: test-id.test-secret>"
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		// Given a provider with no special configuration
		_, provider := setup(t)

		// When parsing input with malformed JSON syntax (missing closing brace)
		input := "This is a secret: ${{ op.test-id.test-secret.password"
		output, err := provider.ParseSecrets(input)

		// Then the malformed reference should be left unchanged
		expectedOutput := "This is a secret: ${{ op.test-id.test-secret.password"
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("MismatchedVaultID", func(t *testing.T) {
		// Given a provider configured for vault "test-id"
		_, provider := setup(t)

		// When parsing input with a secret reference for a different vault ID
		input := "This is a secret: ${{ op.wrong-id.test-secret.password }}"
		output, err := provider.ParseSecrets(input)

		// Then the mismatched reference should be left unchanged
		expectedOutput := "This is a secret: ${{ op.wrong-id.test-secret.password }}"
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("SecretNotFound", func(t *testing.T) {
		// Given a provider with unlocked state and mock client that simulates secret not found
		_, provider := setup(t)
		provider.unlocked = true
		provider.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		provider.shims.ResolveSecret = func(client *onepassword.Client, ctx context.Context, secretRef string) (string, error) {
			return "", errors.New("secret not found")
		}

		// When parsing input with a reference to a nonexistent secret
		input := "This is a secret: ${{ op.test-id.nonexistent-secret.password }}"
		output, err := provider.ParseSecrets(input)

		// Then the reference should be replaced with an error message indicating the secret was not found
		expectedOutput := "This is a secret: <ERROR: failed to resolve: nonexistent-secret.password: failed to resolve secret: secret not found>"
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output to be '%s', got '%s'", expectedOutput, output)
		}
	})
}
