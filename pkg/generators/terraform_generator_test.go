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

func TestTerraformGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then no error should occur during Write
		if err != nil {
			t.Errorf("Expected no error during Write, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock the shell's GetProjectRoot method to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error in GetProjectRoot")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock osMkdirAll to return an error on the second call
		osMkdirAll = func(_ string, _ os.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock GetConfigRoot to return an error
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorMkdirAllComponentFolder", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Counter to track the number of times osMkdirAll is called
		callCount := 0

		// Mock osMkdirAll to return an error on the second call
		osMkdirAll = func(_ string, _ os.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteModuleFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock osWriteFile to return an error when called
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteVariableFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Counter to track the number of times osWriteFile is called
		callCount := 0

		// Mock osWriteFile to return an error on the second call
		osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})

	t.Run("ErrorWriteTfvarsFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osWriteFile function
		originalOsWriteFile := osWriteFile

		// Defer the replacement of osWriteFile to its original function
		defer func() {
			osWriteFile = originalOsWriteFile
		}()

		// Mock osWriteFile to return an error when writing the tfvars file
		osWriteFile = func(filePath string, _ []byte, _ fs.FileMode) error {
			if filepath.Ext(filePath) == ".tfvars" {
				return fmt.Errorf("mock error")
			}
			return nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		err := generator.Write()

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected TerraformGenerator.Write to return an error")
		}
	})
}

func TestTerraformGenerator_writeModuleFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the writeModuleFile method is called
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

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the writeVariableFile method is called
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

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And the writeTfvarsFile method is called with no existing tfvars file
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "bool", Default: true, Description: "desc2", Sensitive: true},
			},
			Values: map[string]interface{}{
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

		// Save the original osStat and osReadFile
		originalStat := osStat
		originalReadFile := osReadFile
		defer func() {
			osStat = originalStat
			osReadFile = originalReadFile
		}()

		// Mock osStat to indicate a file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osReadFile to return some existing content
		existingTfvars := `// Managed by Windsor CLI:
var1 = "oldval1"
// var2 is intentionally missing
`
		osReadFile = func(filename string) ([]byte, error) {
			return []byte(existingTfvars), nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Source: "some-module-source", // to test source comment insertion
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "desc1", Sensitive: false},
				"var2": {Type: "list", Default: []interface{}{"item1"}, Description: "desc2", Sensitive: true},
			},
			Values: map[string]interface{}{
				"var1": "value1",
				"var2": []interface{}{"item2", "item3"},
			},
		})

		// Then we should not have an error merging content
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osMkdirAll function
		originalMkdirAll := osMkdirAll
		defer func() { osMkdirAll = originalMkdirAll }()

		// Mock osMkdirAll to return an error
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "defval", Description: "desc", Sensitive: false},
			},
			Values: map[string]interface{}{
				"var1": "someval",
			},
		})

		// Then we expect an error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}
	})

	t.Run("ErrorReadingExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osStat and osReadFile
		originalStat := osStat
		originalReadFile := osReadFile
		defer func() {
			osStat = originalStat
			osReadFile = originalReadFile
		}()

		// Mock that file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osReadFile to produce an error
		osReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mock read error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]interface{}{
				"var1": "value1",
			},
		})

		// Then it should return an error due to read failure
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}
	})

	t.Run("ErrorParsingExistingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osStat and osReadFile
		originalStat := osStat
		originalReadFile := osReadFile
		defer func() {
			osStat = originalStat
			osReadFile = originalReadFile
		}()

		// Mock that file exists
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osReadFile to return invalid HCL
		osReadFile = func(filename string) ([]byte, error) {
			invalidHCL := `this is definitely not valid HCL`
			return []byte(invalidHCL), nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]interface{}{
				"var1": "val1",
			},
		})

		// Then we expect a parsing error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osWriteFile
		originalWriteFile := osWriteFile
		defer func() { osWriteFile = originalWriteFile }()

		// Mock osWriteFile to return an error
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write error")
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And we call writeTfvarsFile
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Values: map[string]interface{}{
				"var1": "val1",
			},
		})

		// Then it should return an error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}
	})

	t.Run("FileExists", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Save the original osStat
		originalStat := osStat
		defer func() { osStat = originalStat }()

		// Mock osStat to always succeed
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// When a new TerraformGenerator is created
		generator := NewTerraformGenerator(mocks.Injector)
		if initErr := generator.Initialize(); initErr != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return nil, got %v", initErr)
		}

		// And the writeTfvarsFile method is called
		err := generator.writeTfvarsFile("/fake/dir", blueprintv1alpha1.TerraformComponent{
			Variables: map[string]blueprintv1alpha1.TerraformVariable{
				"var1": {Type: "string", Default: "default1", Description: "description1", Sensitive: false},
			},
			Values: map[string]interface{}{
				"var1": "value1",
			},
		})

		// Then it should return an error because this test simulates a scenario
		// in which simply detecting the file's presence triggers a failure
		// (matching the original test's expectation).
		if err == nil {
			t.Errorf("Expected an error, got nil")
		}
	})
}

func TestConvertToCtyValue(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Test cases for different data types
		tests := []struct {
			input    interface{}
			expected cty.Value
		}{
			{input: "string", expected: cty.StringVal("string")},
			{input: 42, expected: cty.NumberIntVal(42)},
			{input: 3.14, expected: cty.NumberFloatVal(3.14)},
			{input: true, expected: cty.BoolVal(true)},
			{input: []interface{}{"item1", "item2"}, expected: cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")})},
			{input: map[string]interface{}{"key1": "value1"}, expected: cty.ObjectVal(map[string]cty.Value{"key1": cty.StringVal("value1")})},
			{input: []interface{}{}, expected: cty.ListValEmpty(cty.DynamicPseudoType)}, // Test for empty list
			{input: nil, expected: cty.NilVal}, // Test for nil value
		}

		for _, test := range tests {
			result := convertToCtyValue(test.input)
			if !result.RawEquals(test.expected) {
				t.Errorf("Expected %v, got %v", test.expected, result)
			}
		}
	})
}
