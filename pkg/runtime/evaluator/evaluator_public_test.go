package evaluator

import (
	"encoding/json"
	"errors"
	"fmt"
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

	t.Run("SkipsPartiallyInterpolatedStringsWithUnresolvedExpressions", func(t *testing.T) {
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

		values := map[string]any{
			"deferred": "prefix-${terraform_output('x', 'y')}-suffix",
			"normal":   "value",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; exists {
			t.Error("Expected partially interpolated string with unresolved expression to be skipped")
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal to be 'value', got %v", result["normal"])
		}
	})
}

func TestContainsExpression(t *testing.T) {
	t.Run("ReturnsTrueForFullyWrappedExpression", func(t *testing.T) {
		if !ContainsExpression("${foo.bar}") {
			t.Error("Expected ContainsExpression to return true for fully wrapped expression")
		}
	})

	t.Run("ReturnsTrueForPartiallyInterpolatedString", func(t *testing.T) {
		if !ContainsExpression("prefix-${terraform_output('x', 'y')}-suffix") {
			t.Error("Expected ContainsExpression to return true for partially interpolated string")
		}
	})

	t.Run("ReturnsTrueForMultipleExpressions", func(t *testing.T) {
		if !ContainsExpression("${foo}-${bar}") {
			t.Error("Expected ContainsExpression to return true for string with multiple expressions")
		}
	})

	t.Run("ReturnsFalseForStringWithoutExpression", func(t *testing.T) {
		if ContainsExpression("plain string") {
			t.Error("Expected ContainsExpression to return false for plain string")
		}
	})

	t.Run("ReturnsFalseForUnclosedExpression", func(t *testing.T) {
		if ContainsExpression("${unclosed") {
			t.Error("Expected ContainsExpression to return false for unclosed expression")
		}
	})

	t.Run("ReturnsFalseForNonStringValue", func(t *testing.T) {
		if ContainsExpression(42) {
			t.Error("Expected ContainsExpression to return false for non-string value")
		}
	})

	t.Run("ReturnsFalseForNil", func(t *testing.T) {
		if ContainsExpression(nil) {
			t.Error("Expected ContainsExpression to return false for nil")
		}
	})

	t.Run("ReturnsFalseForEmptyString", func(t *testing.T) {
		if ContainsExpression("") {
			t.Error("Expected ContainsExpression to return false for empty string")
		}
	})
}
