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

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given nil config handler
		// When creating a new evaluator
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config handler is nil")
			}
		}()
		NewExpressionEvaluator(nil, "/test/project", "/test/template")
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

func TestDeferredError_Error(t *testing.T) {
	t.Run("ReturnsMessageWhenSet", func(t *testing.T) {
		err := &DeferredError{
			Expression: "test",
			Message:    "custom message",
		}

		result := err.Error()

		if result != "custom message" {
			t.Errorf("Expected 'custom message', got '%s'", result)
		}
	})

	t.Run("ReturnsDefaultMessageWhenMessageEmpty", func(t *testing.T) {
		err := &DeferredError{
			Expression: "test_expression",
			Message:    "",
		}

		result := err.Error()

		expected := "deferred expression: test_expression"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
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
		result, err := evaluator.Evaluate("${value}", "", false)

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
		result, err := evaluator.Evaluate("${a + b}", "", false)

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
		result, err := evaluator.Evaluate("${cluster.workers.count}", "", false)

		// Then the result should be correct
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != 3 {
			t.Errorf("Expected result to be 3, got %v", result)
		}
	})

	t.Run("ReturnsEmptyStringAsIs", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an empty string
		result, err := evaluator.Evaluate("", "", false)

		// Then it should be returned as-is
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "" {
			t.Errorf("Expected empty string, got %v", result)
		}
	})

	t.Run("ReturnsErrorForEmptyExpression", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an empty expression
		_, err := evaluator.Evaluate("${}", "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for empty expression, got nil")
		}

		if !strings.Contains(err.Error(), "expression cannot be empty") {
			t.Errorf("Expected error message to contain 'expression cannot be empty', got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidExpression", func(t *testing.T) {
		// Given an evaluator
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an invalid expression
		_, err := evaluator.Evaluate("${invalid +}", "", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid expression, got nil")
		}
	})

	t.Run("EnrichesConfigWithProjectRoot", func(t *testing.T) {
		// Given an evaluator with project root
		evaluator, _, _, _ := setupEvaluatorTest(t)

		// When evaluating an expression that uses project_root
		result, err := evaluator.Evaluate("${project_root}", "", false)

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
		result, err := evaluator.Evaluate("${context_path}", "", false)

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

		// Then evaluation should return the original string (no ${} means no evaluation)
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
		result, err := evaluator.Evaluate("${provider == 'aws'}", "", false)

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
		result, err := evaluator.Evaluate("${provider != 'gcp'}", "", false)

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
		result, err := evaluator.Evaluate("${provider == 'generic' && observability.enabled == true}", "", false)

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
		result, err := evaluator.Evaluate("${provider == 'aws' || provider == 'azure'}", "", false)

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
		result, err := evaluator.Evaluate("${provider == 'generic' && (vm.driver != 'docker-desktop' || loadbalancer.enabled == true)}", "", false)

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
		result, err := evaluator.Evaluate("${provider}", "", false)

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
		result, err := evaluator.Evaluate("${cluster.workers.count}", "", false)

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
		result, err := evaluator.Evaluate("${cluster.workers.instance_types}", "", false)

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
		result, err := evaluator.Evaluate("${cluster.undefined}", "", false)

		// Then the result should be the original string (undefined variable)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "cluster.undefined" {
			t.Errorf("Expected result to be 'cluster.undefined', got %v", result)
		}
	})

	t.Run("DoesNotErrorForMissingNestedProperty", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"addons": map[string]any{},
			}, nil
		}

		result, err := evaluator.Evaluate("${addons.database.enabled}", "", false)

		if err != nil {
			t.Fatalf("Expected no error for missing nested property, got: %v", err)
		}

		if result != "addons.database.enabled" {
			t.Errorf("Expected expression string for missing nested property, got %v", result)
		}
	})

	t.Run("DoesNotErrorForDeeplyNestedMissingProperty", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"config": map[string]any{
					"level1": map[string]any{},
				},
			}, nil
		}

		result, err := evaluator.Evaluate("${config.level1.level2.level3.value}", "", false)

		if err != nil {
			t.Fatalf("Expected no error for deeply nested missing property, got: %v", err)
		}

		if result != "config.level1.level2.level3.value" {
			t.Errorf("Expected expression string for deeply nested missing property, got %v", result)
		}
	})

	t.Run("DoesNotErrorWhenIntermediatePropertyIsMissing", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"services": map[string]any{
					"api": map[string]any{
						"enabled": true,
					},
				},
			}, nil
		}

		result, err := evaluator.Evaluate("${services.database.connection.host}", "", false)

		if err != nil {
			t.Fatalf("Expected no error when intermediate property is missing, got: %v", err)
		}

		if result != "services.database.connection.host" {
			t.Errorf("Expected expression string when intermediate property is missing, got %v", result)
		}
	})

	t.Run("CoalesceOperatorWorksWithMissingNestedProperty", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"settings": map[string]any{},
			}, nil
		}

		result, err := evaluator.Evaluate("${settings.database.port ?? 5432}", "", false)

		if err != nil {
			t.Fatalf("Expected no error with coalesce operator, got: %v", err)
		}

		if result != 5432 {
			t.Errorf("Expected coalesce to return 5432, got %v", result)
		}
	})

	t.Run("CoalesceOperatorReturnsExistingValue", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"settings": map[string]any{
					"database": map[string]any{
						"port": 3306,
					},
				},
			}, nil
		}

		result, err := evaluator.Evaluate("${settings.database.port ?? 5432}", "", false)

		if err != nil {
			t.Fatalf("Expected no error with coalesce operator, got: %v", err)
		}

		if result != 3306 {
			t.Errorf("Expected coalesce to return existing value 3306, got %v", result)
		}
	})

	t.Run("DoesNotErrorWhenRootPropertyIsMissing", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		result, err := evaluator.Evaluate("${nonexistent.nested.property}", "", false)

		if err != nil {
			t.Fatalf("Expected no error when root property is missing, got: %v", err)
		}

		if result != "nonexistent.nested.property" {
			t.Errorf("Expected expression string when root property is missing, got %v", result)
		}
	})

	t.Run("CoalesceOperatorWorksWhenRootPropertyIsMissing", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		result, err := evaluator.Evaluate("${nonexistent.nested.property ?? \"default\"}", "", false)

		if err != nil {
			t.Fatalf("Expected no error with coalesce on missing root property, got: %v", err)
		}

		if result != "default" {
			t.Errorf("Expected coalesce to return 'default', got %v", result)
		}
	})

	t.Run("BooleanExpressionWithMissingPropertyReturnsFalse", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"features": map[string]any{},
			}, nil
		}

		result, err := evaluator.Evaluate("${features.experimental.enabled ?? false}", "", false)

		if err != nil {
			t.Fatalf("Expected no error with boolean coalesce, got: %v", err)
		}

		if result != false {
			t.Errorf("Expected false for missing boolean with coalesce, got %v", result)
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
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, facetPath string, evaluateDeferred bool) (map[string]any, error) {
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
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, facetPath string, evaluateDeferred bool) (map[string]any, error) {
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
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, facetPath string, evaluateDeferred bool) (map[string]any, error) {
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

	t.Run("RecursivelyEvaluatesNestedMaps", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"nested": map[string]any{
				"key": "${value}",
			},
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		nested, ok := result["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested to be a map, got %T", result["nested"])
		}

		if nested["key"] != "evaluated" {
			t.Errorf("Expected nested.key to be 'evaluated', got %v", nested["key"])
		}
	})

	t.Run("RecursivelyEvaluatesArrays", func(t *testing.T) {
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"items": []any{
				"${value}",
				"plain",
			},
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		items, ok := result["items"].([]any)
		if !ok {
			t.Fatalf("Expected items to be an array, got %T", result["items"])
		}

		if len(items) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(items))
		}

		if items[0] != "evaluated" {
			t.Errorf("Expected items[0] to be 'evaluated', got %v", items[0])
		}

		if items[1] != "plain" {
			t.Errorf("Expected items[1] to be 'plain', got %v", items[1])
		}
	})

	t.Run("PreservesDeferredExpressionsInMaps", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"nested": map[string]any{
				"deferred": "${unknown_var}",
				"plain":    "value",
			},
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		nested, ok := result["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested to be a map, got %T", result["nested"])
		}

		if nested["plain"] != "value" {
			t.Errorf("Expected plain to be 'value', got %v", nested["plain"])
		}
	})

	t.Run("PreservesDeferredExpressionsInArrays", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"items": []any{
				"${unknown_var}",
				"plain",
			},
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		items, ok := result["items"].([]any)
		if !ok {
			t.Fatalf("Expected items to be an array, got %T", result["items"])
		}

		if len(items) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(items))
		}

		if items[1] != "plain" {
			t.Errorf("Expected items[1] to be 'plain', got %v", items[1])
		}
	})

	t.Run("HandlesNonDeferredErrorInNestedMap", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"nested": map[string]any{
				"invalid": "${invalid syntax {",
			},
		}

		_, err := evaluator.EvaluateMap(values, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid expression")
		}
	})

	t.Run("HandlesNonDeferredErrorInNestedArray", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"items": []any{
				"${invalid syntax {",
			},
		}

		_, err := evaluator.EvaluateMap(values, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid expression")
		}
	})

	t.Run("HandlesDefaultCaseInEvaluateValue", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"number": 42,
			"bool":   true,
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["number"] != 42 {
			t.Errorf("Expected number to be 42, got %v", result["number"])
		}

		if result["bool"] != true {
			t.Errorf("Expected bool to be true, got %v", result["bool"])
		}
	})

	t.Run("HandlesDeferredErrorInStringEvaluation", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"deferred": "plain_string",
		}

		result, err := evaluator.EvaluateMap(values, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["deferred"] != "plain_string" {
			t.Errorf("Expected plain string to be preserved, got %v", result["deferred"])
		}
	})

	t.Run("HandlesMapWithExpressionInEvaluatedValue", func(t *testing.T) {
		// Given an evaluator with config that returns an expression
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "${another_expr}",
			}, nil
		}

		values := map[string]any{
			"nested": map[string]any{
				"key": "${value}",
			},
		}

		// When evaluating the map
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then it should preserve the original value when evaluated contains expression
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		nested, ok := result["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested to be a map, got %T", result["nested"])
		}

		if nested["key"] != "${value}" {
			t.Errorf("Expected nested.key to preserve original value when evaluated contains expression, got %v", nested["key"])
		}
	})

	t.Run("HandlesArrayWithExpressionInEvaluatedValue", func(t *testing.T) {
		// Given an evaluator with config that returns an expression
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "${another_expr}",
			}, nil
		}

		values := map[string]any{
			"items": []any{
				"${value}",
			},
		}

		// When evaluating the map
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then it should preserve the original value when evaluated contains expression
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		items, ok := result["items"].([]any)
		if !ok {
			t.Fatalf("Expected items to be an array, got %T", result["items"])
		}

		if items[0] != "${value}" {
			t.Errorf("Expected items[0] to preserve original value when evaluated contains expression, got %v", items[0])
		}
	})

	t.Run("HandlesEvaluateDeferredTrueInNestedStructures", func(t *testing.T) {
		// Given an evaluator with config and nested map values
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"nested": map[string]any{
				"key": "${value}",
			},
		}

		// When evaluating with evaluateDeferred=true
		result, err := evaluator.EvaluateMap(values, "", true)

		// Then nested values should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		nested, ok := result["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested to be a map, got %T", result["nested"])
		}

		if nested["key"] != "evaluated" {
			t.Errorf("Expected nested.key to be 'evaluated', got %v", nested["key"])
		}
	})

	t.Run("HandlesEvaluateDeferredTrueWithArray", func(t *testing.T) {
		// Given an evaluator with config and array values
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"items": []any{
				"${value}",
				"plain",
			},
		}

		// When evaluating with evaluateDeferred=true
		result, err := evaluator.EvaluateMap(values, "", true)

		// Then array values should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		items, ok := result["items"].([]any)
		if !ok {
			t.Fatalf("Expected items to be an array, got %T", result["items"])
		}

		if items[0] != "evaluated" {
			t.Errorf("Expected items[0] to be 'evaluated', got %v", items[0])
		}
	})

	t.Run("HandlesStringEvaluationErrorWithEvaluateDeferredTrue", func(t *testing.T) {
		// Given an evaluator and invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"invalid": "${invalid syntax {",
		}

		// When evaluating with evaluateDeferred=true
		_, err := evaluator.EvaluateMap(values, "", true)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression when evaluateDeferred is true")
		}
	})

	t.Run("HandlesMapEvaluationErrorWithEvaluateDeferredTrue", func(t *testing.T) {
		// Given an evaluator and nested map with invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"nested": map[string]any{
				"invalid": "${invalid syntax {",
			},
		}

		// When evaluating with evaluateDeferred=true
		_, err := evaluator.EvaluateMap(values, "", true)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression in nested map when evaluateDeferred is true")
		}
	})

	t.Run("HandlesArrayEvaluationErrorWithEvaluateDeferredTrue", func(t *testing.T) {
		// Given an evaluator and array with invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"items": []any{
				"${invalid syntax {",
			},
		}

		// When evaluating with evaluateDeferred=true
		_, err := evaluator.EvaluateMap(values, "", true)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression in array when evaluateDeferred is true")
		}
	})

	t.Run("HandlesStringWithEvaluateDeferredTrue", func(t *testing.T) {
		// Given an evaluator with config
		evaluator, mockConfigHandler, _, _ := setupEvaluatorTest(t)
		mockHandler := mockConfigHandler.(*config.MockConfigHandler)
		mockHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}

		values := map[string]any{
			"key": "${value}",
		}

		// When evaluating with evaluateDeferred=true
		result, err := evaluator.EvaluateMap(values, "", true)

		// Then the value should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["key"] != "evaluated" {
			t.Errorf("Expected key to be 'evaluated', got %v", result["key"])
		}
	})

	t.Run("HandlesNonDeferredErrorInStringWithEvaluateDeferredFalse", func(t *testing.T) {
		// Given an evaluator and invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"invalid": "${invalid syntax {",
		}

		// When evaluating with evaluateDeferred=false
		_, err := evaluator.EvaluateMap(values, "", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression when evaluateDeferred is false")
		}
	})

	t.Run("HandlesNonDeferredErrorInMapWithEvaluateDeferredFalse", func(t *testing.T) {
		// Given an evaluator and nested map with invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"nested": map[string]any{
				"invalid": "${invalid syntax {",
			},
		}

		// When evaluating with evaluateDeferred=false
		_, err := evaluator.EvaluateMap(values, "", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression in nested map when evaluateDeferred is false")
		}
	})

	t.Run("HandlesNonDeferredErrorInArrayWithEvaluateDeferredFalse", func(t *testing.T) {
		// Given an evaluator and array with invalid expression
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"items": []any{
				"${invalid syntax {",
			},
		}

		// When evaluating with evaluateDeferred=false
		_, err := evaluator.EvaluateMap(values, "", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for invalid expression in array when evaluateDeferred is false")
		}
	})

	t.Run("PassesFacetPathToEvaluate", func(t *testing.T) {
		// Given a mock evaluator that captures feature path
		mockEvaluator := NewMockExpressionEvaluator()
		var receivedPath string
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, facetPath string, evaluateDeferred bool) (map[string]any, error) {
			receivedPath = facetPath
			return values, nil
		}

		values := map[string]any{
			"test": "value",
		}

		expectedPath := "test/feature/path"

		// When evaluating with a feature path
		_, err := mockEvaluator.EvaluateMap(values, expectedPath, false)

		// Then the feature path should be passed correctly
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if receivedPath != expectedPath {
			t.Errorf("Expected feature path to be '%s', got '%s'", expectedPath, receivedPath)
		}
	})

	t.Run("HandlesInterpolatedStrings", func(t *testing.T) {
		// Given an evaluator with config
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

		// When evaluating interpolated strings
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then the interpolation should work
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["greeting"] != "Hello world!" {
			t.Errorf("Expected greeting to be 'Hello world!', got %v", result["greeting"])
		}
	})

	t.Run("HandlesComplexTypesInExpressions", func(t *testing.T) {
		// Given an evaluator with config containing complex types
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

		// When evaluating expressions with complex types
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then complex types should be evaluated correctly
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
		// Given an evaluator with terraform_output helper that returns DeferredError
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
			return nil, &DeferredError{
				Expression: fmt.Sprintf(`terraform_output("%s", "%s")`, component, key),
				Message:    fmt.Sprintf("terraform output '%s' for component %s is deferred", key, component),
			}
		}, new(func(string, string) any))

		input := "prefix-${terraform_output('a','b')}-suffix"

		// When evaluating with deferred expression
		result, err := evaluator.Evaluate(input, "", false)

		// Then the original input should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := input
		if result != expected {
			t.Errorf("Expected result to be preserved as original %q, got %q", expected, result)
		}
	})

	t.Run("SkipsPartiallyInterpolatedStringsWithUnresolvedExpressions", func(t *testing.T) {
		// Given an evaluator with terraform_output helper and mixed values
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
			return nil, &DeferredError{
				Expression: fmt.Sprintf(`terraform_output("%s", "%s")`, component, key),
				Message:    fmt.Sprintf("terraform output '%s' for component %s is deferred", key, component),
			}
		}, new(func(string, string) any))

		values := map[string]any{
			"deferred": "prefix-${terraform_output('x', 'y')}-suffix",
			"normal":   "value",
		}

		// When evaluating the map
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then deferred values should be preserved and normal values evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["deferred"]; !exists {
			t.Error("Expected deferred value to be preserved in result")
		}
		if result["deferred"] != values["deferred"] {
			t.Errorf("Expected deferred to be preserved as original value, got %v", result["deferred"])
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal to be 'value', got %v", result["normal"])
		}
	})

	t.Run("HandlesEmptyStringValues", func(t *testing.T) {
		// Given an evaluator and values with empty strings
		evaluator, _, _, _ := setupEvaluatorTest(t)

		values := map[string]any{
			"empty":    "",
			"nonEmpty": "value",
		}

		// When evaluating the map
		result, err := evaluator.EvaluateMap(values, "", false)

		// Then empty strings should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["empty"] != "" {
			t.Errorf("Expected empty to be empty string, got %v", result["empty"])
		}

		if result["nonEmpty"] != "value" {
			t.Errorf("Expected nonEmpty to be 'value', got %v", result["nonEmpty"])
		}
	})

}

