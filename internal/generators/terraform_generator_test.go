package generators

import (
	"fmt"
	"io/fs"
	"testing"
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

		// Mock osWriteFile to validate the calls
		writeFileCalls := 0
		expectedFiles := []string{
			"/mock/project/root/.tf_modules/remote/path/main.tf",
			"/mock/project/root/.tf_modules/remote/path/variables.tf",
		}
		originalOsWriteFile := osWriteFile
		defer func() { osWriteFile = originalOsWriteFile }()
		osWriteFile = func(name string, _ []byte, _ fs.FileMode) error {
			if writeFileCalls >= len(expectedFiles) {
				t.Errorf("Unexpected call to osWriteFile with name: %s", name)
			} else if name != expectedFiles[writeFileCalls] {
				t.Errorf("Expected osWriteFile to be called with %s, but got %s", expectedFiles[writeFileCalls], name)
			}
			writeFileCalls++
			return nil
		}

		// When a new TerraformGenerator is created and initialized
		generator := NewTerraformGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected TerraformGenerator.Initialize to return a nil value")
		}

		// And the Write method is called
		if err := generator.Write(); err != nil {
			// Then it should succeed without errors
			t.Errorf("Expected TerraformGenerator.Write to return a nil value")
		}

		// Validate that osWriteFile was called the expected number of times
		if writeFileCalls != len(expectedFiles) {
			t.Errorf("Expected osWriteFile to be called %d times, but got %d", len(expectedFiles), writeFileCalls)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// Mock osMkdirAll to return an error
		osMkdirAll = func(_ string, _ fs.FileMode) error {
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
}
