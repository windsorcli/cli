package evaluator_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestExpressionEvaluator_enrichConfig(t *testing.T) {
	t.Run("EnrichesConfigWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		subject, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression (which enriches config)
		result, err := subject.Evaluate("${project_root}", "", nil, false)

		// Then project_root should be in enriched config
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/project" {
			t.Errorf("Expected project_root to be '/test/project', got %v", result)
		}
	})

	t.Run("EnrichesConfigWithContext", func(t *testing.T) {
		// Given an evaluator with config handler that returns context name
		subject, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextFunc = func() string {
			return "test-context"
		}

		// When evaluating an expression (which enriches config)
		result, err := subject.Evaluate("${context}", "", nil, false)

		// Then context should be in enriched config
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "test-context" {
			t.Errorf("Expected context to be 'test-context', got %v", result)
		}
	})

	t.Run("EnrichesConfigWithContextPath", func(t *testing.T) {
		// Given an evaluator with config handler
		subject, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// When evaluating an expression (which enriches config)
		result, err := subject.Evaluate("${context_path}", "", nil, false)

		// Then context_path should be in enriched config
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/config" {
			t.Errorf("Expected context_path to be '/test/config', got %v", result)
		}
	})

	t.Run("HandlesEmptyProjectRoot", func(t *testing.T) {
		// Given an evaluator without project root
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		subject := evaluator.NewExpressionEvaluator(mockConfigHandler, "", "/test/template")

		// When evaluating an expression
		result, err := subject.Evaluate("project_root", "", nil, false)

		// Then project_root should return the original string (no ${} means no evaluation)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "project_root" {
			t.Errorf("Expected project_root to be 'project_root', got %v", result)
		}
	})

	t.Run("HandlesGetConfigRootError", func(t *testing.T) {
		// Given an evaluator with config handler that returns error for GetConfigRoot
		subject, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root error")
		}
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		// When evaluating an expression (which enriches config)
		result, err := subject.Evaluate("value", "", nil, false)

		// Then evaluation should succeed without context_path
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "value" {
			t.Errorf("Expected result to be 'value', got %v", result)
		}
	})

	t.Run("HandlesNilConfigHandlerInEnrichConfig", func(t *testing.T) {
		// Given an evaluator without config handler (but this can't happen due to panic)
		// This test verifies enrichConfig handles nil configHandler gracefully
		// by testing through getConfig which returns empty map when configHandler is nil
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, nil
		}
		subject := evaluator.NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")

		// When evaluating an expression
		result, err := subject.Evaluate("${project_root}", "", nil, false)

		// Then project_root should still be available from projectRoot field
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/project" {
			t.Errorf("Expected project_root to be '/test/project', got %v", result)
		}
	})
}

