package tools

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for mock tools manager initialization
func TestMockToolsManager_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a mock tools manager with InitializeFunc set
		mock := NewMockToolsManager()
		mock.InitializeFunc = func() error {
			return nil
		}
		// When Initialize is called
		err := mock.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a mock tools manager without InitializeFunc set
		mock := NewMockToolsManager()
		// When Initialize is called
		err := mock.Initialize()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

// Tests for mock tools manager manifest writing
func TestMockToolsManager_WriteManifest(t *testing.T) {
	t.Run("WriteManifest", func(t *testing.T) {
		// Given a mock tools manager with WriteManifestFunc set
		mock := NewMockToolsManager()
		mock.WriteManifestFunc = func() error {
			return nil
		}
		// When WriteManifest is called
		err := mock.WriteManifest()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
	})

	t.Run("NoWriteManifestFunc", func(t *testing.T) {
		// Given a mock tools manager without WriteManifestFunc set
		mock := NewMockToolsManager()
		// When WriteManifest is called
		err := mock.WriteManifest()
		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
	})
}

// Tests for mock tools manager installation
func TestMockToolsManager_Install(t *testing.T) {
	t.Run("Install", func(t *testing.T) {
		// Given a mock tools manager with InstallFunc set
		mock := NewMockToolsManager()
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
		mock := NewMockToolsManager()
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
		mock := NewMockToolsManager()
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
		mock := NewMockToolsManager()
		// When Check is called
		err := mock.Check()
		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})
}
