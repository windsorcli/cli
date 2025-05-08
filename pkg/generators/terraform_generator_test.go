package generators

import (
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/zclconf/go-cty/cty"
)

func TestConvertToCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected cty.Value
	}{
		{
			name:     "String",
			input:    "test",
			expected: cty.StringVal("test"),
		},
		{
			name:     "Int",
			input:    42,
			expected: cty.NumberIntVal(42),
		},
		{
			name:     "Float64",
			input:    42.5,
			expected: cty.NumberFloatVal(42.5),
		},
		{
			name:     "Bool",
			input:    true,
			expected: cty.BoolVal(true),
		},
		{
			name:     "EmptyList",
			input:    []any{},
			expected: cty.ListValEmpty(cty.DynamicPseudoType),
		},
		{
			name:     "List",
			input:    []any{"item1", "item2"},
			expected: cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
		},
		{
			name:     "Map",
			input:    map[string]any{"key": "value"},
			expected: cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("value")}),
		},
		{
			name:     "Unsupported",
			input:    struct{}{},
			expected: cty.NilVal,
		},
		{
			name:     "Nil",
			input:    nil,
			expected: cty.NilVal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToCtyValue(tt.input)
			if !result.RawEquals(tt.expected) {
				t.Errorf("expected %#v, got %#v", tt.expected, result)
			}
		})
	}
}

func TestConvertFromCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    cty.Value
		expected any
	}{
		{
			name:     "String",
			input:    cty.StringVal("test"),
			expected: "test",
		},
		{
			name:     "Int",
			input:    cty.NumberIntVal(42),
			expected: int64(42),
		},
		{
			name:     "Float",
			input:    cty.NumberFloatVal(42.5),
			expected: float64(42.5),
		},
		{
			name:     "Bool",
			input:    cty.BoolVal(true),
			expected: true,
		},
		{
			name:     "List",
			input:    cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
			expected: []any{"item1", "item2"},
		},
		{
			name:     "EmptyList",
			input:    cty.ListValEmpty(cty.String),
			expected: []any(nil),
		},
		{
			name:     "Map",
			input:    cty.MapVal(map[string]cty.Value{"key": cty.StringVal("value")}),
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "EmptyMap",
			input:    cty.MapValEmpty(cty.String),
			expected: map[string]any{},
		},
		{
			name:     "Object",
			input:    cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("value")}),
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "Null",
			input:    cty.NullVal(cty.String),
			expected: nil,
		},
		{
			name:     "Unknown",
			input:    cty.UnknownVal(cty.String),
			expected: nil,
		},
		{
			name:     "Set",
			input:    cty.SetVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
			expected: []any{"item1", "item2"},
		},
		{
			name:     "Tuple",
			input:    cty.TupleVal([]cty.Value{cty.StringVal("item1"), cty.NumberIntVal(42)}),
			expected: []any{"item1", int64(42)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFromCtyValue(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %#v (%T), got %#v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestWriteVariableSensitive(t *testing.T) {
	// Given a body and variables with a sensitive variable
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "test_var",
			Description: "Test variable",
			Sensitive:   true,
		},
	}

	// When writeVariable is called
	writeVariable(body, "test_var", "secret", variables)

	// Then the variable should be commented out with (sensitive)
	expected := `// Test variable
// test_var = "(sensitive)"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteVariableNonSensitive(t *testing.T) {
	// Given a body and variables with a non-sensitive variable
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "test_var",
			Description: "Test variable",
			Sensitive:   false,
		},
	}

	// When writeVariable is called
	writeVariable(body, "test_var", "value", variables)

	// Then the variable should be written with its value
	expected := `// Test variable
test_var = "value"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteVariableWithComment(t *testing.T) {
	// Given a body and variables with a variable with comment
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "test_var",
			Description: "Test variable description",
		},
	}

	// When writeVariable is called
	writeVariable(body, "test_var", "value", variables)

	// Then the variable should be written with its comment
	expected := `// Test variable description
test_var = "value"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteComponentValues(t *testing.T) {
	// Given a body and variables with component values
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "var1",
			Description: "Variable 1",
			Sensitive:   true,
			Default:     "default1",
		},
		{
			Name:        "var2",
			Description: "Variable 2",
			Sensitive:   false,
			Default:     "default2",
		},
		{
			Name:        "var3",
			Description: "Variable 3",
			Default:     "default3",
		},
	}
	values := map[string]any{
		"var2": "pinned_value",
	}
	protectedValues := map[string]bool{}

	// When writeComponentValues is called
	writeComponentValues(body, values, protectedValues, variables)

	// Then the variables should be written in order with proper handling of sensitive values
	expected := `
