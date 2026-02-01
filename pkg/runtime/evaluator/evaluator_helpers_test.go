package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestJsonnetHelper(t *testing.T) {
	t.Run("JsonnetFunctionWithValidPath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		facetPath := filepath.Join(tmpDir, "facet.yaml")

		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, facetPath, false)

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

	t.Run("JsonnetFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${jsonnet("path1", "path2")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("JsonnetFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${jsonnet(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})
}

func TestFileHelper(t *testing.T) {
	t.Run("FileFunctionWithValidPath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("file content"), 0644)

		facetPath := filepath.Join(tmpDir, "facet.yaml")

		result, err := evaluator.Evaluate(`${file("test.txt")}`, facetPath, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "file content" {
			t.Errorf("Expected result to be 'file content', got '%v'", result)
		}
	})

	t.Run("FileFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${file("path1", "path2")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("FileFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${file(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})
}

func TestYamlHelper(t *testing.T) {
	t.Run("YamlFunctionWithFilePath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		yamlContent := "key: value\nfoo: bar"
		yamlFile := filepath.Join(tmpDir, "data.yaml")
		os.WriteFile(yamlFile, []byte(yamlContent), 0644)

		facetPath := filepath.Join(tmpDir, "facet.yaml")

		result, err := evaluator.Evaluate(`${yaml("data.yaml")}`, facetPath, false)

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

		result, err := evaluator.Evaluate(`${yaml("a: 1\nb: 2")}`, "", false)

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

		_, err := evaluator.Evaluate(`${yaml("a", "b", "c")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("YamlFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})

	t.Run("YamlFunctionWithInvalidYaml", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yaml("not: valid: yaml: [")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid YAML, got nil")
		}
		if !strings.Contains(err.Error(), "yaml()") {
			t.Errorf("Expected error to mention yaml(), got: %v", err)
		}
	})

	t.Run("YamlFunctionWithSingleLineInline", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${yaml("p: q")}`, "", false)

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

		result, err := evaluator.Evaluate(`${yaml("- a\n- b\n- c")}`, "", false)

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

		result, err := evaluator.Evaluate(`${yaml("data.yaml")}`, facetPath, false)

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

		result, err := evaluator.Evaluate(`${yaml(template, input)}`, "", false)

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

		result, err := evaluator.Evaluate(`${yaml(template, input)}`, "", false)

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

		result, err := evaluator.Evaluate(`${yaml(nodesYaml).controlplane_labels}`, "", false)

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

		result, err := evaluator.Evaluate(`${yaml(nodesYaml).controlplanes}`, "", false)

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

func TestYamlStringHelper(t *testing.T) {
	t.Run("YamlStringWithOneArgMarshalsToYAMLString", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"obj": map[string]any{"a": 1, "b": "two"}}, nil
		}
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/project", "/template")

		result, err := evaluator.Evaluate(`${yamlString(obj)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		str, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string result, got %T", result)
		}
		if !strings.Contains(str, "a: 1") || !strings.Contains(str, "b: two") {
			t.Errorf("Expected YAML string with a and b, got %q", str)
		}
	})

	t.Run("YamlStringWithInvalidArgumentCount", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${yamlString()}`, "", false)
		if err == nil {
			t.Fatal("Expected error for zero arguments")
		}
		_, err = evaluator.Evaluate(`${yamlString("a", "b", "c")}`, "", false)
		if err == nil {
			t.Fatal("Expected error for three arguments")
		}
	})
}

func TestSplitHelper(t *testing.T) {
	t.Run("SplitFunction", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${split("a,b,c", ",")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be an array, got %T", result)
		}

		if len(resultArray) != 3 {
			t.Errorf("Expected result to have 3 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "a" || resultArray[1] != "b" || resultArray[2] != "c" {
			t.Errorf("Expected result to be ['a', 'b', 'c'], got %v", resultArray)
		}
	})

	t.Run("SplitFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${split("a,b")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("SplitFunctionWithNonStringArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${split(42, ",")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string arguments, got nil")
		}
	})
}

func TestCidrHostHelper(t *testing.T) {
	t.Run("CidrHostWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", 10)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.0.10" {
			t.Errorf("Expected result to be '10.5.0.10', got '%v'", result)
		}
	})

	t.Run("CidrHostWithHostnumZero", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("192.168.1.0/24", 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "192.168.1.0" {
			t.Errorf("Expected result to be '192.168.1.0', got '%v'", result)
		}
	})

	t.Run("CidrHostWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrHostWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost(42, 10)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrHostWithNonNumberHostnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", "10")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number hostnum, got nil")
		}
	})

	t.Run("CidrHostWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("invalid", 10)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrHostWithOutOfRangeHostnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", 65536)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for out of range hostnum, got nil")
		}
	})

	t.Run("CidrHostWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("2001:db8::/96", 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8::") {
			t.Errorf("Expected result to contain '2001:db8::', got '%v'", result)
		}
	})

	t.Run("CidrHostWithIPv6Slash64", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("2001:db8::/64", 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error for /64 network with hostnum 0, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8::") {
			t.Errorf("Expected result to contain '2001:db8::', got '%v'", result)
		}
	})

	t.Run("CidrHostWithIPv6OutOfRange", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("2001:db8::/128", 2)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 out of range hostnum, got nil")
		}
	})

	t.Run("CidrHostWithHostnumExceedingMaxUint32", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.0.0.0/8", 4294967296)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for hostnum exceeding MaxUint32, got nil")
		}
		if !strings.Contains(err.Error(), "out of range for IPv4 address") {
			t.Errorf("Expected error message about out of range, got: %v", err)
		}
	})

	t.Run("CidrHostWithOverflowDetection", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("255.255.255.0/24", 256)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for overflow, got nil")
		}
		if !strings.Contains(err.Error(), "causes overflow") {
			t.Errorf("Expected error message about overflow, got: %v", err)
		}
	})
}

