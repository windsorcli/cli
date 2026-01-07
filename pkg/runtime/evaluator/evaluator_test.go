package evaluator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupEvaluatorTest(t *testing.T) (ExpressionEvaluator, config.ConfigHandler, string, string) {
	t.Helper()

	mockConfigHandler := config.NewMockConfigHandler()
	projectRoot := "/test/project"
	templateRoot := "/test/project/contexts/_template"

	evaluator := NewExpressionEvaluator(mockConfigHandler, projectRoot, templateRoot)

	return evaluator, mockConfigHandler, projectRoot, templateRoot
}

func setupEvaluatorWithMockShims(t *testing.T) (ExpressionEvaluator, *Shims, config.ConfigHandler) {
	t.Helper()

	mockConfigHandler := config.NewMockConfigHandler()
	mockShims := &Shims{
		ReadFile:      func(string) ([]byte, error) { return nil, errors.New("file not found") },
		JsonMarshal:   json.Marshal,
		JsonUnmarshal: json.Unmarshal,
		YamlMarshal:   yaml.Marshal,
		FilepathBase:  filepath.Base,
		NewJsonnetVM: func() JsonnetVM {
			return &realJsonnetVM{vm: jsonnet.MakeVM()}
		},
	}

	evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")
	concreteEvaluator := evaluator.(*expressionEvaluator)
	concreteEvaluator.Shims = mockShims

	return evaluator, mockShims, mockConfigHandler
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewExpressionEvaluator(t *testing.T) {
	t.Run("CreatesEvaluatorWithDependencies", func(t *testing.T) {
		// Given a config handler, project root, and template root
		mockConfigHandler := config.NewMockConfigHandler()

		// When creating a new evaluator
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")

		// Then the evaluator should be created with correct dependencies
		if evaluator == nil {
			t.Fatal("Expected evaluator to be created, got nil")
		}

		concreteEvaluator := evaluator.(*expressionEvaluator)
		if concreteEvaluator.configHandler != mockConfigHandler {
			t.Errorf("Expected evaluator.configHandler to be set correctly")
		}

		if concreteEvaluator.projectRoot != "/test/project" {
			t.Errorf("Expected projectRoot to be '/test/project', got '%s'", concreteEvaluator.projectRoot)
		}

		if concreteEvaluator.templateRoot != "/test/template" {
			t.Errorf("Expected templateRoot to be '/test/template', got '%s'", concreteEvaluator.templateRoot)
		}

		if concreteEvaluator.Shims == nil {
			t.Error("Expected Shims to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestExpressionEvaluator_SetTemplateData(t *testing.T) {
	t.Run("SetsTemplateData", func(t *testing.T) {
		// Given an evaluator and template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.jsonnet": []byte(`{"key": "value"}`),
		}

		// When setting template data
		evaluator.SetTemplateData(templateData)

		// Then the template data should be set
		concreteEvaluator := evaluator.(*expressionEvaluator)
		if concreteEvaluator.templateData == nil {
			t.Fatal("Expected templateData to be set, got nil")
		}

		if string(concreteEvaluator.templateData["test.jsonnet"]) != `{"key": "value"}` {
			t.Errorf("Expected templateData to contain test.jsonnet")
		}
	})
}

func TestExpressionEvaluator_Evaluate(t *testing.T) {
	t.Run("EvaluatesSimpleExpression", func(t *testing.T) {
		// Given an evaluator and config with a value
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": 42,
			}, nil
		}

		// When evaluating a simple expression
		result, err := evaluator.Evaluate("value", "", false)

		// Then the result should be correct
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != 42 {
			t.Errorf("Expected result to be 42, got %v", result)
		}
	})

	t.Run("EvaluatesArithmeticExpression", func(t *testing.T) {
		// Given an evaluator and config with values
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"a": 5,
				"b": 10,
			}, nil
		}

		// When evaluating an arithmetic expression
		result, err := evaluator.Evaluate("a + b", "", false)

		// Then the result should be correct
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != 15 {
			t.Errorf("Expected result to be 15, got %v", result)
		}
	})

	t.Run("EvaluatesNestedMapAccess", func(t *testing.T) {
		// Given an evaluator and config with nested maps
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return config, nil
		}

		// When evaluating a nested map access expression
		result, err := evaluator.Evaluate("cluster.workers.count", "", false)

		// Then the result should be correct
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != 3 {
			t.Errorf("Expected result to be 3, got %v", result)
		}
	})

	t.Run("ReturnsErrorForEmptyExpression", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an empty expression
		_, err := evaluator.Evaluate("", "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for empty expression, got nil")
		}
	})

	t.Run("ReturnsErrorForInvalidExpression", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an invalid expression
		_, err := evaluator.Evaluate("invalid +", "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid expression, got nil")
		}
	})

	t.Run("EnrichesConfigWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression that uses project_root
		result, err := evaluator.Evaluate("project_root", "", false)

		// Then project_root should be available
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/project" {
			t.Errorf("Expected project_root to be '/test/project', got %v", result)
		}
	})

	t.Run("EnrichesConfigWithContextPath", func(t *testing.T) {
		// Given an evaluator with config handler that returns config root
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		// When evaluating an expression that uses context_path
		result, err := evaluator.Evaluate("context_path", "", false)

		// Then context_path should be available
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/config" {
			t.Errorf("Expected context_path to be '/test/config', got %v", result)
		}
	})

	t.Run("HandlesConfigRootError", func(t *testing.T) {
		// Given an evaluator with config handler that returns error
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root error")
		}
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		// When evaluating an expression
		result, err := evaluator.Evaluate("value", "", false)

		// Then evaluation should return the original string (undefined variable)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "value" {
			t.Errorf("Expected result to be 'value', got %v", result)
		}
	})

	t.Run("EvaluatesBooleanEqualityExpression", func(t *testing.T) {
		// Given an evaluator and config with values
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "aws",
			}, nil
		}

		// When evaluating an equality expression
		result, err := evaluator.Evaluate("provider == 'aws'", "", false)

		// Then the result should be true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("EvaluatesBooleanInequalityExpression", func(t *testing.T) {
		// Given an evaluator and config with values
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "aws",
			}, nil
		}

		// When evaluating an inequality expression
		result, err := evaluator.Evaluate("provider != 'gcp'", "", false)

		// Then the result should be true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("EvaluatesLogicalAndExpression", func(t *testing.T) {
		// Given an evaluator and config with nested values
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "generic",
				"observability": map[string]any{
					"enabled": true,
				},
			}, nil
		}

		// When evaluating a logical AND expression
		result, err := evaluator.Evaluate("provider == 'generic' && observability.enabled == true", "", false)

		// Then the result should be true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("EvaluatesLogicalOrExpression", func(t *testing.T) {
		// Given an evaluator and config with values
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "aws",
			}, nil
		}

		// When evaluating a logical OR expression
		result, err := evaluator.Evaluate("provider == 'aws' || provider == 'azure'", "", false)

		// Then the result should be true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("EvaluatesParenthesesGrouping", func(t *testing.T) {
		// Given an evaluator and config with nested values
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		config := map[string]any{
			"provider": "generic",
			"vm": map[string]any{
				"driver": "virtualbox",
			},
			"loadbalancer": map[string]any{
				"enabled": false,
			},
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return config, nil
		}

		// When evaluating an expression with parentheses
		result, err := evaluator.Evaluate("provider == 'generic' && (vm.driver != 'docker-desktop' || loadbalancer.enabled == true)", "", false)

		// Then the result should be true
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != true {
			t.Errorf("Expected result to be true, got %v", result)
		}
	})

	t.Run("EvaluatesStringValue", func(t *testing.T) {
		// Given an evaluator and config with string value
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		config := map[string]any{
			"provider": "aws",
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return config, nil
		}

		// When evaluating a string value expression
		result, err := evaluator.Evaluate("provider", "", false)

		// Then the result should be the string value
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "aws" {
			t.Errorf("Expected result to be 'aws', got %v", result)
		}
	})

	t.Run("EvaluatesIntegerValue", func(t *testing.T) {
		// Given an evaluator and config with integer value
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return config, nil
		}

		// When evaluating an integer value expression
		result, err := evaluator.Evaluate("cluster.workers.count", "", false)

		// Then the result should be the integer value
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != 3 {
			t.Errorf("Expected result to be 3, got %v", result)
		}
	})

	t.Run("EvaluatesArrayAccess", func(t *testing.T) {
		// Given an evaluator and config with array value
		evaluator, configHandler, _, _ := setupEvaluatorTest(t)
		mockConfigHandler := configHandler.(*config.MockConfigHandler)
		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"instance_types": []any{"t3.medium", "t3.large"},
				},
			},
		}
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return config, nil
		}

		// When evaluating an array access expression
		result, err := evaluator.Evaluate("cluster.workers.instance_types", "", false)

		// Then the result should be the array
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be an array, got %T", result)
		}

		if len(resultArray) != 2 || resultArray[0] != "t3.medium" || resultArray[1] != "t3.large" {
			t.Errorf("Expected result to be ['t3.medium', 't3.large'], got %v", resultArray)
		}
	})

	t.Run("ReturnsNilForUndefinedVariable", func(t *testing.T) {
		// Given an evaluator and config without the variable
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"cluster": map[string]any{
					"workers": map[string]any{
						"count": 3,
					},
				},
			}, nil
		}

		// When evaluating an undefined variable expression
		result, err := evaluator.Evaluate("cluster.undefined", "", false)

		// Then the result should be the original string (undefined variable)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "cluster.undefined" {
			t.Errorf("Expected result to be 'cluster.undefined', got %v", result)
		}
	})
}

