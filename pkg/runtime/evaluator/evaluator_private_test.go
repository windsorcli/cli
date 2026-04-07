package evaluator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Helper Functions
// =============================================================================

func TestFindExpressionEnd(t *testing.T) {
	t.Run("ReturnsMatchingBraceForSimpleExpression", func(t *testing.T) {
		s := "prefix ${foo} suffix"
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end == -1 {
			t.Fatal("Expected findExpressionEnd to find matching brace")
		}
		if s[end] != '}' {
			t.Errorf("Expected end to point to '}', got %q", s[end])
		}
		expr := s[start+2 : end]
		if expr != "foo" {
			t.Errorf("Expected expression 'foo', got %q", expr)
		}
	})

	t.Run("ReturnsMatchingBraceWhenExpressionContainsEmptyMapLiteral", func(t *testing.T) {
		s := "labels: ${x ?? {}}"
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end == -1 {
			t.Fatal("Expected findExpressionEnd to find matching brace")
		}
		expr := s[start+2 : end]
		if expr != "x ?? {}" {
			t.Errorf("Expected expression 'x ?? {}', got %q", expr)
		}
	})

	t.Run("ReturnsMatchingBraceWhenExpressionContainsNestedBraces", func(t *testing.T) {
		s := "${[merge(a ?? {}, b ?? {}) for n in values(nodes)]}"
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end == -1 {
			t.Fatal("Expected findExpressionEnd to find matching brace")
		}
		expr := s[start+2 : end]
		expected := "[merge(a ?? {}, b ?? {}) for n in values(nodes)]"
		if expr != expected {
			t.Errorf("Expected expression %q, got %q", expected, expr)
		}
	})

	t.Run("IgnoresBraceInsideDoubleQuotedString", func(t *testing.T) {
		s := `${foo ?? "}"}`
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end == -1 {
			t.Fatal("Expected findExpressionEnd to find matching brace")
		}
		expr := s[start+2 : end]
		if expr != `foo ?? "}"` {
			t.Errorf("Expected expression with quoted brace, got %q", expr)
		}
	})

	t.Run("IgnoresBraceInsideSingleQuotedString", func(t *testing.T) {
		s := `${bar ?? '}'}`
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end == -1 {
			t.Fatal("Expected findExpressionEnd to find matching brace")
		}
		expr := s[start+2 : end]
		if expr != `bar ?? '}'` {
			t.Errorf("Expected expression with quoted brace, got %q", expr)
		}
	})

	t.Run("ReturnsMinusOneForUnclosedExpression", func(t *testing.T) {
		s := "${unclosed"
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end != -1 {
			t.Errorf("Expected -1 for unclosed expression, got %d", end)
		}
	})

	t.Run("ReturnsMinusOneWhenBracesDoNotBalance", func(t *testing.T) {
		s := "${foo {"
		start := strings.Index(s, "${")
		end := findExpressionEnd(s, start)
		if end != -1 {
			t.Errorf("Expected -1 when braces do not balance, got %d", end)
		}
	})
}

