package terraform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_GetEnvVars(t *testing.T) {
	t.Run("OnlyIncludesOutputsAccessedViaTerraformOutput", func(t *testing.T) {
		// Given a blueprint with component inputs using terraform_output
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'`

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "isolated_subnet_ids": {"value": ["subnet-1"]}, "private_subnet_ids": {"value": ["subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then only outputs accessed via terraform_output should be included
		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		unexpectedVars := []string{
			"TF_VAR_isolated_subnet_ids",
			"TF_VAR_private_subnet_ids",
			"TF_VAR_public_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			// All outputs are cached when fetched, even if not accessed via terraform_output()
			if isolatedSubnets, exists := networkCache["isolated_subnet_ids"]; !exists {
				t.Error("Expected isolated_subnet_ids to be cached (all outputs are cached when fetched)")
			} else {
				// Verify it's the actual value, not a deferred expression
				expected := []any{"subnet-1"}
				if fmt.Sprintf("%v", isolatedSubnets) != fmt.Sprintf("%v", expected) {
					t.Errorf("Expected isolated_subnet_ids to be cached as actual value, got: %v", isolatedSubnets)
				}
			}
		}
	})

	t.Run("AccumulatesMultipleRequestedOutputs", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'
      subnet_ids: '${terraform_output("network", "private_subnet_ids")}'`

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "private_subnet_ids": {"value": ["subnet-1", "subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}, "isolated_subnet_ids": {"value": ["subnet-4"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		expectedSubnetIds := `["subnet-1","subnet-2"]`
		if envVars["TF_VAR_subnet_ids"] != expectedSubnetIds {
			t.Errorf("Expected TF_VAR_subnet_ids to be %s, got: %s", expectedSubnetIds, envVars["TF_VAR_subnet_ids"])
		}

		unexpectedVars := []string{
			"TF_VAR_public_subnet_ids",
			"TF_VAR_isolated_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			if str, isString := networkCache["private_subnet_ids"].(string); isString && strings.HasPrefix(str, "terraform_output(") {
				t.Errorf("Expected private_subnet_ids to be accessed (not deferred), got: %v", networkCache["private_subnet_ids"])
			}
		}
	})

	t.Run("EvaluatesInputsWithContainsExpression", func(t *testing.T) {
		// Given a blueprint with inputs containing expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			result := make(map[string]any)
			for key, value := range values {
				if strVal, ok := value.(string); ok && strings.Contains(strVal, "${") {
					result[key] = "evaluated-vpc-id"
				} else {
					result[key] = value
				}
			}
			return result, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then inputs with expressions should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "evaluated-vpc-id" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'evaluated-vpc-id', got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("SkipsTFVarForInputsWithoutExpressions", func(t *testing.T) {
		// Given a blueprint with inputs without expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      plain_value: "plain"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-expression inputs should not be emitted as TF_VAR_*
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_cluster_name"]; exists {
			t.Error("Expected TF_VAR_cluster_name to be skipped for non-expression input")
		}
		if _, exists := envVars["TF_VAR_plain_value"]; exists {
			t.Error("Expected TF_VAR_plain_value to be skipped for non-expression input")
		}
	})

	t.Run("IncludesBaseContextAndProjectRootVars", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "my-context"
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/my/project", nil
		}

		envVars, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_context"] != "my-context" {
			t.Errorf("Expected TF_VAR_context to be 'my-context', got %q", envVars["TF_VAR_context"])
		}
		if envVars["TF_VAR_project_root"] != "/my/project" {
			t.Errorf("Expected TF_VAR_project_root to be '/my/project', got %q", envVars["TF_VAR_project_root"])
		}
	})

	t.Run("HandlesComponentNotFoundWithProjectRoot", func(t *testing.T) {
		// Given a provider with project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		// When getting env vars for nonexistent component
		envVars, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return empty env vars without error
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars == nil {
			t.Error("Expected envVars to be non-nil")
		}
	})

	t.Run("HandlesComponentNotFoundWithoutProjectRoot", func(t *testing.T) {
		// Given a provider without project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		// When getting env vars for nonexistent component
		_, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when component not found and project root unavailable")
		}
	})

	t.Run("HandlesEvaluateMapError", func(t *testing.T) {
		// Given a provider with evaluator that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, errors.New("evaluation error")
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when EvaluateMap fails")
		}
	})

	t.Run("HandlesNonStringEvaluatedValue", func(t *testing.T) {
		// Given a provider with evaluator that returns non-string value
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			return map[string]any{
				"vpc_id": 42,
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`"42"`), nil
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-string values should be JSON marshaled
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != `"42"` {
			t.Errorf("Expected TF_VAR_vpc_id to be JSON marshaled, got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		// Given a provider with JsonMarshal that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			return map[string]any{
				"vpc_id": make(chan int),
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then marshal error should be skipped and var not set
		if err != nil {
			t.Fatalf("Expected no error (marshal error is skipped), got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when JSON marshal fails")
		}
	})

	t.Run("HandlesGenerateTerraformArgsError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when GenerateTerraformArgs fails")
		}
		if !strings.Contains(err.Error(), "error generating terraform args") {
			t.Errorf("Expected error to mention generating terraform args, got: %v", err)
		}
	})

	t.Run("HandlesGetBaseEnvVarsForComponentError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// Only getBaseEnvVarsForComponent calls GetProjectRoot in this flow.
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when getBaseEnvVarsForComponent fails")
		}
		if !strings.Contains(err.Error(), "project root") {
			t.Errorf("Expected error to mention project root, got: %v", err)
		}
	})

	t.Run("HandlesEvaluatedKeyNotExist", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when key doesn't exist in evaluated result")
		}
	})

	t.Run("SkipsTFVarForNonRuntimeScopeExpressions", func(t *testing.T) {
		// Given a provider with config scope and component input referencing scope
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextFunc = func() string { return "default" }
		configHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }
		configHandler.GetWindsorScratchPathFunc = func() (string, error) { return "/test/scratch", nil }
		configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BackendType: "none", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		patches := []any{map[string]any{"op": "add", "path": "/machine/foo"}}
		scope := map[string]any{
			"some_block": map[string]any{
				"patchVars": map[string]any{
					"common_config_patches": patches,
				},
			},
		}
		mocks.Provider.SetConfigScope(scope)
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"common_config_patches": "${some_block.patchVars.common_config_patches}",
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-runtime expressions should remain tfvars-only and not emit TF_VAR
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_common_config_patches"]; exists {
			t.Fatal("Expected TF_VAR_common_config_patches to be skipped for non-runtime scope expression")
		}
	})

	t.Run("SkipsTFVarForResolvedInputsWithoutExpressions", func(t *testing.T) {
		// Given a provider with component having resolved (non-expression) input
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		patches := []any{map[string]any{"op": "add", "path": "/machine/bar"}}
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"common_config_patches": patches,
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-expression input should remain tfvars-only and not emit TF_VAR
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_common_config_patches"]; exists {
			t.Fatal("Expected TF_VAR_common_config_patches to be skipped for resolved non-expression input")
		}
	})

	t.Run("SkipsTFVarForSimpleExpression", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      talos_version: "${config.kubernetes.version}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_talos_version"]; exists {
			t.Fatal("Expected TF_VAR_talos_version to be skipped for simple expression")
		}
	})

	t.Run("AllowsRuntimeDeferredExpression", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      talos_version: "${terraform_output('bootstrap', 'talos_version')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if evaluateDeferred {
				return map[string]any{"talos_version": "v1.12.6"}, nil
			}
			return map[string]any{"talos_version": "${terraform_output('bootstrap', 'talos_version')}"}, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if envVars["TF_VAR_talos_version"] != "v1.12.6" {
			t.Fatalf("Expected runtime deferred TF_VAR_talos_version to be exported, got %q", envVars["TF_VAR_talos_version"])
		}
	})

	t.Run("DoesNotSerializeDeferredValueWrapperIntoTFVar", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"network_cidr": evaluator.DeferredValue{
					Expression: "${terraform_output(\"workstation\", \"network_cidr\")}",
				},
				"network_name": &evaluator.DeferredValue{
					Expression: "${terraform_output(\"workstation\", \"network_name\")}",
				},
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			out := make(map[string]any, len(values))
			for key, value := range values {
				out[key] = value
			}
			return out, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		for key, value := range envVars {
			if strings.HasPrefix(key, "TF_VAR_") && strings.HasPrefix(value, "{\"Expression\":") {
				t.Fatalf("Expected TF_VAR to never serialize DeferredValue wrapper, got %s=%s", key, value)
			}
		}

		if got := envVars["TF_VAR_network_cidr"]; got != "${terraform_output(\"workstation\", \"network_cidr\")}" {
			t.Fatalf("Expected TF_VAR_network_cidr to contain raw expression string, got %q", got)
		}
		if got := envVars["TF_VAR_network_name"]; got != "${terraform_output(\"workstation\", \"network_name\")}" {
			t.Fatalf("Expected TF_VAR_network_name to contain raw expression string, got %q", got)
		}
	})

	t.Run("SkipsTFVarWhenDeferredOutputResolvesToNil", func(t *testing.T) {
		// Given a component whose input depends on a terraform_output from an unapplied dependency.
		// The evaluator returns nil for the deferred pass (no outputs yet), which must NOT result
		// in TF_VAR_network_cidr being set to the string "null".
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		component := blueprintv1alpha1.TerraformComponent{
			Path: "compute",
			Inputs: map[string]any{
				"network_cidr": "${terraform_output(\"workstation\", \"network_cidr\")}",
			},
		}
		component.FullPath = "/test/project/terraform/compute"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			out := make(map[string]any, len(values))
			for key, value := range values {
				if evaluateDeferred {
					// Dependency not applied yet — resolve to nil.
					out[key] = nil
				} else {
					// First pass: preserve the expression so the caller sees it is deferred.
					out[key] = value
				}
			}
			return out, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then TF_VAR_network_cidr must not be set (not even to the string "null")
		if val, exists := envVars["TF_VAR_network_cidr"]; exists {
			t.Errorf("Expected TF_VAR_network_cidr to be absent when dependency is not applied, got %q", val)
		}
	})

	t.Run("MergesTerraformScopedDotEnvValues", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file with a plain value
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the terraform/.env value should be present
		if envVars["HYPERV_HOST"] != "hyperv.local" {
			t.Errorf("Expected HYPERV_HOST=hyperv.local, got %q", envVars["HYPERV_HOST"])
		}
	})

	t.Run("WarnsOnLooseTerraformScopedDotEnvPermissions", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file with group/world-readable permissions
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0644}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.Goos = func() string { return "linux" }
		var stderr bytes.Buffer
		mocks.Provider.warningWriter = &stderr

		// When GetEnvVars is called
		if _, _, _, err := mocks.Provider.GetEnvVars("compute", false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then a warning should be written to the injected writer
		if !strings.Contains(stderr.String(), "Warning") {
			t.Errorf("Expected a permission warning, got %q", stderr.String())
		}
	})

	t.Run("NoWarningOnRestrictedTerraformScopedDotEnvPermissions", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file restricted to the owner
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.Goos = func() string { return "linux" }
		var stderr bytes.Buffer
		mocks.Provider.warningWriter = &stderr

		// When GetEnvVars is called
		if _, _, _, err := mocks.Provider.GetEnvVars("compute", false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then no warning should be written
		if stderr.String() != "" {
			t.Errorf("Expected no warning, got %q", stderr.String())
		}
	})

	t.Run("ResolvesSecretExpressionInTerraformScopedDotEnv", func(t *testing.T) {
		// Given a terraform/.env file with a secret expression and a mock evaluator
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte(`HYPERV_PASSWORD=${secret("op://vault/item/field")}` + "\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "secret(") {
				return "resolved-secret", nil
			}
			return expression, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the expression should be resolved through the evaluator
		if envVars["HYPERV_PASSWORD"] != "resolved-secret" {
			t.Errorf("Expected resolved-secret, got %q", envVars["HYPERV_PASSWORD"])
		}
	})

	t.Run("ReturnsErrorWhenTerraformScopedDotEnvReadFails", func(t *testing.T) {
		// Given a terraform/.env file that exists but fails to read
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return nil, fmt.Errorf("mock read error")
			}
			return nil, os.ErrNotExist
		}

		// When GetEnvVars is called
		_, _, _, err := mocks.Provider.GetEnvVars("compute", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func Test_sanitizeForK8s(t *testing.T) {
	t.Run("SanitizesBasicString", func(t *testing.T) {
		// Given a string with mixed case and underscores
		// When sanitizing for K8s
		result := sanitizeForK8s("Test-String_123")

		// Then it should convert to lowercase and replace underscores
		expected := "test-string-123"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("TrimsTo63Characters", func(t *testing.T) {
		// Given a long string
		longString := strings.Repeat("a", 100)

		// When sanitizing for K8s
		result := sanitizeForK8s(longString)

		// Then it should be trimmed to 63 characters
		if len(result) > 63 {
			t.Errorf("Expected max 63 characters, got %d: %s", len(result), result)
		}
	})

	t.Run("HandlesMultipleDashes", func(t *testing.T) {
		// Given a string with multiple dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("test---string")

		// Then multiple dashes should be collapsed
		if strings.Contains(result, "---") {
			t.Errorf("Expected multiple dashes to be collapsed, got %s", result)
		}
	})

	t.Run("TrimsLeadingTrailingDashes", func(t *testing.T) {
		// Given a string with leading and trailing dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("-test-string-")

		// Then leading and trailing dashes should be trimmed
		if strings.HasPrefix(result, "-") || strings.HasSuffix(result, "-") {
			t.Errorf("Expected no leading/trailing dashes, got %s", result)
		}
	})
}
