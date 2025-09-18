package blueprint

import (
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewFeatureEvaluator(t *testing.T) {
	t.Run("CreatesNewFeatureEvaluatorSuccessfully", func(t *testing.T) {
		evaluator, err := NewFeatureEvaluator()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if evaluator == nil {
			t.Fatal("Expected evaluator, got nil")
		}
		if evaluator.env == nil {
			t.Fatal("Expected CEL env to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCompileExpression(t *testing.T) {
	evaluator, err := NewFeatureEvaluator()
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		name        string
		expression  string
		shouldError bool
	}{
		{
			name:        "EmptyExpressionFails",
			expression:  "",
			shouldError: true,
		},
		{
			name:        "SimpleEqualityExpression",
			expression:  "provider == 'aws'",
			shouldError: false,
		},
		{
			name:        "SimpleInequalityExpression",
			expression:  "provider != 'local'",
			shouldError: false,
		},
		{
			name:        "LogicalAndExpression",
			expression:  "provider == 'local' && observability.enabled == true",
			shouldError: false,
		},
		{
			name:        "LogicalOrExpression",
			expression:  "provider == 'aws' || provider == 'azure'",
			shouldError: false,
		},
		{
			name:        "ParenthesesGrouping",
			expression:  "provider == 'local' && (vm.driver != 'docker-desktop' || loadbalancer.enabled == true)",
			shouldError: false,
		},
		{
			name:        "NestedObjectAccess",
			expression:  "observability.enabled == true && observability.backend == 'quickwit'",
			shouldError: false,
		},
		{
			name:        "BooleanComparison",
			expression:  "dns.enabled == true",
			shouldError: false,
		},
		{
			name:        "InvalidSyntaxFails",
			expression:  "provider ===",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{
				"provider": "aws",
				"observability": map[string]any{
					"enabled": true,
					"backend": "quickwit",
				},
				"vm": map[string]any{
					"driver": "virtualbox",
				},
				"loadbalancer": map[string]any{
					"enabled": true,
				},
				"dns": map[string]any{
					"enabled": true,
				},
			}

			program, err := evaluator.CompileExpression(tt.expression, config)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for expression '%s', got none", tt.expression)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for expression '%s', got %v", tt.expression, err)
				}
				if program == nil {
					t.Errorf("Expected program for expression '%s', got nil", tt.expression)
				}
			}
		})
	}
}

func TestEvaluateProgram(t *testing.T) {
	evaluator, err := NewFeatureEvaluator()
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		name       string
		expression string
		config     map[string]any
		expected   bool
		shouldErr  bool
	}{
		{
			name:       "SimpleStringEqualityTrue",
			expression: "provider == 'aws'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "SimpleStringEqualityFalse",
			expression: "provider == 'aws'",
			config:     map[string]any{"provider": "local"},
			expected:   false,
		},
		{
			name:       "StringInequalityTrue",
			expression: "provider != 'local'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "StringInequalityFalse",
			expression: "provider != 'local'",
			config:     map[string]any{"provider": "local"},
			expected:   false,
		},
		{
			name:       "BooleanEqualityTrue",
			expression: "observability.enabled == true",
			config: map[string]any{
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:       "BooleanEqualityFalse",
			expression: "observability.enabled == true",
			config: map[string]any{
				"observability": map[string]any{
					"enabled": false,
				},
			},
			expected: false,
		},
		{
			name:       "LogicalAndBothTrue",
			expression: "provider == 'local' && observability.enabled == true",
			config: map[string]any{
				"provider": "local",
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:       "LogicalAndFirstFalse",
			expression: "provider == 'local' && observability.enabled == true",
			config: map[string]any{
				"provider": "aws",
				"observability": map[string]any{
					"enabled": true,
				},
			},
			expected: false,
		},
		{
			name:       "LogicalOrFirstTrue",
			expression: "provider == 'aws' || provider == 'azure'",
			config:     map[string]any{"provider": "aws"},
			expected:   true,
		},
		{
			name:       "LogicalOrSecondTrue",
			expression: "provider == 'aws' || provider == 'azure'",
			config:     map[string]any{"provider": "azure"},
			expected:   true,
		},
		{
			name:       "LogicalOrBothFalse",
			expression: "provider == 'aws' || provider == 'azure'",
			config:     map[string]any{"provider": "local"},
			expected:   false,
		},
		{
			name:       "ParenthesesGroupingComplexExpressionTrue",
			expression: "provider == 'local' && (vm.driver != 'docker-desktop' || loadbalancer.enabled == true)",
			config: map[string]any{
				"provider": "local",
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
			name:       "NestedObjectAccessMultipleLevels",
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
			name:       "MissingFieldEvaluatesToNullFalseComparison",
			expression: "missing.field == 'value'",
			config: map[string]any{
				"missing": map[string]any{
					"field": nil,
				},
			},
			expected: false,
		},
		{
			name:       "NilConfigHandledGracefully",
			expression: "provider == 'aws'",
			config: map[string]any{
				"provider": nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := evaluator.CompileExpression(tt.expression, tt.config)
			if err != nil {
				t.Fatalf("Failed to compile expression '%s': %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateProgram(program, tt.config)
			if tt.shouldErr {
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

func TestEvaluateExpression(t *testing.T) {
	evaluator, err := NewFeatureEvaluator()
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	t.Run("ConvenienceMethodWorksCorrectly", func(t *testing.T) {
		config := map[string]any{
			"provider": "aws",
			"observability": map[string]any{
				"enabled": true,
				"backend": "quickwit",
			},
		}

		result, err := evaluator.EvaluateExpression("provider == 'aws' && observability.enabled == true", config)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !result {
			t.Errorf("Expected true, got false")
		}
	})

	t.Run("InvalidExpressionReturnsError", func(t *testing.T) {
		_, err := evaluator.EvaluateExpression("invalid === syntax", map[string]any{})
		if err == nil {
			t.Error("Expected error for invalid expression, got none")
		}
	})
}

func TestConvertToBool(t *testing.T) {
	evaluator, err := NewFeatureEvaluator()
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		name       string
		expression string
		config     map[string]any
		shouldErr  bool
	}{
		{
			name:       "BooleanResultConvertsSuccessfully",
			expression: "provider == 'aws'",
			config:     map[string]any{"provider": "aws"},
			shouldErr:  false,
		},
		{
			name:       "StringResultShouldError",
			expression: "provider",
			config:     map[string]any{"provider": "aws"},
			shouldErr:  true,
		},
		{
			name:       "NumberResultShouldError",
			expression: "count",
			config:     map[string]any{"count": 5},
			shouldErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := evaluator.CompileExpression(tt.expression, tt.config)
			if err != nil {
				t.Fatalf("Failed to compile expression '%s': %v", tt.expression, err)
			}

			_, err = evaluator.EvaluateProgram(program, tt.config)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for non-boolean result, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for boolean result, got %v", err)
				}
			}
		})
	}
}
