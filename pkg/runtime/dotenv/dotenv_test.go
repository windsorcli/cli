package dotenv

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestParse(t *testing.T) {
	t.Run("ParsesPlainKeyValuePairs", func(t *testing.T) {
		// Given standard dotenv content
		content := "HYPERV_USER=admin\nHYPERV_HOST=hyperv.local\n"

		// When Parse is called
		result := Parse(content)

		// Then both keys should be present
		if result["HYPERV_USER"] != "admin" {
			t.Errorf("Expected HYPERV_USER=admin, got %q", result["HYPERV_USER"])
		}
		if result["HYPERV_HOST"] != "hyperv.local" {
			t.Errorf("Expected HYPERV_HOST=hyperv.local, got %q", result["HYPERV_HOST"])
		}
	})

	t.Run("SkipsCommentsBlankLinesAndMalformedLines", func(t *testing.T) {
		// Given content with comments, blank lines, and a malformed line
		content := "# a comment\n\nVALID=yes\nNOEQUALSIGN\n"

		// When Parse is called
		result := Parse(content)

		// Then only the valid key should be present
		if len(result) != 1 || result["VALID"] != "yes" {
			t.Errorf("Expected only VALID=yes, got %v", result)
		}
	})

	t.Run("TrimsWhitespaceAroundKeysAndValues", func(t *testing.T) {
		// Given content with surrounding whitespace
		content := "  SPACED_KEY  =  spaced value  \n"

		// When Parse is called
		result := Parse(content)

		// Then the key and value should be trimmed
		if _, exists := result["SPACED_KEY"]; !exists {
			t.Fatalf("Expected SPACED_KEY to be present, got %v", result)
		}
		if result["SPACED_KEY"] != "spaced value" {
			t.Errorf("Expected trimmed value 'spaced value', got %q", result["SPACED_KEY"])
		}
	})
}

func TestShouldUseCache(t *testing.T) {
	t.Run("TrueWhenUnset", func(t *testing.T) {
		// Given NO_CACHE is unset
		lookupEnv := func(string) (string, bool) { return "", false }

		// When ShouldUseCache is called
		result := ShouldUseCache(lookupEnv)

		// Then it should return true
		if !result {
			t.Error("Expected true when NO_CACHE is unset")
		}
	})

	t.Run("TrueWhenZero", func(t *testing.T) {
		// Given NO_CACHE is "0"
		lookupEnv := func(string) (string, bool) { return "0", true }

		// When ShouldUseCache is called
		result := ShouldUseCache(lookupEnv)

		// Then it should return true
		if !result {
			t.Error("Expected true when NO_CACHE=0")
		}
	})

	t.Run("FalseWhenOne", func(t *testing.T) {
		// Given NO_CACHE is "1"
		lookupEnv := func(string) (string, bool) { return "1", true }

		// When ShouldUseCache is called
		result := ShouldUseCache(lookupEnv)

		// Then it should return false
		if result {
			t.Error("Expected false when NO_CACHE=1")
		}
	})
}

func TestEvaluateExpressionValue(t *testing.T) {
	t.Run("ReturnsEvaluatedResult", func(t *testing.T) {
		// Given an evaluator that resolves an expression
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "resolved-value", nil
		}

		// When EvaluateExpressionValue is called
		result := EvaluateExpressionValue(mockEval, `${secret("op://vault/item/field")}`)

		// Then the resolved value should be returned
		if result != "resolved-value" {
			t.Errorf("Expected resolved-value, got %q", result)
		}
	})

	t.Run("ReturnsErrorMarkerOnEvaluationError", func(t *testing.T) {
		// Given an evaluator that fails
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, fmt.Errorf("resolution failed")
		}

		// When EvaluateExpressionValue is called
		result := EvaluateExpressionValue(mockEval, "${bad}")

		// Then an inline error marker should be returned
		if !strings.Contains(result, "<ERROR") {
			t.Errorf("Expected an <ERROR marker, got %q", result)
		}
	})

	t.Run("ReturnsEmptyStringForNilResult", func(t *testing.T) {
		// Given an evaluator that resolves to nil
		mockEval := evaluator.NewMockExpressionEvaluator()
		mockEval.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, nil
		}

		// When EvaluateExpressionValue is called
		result := EvaluateExpressionValue(mockEval, "${nil_expr}")

		// Then an empty string should be returned
		if result != "" {
			t.Errorf("Expected empty string, got %q", result)
		}
	})
}

func TestWarnOnLoosePermissions(t *testing.T) {
	t.Run("WarnsOnLoosePermissionsOnNonWindows", func(t *testing.T) {
		// Given a group/world-readable file on a POSIX-like OS
		var buf bytes.Buffer

		// When WarnOnLoosePermissions is called
		WarnOnLoosePermissions(&buf, "linux", "/path/.env", os.FileMode(0644))

		// Then a warning should be written
		if !strings.Contains(buf.String(), "Warning") {
			t.Errorf("Expected a warning, got %q", buf.String())
		}
	})

	t.Run("NoWarningOnRestrictedPermissions", func(t *testing.T) {
		// Given an owner-only file on a POSIX-like OS
		var buf bytes.Buffer

		// When WarnOnLoosePermissions is called
		WarnOnLoosePermissions(&buf, "linux", "/path/.env", os.FileMode(0600))

		// Then no warning should be written
		if buf.String() != "" {
			t.Errorf("Expected no warning, got %q", buf.String())
		}
	})

	t.Run("NoWarningOnWindowsRegardlessOfPermissions", func(t *testing.T) {
		// Given a file that would trip the check, on Windows
		var buf bytes.Buffer

		// When WarnOnLoosePermissions is called
		WarnOnLoosePermissions(&buf, "windows", "/path/.env", os.FileMode(0644))

		// Then no warning should be written
		if buf.String() != "" {
			t.Errorf("Expected no warning on Windows, got %q", buf.String())
		}
	})
}
