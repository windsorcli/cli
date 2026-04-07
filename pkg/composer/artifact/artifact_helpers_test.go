package artifact

import (
	"fmt"
	"strings"
	"testing"
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
	t.Run("ReturnsTrueForUNAUTHORIZED", func(t *testing.T) {
		// Given an error with UNAUTHORIZED
		err := fmt.Errorf("UNAUTHORIZED: access denied")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for UNAUTHORIZED error")
		}
	})

	t.Run("ReturnsTrueForUnauthorized", func(t *testing.T) {
		// Given an error with unauthorized
		err := fmt.Errorf("unauthorized access")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for unauthorized error")
		}
	})

	t.Run("ReturnsTrueForAuthenticationRequired", func(t *testing.T) {
		// Given an error with authentication required
		err := fmt.Errorf("authentication required to access this resource")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for authentication required error")
		}
	})

	t.Run("ReturnsTrueForAuthenticationFailed", func(t *testing.T) {
		// Given an error with authentication failed
		err := fmt.Errorf("authentication failed")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for authentication failed error")
		}
	})

	t.Run("ReturnsTrueForHTTP401", func(t *testing.T) {
		// Given an error with HTTP 401
		err := fmt.Errorf("HTTP 401: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for HTTP 401 error")
		}
	})

	t.Run("ReturnsTrueForHTTP403", func(t *testing.T) {
		// Given an error with HTTP 403
		err := fmt.Errorf("HTTP 403: forbidden")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for HTTP 403 error")
		}
	})

	t.Run("ReturnsTrueForBlobsUploads", func(t *testing.T) {
		// Given an error with blobs/uploads
		err := fmt.Errorf("POST https://registry.com/v2/repo/blobs/uploads: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for blobs/uploads error")
		}
	})

	t.Run("ReturnsTrueForPOSTHTTPS", func(t *testing.T) {
		// Given an error with POST https://
		err := fmt.Errorf("POST https://registry.com/v2/repo/manifests/latest: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for POST https:// error")
		}
	})

	t.Run("ReturnsTrueForFailedToPushArtifact", func(t *testing.T) {
		// Given an error with failed to push artifact
		err := fmt.Errorf("failed to push artifact: unauthorized")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for failed to push artifact error")
		}
	})

	t.Run("ReturnsTrueForUserCannotBeAuthenticated", func(t *testing.T) {
		// Given an error with User cannot be authenticated
		err := fmt.Errorf("User cannot be authenticated")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return true
		if !result {
			t.Error("Expected true for User cannot be authenticated error")
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

	t.Run("ReturnsFalseForGenericError", func(t *testing.T) {
		// Given a generic error
		err := fmt.Errorf("network timeout")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for generic error")
		}
	})

	t.Run("ReturnsFalseForParseError", func(t *testing.T) {
		// Given a parse error
		err := fmt.Errorf("failed to parse JSON")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for parse error")
		}
	})

	t.Run("ReturnsFalseForNotFoundError", func(t *testing.T) {
		// Given a not found error
		err := fmt.Errorf("resource not found")

		// When checking if it's an authentication error
		result := IsAuthenticationError(err)

		// Then it should return false
		if result {
			t.Error("Expected false for not found error")
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
