package env

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/shell"
)

func captureStdout(t *testing.T, f func()) string {
	// Save the current stdout
	old := os.Stdout
	// Create a pipe to capture stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	// Set stdout to the write end of the pipe
	os.Stdout = w

	// Run the function
	f()

	// Close the write end of the pipe and restore stdout
	w.Close()
	os.Stdout = old

	// Read the captured output
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}

	return buf.String()
}

func TestMockEnvPrinter_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment with a custom InitializeFunc
		mockEnv := NewMockEnvPrinter()
		var initialized bool
		mockEnv.InitializeFunc = func() error {
			initialized = true
			return nil
		}

		// When calling Initialize
		err := mockEnv.Initialize()

		// Then no error should be returned and initialized should be true
		if err != nil {
			t.Errorf("Initialize() error = %v, want nil", err)
		}
		if !initialized {
			t.Errorf("Initialize() did not set initialized to true")
		}
	})

	t.Run("DefaultInitialize", func(t *testing.T) {
		// Given a mock environment with default Initialize implementation
		mockEnv := NewMockEnvPrinter()
		// When calling Initialize
		if err := mockEnv.Initialize(); err != nil {
			t.Errorf("Initialize() error = %v, want nil", err)
		}
	})
}

func TestMockEnvPrinter_NewMockEnvPrinter(t *testing.T) {
	t.Run("CreateMockEnvPrinterWithoutContainer", func(t *testing.T) {
		// When creating a new mock environment without an injector
		mockEnv := NewMockEnvPrinter()
		// Then no error should be returned
		if mockEnv == nil {
			t.Errorf("Expected mockEnv, got nil")
		}
	})
}

func TestMockEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("DefaultGetEnvVars", func(t *testing.T) {
		// Given a mock environment with default GetEnvVars implementation
		mockEnv := NewMockEnvPrinter()
		// When calling GetEnvVars
		envVars, err := mockEnv.GetEnvVars()
		// Then no error should be returned and envVars should be an empty map
		if err != nil {
			t.Errorf("GetEnvVars() error = %v, want nil", err)
		}
		if len(envVars) != 0 {
			t.Errorf("GetEnvVars() = %v, want empty map", envVars)
		}
	})

	t.Run("CustomGetEnvVars", func(t *testing.T) {
		// Given a mock environment with custom GetEnvVars implementation
		mockEnv := NewMockEnvPrinter()
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		mockEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return expectedEnvVars, nil
		}
		// When calling GetEnvVars
		envVars, err := mockEnv.GetEnvVars()
		// Then no error should be returned and envVars should match expectedEnvVars
		if err != nil {
			t.Errorf("GetEnvVars() error = %v, want nil", err)
		}
		if len(envVars) != len(expectedEnvVars) {
			t.Errorf("GetEnvVars() = %v, want %v", envVars, expectedEnvVars)
		}
		for key, value := range expectedEnvVars {
			if envVars[key] != value {
				t.Errorf("GetEnvVars()[%v] = %v, want %v", key, envVars[key], value)
			}
		}
	})
}

func TestMockEnvPrinter_Print(t *testing.T) {
	t.Run("DefaultPrint", func(t *testing.T) {
		// Given a mock environment with default Print implementation
		mockEnv := NewMockEnvPrinter()
		// When calling Print
		err := mockEnv.Print()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Print() error = %v, want nil", err)
		}
	})

	t.Run("CustomPrint", func(t *testing.T) {
		// Given a mock environment with custom Print implementation
		mockEnv := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom print error")
		mockEnv.PrintFunc = func(customVars ...map[string]string) error {
			return expectedError
		}
		// When calling Print
		err := mockEnv.Print()
		// Then the custom error should be returned
		if err != expectedError {
			t.Errorf("Print() error = %v, want %v", err, expectedError)
		}
	})

	t.Run("WithCustomVars", func(t *testing.T) {
		// Given a mock environment with default Print implementation
		mockEnv := NewMockEnvPrinter()

		// Setup a shell to avoid nil pointer
		mockShell := &shell.MockShell{}

		// Set up a variable to check if PrintEnvVars was called
		var capturedVars map[string]string
		mockShell.PrintEnvVarsFunc = func(vars map[string]string) error {
			capturedVars = vars
			return nil
		}

		// Assign the mock shell to the BaseEnvPrinter
		mockEnv.BaseEnvPrinter.shell = mockShell

		// Create custom vars to test with
		customVars := map[string]string{
			"CUSTOM_VAR1": "value1",
			"CUSTOM_VAR2": "value2",
		}

		// Call Print with custom vars
		err := mockEnv.Print(customVars)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Print(customVars) error = %v, want nil", err)
		}

		// Verify that PrintEnvVars was called with our custom vars
		if capturedVars == nil {
			t.Error("Expected PrintEnvVars to be called, but it wasn't")
		} else {
			for k, v := range customVars {
				if capturedVars[k] != v {
					t.Errorf("Expected %s to be %s, but got %s", k, v, capturedVars[k])
				}
			}
		}
	})
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestMockEnvPrinter_PostEnvHook(t *testing.T) {
	t.Run("DefaultPostEnvHook", func(t *testing.T) {
		// Given a mock environment with default PostEnvHook implementation
		mockEnv := NewMockEnvPrinter()
		// When calling PostEnvHook
		err := mockEnv.PostEnvHook()
		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() error = %v, want nil", err)
		}
	})

	t.Run("CustomPostEnvHook", func(t *testing.T) {
		// Given a mock environment with custom PostEnvHook implementation
		mockEnv := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom post env hook error")
		mockEnv.PostEnvHookFunc = func() error {
			return expectedError
		}
		// When calling PostEnvHook
		err := mockEnv.PostEnvHook()
		// Then the custom error should be returned
		if err != expectedError {
			t.Errorf("PostEnvHook() error = %v, want %v", err, expectedError)
		}
	})
}