func TestCidrSubnetHelper(t *testing.T) {
	t.Run("CidrSubnetWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8, 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.1.0/24" {
			t.Errorf("Expected result to be '10.5.1.0/24', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithNetnumZero", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.0.0.0/8", 8, 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.0.0.0/16" {
			t.Errorf("Expected result to be '10.0.0.0/16', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrSubnetWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet(42, 8, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrSubnetWithNonNumberNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", "8", 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number newbits, got nil")
		}
	})

	t.Run("CidrSubnetWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("invalid", 8, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrSubnetWithInvalidNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 17, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid newbits, got nil")
		}
	})

	t.Run("CidrSubnetWithOutOfRangeNetnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8, 256)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for out of range netnum, got nil")
		}
	})

	t.Run("CidrSubnetWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("2001:db8::/64", 16, 256)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8") {
			t.Errorf("Expected result to contain '2001:db8', got '%v'", result)
		}
		if !strings.Contains(result.(string), "/80") {
			t.Errorf("Expected result to contain '/80', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithLargeNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("2001:db8::/32", 64, 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error for /32 network with newbits=64 and netnum=0, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8") {
			t.Errorf("Expected result to contain '2001:db8', got '%v'", result)
		}
		if !strings.Contains(result.(string), "/96") {
			t.Errorf("Expected result to contain '/96', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithNonCanonicalCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.5.3.7/16", 8, 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.1.0/24" {
			t.Errorf("Expected result to be '10.5.1.0/24' (using network base address), got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithOffsetExceedingMaxUint32", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("0.0.0.0/0", 0, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for offset exceeding MaxUint32, got nil")
		}
		if !strings.Contains(err.Error(), "out of range for IPv4 address") {
			t.Errorf("Expected error message about out of range, got: %v", err)
		}
	})

	t.Run("CidrSubnetWithOverflowDetection", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("255.255.255.0/24", 0, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for overflow, got nil")
		}
		if !strings.Contains(err.Error(), "causes overflow") {
			t.Errorf("Expected error message about overflow, got: %v", err)
		}
	})

	t.Run("CidrSubnetWithIPv6OffsetOverflow", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ff00/120", 0, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 offset overflow, got nil")
		}
		if !strings.Contains(err.Error(), "too large") {
			t.Errorf("Expected error message about offset being too large, got: %v", err)
		}
	})
}

func TestCidrSubnetsHelper(t *testing.T) {
	t.Run("CidrSubnetsWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16", 8, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.5.0.0/24" {
			t.Errorf("Expected result[0] to be '10.5.0.0/24', got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.5.1.0/24" {
			t.Errorf("Expected result[1] to be '10.5.1.0/24', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithSingleSubnet", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.0.0.0/8", 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 1 {
			t.Fatalf("Expected result to have 1 element, got %d", len(resultArray))
		}

		if resultArray[0] != "10.0.0.0/16" {
			t.Errorf("Expected result[0] to be '10.0.0.0/16', got '%v'", resultArray[0])
		}
	})

	t.Run("CidrSubnetsWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrSubnetsWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets(42, 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrSubnetsWithNonNumberNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16", "8")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number newbits, got nil")
		}
	})

	t.Run("CidrSubnetsWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("invalid", 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrSubnetsWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("2001:db8::/64", 16, 16)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if !strings.Contains(resultArray[0].(string), "2001:db8") {
			t.Errorf("Expected result[0] to contain '2001:db8', got '%v'", resultArray[0])
		}
		if !strings.Contains(resultArray[1].(string), "2001:db8") {
			t.Errorf("Expected result[1] to contain '2001:db8', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithDifferentSizes", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.0.0.0/8", 4, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.0.0.0/12" {
			t.Errorf("Expected result[0] to be '10.0.0.0/12', got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.16.0.0/16" {
			t.Errorf("Expected result[1] to be '10.16.0.0/16', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithNonCanonicalCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.5.3.7/16", 8, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.5.0.0/24" {
			t.Errorf("Expected result[0] to be '10.5.0.0/24' (using network base address), got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.5.1.0/24" {
			t.Errorf("Expected result[1] to be '10.5.1.0/24' (using network base address), got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithIPv6OffsetOverflow", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ff00/120", 0, 0)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 offset overflow, got nil")
		}
		if !strings.Contains(err.Error(), "too large") {
			t.Errorf("Expected error message about offset being too large, got: %v", err)
		}
	})
}

func TestCidrNetmaskHelper(t *testing.T) {
	t.Run("CidrNetmaskWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("10.5.0.0/16")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.255.0.0" {
			t.Errorf("Expected result to be '255.255.0.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithSlash24", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("192.168.1.0/24")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.255.255.0" {
			t.Errorf("Expected result to be '255.255.255.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithSlash8", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("10.0.0.0/8")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.0.0.0" {
			t.Errorf("Expected result to be '255.0.0.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("10.5.0.0/16", "extra")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrNetmaskWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrNetmaskWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("invalid")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrNetmaskWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("2001:db8::/32")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 CIDR, got nil")
		}
		if !strings.Contains(err.Error(), "IPv4") {
			t.Errorf("Expected error to mention 'IPv4', got: %v", err)
		}
	})
}