func TestContainsExpression(t *testing.T) {
	t.Run("ReturnsTrueForFullyWrappedExpression", func(t *testing.T) {
		// Given a fully wrapped expression
		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression("${foo.bar}") {
			t.Error("Expected ContainsExpression to return true for fully wrapped expression")
		}
	})

	t.Run("ReturnsTrueForPartiallyInterpolatedString", func(t *testing.T) {
		// Given a partially interpolated string
		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression("prefix-${terraform_output('x', 'y')}-suffix") {
			t.Error("Expected ContainsExpression to return true for partially interpolated string")
		}
	})

	t.Run("ReturnsTrueForMultipleExpressions", func(t *testing.T) {
		// Given a string with multiple expressions
		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression("${foo}-${bar}") {
			t.Error("Expected ContainsExpression to return true for string with multiple expressions")
		}
	})

	t.Run("ReturnsFalseForStringWithoutExpression", func(t *testing.T) {
		// Given a plain string without expression
		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression("plain string") {
			t.Error("Expected ContainsExpression to return false for plain string")
		}
	})

	t.Run("ReturnsFalseForUnclosedExpression", func(t *testing.T) {
		// Given a string with unclosed expression
		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression("${unclosed") {
			t.Error("Expected ContainsExpression to return false for unclosed expression")
		}
	})

	t.Run("ReturnsFalseForNonStringValue", func(t *testing.T) {
		// Given a non-string value
		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression(42) {
			t.Error("Expected ContainsExpression to return false for non-string value")
		}
	})

	t.Run("ReturnsFalseForNil", func(t *testing.T) {
		// Given a nil value
		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression(nil) {
			t.Error("Expected ContainsExpression to return false for nil")
		}
	})

	t.Run("ReturnsFalseForEmptyString", func(t *testing.T) {
		// Given an empty string
		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression("") {
			t.Error("Expected ContainsExpression to return false for empty string")
		}
	})

	t.Run("ReturnsTrueForMapWithExpression", func(t *testing.T) {
		// Given a map with an expression value
		value := map[string]any{
			"key": "${expression}",
		}

		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return true for map with expression")
		}
	})

	t.Run("ReturnsFalseForMapWithoutExpression", func(t *testing.T) {
		// Given a map without expression values
		value := map[string]any{
			"key": "plain value",
		}

		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return false for map without expression")
		}
	})

	t.Run("ReturnsTrueForNestedMapWithExpression", func(t *testing.T) {
		// Given a nested map with an expression
		value := map[string]any{
			"outer": map[string]any{
				"inner": "${expression}",
			},
		}

		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return true for nested map with expression")
		}
	})

	t.Run("ReturnsTrueForArrayWithExpression", func(t *testing.T) {
		// Given an array with an expression
		value := []any{"plain", "${expression}"}

		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return true for array with expression")
		}
	})

	t.Run("ReturnsFalseForArrayWithoutExpression", func(t *testing.T) {
		// Given an array without expressions
		value := []any{"plain", "value"}

		// When checking if it contains an expression
		// Then it should return false
		if ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return false for array without expression")
		}
	})

	t.Run("ReturnsTrueForNestedArrayWithExpression", func(t *testing.T) {
		// Given a nested array with an expression
		value := []any{[]any{"${expression}"}}

		// When checking if it contains an expression
		// Then it should return true
		if !ContainsExpression(value) {
			t.Error("Expected ContainsExpression to return true for nested array with expression")
		}
	})
}
