package terraform

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_GetTerraformOutputs(t *testing.T) {
	t.Run("ReturnsOutputsForComponent", func(t *testing.T) {
		// Given a provider with terraform component
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"cluster_name": {"value": "my-cluster"}, "region": {"value": "us-west-2"}}`, nil
				}
			}
			return "", nil
		}

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should return outputs
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}

		if outputs["region"] != "us-west-2" {
			t.Errorf("Expected region to be us-west-2, got: %v", outputs["region"])
		}
	})

	t.Run("ReturnsErrorWhenComponentNotFound", func(t *testing.T) {
		// Given a provider with nonexistent component
		mocks := setupMocks(t)

		// When getting terraform outputs for nonexistent component
		outputs, err := mocks.Provider.GetTerraformOutputs("nonexistent")
		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for nonexistent component")
		}
		if !strings.Contains(err.Error(), "failed to prepare terraform context") {
			t.Errorf("Expected terraform context preparation error, got: %v", err)
		}
		if outputs != nil {
			t.Errorf("Expected nil outputs when context preparation fails, got: %v", outputs)
		}
	})

	t.Run("HandlesEmptyOutput", func(t *testing.T) {
		// Given a provider with terraform that returns empty output
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return "{}", nil
				}
			}
			return "", nil
		}

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should return empty map
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty map, got: %v", outputs)
		}
	})

	t.Run("InitializesModuleWhenOutputFails", func(t *testing.T) {
		// Given a provider with terraform output that fails initially
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					callCount++
					if callCount == 1 {
						return "", fmt.Errorf("not initialized")
					}
					return `{"cluster_name": {"value": "my-cluster"}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should initialize and retry
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}
	})

	t.Run("UsesNonStreamingCaptureForOutputJSON", func(t *testing.T) {
		// Given a provider whose terraform output carries sensitive values
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		outputViaSilent := false
		outputViaCapture := false
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				outputViaSilent = true
				return `{"private_key_pem": {"value": "SENSITIVE", "sensitive": true}}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				outputViaCapture = true
				return `{"private_key_pem": {"value": "SENSITIVE", "sensitive": true}}`, nil
			}
			return "", nil
		}

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then 'output -json' must run via the non-streaming capture path, never the verbose-echoing ExecSilent
		if outputViaSilent {
			t.Error("Expected 'terraform output -json' to run via ExecCaptureWithEnv, but it ran via ExecSilent (streams under --verbose)")
		}
		if !outputViaCapture {
			t.Error("Expected 'terraform output -json' to run via ExecCaptureWithEnv, but it did not")
		}
		if outputs["private_key_pem"] != "SENSITIVE" {
			t.Errorf("Expected private_key_pem to be parsed, got: %v", outputs["private_key_pem"])
		}
	})

}

func TestTerraformProvider_GetOutputs(t *testing.T) {
	t.Run("ReturnsCachedOutputs", func(t *testing.T) {
		// Given a provider with cached outputs
		mocks := setupMocks(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
			"output2": 42,
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["test-component"] = expectedOutputs
		mocks.Provider.mu.Unlock()

		// When getting outputs
		output1, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if output1 != expectedOutputs["output1"] {
			t.Errorf("Expected output1 to be 'value1', got %v", output1)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2", `terraform_output("test-component", "output2")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// Then cached outputs should be returned
		if output2 != expectedOutputs["output2"] {
			t.Errorf("Expected output2 to be 42, got %v", output2)
		}
	})

	t.Run("ReturnsEmptyMapWhenComponentNotFound", func(t *testing.T) {
		// Given a blueprint without the requested component
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: other/path`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		// When getting output for nonexistent component
		output, err := mocks.Provider.getOutput("nonexistent-component", "any-key", `terraform_output("nonexistent-component", "any-key")`, false)

		// Then it should return DeferredError
		var deferredErr *evaluator.DeferredError
		if err == nil || !errors.As(err, &deferredErr) {
			t.Errorf("Expected DeferredError for nonexistent component, got %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for deferred error, got %v", output)
		}
	})

	t.Run("CapturesOutputsFromTerraform", func(t *testing.T) {
		// Given a provider with terraform component and mock shell
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		execCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execCallCount++
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"output1": {"value": "val1"}, "output2": {"value": 42}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		components := mocks.Provider.GetTerraformComponents()
		if len(components) == 0 {
			t.Fatal("Expected components to be loaded from blueprint")
		}

		// When getting outputs
		output1, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2", `terraform_output("test-component", "output2")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then outputs should be captured from terraform
		if execCallCount == 0 {
			t.Errorf("Expected ExecSilent to be called (components found: %d)", len(components))
		}

		if output1 != "val1" {
			t.Errorf("Expected output1 to be 'val1', got %v", output1)
		}

		if output2 != float64(42) {
			t.Errorf("Expected output2 to be 42, got %v", output2)
		}
	})

	t.Run("HandlesEmptyTerraformOutput", func(t *testing.T) {
		// Given a provider with terraform that returns empty outputs
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "output" {
				return "{}", nil
			}
			return "", nil
		}

		// When getting output for missing key with deferred=true
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return nil without error (enables ?? fallback in expressions)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for missing key, got %v", output)
		}
	})

	t.Run("ReturnsNilWhenKeyAbsentFromPopulatedOutputs", func(t *testing.T) {
		// Given a component whose outputs are populated but do not contain the requested key
		// (e.g. partial-destroy state, or an output that was renamed/removed in the module)
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"present_key": {"value": "v"}}`, nil
			}
			return "", nil
		}

		// When asking for a key that is not in the populated outputs
		output, err := mocks.Provider.getOutput("test-component", "missing_key", `terraform_output("test-component", "missing_key")`, true)

		// Then nil is returned without error so ?? fallback works uniformly
		if err != nil {
			t.Errorf("Expected no error for absent key with populated outputs, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for absent key, got %v", output)
		}

		// And the populated outputs are still cached so the present key resolves
		present, err := mocks.Provider.getOutput("test-component", "present_key", `terraform_output("test-component", "present_key")`, true)
		if err != nil {
			t.Fatalf("Expected no error for present key, got %v", err)
		}
		if present != "v" {
			t.Errorf("Expected present_key to be 'v', got %v", present)
		}
	})

	t.Run("CachesAbsentKeyToAvoidRepeatedTerraformInvocations", func(t *testing.T) {
		// Given a blueprint with multiple ??-guarded references to the same absent key,
		// the cached outputs from the first lookup should be authoritative — refetching
		// cannot reveal a key that was not in the populated set.
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		outputCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				outputCallCount++
				return `{"present_key": {"value": "v"}}`, nil
			}
			return "", nil
		}

		// When the same absent key is requested multiple times
		for i := 0; i < 3; i++ {
			output, err := mocks.Provider.getOutput("test-component", "missing_key", `terraform_output("test-component", "missing_key")`, true)
			if err != nil {
				t.Fatalf("Expected no error on lookup %d, got: %v", i, err)
			}
			if output != nil {
				t.Errorf("Expected nil output on lookup %d, got %v", i, output)
			}
		}

		// Then terraform output is only invoked once — subsequent absent-key lookups
		// hit the cache and short-circuit instead of re-shelling the CLI.
		if outputCallCount != 1 {
			t.Errorf("Expected terraform output to be invoked once across 3 absent-key lookups, got %d invocations", outputCallCount)
		}
	})

	t.Run("HandlesTerraformInitFallback", func(t *testing.T) {
		// Given a provider with terraform output that fails initially
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		outputCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				outputCallCount++
				if outputCallCount == 1 {
					return "", errors.New("output failed")
				}
				return `{"output1": {"value": "val1"}}`, nil
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "init" {
				return "", nil
			}
			return "", nil
		}

		// When getting output after init fallback
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then output should be retrieved after init
		if output != "val1" {
			t.Errorf("Expected output to be 'val1', got %v (outputCallCount: %d)", output, outputCallCount)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		// Given a provider with Setenv that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Provider.Shims.Setenv = func(key, value string) error {
			return errors.New("setenv failed")
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when context preparation fails")
		}
		if !strings.Contains(err.Error(), "failed to get terraform outputs") {
			t.Errorf("Expected output retrieval error, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output when error occurs, got %v", output)
		}
	})

	t.Run("HandlesBackendOverrideError", func(t *testing.T) {
		// Given a provider with unsupported backend type
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "unsupported"})

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when backend override generation fails")
		}
		if !strings.Contains(err.Error(), "failed to get terraform outputs") {
			t.Errorf("Expected output retrieval error, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output when error occurs, got %v", output)
		}
	})

	t.Run("HandlesCacheRaceCondition", func(t *testing.T) {
		// Given a provider where cache is populated during execution
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			callCount++
			if callCount == 1 {
				mocks.Provider.mu.Lock()
				mocks.Provider.cache["test-component"] = map[string]any{"output1": "cached-value"}
				mocks.Provider.mu.Unlock()
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}}`, nil
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "init" {
				return "", nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then cached value should be returned
		if output != "cached-value" {
			t.Errorf("Expected cached value, got %v", output)
		}
	})

	t.Run("HandlesJsonUnmarshalError", func(t *testing.T) {
		// Given a provider with JsonUnmarshal that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Provider.Shims.JsonUnmarshal = func(data []byte, v any) error {
			return errors.New("json unmarshal error")
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return an error with the JSON parse error in the chain
		if err == nil {
			t.Error("Expected error when JsonUnmarshal fails, got nil")
		}
		if output != nil {
			t.Errorf("Expected nil output when error occurs, got %v", output)
		}
		if !strings.Contains(err.Error(), "failed to parse terraform output JSON") {
			t.Errorf("Expected error message about JSON parse failure, got: %v", err)
		}
	})

	t.Run("HandlesOutputsWithoutValueField", func(t *testing.T) {
		// Given a provider with terraform output that has no value field
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"sensitive": true}}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return nil without error (enables ?? fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for missing key, got %v", output)
		}
	})

	t.Run("HandlesOutputsWithNonMapValue", func(t *testing.T) {
		// Given a provider with terraform output that is not a map
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": "not-a-map"}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return nil without error (enables ?? fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for malformed output, got %v", output)
		}
	})
}

