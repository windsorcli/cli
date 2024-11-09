package env

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
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

func TestMockEnv_NewMockEnv(t *testing.T) {
	t.Run("CreateMockEnvWithoutContainer", func(t *testing.T) {
		// When creating a new mock environment without an injector
		mockEnv := NewMockEnv(nil)
		// Then no error should be returned
		if mockEnv == nil {
			t.Errorf("Expected mockEnv, got nil")
		}
	})

	t.Run("CreateMockEnvWithContainer", func(t *testing.T) {
		// Given a mock injector
		mockInjector := di.NewMockInjector()
		// When creating a new mock environment with the injector
		mockEnv := NewMockEnv(mockInjector)
		// Then no error should be returned and the injector should be set
		if mockEnv == nil {
			t.Errorf("Expected mockEnv, got nil")
		}
		if mockEnv.Injector != mockInjector {
			t.Errorf("Expected injector to be set, got %v", mockEnv.Injector)
		}
	})
}

func TestMockEnv_GetEnvVars(t *testing.T) {
	t.Run("DefaultGetEnvVars", func(t *testing.T) {
		// Given a mock environment with default GetEnvVars implementation
		mockEnv := NewMockEnv(nil)
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
		mockEnv := NewMockEnv(nil)
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

func TestMockEnv_Print(t *testing.T) {
	t.Run("DefaultPrint", func(t *testing.T) {
		// Given a mock environment with default Print implementation
		mockEnv := NewMockEnv(nil)
		// When calling Print
		err := mockEnv.Print()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Print() error = %v, want nil", err)
		}
	})

	t.Run("CustomPrint", func(t *testing.T) {
		// Given a mock environment with custom Print implementation
		mockEnv := NewMockEnv(nil)
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

func TestMockEnv_PostEnvHook(t *testing.T) {
	t.Run("DefaultPostEnvHook", func(t *testing.T) {
		// Given a mock environment with default PostEnvHook implementation
		mockEnv := NewMockEnv(nil)
		// When calling PostEnvHook
		err := mockEnv.PostEnvHook()
		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() error = %v, want nil", err)
		}
	})

	t.Run("CustomPostEnvHook", func(t *testing.T) {
		// Given a mock environment with custom PostEnvHook implementation
		mockEnv := NewMockEnv(nil)
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
