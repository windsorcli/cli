package env

import (
	"bytes"
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
		// When creating a new mock environment without a container
		mockEnv := NewMockEnv(nil)
		// Then no error should be returned
		if mockEnv == nil {
			t.Errorf("Expected mockEnv, got nil")
		}
	})

	t.Run("CreateMockEnvWithContainer", func(t *testing.T) {
		// Given a mock DI container
		mockContainer := &di.MockContainer{}
		// When creating a new mock environment with the container
		mockEnv := NewMockEnv(mockContainer)
		// Then no error should be returned and the container should be set
		if mockEnv == nil {
			t.Errorf("Expected mockEnv, got nil")
		}
		if mockEnv.diContainer != mockContainer {
			t.Errorf("Expected container to be set, got %v", mockEnv.diContainer)
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