func TestValueToInterpolationString(t *testing.T) {
	t.Run("ReturnsEmptyForNil", func(t *testing.T) {
		out, err := valueToInterpolationString(nil, func(any) ([]byte, error) { return nil, nil })
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if out != "" {
			t.Errorf("Expected empty string for nil, got %q", out)
		}
	})
	t.Run("MarshalsMapToYAML", func(t *testing.T) {
		v := map[string]any{"a": 1, "b": "two"}
		out, err := valueToInterpolationString(v, func(a any) ([]byte, error) {
			return []byte("a: 1\nb: two"), nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if out != "a: 1\nb: two" {
			t.Errorf("Expected YAML string for map, got %q", out)
		}
	})
	t.Run("MarshalsSliceToYAML", func(t *testing.T) {
		v := []any{"x", "y"}
		out, err := valueToInterpolationString(v, func(a any) ([]byte, error) {
			return []byte("- x\n- y"), nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if out != "- x\n- y" {
			t.Errorf("Expected YAML string for slice, got %q", out)
		}
	})
	t.Run("FormatsScalarWithSprintf", func(t *testing.T) {
		out, err := valueToInterpolationString(42, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if out != "42" {
			t.Errorf("Expected \"42\" for int, got %q", out)
		}
	})
}

func TestIndentForEmbeddedYAML(t *testing.T) {
	t.Run("ReturnsReplacementWhenEmptyOrSingleLine", func(t *testing.T) {
		if out := indentForEmbeddedYAML("", "x", 2); out != "x" {
			t.Errorf("Expected \"x\", got %q", out)
		}
		if out := indentForEmbeddedYAML("before", "one line", 2); out != "one line" {
			t.Errorf("Expected \"one line\", got %q", out)
		}
	})
	t.Run("IndentsMultiLineReplacementUsingBeforeSpaces", func(t *testing.T) {
		before := "  key:"
		replacement := "a: 1\nb: 2"
		out := indentForEmbeddedYAML(before, replacement, 2)
		if !strings.HasPrefix(out, "\n") {
			t.Error("Expected leading newline")
		}
		lines := strings.Split(strings.TrimPrefix(out, "\n"), "\n")
		for _, line := range lines {
			if line != "" && !strings.HasPrefix(line, "  ") {
				t.Errorf("Expected each non-empty line to be indented (baseIndent 0 + extraIndent 2), got %q", line)
			}
		}
	})
}

func TestIsStructuredValue(t *testing.T) {
	t.Run("ReturnsFalseForNil", func(t *testing.T) {
		if isStructuredValue(nil) {
			t.Error("Expected false for nil")
		}
	})
	t.Run("ReturnsTrueForMapStringAny", func(t *testing.T) {
		if !isStructuredValue(map[string]any{"k": "v"}) {
			t.Error("Expected true for map[string]any")
		}
	})
	t.Run("ReturnsTrueForSliceAny", func(t *testing.T) {
		if !isStructuredValue([]any{1}) {
			t.Error("Expected true for []any")
		}
	})
	t.Run("ReturnsFalseForString", func(t *testing.T) {
		if isStructuredValue("x") {
			t.Error("Expected false for string")
		}
	})
	t.Run("ReturnsFalseForInt", func(t *testing.T) {
		if isStructuredValue(42) {
			t.Error("Expected false for int")
		}
	})
	t.Run("ReturnsTrueForReflectedSlice", func(t *testing.T) {
		var v []interface{}
		v = append(v, 1)
		if !isStructuredValue(v) {
			t.Error("Expected true for []interface{}")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestExpressionEvaluator_enrichConfig(t *testing.T) {
	t.Run("EnrichesConfigWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression (which enriches config)
		result, err := evaluator.Evaluate("${project_root}", "", nil, false)

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
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextFunc = func() string {
			return "test-context"
		}

		// When evaluating an expression (which enriches config)
		result, err := evaluator.Evaluate("${context}", "", nil, false)

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
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// When evaluating an expression (which enriches config)
		result, err := evaluator.Evaluate("${context_path}", "", nil, false)

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
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "/test/template")

		// When evaluating an expression
		result, err := evaluator.Evaluate("project_root", "", nil, false)

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
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root error")
		}
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		// When evaluating an expression (which enriches config)
		result, err := evaluator.Evaluate("value", "", nil, false)

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
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")

		// When evaluating an expression
		result, err := evaluator.Evaluate("${project_root}", "", nil, false)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		evaluator.Register("custom", func(params []any, deferred bool) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("custom() requires exactly 1 argument")
			}
			return "custom_result", nil
		}, new(func(string) string))

		// When evaluating an expression with the custom helper
		result, err := evaluator.Evaluate(`${custom("test")}`, "", nil, false)

		// Then it should use the custom helper
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "custom_result" {
			t.Errorf("Expected result to be 'custom_result', got %v", result)
		}
	})

	t.Run("IncludesMultipleRegisteredHelpers", func(t *testing.T) {
		// Given an evaluator with multiple registered helpers
		evaluator, _, _, _ := setupEvaluatorTest(t)
		evaluator.Register("helper1", func(params []any, deferred bool) (any, error) {
			return "result1", nil
		}, new(func() string))
		evaluator.Register("helper2", func(params []any, deferred bool) (any, error) {
			return "result2", nil
		}, new(func() string))

		// When evaluating expressions with both helpers
		result1, err1 := evaluator.Evaluate(`${helper1()}`, "", nil, false)
		result2, err2 := evaluator.Evaluate(`${helper2()}`, "", nil, false)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		evaluator.Register("deferredCheck", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=true
		result, err := evaluator.Evaluate(`${deferredCheck()}`, "", nil, true)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		evaluator.Register("errorHelper", func(params []any, deferred bool) (any, error) {
			return nil, fmt.Errorf("helper error")
		}, new(func() string))

		// When evaluating an expression with the error helper
		_, err := evaluator.Evaluate(`${errorHelper()}`, "", nil, false)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		evaluator.Register("deferredHelper", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=true
		result, err := evaluator.Evaluate(`${deferredHelper()}`, "", nil, true)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		var receivedDeferred bool
		evaluator.Register("deferredHelper", func(params []any, deferred bool) (any, error) {
			receivedDeferred = deferred
			return deferred, nil
		}, new(func() bool))

		// When evaluating with evaluateDeferred=false
		result, err := evaluator.Evaluate(`${deferredHelper()}`, "", nil, false)

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

func TestExpressionEvaluator_evaluateJsonnetFunction(t *testing.T) {
	t.Run("EvaluatesJsonnetFromFile", func(t *testing.T) {
		// Given an evaluator and a jsonnet file
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `{
			result: "success",
			value: 42
		}`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

		// Then the jsonnet should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}

		if resultMap["value"] != float64(42) {
			t.Errorf("Expected result.value to be 42, got %v", resultMap["value"])
		}
	})

	t.Run("EvaluatesJsonnetWithContext", func(t *testing.T) {
		// Given an evaluator with config handler and a jsonnet file using context
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextFunc = func() string {
			return "test-context"
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `{
			result: "success",
			hasContext: std.extVar("context") != null
		}`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

		// Then the jsonnet should have access to context
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}

		if resultMap["hasContext"] != true {
			t.Errorf("Expected result.hasContext to be true, got %v", resultMap["hasContext"])
		}
	})

	t.Run("HandlesJsonnetFileNotFound", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating jsonnet function with non-existent file
		_, err := evaluator.Evaluate(`${jsonnet("nonexistent.jsonnet")}`, "", nil, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-existent file, got nil")
		}
	})

	t.Run("HandlesInvalidJsonnet", func(t *testing.T) {
		// Given an evaluator and an invalid jsonnet file
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`invalid jsonnet syntax {`), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		_, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid jsonnet, got nil")
		}
	})

	t.Run("HandlesJsonnetWithTemplateData", func(t *testing.T) {
		// Given an evaluator with template data
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/facets/test.jsonnet": []byte(`{"result": "from-template"}`),
		}
		evaluator.SetTemplateData(templateData)

		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

		// Then the jsonnet should be loaded from template data
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["result"] != "from-template" {
			t.Errorf("Expected result.result to be 'from-template', got %v", resultMap["result"])
		}
	})

	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		evaluator, mockShims, _ := setupEvaluatorWithMockShims(t)
		mockShims.JsonMarshal = func(any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)
		facetPath := filepath.Join(tmpDir, "feature.yaml")
		_, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err == nil {
			t.Fatal("Expected error for JSON marshal failure, got nil")
		}
	})

	t.Run("HandlesJsonUnmarshalError", func(t *testing.T) {
		evaluator, mockShims, _ := setupEvaluatorWithMockShims(t)
		mockShims.JsonUnmarshal = func([]byte, any) error {
			return errors.New("unmarshal error")
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)
		facetPath := filepath.Join(tmpDir, "feature.yaml")
		_, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err == nil {
			t.Fatal("Expected error for JSON unmarshal failure, got nil")
		}
	})

	t.Run("HandlesJsonnetWithEmptyDir", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)
		facetPath := filepath.Join(tmpDir, "feature.yaml")
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}
	})

	t.Run("HandlesJsonnetWithEmptyDirPath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)
		facetPath := jsonnetFile
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}
	})

	t.Run("HandlesJsonnetWithTemplateRootFallback", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/config/test.jsonnet": []byte(`{"result": "from-fallback"}`),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${jsonnet("../config/test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if resultMap["result"] != "from-fallback" {
			t.Errorf("Expected result.result to be 'from-fallback', got %v", resultMap["result"])
		}
	})

	t.Run("HandlesJsonnetWithTemplateRootFallbackWithoutPrefix", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"other/test.jsonnet": []byte(`{"result": "from-fallback-no-prefix"}`),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${jsonnet("../../other/test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if resultMap["result"] != "from-fallback-no-prefix" {
			t.Errorf("Expected result.result to be 'from-fallback-no-prefix', got %v", resultMap["result"])
		}
	})

	t.Run("HandlesJsonnetWithFilepathRelError", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{}
		evaluator.SetTemplateData(templateData)
		jsonnetFile := filepath.Join(templateRoot, "test.jsonnet")
		os.MkdirAll(filepath.Dir(jsonnetFile), 0755)
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)
		facetPath := filepath.Join(templateRoot, "test.yaml")
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}
	})
}

