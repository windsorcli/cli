package blueprint

import (
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Helper Functions
// =============================================================================

func TestYamlMarshalWithDefinedPaths(t *testing.T) {
	setup := func(t *testing.T) (BlueprintHandler, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewBlueprintHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BlueprintHandler: %v", err)
		}
		return handler, mocks
	}

	t.Run("IgnoreYamlMinusTag", func(t *testing.T) {
		// Given a struct with a YAML minus tag
		type testStruct struct {
			Public  string `yaml:"public"`
			private string `yaml:"-"`
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the public field should be included
		if !strings.Contains(string(result), "public: value") {
			t.Errorf("Expected 'public: value' in result, got: %s", string(result))
		}

		// And the ignored field should be excluded
		if strings.Contains(string(result), "ignored") {
			t.Errorf("Expected 'ignored' not to be in result, got: %s", string(result))
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		// When marshalling nil input
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nil input, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "invalid input: nil value") {
			t.Errorf("Expected error about nil input, got: %v", err)
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		// Given an empty slice
		input := []string{}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the result should be an empty array
		if string(result) != "[]\n" {
			t.Errorf("Expected '[]\n', got: %s", string(result))
		}
	})

	t.Run("NoYamlTag", func(t *testing.T) {
		// Given a struct with no YAML tags
		type testStruct struct {
			Field string
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the field name should be used as is
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("CustomYamlTag", func(t *testing.T) {
		// Given a struct with a custom YAML tag
		type testStruct struct {
			Field string `yaml:"custom_field"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_field: value") {
			t.Errorf("Expected 'custom_field: value' in result, got: %s", string(result))
		}
	})

	t.Run("MapWithCustomTags", func(t *testing.T) {
		// Given a map with nested structs using custom YAML tags
		type nestedStruct struct {
			Value string `yaml:"custom_value"`
		}
		input := map[string]nestedStruct{
			"key": {Value: "test"},
		}

		// When marshalling the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the map key should be preserved
		if !strings.Contains(string(result), "key:") {
			t.Errorf("Expected 'key:' in result, got: %s", string(result))
		}

		// And the nested custom field name should be used
		if !strings.Contains(string(result), "  custom_value: test") {
			t.Errorf("Expected '  custom_value: test' in result, got: %s", string(result))
		}
	})

	t.Run("DefaultFieldName", func(t *testing.T) {
		// Given a struct with default field names
		data := struct {
			Field string
		}{
			Field: "value",
		}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(data)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the default field name should be used
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		// When marshalling nil input
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nil input, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "invalid input: nil value") {
			t.Errorf("Expected error about nil input, got: %v", err)
		}
	})

	t.Run("FuncType", func(t *testing.T) {
		// When marshalling a function type
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(func() {})

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for func type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type func") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// When marshalling an unsupported type
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(make(chan int))

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		// Given a map with nil values
		input := map[string]any{
			"key1": nil,
			"key2": "value2",
		}

		// When marshalling the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "key1: null") {
			t.Errorf("Expected 'key1: null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "key2: value2") {
			t.Errorf("Expected 'key2: value2' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilValues", func(t *testing.T) {
		// Given a slice with nil values
		input := []any{nil, "value", nil}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "- null") {
			t.Errorf("Expected '- null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "- value") {
			t.Errorf("Expected '- value' in result, got: %s", string(result))
		}
	})

	t.Run("StructWithPrivateFields", func(t *testing.T) {
		// Given a struct with both public and private fields
		type testStruct struct {
			Public  string
			private string
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And public fields should be included
		if !strings.Contains(string(result), "Public: value") {
			t.Errorf("Expected 'Public: value' in result, got: %s", string(result))
		}

		// And private fields should be excluded
		if strings.Contains(string(result), "private") {
			t.Errorf("Expected 'private' not to be in result, got: %s", string(result))
		}
	})

	t.Run("StructWithYamlTag", func(t *testing.T) {
		// Given a struct with a YAML tag
		type testStruct struct {
			Field string `yaml:"custom_name"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_name: value") {
			t.Errorf("Expected 'custom_name: value' in result, got: %s", string(result))
		}
	})

	t.Run("NestedStructs", func(t *testing.T) {
		// Given nested structs
		type nested struct {
			Value string
		}
		type parent struct {
			Nested nested
		}
		input := parent{Nested: nested{Value: "test"}}

		// When marshalling the nested structs
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the parent field should be included
		if !strings.Contains(string(result), "Nested:") {
			t.Errorf("Expected 'Nested:' in result, got: %s", string(result))
		}

		// And the nested field should be properly indented
		if !strings.Contains(string(result), "  Value: test") {
			t.Errorf("Expected '  Value: test' in result, got: %s", string(result))
		}
	})

	t.Run("NumericTypes", func(t *testing.T) {
		// Given a struct with various numeric types
		type numbers struct {
			Int     int     `yaml:"int"`
			Int8    int8    `yaml:"int8"`
			Int16   int16   `yaml:"int16"`
			Int32   int32   `yaml:"int32"`
			Int64   int64   `yaml:"int64"`
			Uint    uint    `yaml:"uint"`
			Uint8   uint8   `yaml:"uint8"`
			Uint16  uint16  `yaml:"uint16"`
			Uint32  uint32  `yaml:"uint32"`
			Uint64  uint64  `yaml:"uint64"`
			Float32 float32 `yaml:"float32"`
			Float64 float64 `yaml:"float64"`
		}
		input := numbers{
			Int: 1, Int8: 2, Int16: 3, Int32: 4, Int64: 5,
			Uint: 6, Uint8: 7, Uint16: 8, Uint32: 9, Uint64: 10,
			Float32: 11.1, Float64: 12.2,
		}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And all numeric values should be correctly represented
		for _, expected := range []string{
			"int: 1", "int8: 2", "int16: 3", "int32: 4", "int64: 5",
			"uint: 6", "uint8: 7", "uint16: 8", "uint32: 9", "uint64: 10",
			"float32: 11.1", "float64: 12.2",
		} {
			if !strings.Contains(string(result), expected) {
				t.Errorf("Expected '%s' in result, got: %s", expected, string(result))
			}
		}
	})

	t.Run("BooleanType", func(t *testing.T) {
		// Given a struct with boolean fields
		type boolStruct struct {
			True  bool `yaml:"true"`
			False bool `yaml:"false"`
		}
		input := boolStruct{True: true, False: false}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the boolean values should be correctly represented
		if !strings.Contains(string(result), `"true": true`) {
			t.Errorf("Expected '\"true\": true' in result, got: %s", string(result))
		}
		if !strings.Contains(string(result), `"false": false`) {
			t.Errorf("Expected '\"false\": false' in result, got: %s", string(result))
		}
	})

	t.Run("NilPointerAndInterface", func(t *testing.T) {
		// Given a struct with nil pointers and interfaces
		type testStruct struct {
			NilPtr       *string              `yaml:"nil_ptr"`
			NilInterface any                  `yaml:"nil_interface"`
			NilMap       map[string]string    `yaml:"nil_map"`
			NilSlice     []string             `yaml:"nil_slice"`
			NilStruct    *struct{ Field int } `yaml:"nil_struct"`
		}
		input := testStruct{}

		// When marshalling the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil interfaces should be represented as empty objects
		if !strings.Contains(string(result), "nil_interface: {}") {
			t.Errorf("Expected 'nil_interface: {}' in result, got: %s", string(result))
		}

		// And nil slices should be represented as empty arrays
		if !strings.Contains(string(result), "nil_slice: []") {
			t.Errorf("Expected 'nil_slice: []' in result, got: %s", string(result))
		}

		// And nil maps should be represented as empty objects
		if !strings.Contains(string(result), "nil_map: {}") {
			t.Errorf("Expected 'nil_map: {}' in result, got: %s", string(result))
		}

		// And nil structs should be represented as empty objects
		if !strings.Contains(string(result), "nil_struct: {}") {
			t.Errorf("Expected 'nil_struct: {}' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilElements", func(t *testing.T) {
		// Given a slice with nil elements
		type elem struct {
			Field string
		}
		input := []*elem{nil, {Field: "value"}, nil}

		// When marshalling the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And non-nil elements should be correctly represented
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		// Given a map with nil and non-nil values
		input := map[string]any{
			"nil":    nil,
			"nonnil": "value",
			"nilptr": (*string)(nil),
		}

		// When marshalling the map to YAML
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		result, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("yamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "nil: null") {
			t.Errorf("Expected 'nil: null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "nonnil: value") {
			t.Errorf("Expected 'nonnil: value' in result, got: %s", string(result))
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// Given an unsupported channel type
		input := make(chan int)

		// When attempting to marshal the channel
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}

		// And the error should indicate the unsupported type
		if !strings.Contains(err.Error(), "unsupported value type chan") {
			t.Errorf("Expected error about unsupported type, got: %v", err)
		}
	})

	t.Run("FunctionType", func(t *testing.T) {
		// Given a function type
		input := func() {}

		// When attempting to marshal the function
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for function type, got nil")
		}

		// And the error should indicate the unsupported type
		if !strings.Contains(err.Error(), "unsupported value type func") {
			t.Errorf("Expected error about unsupported type, got: %v", err)
		}
	})

	t.Run("ErrorInSliceConversion", func(t *testing.T) {
		// Given a slice containing an unsupported type
		input := []any{make(chan int)}

		// When attempting to marshal the slice
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for slice with unsupported type, got nil")
		}

		// And the error should indicate the slice conversion issue
		if !strings.Contains(err.Error(), "error converting slice element") {
			t.Errorf("Expected error about slice conversion, got: %v", err)
		}
	})

	t.Run("ErrorInMapConversion", func(t *testing.T) {
		// Given a map containing an unsupported type
		input := map[string]any{
			"channel": make(chan int),
		}

		// When attempting to marshal the map
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for map with unsupported type, got nil")
		}

		// And the error should indicate the map conversion issue
		if !strings.Contains(err.Error(), "error converting map value") {
			t.Errorf("Expected error about map conversion, got: %v", err)
		}
	})

	t.Run("ErrorInStructFieldConversion", func(t *testing.T) {
		// Given a struct containing an unsupported field type
		type testStruct struct {
			Channel chan int
		}
		input := testStruct{Channel: make(chan int)}

		// When attempting to marshal the struct
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for struct with unsupported field type, got nil")
		}

		// And the error should indicate the field conversion issue
		if !strings.Contains(err.Error(), "error converting field") {
			t.Errorf("Expected error about field conversion, got: %v", err)
		}
	})

	t.Run("YamlMarshalError", func(t *testing.T) {
		// Given a blueprint handler
		handler, _ := setup(t)
		baseHandler := handler.(*BaseBlueprintHandler)

		// And a mock YAML marshaller that returns an error
		baseHandler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mock yaml marshal error")
		}

		// And a simple struct to marshal
		input := struct{ Field string }{"value"}

		// When marshalling the struct
		_, err := baseHandler.yamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from yaml marshal, got nil")
		}

		// And the error should indicate the YAML marshalling issue
		if !strings.Contains(err.Error(), "error marshalling yaml") {
			t.Errorf("Expected error about yaml marshalling, got: %v", err)
		}
	})
}

func TestTLACode(t *testing.T) {
	// Given a mock Jsonnet VM that returns an error about missing authors
	vm := NewMockJsonnetVM(func(filename, snippet string) (string, error) {
		return "", fmt.Errorf("blueprint has no authors")
	})

	// When evaluating an empty snippet
	_, err := vm.EvaluateAnonymousSnippet("test.jsonnet", "")

	// Then an error about missing authors should be returned
	if err == nil || !strings.Contains(err.Error(), "blueprint has no authors") {
		t.Errorf("expected error containing 'blueprint has no authors', got %v", err)
	}
}

func TestBaseBlueprintHandler_calculateMaxWaitTime(t *testing.T) {
	t.Run("EmptyKustomizations", func(t *testing.T) {
		// Given a blueprint handler with no kustomizations
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return 0 since there are no kustomizations
		if waitTime != 0 {
			t.Errorf("expected 0 duration, got %v", waitTime)
		}
	})

	t.Run("SingleKustomization", func(t *testing.T) {
		// Given a blueprint handler with a single kustomization
		customTimeout := 2 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "test-kustomization",
						Timeout: &metav1.Duration{
							Duration: customTimeout,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the kustomization's timeout
		if waitTime != customTimeout {
			t.Errorf("expected timeout %v, got %v", customTimeout, waitTime)
		}
	})

	t.Run("LinearDependencies", func(t *testing.T) {
		// Given a blueprint handler with linear dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts
		expectedTime := timeout1 + timeout2 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("BranchingDependencies", func(t *testing.T) {
		// Given a blueprint handler with branching dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		timeout4 := 4 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2", "kustomization-3"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-4"},
					},
					{
						Name: "kustomization-4",
						Timeout: &metav1.Duration{
							Duration: timeout4,
						},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the longest path (1 -> 3 -> 4)
		expectedTime := timeout1 + timeout3 + timeout4
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})

	t.Run("CircularDependencies", func(t *testing.T) {
		// Given a blueprint handler with circular dependencies
		timeout1 := 1 * time.Minute
		timeout2 := 2 * time.Minute
		timeout3 := 3 * time.Minute
		handler := &BaseBlueprintHandler{
			blueprint: blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{
					{
						Name: "kustomization-1",
						Timeout: &metav1.Duration{
							Duration: timeout1,
						},
						DependsOn: []string{"kustomization-2"},
					},
					{
						Name: "kustomization-2",
						Timeout: &metav1.Duration{
							Duration: timeout2,
						},
						DependsOn: []string{"kustomization-3"},
					},
					{
						Name: "kustomization-3",
						Timeout: &metav1.Duration{
							Duration: timeout3,
						},
						DependsOn: []string{"kustomization-1"},
					},
				},
			},
		}

		// When calculating max wait time
		waitTime := handler.calculateMaxWaitTime()

		// Then it should return the sum of all timeouts in the cycle (1+2+3+3)
		expectedTime := timeout1 + timeout2 + timeout3 + timeout3
		if waitTime != expectedTime {
			t.Errorf("expected timeout %v, got %v", expectedTime, waitTime)
		}
	})
}

func TestBaseBlueprintHandler_loadPlatformTemplate(t *testing.T) {
	t.Run("ValidPlatforms", func(t *testing.T) {
		// Given a BaseBlueprintHandler
		handler := &BaseBlueprintHandler{}

		// When loading templates for valid platforms
		platforms := []string{"local", "metal", "aws", "azure", "default"}
		for _, platform := range platforms {
			// Then the template should be loaded successfully
			template, err := handler.loadPlatformTemplate(platform)
			if err != nil {
				t.Errorf("Expected no error for platform %s, got: %v", platform, err)
			}
			if len(template) == 0 {
				t.Errorf("Expected non-empty template for platform %s", platform)
			}
		}
	})

	t.Run("InvalidPlatform", func(t *testing.T) {
		// Given a BaseBlueprintHandler
		handler := &BaseBlueprintHandler{}

		// When loading template for invalid platform
		template, err := handler.loadPlatformTemplate("invalid-platform")

		// Then no error should occur but template should be empty
		if err != nil {
			t.Errorf("Expected no error for invalid platform, got: %v", err)
		}
		if len(template) != 0 {
			t.Errorf("Expected empty template for invalid platform, got length: %d", len(template))
		}
	})

	t.Run("EmptyPlatform", func(t *testing.T) {
		// Given a BaseBlueprintHandler
		handler := &BaseBlueprintHandler{}

		// When loading template with empty platform
		template, err := handler.loadPlatformTemplate("")

		// Then no error should occur and template should contain default template
		if err != nil {
			t.Errorf("Expected no error for empty platform, got: %v", err)
		}
		if len(template) == 0 {
			t.Errorf("Expected default template for empty platform, got empty template")
		}
	})
}

func TestBaseBlueprintHandler_loadFileData(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		// Test cases will go here
	})
}
