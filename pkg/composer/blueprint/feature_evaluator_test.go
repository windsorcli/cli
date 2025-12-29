package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupFeatureEvaluator(t *testing.T) *FeatureEvaluator {
	t.Helper()
	tmpDir := t.TempDir()
	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()
	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}
	return NewFeatureEvaluator(rt)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewFeatureEvaluator(t *testing.T) {
	t.Run("CreatesNewFeatureEvaluatorSuccessfully", func(t *testing.T) {
		evaluator := setupFeatureEvaluator(t)
		if evaluator == nil {
			t.Fatal("Expected evaluator, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFeatureEvaluator_Evaluate(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)
	projectRoot := strings.ReplaceAll(evaluator.runtime.ProjectRoot, "\\", "/")
	contextPath := "mock-config-root"

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
		{
			name:       "ProjectRootVariable",
			expression: "project_root",
			config:     map[string]any{},
			expected:   projectRoot,
		},
		{
			name:       "ContextPathVariable",
			expression: "context_path",
			config:     map[string]any{},
			expected:   contextPath,
		},
		{
			name:       "ProjectRootInStringInterpolation",
			expression: `project_root + "/config.yaml"`,
			config:     map[string]any{},
			expected:   projectRoot + "/config.yaml",
		},
		{
			name:       "ContextPathInStringInterpolation",
			expression: `context_path + "/blueprint.yaml"`,
			config:     map[string]any{},
			expected:   contextPath + "/blueprint.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expression, tt.config, "features/test.yaml")

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
				t.Errorf("Expected %v for expression '%s' with config %v, got %v",
					tt.expected, tt.expression, tt.config, result)
			}
		})
	}
}

func TestFeatureEvaluator_EvaluateDefaults(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

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
	evaluator := setupFeatureEvaluator(t)

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
	evaluator := setupFeatureEvaluator(t)

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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()

		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("config.jsonnet")`, config, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()

		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{
			"namespace": "production",
			"region":    "us-west-2",
		}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("context-config.jsonnet")`, config, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()

		rt := &runtime.Runtime{
			ProjectRoot:   projectDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{
			"environment": "production",
		}

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("enriched-config.jsonnet")`, config, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{}

		featurePath := filepath.Join(tmpDir, "contexts", "_template", "features", "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("configs/nested.jsonnet")`, config, featurePath)
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
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{}

		_, err := evaluator.Evaluate(`jsonnet("nonexistent.jsonnet")`, config, "features/test.yaml")
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)
		config := map[string]any{}

		_, err := evaluator.Evaluate(`jsonnet("invalid.jsonnet")`, config, "features/test.yaml")
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()

		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`file("test.txt")`, map[string]any{}, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`file("config.yaml")`, map[string]any{}, featurePath)
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
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		_, err := evaluator.Evaluate(`file("nonexistent.txt")`, map[string]any{}, "features/test.yaml")
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) { return tmpDir, nil }
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

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

		configHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		rt := &runtime.Runtime{
			ConfigHandler: configHandler,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		result, err := evaluator.Evaluate(`jsonnet("`+strings.ReplaceAll(jsonnetPath, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    "",
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		result, err := evaluator.Evaluate(`jsonnet("config.jsonnet")`, map[string]any{}, "")
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

		configHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: configHandler,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		result, err := evaluator.Evaluate(`jsonnet("`+strings.ReplaceAll(testFile, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(featuresDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("config.jsonnet")`, map[string]any{}, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("talos-dev.jsonnet").worker_config_patches`, map[string]any{}, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		rt := &runtime.Runtime{
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(expectedDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("nested.jsonnet").config.nested.deeply.value`, map[string]any{}, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		rt := &runtime.Runtime{
			ProjectRoot:   tmpDir,
			ConfigRoot:    tmpDir,
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		featurePath := filepath.Join(featuresDir, "test.yaml")
		result, err := evaluator.Evaluate(`jsonnet("../configs/talos-dev.jsonnet").worker_config_patches`, map[string]any{}, featurePath)
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

		mockConfig := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		rt := &runtime.Runtime{
			ProjectRoot:   "",
			ConfigRoot:    "",
			ConfigHandler: mockConfig,
			Shell:         mockShell,
		}
		evaluator := NewFeatureEvaluator(rt)

		result, err := evaluator.Evaluate(`jsonnet("`+strings.ReplaceAll(testFile, "\\", "\\\\")+`")`, map[string]any{}, "features/test.yaml")
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

func TestFeatureEvaluator_ProcessFeature(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenWhenConditionIsFalse", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			When:     "provider == 'aws'",
		}
		config := map[string]any{"provider": "gcp"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Error("Expected nil when condition is false")
		}
	})

	t.Run("ReturnsErrorWhenWhenConditionEvaluationFails", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			When:     "invalid expression",
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when condition evaluation fails")
		}
	})

	t.Run("ProcessesFeatureWithoutWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
		}
		config := map[string]any{}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil {
			t.Fatal("Expected processed feature, got nil")
		}
		if result.Metadata.Name != "test-feature" {
			t.Errorf("Expected feature name 'test-feature', got '%s'", result.Metadata.Name)
		}
	})

	t.Run("FiltersTerraformComponentsByWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"},
					When:               "provider == 'aws'",
				},
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component2"},
					When:               "provider == 'gcp'",
				},
			},
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 component, got %d", len(result.TerraformComponents))
		}
		if result.TerraformComponents[0].TerraformComponent.Path != "component1" {
			t.Errorf("Expected component1, got %s", result.TerraformComponents[0].TerraformComponent.Path)
		}
	})

	t.Run("FiltersKustomizationsByWhenCondition", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{
					Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"},
					When:          "provider == 'aws'",
				},
				{
					Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization2"},
					When:          "provider == 'gcp'",
				},
			},
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.ProcessFeature(feature, config)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
		if result.Kustomizations[0].Kustomization.Name != "kustomization1" {
			t.Errorf("Expected kustomization1, got %s", result.Kustomizations[0].Kustomization.Name)
		}
	})

	t.Run("HandlesEvaluateDefaultsErrorForTerraformComponent", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{
					TerraformComponent: blueprintv1alpha1.TerraformComponent{
						Path: "component1",
						Inputs: map[string]any{
							"key": "${invalid expression [[[",
						},
					},
				},
			},
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when EvaluateDefaults fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate inputs") {
			t.Errorf("Expected error about evaluating inputs, got: %v", err)
		}
	})

	t.Run("HandlesEvaluateSubstitutionsErrorForKustomization", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{
					Kustomization: blueprintv1alpha1.Kustomization{
						Name: "kustomization1",
						Substitutions: map[string]string{
							"key": "${invalid expression [[[",
						},
					},
				},
			},
		}
		config := map[string]any{}

		_, err := evaluator.ProcessFeature(feature, config)

		if err == nil {
			t.Fatal("Expected error when evaluateSubstitutions fails")
		}
		if !strings.Contains(err.Error(), "failed to evaluate substitutions") {
			t.Errorf("Expected error about evaluating substitutions, got: %v", err)
		}
	})
}

func TestFeatureEvaluator_MergeFeatures(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenFeaturesIsEmpty", func(t *testing.T) {
		result := evaluator.MergeFeatures([]*blueprintv1alpha1.Feature{})

		if result != nil {
			t.Error("Expected nil when features is empty")
		}
	})

	t.Run("MergesMultipleFeatures", func(t *testing.T) {
		feature1 := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "feature1"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"}},
			},
		}
		feature2 := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "feature2"},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"}},
			},
		}

		result := evaluator.MergeFeatures([]*blueprintv1alpha1.Feature{feature1, feature2})

		if result == nil {
			t.Fatal("Expected merged feature, got nil")
		}
		if result.Metadata.Name != "merged-features" {
			t.Errorf("Expected name 'merged-features', got '%s'", result.Metadata.Name)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
	})
}

func TestFeatureEvaluator_FeatureToBlueprint(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("ReturnsNilWhenFeatureIsNil", func(t *testing.T) {
		result := evaluator.FeatureToBlueprint(nil)

		if result != nil {
			t.Error("Expected nil when feature is nil")
		}
	})

	t.Run("ConvertsFeatureToBlueprint", func(t *testing.T) {
		feature := &blueprintv1alpha1.Feature{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-feature"},
			TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
				{TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "component1"}},
			},
			Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "kustomization1"}},
			},
		}

		result := evaluator.FeatureToBlueprint(feature)

		if result == nil {
			t.Fatal("Expected blueprint, got nil")
		}
		if result.Kind != "Blueprint" {
			t.Errorf("Expected kind 'Blueprint', got '%s'", result.Kind)
		}
		if result.ApiVersion != "v1alpha1" {
			t.Errorf("Expected apiVersion 'v1alpha1', got '%s'", result.ApiVersion)
		}
		if result.Metadata.Name != "test-feature" {
			t.Errorf("Expected name 'test-feature', got '%s'", result.Metadata.Name)
		}
		if len(result.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(result.TerraformComponents))
		}
		if len(result.Kustomizations) != 1 {
			t.Errorf("Expected 1 kustomization, got %d", len(result.Kustomizations))
		}
	})
}

func TestFeatureEvaluator_evaluateSubstitutions(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("HandlesSubstitutionsWithoutExpressions", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		config := map[string]any{}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "value1" {
			t.Errorf("Expected 'value1', got '%s'", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected 'value2', got '%s'", result["key2"])
		}
	})

	t.Run("HandlesSubstitutionsWithExpressions", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${provider}",
		}
		config := map[string]any{"provider": "aws"}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "aws" {
			t.Errorf("Expected 'aws', got '%s'", result["key1"])
		}
	})

	t.Run("HandlesNilEvaluatedValue", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${nonexistent}",
		}
		config := map[string]any{}

		result, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["key1"] != "" {
			t.Errorf("Expected empty string for nil value, got '%s'", result["key1"])
		}
	})

	t.Run("ReturnsErrorWhenEvaluationFails", func(t *testing.T) {
		substitutions := map[string]string{
			"key1": "${invalid expression",
		}
		config := map[string]any{}

		_, err := evaluator.evaluateSubstitutions(substitutions, config, "")

		if err == nil {
			t.Fatal("Expected error when evaluation fails")
		}
	})
}

func TestFeatureEvaluator_buildExprEnvironment(t *testing.T) {
	evaluator := setupFeatureEvaluator(t)

	t.Run("HandlesJsonnetFunctionWithWrongNumberOfArguments", func(t *testing.T) {
		_, err := evaluator.Evaluate("jsonnet()", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when jsonnet() called with no arguments")
		}
		if !strings.Contains(err.Error(), "not enough arguments") {
			t.Errorf("Expected error about wrong number of arguments, got: %v", err)
		}
	})

	t.Run("HandlesJsonnetFunctionWithWrongType", func(t *testing.T) {
		_, err := evaluator.Evaluate("jsonnet(123)", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when jsonnet() called with non-string")
		}
		if !strings.Contains(err.Error(), "cannot use int") {
			t.Errorf("Expected error about wrong type, got: %v", err)
		}
	})

	t.Run("HandlesFileFunctionWithWrongNumberOfArguments", func(t *testing.T) {
		_, err := evaluator.Evaluate("file()", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when file() called with no arguments")
		}
		if !strings.Contains(err.Error(), "not enough arguments") {
			t.Errorf("Expected error about wrong number of arguments, got: %v", err)
		}
	})

	t.Run("HandlesFileFunctionWithWrongType", func(t *testing.T) {
		_, err := evaluator.Evaluate("file(123)", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when file() called with non-string")
		}
		if !strings.Contains(err.Error(), "cannot use int") {
			t.Errorf("Expected error about wrong type, got: %v", err)
		}
	})

	t.Run("HandlesSplitFunctionWithWrongNumberOfArguments", func(t *testing.T) {
		_, err := evaluator.Evaluate("split('a')", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when split() called with 1 argument")
		}
		if !strings.Contains(err.Error(), "not enough arguments") {
			t.Errorf("Expected error about wrong number of arguments, got: %v", err)
		}
	})

	t.Run("HandlesSplitFunctionWithWrongFirstArgumentType", func(t *testing.T) {
		_, err := evaluator.Evaluate("split(123, ',')", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when split() called with non-string first argument")
		}
		if !strings.Contains(err.Error(), "cannot use int") {
			t.Errorf("Expected error about wrong type, got: %v", err)
		}
	})

	t.Run("HandlesSplitFunctionWithWrongSecondArgumentType", func(t *testing.T) {
		_, err := evaluator.Evaluate("split('a,b', 123)", map[string]any{}, "")

		if err == nil {
			t.Fatal("Expected error when split() called with non-string second argument")
		}
		if !strings.Contains(err.Error(), "cannot use int") {
			t.Errorf("Expected error about wrong type, got: %v", err)
		}
	})

	t.Run("HandlesSplitFunctionSuccessfully", func(t *testing.T) {
		result, err := evaluator.Evaluate("split('a,b,c', ',')", map[string]any{}, "")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		resultSlice, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", result)
		}
		if len(resultSlice) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(resultSlice))
		}
		if resultSlice[0] != "a" || resultSlice[1] != "b" || resultSlice[2] != "c" {
			t.Errorf("Expected ['a', 'b', 'c'], got %v", resultSlice)
		}
	})
}
