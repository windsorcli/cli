package shell

import (
	"fmt"
	"testing"
	"time"
)

// The MockShellTest is a test suite for the MockShell implementation.
// It provides comprehensive test coverage for mock shell operations,
// ensuring reliable testing of shell-dependent functionality.
// The MockShellTest validates the mock implementation's behavior.

// =============================================================================
// Test Setup
// =============================================================================

// setupMockShellMocks creates a new set of mocks for testing MockShell
func setupMockShellMocks(t *testing.T) *MockShell {
	t.Helper()

	// Create mock shell
	mockShell := NewMockShell()

	return mockShell
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockShell_NewMockShell tests the constructor for MockShell
func TestMockShell_NewMockShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockShell := setupMockShellMocks(t)

		// Then the mock shell should be created successfully
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})
}

// TestMockShell_Initialize tests the Initialize method of MockShell
func TestMockShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell
		mockShell := setupMockShellMocks(t)

		// Then it should be created
		if mockShell == nil {
			t.Error("Expected mock shell to be created")
		}
	})

	t.Run("Created", func(t *testing.T) {
		// Given a mock shell
		mockShell := setupMockShellMocks(t)

		// Then it should be created
		if mockShell == nil {
			t.Error("Expected mock shell to be created")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell
		mockShell := setupMockShellMocks(t)

		// Then it should be created
		if mockShell == nil {
			t.Error("Expected mock shell to be created")
		}
	})
}

// TestMockShell_GetProjectRoot tests the GetProjectRoot method of MockShell
func TestMockShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with GetProjectRootFunc set
		mockShell := setupMockShellMocks(t)
		expectedRoot := "/mock/project/root"
		mockShell.GetProjectRootFunc = func() (string, error) {
			return expectedRoot, nil
		}

		// When calling GetProjectRoot
		root, err := mockShell.GetProjectRoot()

		// Then no error should be returned and the root should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != expectedRoot {
			t.Errorf("Expected root %v, got %v", expectedRoot, root)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with GetProjectRootFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock get project root error")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", expectedError
		}

		// When calling GetProjectRoot
		root, err := mockShell.GetProjectRoot()

		// Then the expected error should be returned and root should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if root != "" {
			t.Errorf("Expected empty root, got %v", root)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with GetProjectRootFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling GetProjectRoot
		root, err := mockShell.GetProjectRoot()

		// Then no error should be returned and root should be empty (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if root != "" {
			t.Errorf("Expected empty root, got %v", root)
		}
	})
}

// TestMockShell_Exec tests the Exec method of MockShell
func TestMockShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with ExecFunc set
		mockShell := setupMockShellMocks(t)
		expectedOutput := "mock output"
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return expectedOutput, nil
		}

		// When calling Exec
		output, err := mockShell.Exec("mock-command", "arg1", "arg2")

		// Then no error should be returned and the output should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with ExecFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock exec error")
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", expectedError
		}

		// When calling Exec
		output, err := mockShell.Exec("mock-command", "arg1", "arg2")

		// Then the expected error should be returned and output should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with ExecFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling Exec
		output, err := mockShell.Exec("mock-command", "arg1", "arg2")

		// Then no error should be returned and output should be empty (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})
}

// TestMockShell_ExecSudo tests the ExecSudo method of MockShell
func TestMockShell_ExecSudo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with ExecSudoFunc set
		mockShell := setupMockShellMocks(t)
		expectedOutput := "mock sudo output"
		mockShell.ExecSudoFunc = func(sudoPrompt string, command string, args ...string) (string, error) {
			return expectedOutput, nil
		}

		// When calling ExecSudo
		output, err := mockShell.ExecSudo("mock-sudo-prompt", "mock-command", "arg1", "arg2")

		// Then no error should be returned and the output should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with ExecSudoFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock exec sudo error")
		mockShell.ExecSudoFunc = func(sudoPrompt string, command string, args ...string) (string, error) {
			return "", expectedError
		}

		// When calling ExecSudo
		output, err := mockShell.ExecSudo("mock-sudo-prompt", "mock-command", "arg1", "arg2")

		// Then the expected error should be returned and output should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with ExecSudoFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling ExecSudo
		output, err := mockShell.ExecSudo("mock-sudo-prompt", "mock-command", "arg1", "arg2")

		// Then no error should be returned and output should be empty (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})
}