// Variable 1
// var1 = "(sensitive)"

// Variable 2
var2 = "pinned_value"

// Variable 3
// var3 = "default3"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteDefaultValues(t *testing.T) {
	// Given a body and variables with default values
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "var1",
			Description: "Variable 1",
			Sensitive:   true,
			Default:     "default1",
		},
		{
			Name:        "var2",
			Description: "Variable 2",
			Sensitive:   false,
			Default:     "default2",
		},
		{
			Name:        "var3",
			Description: "Variable 3",
			Default:     "default3",
		},
	}

	// When writeDefaultValues is called
	writeDefaultValues(body, variables, nil)

	// Then the variables should be written in order with proper handling of sensitive values
	expected := `
// Variable 1
// var1 = "(sensitive)"

// Variable 2
// var2 = "default2"

// Variable 3
// var3 = "default3"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteVariableYAML(t *testing.T) {
	// Given a body and variables with a YAML multiline value
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "worker_config_patches",
			Description: "Worker configuration patches",
		},
	}

	// When writeVariable is called with a YAML multiline string
	yamlValue := `machine:
  kubelet:
    extraMounts:
    - destination: /var/local
      options:
      - rbind
      - rw
      source: /var/local
      type: bind`
	writeVariable(body, "worker_config_patches", yamlValue, variables)

	// Then the variable should be written as a heredoc with valid YAML
	actual := string(file.Bytes())

	// Extract the YAML content from the heredoc
	lines := strings.Split(actual, "\n")
	var yamlContent strings.Builder
	inYAML := false
	for _, line := range lines {
		if strings.Contains(line, "<<EOF") {
			inYAML = true
			continue
		}
		if strings.Contains(line, "EOF") {
			inYAML = false
			continue
		}
		if inYAML {
			yamlContent.WriteString(line + "\n")
		}
	}

	// Parse both expected and actual YAML
	var expectedData, actualData any
	expectedYAML := `machine:
  kubelet:
    extraMounts:
    - destination: /var/local
      options:
      - rbind
      - rw
      source: /var/local
      type: bind`

	if err := yaml.UnmarshalWithOptions([]byte(expectedYAML), &expectedData); err != nil {
		t.Fatalf("failed to parse expected YAML: %v", err)
	}
	if err := yaml.UnmarshalWithOptions([]byte(yamlContent.String()), &actualData); err != nil {
		t.Fatalf("failed to parse actual YAML: %v", err)
	}

	// Compare the YAML data structures
	var expectedBytes, actualBytes []byte
	expectedBytes, _ = yaml.MarshalWithOptions(expectedData)
	actualBytes, _ = yaml.MarshalWithOptions(actualData)
	if string(expectedBytes) != string(actualBytes) {
		t.Errorf("YAML content does not match.\nExpected:\n%s\nGot:\n%s", expectedBytes, actualBytes)
	}

	// Verify the comment and heredoc syntax
	if !strings.Contains(actual, "// Worker configuration patches") {
		t.Error("missing description comment")
	}
	if !strings.Contains(actual, "worker_config_patches = <<EOF") {
		t.Error("missing heredoc start")
	}
	if !strings.Contains(actual, "EOF") {
		t.Error("missing heredoc end")
	}
}

func TestWriteVariable(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name: "multiline string",
			value: `first line
  indented line
    more indented
last line`,
			expected: `first line
  indented line
    more indented
