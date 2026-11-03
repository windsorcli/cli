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
// Expect and Exclude match partially: only the fields they set are checked, and Kind,
// ApiVersion, and Metadata are ignored. env() sees only Env, never the host environment.
// Composition skips schema defaults unless ApplySchemaDefaults is set.
type TestCase struct {
	// Name is the unique identifier for this test case.
	Name string `yaml:"name"`

	// Values are configuration values that override the existing configuration before composition.
	Values map[string]any `yaml:"values,omitempty"`

	// Env provides the only environment variables that env() expressions see during composition.
	Env map[string]string `yaml:"env,omitempty"`

	// TerraformOutputs maps component IDs to mock outputs for terraform_output() expressions.
	TerraformOutputs map[string]map[string]any `yaml:"terraformOutputs,omitempty"`

	// Expect defines components that must be present in the composed blueprint.
	Expect *Blueprint `yaml:"expect,omitempty"`

	// Exclude defines components that must not be present in the composed blueprint.
	Exclude *Blueprint `yaml:"exclude,omitempty"`

	// ExpectError makes the case pass only when composition fails.
	ExpectError bool `yaml:"expectError,omitempty"`

	// ApplySchemaDefaults composes this case with schema default: values materialized, as production does.
	ApplySchemaDefaults bool `yaml:"applySchemaDefaults,omitempty"`
}
