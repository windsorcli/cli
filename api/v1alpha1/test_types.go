// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

// TestFile represents a test file containing multiple test cases for blueprint composition validation.
// Test files are stored in contexts/_template/tests/*.test.yaml and define input values
// along with expected and excluded blueprint components.
type TestFile struct {
	// Cases is a list of test cases to execute.
	Cases []TestCase `yaml:"cases"`
}

// TestCase represents a single test case with input values and expected output.
// Each test case applies the specified values to configuration and validates
// the resulting composed blueprint against expectations.
type TestCase struct {
	// Name is the unique identifier for this test case.
	Name string `yaml:"name"`

	// Values are configuration values to apply before composing the blueprint.
	// These override any existing configuration for the test.
	Values map[string]any `yaml:"values,omitempty"`

	// TerraformOutputs provides mock terraform outputs for terraform_output() expressions.
	// Keys are component IDs, values are maps of output key-value pairs.
	// Example: {"network": {"vpc_id": "vpc-123", "subnet_ids": ["subnet-1", "subnet-2"]}}
	TerraformOutputs map[string]map[string]any `yaml:"terraformOutputs,omitempty"`

	// Expect defines components that must be present in the composed blueprint.
	// Uses partial matching: only specified fields are checked.
	// Kind, ApiVersion, and Metadata are ignored for matching purposes.
	Expect *Blueprint `yaml:"expect,omitempty"`

	// Exclude defines components that must NOT be present in the composed blueprint.
	// Uses partial matching: only specified fields are checked.
	// Kind, ApiVersion, and Metadata are ignored for matching purposes.
	Exclude *Blueprint `yaml:"exclude,omitempty"`

	// ExpectError indicates that blueprint composition is expected to fail with an error.
	// If true, the test passes when composition fails and fails when composition succeeds.
	// This is useful for testing invalid configurations that should be rejected by the framework.
	ExpectError bool `yaml:"expectError,omitempty"`
}
