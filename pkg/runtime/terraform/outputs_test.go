package terraform_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
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
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "terraform component") {
			t.Errorf("Expected a remediation that names the terraform component, got: %v", we)
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
