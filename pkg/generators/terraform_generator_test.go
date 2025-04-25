package generators

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTerraformGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}

		// Then the TerraformGenerator should be created correctly
		if generator == nil {
			t.Fatalf("expected TerraformGenerator to be created, got nil")
		}

		// And the TerraformGenerator should have the correct injector
		if generator.injector != mocks.Injector {
			t.Errorf("expected TerraformGenerator to have the correct injector")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

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
		generator, _ := setup(t)

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
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: configHandler,
		})
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
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

	t.Run("ErrorMkdirAllComponentFolder", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a counter to track the number of times MkdirAll is called
		callCount := 0

		// And MkdirAll is mocked to return an error on the second call
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error creating component directory")
			}
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create component directory: mock error creating component directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteModuleFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing module file")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write module file: mock error writing module file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteVariableFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a counter to track the number of times WriteFile is called
		callCount := 0

		// And WriteFile is mocked to return an error on the second call
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error writing variable file")
			}
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write variable file: mock error writing variable file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteTfvarsFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And WriteFile is mocked to return an error when writing tfvars file
		mocks.Shims.WriteFile = func(filePath string, _ []byte, _ fs.FileMode) error {
			if filepath.Ext(filePath) == ".tfvars" {
				return fmt.Errorf("mock error writing tfvars file")
			}
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write tfvars file: error writing tfvars file: mock error writing tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

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
		generator, _ := setup(t)

		// And a component with variables
		component := blueprintv1alpha1.TerraformComponent{
			Source: "fake-source",
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
				"var2": {Type: "number", Default: 2, Description: "description2", Sensitive: true},
			},
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("/fake/dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestTerraformGenerator_writeVariableFile(t *testing.T) {
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
		generator, _ := setup(t)

		// And a component with variables
		component := blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
				"var2": {Type: "number", Default: 2, Description: "description2", Sensitive: true},
			},
		}

		// When writeVariableFile is called
		err := generator.writeVariableFile("/fake/dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

	t.Run("SuccessNoExistingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return an error (file doesn't exist)
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("file not found")
		}

		// And a component with variables and values
		component := blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "bool", Default: true, Description: "desc2", Sensitive: true},
			},
			Values: map[string]any{
				"var1": "newval1",
				"var2": false,
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessExistingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success (file exists)
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return existing content
		existingTfvars := `// Managed by Windsor CLI:
var1 = "oldval1"
// var2 is intentionally missing
`
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return []byte(existingTfvars), nil
		}

		// And a component with variables and values
		component := blueprintv1alpha1.TerraformComponent{
			Source: "some-module-source", // to test source comment insertion
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "list", Default: []any{"item1"}, Description: "desc2", Sensitive: true},
			},
			Values: map[string]any{
				"var1": "value1",
				"var2": []any{"item2", "item3"},
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// And a component with variables and values
		component := blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "defval", Description: "desc", Sensitive: false},
			},
			Values: map[string]any{
				"var1": "someval",
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

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

	t.Run("ErrorReadingExistingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success (file exists)
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "value1",
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

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

	t.Run("ErrorParsingExistingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success (file exists)
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return invalid HCL
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			invalidHCL := `this is definitely not valid HCL`
			return []byte(invalidHCL), nil
		}

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "val1",
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "val1",
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

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

	t.Run("FileExists", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success (file exists)
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "val1",
			},
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("/fake/dir", component)

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

// =============================================================================
// Helper Tests
// =============================================================================

func TestConvertToCtyValue(t *testing.T) {
	t.Run("StringValue", func(t *testing.T) {
		result := convertToCtyValue("string")
		expected := cty.StringVal("string")
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("IntegerValue", func(t *testing.T) {
		result := convertToCtyValue(42)
		expected := cty.NumberIntVal(42)
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("FloatValue", func(t *testing.T) {
		result := convertToCtyValue(3.14)
		expected := cty.NumberFloatVal(3.14)
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("BooleanValue", func(t *testing.T) {
		result := convertToCtyValue(true)
		expected := cty.BoolVal(true)
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("ListValue", func(t *testing.T) {
		result := convertToCtyValue([]any{"item1", "item2"})
		expected := cty.ListVal([]cty.Value{
			cty.StringVal("item1"),
			cty.StringVal("item2"),
		})
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("MapValue", func(t *testing.T) {
		result := convertToCtyValue(map[string]any{"key1": "value1"})
		expected := cty.ObjectVal(map[string]cty.Value{
			"key1": cty.StringVal("value1"),
		})
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("EmptyList", func(t *testing.T) {
		result := convertToCtyValue([]any{})
		expected := cty.ListValEmpty(cty.DynamicPseudoType)
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		result := convertToCtyValue(nil)
		expected := cty.NilVal
		if !result.RawEquals(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})
}