last line`,
		},
		{
			name: "multiline with tabs",
			value: `first line
	tabbed line
		more tabbed
last line`,
			expected: `first line
	tabbed line
		more tabbed
last line`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := hclwrite.NewEmptyFile()
			writeVariable(file.Body(), "test_var", tt.value, nil)

			// Extract the heredoc content
			content := string(file.Bytes())
			lines := strings.Split(content, "\n")
			var actualLines []string
			inHeredoc := false
			for _, line := range lines {
				if strings.Contains(line, "<<EOF") {
					inHeredoc = true
					continue
				}
				if strings.Contains(line, "EOF") {
					break
				}
				if inHeredoc {
					actualLines = append(actualLines, line)
				}
			}
			actual := strings.Join(actualLines, "\n")

			// Compare the exact strings to ensure whitespace is preserved
			if actual != tt.expected {
				t.Errorf("content does not match.\nExpected:\n%s\nGot:\n%s", tt.expected, actual)
			}
		})
	}
}

func TestParseVariablesFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("AllVariableTypes", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return variables with all types and attributes
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`
variable "string_var" {
  description = "String variable"
  type        = string
  default     = "default_value"
  sensitive   = false
}

variable "number_var" {
  description = "Number variable"
  type        = number
  default     = 42
  sensitive   = false
}

variable "bool_var" {
  description = "Boolean variable"
  type        = bool
  default     = true
  sensitive   = true
}

variable "list_var" {
  description = "List variable"
  type        = list(string)
  default     = ["item1", "item2"]
}

variable "map_var" {
  description = "Map variable"
  type        = map(string)
  default     = { key = "value" }
}

variable "no_default" {
  description = "Variable without default"
  type        = string
}

variable "no_description" {
  type    = string
  default = "value"
}

variable "invalid_default" {
  description = "Variable with invalid default"
  type        = string
  default     = invalid
}

variable "invalid_sensitive" {
  description = "Variable with invalid sensitive"
  type        = string
  sensitive   = invalid
}`), nil
		}

		// When parseVariablesFile is called
		variables, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And all variables should be parsed correctly
		expectedVars := map[string]VariableInfo{
			"string_var": {
				Name:        "string_var",
				Description: "String variable",
				Default:     "default_value",
				Sensitive:   false,
			},
			"number_var": {
				Name:        "number_var",
				Description: "Number variable",
				Default:     int64(42),
				Sensitive:   false,
			},
			"bool_var": {
				Name:        "bool_var",
				Description: "Boolean variable",
				Default:     true,
				Sensitive:   true,
			},
			"list_var": {
				Name:        "list_var",
				Description: "List variable",
				Default:     []any{"item1", "item2"},
			},
			"map_var": {
				Name:        "map_var",
				Description: "Map variable",
				Default:     map[string]any{"key": "value"},
			},
			"no_default": {
				Name:        "no_default",
				Description: "Variable without default",
			},
			"no_description": {
				Name:    "no_description",
				Default: "value",
			},
			"invalid_default": {
				Name:        "invalid_default",
				Description: "Variable with invalid default",
			},
			"invalid_sensitive": {
				Name:        "invalid_sensitive",
				Description: "Variable with invalid sensitive",
			},
		}

		// Verify each variable
		if len(variables) != len(expectedVars) {
			t.Errorf("expected %d variables, got %d", len(expectedVars), len(variables))
		}

		for _, v := range variables {
			expected, exists := expectedVars[v.Name]
			if !exists {
				t.Errorf("unexpected variable %s", v.Name)
				continue
			}

			if v.Description != expected.Description {
				t.Errorf("variable %s: expected description %q, got %q", v.Name, expected.Description, v.Description)
			}
			if !reflect.DeepEqual(v.Default, expected.Default) {
				t.Errorf("variable %s: expected default %v (%T), got %v (%T)", v.Name, expected.Default, expected.Default, v.Default, v.Default)
			}
			if v.Sensitive != expected.Sensitive {
				t.Errorf("variable %s: expected sensitive %v, got %v", v.Name, expected.Sensitive, v.Sensitive)
			}
		}
	})

	t.Run("ProtectedVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return variables
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`
variable "protected_var" {
  description = "Protected variable"
  type        = string
}