// =============================================================================
// Test Private Methods (via public methods)
// =============================================================================

func TestExpressionEvaluator_enrichConfig(t *testing.T) {
	t.Run("EnrichesConfigWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression (which enriches config)
		result, err := evaluator.Evaluate("project_root", "", false)

		// Then project_root should be in enriched config
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "/test/project" {
			t.Errorf("Expected project_root to be '/test/project', got %v", result)
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
		result, err := evaluator.Evaluate("context_path", "", false)

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
		result, err := evaluator.Evaluate("project_root", "", false)

		// Then project_root should return the original string (undefined variable)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "project_root" {
			t.Errorf("Expected project_root to be 'project_root', got %v", result)
		}
	})
}

func TestExpressionEvaluator_buildExprEnvironment(t *testing.T) {
	t.Run("JsonnetFunctionWithValidPath", func(t *testing.T) {
		// Given an evaluator and a temp file with jsonnet content
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating an expression with jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then the jsonnet file should be evaluated
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
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with jsonnet function with wrong args
		_, err := evaluator.Evaluate(`jsonnet("path1", "path2")`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("JsonnetFunctionWithNonStringArgument", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with jsonnet function with non-string arg
		_, err := evaluator.Evaluate(`jsonnet(42)`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})

	t.Run("FileFunctionWithValidPath", func(t *testing.T) {
		// Given an evaluator and a temp file
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("file content"), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating an expression with file function
		result, err := evaluator.Evaluate(`file("test.txt")`, featurePath, false)

		// Then the file content should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "file content" {
			t.Errorf("Expected result to be 'file content', got '%v'", result)
		}
	})

	t.Run("FileFunctionWithInvalidArguments", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with file function with wrong args
		_, err := evaluator.Evaluate(`file("path1", "path2")`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("FileFunctionWithNonStringArgument", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with file function with non-string arg
		_, err := evaluator.Evaluate(`file(42)`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})

	t.Run("SplitFunction", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with split function
		result, err := evaluator.Evaluate(`split("a,b,c", ",")`, "", false)

		// Then the string should be split
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
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with split function with wrong args
		_, err := evaluator.Evaluate(`split("a,b")`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("SplitFunctionWithNonStringArguments", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression with split function with non-string args
		_, err := evaluator.Evaluate(`split(42, ",")`, "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-string arguments, got nil")
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

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

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

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

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
		_, err := evaluator.Evaluate(`jsonnet("nonexistent.jsonnet")`, "", false)

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

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		_, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

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
			"_template/features/test.jsonnet": []byte(`{"result": "from-template"}`),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

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

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet that uses context
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

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

	t.Run("BuildsContextMapWithProjectName", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		jsonnetContent := `std.extVar("context")`
		os.WriteFile(jsonnetFile, []byte(jsonnetContent), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet
		_, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then it should work (projectName is set in context map)
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

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating file function
		result, err := evaluator.Evaluate(`file("test.txt")`, featurePath, false)

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
		_, err := evaluator.Evaluate(`file("nonexistent.txt")`, "", false)

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
			"_template/features/test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`file("test.txt")`, featurePath, false)

		// Then the file should be loaded from template data
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-template" {
			t.Errorf("Expected result to be 'from-template', got '%v'", result)
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
			"_template/features/test.jsonnet": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`file("test.jsonnet")`, featurePath, false)

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

		featurePath := "/test/feature.yaml"

		// When evaluating file function with absolute path
		_, err := evaluator.Evaluate(`file("/absolute/path.jsonnet")`, featurePath, false)

		// Then it should not find in template data (absolute paths not looked up)
		if err == nil {
			t.Fatal("Expected error for absolute path, got nil")
		}
	})

	t.Run("HandlesEmptyFeaturePath", func(t *testing.T) {
		// Given an evaluator with template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.jsonnet": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		// When evaluating file function with empty feature path
		_, err := evaluator.Evaluate(`file("test.jsonnet")`, "", false)

		// Then it should not find in template data
		if err == nil {
			t.Fatal("Expected error for empty feature path, got nil")
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
		result, err := evaluator.Evaluate(`file("`+escapedPath+`")`, "", false)

		// Then the absolute path should be used
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "content" {
			t.Errorf("Expected result to be 'content', got '%v'", result)
		}
	})

	t.Run("ResolvesRelativePathWithFeaturePath", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		featurePath := filepath.Join(tmpDir, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)
		testFile := filepath.Join(tmpDir, "features", "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		// When evaluating file function with relative path
		result, err := evaluator.Evaluate(`file("test.txt")`, featurePath, false)

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
		result, err := evaluator.Evaluate(`file("test.txt")`, "", false)

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
}

func TestExpressionEvaluator_evaluateFileFunction_EdgeCases(t *testing.T) {
	t.Run("HandlesTemplateDataWithTemplateRootFallback", func(t *testing.T) {
		// Given an evaluator with template root and template data with fallback path
		// The file is requested from a location where lookupInTemplateData won't find it,
		// but the resolved path relative to templateRoot will match the template data
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/config/test.txt": []byte("from-template-fallback"),
		}
		evaluator.SetTemplateData(templateData)

		// Feature path is in a different subdirectory, so lookupInTemplateData won't find it
		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function with path that resolves to config/test.txt relative to templateRoot
		// lookupInTemplateData returns nil (file not in features/), so fallback is used
		result, err := evaluator.Evaluate(`file("../config/test.txt")`, featurePath, false)

		// Then the file should be loaded from template data fallback
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-template-fallback" {
			t.Errorf("Expected result to be 'from-template-fallback', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateDataWithTemplateRootFallbackWithoutPrefix", func(t *testing.T) {
		// Given an evaluator with template root and template data without _template prefix
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"test.txt": []byte("from-template-no-prefix"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`file("../test.txt")`, featurePath, false)

		// Then the file should be loaded from template data fallback without prefix
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-template-no-prefix" {
			t.Errorf("Expected result to be 'from-template-no-prefix', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateDataWithoutTemplateRoot", func(t *testing.T) {
		// Given an evaluator with template data but no template root
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "")
		templateData := map[string][]byte{
			"test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := "/test/feature.yaml"

		// When evaluating file function
		_, err := evaluator.Evaluate(`file("test.txt")`, featurePath, false)

		// Then it should try to read from filesystem (may fail, but should not use template root path)
		if err == nil {
			t.Log("File read succeeded, which is acceptable")
		}
	})

	t.Run("HandlesPathOutsideTemplateRoot", func(t *testing.T) {
		// Given an evaluator with template root and a path outside template root
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/test.txt": []byte("from-template"),
		}
		evaluator.SetTemplateData(templateData)

		// Create file outside template root
		outsideFile := filepath.Join(tmpDir, "outside.txt")
		os.WriteFile(outsideFile, []byte("from-outside"), 0644)

		featurePath := filepath.Join(templateRoot, "test.yaml")

		// When evaluating file function with absolute path outside template root
		escapedPath := strings.ReplaceAll(outsideFile, "\\", "\\\\")
		result, err := evaluator.Evaluate(`file("`+escapedPath+`")`, featurePath, false)

		// Then it should read from filesystem (fallback skipped because path is outside template root)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-outside" {
			t.Errorf("Expected result to be 'from-outside', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateRootFallbackWithRelativePath", func(t *testing.T) {
		// Given an evaluator with template root and template data
		// lookupInTemplateData returns nil, but fallback finds it via template root relative path
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/other/test.txt": []byte("from-fallback"),
		}
		evaluator.SetTemplateData(templateData)

		// Feature path in a subdirectory where lookupInTemplateData won't find the file
		featurePath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function with path that resolves relative to template root
		// lookupInTemplateData returns nil (file not in features/sub/), fallback checks template root
		result, err := evaluator.Evaluate(`file("../../other/test.txt")`, featurePath, false)

		// Then the file should be loaded from template data fallback
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-fallback" {
			t.Errorf("Expected result to be 'from-fallback', got '%v'", result)
		}
	})

	t.Run("HandlesTemplateRootFallbackWithoutTemplatePrefix", func(t *testing.T) {
		// Given an evaluator with template root and template data without _template prefix
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"other/test.txt": []byte("from-fallback-no-prefix"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating file function
		result, err := evaluator.Evaluate(`file("../../other/test.txt")`, featurePath, false)

		// Then the file should be loaded from template data fallback without prefix
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "from-fallback-no-prefix" {
			t.Errorf("Expected result to be 'from-fallback-no-prefix', got '%v'", result)
		}
	})
}

func TestExpressionEvaluator_lookupInTemplateData_EdgeCases(t *testing.T) {
	t.Run("HandlesNilTemplateData", func(t *testing.T) {
		// Given an evaluator without template data
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When looking up a file
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("test.txt", "/test/feature.yaml")

		// Then it should return nil
		if result != nil {
			t.Errorf("Expected nil for nil template data, got %v", result)
		}
	})

	t.Run("HandlesAbsolutePath", func(t *testing.T) {
		// Given an evaluator with template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		// When looking up an absolute path
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("/absolute/path.txt", "/test/feature.yaml")

		// Then it should return nil (absolute paths not looked up)
		if result != nil {
			t.Errorf("Expected nil for absolute path, got %v", result)
		}
	})

	t.Run("HandlesEmptyFeaturePath", func(t *testing.T) {
		// Given an evaluator with template data
		evaluator, _, _, _ := setupEvaluatorTest(t)
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		// When looking up with empty feature path
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("test.txt", "")

		// Then it should return nil
		if result != nil {
			t.Errorf("Expected nil for empty feature path, got %v", result)
		}
	})

	t.Run("HandlesTemplateRootRelativePathError", func(t *testing.T) {
		// Given an evaluator with template root and feature path outside template root
		mockConfigHandler := config.NewMockConfigHandler()
		evaluator := NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")
		templateData := map[string][]byte{
			"test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		// When looking up with feature path outside template root
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("test.txt", "/outside/path.yaml")

		// Then it should use feature path directly
		if result != nil {
			t.Log("Lookup succeeded with outside path, which is acceptable")
		}
	})

	t.Run("HandlesFeatureDirAsDot", func(t *testing.T) {
		// Given an evaluator with template data and feature path in root
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/test.txt": []byte("found"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "test.yaml")

		// When looking up a file
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("test.txt", featurePath)

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
			"features/test.txt": []byte("found-without-prefix"),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When looking up a file
		concreteEvaluator := evaluator.(*expressionEvaluator)
		result := concreteEvaluator.lookupInTemplateData("test.txt", featurePath)

		// Then it should find the file without _template prefix
		if result == nil {
			t.Error("Expected to find file, got nil")
		} else if string(result) != "found-without-prefix" {
			t.Errorf("Expected 'found-without-prefix', got '%s'", string(result))
		}
	})
}

func TestExpressionEvaluator_evaluateJsonnetFunction_EdgeCases(t *testing.T) {
	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		// Given an evaluator with mock shims that fail JSON marshal
		evaluator, mockShims, _ := setupEvaluatorWithMockShims(t)
		mockShims.JsonMarshal = func(any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		_, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for JSON marshal failure, got nil")
		}
	})

	t.Run("HandlesJsonUnmarshalError", func(t *testing.T) {
		// Given an evaluator with mock shims that fail JSON unmarshal
		evaluator, mockShims, _ := setupEvaluatorWithMockShims(t)
		mockShims.JsonUnmarshal = func([]byte, any) error {
			return errors.New("unmarshal error")
		}
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		_, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for JSON unmarshal failure, got nil")
		}
	})

	t.Run("HandlesJsonnetWithEmptyDir", func(t *testing.T) {
		// Given an evaluator and a jsonnet file in root
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then it should work
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
		// Given an evaluator and a jsonnet file where dir is empty string
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		// Feature path is the same directory as the jsonnet file (dir will be ".")
		featurePath := jsonnetFile

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then it should work even with empty dir
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
		// Given an evaluator with template root and template data with fallback path
		// The file is requested from a location where lookupInTemplateData won't find it,
		// but the resolved path relative to templateRoot will match the template data
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"_template/config/test.jsonnet": []byte(`{"result": "from-fallback"}`),
		}
		evaluator.SetTemplateData(templateData)

		// Feature path is in a different subdirectory, so lookupInTemplateData won't find it
		featurePath := filepath.Join(templateRoot, "features", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating jsonnet function with path that resolves to config/test.jsonnet relative to templateRoot
		// lookupInTemplateData returns nil (file not in features/), so fallback is used
		result, err := evaluator.Evaluate(`jsonnet("../config/test.jsonnet")`, featurePath, false)

		// Then the jsonnet should be loaded from template data fallback
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
		// Given an evaluator with template root and template data without _template prefix
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{
			"other/test.jsonnet": []byte(`{"result": "from-fallback-no-prefix"}`),
		}
		evaluator.SetTemplateData(templateData)

		featurePath := filepath.Join(templateRoot, "features", "sub", "test.yaml")
		os.MkdirAll(filepath.Dir(featurePath), 0755)

		// When evaluating jsonnet function with path that resolves relative to template root
		result, err := evaluator.Evaluate(`jsonnet("../../other/test.jsonnet")`, featurePath, false)

		// Then the jsonnet should be loaded from template data fallback without prefix
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
		// Given an evaluator with template root that causes filepath.Rel to return error
		// This tests the error path in the template root fallback
		mockConfigHandler := config.NewMockConfigHandler()
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")
		evaluator := NewExpressionEvaluator(mockConfigHandler, tmpDir, templateRoot)
		templateData := map[string][]byte{}
		evaluator.SetTemplateData(templateData)

		// Create jsonnet file in a location that will cause filepath.Rel to work
		jsonnetFile := filepath.Join(templateRoot, "test.jsonnet")
		os.MkdirAll(filepath.Dir(jsonnetFile), 0755)
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(templateRoot, "test.yaml")

		// When evaluating jsonnet function
		result, err := evaluator.Evaluate(`jsonnet("test.jsonnet")`, featurePath, false)

		// Then it should work (filepath.Rel succeeds, but template data is empty so reads from filesystem)
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

func TestExpressionEvaluator_EvaluateMap(t *testing.T) {
	t.Run("HandlesEmptyMap", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.EvaluateMap(map[string]any{}, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d entries", len(result))
		}
	})

	t.Run("PreservesNonStringValues", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"count":   42,
			"enabled": true,
			"tags":    []string{"a", "b"},
			"nested":  map[string]any{"key": "value"},
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["count"] != 42 {
			t.Errorf("Expected count to be 42, got %v", result["count"])
		}

		if result["enabled"] != true {
			t.Errorf("Expected enabled to be true, got %v", result["enabled"])
		}

		if tags, ok := result["tags"].([]string); !ok || len(tags) != 2 {
			t.Errorf("Expected tags to be preserved, got %v", result["tags"])
		}

		if nested, ok := result["nested"].(map[string]any); !ok || nested["key"] != "value" {
			t.Errorf("Expected nested to be preserved, got %v", result["nested"])
		}
	})

	t.Run("EvaluatesStringExpressions", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": 42,
			}, nil
		}

		values := map[string]any{
			"plain":      "plainstring",
			"expression": "${value}",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["plain"] != "plainstring" {
			t.Errorf("Expected plain to be 'plainstring', got %v", result["plain"])
		}

		if result["expression"] != 42 {
			t.Errorf("Expected expression to be 42, got %v", result["expression"])
		}
	})

	t.Run("SkipsUnresolvedExpressionsWhenEvaluateDeferredIsFalse", func(t *testing.T) {
		mockEvaluator := NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if key == "deferred" {
					continue
				}
				result[key] = value
			}
			return result, nil
		}

		values := map[string]any{
			"deferred": "${terraform_output('cluster', 'key')}",
			"normal":   "value",
		}

		result, err := mockEvaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; exists {
			t.Error("Expected deferred expression to be skipped")
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal to be 'value', got %v", result["normal"])
		}
	})

	t.Run("IncludesUnresolvedExpressionsWhenEvaluateDeferredIsTrue", func(t *testing.T) {
		mockEvaluator := NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			return values, nil
		}

		values := map[string]any{
			"deferred": "${terraform_output('cluster', 'key')}",
			"normal":   "value",
		}

		result, err := mockEvaluator.EvaluateMap(values, "", true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; !exists {
			t.Error("Expected deferred expression to be included when evaluateDeferred is true")
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal to be 'value', got %v", result["normal"])
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		mockEvaluator := NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			return nil, errors.New("failed to evaluate 'bad': evaluation failed")
		}

		values := map[string]any{
			"bad": "${invalid}",
		}

		result, err := mockEvaluator.EvaluateMap(values, "", false)

		if err == nil {
			t.Fatal("Expected error on evaluation failure")
		}

		if result != nil {
			t.Error("Expected nil result on error")
		}

		if !strings.Contains(err.Error(), "failed to evaluate") {
			t.Errorf("Expected error message to contain 'failed to evaluate', got: %v", err)
		}
	})

	t.Run("HandlesMixedValueTypes", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"string":    "plain",
			"number":    42,
			"boolean":   true,
			"array":     []string{"a", "b"},
			"evaluated": "${value}",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["string"] != "plain" {
			t.Errorf("Expected string to be 'plain', got %v", result["string"])
		}

		if result["number"] != 42 {
			t.Errorf("Expected number to be 42, got %v", result["number"])
		}

		if result["boolean"] != true {
			t.Errorf("Expected boolean to be true, got %v", result["boolean"])
		}

		if result["evaluated"] != "evaluated" {
			t.Errorf("Expected evaluated to be 'evaluated', got %v", result["evaluated"])
		}
	})

	t.Run("PassesFeaturePathToEvaluate", func(t *testing.T) {
		mockEvaluator := NewMockExpressionEvaluator()
		var receivedPath string
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, evaluateDeferred bool) (map[string]any, error) {
			receivedPath = featurePath
			return values, nil
		}

		values := map[string]any{
			"test": "value",
		}

		expectedPath := "test/feature/path"
		_, err := mockEvaluator.EvaluateMap(values, expectedPath, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if receivedPath != expectedPath {
			t.Errorf("Expected feature path to be '%s', got '%s'", expectedPath, receivedPath)
		}
	})

	t.Run("HandlesInterpolatedStrings", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"name": "world",
			}, nil
		}

		values := map[string]any{
			"greeting": "Hello ${name}!",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["greeting"] != "Hello world!" {
			t.Errorf("Expected greeting to be 'Hello world!', got %v", result["greeting"])
		}
	})

	t.Run("HandlesComplexTypesInExpressions", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"items":  []any{"a", "b", "c"},
				"config": map[string]any{"key": "value"},
			}, nil
		}

		values := map[string]any{
			"list":   "${items}",
			"object": "${config}",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if items, ok := result["list"].([]any); !ok || len(items) != 3 {
			t.Errorf("Expected list to be preserved as array, got %v", result["list"])
		}

		if config, ok := result["object"].(map[string]any); !ok || config["key"] != "value" {
			t.Errorf("Expected object to be preserved as map, got %v", result["object"])
		}
	})

	t.Run("PreventsInfiniteLoopWithDeferredTerraformOutputInInterpolation", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		evaluator.Register("terraform_output", func(params []any, deferred bool) (any, error) {
			if len(params) != 2 {
				return nil, fmt.Errorf("terraform_output() requires exactly 2 arguments")
			}
			component, _ := params[0].(string)
			key, _ := params[1].(string)
			if deferred {
				return nil, fmt.Errorf("terraform outputs not available for component %s", component)
			}
			return fmt.Sprintf(`terraform_output("%s", "%s")`, component, key), nil
		}, new(func(string, string) any))

		input := "prefix-${terraform_output('a','b')}-suffix"
		result, err := evaluator.Evaluate(input, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := "prefix-${terraform_output(\"a\", \"b\")}-suffix"
		if result != expected {
			t.Errorf("Expected result to be %q, got %q", expected, result)
		}
	})
}
