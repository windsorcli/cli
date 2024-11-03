package env

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
)

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

func TestMockEnv_Print(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}
	wantOutput := "VAR1=value1\nVAR2=value2\n"

	t.Run("DefaultPrint", func(t *testing.T) {
		// Given a mock environment with default Print implementation
		mockEnv := NewMockEnv(nil)
		// When calling Print
		var buf bytes.Buffer
		mockEnv.Print(envVars)
		output := buf.String()
		// Then the output should be empty as default Print does nothing
		if output != "" {
			t.Errorf("Print() output = %v, want %v", output, "")
		}
	})

	t.Run("CustomPrint", func(t *testing.T) {
		// Given a mock environment with custom Print implementation
		mockEnv := NewMockEnv(nil)
		var buf bytes.Buffer
		mockEnv.PrintFunc = func(envVars map[string]string) {
			for key, value := range envVars {
				fmt.Fprintf(&buf, "%s=%s\n", key, value)
			}
		}
		// When calling Print
		mockEnv.Print(envVars)
		output := buf.String()
		// Then the output should match the expected output
		if output != wantOutput {
			t.Errorf("Print() output = %v, want %v", output, wantOutput)
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
		mockEnv.PostEnvHookFunc = func() error {
			return fmt.Errorf("custom error")
		}
		// When calling PostEnvHook
		err := mockEnv.PostEnvHook()
		// Then the custom error should be returned
		if err == nil || err.Error() != "custom error" {
			t.Errorf("PostEnvHook() error = %v, want custom error", err)
		}
	})
}