// TestMockShell_ExecSudoError tests the ExecSudo method of MockShell when it returns an error
func TestMockShell_ExecSudoError(t *testing.T) {
	// Given a mock shell with ExecSudoFunc set to return an error
	mockShell := setupMockShellMocks(t)
	expectedError := fmt.Errorf("mock exec sudo error")
	mockShell.ExecSudoFunc = func(sudoPrompt string, command string, args ...string) (string, error) {
		return "", expectedError
	}

	// When calling ExecSudo
	output, err := mockShell.ExecSudo("mock-sudo-prompt", "mock-command", "arg1", "arg2")

	// Then the expected error should be returned and output should be empty
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_ExecSudoNotImplemented tests the ExecSudo method of MockShell when not implemented
func TestMockShell_ExecSudoNotImplemented(t *testing.T) {
	// Given a mock shell with ExecSudoFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling ExecSudo
	output, err := mockShell.ExecSudo("mock-sudo-prompt", "mock-command", "arg1", "arg2")

	// Then no error should be returned and output should be empty (default implementation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_ExecSilent tests the ExecSilent method of MockShell
func TestMockShell_ExecSilent(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with ExecSilentFunc set
		mockShell := setupMockShellMocks(t)
		expectedOutput := "mock silent output"
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return expectedOutput, nil
		}

		// When calling ExecSilent
		output, err := mockShell.ExecSilent("mock-command", "arg1", "arg2")

		// Then no error should be returned and the output should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with ExecSilentFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock exec silent error")
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", expectedError
		}

		// When calling ExecSilent
		output, err := mockShell.ExecSilent("mock-command", "arg1", "arg2")

		// Then the expected error should be returned and output should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with ExecSilentFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling ExecSilent
		output, err := mockShell.ExecSilent("mock-command", "arg1", "arg2")

		// Then no error should be returned and output should be empty (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})
}

// TestMockShell_ExecSilentError tests the ExecSilent method of MockShell when it returns an error
func TestMockShell_ExecSilentError(t *testing.T) {
	// Given a mock shell with ExecSilentFunc set to return an error
	mockShell := setupMockShellMocks(t)
	expectedError := fmt.Errorf("mock exec silent error")
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "", expectedError
	}

	// When calling ExecSilent
	output, err := mockShell.ExecSilent("mock-command", "arg1", "arg2")

	// Then the expected error should be returned and output should be empty
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_ExecSilentNotImplemented tests the ExecSilent method of MockShell when not implemented
func TestMockShell_ExecSilentNotImplemented(t *testing.T) {
	// Given a mock shell with ExecSilentFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling ExecSilent
	output, err := mockShell.ExecSilent("mock-command", "arg1", "arg2")

	// Then no error should be returned and output should be empty (default implementation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_ExecSilentWithTimeout tests the ExecSilentWithTimeout method of MockShell
func TestMockShell_ExecSilentWithTimeout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockShell := setupMockShellMocks(t)
		expectedOutput := "mock output"
		mockShell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return expectedOutput, nil
		}

		output, err := mockShell.ExecSilentWithTimeout("mock-command", []string{"arg1", "arg2"}, 5*time.Second)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock timeout error")
		mockShell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
			return "", expectedError
		}

		output, err := mockShell.ExecSilentWithTimeout("mock-command", []string{"arg1", "arg2"}, 5*time.Second)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})

	t.Run("DelegatesToExecSilent", func(t *testing.T) {
		mockShell := setupMockShellMocks(t)
		expectedOutput := "delegated output"
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return expectedOutput, nil
		}

		output, err := mockShell.ExecSilentWithTimeout("mock-command", []string{"arg1", "arg2"}, 5*time.Second)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})
}

