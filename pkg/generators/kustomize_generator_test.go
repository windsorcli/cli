package generators

import (
	"strings"
	"testing"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestKustomizeGenerator_Generate_InMemory(t *testing.T) {
	t.Run("FiltersAndStoresKustomizeData", func(t *testing.T) {
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		// Mock blueprint handler
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		var setData map[string]any
		mockBlueprintHandler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			setData = data
		}

		// Initialize with mock
		generator.blueprintHandler = mockBlueprintHandler

		data := map[string]any{
			"patches/test/configmap": map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test-config",
				},
			},
			"substitution": map[string]any{
				"environment": "test",
			},
			"other/file":        "should be ignored",
			"terraform/main.tf": "terraform content",
		}

		err := generator.Generate(data, false)
		if err != nil {
			t.Fatalf("expected Generate to succeed, got: %v", err)
		}

		// Verify only kustomize data was stored
		if len(setData) != 2 {
			t.Errorf("expected 2 kustomize items, got %d", len(setData))
		}
		if _, exists := setData["patches/test/configmap"]; !exists {
			t.Error("expected patches/test/configmap to be stored")
		}
		if _, exists := setData["substitution"]; !exists {
			t.Error("expected substitution to be stored")
		}
		if _, exists := setData["other/file"]; exists {
			t.Error("expected non-kustomize data to be filtered out")
		}
	})

	t.Run("NoKustomizeData", func(t *testing.T) {
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		called := false
		mockBlueprintHandler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			called = true
		}

		generator.blueprintHandler = mockBlueprintHandler

		data := map[string]any{
			"other/file":        "should be ignored",
			"terraform/main.tf": "terraform content",
		}

		err := generator.Generate(data, false)
		if err != nil {
			t.Fatalf("expected Generate to succeed with no kustomize data, got: %v", err)
		}

		if called {
			t.Error("expected SetRenderedKustomizeData not to be called when no kustomize data present")
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		generator.blueprintHandler = mockBlueprintHandler

		data := map[string]any{
			"patches/test/configmap": "invalid data - should be map",
		}

		err := generator.Generate(data, false)
		if err == nil {
			t.Fatal("expected Generate to fail with validation error")
		}
		if !strings.Contains(err.Error(), "invalid kustomize data") {
			t.Errorf("expected validation error, got: %v", err)
		}
	})

	t.Run("NilData", func(t *testing.T) {
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		err := generator.Generate(nil, false)
		if err == nil {
			t.Fatal("expected Generate to fail with nil data")
		}
		if !strings.Contains(err.Error(), "data cannot be nil") {
			t.Errorf("expected nil data error, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_validateKustomizeData(t *testing.T) {
	injector := di.NewInjector()
	generator := NewKustomizeGenerator(injector)

	t.Run("ValidPatch", func(t *testing.T) {
		data := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "test-config",
			},
		}

		err := generator.validateKustomizeData("patches/test/configmap", data)
		if err != nil {
			t.Errorf("expected valid patch to pass validation, got: %v", err)
		}
	})

	t.Run("InvalidPatch", func(t *testing.T) {
		data := map[string]any{
			"kind": "ConfigMap",
			// Missing apiVersion and metadata.name
		}

		err := generator.validateKustomizeData("patches/test/configmap", data)
		if err == nil {
			t.Error("expected invalid patch to fail validation")
		}
	})

	t.Run("ValidValues", func(t *testing.T) {
		data := map[string]any{
			"environment": "test",
			"port":        80,
			"enabled":     true,
		}

		err := generator.validateKustomizeData("substitution", data)
		if err != nil {
			t.Errorf("expected valid values to pass validation, got: %v", err)
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		data := map[string]any{
			"invalid": []string{"slice", "not", "allowed"},
		}

		err := generator.validateKustomizeData("substitution", data)
		if err == nil {
			t.Error("expected invalid values to fail validation")
		}
	})

	t.Run("NonMapData", func(t *testing.T) {
		err := generator.validateKustomizeData("patches/test/configmap", "not a map")
		if err == nil {
			t.Error("expected non-map data to fail validation")
		}
		if !strings.Contains(err.Error(), "patch values must be a map") {
			t.Errorf("expected map type error, got: %v", err)
		}
	})

	t.Run("UnknownKey", func(t *testing.T) {
		data := map[string]any{"test": "value"}
		err := generator.validateKustomizeData("unknown/key", data)
		if err != nil {
			t.Errorf("expected unknown key to pass (no validation), got: %v", err)
		}
	})
}

func TestKustomizeGenerator_Initialize(t *testing.T) {
	setup := func(t *testing.T) *KustomizeGenerator {
		t.Helper()
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)
		return generator
	}

	setupWithBaseDependencies := func(t *testing.T) *KustomizeGenerator {
		t.Helper()
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		// Register required base dependencies
		mockConfigHandler := &config.MockConfigHandler{}
		mockShell := &shell.MockShell{}
		mockArtifactBuilder := &bundler.MockArtifact{}

		generator.injector.Register("configHandler", mockConfigHandler)
		generator.injector.Register("shell", mockShell)
		generator.injector.Register("artifactBuilder", mockArtifactBuilder)

		return generator
	}

	t.Run("Success", func(t *testing.T) {
		// Given a generator with all required dependencies
		generator := setupWithBaseDependencies(t)
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		generator.injector.Register("blueprintHandler", mockBlueprintHandler)
		// When initializing
		err := generator.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
		if generator.blueprintHandler != mockBlueprintHandler {
			t.Error("Expected blueprint handler to be set")
		}
	})

	t.Run("BaseGeneratorInitializationFailure", func(t *testing.T) {
		// Given a generator with missing config handler
		generator := setup(t)
		// When initializing
		err := generator.Initialize()
		// Then error should be returned
		if err == nil {
			t.Error("Expected error for base generator initialization failure")
		}
		if !strings.Contains(err.Error(), "failed to initialize base generator") {
			t.Errorf("Expected base generator error, got: %v", err)
		}
	})

	t.Run("BlueprintHandlerNotFound", func(t *testing.T) {
		// Given a generator with base dependencies but no blueprint handler
		generator := setupWithBaseDependencies(t)
		// When initializing
		err := generator.Initialize()
		// Then error should be returned from base generator
		if err == nil {
			t.Error("Expected error for missing blueprint handler")
		}
		if !strings.Contains(err.Error(), "failed to initialize base generator") {
			t.Errorf("Expected base generator error, got: %v", err)
		}
	})

	t.Run("BlueprintHandlerWrongType", func(t *testing.T) {
		// Given a generator with wrong type in injector
		generator := setupWithBaseDependencies(t)
		generator.injector.Register("blueprintHandler", "not a blueprint handler")
		// When initializing
		err := generator.Initialize()
		// Then error should be returned from base generator
		if err == nil {
			t.Error("Expected error for wrong blueprint handler type")
		}
		if !strings.Contains(err.Error(), "failed to initialize base generator") {
			t.Errorf("Expected base generator error, got: %v", err)
		}
	})

	t.Run("BlueprintHandlerNil", func(t *testing.T) {
		// Given a generator with nil blueprint handler
		generator := setupWithBaseDependencies(t)
		generator.injector.Register("blueprintHandler", nil)
		// When initializing
		err := generator.Initialize()
		// Then error should be returned from base generator
		if err == nil {
			t.Error("Expected error for nil blueprint handler")
		}
		if !strings.Contains(err.Error(), "failed to initialize base generator") {
			t.Errorf("Expected base generator error, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_validatePostBuildValues(t *testing.T) {
	setup := func(t *testing.T) *KustomizeGenerator {
		t.Helper()
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)
		return generator
	}

	t.Run("ValidScalarTypes", func(t *testing.T) {
		// Given a generator and valid scalar values
		generator := setup(t)
		values := map[string]any{
			"string":  "test",
			"int":     42,
			"int8":    int8(8),
			"int16":   int16(16),
			"int32":   int32(32),
			"int64":   int64(64),
			"uint":    uint(100),
			"uint8":   uint8(8),
			"uint16":  uint16(16),
			"uint32":  uint32(32),
			"uint64":  uint64(64),
			"float32": float32(3.14),
			"float64": float64(3.14159),
			"bool":    true,
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ValidNestedMap", func(t *testing.T) {
		// Given a generator and valid nested map
		generator := setup(t)
		values := map[string]any{
			"nested": map[string]any{
				"string": "test",
				"int":    42,
				"bool":   true,
			},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("InvalidSlice", func(t *testing.T) {
		// Given a generator and values containing a slice
		generator := setup(t)
		values := map[string]any{
			"invalid": []any{"slice", "not", "allowed"},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned
		if err == nil {
			t.Error("Expected error for slice values")
		}
		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected slice error, got: %v", err)
		}
	})

	t.Run("InvalidNestedComplexType", func(t *testing.T) {
		// Given a generator and values with nested complex types
		generator := setup(t)
		values := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": "too deep",
				},
			},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned
		if err == nil {
			t.Error("Expected error for nested complex types")
		}
		if !strings.Contains(err.Error(), "cannot contain nested complex types") {
			t.Errorf("Expected nested complex type error, got: %v", err)
		}
	})

	t.Run("InvalidUnsupportedType", func(t *testing.T) {
		// Given a generator and values with unsupported type
		generator := setup(t)
		values := map[string]any{
			"unsupported": struct{}{},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("Expected unsupported type error, got: %v", err)
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		// Given a generator and empty values map
		generator := setup(t)
		values := map[string]any{}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ParentKeyReporting", func(t *testing.T) {
		// Given a generator and values with parent key context
		generator := setup(t)
		values := map[string]any{
			"invalid": []any{"slice", "not", "allowed"},
		}
		// When validating post-build values with parent key
		err := generator.validatePostBuildValues(values, "parent", 0)
		// Then error should include parent key in message
		if err == nil {
			t.Error("Expected error for slice values")
		}
		if !strings.Contains(err.Error(), "parent.invalid") {
			t.Errorf("Expected parent key in error message, got: %v", err)
		}
	})

	t.Run("NestedSliceInMap", func(t *testing.T) {
		// Given a generator and nested map containing slice
		generator := setup(t)
		values := map[string]any{
			"nested": map[string]any{
				"invalid": []any{"slice", "in", "nested"},
			},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned
		if err == nil {
			t.Error("Expected error for slice in nested map")
		}
		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected slice error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nested.invalid") {
			t.Errorf("Expected nested key in error message, got: %v", err)
		}
	})

	t.Run("MixedValidAndInvalid", func(t *testing.T) {
		// Given a generator and values with mix of valid and invalid types
		generator := setup(t)
		values := map[string]any{
			"valid":   "string",
			"invalid": []any{"slice", "not", "allowed"},
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned for the invalid type
		if err == nil {
			t.Error("Expected error for invalid type")
		}
		if !strings.Contains(err.Error(), "cannot contain slices") {
			t.Errorf("Expected slice error, got: %v", err)
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		// Given a generator and values with nil value
		generator := setup(t)
		values := map[string]any{
			"nil": nil,
		}
		// When validating post-build values
		err := generator.validatePostBuildValues(values, "", 0)
		// Then error should be returned for unsupported type
		if err == nil {
			t.Error("Expected error for nil value")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("Expected unsupported type error, got: %v", err)
		}
	})
}

func TestKustomizeGenerator_Generate_ValuesHandling(t *testing.T) {
	t.Run("HandlesKustomizeValuesAsYAMLBytes", func(t *testing.T) {
		// Given a kustomize generator with YAML values data
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		// Mock blueprint handler
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		var setData map[string]any
		mockBlueprintHandler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			setData = data
		}

		// Initialize with mock
		generator.blueprintHandler = mockBlueprintHandler

		// Create YAML values data
		yamlData := []byte(`common:
  external_domain: test.example.com
  registry_url: registry.test.com
logging:
  enabled: true
monitoring:
  enabled: false`)

		data := map[string]any{
			"substitution": yamlData,
		}

		// When Generate is called
		err := generator.Generate(data, false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the data was stored
		if len(setData) != 1 {
			t.Errorf("Expected 1 substitution item, got %d", len(setData))
		}
		if _, exists := setData["substitution"]; !exists {
			t.Error("Expected substitution to be stored")
		}
	})

	t.Run("HandlesKustomizeValuesAsMap", func(t *testing.T) {
		// Given a kustomize generator with map values data
		injector := di.NewInjector()
		generator := NewKustomizeGenerator(injector)

		// Mock blueprint handler
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		var setData map[string]any
		mockBlueprintHandler.SetRenderedKustomizeDataFunc = func(data map[string]any) {
			setData = data
		}

		// Initialize with mock
		generator.blueprintHandler = mockBlueprintHandler

		// Create map values data
		mapData := map[string]any{
			"common": map[string]any{
				"external_domain": "test.example.com",
				"registry_url":    "registry.test.com",
			},
			"logging": map[string]any{
				"enabled": true,
			},
			"monitoring": map[string]any{
				"enabled": false,
			},
		}

		data := map[string]any{
			"substitution": mapData,
		}

		// When Generate is called
		err := generator.Generate(data, false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify the data was stored
		if len(setData) != 1 {
			t.Errorf("Expected 1 substitution item, got %d", len(setData))
		}
		if _, exists := setData["substitution"]; !exists {
			t.Error("Expected substitution to be stored")
		}
	})
}
