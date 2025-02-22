package secrets

import (
	"fmt"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockComponents struct {
	Shell    *shell.MockShell
	Injector di.Injector
}

func setupOnePasswordCLISecretsProviderMocks(injector ...di.Injector) *MockComponents {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewInjector()
	}

	// Create a new mock shell
	mockShell := shell.NewMockShell()

	mockInjector.Register("shell", mockShell)

	return &MockComponents{
		Shell:    mockShell,
		Injector: mockInjector,
	}
}

func TestOnePasswordCLISecretsProvider_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			if command == "op" && args[0] == "signin" && args[1] == "--account" && args[2] == "https://example.1password.com" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		err = provider.LoadSecrets()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !provider.unlocked {
			t.Errorf("expected provider to be unlocked")
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})

	t.Run("SigninFailure", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			if command == "op" && args[0] == "signin" && args[1] == "--account" && args[2] == "https://example.1password.com" {
				return "", fmt.Errorf("signin error")
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		err = provider.LoadSecrets()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedErrorMessage := "failed to sign in to 1Password: signin error"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})
}

func TestOnePasswordCLISecretsProvider_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			if command == "op" &&
				args[0] == "item" &&
				args[1] == "get" &&
				args[2] == "secretName" &&
				args[3] == "--vault" &&
				args[4] == "ExampleVault" &&
				args[5] == "--fields" &&
				args[6] == "fieldName" &&
				args[7] == "--reveal" {
				return "secretValue", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set the provider as unlocked
		provider.unlocked = true

		// Retrieve the secret
		secret, err := provider.GetSecret("secretName.fieldName")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if secret != "secretValue" {
			t.Errorf("expected secretValue, got %s", secret)
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})

	t.Run("InvalidKeyFormat", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Set the provider as unlocked
		provider.unlocked = true

		// Attempt to retrieve a secret with an invalid key format
		_, err = provider.GetSecret("invalidKeyFormat")
		if err == nil {
			t.Fatalf("expected an error due to invalid key format, got nil")
		}

		expectedErrorMessage := "invalid key notation: invalidKeyFormat. Expected format is 'secret.field'"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message to be %v, got %v", expectedErrorMessage, err.Error())
		}
	})

	t.Run("RedactedSecretWhenLocked", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Ensure the provider is locked
		provider.unlocked = false

		// Attempt to retrieve a secret while locked
		secret, err := provider.GetSecret("secretName.fieldName")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedRedactedSecret := "********"
		if secret != expectedRedactedSecret {
			t.Errorf("expected redacted secret %s, got %s", expectedRedactedSecret, secret)
		}
	})
}

func TestOnePasswordCLISecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
			ID:   "exampleVaultID",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			if command == "op" && args[0] == "item" && args[1] == "get" && args[2] == "secretName" && args[3] == "--vault" && args[4] == "ExampleVault" && args[5] == "--fields" && args[6] == "fieldName" {
				return "secretValue", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Set the provider as unlocked
		provider.unlocked = true

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		input := "The secret is ${{ op.exampleVaultID.secretName.fieldName }}."
		expectedOutput := "The secret is secretValue."

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("expected %s, got %s", expectedOutput, output)
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})

	t.Run("MissingSecret", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
			ID:   "exampleVaultID",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			return "", fmt.Errorf("item not found")
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Set the provider as unlocked
		provider.unlocked = true

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		input := "The secret is ${{ op.exampleVaultID.nonExistentSecret.fieldName }}"
		expectedOutput := "The secret is <ERROR: secret not found>"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("expected %q, got %q", expectedOutput, output)
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})

	t.Run("ReturnsErrorForInvalidKeyPath", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
			ID:   "exampleVaultID",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Set the provider as unlocked
		provider.unlocked = true

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// Test with invalid key path
		input := "This is a secret: ${{ op.invalidFormat }}"
		expectedOutput := "This is a secret: <ERROR: invalid key path>"

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("expected %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ReturnsErrorForEmptySecret", func(t *testing.T) {
		vault := secretsConfigType.OnePasswordVault{
			URL:  "https://example.1password.com",
			Name: "ExampleVault",
			ID:   "exampleVaultID",
		}

		// Setup mocks
		mocks := setupOnePasswordCLISecretsProviderMocks()
		execSilentCalled := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execSilentCalled = true
			if command == "op" && args[0] == "item" && args[1] == "get" && args[2] == "emptySecret" && args[3] == "--vault" && args[4] == "ExampleVault" && args[5] == "--fields" && args[6] == "fieldName" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}

		// Pass the injector from mocks to the provider
		provider := NewOnePasswordCLISecretsProvider(vault, mocks.Injector)

		// Set the provider as unlocked
		provider.unlocked = true

		// Initialize the provider
		err := provider.Initialize()
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		input := "The secret is ${{ op.exampleVaultID.emptySecret.fieldName }}."
		expectedOutput := "The secret is ."

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != expectedOutput {
			t.Errorf("expected %q, got %q", expectedOutput, output)
		}

		if !execSilentCalled {
			t.Errorf("expected ExecSilent to be called, but it was not")
		}
	})
}
