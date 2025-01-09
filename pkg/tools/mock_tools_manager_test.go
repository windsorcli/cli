package tools

import (
	"testing"
)

func TestMockToolsManager_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		mock := NewMockToolsManager()
		mock.InitializeFunc = func() error {
			return nil
		}
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		mock := NewMockToolsManager()
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockToolsManager_WriteManifest(t *testing.T) {
	t.Run("WriteManifest", func(t *testing.T) {
		mock := NewMockToolsManager()
		mock.WriteManifestFunc = func() error {
			return nil
		}
		err := mock.WriteManifest()
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
	})

	t.Run("NoWriteManifestFunc", func(t *testing.T) {
		mock := NewMockToolsManager()
		err := mock.WriteManifest()
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
	})
}

func TestMockToolsManager_Install(t *testing.T) {
	t.Run("Install", func(t *testing.T) {
		mock := NewMockToolsManager()
		mock.InstallFunc = func() error {
			return nil
		}
		err := mock.Install()
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})

	t.Run("NoInstallFunc", func(t *testing.T) {
		mock := NewMockToolsManager()
		err := mock.Install()
		if err != nil {
			t.Fatalf("Expected no error, got = %v", err)
		}
	})
}
