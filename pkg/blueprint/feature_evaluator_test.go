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

func TestEvaluateExpression(t *testing.T) {
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

func TestMatchConditions(t *testing.T) {
	evaluator := NewFeatureEvaluator()

	tests := []struct {
		name       string
		conditions map[string]any
		config     map[string]any
		expected   bool
	}{
		{
			name:       "EmptyConditionsAlwaysMatch",
			conditions: map[string]any{},
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "SimpleStringEqualityTrue",
			conditions: map[string]any{"provider": "aws"},
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "SimpleStringEqualityFalse",
			conditions: map[string]any{"provider": "aws"},
			config:     map[string]any{"provider": "generic"},
			expected:   false,
		},
		{
			name:       "BooleanEqualityTrue",
			conditions: map[string]any{"observability.enabled": true},
			config: map[string]any{
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:       "BooleanEqualityFalse",
			conditions: map[string]any{"observability.enabled": true},
			config: map[string]any{
				"observability": map[string]any{
					"enabled": false,
				},
			},
			expected: false,
		},
		{
			name: "MultipleConditionsAllMatch",
			conditions: map[string]any{
				"provider":              "generic",
				"observability.enabled": true,
				"observability.backend": "quickwit",
			},
			config: map[string]any{
				"provider": "generic",
				"observability": map[string]any{
					"enabled": true,
					"backend": "quickwit",
				},
			},
			expected: true,
		},
		{
			name: "MultipleConditionsOneDoesNotMatch",
			conditions: map[string]any{
				"provider":              "generic",
				"observability.enabled": true,
				"observability.backend": "elk",
			},
			config: map[string]any{
				"provider": "generic",
				"observability": map[string]any{
					"enabled": true,
					"backend": "quickwit",
				},
			},
			expected: false,
		},
		{
			name:       "ArrayConditionMatchesFirst",
			conditions: map[string]any{"storage.provider": []any{"auto", "openebs"}},
			config:     map[string]any{"storage": map[string]any{"provider": "auto"}},
			expected:   true,
		},
		{
			name:       "ArrayConditionMatchesSecond",
			conditions: map[string]any{"storage.provider": []any{"auto", "openebs"}},
			config:     map[string]any{"storage": map[string]any{"provider": "openebs"}},
			expected:   true,
		},
		{
			name:       "ArrayConditionNoMatch",
			conditions: map[string]any{"storage.provider": []any{"auto", "openebs"}},
			config:     map[string]any{"storage": map[string]any{"provider": "ebs"}},
			expected:   false,
		},
		{
			name:       "MissingFieldDoesNotMatch",
			conditions: map[string]any{"missing.field": "value"},
			config:     map[string]any{"other": "data"},
			expected:   false,
		},
		{
			name:       "NilValueDoesNotMatch",
			conditions: map[string]any{"provider": "aws"},
			config:     map[string]any{"provider": nil},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.MatchConditions(tt.conditions, tt.config)

			if result != tt.expected {
				t.Errorf("Expected %v for conditions %v with config %v, got %v",
					tt.expected, tt.conditions, tt.config, result)
			}
		})
	}
}
