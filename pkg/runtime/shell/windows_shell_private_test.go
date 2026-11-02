//go:build windows
// +build windows

package shell

import (
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDefaultShell_quoteValueForPowerShell tests the quoteValueForPowerShell method
func TestDefaultShell_quoteValueForPowerShell(t *testing.T) {
	setup := func(t *testing.T) *DefaultShell {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell
	}

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		shell := setup(t)

		testCases := []struct {
			name     string
			value    string
			expected string
		}{
			{"JSONArray", `["item1","item2"]`, `'["item1","item2"]'`},
			{"JSONObject", `{"key": "value"}`, `'{"key": "value"}'`},
			{"WithSpaces", "value with spaces", "'value with spaces'"},
			{"WithDollars", "value$test", "'value$test'"},
			{"WithBrackets", "[test]", "'[test]'"},
			{"SimpleValue", "simple", "'simple'"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := shell.quoteValueForPowerShell(tc.value)
				if result != tc.expected {
					t.Errorf("Expected %q, got %q", tc.expected, result)
				}
			})
		}
	})

	t.Run("EscapesSingleQuotesByDoubling", func(t *testing.T) {
		shell := setup(t)

		result := shell.quoteValueForPowerShell("value'with'quotes")
		expected := `'value''with''quotes'`
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

// TestDefaultShell_renderEnvVarsWithExport_PowerShell tests renderEnvVarsWithExport with special characters
func TestDefaultShell_renderEnvVarsWithExport_PowerShell(t *testing.T) {
	setup := func(t *testing.T) (*DefaultShell, *ShellTestMocks) {
		t.Helper()
		mocks := setupShellMocks(t)
		shell := NewDefaultShell()
		shell.shims = mocks.Shims
		return shell, mocks
	}

	t.Run("QuotesValuesWithSpecialCharacters", func(t *testing.T) {
		// Given a shell with environment variables containing special characters
		shell, _ := setup(t)
		envVars := map[string]string{
			"VAR1": `["item1","item2"]`,
			"VAR2": `{"key": "value"}`,
			"VAR3": "simple_value",
			"VAR4": "value with spaces",
		}

		// When rendering environment variables with export
		result := shell.renderEnvVarsWithExport(envVars)

		// Then values with special characters should be quoted with single quotes
		if !strings.Contains(result, "$env:VAR1='[\"item1\",\"item2\"]'\n") {
			t.Errorf("Expected VAR1 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR2='{\"key\": \"value\"}'\n") {
			t.Errorf("Expected VAR2 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR3='simple_value'\n") {
			t.Errorf("Expected VAR3 to be quoted, got: %s", result)
		}
		if !strings.Contains(result, "$env:VAR4='value with spaces'\n") {
			t.Errorf("Expected VAR4 to be quoted, got: %s", result)
		}
	})
}