func TestExpressionEvaluator_buildExprEnvironment(t *testing.T) {
	t.Run("IncludesRegisteredHelpers", func(t *testing.T) {
		// Given an evaluator with a registered helper
		subject, _, _, _ := setupEvaluatorTest(t)
		subject.Register("custom", func(params []any, deferred bool) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("custom() requires exactly 1 argument")
			}
			return "custom_result", nil
		}, new(func(string) string))

		// When evaluating an expression with the custom helper
		result, err := subject.Evaluate(`${custom("test")}`, "", nil, false)

		// Then it should use the custom helper
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "custom_result" {
			t.Errorf("Expected result to be 'custom_result', got %v", result)
		}
	})

	t.Run("AcceptsEachRegisteredSignature", func(t *testing.T) {
		// Given a helper registered with two- and three-argument signatures
		subject, _, _, _ := setupEvaluatorTest(t)
		subject.Register("pick", func(params []any, deferred bool) (any, error) {
			return len(params), nil
		}, new(func(string, string) any), new(func(string, string, string) any))

		// When evaluating calls with two and three arguments
		two, twoErr := subject.Evaluate(`${pick("a", "b")}`, "", nil, false)
		three, threeErr := subject.Evaluate(`${pick("a", "b", "c")}`, "", nil, false)

		// Then both calls compile and reach the helper
		if twoErr != nil || threeErr != nil {
			t.Fatalf("Expected no errors, got %v and %v", twoErr, threeErr)
		}
		if two != 2 || three != 3 {
			t.Errorf("Expected 2 and 3 arguments, got %v and %v", two, three)
		}
	})

	t.Run("IncludesMultipleRegisteredHelpers", func(t *testing.T) {
		// Given an evaluator with multiple registered helpers
		subject, _, _, _ := setupEvaluatorTest(t)
		subject.Register("helper1", func(params []any, deferred bool) (any, error) {
			return "result1", nil
		}, new(func() string))
		subject.Register("helper2", func(params []any, deferred bool) (any, error) {
			return "result2", nil
		}, new(func() string))

		// When evaluating expressions with both helpers
		result1, err1 := subject.Evaluate(`${helper1()}`, "", nil, false)
		result2, err2 := subject.Evaluate(`${helper2()}`, "", nil, false)

		// Then both helpers should work
		if err1 != nil {
			t.Fatalf("Expected no error for helper1, got: %v", err1)
		}
		if result1 != "result1" {
			t.Errorf("Expected result1 to be 'result1', got %v", result1)
		}
		if err2 != nil {
			t.Fatalf("Expected no error for helper2, got: %v", err2)
		}
		if result2 != "result2" {
			t.Errorf("Expected result2 to be 'result2', got %v", result2)
		}
	})

	t.Run("PassesDeferredFlagToHelpers", func(t *testing.T) {
		// Given an evaluator with a helper that checks deferred flag
		subject, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		subject.Register("deferredCheck", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=true
		result, err := subject.Evaluate(`${deferredCheck()}`, "", nil, true)

		// Then the helper should receive deferred=true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !receivedDeferred {
			t.Error("Expected helper to receive deferred=true")
		}
		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("HandlesHelperErrors", func(t *testing.T) {
		// Given an evaluator with a helper that returns an error
		subject, _, _, _ := setupEvaluatorTest(t)
		subject.Register("errorHelper", func(params []any, deferred bool) (any, error) {
			return nil, fmt.Errorf("helper error")
		}, new(func() string))

		// When evaluating an expression with the error helper
		_, err := subject.Evaluate(`${errorHelper()}`, "", nil, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error from helper, got nil")
		}
		if !strings.Contains(err.Error(), "helper error") {
			t.Errorf("Expected error to contain 'helper error', got: %v", err)
		}
	})

	t.Run("BuildsEnvironmentWithDeferredTrue", func(t *testing.T) {
		// Given an evaluator with a helper that checks deferred flag
		subject, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		subject.Register("deferredHelper", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=true
		result, err := subject.Evaluate(`${deferredHelper()}`, "", nil, true)

		// Then the helper should receive deferred=true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !receivedDeferred {
			t.Error("Expected helper to receive deferred=true")
		}
		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("BuildsEnvironmentWithDeferredFalse", func(t *testing.T) {
		// Given an evaluator with a helper that checks deferred flag
		subject, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		subject.Register("deferredHelper", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=false
		result, err := subject.Evaluate(`${deferredHelper()}`, "", nil, false)

		// Then the helper should receive deferred=false
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if receivedDeferred {
			t.Error("Expected helper to receive deferred=false")
		}
		if result != false {
			t.Errorf("Expected result to be false, got %v", result)
		}
	})
}

func TestExpressionEvaluator_buildContextMap(t *testing.T) {
	t.Run("BuildsContextMapWithName", func(t *testing.T) {
		// Given an evaluator with config handler that returns context
		subject, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextFunc = func() string {
			return "test-context"
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `{
			hasContext: std.extVar("context") != null,
			result: "success"
		}`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet that uses context
		result, err := subject.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

		// Then context should be available
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["hasContext"] != true {
			t.Errorf("Expected result.hasContext to be true, got %v", resultMap["hasContext"])
		}
	})

	t.Run("BuildsContextMapWithContextExtVar", func(t *testing.T) {
		subject, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `std.extVar("context")`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		_, err := subject.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})
}
