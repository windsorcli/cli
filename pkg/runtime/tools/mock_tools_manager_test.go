package tools_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for mock tools manager installation
func TestMockToolsManager_Install(t *testing.T) {
	t.Run("Install", func(t *testing.T) {
		// Given a mock tools manager with InstallFunc set
		mock := tools.NewMockToolsManager()
		mock.InstallFunc = func() error {
			return nil
		}
		// When Install is called
		err := mock.Install()
		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})

	t.Run("NoInstallFunc", func(t *testing.T) {
		// Given a mock tools manager without InstallFunc set
		mock := tools.NewMockToolsManager()
		// When Install is called
		err := mock.Install()
		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})
}

// Tests for mock tools manager version checking
func TestMockToolsManager_Check(t *testing.T) {
	t.Run("Check", func(t *testing.T) {
		// Given a mock tools manager with CheckFunc set
		mock := tools.NewMockToolsManager()
		mock.CheckFunc = func() error {
			return nil
		}
		// When Check is called
		err := mock.Check()
		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})

	t.Run("NoCheckFunc", func(t *testing.T) {
		// Given a mock tools manager without CheckFunc set
		mock := tools.NewMockToolsManager()
		// When Check is called
		err := mock.Check()
		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})
}