// TestMockShell_ExecProgress tests the ExecProgress method of MockShell
func TestMockShell_ExecProgress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with ExecProgressFunc set
		mockShell := setupMockShellMocks(t)
		expectedOutput := "mock progress output"
		mockShell.ExecProgressFunc = func(progressPrompt string, command string, args ...string) (string, error) {
			return expectedOutput, nil
		}

		// When calling ExecProgress
		output, err := mockShell.ExecProgress("mock-progress-prompt", "mock-command", "arg1", "arg2")

		// Then no error should be returned and the output should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected output %v, got %v", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with ExecProgressFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock exec progress error")
		mockShell.ExecProgressFunc = func(progressPrompt string, command string, args ...string) (string, error) {
			return "", expectedError
		}

		// When calling ExecProgress
		output, err := mockShell.ExecProgress("mock-progress-prompt", "mock-command", "arg1", "arg2")

		// Then the expected error should be returned and output should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with ExecProgressFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling ExecProgress
		output, err := mockShell.ExecProgress("mock-progress-prompt", "mock-command", "arg1", "arg2")

		// Then no error should be returned and output should be empty (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if output != "" {
			t.Errorf("Expected empty output, got %v", output)
		}
	})
}

// TestMockShell_ExecProgressError tests the ExecProgress method of MockShell when it returns an error
func TestMockShell_ExecProgressError(t *testing.T) {
	// Given a mock shell with ExecProgressFunc set to return an error
	mockShell := setupMockShellMocks(t)
	expectedError := fmt.Errorf("mock exec progress error")
	mockShell.ExecProgressFunc = func(progressPrompt string, command string, args ...string) (string, error) {
		return "", expectedError
	}

	// When calling ExecProgress
	output, err := mockShell.ExecProgress("mock-progress-prompt", "mock-command", "arg1", "arg2")

	// Then the expected error should be returned and output should be empty
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_ExecProgressNotImplemented tests the ExecProgress method of MockShell when not implemented
func TestMockShell_ExecProgressNotImplemented(t *testing.T) {
	// Given a mock shell with ExecProgressFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling ExecProgress
	output, err := mockShell.ExecProgress("mock-progress-prompt", "mock-command", "arg1", "arg2")

	// Then no error should be returned and output should be empty (default implementation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got %v", output)
	}
}

// TestMockShell_InstallHook tests the InstallHook method of MockShell
func TestMockShell_InstallHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with InstallHookFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.InstallHookFunc = func(shellType string) error {
			return nil
		}

		// When calling InstallHook
		err := mockShell.InstallHook("mock-shell-type")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with InstallHookFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock install hook error")
		mockShell.InstallHookFunc = func(shellType string) error {
			return expectedError
		}

		// When calling InstallHook
		err := mockShell.InstallHook("mock-shell-type")

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with InstallHookFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling InstallHook
		err := mockShell.InstallHook("mock-shell-type")

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockShell_InstallHookError tests the InstallHook method of MockShell when it returns an error
func TestMockShell_InstallHookError(t *testing.T) {
	// Given a mock shell with InstallHookFunc set to return an error
	mockShell := setupMockShellMocks(t)
	expectedError := fmt.Errorf("mock install hook error")
	mockShell.InstallHookFunc = func(shellType string) error {
		return expectedError
	}

	// When calling InstallHook
	err := mockShell.InstallHook("mock-shell-type")

	// Then the expected error should be returned
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}

// TestMockShell_InstallHookNotImplemented tests the InstallHook method of MockShell when not implemented
func TestMockShell_InstallHookNotImplemented(t *testing.T) {
	// Given a mock shell with InstallHookFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling InstallHook
	err := mockShell.InstallHook("mock-shell-type")

	// Then no error should be returned (default implementation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestMockShell_SetVerbosity tests the SetVerbosity method of MockShell
func TestMockShell_SetVerbosity(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with SetVerbosityFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.SetVerbosityFunc = func(verbose bool) {
			// No-op implementation
		}

		// When calling SetVerbosity
		mockShell.SetVerbosity(true)

		// Then no error should be returned
		// (SetVerbosity doesn't return an error, so we just verify it doesn't panic)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with SetVerbosityFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling SetVerbosity
		// Then it should not panic
		mockShell.SetVerbosity(true)
	})
}

// TestMockShell_SetVerbosityNotImplemented tests the SetVerbosity method of MockShell when not implemented
func TestMockShell_SetVerbosityNotImplemented(t *testing.T) {
	// Given a mock shell with SetVerbosityFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling SetVerbosity
	// Then it should not panic
	mockShell.SetVerbosity(true)
}

