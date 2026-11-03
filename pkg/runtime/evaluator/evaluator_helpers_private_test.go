package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestYamlHelper(t *testing.T) {
	t.Run("YamlFunctionWithFilePath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		yamlContent := "key: value\nfoo: bar"
		yamlFile := filepath.Join(tmpDir, "data.yaml")
		os.WriteFile(yamlFile, []byte(yamlContent), 0644)

		facetPath := filepath.Join(tmpDir, "facet.yaml")

		result, err := evaluator.Evaluate(`${yaml("data.yaml")}`, facetPath, nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if m["key"] != "value" || m["foo"] != "bar" {
			t.Errorf("Expected map key:value, foo:bar, got %v", m)
		}
	})

	t.Run("YamlFunctionWithInlineString", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${yaml("a: 1\nb: 2")}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if len(m) != 2 {
			t.Errorf("Expected map with 2 entries, got %v", m)
		}
		numEq := func(v any, want int) bool {
			switch n := v.(type) {
			case int:
				return n == want
			case uint64:
				return n == uint64(want)
			case float64:
				return n == float64(want)
			default:
				return false
			}
		}
		if !numEq(m["a"], 1) || !numEq(m["b"], 2) {
			t.Errorf("Expected map a:1, b:2, got %v", m)
		}
	})

	t.Run("YamlFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml("a", "b", "c")}`, "", nil, false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("YamlFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml(42)}`, "", nil, false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})

	t.Run("YamlFunctionWithInvalidYaml", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml("not: valid: yaml: [")}`, "", nil, false)

		if err == nil {
			t.Fatal("Expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "yaml()") {
			t.Errorf("Expected error to mention yaml(), got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Message, "cannot find the file") || we.Remediation != filePathRemediation {
			t.Errorf("Expected a file-not-found error with the file path remediation, got %v", we)
		}
	})

	t.Run("YamlFunctionWithInvalidInlineYamlNamesTheParseError", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml("a:\n  b: [")}`, "", nil, false)

		if err == nil {
			t.Fatal("Expected error for invalid YAML, got nil")
		}
		we := werror.Find(err)
		if we == nil || !strings.Contains(we.Message, "The yaml() helper cannot parse the YAML in its inline input") {
			t.Errorf("Expected a parse error for the inline input, got %v", err)
		}
		if we != nil && we.Remediation == "" {
			t.Error("Expected a remediation for the parse error")
		}
	})

	t.Run("YamlFunctionWithSingleLineInline", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${yaml("p: q")}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if m["p"] != "q" {
			t.Errorf("Expected map p:q, got %v", m)
		}
	})

	t.Run("YamlFunctionReturnsArray", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${yaml("- a\n- b\n- c")}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		arr, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be slice, got %T", result)
		}
		if len(arr) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(arr))
		}
		if arr[0] != "a" || arr[1] != "b" || arr[2] != "c" {
			t.Errorf("Expected [a b c], got %v", arr)
		}
	})

	t.Run("YamlFunctionWithTemplateData", func(t *testing.T) {
		tmpDir := t.TempDir()
		projectRoot := filepath.Join(tmpDir, "project")
		templateRoot := filepath.Join(tmpDir, "project", "contexts", "_template")
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, projectRoot, templateRoot)
		templateData := map[string][]byte{
			"_template/facets/data.yaml": []byte("from: template\ncount: 1"),
		}
		evaluator.SetTemplateData(templateData)
		facetPath := filepath.Join(templateRoot, "facets", "test.yaml")

		result, err := evaluator.Evaluate(`${yaml("data.yaml")}`, facetPath, nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if m["from"] != "template" {
			t.Errorf("Expected from: template, got %v", m["from"])
		}
	})

	t.Run("YamlFunctionWithInputTemplatesContent", func(t *testing.T) {
		template := "key: ${input.foo}\nbar: ${input.nested.baz}"
		input := map[string]any{
			"foo":    "value-from-input",
			"nested": map[string]any{"baz": "qux"},
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"template": template, "input": input}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/project", "/template")

		result, err := evaluator.Evaluate(`${yaml(template, input)}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if m["key"] != "value-from-input" {
			t.Errorf("Expected key: value-from-input, got %v", m["key"])
		}
		if m["bar"] != "qux" {
			t.Errorf("Expected bar: qux, got %v", m["bar"])
		}
	})

	t.Run("YamlFunctionWithInputAndContext", func(t *testing.T) {
		input := map[string]any{"patchVar": "from-patch-vars"}
		templateStr := "provider: ${provider}\nclusterName: ${cluster.name}\npatchVar: ${input.patchVar}"
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "aws",
				"cluster":  map[string]any{"name": "my-cluster"},
				"input":    input,
				"template": templateStr,
			}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/project", "/template")

		result, err := evaluator.Evaluate(`${yaml(template, input)}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be map, got %T", result)
		}
		if m["provider"] != "aws" {
			t.Errorf("Expected provider: aws, got %v", m["provider"])
		}
		if m["clusterName"] != "my-cluster" {
			t.Errorf("Expected clusterName: my-cluster, got %v", m["clusterName"])
		}
		if m["patchVar"] != "from-patch-vars" {
			t.Errorf("Expected patchVar: from-patch-vars, got %v", m["patchVar"])
		}
	})

	t.Run("YamlFunctionSingleElementListAllowsMemberAccess", func(t *testing.T) {
		inlineYAML := "- controlplane_labels: foo\n  other: bar"
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"nodesYaml": inlineYAML}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/project", "/template")

		result, err := evaluator.Evaluate(`${yaml(nodesYaml).controlplane_labels}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "foo" {
			t.Errorf("Expected controlplane_labels to be 'foo', got %v", result)
		}
	})

	t.Run("YamlFunctionMapRootAllowsControlplanesMemberAccess", func(t *testing.T) {
		inlineYAML := "controlplanes:\n  - hostname: cp1\nworkers:\n  - hostname: w1\n"
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"nodesYaml": inlineYAML}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/project", "/template")

		result, err := evaluator.Evaluate(`${yaml(nodesYaml).controlplanes}`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error (avoids int(string) on .controlplanes), got: %v", err)
		}
		slice, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected controlplanes to be []any, got %T", result)
		}
		if len(slice) != 1 {
			t.Errorf("Expected controlplanes to have 1 element, got %d", len(slice))
		}
		m, ok := slice[0].(map[string]any)
		if !ok {
			t.Fatalf("Expected first element to be map[string]any, got %T", slice[0])
		}
		if m["hostname"] != "cp1" {
			t.Errorf("Expected hostname cp1, got %v", m["hostname"])
		}
	})
}
