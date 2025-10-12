package blueprint

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewFeatureEvaluator(t *testing.T) {
	t.Run("CreatesNewFeatureEvaluatorSuccessfully", func(t *testing.T) {
		evaluator := NewFeatureEvaluator()
		if evaluator == nil {
			t.Fatal("Expected evaluator, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFeatureEvaluator_EvaluateExpression(t *testing.T) {
	evaluator := NewFeatureEvaluator()

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
			result, err := evaluator.EvaluateExpression(tt.expression, tt.config)

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
	evaluator := NewFeatureEvaluator()

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
			result, err := evaluator.EvaluateValue(tt.expression, tt.config)

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
	evaluator := NewFeatureEvaluator()

	t.Run("EvaluatesLiteralValues", func(t *testing.T) {
		defaults := map[string]any{
			"cluster_name": "talos",
			"region":       "us-east-1",
		}

		config := map[string]any{}

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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

		_, err := evaluator.EvaluateDefaults(defaults, config)
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

		result, err := evaluator.EvaluateDefaults(defaults, config)
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
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestFeatureEvaluator_extractExpression(t *testing.T) {
	evaluator := NewFeatureEvaluator()

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
	evaluator := NewFeatureEvaluator()

	t.Run("LiteralStringPassesThrough", func(t *testing.T) {
		result, err := evaluator.evaluateDefaultValue("talos", map[string]any{})
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

		result, err := evaluator.evaluateDefaultValue("${cluster.workers.count}", config)
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

		result, err := evaluator.evaluateDefaultValue(input, config)
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

		result, err := evaluator.evaluateDefaultValue(input, config)
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
		result, err := evaluator.evaluateDefaultValue(42, map[string]any{})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}

		result, err = evaluator.evaluateDefaultValue(true, map[string]any{})
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