// TestMockShell_UnsetEnvs tests the UnsetEnvs method of MockShell
func TestMockShell_UnsetEnvs(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with UnsetEnvsFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.UnsetEnvsFunc = func(envVars []string) {
			// No-op implementation
		}

		// When calling UnsetEnvs
		mockShell.UnsetEnvs([]string{"ENV1", "ENV2"})

		// Then no error should be returned
		// (UnsetEnvs doesn't return an error, so we just verify it doesn't panic)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with UnsetEnvsFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling UnsetEnvs
		// Then it should not panic
		mockShell.UnsetEnvs([]string{"ENV1", "ENV2"})
	})
}

// TestMockShell_UnsetEnvsNotImplemented tests the UnsetEnvs method of MockShell when not implemented
func TestMockShell_UnsetEnvsNotImplemented(t *testing.T) {
	// Given a mock shell with UnsetEnvsFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling UnsetEnvs
	// Then it should not panic
	mockShell.UnsetEnvs([]string{"ENV1", "ENV2"})
}

// TestMockShell_UnsetAlias tests the UnsetAlias method of MockShell
func TestMockShell_UnsetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with UnsetAliasFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.UnsetAliasFunc = func(aliases []string) {
			// No-op implementation
		}

		// When calling UnsetAlias
		mockShell.UnsetAlias([]string{"ALIAS1", "ALIAS2"})

		// Then no error should be returned
		// (UnsetAlias doesn't return an error, so we just verify it doesn't panic)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with UnsetAliasFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling UnsetAlias
		// Then it should not panic
		mockShell.UnsetAlias([]string{"ALIAS1", "ALIAS2"})
	})
}

// TestMockShell_UnsetAliasNotImplemented tests the UnsetAlias method of MockShell when not implemented
func TestMockShell_UnsetAliasNotImplemented(t *testing.T) {
	// Given a mock shell with UnsetAliasFunc not set
	mockShell := setupMockShellMocks(t)

	// When calling UnsetAlias
	// Then it should not panic
	mockShell.UnsetAlias([]string{"ALIAS1", "ALIAS2"})
}

// TestMockShell_CheckTrustedDirectory tests the CheckTrustedDirectory method of MockShell
func TestMockShell_CheckTrustedDirectory(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with CheckTrustedDirectoryFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		// When calling CheckTrustedDirectory
		err := mockShell.CheckTrustedDirectory()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with CheckTrustedDirectoryFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock check trusted directory error")
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return expectedError
		}

		// When calling CheckTrustedDirectory
		err := mockShell.CheckTrustedDirectory()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with CheckTrustedDirectoryFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling CheckTrustedDirectory
		err := mockShell.CheckTrustedDirectory()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockShell_AddCurrentDirToTrustedFile tests the AddCurrentDirToTrustedFile method of MockShell
func TestMockShell_AddCurrentDirToTrustedFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with AddCurrentDirToTrustedFileFunc set
		mockShell := setupMockShellMocks(t)
		mockShell.AddCurrentDirToTrustedFileFunc = func() error {
			return nil
		}

		// When calling AddCurrentDirToTrustedFile
		err := mockShell.AddCurrentDirToTrustedFile()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with AddCurrentDirToTrustedFileFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock add current dir to trusted file error")
		mockShell.AddCurrentDirToTrustedFileFunc = func() error {
			return expectedError
		}

		// When calling AddCurrentDirToTrustedFile
		err := mockShell.AddCurrentDirToTrustedFile()

		// Then the expected error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with AddCurrentDirToTrustedFileFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling AddCurrentDirToTrustedFile
		err := mockShell.AddCurrentDirToTrustedFile()

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// TestMockShell_WriteResetToken tests the WriteResetToken method of MockShell
func TestMockShell_WriteResetToken(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with WriteResetTokenFunc set
		mockShell := setupMockShellMocks(t)
		expectedPath := "/mock/reset/token/path"
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return expectedPath, nil
		}

		// When calling WriteResetToken
		path, err := mockShell.WriteResetToken()

		// Then no error should be returned and the path should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if path != expectedPath {
			t.Errorf("Expected path %v, got %v", expectedPath, path)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with WriteResetTokenFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock write reset token error")
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "", expectedError
		}

		// When calling WriteResetToken
		path, err := mockShell.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %v", path)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with WriteResetTokenFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling WriteResetToken
		path, err := mockShell.WriteResetToken()

		// Then an error should be returned and path should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedError := "WriteResetToken not implemented"
		if err.Error() != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got %v", path)
		}
	})
}