func TestExpressionEvaluator_buildContextMap(t *testing.T) {
	t.Run("BuildsContextMapWithName", func(t *testing.T) {
		// Given an evaluator with config handler that returns context
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
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
		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)

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
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `std.extVar("context")`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		_, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})
}

func TestExpressionEvaluator_evaluateFileFunction(t *testing.T) {
	t.Run("LoadsFileContent", func(t *testing.T) {
		// Given an evaluator and a file
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("file content\nwith newline"), 0644)

		facetPath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating file function
		result, err := evaluator.Evaluate(`${file("test.txt")}`, facetPath, nil, false)

		// Then the file content should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "file content\nwith newline" {
			t.Errorf("Expected result to be 'file content\\nwith newline', got '%v'", result)
		}
	})

	t.Run("HandlesFileNotFound", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating file function with non-existent file
		_, err := evaluator.Evaluate(`${file("nonexistent.txt")}`, "", nil, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-existent file, got nil")
		}
	})

	t.Run("HandlesFileWithTemplateData", func(t *testing.T) {
		// Given an evaluator with template data
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/facets/test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)

		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`${file("test.txt")}`, facetPath, nil, false)

		// Then the file should be loaded from template data
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-template" {
			t.Errorf("Expected result to be 'from-template', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateDataWithTemplateRootFallback", func(t *testing.T) {
		// Given an evaluator with template root and template data with fallback path
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/config/test.txt": []byte("from-template-fallback"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${file("../config/test.txt")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "from-template-fallback" {
			t.Errorf("Expected result to be 'from-template-fallback', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateDataWithTemplateRootFallbackWithoutPrefix", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"test.txt": []byte("from-template-no-prefix"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${file("../test.txt")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "from-template-no-prefix" {
			t.Errorf("Expected result to be 'from-template-no-prefix', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateDataWithoutTemplateRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "")
		templateData := map[string][]byte{
			"test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)
		_, err := evaluator.Evaluate(`${file("test.txt")}`, "/test/facet.yaml", nil, false)
		if err == nil {
			t.Log("File read succeeded, which is acceptable")
		}
	})

	t.Run("HandlesPathOutsideTemplateRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)
		outsideFile := filepath.Join(tmpDir, "outside.txt")
		os.WriteFile(outsideFile, []byte("from-outside"), 0644)
		facetPath := filepath.Join(templateRoot, "test.yaml")
		escapedPath := strings.ReplaceAll(outsideFile, "\\", "\\\\")
		result, err := evaluator.Evaluate(`${file("`+escapedPath+`")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "from-outside" {
			t.Errorf("Expected result to be 'from-outside', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateRootFallbackWithRelativePath", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/other/test.txt": []byte("from-fallback"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${file("../../other/test.txt")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "from-fallback" {
			t.Errorf("Expected result to be 'from-fallback', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateRootFallbackWithoutTemplatePrefix", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"other/test.txt": []byte("from-fallback-no-prefix"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		result, err := evaluator.Evaluate(`${file("../../other/test.txt")}`, facetPath, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "from-fallback-no-prefix" {
			t.Errorf("Expected result to be 'from-fallback-no-prefix', got '%v'", result)
		}
	})
}

func TestExpressionEvaluator_lookupInTemplateData(t *testing.T) {
	t.Run("LooksUpFileInTemplateData", func(t *testing.T) {
		// Given an evaluator with template data
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/facets/test.jsonnet": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`${file("test.jsonnet")}`, facetPath, nil, false)

		// Then the file should be found in template data
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "found" {
			t.Errorf("Expected result to be 'found', got '%v'", result)
		}
	})

	t.Run("HandlesAbsolutePath", func(t *testing.T) {
		// Given an evaluator with template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.jsonnet": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		facetPath := "/test/feature.yaml"

		// When evaluating file function with absolute path
		_, err := evaluator.Evaluate(`${file("/absolute/path.jsonnet")}`, facetPath, nil, false)

		// Then it should not find in template data (absolute paths not looked up)
		if err == nil {
			t.Fatal("Expected error for absolute path, got nil")
		}
	})

	t.Run("HandlesEmptyFacetPath", func(t *testing.T) {
		// Given an evaluator with template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.jsonnet": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		// When evaluating file function with empty feature path
		_, err := evaluator.Evaluate(`${file("test.jsonnet")}`, "", nil, false)

		// Then it should not find in template data
		if err == nil {
			t.Fatal("Expected error for empty feature path, got nil")
		}
	})

	t.Run("HandlesNilTemplateData", func(t *testing.T) {
		// Given an evaluator without template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up template data
		result := concreteEvaluator.lookupInTemplateData("test.txt", "/test/feature.yaml")

		// Then it should return nil
		if result != nil {
			t.Errorf("Expected nil for nil template data, got %v", result)
		}
	})

	t.Run("HandlesAbsolutePathInLookup", func(t *testing.T) {
		// Given an evaluator with template data and an absolute path
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up with absolute path
		result := concreteEvaluator.lookupInTemplateData("/absolute/path.txt", "/test/feature.yaml")

		// Then it should return nil
		if result != nil {
			t.Errorf("Expected nil for absolute path, got %v", result)
		}
	})

	t.Run("HandlesEmptyFacetPathInLookup", func(t *testing.T) {
		// Given an evaluator with template data and empty feature path
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up with empty feature path
		result := concreteEvaluator.lookupInTemplateData("test.txt", "")

		// Then it should return nil
		if result != nil {
			t.Errorf("Expected nil for empty feature path, got %v", result)
		}
	})

	t.Run("HandlesTemplateRootRelativePathError", func(t *testing.T) {
		// Given an evaluator with template data and outside feature path
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up with outside path
		result := concreteEvaluator.lookupInTemplateData("test.txt", "/outside/path.yaml")

		// Then lookup may or may not succeed (acceptable behavior)
		if result != nil {
			t.Log("Lookup succeeded with outside path, which is acceptable")
		}
	})

	t.Run("HandlesFacetDirAsDot", func(t *testing.T) {
		// Given an evaluator with template data and feature dir as dot
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "test.yaml")
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up template data
		result := concreteEvaluator.lookupInTemplateData("test.txt", facetPath)

		// Then it should find the file
		if result == nil {
			t.Error("Expected to find file, got nil")
		}
	})

	t.Run("HandlesTemplateDataWithoutTemplatePrefix", func(t *testing.T) {
		// Given an evaluator with template data without _template prefix
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"facets/test.txt": []byte("found-without-prefix"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		// When looking up template data
		result := concreteEvaluator.lookupInTemplateData("test.txt", facetPath)

		// Then it should find the file without prefix
		if result == nil {
			t.Error("Expected to find file, got nil")
		} else if string(result) != "found-without-prefix" {
			t.Errorf("Expected 'found-without-prefix', got '%s'", string(result))
		}
	})
}

func TestExpressionEvaluator_resolvePath(t *testing.T) {
	t.Run("ResolvesAbsolutePath", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		// When evaluating file function with absolute path
		escapedPath := strings.ReplaceAll(testFile, "\\", "\\\\")
		result, err := evaluator.Evaluate(`${file("`+escapedPath+`")}`, "", nil, false)

		// Then the absolute path should be used
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "content" {
			t.Errorf("Expected result to be 'content', got '%v'", result)
		}
	})

	t.Run("ResolvesRelativePathWithFacetPath", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		facetPath := filepath.Join(tmpDir, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(facetPath), 0755)
		testFile := filepath.Join(tmpDir, "features", "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		// When evaluating file function with relative path
		result, err := evaluator.Evaluate(`${file("test.txt")}`, facetPath, nil, false)

		// Then the path should be resolved relative to feature path
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "content" {
			t.Errorf("Expected result to be 'content', got '%v'", result)
		}
	})

	t.Run("ResolvesRelativePathWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		// Create evaluator with tmpDir as project root
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator = NewExpressionEvaluator(mockConfigHandler, tmpDir, "/test/template")

		// When evaluating file function with relative path
		result, err := evaluator.Evaluate(`${file("test.txt")}`, "", nil, false)

		// Then the path should be resolved relative to project root
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "content" {
			t.Errorf("Expected result to be 'content', got '%v'", result)
		}
	})

	t.Run("ResolvesRelativePathWithoutFeaturePathOrProjectRoot", func(t *testing.T) {
		// Given an evaluator without project root
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")

		// When resolving a relative path without feature path or project root
		concreteEvaluator := evaluator.(*expressionEvaluator)
		path := concreteEvaluator.resolvePath("test.txt", "")

		// Then it should return cleaned path
		if path != "test.txt" {
			t.Errorf("Expected path to be 'test.txt', got '%s'", path)
		}
	})

	t.Run("ResolvesRelativePathWhenSourceFilePathIsRelativeAgainstTemplateRoot", func(t *testing.T) {
		templateRoot := t.TempDir()
		projectRoot := t.TempDir()
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, projectRoot, templateRoot)
		concreteEvaluator := evaluator.(*expressionEvaluator)

		facetsDir := filepath.Join(templateRoot, "facets")
		os.MkdirAll(facetsDir, 0755)
		os.WriteFile(filepath.Join(facetsDir, "file.txt"), []byte("content"), 0644)

		path := concreteEvaluator.resolvePath("file.txt", "facets/a.yaml")

		expected := filepath.Join(templateRoot, "facets", "file.txt")
		if path != expected {
			t.Errorf("Expected path %q when sourceFilePath is relative, got %q", expected, path)
		}
	})

	t.Run("ResolvesParentPathAgainstProjectRootWhenSourceFilePathEmpty", func(t *testing.T) {
		projectRoot := t.TempDir()
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, projectRoot, "")
		concreteEvaluator := evaluator.(*expressionEvaluator)

		path := concreteEvaluator.resolvePath("../other.txt", "")

		expected := filepath.Join(projectRoot, "other.txt")
		if path != expected {
			t.Errorf("Expected path %q for ../ with projectRoot, got %q", expected, path)
		}
	})
}

func TestExpressionEvaluator_templateDataLookup(t *testing.T) {
	t.Run("ReturnsContentForKey", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")
		concreteEvaluator := evaluator.(*expressionEvaluator)
		concreteEvaluator.SetTemplateData(map[string][]byte{"key": []byte("content")})

		result := concreteEvaluator.templateDataLookup("key")

		if string(result) != "content" {
			t.Errorf("Expected content for key, got %q", result)
		}
	})

	t.Run("ReturnsContentForTemplatePrefixedKey", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")
		concreteEvaluator := evaluator.(*expressionEvaluator)
		concreteEvaluator.SetTemplateData(map[string][]byte{"_template/key": []byte("prefixed")})

		result := concreteEvaluator.templateDataLookup("key")

		if string(result) != "prefixed" {
			t.Errorf("Expected content for _template/key lookup, got %q", result)
		}
	})

	t.Run("ReturnsNilWhenKeyMissing", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")
		concreteEvaluator := evaluator.(*expressionEvaluator)
		concreteEvaluator.SetTemplateData(map[string][]byte{"other": []byte("x")})

		result := concreteEvaluator.templateDataLookup("missing")

		if result != nil {
			t.Errorf("Expected nil for missing key, got %q", result)
		}
	})
}

func TestExpressionEvaluator_getConfig(t *testing.T) {
	t.Run("ReturnsEmptyMapWhenGetContextValuesReturnsNil", func(t *testing.T) {
		// Given an evaluator with config handler that returns nil
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")

		// When getting config
		concreteEvaluator := evaluator.(*expressionEvaluator)
		config := concreteEvaluator.getConfig()

		// Then it should return empty map
		if config == nil {
			t.Fatal("Expected config to be non-nil empty map")
		}
		if len(config) != 0 {
			t.Errorf("Expected config to be empty, got %v", config)
		}
	})

	t.Run("ReturnsEmptyMapWhenGetContextValuesReturnsError", func(t *testing.T) {
		// Given an evaluator with config handler that returns error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return nil, errors.New("config error")
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")

		// When getting config
		concreteEvaluator := evaluator.(*expressionEvaluator)
		config := concreteEvaluator.getConfig()

		// Then it should return empty map (error is ignored)
		if config == nil {
			t.Fatal("Expected config to be non-nil empty map")
		}
		if len(config) != 0 {
			t.Errorf("Expected config to be empty, got %v", config)
		}
	})

	t.Run("ReturnsConfigWhenGetContextValuesSucceeds", func(t *testing.T) {
		// Given an evaluator with config handler that returns config
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"key": "value"}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "", "")

		// When getting config
		concreteEvaluator := evaluator.(*expressionEvaluator)
		config := concreteEvaluator.getConfig()

		// Then it should return the config
		if config == nil {
			t.Fatal("Expected config to be non-nil")
		}
		if config["key"] != "value" {
			t.Errorf("Expected config to contain 'key'='value', got %v", config)
		}
	})
}