// TestMockEnvPrinter_GetAlias tests the GetAlias method of the MockEnvPrinter
func TestMockEnvPrinter_GetAlias(t *testing.T) {
	t.Run("DefaultGetAlias", func(t *testing.T) {
		mockEnv := NewMockEnvPrinter()

		// Call GetAlias with the default implementation
		aliases, err := mockEnv.GetAlias()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedAliases := map[string]string{}
		if !reflect.DeepEqual(aliases, expectedAliases) {
			t.Errorf("Expected %v, got %v", expectedAliases, aliases)
		}
	})

	t.Run("CustomGetAlias", func(t *testing.T) {
		mockEnv := NewMockEnvPrinter()

		// Define custom aliases to return
		expectedAliases := map[string]string{
			"alias1": "command1",
			"alias2": "command2",
		}

		// Set custom GetAliasFunc
		mockEnv.GetAliasFunc = func() (map[string]string, error) {
			return expectedAliases, nil
		}

		// Call GetAlias with the custom implementation
		aliases, err := mockEnv.GetAlias()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !reflect.DeepEqual(aliases, expectedAliases) {
			t.Errorf("Expected %v, got %v", expectedAliases, aliases)
		}
	})
}

// TestMockEnvPrinter_PrintAlias tests the PrintAlias method of the MockEnvPrinter
func TestMockEnvPrinter_PrintAlias(t *testing.T) {
	t.Run("DefaultPrintAlias", func(t *testing.T) {
		mockEnv := NewMockEnvPrinter()

		// Call PrintAlias with the default implementation
		err := mockEnv.PrintAlias()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CustomPrintAlias", func(t *testing.T) {
		mockEnv := NewMockEnvPrinter()

		// Track if the custom function was called
		called := false

		// Set custom PrintAliasFunc
		mockEnv.PrintAliasFunc = func(customAliases ...map[string]string) error {
			called = true

			// Verify the custom aliases if provided
			if len(customAliases) > 0 {
				expectedAliases := map[string]string{"test": "alias"}
				if !reflect.DeepEqual(customAliases[0], expectedAliases) {
					t.Errorf("Expected %v, got %v", expectedAliases, customAliases[0])
				}
			}

			return nil
		}

		// Call PrintAlias with custom aliases
		err := mockEnv.PrintAlias(map[string]string{"test": "alias"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !called {
			t.Error("Expected custom PrintAliasFunc to be called, but it wasn't")
		}
	})

	t.Run("WithCustomAliases", func(t *testing.T) {
		mockEnv := NewMockEnvPrinter()

		// Setup a shell to avoid nil pointer
		mockShell := &shell.MockShell{}

		// Set up a variable to check if PrintAlias was called
		var capturedAliases map[string]string
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			capturedAliases = aliases
			return nil
		}

		// Assign the mock shell to the BaseEnvPrinter
		mockEnv.BaseEnvPrinter.shell = mockShell

		// Setup custom aliases
		customAliases := map[string]string{
			"custom_alias1": "command1",
			"custom_alias2": "command2",
		}

		// Call PrintAlias with custom aliases
		err := mockEnv.PrintAlias(customAliases)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify that PrintAlias was called with our custom aliases
		if capturedAliases == nil {
			t.Error("Expected PrintAlias to be called, but it wasn't")
		} else {
			for k, v := range customAliases {
				if capturedAliases[k] != v {
					t.Errorf("Expected %s to be %s, but got %s", k, v, capturedAliases[k])
				}
			}
		}
	})
}
