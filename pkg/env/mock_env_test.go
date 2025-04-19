package env

import (
	"fmt"
	"reflect"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockEnvPrinter_Initialize tests the Initialize method of the MockEnvPrinter
func TestMockEnvPrinter_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment printer
		mockEnv := NewMockEnvPrinter()
		var initialized bool
		mockEnv.InitializeFunc = func() error {
			initialized = true
			return nil
		}

		// When initializing
		err := mockEnv.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Initialize() error = %v, want nil", err)
		}
		// And initialized should be true
		if !initialized {
			t.Errorf("Initialize() did not set initialized to true")
		}
	})

	t.Run("DefaultInitialize", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When initializing
		err := mockEnv.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Initialize() error = %v, want nil", err)
		}
	})
}

// TestMockEnvPrinter_NewMockEnvPrinter tests the NewMockEnvPrinter constructor
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

// TestMockEnvPrinter_GetEnvVars tests the GetEnvVars method of the MockEnvPrinter
func TestMockEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("DefaultGetEnvVars", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When getting environment variables
		envVars, err := mockEnv.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("GetEnvVars() error = %v, want nil", err)
		}
		// And envVars should be empty
		if len(envVars) != 0 {
			t.Errorf("GetEnvVars() = %v, want empty map", envVars)
		}
	})

	t.Run("CustomGetEnvVars", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		mockEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return expectedEnvVars, nil
		}

		// When getting environment variables
		envVars, err := mockEnv.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("GetEnvVars() error = %v, want nil", err)
		}
		// And envVars should match expected values
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("GetEnvVars() = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

// TestMockEnvPrinter_Print tests the Print method of the MockEnvPrinter
func TestMockEnvPrinter_Print(t *testing.T) {
	t.Run("DefaultPrint", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When printing
		err := mockEnv.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Print() error = %v, want nil", err)
		}
	})

	t.Run("CustomPrint", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom print error")
		mockEnv.PrintFunc = func() error {
			return expectedError
		}

		// When printing
		err := mockEnv.Print()

		// Then the expected error should be returned
		if err != expectedError {
			t.Errorf("Print() error = %v, want %v", err, expectedError)
		}
	})
}

// TestMockPrinter_GetAlias tests the GetAlias method of the MockEnvPrinter
func TestMockPrinter_GetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		expectedAlias := map[string]string{"test": "echo test"}
		mockEnv.GetAliasFunc = func() (map[string]string, error) {
			return expectedAlias, nil
		}

		// When getting aliases
		alias, err := mockEnv.GetAlias()

		// Then no error should be returned
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
		// And aliases should match expected values
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("GetAlias() = %v, want %v", alias, expectedAlias)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When getting aliases
		alias, err := mockEnv.GetAlias()

		// Then no error should be returned
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
		// And an empty map should be returned
		expectedAlias := map[string]string{}
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("GetAlias() = %v, want %v", alias, expectedAlias)
		}
	})
}

// TestMockEnvPrinter_PrintAlias tests the PrintAlias method of the MockEnvPrinter
func TestMockEnvPrinter_PrintAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		mockEnv.PrintAliasFunc = func() error {
			return nil
		}

		// When printing aliases
		err := mockEnv.PrintAlias()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PrintAlias() error = %v, want nil", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When printing aliases
		err := mockEnv.PrintAlias()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PrintAlias() error = %v, want nil", err)
		}
	})
}

// TestMockEnvPrinter_PostEnvHook tests the PostEnvHook method of the MockEnvPrinter
func TestMockEnvPrinter_PostEnvHook(t *testing.T) {
	t.Run("DefaultPostEnvHook", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When running post-env hook
		err := mockEnv.PostEnvHook()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() error = %v, want nil", err)
		}
	})

	t.Run("CustomPostEnvHook", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom post env hook error")
		mockEnv.PostEnvHookFunc = func() error {
			return expectedError
		}

		// When running post-env hook
		err := mockEnv.PostEnvHook()

		// Then the expected error should be returned
		if err != expectedError {
			t.Errorf("PostEnvHook() error = %v, want %v", err, expectedError)
		}
	})
}

// TestMockEnvPrinter_Reset tests the Reset method of the MockEnvPrinter
func TestMockEnvPrinter_Reset(t *testing.T) {
	t.Run("DefaultReset", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		mockEnv := NewMockEnvPrinter()

		// When resetting
		mockEnv.Reset()

		// Then no panic should occur
	})

	t.Run("CustomReset", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		mockEnv := NewMockEnvPrinter()
		resetCalled := false
		mockEnv.ResetFunc = func() {
			resetCalled = true
		}

		// When resetting
		mockEnv.Reset()

		// Then the custom reset function should be called
		if !resetCalled {
			t.Error("Reset() did not call ResetFunc")
		}
	})
}
