package artifact

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

func TestParseOCIReference(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    *OCIArtifactInfo
		expectError bool
	}{
		{
			name:        "EmptyString",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:  "FullOCIURL",
			input: "oci://ghcr.io/windsorcli/core:v1.0.0",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core:v1.0.0",
				Tag:  "v1.0.0",
			},
			expectError: false,
		},
		{
			name:  "ShortFormat",
			input: "windsorcli/core:v1.0.0",
			expected: &OCIArtifactInfo{
				Name: "core",
				URL:  "oci://ghcr.io/windsorcli/core:v1.0.0",
				Tag:  "v1.0.0",
			},
			expectError: false,
		},
		{
			name:        "MissingVersion",
			input:       "windsorcli/core",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "InvalidFormat",
			input:       "core:v1.0.0",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "EmptyVersion",
			input:       "windsorcli/core:",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseOCIReference(tc.input)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.expected == nil {
				if result != nil {
					t.Errorf("Expected nil result but got: %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("Expected result but got nil")
				return
			}

			if result.Name != tc.expected.Name {
				t.Errorf("Expected name %s but got %s", tc.expected.Name, result.Name)
			}

			if result.URL != tc.expected.URL {
				t.Errorf("Expected URL %s but got %s", tc.expected.URL, result.URL)
			}

			if result.Tag != tc.expected.Tag {
				t.Errorf("Expected tag %s but got %s", tc.expected.Tag, result.Tag)
			}
		})
	}
}

