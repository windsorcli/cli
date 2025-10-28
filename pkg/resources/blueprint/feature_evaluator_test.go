package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewFeatureEvaluator(t *testing.T) {
	t.Run("CreatesNewFeatureEvaluatorSuccessfully", func(t *testing.T) {
		injector := di.NewInjector()
		evaluator := NewFeatureEvaluator(injector)
		if evaluator == nil {
			t.Fatal("Expected evaluator, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFeatureEvaluator_EvaluateExpression(t *testing.T) {
	injector := di.NewInjector()
	evaluator := NewFeatureEvaluator(injector)
	_ = evaluator.Initialize()

	tests := []struct {
		name        string
		expression  string
		config      map[string]any
		expected    bool
		shouldError bool
	}{
		{
			name:        "EmptyExpressionFails",
			expression:  "",
			config:      map[string]any{},
			shouldError: true,
		},
		{
			name:       "SimpleEqualityExpressionTrue",
			expression: "provider == 'aws'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "SimpleEqualityExpressionFalse",
			expression: "provider == 'aws'",
			config:     map[string]any{"provider": "local"},
			expected:   false,
		},
		{
			name:       "SimpleInequalityExpressionTrue",
			expression: "provider != 'generic'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "SimpleInequalityExpressionFalse",
			expression: "provider != 'generic'",
			config:     map[string]any{"provider": "generic"},
			expected:   false,
		},
		{
			name:       "LogicalAndExpressionTrue",
			expression: "provider == 'generic' && observability.enabled == true",
			config: map[string]any{
				"provider": "generic",
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:       "LogicalAndExpressionFalse",
			expression: "provider == 'generic' && observability.enabled == true",
			config: map[string]any{
				"provider": "aws",
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: false,
		},
		{
			name:       "LogicalOrExpressionTrue",
			expression: "provider == 'aws' || provider == 'azure'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "LogicalOrExpressionFalse",
			expression: "provider == 'aws' || provider == 'azure'",
			config:     map[string]any{"provider": "generic"},
			expected:   false,
		},
		{
			name:       "ParenthesesGrouping",
			expression: "provider == 'generic' && (vm.driver != 'docker-desktop' || loadbalancer.enabled == true)",
			config: map[string]any{
				"provider": "generic",
				"vm": map[string]any{
					"driver": "virtualbox",
				},
				"loadbalancer": map[string]any{
					"enabled": false,
				},
			},
			expected: true,
		},
		{
			name:       "NestedObjectAccess",
			expression: "observability.enabled == true && observability.backend == 'quickwit'",
			config: map[string]any{
				"observability": map[string]any{
					"enabled": true,
					"backend": "quickwit",
				},
			},
			expected: true,
		},
		{
			name:       "BooleanComparison",
			expression: "dns.enabled == true",
			config: map[string]any{
				"dns": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:        "InvalidSyntaxFails",
			expression:  "provider ===",
			config:      map[string]any{"provider": "aws"},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateExpression(tt.expression, tt.config, "features/test.yaml")

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for expression '%s', got none", tt.expression)
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error for expression '%s', got %v", tt.expression, err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %v for expression '%s' with config %v, got %v",
					tt.expected, tt.expression, tt.config, result)
			}
		})
	}
}

func TestFeatureEvaluator_EvaluateValue(t *testing.T) {
	injector := di.NewInjector()
	evaluator := NewFeatureEvaluator(injector)
	_ = evaluator.Initialize()

	tests := []struct {
		name        string
		expression  string
		config      map[string]any
		expected    any
		shouldError bool
	}{
		{
			name:        "EmptyExpressionFails",
			expression:  "",
			config:      map[string]any{},
			shouldError: true,
		},
		{
			name:       "StringValue",
			expression: "provider",
			config:     map[string]any{"provider": "aws"},
			expected:   "aws",
		},
		{
			name:       "IntegerValue",
			expression: "cluster.workers.count",
			config: map[string]any{
				"cluster": map[string]any{
					"workers": map[string]any{
						"count": 3,
					},
				},
			},
			expected: 3,
		},
		{
			name:       "ArithmeticExpression",
			expression: "cluster.workers.count + 2",
			config: map[string]any{
				"cluster": map[string]any{
					"workers": map[string]any{
						"count": 3,
					},
				},
			},
			expected: 5,
		},
		{
			name:       "NestedMapAccess",
			expression: "cluster.controlplanes.nodes",
			config: map[string]any{
				"cluster": map[string]any{
					"controlplanes": map[string]any{
						"nodes": map[string]any{
							"node1": "value1",
						},
					},
				},
			},
			expected: map[string]any{"node1": "value1"},
		},
		{
			name:       "ArrayAccess",
			expression: "cluster.workers.instance_types",
			config: map[string]any{
				"cluster": map[string]any{
					"workers": map[string]any{
						"instance_types": []any{"t3.medium", "t3.large"},
					},
				},
			},
			expected: []any{"t3.medium", "t3.large"},
		},
		{
			name:       "UndefinedVariableReturnsNil",
			expression: "cluster.undefined",
			config: map[string]any{
				"cluster": map[string]any{
					"workers": map[string]any{
						"count": 3,
					},
				},
			},
			expected: nil,
		},
		{
			name:        "InvalidExpressionFails",
			expression:  "cluster.workers.count +",
			config:      map[string]any{},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateValue(tt.expression, tt.config, "features/test.yaml")

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for expression '%s', got none", tt.expression)
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error for expression '%s', got %v", tt.expression, err)
				return
			}

			if !deepEqual(result, tt.expected) {
				t.Errorf("Expected %v (type: %T) for expression '%s', got %v (type: %T)",
					tt.expected, tt.expected, tt.expression, result, result)
			}
		})
	}
}

func TestFeatureEvaluator_EvaluateDefaults(t *testing.T) {
	injector := di.NewInjector()
	evaluator := NewFeatureEvaluator(injector)
	_ = evaluator.Initialize()

	t.Run("EvaluatesLiteralValues", func(t *testing.T) {
		defaults := map[string]any{
			"cluster_name": "talos",
			"region":       "us-east-1",
		}

		config := map[string]any{}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["cluster_name"] != "talos" {
			t.Errorf("Expected cluster_name to be 'talos', got %v", result["cluster_name"])
		}
		if result["region"] != "us-east-1" {
			t.Errorf("Expected region to be 'us-east-1', got %v", result["region"])
		}
	})

	t.Run("EvaluatesSimpleExpressions", func(t *testing.T) {
		defaults := map[string]any{
			"count":    "${cluster.workers.count}",
			"endpoint": "${cluster.endpoint}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
				"endpoint": "https://localhost:6443",
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["count"] != 3 {
			t.Errorf("Expected count to be 3, got %v", result["count"])
		}
		if result["endpoint"] != "https://localhost:6443" {
			t.Errorf("Expected endpoint to be 'https://localhost:6443', got %v", result["endpoint"])
		}
	})

	t.Run("EvaluatesArithmeticExpressions", func(t *testing.T) {
		defaults := map[string]any{
			"min_size": "${cluster.workers.count}",
			"max_size": "${cluster.workers.count + 2}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["min_size"] != 3 {
			t.Errorf("Expected min_size to be 3, got %v", result["min_size"])
		}
		if result["max_size"] != 5 {
			t.Errorf("Expected max_size to be 5, got %v", result["max_size"])
		}
	})

	t.Run("EvaluatesNestedMaps", func(t *testing.T) {
		defaults := map[string]any{
			"node_groups": map[string]any{
				"default": map[string]any{
					"min_size": "${cluster.workers.count}",
					"max_size": "${cluster.workers.count + 2}",
				},
			},
			"region": "us-east-1",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		nodeGroups, ok := result["node_groups"].(map[string]any)
		if !ok {
			t.Fatalf("Expected node_groups to be a map, got %T", result["node_groups"])
		}

		defaultGroup, ok := nodeGroups["default"].(map[string]any)
		if !ok {
			t.Fatalf("Expected default group to be a map, got %T", nodeGroups["default"])
		}

		if defaultGroup["min_size"] != 3 {
			t.Errorf("Expected min_size to be 3, got %v", defaultGroup["min_size"])
		}
		if defaultGroup["max_size"] != 5 {
			t.Errorf("Expected max_size to be 5, got %v", defaultGroup["max_size"])
		}
	})

	t.Run("EvaluatesArraysWithExpressions", func(t *testing.T) {
		defaults := map[string]any{
			"instance_types": []any{
				"${cluster.workers.instance_type}",
			},
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"instance_type": "t3.medium",
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		instanceTypes, ok := result["instance_types"].([]any)
		if !ok {
			t.Fatalf("Expected instance_types to be an array, got %T", result["instance_types"])
		}

		if len(instanceTypes) != 1 || instanceTypes[0] != "t3.medium" {
			t.Errorf("Expected instance_types to be ['t3.medium'], got %v", instanceTypes)
		}
	})

	t.Run("UndefinedVariablesReturnNil", func(t *testing.T) {
		defaults := map[string]any{
			"endpoint":  "${cluster.endpoint}",
			"defined":   "${cluster.workers.count}",
			"undefined": "${cluster.undefined}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["endpoint"] != nil {
			t.Errorf("Expected endpoint to be nil, got %v", result["endpoint"])
		}
		if result["defined"] != 3 {
			t.Errorf("Expected defined to be 3, got %v", result["defined"])
		}
		if result["undefined"] != nil {
			t.Errorf("Expected undefined to be nil, got %v", result["undefined"])
		}
	})

	t.Run("InvalidExpressionFails", func(t *testing.T) {
		defaults := map[string]any{
			"bad_expr": "${cluster.workers.count +}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		_, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for invalid expression, got nil")
		}
	})

	t.Run("MixesLiteralsAndExpressions", func(t *testing.T) {
		defaults := map[string]any{
			"cluster_name":   "talos",
			"region":         "us-east-1",
			"count":          "${cluster.workers.count}",
			"max":            "${cluster.workers.count + 2}",
			"literal_number": 42,
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["cluster_name"] != "talos" {
			t.Errorf("Expected cluster_name to be 'talos', got %v", result["cluster_name"])
		}
		if result["region"] != "us-east-1" {
			t.Errorf("Expected region to be 'us-east-1', got %v", result["region"])
		}
		if result["count"] != 3 {
			t.Errorf("Expected count to be 3, got %v", result["count"])
		}
		if result["max"] != 5 {
			t.Errorf("Expected max to be 5, got %v", result["max"])
		}
		if result["literal_number"] != 42 {
			t.Errorf("Expected literal_number to be 42, got %v", result["literal_number"])
		}
	})

	t.Run("InterpolatesStringsWithSingleExpression", func(t *testing.T) {
		defaults := map[string]any{
			"domain": "grafana.${dns.domain}",
		}

		config := map[string]any{
			"dns": map[string]any{
				"domain": "example.com",
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["domain"] != "grafana.example.com" {
			t.Errorf("Expected domain to be 'grafana.example.com', got %v", result["domain"])
		}
	})

	t.Run("InterpolatesStringsWithMultipleExpressions", func(t *testing.T) {
		defaults := map[string]any{
			"url": "${protocol}://${dns.domain}:${port}",
		}

		config := map[string]any{
			"protocol": "https",
			"dns": map[string]any{
				"domain": "example.com",
			},
			"port": 8080,
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["url"] != "https://example.com:8080" {
			t.Errorf("Expected url to be 'https://example.com:8080', got %v", result["url"])
		}
	})

	t.Run("InterpolatesStringsWithNumbers", func(t *testing.T) {
		defaults := map[string]any{
			"label": "worker-${cluster.workers.count}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["label"] != "worker-3" {
			t.Errorf("Expected label to be 'worker-3', got %v", result["label"])
		}
	})

	t.Run("FailsOnUnclosedInterpolationExpression", func(t *testing.T) {
		defaults := map[string]any{
			"bad": "prefix-${dns.domain",
		}

		config := map[string]any{
			"dns": map[string]any{
				"domain": "example.com",
			},
		}

		_, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for unclosed expression, got nil")
		}
	})

	t.Run("FailsOnInvalidInterpolationExpression", func(t *testing.T) {
		defaults := map[string]any{
			"bad": "prefix-${invalid + }",
		}

		config := map[string]any{}

		_, err := evaluator.EvaluateDefaults(defaults, config, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for invalid interpolation expression, got nil")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestFeatureEvaluator_extractExpression(t *testing.T) {
	injector := di.NewInjector()
	evaluator := NewFeatureEvaluator(injector)
	_ = evaluator.Initialize()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SimpleExpression",
			input:    "${cluster.endpoint}",
			expected: "cluster.endpoint",
		},
		{
			name:     "ArithmeticExpression",
			input:    "${cluster.workers.count + 2}",
			expected: "cluster.workers.count + 2",
		},
		{
			name:     "LiteralString",
			input:    "talos",
			expected: "",
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "PartialExpressionNotExtracted",
			input:    "${cluster.endpoint} additional text",
			expected: "",
		},
		{
			name:     "MissingClosingBrace",
			input:    "${cluster.endpoint",
			expected: "",
		},
		{
			name:     "EmptyExpression",
			input:    "${}",
			expected: "",
		},
		{
			name:     "ComplexExpression",
			input:    "${cluster.workers.count * 2 + 1}",
			expected: "cluster.workers.count * 2 + 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.extractExpression(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s' for input '%s'", tt.expected, result, tt.input)
			}
		})
	}
}

func TestFeatureEvaluator_evaluateDefaultValue(t *testing.T) {
	injector := di.NewInjector()
	evaluator := NewFeatureEvaluator(injector)
	_ = evaluator.Initialize()

	t.Run("LiteralStringPassesThrough", func(t *testing.T) {
		result, err := evaluator.evaluateDefaultValue("talos", map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != "talos" {
			t.Errorf("Expected 'talos', got %v", result)
		}
	})

	t.Run("ExpressionEvaluates", func(t *testing.T) {
		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.evaluateDefaultValue("${cluster.workers.count}", config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != 3 {
			t.Errorf("Expected 3, got %v", result)
		}
	})

	t.Run("NestedMapEvaluates", func(t *testing.T) {
		input := map[string]any{
			"literal": "value",
			"expr":    "${cluster.workers.count}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.evaluateDefaultValue(input, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map, got %T", result)
		}

		if resultMap["literal"] != "value" {
			t.Errorf("Expected literal to be 'value', got %v", resultMap["literal"])
		}
		if resultMap["expr"] != 3 {
			t.Errorf("Expected expr to be 3, got %v", resultMap["expr"])
		}
	})

	t.Run("ArrayEvaluates", func(t *testing.T) {
		input := []any{
			"literal",
			"${cluster.workers.count}",
		}

		config := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": 3,
				},
			},
		}

		result, err := evaluator.evaluateDefaultValue(input, config, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected array, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected 2 elements, got %d", len(resultArray))
		}
		if resultArray[0] != "literal" {
			t.Errorf("Expected first element to be 'literal', got %v", resultArray[0])
		}
		if resultArray[1] != 3 {
			t.Errorf("Expected second element to be 3, got %v", resultArray[1])
		}
	})

	t.Run("NonStringTypesPassThrough", func(t *testing.T) {
		result, err := evaluator.evaluateDefaultValue(42, map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}

		result, err = evaluator.evaluateDefaultValue(true, map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func deepEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch aVal := a.(type) {
	case map[string]any:
		bVal, ok := b.(map[string]any)
		if !ok {
			return false
		}
		if len(aVal) != len(bVal) {
			return false
		}
		for k, v := range aVal {
			if !deepEqual(v, bVal[k]) {
				return false
			}
		}
		return true
	case []any:
		bVal, ok := b.([]any)
		if !ok {
			return false
		}
		if len(aVal) != len(bVal) {
			return false
		}
		for i := range aVal {
			if !deepEqual(aVal[i], bVal[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// =============================================================================
// Test File Loading Functions
// =============================================================================

func TestFeatureEvaluator_JsonnetFunction(t *testing.T) {
	t.Run("LoadsAndEvaluatesJsonnetFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `{
  name: "test-config",
  replicas: 3,
  enabled: true
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "config.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }

		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("config.jsonnet")`, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["name"] != "test-config" {
			t.Errorf("Expected name='test-config', got %v", resultMap["name"])
		}
		if resultMap["replicas"] != float64(3) {
			t.Errorf("Expected replicas=3, got %v", resultMap["replicas"])
		}
		if resultMap["enabled"] != true {
			t.Errorf("Expected enabled=true, got %v", resultMap["enabled"])
		}
	})

	t.Run("JsonnetFileWithContextVariable", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `local ctx = std.extVar('context');
{
  namespace: ctx.namespace,
  region: ctx.region
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "context-config.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }

		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{
			"namespace": "production",
			"region":    "us-west-2",
		}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("context-config.jsonnet")`, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["namespace"] != "production" {
			t.Errorf("Expected namespace='production', got %v", resultMap["namespace"])
		}
		if resultMap["region"] != "us-west-2" {
			t.Errorf("Expected region='us-west-2', got %v", resultMap["region"])
		}
	})

	t.Run("JsonnetFileWithEnrichedContext", func(t *testing.T) {
		tmpDir := t.TempDir()
		projectDir := tmpDir + "/my-project"

		jsonnetContent := `local ctx = std.extVar('context');
{
  projectName: if std.objectHas(ctx, 'projectName') then ctx.projectName else 'unknown',
  environment: ctx.environment
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(projectDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "enriched-config.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return projectDir, nil }

		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{
			"environment": "production",
		}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("enriched-config.jsonnet")`, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["projectName"] != "my-project" {
			t.Errorf("Expected projectName='my-project', got %v", resultMap["projectName"])
		}
		if resultMap["environment"] != "production" {
			t.Errorf("Expected environment='production', got %v", resultMap["environment"])
		}
	})

	t.Run("JsonnetFileWithRelativePath", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `{
  source: "nested"
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features", "configs")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "nested.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{}

		featurePath := filepath.Join(tmpDir, "contexts", "_template", "features", "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("configs/nested.jsonnet")`, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["source"] != "nested" {
			t.Errorf("Expected source='nested', got %v", resultMap["source"])
		}
	})

	t.Run("JsonnetFileNotFound", func(t *testing.T) {
		tmpDir := t.TempDir()
		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{}

		_, err := evaluator.EvaluateValue(`jsonnet("nonexistent.jsonnet")`, config, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for nonexistent file, got nil")
		}
	})

	t.Run("JsonnetFileInvalidSyntax", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `{
  invalid syntax here
}`
		jsonnetPath := tmpDir + "/invalid.jsonnet"
		if err := writeTestFile(jsonnetPath, jsonnetContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()
		config := map[string]any{}

		_, err := evaluator.EvaluateValue(`jsonnet("invalid.jsonnet")`, config, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for invalid jsonnet, got nil")
		}
	})
}

func TestFeatureEvaluator_FileFunction(t *testing.T) {
	t.Run("LoadsRawFileContent", func(t *testing.T) {
		tmpDir := t.TempDir()

		content := "Hello, World!\nThis is a test file."

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "test.txt")
		if err := os.WriteFile(expectedPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }

		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`file("test.txt")`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string, got %T", result)
		}

		if resultStr != content {
			t.Errorf("Expected content='%s', got '%s'", content, resultStr)
		}
	})

	t.Run("LoadsYAMLFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		yamlContent := `name: test-service
version: 1.0.0
enabled: true`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "config.yaml")
		if err := os.WriteFile(expectedPath, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`file("config.yaml")`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string, got %T", result)
		}

		if resultStr != yamlContent {
			t.Errorf("Expected yaml content, got different content")
		}
	})

	t.Run("FileNotFound", func(t *testing.T) {
		tmpDir := t.TempDir()
		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		_, err := evaluator.EvaluateValue(`file("nonexistent.txt")`, map[string]any{}, "features/test.yaml")
		if err == nil {
			t.Fatal("Expected error for nonexistent file, got nil")
		}
	})
}

func TestFeatureEvaluator_FileLoadingInDefaults(t *testing.T) {
	t.Run("EvaluatesJsonnetInDefaults", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `{
  database: {
    host: "localhost",
    port: 5432
  }
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "db-config.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		defaults := map[string]any{
			"db_config": `${jsonnet("db-config.jsonnet")}`,
			"name":      "my-service",
		}

		config := map[string]any{}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateDefaults(defaults, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		dbConfig, ok := result["db_config"].(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any for db_config, got %T", result["db_config"])
		}

		database, ok := dbConfig["database"].(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any for database, got %T", dbConfig["database"])
		}

		if database["host"] != "localhost" {
			t.Errorf("Expected host='localhost', got %v", database["host"])
		}
		if database["port"] != float64(5432) {
			t.Errorf("Expected port=5432, got %v", database["port"])
		}
	})

	t.Run("EvaluatesFileInDefaults", func(t *testing.T) {
		tmpDir := t.TempDir()

		fileContent := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "key.pub")
		if err := os.WriteFile(expectedPath, []byte(fileContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		defaults := map[string]any{
			"ssh_key": `${file("key.pub")}`,
		}

		config := map[string]any{}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateDefaults(defaults, config, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["ssh_key"] != fileContent {
			t.Errorf("Expected ssh_key to contain file content")
		}
	})
}

func TestFeatureEvaluator_AbsolutePaths(t *testing.T) {
	t.Run("HandlesAbsolutePathForJsonnet", func(t *testing.T) {
		tmpDir := t.TempDir()

		jsonnetContent := `{
  test: "absolute"
}`
		jsonnetPath := filepath.Join(tmpDir, "absolute.jsonnet")
		if err := writeTestFile(jsonnetPath, jsonnetContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		result, err := evaluator.EvaluateValue(`jsonnet("`+strings.ReplaceAll(jsonnetPath, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["test"] != "absolute" {
			t.Errorf("Expected test='absolute', got %v", resultMap["test"])
		}
	})
}

func TestFeatureEvaluator_PathResolution(t *testing.T) {
	t.Run("FallbackToProjectRootWhenNoContextRoot", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonnetContent := `{
  test: "project-root"
}`
		if err := writeTestFile(filepath.Join(tmpDir, "config.jsonnet"), jsonnetContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("no context root")
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		result, err := evaluator.EvaluateValue(`jsonnet("config.jsonnet")`, map[string]any{}, "")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["test"] != "project-root" {
			t.Errorf("Expected test='project-root', got %v", resultMap["test"])
		}
	})

	t.Run("FallbackToCleanPathWhenNoShell", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonnetContent := `{
  test: "clean-path"
}`
		testFile := filepath.Join(tmpDir, "test.jsonnet")
		if err := writeTestFile(testFile, jsonnetContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("no context root")
		}
		injector.Register("configHandler", mockConfig)

		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		result, err := evaluator.EvaluateValue(`jsonnet("`+strings.ReplaceAll(testFile, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["test"] != "clean-path" {
			t.Errorf("Expected test='clean-path', got %v", resultMap["test"])
		}
	})

	t.Run("FeatureDirTakesPrecedenceOverContextRoot", func(t *testing.T) {
		tmpDir := t.TempDir()
		featureSubDir := filepath.Join(tmpDir, "contexts", "_template", "features", "aws")
		if err := os.MkdirAll(featureSubDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		featuresDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		jsonnetContent := `{
  test: "feature-dir"
}`
		if err := os.WriteFile(filepath.Join(featuresDir, "config.jsonnet"), []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		wrongJsonnetContent := `{
  test: "wrong"
}`
		if err := os.WriteFile(filepath.Join(tmpDir, "config.jsonnet"), []byte(wrongJsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(featuresDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("config.jsonnet")`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["test"] != "feature-dir" {
			t.Errorf("Expected test='feature-dir', got %v", resultMap["test"])
		}
	})

	t.Run("AccessNestedFieldFromJsonnetFunction", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonnetContent := `{
  worker_config_patches: ["patch1", "patch2"],
  control_plane_patches: ["cp-patch1"],
  other_config: {
    nested: "value"
  }
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "talos-dev.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("talos-dev.jsonnet").worker_config_patches`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultSlice, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", result)
		}

		if len(resultSlice) != 2 {
			t.Errorf("Expected 2 patches, got %d", len(resultSlice))
		}

		if resultSlice[0] != "patch1" || resultSlice[1] != "patch2" {
			t.Errorf("Expected ['patch1', 'patch2'], got %v", resultSlice)
		}
	})

	t.Run("AccessDeeplyNestedFieldFromJsonnetFunction", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonnetContent := `{
  config: {
    nested: {
      deeply: {
        value: "found it!"
      }
    }
  }
}`

		// Create the expected directory structure and file
		expectedDir := filepath.Join(tmpDir, "contexts", "_template", "features")
		if err := os.MkdirAll(expectedDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		expectedPath := filepath.Join(expectedDir, "nested.jsonnet")
		if err := os.WriteFile(expectedPath, []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("nested.jsonnet").config.nested.deeply.value`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "found it!" {
			t.Errorf("Expected 'found it!', got %v", result)
		}
	})

	t.Run("RelativePathFromFeatureFileInFeaturesDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		templateRoot := filepath.Join(tmpDir, "contexts", "_template")

		configsDir := filepath.Join(templateRoot, "configs")
		if err := os.MkdirAll(configsDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		featuresDir := filepath.Join(templateRoot, "features")
		if err := os.MkdirAll(featuresDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		jsonnetContent := `{
  worker_config_patches: ["patch1", "patch2"]
}`
		if err := os.WriteFile(filepath.Join(configsDir, "talos-dev.jsonnet"), []byte(jsonnetContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		featurePath := filepath.Join(featuresDir, "test.yaml")
		result, err := evaluator.EvaluateValue(`jsonnet("../configs/talos-dev.jsonnet").worker_config_patches`, map[string]any{}, featurePath)
		if err != nil {
			t.Fatalf("Expected no error, got: %v\nFeatureDir: %s\nExpected file: %s", err, featuresDir, filepath.Join(configsDir, "talos-dev.jsonnet"))
		}

		resultSlice, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", result)
		}

		if len(resultSlice) != 2 {
			t.Errorf("Expected 2 patches, got %d", len(resultSlice))
		}

		if resultSlice[0] != "patch1" || resultSlice[1] != "patch2" {
			t.Errorf("Expected ['patch1', 'patch2'], got %v", resultSlice)
		}
	})

	t.Run("ProjectRootErrorFallsBackToCleanPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonnetContent := `{
  test: "clean-fallback"
}`
		testFile := filepath.Join(tmpDir, "fallback.jsonnet")
		if err := writeTestFile(testFile, jsonnetContent); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		injector := di.NewInjector()
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("no context root")
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("no project root")
		}
		injector.Register("configHandler", mockConfig)
		injector.Register("shell", mockShell)
		evaluator := NewFeatureEvaluator(injector)
		_ = evaluator.Initialize()

		result, err := evaluator.EvaluateValue(`jsonnet("`+strings.ReplaceAll(testFile, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected map[string]any, got %T", result)
		}

		if resultMap["test"] != "clean-fallback" {
			t.Errorf("Expected test='clean-fallback', got %v", resultMap["test"])
		}
	})
}

func writeTestFile(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}
