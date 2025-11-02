package generators

import (
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/context/config"
	bundler "github.com/windsorcli/cli/pkg/resources/artifact"
	"github.com/zclconf/go-cty/cty"
)

// =============================================================================
// Test Setup
// =============================================================================

type simpleDirEntry struct {
	name  string
	isDir bool
}

func (s *simpleDirEntry) Name() string {
	return s.name
}

func (s *simpleDirEntry) IsDir() bool {
	return s.isDir
}

func (s *simpleDirEntry) Type() fs.FileMode {
	if s.isDir {
		return fs.ModeDir
	}
	return 0
}

func (s *simpleDirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name  string
	isDir bool
	mode  os.FileMode
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

// mockDirEntry implements os.DirEntry for testing
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string      { return m.name }
func (m *mockDirEntry) IsDir() bool       { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{name: m.name, isDir: m.isDir}, nil
}

// =============================================================================
// Test Private Methods
// =============================================================================

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
		expected := `# Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
# Module source: fake-source
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
		expected := `# Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})
}

func TestTerraformGenerator_parseVariablesFile(t *testing.T) {
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

// =============================================================================
// Test Helpers
// =============================================================================

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
			name:     "StringSliceEmpty",
			input:    []string{},
			expected: cty.ListValEmpty(cty.String),
		},
		{
			name:     "StringSliceNonEmpty",
			input:    []string{"a", "b"},
			expected: cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
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
			name:     "NumberBigFloat",
			input:    cty.NumberVal(big.NewFloat(42.5)),
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
		{
			name:     "UnsupportedType",
			input:    cty.DynamicVal,
			expected: nil,
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

func TestFormatValue(t *testing.T) {
	t.Run("EmptyArray", func(t *testing.T) {
		result := formatValue([]any{})
		if result != "[]" {
			t.Errorf("expected [] got %q", result)
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		result := formatValue(map[string]any{})
		if result != "{}" {
			t.Errorf("expected {} got %q", result)
		}
	})

	t.Run("StringSliceEmpty", func(t *testing.T) {
		result := formatValue([]string{})
		if result != "[]" {
			t.Errorf("expected [] got %q", result)
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		result := formatValue(nil)
		if result != "null" {
			t.Errorf("expected null got %q", result)
		}
	})

	t.Run("ComplexNestedObject", func(t *testing.T) {
		input := map[string]any{
			"node_groups": map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}
		expected := `{
  node_groups = {
    default = {
      desired_size = 2
      instance_types = ["t3.medium"]
      max_size = 3
      min_size = 1
    }
  }
}`
		result := formatValue(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("StringSliceNonEmpty", func(t *testing.T) {
		result := formatValue([]string{"a", "b"})
		if result != "[\"a\", \"b\"]" {
			t.Errorf("expected [\"a\", \"b\"] got %q", result)
		}
	})

	t.Run("EmptyAddons", func(t *testing.T) {
		input := map[string]any{
			"addons": map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}
		expected := `{
  addons = {
    aws-ebs-csi-driver = {}
    aws-efs-csi-driver = {}
    coredns = {}
    eks-pod-identity-agent = {}
    external-dns = {}
    vpc-cni = {}
  }
}`
		result := formatValue(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestWriteComponentValues(t *testing.T) {
	t.Run("BasicComponentValues", func(t *testing.T) {
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
# Variable 1
# var1 = "(sensitive)"

# Variable 2
var2 = "pinned_value"

# Variable 3
# var3 = "default3"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("ComplexDefaultsNodeGroups", func(t *testing.T) {
		// Given a body and variables with complex nested default for node groups
		file := hclwrite.NewEmptyFile()
		variable := VariableInfo{
			Name:        "node_groups",
			Description: "Map of EKS managed node group definitions to create.",
			Default: map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}

		// When writeComponentValues is called
		writeComponentValues(file.Body(), map[string]any{}, map[string]bool{}, []VariableInfo{variable})

		// Then the complex default should be commented out correctly
		expected := `
# Map of EKS managed node group definitions to create.
# node_groups = {
#   default = {
#     desired_size = 2
#     instance_types = ["t3.medium"]
#     max_size = 3
#     min_size = 1
#   }
# }`
		result := string(file.Bytes())
		// Check that every line is commented
		for _, line := range strings.Split(result, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if !strings.HasPrefix(line, "#") {
				t.Errorf("uncommented line found: %q", line)
			}
		}
		// Check that the output matches expected ignoring leading/trailing whitespace
		if strings.TrimSpace(result) != strings.TrimSpace(expected) {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ComplexDefaultsEmptyAddons", func(t *testing.T) {
		// Given a body and variables with complex empty map defaults for addons
		file := hclwrite.NewEmptyFile()
		variable := VariableInfo{
			Name:        "addons",
			Description: "Map of EKS add-ons",
			Default: map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}

		// When writeComponentValues is called
		writeComponentValues(file.Body(), map[string]any{}, map[string]bool{}, []VariableInfo{variable})

		// Then the complex empty map defaults should be commented correctly
		expected := "\n# Map of EKS add-ons\n# addons = {\n#   aws-ebs-csi-driver = {}\n#   aws-efs-csi-driver = {}\n#   coredns = {}\n#   eks-pod-identity-agent = {}\n#   external-dns = {}\n#   vpc-cni = {}\n# }\n"
		result := string(file.Bytes())
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
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

	// When writeComponentValues is called with empty values
	writeComponentValues(body, nil, map[string]bool{}, variables)

	// Then the variables should be written in order with proper handling of sensitive values
	expected := `
# Variable 1
# var1 = "(sensitive)"

# Variable 2
# var2 = "default2"

# Variable 3
# var3 = "default3"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteVariable(t *testing.T) {
	t.Run("SensitiveVariable", func(t *testing.T) {
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
		writeVariable(body, "test_var", "value", variables)

		// Then the variable should be commented out with (sensitive)
		expected := `# Test variable
# test_var = "(sensitive)"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("NonSensitiveVariable", func(t *testing.T) {
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
		expected := `# Test variable
test_var = "value"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("VariableWithComment", func(t *testing.T) {
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
		expected := `# Test variable description
test_var = "value"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("YAMLMultilineValue", func(t *testing.T) {
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
		if !strings.Contains(actual, "# Worker configuration patches") {
			t.Error("missing description comment")
		}
		if !strings.Contains(actual, "worker_config_patches = <<EOF") {
			t.Error("missing heredoc start")
		}
		if !strings.Contains(actual, "EOF") {
			t.Error("missing heredoc end")
		}
	})

	t.Run("MultilineString", func(t *testing.T) {
		// Given a multiline string value
		file := hclwrite.NewEmptyFile()
		value := `first line
  indented line
    more indented
last line`

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the content should be written as heredoc
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
		if actual != value {
			t.Errorf("content does not match.\nExpected:\n%s\nGot:\n%s", value, actual)
		}
	})

	t.Run("MultilineStringWithTabs", func(t *testing.T) {
		// Given a multiline string with tabs
		file := hclwrite.NewEmptyFile()
		value := `first line
	tabbed line
		more tabbed
last line`

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the content should preserve tabs
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
		if actual != value {
			t.Errorf("content does not match.\nExpected:\n%s\nGot:\n%s", value, actual)
		}
	})

	t.Run("MapValue", func(t *testing.T) {
		// Given a map value
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"string": "value",
			"number": 42,
			"bool":   true,
			"nested": map[string]any{
				"key": "value",
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the map should be formatted correctly
		content := string(file.Bytes())
		assignmentPrefix := "test_var = "
		idx := strings.Index(content, assignmentPrefix)
		if idx == -1 {
			t.Fatalf("assignment not found in output: %q", content)
		}
		actual := strings.TrimSpace(content[idx+len(assignmentPrefix):])
		expected := `{
  bool = true
  nested = {
    key = "value"
  }
  number = 42
  string = "value"
}`
		if actual != expected {
			t.Errorf("map content does not match.\nExpected:\n%s\nGot:\n%s", expected, actual)
		}
	})

	t.Run("ComplexObjectNodeGroups", func(t *testing.T) {
		// Given a complex nested object for node groups
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"node_groups": map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "node_groups", value, nil)

		// Then the complex object should be formatted correctly
		expected := "node_groups = {\n  node_groups = {\n    default = {\n      desired_size = 2\n      instance_types = [\"t3.medium\"]\n      max_size = 3\n      min_size = 1\n    }\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ComplexObjectAddons", func(t *testing.T) {
		// Given a complex object with empty maps for addons
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"addons": map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "addons", value, nil)

		// Then the addons object should be formatted correctly with sorted keys
		expected := "addons = {\n  addons = {\n    aws-ebs-csi-driver = {}\n    aws-efs-csi-driver = {}\n    coredns = {}\n    eks-pod-identity-agent = {}\n    external-dns = {}\n    vpc-cni = {}\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ObjectAssignmentNoHeredoc", func(t *testing.T) {
		// Given a nested object value without requiring heredoc
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"default": map[string]any{
				"desired_size":   2,
				"instance_types": []any{"t3.medium"},
				"max_size":       3,
				"min_size":       1,
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "node_groups", value, nil)

		// Then the object should be assigned directly without heredoc
		expected := "node_groups = {\n  default = {\n    desired_size = 2\n    instance_types = [\"t3.medium\"]\n    max_size = 3\n    min_size = 1\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})
}

func TestTerraformGenerator_Generate(t *testing.T) {
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

		// And mock paths for project and context
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return terraform components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "cluster",
					Source:   "", // No source, so should look in terraform/ directory
					FullPath: "/mock/project/terraform/cluster",
				},
				{
					Path:     "network",
					Source:   "", // No source, so should look in terraform/ directory
					FullPath: "/mock/project/terraform/network",
				},
			}
		}

		// And mock Stat to simulate finding variables.tf files
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			// Normalize path separators for cross-platform compatibility
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "terraform/cluster/variables.tf") ||
				strings.Contains(normalizedPath, "terraform/network/variables.tf") {
				return &mockFileInfo{name: "variables.tf", isDir: false}, nil // File exists
			}
			return nil, os.ErrNotExist
		}

		// And mock ReadFile to return variables.tf content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.micro"
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And mock MkdirAll to avoid filesystem operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// And track written files
		var writtenFiles []string
		mocks.Shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			writtenFiles = append(writtenFiles, path)
			return nil
		}

		// When Generate is called with terraform template data
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name":  "test-cluster",
				"instance_type": "t3.large",
			},
			"terraform/network": map[string]any{
				"vpc_cidr": "10.0.0.0/16",
			},
			"blueprint": map[string]any{
				"kind": "Blueprint",
			},
		}

		err := generator.Generate(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And tfvars files should be written for terraform components only
		expectedFiles := []string{
			"/mock/context/terraform/cluster.tfvars",
			"/mock/context/terraform/network.tfvars",
		}
		if len(writtenFiles) != len(expectedFiles) {
			t.Errorf("expected %d files written, got %d", len(expectedFiles), len(writtenFiles))
		}
		for _, expectedFile := range expectedFiles {
			found := false
			for _, writtenFile := range writtenFiles {
				// Normalize both paths for cross-platform comparison
				if filepath.ToSlash(writtenFile) == filepath.ToSlash(expectedFile) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected file %s to be written", expectedFile)
			}
		}
	})

	t.Run("SkipsNonTerraformData", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And mock paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return no terraform components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}

		// And mock MkdirAll to avoid filesystem operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// And track written files
		var writtenFiles []string
		mocks.Shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			writtenFiles = append(writtenFiles, path)
			return nil
		}

		// When Generate is called with non-terraform data only
		data := map[string]any{
			"blueprint": map[string]any{
				"kind": "Blueprint",
			},
			"config": map[string]any{
				"setting": "value",
			},
		}

		err := generator.Generate(data)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And no files should be written
		if len(writtenFiles) != 0 {
			t.Errorf("expected no files written, got %d", len(writtenFiles))
		}
	})

	t.Run("ErrorWhenComponentNotFoundInBlueprint", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And mock paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return empty components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}

		// When Generate is called with terraform data for non-existent component
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		err := generator.Generate(data)

		// Then no error should be returned (component should be skipped)
		if err != nil {
			t.Errorf("expected no error when component not found in blueprint, got: %v", err)
		}
	})

	t.Run("ErrorWhenVariablesTfNotFound", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And mock paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return terraform components without source
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "cluster",
					Source:   "", // No source to avoid module shim generation
					FullPath: "/mock/project/terraform/cluster",
				},
			}
		}

		// And mock MkdirAll to avoid filesystem operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// And mock Stat to simulate variables.tf not found
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When Generate is called with terraform data
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		err := generator.Generate(data)

		// Then an error should be returned
		if err == nil {
			t.Error("expected error when variables.tf not found, got nil")
		}
		if !strings.Contains(err.Error(), "variables.tf not found") {
			t.Errorf("expected error about variables.tf not found, got: %v", err)
		}
	})

	t.Run("SkipsComponentNotFoundInBlueprint", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And mock paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return empty components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}

		// When Generate is called with terraform data for non-existent component
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		err := generator.Generate(data)

		// Then no error should be returned (component should be skipped)
		if err != nil {
			t.Errorf("expected no error when component not found in blueprint, got: %v", err)
		}
	})

	t.Run("SkipsMultipleComponentsNotInBlueprintAndProcessesExisting", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And mock paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// And mock blueprint handler to return only AWS components (simulating AWS provider)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "cluster/aws-eks",
					Source:   "core",
					FullPath: "/mock/project/.windsor/.tf_modules/cluster/aws-eks",
				},
				{
					Path:     "network/aws-vpc",
					Source:   "core",
					FullPath: "/mock/project/.windsor/.tf_modules/network/aws-vpc",
				},
			}
		}

		// And mock Stat to simulate finding variables.tf files for AWS components only
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			normalizedPath := filepath.ToSlash(path)
			if strings.Contains(normalizedPath, "cluster/aws-eks/variables.tf") ||
				strings.Contains(normalizedPath, "network/aws-vpc/variables.tf") {
				return &mockFileInfo{name: "variables.tf", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		// And mock ReadFile to return variables.tf content for AWS components
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.Contains(path, "variables.tf") {
				return []byte(`variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And mock MkdirAll and WriteFile
		var writtenFiles []string
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error { return nil }
		mocks.Shims.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
			writtenFiles = append(writtenFiles, name)
			return nil
		}

		// When Generate is called with mixed provider template data
		data := map[string]any{
			// AWS components that exist in blueprint
			"terraform/cluster/aws-eks": map[string]any{
				"cluster_name": "test-aws-cluster",
			},
			"terraform/network/aws-vpc": map[string]any{
				"vpc_name": "test-vpc",
			},
			// Azure components that DON'T exist in blueprint (should be skipped)
			"terraform/cluster/azure-aks": map[string]any{
				"cluster_name": "test-azure-cluster",
			},
			"terraform/network/azure-vnet": map[string]any{
				"vnet_name": "test-vnet",
			},
		}

		err := generator.Generate(data)

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error with mixed provider templates, got: %v", err)
		}

		// And AWS component tfvars files should be written (only module files for sourced components)
		expectedFiles := []string{
			"/mock/project/.windsor/.tf_modules/cluster/aws-eks/terraform.tfvars",
			"/mock/project/.windsor/.tf_modules/network/aws-vpc/terraform.tfvars",
		}

		if len(writtenFiles) != len(expectedFiles) {
			t.Errorf("expected %d tfvars files to be written, got %d: %v", len(expectedFiles), len(writtenFiles), writtenFiles)
		}

		for _, expectedFile := range expectedFiles {
			found := false
			for _, writtenFile := range writtenFiles {
				// Normalize both paths for cross-platform comparison
				if filepath.ToSlash(writtenFile) == filepath.ToSlash(expectedFile) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected tfvars file %s to be written, but it wasn't", expectedFile)
			}
		}

		// Verify no Azure files were written
		for _, writtenFile := range writtenFiles {
			if strings.Contains(writtenFile, "azure") {
				t.Errorf("unexpected Azure tfvars file written: %s", writtenFile)
			}
		}
	})
}

func TestTerraformGenerator_generateTfvarsFile_AdditionalCases(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ErrorParsingVariablesFile", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, mocks := setup(t)

		// And ReadFile returns invalid HCL content
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte("invalid hcl content {{{"), nil
		}

		// When generateTfvarsFile is called
		err := generator.generateTfvarsFile("/test/output.tfvars", "/test/variables.tf", map[string]any{}, "")

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from parsing variables file, got nil")
		}
	})

	t.Run("ErrorCreatingParentDirectory", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, mocks := setup(t)

		// And ReadFile returns valid variables.tf content
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`variable "test_var" {
  description = "A test variable"
  type        = string
  default     = "default_value"
}`), nil
		}

		// And MkdirAll returns an error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		// When generateTfvarsFile is called
		err := generator.generateTfvarsFile("/test/subdir/output.tfvars", "/test/variables.tf", map[string]any{}, "")

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from MkdirAll, got nil")
		}
	})

	t.Run("ErrorWritingTfvarsFile", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, mocks := setup(t)

		// And ReadFile returns valid variables.tf content
		mocks.Shims.ReadFile = func(name string) ([]byte, error) {
			return []byte(`variable "test_var" {
  description = "A test variable"
  type        = string
  default     = "default_value"
}`), nil
		}

		// And MkdirAll succeeds
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// And WriteFile returns an error
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		// When generateTfvarsFile is called
		err := generator.generateTfvarsFile("/test/output.tfvars", "/test/variables.tf", map[string]any{}, "")

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from WriteFile, got nil")
		}
	})
}

func TestTerraformGenerator_Generate_AdditionalCases(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims

		// Mock GetProjectRoot to return /project for tests
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/project", nil
		}

		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ErrorGetProjectRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot returns an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// And valid terraform data
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from GetProjectRoot, got nil")
		}
	})

	t.Run("ErrorPreloadingOCIArtifacts", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And blueprint handler returns components with OCI sources
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "cluster",
					Source: "oci://registry.example.com/terraform/cluster:v1.0.0",
				},
			}
		}

		// And artifact builder returns an error
		artifactBuilder := mocks.Injector.Resolve("artifactBuilder").(bundler.Artifact)
		if mockArtifact, ok := artifactBuilder.(*bundler.MockArtifact); ok {
			mockArtifact.PullFunc = func(refs []string) (map[string][]byte, error) {
				return nil, fmt.Errorf("artifact pull error")
			}
		}

		// And valid terraform data
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from preloadOCIArtifacts, got nil")
		}
	})

	t.Run("ErrorFindingVariablesTfFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And blueprint handler returns components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "cluster",
					Source:   "", // No source, so should look in terraform/ directory
					FullPath: "/project/terraform/cluster",
				},
			}
		}

		// And Stat returns file not found for variables.tf
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "variables.tf") {
				return nil, os.ErrNotExist
			}
			return &mockFileInfo{name: "somefile"}, nil
		}

		// And valid terraform data
		data := map[string]any{
			"terraform/cluster": map[string]any{
				"cluster_name": "test-cluster",
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected error from findVariablesTfFileForComponent, got nil")
		}
	})

	t.Run("ComponentNotFoundInBlueprint", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And blueprint handler returns no components
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}

		// And valid terraform data for a component that doesn't exist in blueprint
		data := map[string]any{
			"terraform/nonexistent": map[string]any{
				"some_var": "value",
			},
		}

		// When Generate is called
		err := generator.Generate(data)

		// Then no error should occur (component should be skipped)
		if err != nil {
			t.Errorf("Expected no error when component not found in blueprint, got: %v", err)
		}
	})
}

func TestTerraformGenerator_Generate_ResetFlag(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("GenerateRespectsResetFlag", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Set up project and context paths
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}

		// Set up component that the generator can process
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test-module",
			FullPath: "/mock/project/terraform/test-module",
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{component}
		}

		// Mock variables.tf file exists
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return &mockFileInfo{name: "variables.tf", isDir: false}, nil
			}
			// Return that tfvars file exists
			if strings.HasSuffix(path, ".tfvars") {
				return &mockFileInfo{name: "test.tfvars", isDir: false}, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock variables.tf content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test_var" {
  description = "Test variable"
  type        = string
}`), nil
			}
			// Return existing tfvars content
			if strings.HasSuffix(path, ".tfvars") {
				return []byte("# existing content"), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// Track if WriteFile was called
		writeFileCalled := false
		mocks.Shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			writeFileCalled = true
			return nil
		}
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// When Generate is called WITHOUT reset flag (overwrite=false)
		data := map[string]any{
			"terraform/test-module": map[string]any{
				"test_var": "test_value",
			},
		}
		err := generator.Generate(data, false)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And WriteFile should NOT be called (file should not be overwritten)
		if writeFileCalled {
			t.Error("expected WriteFile to not be called when reset=false and file exists")
		}

		// Reset the flag for the next test
		writeFileCalled = false

		// When Generate is called WITH reset flag (overwrite=true)
		err = generator.Generate(data, true)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And WriteFile should be called (file should be overwritten)
		if !writeFileCalled {
			t.Error("expected WriteFile to be called when reset=true")
		}
	})
}