// TestMockShell_CheckReset tests the MockShell's CheckReset method
func TestMockShell_CheckReset(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mockShell := setupMockShellMocks(t)

		// Configure the mock to return a success response
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return true, nil
		}

		// When
		result, err := mockShell.CheckResetFlags()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !result {
			t.Errorf("Expected result to be true, got false")
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given
		mockShell := setupMockShellMocks(t)

		// Configure the mock to return an error
		expectedError := fmt.Errorf("custom error")
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, expectedError
		}

		// When
		result, err := mockShell.CheckResetFlags()

		// Then
		if err == nil || err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result {
			t.Errorf("Expected result to be false, got true")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given
		mockShell := setupMockShellMocks(t)

		// When
		result, err := mockShell.CheckResetFlags()

		// Then
		if err != nil {
			t.Error("Expected no error, got error")
		}
		if result {
			t.Error("Expected result to be false, got true")
		}
	})
}

// TestMockShell_GetSessionToken tests the GetSessionToken method of MockShell
func TestMockShell_GetSessionToken(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with GetSessionTokenFunc set
		mockShell := setupMockShellMocks(t)
		expectedToken := "mock-session-token"
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return expectedToken, nil
		}

		// When calling GetSessionToken
		token, err := mockShell.GetSessionToken()

		// Then no error should be returned and the token should match
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if token != expectedToken {
			t.Errorf("Expected token %v, got %v", expectedToken, token)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell with GetSessionTokenFunc set to return an error
		mockShell := setupMockShellMocks(t)
		expectedError := fmt.Errorf("mock get session token error")
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "", expectedError
		}

		// When calling GetSessionToken
		token, err := mockShell.GetSessionToken()

		// Then the expected error should be returned and token should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if token != "" {
			t.Errorf("Expected empty token, got %v", token)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with GetSessionTokenFunc not set
		mockShell := setupMockShellMocks(t)

		// When calling GetSessionToken
		token, err := mockShell.GetSessionToken()

		// Then the expected error should be returned and token should be empty
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "GetSessionToken not implemented" {
			t.Errorf("Expected error 'GetSessionToken not implemented', got %v", err)
		}
		if token != "" {
			t.Errorf("Expected empty token, got %v", token)
		}
	})
}

func TestMockShell_Reset(t *testing.T) {
	t.Run("CallsResetFunc", func(t *testing.T) {
		// Given a mock shell with ResetFunc set
		mockShell := setupMockShellMocks(t)
		called := false
		mockShell.ResetFunc = func(...bool) {
			called = true
		}

		// When Reset is called
		mockShell.Reset()

		// Then the mock function should be called
		if !called {
			t.Error("Expected ResetFunc to be called")
		}
	})

	t.Run("NilFuncDoesNotPanic", func(t *testing.T) {
		// Given a mock shell without ResetFunc set
		mockShell := setupMockShellMocks(t)

		// When Reset is called
		// Then it should not panic
		mockShell.Reset()
	})
}

func TestMockShell_RegisterSecret(t *testing.T) {
	t.Run("CallsRegisterSecretFunc", func(t *testing.T) {
		// Given a mock shell with RegisterSecretFunc configured to track calls
		mockShell := setupMockShellMocks(t)
		called := false
		expectedValue := "test-secret-value"
		var receivedValue string
		mockShell.RegisterSecretFunc = func(value string) {
			called = true
			receivedValue = value
		}

		// When RegisterSecret is called with a secret value
		mockShell.RegisterSecret(expectedValue)

		// Then the mock function should be called with the expected secret value
		if !called {
			t.Error("Expected RegisterSecretFunc to be called")
		}
		if receivedValue != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, receivedValue)
		}
	})

	t.Run("NilFuncDoesNotPanic", func(t *testing.T) {
		// Given a mock shell without RegisterSecretFunc configured
		mockShell := setupMockShellMocks(t)

		// When RegisterSecret is called with no function set
		// Then it should not panic and handle the nil function gracefully
		mockShell.RegisterSecret("test-secret")
	})
}
