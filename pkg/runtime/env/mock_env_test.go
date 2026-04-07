package env

import (
	"fmt"
	"reflect"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockEnvPrinter_NewMockEnvPrinter tests the NewMockEnvPrinter constructor
func TestMockEnvPrinter_NewMockEnvPrinter(t *testing.T) {
	t.Run("CreateMockEnvPrinterWithoutContainer", func(t *testing.T) {
		// When creating a new mock environment without an injector
		printer := NewMockEnvPrinter()
		// Then no error should be returned
		if printer == nil {
			t.Errorf("Expected printer, got nil")
		}
	})
}

// TestMockEnvPrinter_GetEnvVars tests the GetEnvVars method of the MockEnvPrinter
func TestMockEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("DefaultGetEnvVars", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		printer := NewMockEnvPrinter()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

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
		printer := NewMockEnvPrinter()
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		printer.GetEnvVarsFunc = func() (map[string]string, error) {
			return expectedEnvVars, nil
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

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
		printer := NewMockEnvPrinter()

		// When printing
		err := printer.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Print() error = %v, want nil", err)
		}
	})

	t.Run("CustomPrint", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		printer := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom print error")
		printer.PrintFunc = func() error {
			return expectedError
		}

		// When printing
		err := printer.Print()

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
		printer := NewMockEnvPrinter()
		expectedAlias := map[string]string{"test": "echo test"}
		printer.GetAliasFunc = func() (map[string]string, error) {
			return expectedAlias, nil
		}

		// When getting aliases
		alias, err := printer.GetAlias()

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
		printer := NewMockEnvPrinter()

		// When getting aliases
		alias, err := printer.GetAlias()

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
		printer := NewMockEnvPrinter()
		printer.PrintAliasFunc = func() error {
			return nil
		}

		// When printing aliases
		err := printer.PrintAlias()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PrintAlias() error = %v, want nil", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		printer := NewMockEnvPrinter()

		// When printing aliases
		err := printer.PrintAlias()

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
		printer := NewMockEnvPrinter()

		// When running post-env hook
		err := printer.PostEnvHook()

		// Then no error should be returned
		if err != nil {
			t.Errorf("PostEnvHook() error = %v, want nil", err)
		}
	})

	t.Run("CustomPostEnvHook", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		printer := NewMockEnvPrinter()
		expectedError := fmt.Errorf("custom post env hook error")
		printer.PostEnvHookFunc = func(directory ...string) error {
			return expectedError
		}

		// When running post-env hook
		err := printer.PostEnvHook()

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
		printer := NewMockEnvPrinter()

		// When resetting
		printer.Reset()

		// Then no panic should occur
	})

	t.Run("CustomReset", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		printer := NewMockEnvPrinter()
		resetCalled := false
		printer.ResetFunc = func() {
			resetCalled = true
		}

		// When resetting
		printer.Reset()

		// Then the custom reset function should be called
		if !resetCalled {
			t.Error("Reset() did not call ResetFunc")
		}
	})
}

// TestMockEnvPrinter_GetManagedEnv tests the GetManagedEnv method of the MockEnvPrinter
func TestMockEnvPrinter_GetManagedEnv(t *testing.T) {
	t.Run("CustomGetManagedEnv", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		printer := NewMockEnvPrinter()
		expectedEnv := []string{"VAR1", "VAR2"}
		printer.GetManagedEnvFunc = func() []string {
			return expectedEnv
		}

		// When getting managed environment variables
		managedEnv := printer.GetManagedEnv()

		// Then the expected environment variables should be returned
		if !reflect.DeepEqual(managedEnv, expectedEnv) {
			t.Errorf("GetManagedEnv() = %v, want %v", managedEnv, expectedEnv)
		}
	})

	t.Run("DefaultGetManagedEnv", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		printer := NewMockEnvPrinter()

		// When getting managed environment variables
		managedEnv := printer.GetManagedEnv()

		// Then the base implementation should be used
		baseEnv := printer.BaseEnvPrinter.GetManagedEnv()
		if !reflect.DeepEqual(managedEnv, baseEnv) {
			t.Errorf("GetManagedEnv() = %v, want %v", managedEnv, baseEnv)
		}
	})
}

// TestMockEnvPrinter_GetManagedAlias tests the GetManagedAlias method of the MockEnvPrinter
func TestMockEnvPrinter_GetManagedAlias(t *testing.T) {
	t.Run("CustomGetManagedAlias", func(t *testing.T) {
		// Given a mock environment printer with custom implementation
		printer := NewMockEnvPrinter()
		expectedAlias := []string{"alias1", "alias2"}
		printer.GetManagedAliasFunc = func() []string {
			return expectedAlias
		}

		// When getting managed aliases
		managedAlias := printer.GetManagedAlias()

		// Then the expected aliases should be returned
		if !reflect.DeepEqual(managedAlias, expectedAlias) {
			t.Errorf("GetManagedAlias() = %v, want %v", managedAlias, expectedAlias)
		}
	})

	t.Run("DefaultGetManagedAlias", func(t *testing.T) {
		// Given a mock environment printer with default implementation
		printer := NewMockEnvPrinter()

		// When getting managed aliases
		managedAlias := printer.GetManagedAlias()

		// Then the base implementation should be used
		baseAlias := printer.BaseEnvPrinter.GetManagedAlias()
		if !reflect.DeepEqual(managedAlias, baseAlias) {
			t.Errorf("GetManagedAlias() = %v, want %v", managedAlias, baseAlias)
		}
	})
}
