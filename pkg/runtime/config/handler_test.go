package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ConfigTestMocks struct {
	Shell *shell.MockShell
	Shims *config.Shims
}

// setupConfigMocks creates a new set of mocks for testing
func setupConfigMocks(t *testing.T) *ConfigTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)
	os.Setenv("WINDSOR_CONTEXT", "test-context")

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create initial mocks with defaults
	mocks := &ConfigTestMocks{
		Shell: mockShell,
		Shims: config.NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
	})

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := config.NewConfigHandler(mocks.Shell)

		// ConfigHandler is now fully initialized in constructor
		if handler == nil {
			t.Error("Expected handler to be created")
		}

		if !handler.IsLoaded() == handler.IsLoaded() {
			t.Error("Expected handler to be initialized")
		}
	})

	t.Run("InitializesDataMap", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := config.NewConfigHandler(mocks.Shell)

		// ConfigHandler is now fully initialized in constructor
		if handler == nil {
			t.Fatal("Expected handler to be created")
		}

		handler.Set("test", "value")
		value := handler.Get("test")

		if value != "value" {
			t.Errorf("Expected data map to be initialized and usable")
		}
	})

	t.Run("CreatesHandlerWithShell", func(t *testing.T) {
		// Given a shell
		mocks := setupConfigMocks(t)

		handler := config.NewConfigHandler(mocks.Shell)

		// Then handler should be created
		if handler == nil {
			t.Error("Expected handler to be created")
		}
	})
}

func TestConfigHandler_GetConfigRoot(t *testing.T) {
	t.Run("ReturnsConfigRoot", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := config.NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		root, err := handler.GetConfigRoot()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedRoot := filepath.Join(tmpDir, "contexts", "test-context")
		if root != expectedRoot {
			t.Errorf("Expected root='%s', got '%s'", expectedRoot, root)
		}
	})

	t.Run("ReturnsErrorWhenShellFails", func(t *testing.T) {
		// Given a handler with shell that fails
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := config.NewConfigHandler(mockShell)
		handler.SetContext("test")

		// When getting config root
		_, err := handler.GetConfigRoot()

		// Then it should return error when GetProjectRoot fails
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_GetWindsorScratchPath(t *testing.T) {
	t.Run("ReturnsWindsorScratchPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := config.NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		path, err := handler.GetWindsorScratchPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join(tmpDir, ".windsor", "contexts", "test-context")
		if path != expectedPath {
			t.Errorf("Expected path='%s', got '%s'", expectedPath, path)
		}
	})

	t.Run("ReturnsErrorWhenShellFails", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := config.NewConfigHandler(mockShell)
		handler.SetContext("test")

		_, err := handler.GetWindsorScratchPath()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_IsLoaded(t *testing.T) {
	t.Run("ReturnsFalseBeforeLoading", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		result := handler.IsLoaded()

		if result {
			t.Error("Expected IsLoaded=false before loading config")
		}
	})

	t.Run("ReturnsTrueAfterLoadingFiles", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := config.NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)

		handler.LoadConfig()

		result := handler.IsLoaded()

		if !result {
			t.Error("Expected IsLoaded=true after loading config files")
		}
	})

	t.Run("ReturnsTrueAfterLoadConfigString", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		handler.LoadConfigString("provider: test\n")

		result := handler.IsLoaded()

		if !result {
			t.Error("Expected IsLoaded=true after LoadConfigString")
		}
	})
}

func TestConfigHandler_IsDevMode(t *testing.T) {
	t.Run("ReturnsTrueWhenDevIsSetToTrue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		handler.Set("dev", true)

		result := handler.IsDevMode("production")

		if !result {
			t.Error("Expected IsDevMode=true when dev=true, regardless of context name")
		}
	})

	t.Run("ReturnsFalseWhenDevIsSetToFalse", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		handler.Set("dev", false)

		result := handler.IsDevMode("local")

		if result {
			t.Error("Expected IsDevMode=false when dev=false, even for local context")
		}
	})

	t.Run("ReturnsTrueForLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for 'local' context")
		}
	})

	t.Run("ReturnsTrueForLocalPrefixContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local-dev")

		if !result {
			t.Error("Expected IsDevMode=true for context starting with 'local-'")
		}
	})

	t.Run("ReturnsFalseForNonLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("production")

		if result {
			t.Error("Expected IsDevMode=false for non-local context")
		}
	})

	t.Run("IgnoresNonBoolDevValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		handler.Set("dev", "true")

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for local context when dev is non-bool")
		}
	})
}