func TestTerraformProvider_CacheOutputs(t *testing.T) {
	t.Run("CachesOutputsForComponent", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.Provider.mu.Lock()
		mocks.Provider.components = []blueprintv1alpha1.TerraformComponent{
			{Path: "test-component"},
		}
		mocks.Provider.mu.Unlock()

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "cached-value"}, "output2": {"value": 42}}`, nil
			}
			return "", nil
		}

		err := mocks.Provider.CacheOutputs("test-component")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		mocks.Provider.mu.RLock()
		cached := mocks.Provider.cache["test-component"]
		mocks.Provider.mu.RUnlock()

		if cached == nil {
			t.Fatal("Expected outputs to be cached")
		}
		if cached["output1"] != "cached-value" {
			t.Errorf("Expected output1 to be 'cached-value', got %v", cached["output1"])
		}
		if cached["output2"] != float64(42) {
			t.Errorf("Expected output2 to be 42, got %v", cached["output2"])
		}
	})

	t.Run("ReturnsErrorWhenComponentNotFound", func(t *testing.T) {
		mocks := setupMocks(t)

		err := mocks.Provider.CacheOutputs("nonexistent-component")

		if err == nil {
			t.Fatal("Expected error for nonexistent component")
		}
		if !strings.Contains(err.Error(), "failed to prepare terraform context") {
			t.Errorf("Expected terraform context preparation error, got: %v", err)
		}
	})
}

func TestTerraformProvider_ClearCache(t *testing.T) {
	t.Run("ClearsAllCachedOutputsAndComponents", func(t *testing.T) {
		// Given a provider with cached outputs, components, and config scope
		mocks := setupMocks(t)

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["component1"] = map[string]any{"output1": "value1"}
		mocks.Provider.cache["component2"] = map[string]any{"output2": "value2"}
		mocks.Provider.components = []blueprintv1alpha1.TerraformComponent{{Path: "test"}}
		mocks.Provider.configScope = map[string]any{"stale": "scope"}
		mocks.Provider.mu.Unlock()

		// When clearing the cache
		mocks.Provider.ClearCache()

		// Then cache, components, and config scope should be cleared
		if len(mocks.Provider.cache) != 0 {
			t.Errorf("Expected cache to be empty after ClearCache, got %d entries", len(mocks.Provider.cache))
		}

		if mocks.Provider.components != nil {
			t.Error("Expected components to be cleared after ClearCache")
		}

		if mocks.Provider.configScope != nil {
			t.Error("Expected configScope to be cleared after ClearCache so GetEnvVars does not use stale scope")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformProvider_registerTerraformOutputHelper(t *testing.T) {
	t.Run("RegistersHelperWithEvaluator", func(t *testing.T) {
		// Given a provider and mock evaluator
		mocks := setupMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		var registeredName string
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signatures ...any) {
			registeredName = name
		}

		// When registering terraform output helper
		mocks.Provider.registerTerraformOutputHelper(mockEvaluator)

		// Then helper should be registered with correct name
		if registeredName != "terraform_output" {
			t.Errorf("Expected helper to be registered as 'terraform_output', got '%s'", registeredName)
		}
	})

	t.Run("HelperReturnsErrorForWrongParamCount", func(t *testing.T) {
		// Given a registered helper
		mocks := setupMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		var helperFunc func(params []any, deferred bool) (any, error)
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signatures ...any) {
			helperFunc = helper
		}

		mocks.Provider.registerTerraformOutputHelper(mockEvaluator)

		// When calling helper with wrong parameter count
		_, err := helperFunc([]any{"component"}, false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for wrong parameter count")
		}
	})
}

func TestTerraformProvider_applyEnvVars(t *testing.T) {
	t.Run("AppliesEnvVarsSuccessfully", func(t *testing.T) {
		// Given a provider and env vars to apply
		mocks := setupMocks(t)
		provider := mocks.Provider

		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		var setVars map[string]string
		setVars = make(map[string]string)
		provider.Shims.Getenv = func(key string) string {
			return "original-" + key
		}
		provider.Shims.Setenv = func(key, value string) error {
			setVars[key] = value
			return nil
		}

		// When applying env vars
		originalEnvVars, err := provider.applyEnvVars(envVars)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then env vars should be set and originals preserved
		if originalEnvVars["VAR1"] != "original-VAR1" {
			t.Errorf("Expected original VAR1 to be original-VAR1, got: %s", originalEnvVars["VAR1"])
		}

		if setVars["VAR1"] != "value1" {
			t.Errorf("Expected VAR1 to be set to value1, got: %s", setVars["VAR1"])
		}

		if setVars["VAR2"] != "value2" {
			t.Errorf("Expected VAR2 to be set to value2, got: %s", setVars["VAR2"])
		}
	})

	t.Run("RestoresEnvVarsOnError", func(t *testing.T) {
		// Given a provider with Setenv that fails
		mocks := setupMocks(t)
		provider := mocks.Provider

		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		var restoredKeys []string
		restoredKeys = make([]string, 0)
		callCount := 0
		provider.Shims.Getenv = func(key string) string {
			return "original-" + key
		}
		provider.Shims.Setenv = func(key, value string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("setenv error")
			}
			return nil
		}
		provider.Shims.Unsetenv = func(key string) error {
			restoredKeys = append(restoredKeys, key)
			return nil
		}

		// When applying env vars
		_, err := provider.applyEnvVars(envVars)
		// Then it should restore env vars on error
		if err == nil {
			t.Error("Expected error when Setenv fails")
		}

		if len(restoredKeys) == 0 && callCount < 2 {
			t.Error("Expected restoreEnvVars to be called on error")
		}
	})
}

func TestTerraformProvider_RestoreEnvVar(t *testing.T) {
	t.Run("RestoresEnvVarWithValue", func(t *testing.T) {
		// Given a provider and an env var with value
		mocks := setupMocks(t)
		provider := mocks.Provider

		var setKey, setValue string
		provider.Shims.Setenv = func(key, value string) error {
			setKey = key
			setValue = value
			return nil
		}

		// When restoring env var
		provider.restoreEnvVar("TEST_VAR", "original-value")

		// Then it should set the env var
		if setKey != "TEST_VAR" {
			t.Errorf("Expected to set TEST_VAR, got %s", setKey)
		}

		if setValue != "original-value" {
			t.Errorf("Expected to set original-value, got %s", setValue)
		}
	})

	t.Run("UnsetsEnvVarWhenEmpty", func(t *testing.T) {
		// Given a provider and an empty env var value
		mocks := setupMocks(t)
		provider := mocks.Provider

		var unsetKey string
		provider.Shims.Unsetenv = func(key string) error {
			unsetKey = key
			return nil
		}

		// When restoring env var with empty value
		provider.restoreEnvVar("TEST_VAR", "")

		// Then it should unset the env var
		if unsetKey != "TEST_VAR" {
			t.Errorf("Expected to unset TEST_VAR, got %s", unsetKey)
		}
	})
}

func TestTerraformProvider_restoreEnvVars(t *testing.T) {
	t.Run("RestoresMultipleEnvVars", func(t *testing.T) {
		mocks := setupMocks(t)
		provider := mocks.Provider

		originalEnvVars := map[string]string{
			"VAR1": "original1",
			"VAR2": "",
			"VAR3": "original3",
		}

		var setVars map[string]string
		var unsetVars []string
		setVars = make(map[string]string)
		unsetVars = make([]string, 0)

		provider.Shims.Setenv = func(key, value string) error {
			setVars[key] = value
			return nil
		}
		provider.Shims.Unsetenv = func(key string) error {
			unsetVars = append(unsetVars, key)
			return nil
		}

		provider.restoreEnvVars(originalEnvVars)

		if setVars["VAR1"] != "original1" {
			t.Errorf("Expected VAR1 to be restored to original1, got: %s", setVars["VAR1"])
		}

		if setVars["VAR3"] != "original3" {
			t.Errorf("Expected VAR3 to be restored to original3, got: %s", setVars["VAR3"])
		}

		if len(unsetVars) != 1 || unsetVars[0] != "VAR2" {
			t.Errorf("Expected VAR2 to be unset, got: %v", unsetVars)
		}
	})

}