variable "normal_var" {
  description = "Normal variable"
  type        = string
}`), nil
		}

		// And protected values are set
		protectedValues := map[string]bool{
			"protected_var": true,
		}

		// When parseVariablesFile is called
		variables, err := generator.parseVariablesFile("test.tf", protectedValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And only non-protected variables should be included
		if len(variables) != 1 {
			t.Errorf("expected 1 variable, got %d", len(variables))
		}
		if variables[0].Name != "normal_var" {
			t.Errorf("expected variable normal_var, got %s", variables[0].Name)
		}
	})

	t.Run("InvalidHCL", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return invalid HCL
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`invalid hcl content`), nil
		}

		// When parseVariablesFile is called
		_, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("read error")
		}

		// When parseVariablesFile is called
		_, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got nil")
		}
		expectedError := "failed to read variables.tf: read error"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{component}
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get project root: error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create terraform directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get config root: mock error getting config root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_generateModuleShim(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create module directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorChdir", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And Chdir is mocked to return an error
		mocks.Shims.Chdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to change to module directory: mock error changing directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorSetenv", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And Setenv is mocked to return an error
		mocks.Shims.Setenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to set TF_DATA_DIR: mock error setting environment variable"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorExecSilent", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return an error
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "", fmt.Errorf("mock error running terraform init")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to initialize terraform: mock error running terraform init"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get config root: mock error getting config root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return an error for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return nil, fmt.Errorf("mock error reading variables.tf")
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read variables.tf: mock error reading variables.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingMainTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for main.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return fmt.Errorf("mock error writing main.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write main.tf: mock error writing main.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingVariablesTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for variables.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				return fmt.Errorf("mock error writing variables.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write shim variables.tf: mock error writing variables.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingOutputsTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return "Initializing modules...\n- main in /path/to/module", nil
		}

		// And ReadFile is mocked to return content for variables.tf and outputs.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			if strings.HasSuffix(path, "outputs.tf") {
				return []byte(`output "test" {
  value = "test"
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And Stat is mocked to return success for outputs.tf
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.HasSuffix(path, "outputs.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And WriteFile is mocked to return an error for outputs.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "outputs.tf") {
				return fmt.Errorf("mock error writing outputs.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write shim outputs.tf: mock error writing outputs.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_writeModuleFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorReadFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read variables.tf: mock error reading file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "mock error writing file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_writeTfvarsFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckExistingTfvarsFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path: "module/path1",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return an error
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("mock error checking existing tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error checking tfvars file: mock error checking existing tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path: "module/path1",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("mock error reading existing tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read existing tfvars file: mock error reading existing tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteComponentValues", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "module/path1",
			FullPath: "original/full/path",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return not exist
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for component values
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing component values")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing component values"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFileAfterValues", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "module/path1",
			FullPath: "original/full/path",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return not exist
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing final tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing final tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_checkExistingTfvarsFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return not found
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("FileExists", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte("test content"), nil
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then os.ErrExist should be returned
		if err != os.ErrExist {
			t.Errorf("expected os.ErrExist, got %v", err)
		}
	})

	t.Run("ErrorReadingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read existing tfvars file: mock error reading file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_addTfvarsHeader(t *testing.T) {
	t.Run("WithSource", func(t *testing.T) {
		// Given a body and source
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		source := "fake-source"

		// When addTfvarsHeader is called
		addTfvarsHeader(body, source)

		// Then the header should be written with source
		expected := `// Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
// Module source: fake-source
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("WithoutSource", func(t *testing.T) {
		// Given a body without source
		file := hclwrite.NewEmptyFile()
		body := file.Body()

		// When addTfvarsHeader is called
		addTfvarsHeader(body, "")

		// Then the header should be written without source
		expected := `// Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})
}
