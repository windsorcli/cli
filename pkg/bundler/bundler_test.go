package bundler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type BundlerMocks struct {
	Injector di.Injector
	Shell    *shell.MockShell
	Shims    *Shims
	Artifact *MockArtifact
}

func setupBundlerMocks(t *testing.T) *BundlerMocks {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create injector
	injector := di.NewInjector()

	// Set up shell
	mockShell := shell.NewMockShell(injector)
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	injector.Register("shell", mockShell)

	// Create test-friendly shims
	shims := NewShims()
	shims.Stat = func(name string) (os.FileInfo, error) {
		return &mockFileInfo{name: name, isDir: true}, nil
	}
	shims.Walk = func(root string, fn filepath.WalkFunc) error {
		return nil
	}
	shims.FilepathRel = func(basepath, targpath string) (string, error) {
		return "test.txt", nil
	}
	shims.ReadFile = func(filename string) ([]byte, error) {
		return []byte("test content"), nil
	}

	// Create mock artifact
	artifact := NewMockArtifact()

	return &BundlerMocks{
		Injector: injector,
		Shell:    mockShell,
		Shims:    shims,
		Artifact: artifact,
	}
}

// =============================================================================
// Test BaseBundler
// =============================================================================

func TestBaseBundler_NewBaseBundler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given no preconditions
		// When creating a new base bundler
		bundler := NewBaseBundler()

		// Then it should not be nil
		if bundler == nil {
			t.Fatal("Expected non-nil bundler")
		}
		// And shims should be initialized
		if bundler.shims == nil {
			t.Error("Expected shims to be initialized")
		}
		// And other fields should be nil until Initialize
		if bundler.shell != nil {
			t.Error("Expected shell to be nil before Initialize")
		}
	})
}

func TestBaseBundler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a base bundler and mocks
		mocks := setupBundlerMocks(t)
		bundler := NewBaseBundler()

		// When calling Initialize
		err := bundler.Initialize(mocks.Injector)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		// And shell should be injected
		if bundler.shell == nil {
			t.Error("Expected shell to be set after Initialize")
		}
		// And injector should be stored
		if bundler.injector == nil {
			t.Error("Expected injector to be stored")
		}
	})

	t.Run("ErrorWhenShellNotFound", func(t *testing.T) {
		// Given a bundler and injector without shell
		bundler := NewBaseBundler()
		injector := di.NewInjector()
		injector.Register("shell", "not-a-shell")

		// When calling Initialize
		err := bundler.Initialize(injector)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when shell not found")
		}
		if err.Error() != "failed to resolve shell from injector" {
			t.Errorf("Expected shell resolution error, got: %v", err)
		}
	})
}

func TestBaseBundler_Bundle(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a base bundler
		mocks := setupBundlerMocks(t)
		bundler := NewBaseBundler()
		bundler.Initialize(mocks.Injector)

		// When calling Bundle
		err := bundler.Bundle(mocks.Artifact)

		// Then no error should be returned (default implementation)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}
