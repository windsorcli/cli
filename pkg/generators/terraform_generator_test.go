package generators

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTerraformGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)

		// Then the generator should be non-nil
		if generator == nil {
			t.Errorf("Expected NewTerraformGenerator to return a non-nil value")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then no error should occur during Write
		if err != nil {
			t.Errorf("Expected no error during Write, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the shell's GetProjectRoot method is mocked to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error in GetProjectRoot")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And osMkdirAll is mocked to return an error
		osMkdirAll = func(_ string, _ os.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And GetConfigRoot is mocked to return an error
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorMkdirAllComponentFolder", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a counter to track the number of times osMkdirAll is called
		callCount := 0

		// And osMkdirAll is mocked to return an error on the second call
		osMkdirAll = func(_ string, _ os.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteModuleFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And osWriteFile is mocked to return an error
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteVariableFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a counter to track the number of times osWriteFile is called
		callCount := 0

		// And osWriteFile is mocked to return an error on the second call
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteTfvarsFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osWriteFile function is saved
		originalOsWriteFile := osWriteFile

		// And osWriteFile is mocked to return an error when writing the tfvars file
		osWriteFile = func(filePath string, _ []byte, _ fs.FileMode) error {
			if filepath.Ext(filePath) == ".tfvars" {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}

		// And the original osWriteFile function is restored
		osWriteFile = originalOsWriteFile
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformGenerator_writeModuleFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the writeModuleFile method is called
		err := generator.writeModuleFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Source: "fake-source",
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
				"var2": {Type: "number", Default: 2, Description: "description2", Sensitive: true},
			},
		})

		// Then it should not return an error
		if err != nil {
			t.Errorf("Expected TerraformGenerator.writeModuleFile to return a nil value, got %v", err)
		}
	})
}

func TestTerraformGenerator_writeVariableFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// When the writeVariableFile method is called
		err := generator.writeVariableFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
				"var2": {Type: "number", Default: 2, Description: "description2", Sensitive: true},
			},
		})

		// Then it should not return an error
		if err != nil {
			t.Errorf("Expected TerraformGenerator.writeVariableFile to return a nil value, got %v", err)
		}
	})
}

func TestTerraformGenerator_writeTfvarsFile(t *testing.T) {
	t.Run("SuccessNoExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When the writeTfvarsFile method is called with no existing tfvars file
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "bool", Default: true, Description: "desc2", Sensitive: true},
			},
			Values: map[string]any{
				"var1": "newval1",
				"var2": false,
			},
		})

		// Then it should not return an error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osStat and osReadFile are saved
		originalStat := osStat
		originalReadFile := osReadFile

		// And osStat is mocked to indicate a file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And osReadFile is mocked to return some existing content
		existingTfvars := `// Managed by Windsor CLI:
var1 = "oldval1"
// var2 is intentionally missing
`
		osReadFile = func(filename string) ([]byte, error) {
			return []byte(existingTfvars), nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Source: "some-module-source", // to test source comment insertion
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "list", Default: []any{"item1"}, Description: "desc2", Sensitive: true},
			},
			Values: map[string]any{
				"var1": "value1",
				"var2": []any{"item2", "item3"},
			},
		})

		// Then we should not have an error merging content
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the original functions are restored
		osStat = originalStat
		osReadFile = originalReadFile
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osMkdirAll function is saved
		originalMkdirAll := osMkdirAll

		// And osMkdirAll is mocked to return an error
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "defval", Description: "desc", Sensitive: false},
			},
			Values: map[string]any{
				"var1": "someval",
			},
		})

		// Then we expect an error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}

		// And the original function is restored
		osMkdirAll = originalMkdirAll
	})

	t.Run("ErrorReadingExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osStat and osReadFile are saved
		originalStat := osStat
		originalReadFile := osReadFile

		// And osStat is mocked to indicate a file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And osReadFile is mocked to produce an error
		osReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mock read error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "value1",
			},
		})

		// Then it should return an error due to read failure
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}

		// And the original functions are restored
		osStat = originalStat
		osReadFile = originalReadFile
	})

	t.Run("ErrorParsingExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osStat and osReadFile are saved
		originalStat := osStat
		originalReadFile := osReadFile

		// And osStat is mocked to indicate a file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And osReadFile is mocked to return invalid HCL
		osReadFile = func(filename string) ([]byte, error) {
			invalidHCL := `this is definitely not valid HCL`
			return []byte(invalidHCL), nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "val1",
			},
		})

		// Then we expect a parsing error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}

		// And the original functions are restored
		osStat = originalStat
		osReadFile = originalReadFile
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osWriteFile is saved
		originalWriteFile := osWriteFile

		// And osWriteFile is mocked to return an error
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write error")
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]any{
				"var1": "val1",
			},
		})

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}

		// And the original function is restored
		osWriteFile = originalWriteFile
	})

	t.Run("FileExists", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osStat is saved
		originalStat := osStat

		// And osStat is mocked to always succeed
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// When the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
			},
			Values: map[string]any{
				"var1": "value1",
			},
		})

		// Then it should return an error because this test simulates a scenario
		// in which simply detecting the file's presence triggers a failure
		// (matching the original test's expectation).
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}

		// And the original function is restored
		osStat = originalStat
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestConvertToCtyValue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given test cases for different data types
		tests := []struct {
			input    any
			expected cty.Value
		}{
			{input: "string", expected: cty.StringVal("string")},
			{input: 42, expected: cty.NumberIntVal(42)},
			{input: 3.14, expected: cty.NumberFloatVal(3.14)},
			{input: true, expected: cty.BoolVal(true)},
			{input: []any{"item1", "item2"}, expected: cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")})},
			{input: map[string]any{"key1": "value1"}, expected: cty.ObjectVal(map[string]cty.Value{"key1": cty.StringVal("value1")})},
			{input: []any{}, expected: cty.ListValEmpty(cty.DynamicPseudoType)}, // Test for empty list
			{input: nil, expected: cty.NilVal}, // Test for nil value
		}

		// When each test case is processed
		for _, test := range tests {
			// And convertToCtyValue is called with the input
			result := convertToCtyValue(test.input)

			// Then the result should match the expected value
			if !result.RawEquals(test.expected) {
				t.Errorf("Expected %v, got %v", test.expected, result)
			}
		}
	})
}
