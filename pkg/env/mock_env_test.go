package env

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"
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
		mockEnv.PrintFunc = func() error {
			return expectedError
		}
		// When calling Print
		err := mockEnv.Print()
		// Then the custom error should be returned
		if err != expectedError {
			t.Errorf("Print() error = %v, want %v", err, expectedError)
		}
	})
}

func TestMockPrinter_GetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment with custom GetAlias implementation
		mockEnv := NewMockEnvPrinter()
		expectedAlias := map[string]string{"test": "echo test"}
		mockEnv.GetAliasFunc = func() (map[string]string, error) {
			return expectedAlias, nil
		}
		// When calling GetAlias
		alias, err := mockEnv.GetAlias()
		// Then no error should be returned and alias should match expectedAlias
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("GetAlias() = %v, want %v", alias, expectedAlias)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock environment with default GetAlias implementation
		mockEnv := NewMockEnvPrinter()
		// When calling GetAlias
		alias, err := mockEnv.GetAlias()
		// Then no error should be returned and alias should be an empty map
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
		expectedAlias := map[string]string{}
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("GetAlias() = %v, want %v", alias, expectedAlias)
		}
	})
}

func TestMockEnvPrinter_PrintAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment with custom PrintAlias implementation
		mockEnv := NewMockEnvPrinter()
		mockEnv.PrintAliasFunc = func() error {
			return nil
		}
		// When calling PrintAlias
		err := mockEnv.PrintAlias()
		// Then no error should be returned
		if err != nil {
			t.Errorf("PrintAlias() error = %v, want nil", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock environment with default GetAlias implementation
		mockEnv := NewMockEnvPrinter()
		// When calling GetAlias
		err := mockEnv.PrintAlias()
		// Then no error should be returned
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
	})
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

func TestMockEnvPrinter_Reset(t *testing.T) {
	t.Run("DefaultReset", func(t *testing.T) {
		// Given a mock environment with default Reset implementation
		mockEnv := NewMockEnvPrinter()

		// When calling Reset (without a custom implementation)
		// This is a no-op, so we just call it to ensure it doesn't panic
		mockEnv.Reset()
	})

	t.Run("CustomReset", func(t *testing.T) {
		// Given a mock environment with custom Reset implementation
		mockEnv := NewMockEnvPrinter()
		resetCalled := false
		mockEnv.ResetFunc = func() {
			resetCalled = true
		}

		// When calling Reset
		mockEnv.Reset()

		// Then it should call the custom reset function
		if !resetCalled {
			t.Error("Reset() did not call ResetFunc")
		}
	})
}
