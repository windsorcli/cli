package shell

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShell_RegisterSecret(t *testing.T) {
	// setup creates a new shell with mocked dependencies for testing
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		return shell, mocks
	}

	t.Run("RegisterSingleSecret", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering a single secret value
		shell.RegisterSecret("mysecret123")

		// Then the secret should be stored in the secrets list
		if len(shell.secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(shell.secrets))
		}
		if shell.secrets[0] != "mysecret123" {
			t.Errorf("Expected secret 'mysecret123', got '%s'", shell.secrets[0])
		}
	})

	t.Run("RegisterMultipleSecrets", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering multiple different secret values
		shell.RegisterSecret("secret001")
		shell.RegisterSecret("secret002")
		shell.RegisterSecret("secret003")

		// Then all secrets should be stored in the secrets list
		if len(shell.secrets) != 3 {
			t.Errorf("Expected 3 secrets, got %d", len(shell.secrets))
		}
		expectedSecrets := []string{"secret001", "secret002", "secret003"}
		for i, expected := range expectedSecrets {
			if shell.secrets[i] != expected {
				t.Errorf("Expected secret[%d] to be '%s', got '%s'", i, expected, shell.secrets[i])
			}
		}
	})

	t.Run("RegisterEmptySecret", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When attempting to register an empty string as a secret
		shell.RegisterSecret("")

		// Then the empty secret should be ignored and not stored
		if len(shell.secrets) != 0 {
			t.Errorf("Expected 0 secrets, got %d", len(shell.secrets))
		}
	})

	t.Run("IgnoresValuesBelowMinimumLength", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering common short values that carry no real confidentiality
		for _, short := range []string{"true", "1", "admin", "dev"} {
			shell.RegisterSecret(short)
		}

		// Then none are stored, since they're shorter than minRegisteredSecretLength
		if len(shell.secrets) != 0 {
			t.Errorf("Expected 0 secrets, got %d: %v", len(shell.secrets), shell.secrets)
		}
	})

	t.Run("RegisterResolvedSecretKeepsShortValues", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering short values that a provider resolved, including an empty one
		for _, short := range []string{"abc123", "dev", ""} {
			shell.RegisterResolvedSecret(short)
		}

		// Then the non-empty values are stored despite the length floor
		if len(shell.secrets) != 2 || shell.secrets[0] != "abc123" || shell.secrets[1] != "dev" {
			t.Errorf("Expected [abc123 dev], got %v", shell.secrets)
		}
	})

	t.Run("RegisterDuplicateSecrets", func(t *testing.T) {
		// Given a shell instance with no registered secrets
		shell, _ := setup(t)

		// When registering the same secret value multiple times
		shell.RegisterSecret("duplicate")
		shell.RegisterSecret("duplicate")
		shell.RegisterSecret("duplicate")

		// Then only one instance should be stored to prevent duplicates
		if len(shell.secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(shell.secrets))
		}
		if shell.secrets[0] != "duplicate" {
			t.Errorf("Expected secret to be 'duplicate', got '%s'", shell.secrets[0])
		}
	})

	t.Run("ConcurrentRegisterAndScrubIsRaceFree", func(t *testing.T) {
		// Given a shell instance, exercised the way ExecProgress's concurrent scanner
		// goroutines and any in-flight RegisterSecret call would overlap in production
		shell, _ := setup(t)

		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(2)
			go func(i int) {
				defer wg.Done()
				shell.RegisterSecret(fmt.Sprintf("concurrent-secret-%d", i))
			}(i)
			go func() {
				defer wg.Done()
				_ = shell.scrubString("some command output")
			}()
		}
		wg.Wait()

		// Then every registration landed without a torn read/write (run with -race to verify)
		if len(shell.secrets) != 20 {
			t.Errorf("Expected 20 secrets registered, got %d", len(shell.secrets))
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestShell_scrubString(t *testing.T) {
	// setup creates a new shell with mocked dependencies for testing
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		return shell, mocks
	}

	t.Run("ScrubSingleSecret", func(t *testing.T) {
		// Given a shell with one registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("mysecret123")

		// When scrubbing a string that contains the registered secret
		input := "The password is mysecret123 and it's confidential"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks while preserving the rest of the text
		expected := "The password is ******** and it's confidential"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubMultipleSecrets", func(t *testing.T) {
		// Given a shell with multiple registered secret values
		shell, _ := setup(t)
		shell.RegisterSecret("secret001")
		shell.RegisterSecret("secret002")
		shell.RegisterSecret("secret003")

		// When scrubbing a string that contains multiple registered secrets
		input := "First secret001, then secret002, finally secret003"
		result := shell.scrubString(input)

		// Then all secrets should be replaced with asterisks
		expected := "First ********, then ********, finally ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubNoSecrets", func(t *testing.T) {
		// Given a shell with no registered secrets
		shell, _ := setup(t)

		// When scrubbing a string that contains no sensitive information
		input := "This string contains no secrets"
		result := shell.scrubString(input)

		// Then the string should remain completely unchanged
		if result != input {
			t.Errorf("Expected unchanged string '%s', got '%s'", input, result)
		}
	})

	t.Run("ScrubEmptyString", func(t *testing.T) {
		// Given a shell with registered secrets
		shell, _ := setup(t)
		shell.RegisterSecret("secret")

		// When scrubbing an empty input string
		input := ""
		result := shell.scrubString(input)

		// Then the result should remain empty
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("ScrubEmptySecret", func(t *testing.T) {
		// Given a shell with an empty string registered as a secret
		shell, _ := setup(t)
		shell.RegisterSecret("")

		// When scrubbing a normal text string
		input := "This is a test string"
		result := shell.scrubString(input)

		// Then the string should remain unchanged since empty secrets don't match anything
		if result != input {
			t.Errorf("Expected unchanged string '%s', got '%s'", input, result)
		}
	})

	t.Run("ScrubSecretAtBeginning", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")

		// When scrubbing a string where the secret appears at the beginning
		input := "secret123 is at the start"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks
		expected := "******** is at the start"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretAtEnd", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")

		// When scrubbing a string where the secret appears at the end
		input := "The secret is secret123"
		result := shell.scrubString(input)

		// Then the secret should be replaced with asterisks
		expected := "The secret is ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretMultipleOccurrences", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("mysecret")

		// When scrubbing a string where the same secret appears multiple times
		input := "mysecret appears here and mysecret appears there, mysecret everywhere"
		result := shell.scrubString(input)

		// Then all occurrences of the secret should be replaced with asterisks
		expected := "******** appears here and ******** appears there, ******** everywhere"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretPartialMatch", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("mysecret")

		// When scrubbing a string containing words that partially match the secret
		input := "mysecretive and mysecrets contain mysecret"
		result := shell.scrubString(input)

		// Then all occurrences should be replaced using simple string replacement
		expected := "********ive and ********s contain ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubLongSecret", func(t *testing.T) {
		// Given a shell with a very long secret value registered
		shell, _ := setup(t)
		longSecret := "this-is-a-very-long-secret-key-with-many-characters-1234567890"
		shell.RegisterSecret(longSecret)

		// When scrubbing a string containing the long secret
		input := fmt.Sprintf("The key is %s and should be hidden", longSecret)
		result := shell.scrubString(input)

		// Then the long secret should be replaced with a fixed-length asterisk string
		expected := "The key is ******** and should be hidden"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSpecialCharacters", func(t *testing.T) {
		// Given a shell with a secret containing special characters
		shell, _ := setup(t)
		specialSecret := "p@ssw0rd!#$%^&*()"
		shell.RegisterSecret(specialSecret)

		// When scrubbing a string containing the special character secret
		input := fmt.Sprintf("Password: %s", specialSecret)
		result := shell.scrubString(input)

		// Then the special character secret should be replaced with asterisks
		expected := "Password: ********"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubComplexMultipleSecrets", func(t *testing.T) {
		// Given a shell with multiple different registered secrets
		shell, _ := setup(t)
		shell.RegisterSecret("secret123")
		shell.RegisterSecret("password456")
		shell.RegisterSecret("token789")

		// When scrubbing a complex configuration-like string with multiple secrets
		input := "Config: secret123, Auth: password456, API: token789, Normal: text"
		result := shell.scrubString(input)

		// Then all secrets should be scrubbed while preserving non-secret text
		expected := "Config: ********, Auth: ********, API: ********, Normal: text"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInJSON", func(t *testing.T) {
		// Given a shell with registered password and token secrets
		shell, _ := setup(t)
		shell.RegisterSecret("mypassword")
		shell.RegisterSecret("mytoken1")

		// When scrubbing a JSON-formatted string containing secrets
		input := `{"password": "mypassword", "token": "mytoken1", "user": "admin"}`
		result := shell.scrubString(input)

		// Then the secrets should be scrubbed while preserving JSON structure
		expected := `{"password": "********", "token": "********", "user": "admin"}`
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInCommandLine", func(t *testing.T) {
		// Given a shell with a registered secret value
		shell, _ := setup(t)
		shell.RegisterSecret("supersecret")

		// When scrubbing a command line string containing the secret
		input := "terraform apply -var password=supersecret -var user=admin"
		result := shell.scrubString(input)

		// Then the secret should be scrubbed while preserving the command structure
		expected := "terraform apply -var password=******** -var user=admin"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("ScrubSecretsInErrorMessages", func(t *testing.T) {
		// Given a shell with a registered password secret
		shell, _ := setup(t)
		shell.RegisterSecret("badpassword")

		// When scrubbing an error message that accidentally contains the secret
		input := "Error: authentication failed with password 'badpassword'"
		result := shell.scrubString(input)

		// Then the secret should be scrubbed to prevent leakage in error logs
		expected := "Error: authentication failed with password '********'"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})
}

func TestShell_scrubStringWithMaxLen(t *testing.T) {
	// setup creates a new shell with mocked dependencies for testing
	setup := func(t *testing.T) *DefaultShell {
		t.Helper()
		setupShellMocks(t)
		return NewDefaultShell()
	}

	t.Run("ReturnsScrubbedTextAndLongestSecretLengthFromOneSnapshot", func(t *testing.T) {
		// Given a shell with secrets of different lengths registered
		shell := setup(t)
		shell.RegisterSecret("shortsecret")
		shell.RegisterSecret("averylongregisteredsecretvalue")

		// When scrubbing with the combined call
		result, maxLen := shell.scrubStringWithMaxLen("value is shortsecret here")

		// Then the text is scrubbed exactly as scrubString would, and maxLen reflects the
		// longest secret from the same locked snapshot used for the scrub
		if strings.Contains(result, "shortsecret") {
			t.Errorf("Expected secret scrubbed, got %q", result)
		}
		if want := len("averylongregisteredsecretvalue"); maxLen != want {
			t.Errorf("Expected maxLen %d, got %d", want, maxLen)
		}
	})

	t.Run("ReturnsZeroMaxLenWhenNoSecretsRegistered", func(t *testing.T) {
		// Given a shell with no registered secrets
		shell := setup(t)

		// When scrubbing with the combined call
		result, maxLen := shell.scrubStringWithMaxLen("plain text")

		// Then the text is unchanged and maxLen is zero
		if result != "plain text" {
			t.Errorf("Expected unchanged text, got %q", result)
		}
		if maxLen != 0 {
			t.Errorf("Expected maxLen 0, got %d", maxLen)
		}
	})
}