func TestParseRegistryURL(t *testing.T) {
	t.Run("ParsesRegistryURLWithTag", func(t *testing.T) {
		// Given a registry URL with tag
		url := "ghcr.io/windsorcli/core:v1.0.0"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct
		if registryBase != "ghcr.io" {
			t.Errorf("Expected registryBase 'ghcr.io', got '%s'", registryBase)
		}
		if repoName != "windsorcli/core" {
			t.Errorf("Expected repoName 'windsorcli/core', got '%s'", repoName)
		}
		if tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithoutTag", func(t *testing.T) {
		// Given a registry URL without tag
		url := "docker.io/myuser/myblueprint"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct
		if registryBase != "docker.io" {
			t.Errorf("Expected registryBase 'docker.io', got '%s'", registryBase)
		}
		if repoName != "myuser/myblueprint" {
			t.Errorf("Expected repoName 'myuser/myblueprint', got '%s'", repoName)
		}
		if tag != "" {
			t.Errorf("Expected empty tag, got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithOCIPrefix", func(t *testing.T) {
		// Given a registry URL with oci:// prefix
		url := "oci://registry.example.com/namespace/repo:latest"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And prefix should be stripped
		if registryBase != "registry.example.com" {
			t.Errorf("Expected registryBase 'registry.example.com', got '%s'", registryBase)
		}
		if repoName != "namespace/repo" {
			t.Errorf("Expected repoName 'namespace/repo', got '%s'", repoName)
		}
		if tag != "latest" {
			t.Errorf("Expected tag 'latest', got '%s'", tag)
		}
	})

	t.Run("ParsesRegistryURLWithMultipleSlashes", func(t *testing.T) {
		// Given a registry URL with nested repository path
		url := "registry.com/org/project/subproject:v2.0"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And full repository path should be preserved
		if registryBase != "registry.com" {
			t.Errorf("Expected registryBase 'registry.com', got '%s'", registryBase)
		}
		if repoName != "org/project/subproject" {
			t.Errorf("Expected repoName 'org/project/subproject', got '%s'", repoName)
		}
		if tag != "v2.0" {
			t.Errorf("Expected tag 'v2.0', got '%s'", tag)
		}
	})

	t.Run("ReturnsErrorForInvalidFormatWithoutSlash", func(t *testing.T) {
		// Given an invalid registry URL without slash
		url := "registry.example.com"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then error should be returned
		if err == nil {
			t.Error("Expected error for invalid format, got nil")
		}

		// And error should indicate invalid format
		if !strings.Contains(err.Error(), "invalid registry format") {
			t.Errorf("Expected error about invalid format, got: %v", err)
		}

		// And components should be empty
		if registryBase != "" || repoName != "" || tag != "" {
			t.Errorf("Expected empty components on error, got: base=%s, repo=%s, tag=%s", registryBase, repoName, tag)
		}
	})

	t.Run("ReturnsErrorForEmptyString", func(t *testing.T) {
		// Given an empty URL
		url := ""

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then error should be returned
		if err == nil {
			t.Error("Expected error for empty string, got nil")
		}

		// And components should be empty
		if registryBase != "" || repoName != "" || tag != "" {
			t.Errorf("Expected empty components on error, got: base=%s, repo=%s, tag=%s", registryBase, repoName, tag)
		}
	})

	t.Run("HandlesRegistryURLWithColonInTag", func(t *testing.T) {
		// Given a registry URL with multiple colons (edge case)
		// The parser uses the last colon to separate repo from tag
		url := "registry.com/repo:tag:with:colons"

		// When parsing the URL
		registryBase, repoName, tag, err := ParseRegistryURL(url)

		// Then parsing should succeed using last colon
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And components should be correct (last colon is used for tag)
		if registryBase != "registry.com" {
			t.Errorf("Expected registryBase 'registry.com', got '%s'", registryBase)
		}
		if repoName != "repo:tag:with" {
			t.Errorf("Expected repoName 'repo:tag:with', got '%s'", repoName)
		}
		if tag != "colons" {
			t.Errorf("Expected tag 'colons', got '%s'", tag)
		}
	})
}
func TestIsAuthenticationError(t *testing.T) {
	t.Run("ReturnsTrueForWrappedTransportError401", func(t *testing.T) {
		// Given a *transport.Error with HTTP 401 wrapped by fmt.Errorf %w (the shape the push
		// path actually produces: composer.Push wraps artifact.Push wraps shim's transport.Error)
		err := fmt.Errorf("failed to push artifact: %w", &transport.Error{StatusCode: http.StatusUnauthorized})

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then errors.As should reach through the wrap chain and classify as auth
		if !result {
			t.Error("Expected true for wrapped transport.Error with status 401")
		}
	})

	t.Run("ReturnsTrueForWrappedTransportError403", func(t *testing.T) {
		// Given a *transport.Error with HTTP 403 wrapped through the push call chain
		err := fmt.Errorf("failed to push artifact: %w", &transport.Error{StatusCode: http.StatusForbidden})

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should be classified as auth
		if !result {
			t.Error("Expected true for wrapped transport.Error with status 403")
		}
	})

	t.Run("ReturnsTrueForTransportErrorWithUnauthorizedDiagnostic", func(t *testing.T) {
		// Given a *transport.Error whose HTTP status is not 401/403 but whose diagnostic
		// payload carries an UNAUTHORIZED code (some registries return 200 OK on the token
		// endpoint with the auth refusal embedded in the body diagnostics)
		err := &transport.Error{StatusCode: http.StatusOK, Errors: []transport.Diagnostic{{Code: transport.UnauthorizedErrorCode}}}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then the diagnostic code should be honored
		if !result {
			t.Error("Expected true for transport.Error with UNAUTHORIZED diagnostic code")
		}
	})

	t.Run("ReturnsTrueForTransportErrorWithDeniedDiagnostic", func(t *testing.T) {
		// Given a *transport.Error carrying a DENIED diagnostic code (ghcr.io's token-endpoint
		// shape — the exact case that triggered this fix)
		err := &transport.Error{StatusCode: http.StatusOK, Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then the diagnostic code should be honored
		if !result {
			t.Error("Expected true for transport.Error with DENIED diagnostic code")
		}
	})

	t.Run("ReturnsFalseForTransportErrorWithNonAuthStatus", func(t *testing.T) {
		// Given a *transport.Error with a status code unrelated to auth (404 from a missing manifest)
		err := &transport.Error{StatusCode: http.StatusNotFound}

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should NOT be classified as auth — the prior substring impl would falsely
		// match "404" against any error containing that digit
		if result {
			t.Error("Expected false for transport.Error 404 — not an auth failure")
		}
	})

	t.Run("ReturnsFalseForNilError", func(t *testing.T) {
		// Given a nil error
		var err error

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for nil error")
		}
	})

	t.Run("ReturnsFalseForNonTransportError", func(t *testing.T) {
		// Given an error that is not a *transport.Error and contains no embedded one — for example
		// a network timeout or a pre-flight error before any registry round-trip
		err := fmt.Errorf("network timeout: dial tcp: i/o timeout")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false — only typed registry auth errors qualify
		if result {
			t.Error("Expected false for non-transport error")
		}
	})
}
func TestValidateCliVersion(t *testing.T) {
	t.Run("ReturnsNilWhenConstraintIsEmpty", func(t *testing.T) {
		// Given an empty constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for empty constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenCliVersionIsEmpty", func(t *testing.T) {
		// Given an empty CLI version
		// When validating
		err := ValidateCliVersion("", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for empty CLI version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForDevVersion", func(t *testing.T) {
		// Given dev version
		// When validating
		err := ValidateCliVersion("dev", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for dev version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForMainVersion", func(t *testing.T) {
		// Given main version
		// When validating
		err := ValidateCliVersion("main", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for main version, got: %v", err)
		}
	})

	t.Run("ReturnsNilForLatestVersion", func(t *testing.T) {
		// Given latest version
		// When validating
		err := ValidateCliVersion("latest", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for latest version, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidCliVersionFormat", func(t *testing.T) {
		// Given an invalid CLI version format
		// When validating
		err := ValidateCliVersion("invalid-version", ">=1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid CLI version format")
		}
		if !strings.Contains(err.Error(), "invalid CLI version format") {
			t.Errorf("Expected error to contain 'invalid CLI version format', got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidConstraint", func(t *testing.T) {
		// Given an invalid constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "invalid-constraint")

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid constraint")
		}
		if !strings.Contains(err.Error(), "invalid cliVersion constraint") {
			t.Errorf("Expected error to contain 'invalid cliVersion constraint', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionDoesNotSatisfyConstraint", func(t *testing.T) {
		// Given a version that doesn't satisfy constraint
		// When validating
		err := ValidateCliVersion("1.0.0", ">=2.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesGreaterThanConstraint", func(t *testing.T) {
		// Given a version that satisfies >= constraint
		// When validating
		err := ValidateCliVersion("2.0.0", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesLessThanConstraint", func(t *testing.T) {
		// Given a version that satisfies < constraint
		// When validating
		err := ValidateCliVersion("1.0.0", "<2.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied constraint, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesRangeConstraint", func(t *testing.T) {
		// Given a version that satisfies range constraint
		// When validating
		err := ValidateCliVersion("1.5.0", ">=1.0.0 <2.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied range constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionOutsideRange", func(t *testing.T) {
		// Given a version outside range
		// When validating
		err := ValidateCliVersion("2.5.0", ">=1.0.0 <2.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when version outside range")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionSatisfiesTildeConstraint", func(t *testing.T) {
		// Given a version that satisfies ~ constraint
		// When validating
		err := ValidateCliVersion("1.2.3", "~1.2.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for satisfied tilde constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionDoesNotSatisfyTildeConstraint", func(t *testing.T) {
		// Given a version that doesn't satisfy ~ constraint
		// When validating
		err := ValidateCliVersion("1.3.0", "~1.2.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when version doesn't satisfy tilde constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenVersionWithVPrefixSatisfiesConstraint", func(t *testing.T) {
		// Given a version with v prefix that satisfies constraint
		// When validating
		err := ValidateCliVersion("v1.0.0", ">=1.0.0")

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil for v-prefixed version satisfying constraint, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVersionWithVPrefixDoesNotSatisfyConstraint", func(t *testing.T) {
		// Given a version with v prefix that doesn't satisfy constraint
		// When validating
		err := ValidateCliVersion("v0.5.0", ">=1.0.0")

		// Then should return error
		if err == nil {
			t.Error("Expected error when v-prefixed version doesn't satisfy constraint")
		}
		if !strings.Contains(err.Error(), "does not satisfy required constraint") {
			t.Errorf("Expected error to contain 'does not satisfy required constraint', got: %v", err)
		}
	})
}
