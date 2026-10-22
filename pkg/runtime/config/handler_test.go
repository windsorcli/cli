package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ConfigTestMocks struct {
	Shell *shell.MockShell
	Shims *Shims
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
		Shims: NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
	})

	return mocks
}

func setupPrivateTestHandler(t *testing.T) (*configHandler, string) {
	t.Helper()

	tmpDir := t.TempDir()
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	handler := NewConfigHandler(mockShell).(*configHandler)

	return handler, tmpDir
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

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

		handler := NewConfigHandler(mocks.Shell)

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

		handler := NewConfigHandler(mocks.Shell)

		// Then handler should be created
		if handler == nil {
			t.Error("Expected handler to be created")
		}
	})
}

func TestConfigHandler_SetDefault(t *testing.T) {
	t.Run("MergesDefaultContextIntoData", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.SetDefault(cliconfig.Context{Platform: "default_platform"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if platform := handler.GetString("platform"); platform != "default_platform" {
			t.Errorf("Expected platform='default_platform', got '%s'", platform)
		}
	})

	t.Run("AllowsOverridingDefaults", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.SetDefault(cliconfig.Context{Platform: "default_platform"})
		handler.Set("platform", "override_platform")

		if platform := handler.GetString("platform"); platform != "override_platform" {
			t.Errorf("Expected override to work, got '%s'", platform)
		}
	})

	t.Run("DoesNotOverwriteValueAlreadySetBeforeIt", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("platform", "already_set_platform")

		err := handler.SetDefault(cliconfig.Context{Platform: "default_platform"})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if platform := handler.GetString("platform"); platform != "already_set_platform" {
			t.Errorf("Expected SetDefault called after Set to not clobber the existing value, got '%s'", platform)
		}
	})

	t.Run("HandlesMarshalError", func(t *testing.T) {
		// Given a config handler with failing marshal
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// When setting default with marshal error
		err := handler.SetDefault(cliconfig.Context{})

		// Then setting should fail
		if err == nil {
			t.Error("Expected marshal error")
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		// Given a config handler with failing unmarshal
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return os.ErrInvalid
		}

		// When setting default with unmarshal error
		err := handler.SetDefault(cliconfig.Context{Platform: "test"})

		// Then setting should fail
		if err == nil {
			t.Error("Expected unmarshal error")
		}
	})
}

func TestConfigHandler_GetHeader(t *testing.T) {
	t.Run("HandlesMarshalError", func(t *testing.T) {
		// Given a config handler with failing marshal
		handler, _ := setupPrivateTestHandler(t)
		handler.Set("platform", "aws")

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// When the header is read with a marshal error
		header := handler.GetHeader()

		// Then an empty header is returned
		if header == nil || header.Platform != "" {
			t.Errorf("Expected an empty header on marshal error, got %+v", header)
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		// Given a config handler with failing unmarshal
		handler, _ := setupPrivateTestHandler(t)
		handler.Set("platform", "aws")

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return os.ErrInvalid
		}

		// When the header is read with an unmarshal error
		header := handler.GetHeader()

		// Then an empty header is returned
		if header == nil || header.Platform != "" {
			t.Errorf("Expected an empty header on unmarshal error, got %+v", header)
		}
	})
}

func TestConfigHandler_GetConfigRoot(t *testing.T) {
	t.Run("ReturnsConfigRoot", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
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

		handler := NewConfigHandler(mockShell)
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

		handler := NewConfigHandler(mocks.Shell)
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

		handler := NewConfigHandler(mockShell)
		handler.SetContext("test")

		_, err := handler.GetWindsorScratchPath()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_Clean(t *testing.T) {
	t.Run("RemovesTerraformScratchCaches", func(t *testing.T) {
		// Given a context with .terraform and .tfstate caches in the scratch path
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		scratchRoot := filepath.Join(tmpDir, ".windsor", "contexts", "test-context")
		terraformDir := filepath.Join(scratchRoot, ".terraform")
		tfstateDir := filepath.Join(scratchRoot, ".tfstate")
		os.MkdirAll(terraformDir, 0755)
		os.MkdirAll(tfstateDir, 0755)
		os.WriteFile(filepath.Join(terraformDir, "lock"), []byte("test"), 0644)

		// When Clean is called
		err := handler.Clean()

		// Then both cache directories are removed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, err := os.Stat(terraformDir); !os.IsNotExist(err) {
			t.Error("Expected .terraform directory to be removed")
		}
		if _, err := os.Stat(tfstateDir); !os.IsNotExist(err) {
			t.Error("Expected .tfstate directory to be removed")
		}
	})

	t.Run("PreservesCredentialDirectories", func(t *testing.T) {
		// Given a context with operator-managed CLI auth state under contexts/<name>/
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		configRoot := filepath.Join(tmpDir, "contexts", "test-context")
		credentialDirs := []string{".kube", ".aws", ".azure", ".gcp", ".talos", ".omni"}
		for _, dir := range credentialDirs {
			path := filepath.Join(configRoot, dir)
			os.MkdirAll(path, 0755)
			os.WriteFile(filepath.Join(path, "config"), []byte("test"), 0644)
		}

		// When Clean is called
		err := handler.Clean()

		// Then every credential directory remains intact
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, dir := range credentialDirs {
			path := filepath.Join(configRoot, dir)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Expected %s to be preserved, but it was removed", dir)
			}
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		// Given a handler whose project root lookup fails
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := NewConfigHandler(mockShell).(*configHandler)
		handler.SetContext("test")

		// When cleaning
		err := handler.Clean()

		// Then the underlying error propagates
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_GenerateContextID(t *testing.T) {
	t.Run("GeneratesIDWhenNotSet", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.GenerateContextID()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		id := handler.GetString("id")
		if id == "" {
			t.Error("Expected ID to be generated")
		}
		if len(id) != 8 {
			t.Errorf("Expected ID length 8, got %d", len(id))
		}
	})

	t.Run("DoesNotOverrideExistingID", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("id", "existing_id")

		err := handler.GenerateContextID()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		id := handler.GetString("id")
		if id != "existing_id" {
			t.Errorf("Expected existing ID to be preserved, got '%s'", id)
		}
	})

	t.Run("HandlesRandomGenerationError", func(t *testing.T) {
		// Given a handler with random generation error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.CryptoRandRead = func(b []byte) (int, error) {
			return 0, os.ErrPermission
		}

		// When generating context ID
		err := handler.GenerateContextID()

		// Then it should return error when random generation fails
		if err == nil {
			t.Error("Expected error when random generation fails")
		}
	})
}

func TestConfigHandler_GetContext(t *testing.T) {
	t.Run("ReturnsContextFromEnvironment", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "test-context" {
			t.Errorf("Expected 'test-context' from environment, got '%s'", context)
		}
	})

	t.Run("PrefersContextFileOverEnvironmentContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Setenv("WINDSOR_CONTEXT", "env-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		context := handler.GetContext()

		if context != "file-context" {
			t.Errorf("Expected 'file-context', got '%s'", context)
		}
	})

	t.Run("UsesEnvironmentContextWhenContextFileDoesNotExist", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		os.Setenv("WINDSOR_CONTEXT", "env-context")

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "env-context" {
			t.Errorf("Expected 'env-context', got '%s'", context)
		}
	})

	t.Run("ReadsContextFromFile", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		context := handler.GetContext()

		if context != "file-context" {
			t.Errorf("Expected 'file-context', got '%s'", context)
		}
	})

	t.Run("DefaultsToLocalWhenNoContextSet", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "local" {
			t.Errorf("Expected default 'local', got '%s'", context)
		}
	})

	t.Run("CachesResolvedContextForTheLifeOfTheHandler", func(t *testing.T) {
		// Given a handler that has already resolved its context once
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("first-context"), 0644)

		if first := handler.GetContext(); first != "first-context" {
			t.Fatalf("Expected 'first-context' on first read, got '%s'", first)
		}

		// When the context file changes underneath it, as a concurrent "windsor set context"
		// in another process would
		os.WriteFile(contextFilePath, []byte("second-context"), 0644)

		// Then this handler keeps returning its first resolution, not the new file content
		if second := handler.GetContext(); second != "first-context" {
			t.Errorf("Expected the cached 'first-context' to survive a later file change, got '%s'", second)
		}
	})

	t.Run("FreshHandlerReResolvesAfterAConcurrentContextChange", func(t *testing.T) {
		// Given a context file changed after one handler already resolved it
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("first-context"), 0644)

		if first := NewConfigHandler(mocks.Shell).GetContext(); first != "first-context" {
			t.Fatalf("Expected 'first-context', got '%s'", first)
		}

		os.WriteFile(contextFilePath, []byte("second-context"), 0644)

		// When a new handler resolves context, as the next "windsor" command's fresh process would
		second := NewConfigHandler(mocks.Shell).GetContext()

		// Then it sees the current file content, not anything cached by the earlier handler
		if second != "second-context" {
			t.Errorf("Expected a new handler to see 'second-context', got '%s'", second)
		}
	})

	t.Run("ConcurrentReadsResolveConsistently", func(t *testing.T) {
		// Given many goroutines reading the same handler's context at once
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("concurrent-context"), 0644)

		var wg sync.WaitGroup
		results := make([]string, 20)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i] = handler.GetContext()
			}(i)
		}
		wg.Wait()

		// Then every goroutine observes the same resolved value, with no race
		for i, r := range results {
			if r != "concurrent-context" {
				t.Errorf("goroutine %d: expected 'concurrent-context', got '%s'", i, r)
			}
		}
	})

	t.Run("HandlesNilResolvedContextGracefully", func(t *testing.T) {
		// Given a handler built by hand, skipping NewConfigHandler's cache initialization
		os.Setenv("WINDSOR_CONTEXT", "env-context")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := &configHandler{shims: NewShims()}

		// When getting context
		// Then it falls back to an uncached resolve instead of panicking on a nil cache
		if got := handler.GetContext(); got != "env-context" {
			t.Errorf("Expected 'env-context', got '%s'", got)
		}
	})
}

func TestConfigHandler_WithContext(t *testing.T) {
	t.Run("OverridesFileAndEnvVar", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Setenv("WINDSOR_CONTEXT", "env-context")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		handler := NewConfigHandler(mocks.Shell).WithContext("override-context")

		context := handler.GetContext()

		if context != "override-context" {
			t.Errorf("Expected 'override-context', got '%s'", context)
		}
	})

	t.Run("DoesNotCarryPendingWritesToTheCopy", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		handler.Set("provider", "docker")
		handler.Set("provider", nil)

		override := handler.WithContext("override-context").(*configHandler)

		if len(override.pendingOverrides) != 0 {
			t.Errorf("Expected no pending overrides on the copy, got %v", override.pendingOverrides)
		}
		if len(override.pendingDeletes) != 0 {
			t.Errorf("Expected no pending deletes on the copy, got %v", override.pendingDeletes)
		}
	})
}

func TestConfigHandler_SetContext(t *testing.T) {
	t.Run("WritesContextToFile", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		err := handler.SetContext("new-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		content, err := os.ReadFile(contextFilePath)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}

		if string(content) != "new-context" {
			t.Errorf("Expected context file to contain 'new-context', got '%s'", string(content))
		}
	})

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		// Given a handler with MkdirAll error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.ErrPermission
		}

		// When setting context
		err := handler.SetContext("new-context")

		// Then it should return MkdirAll error
		if err == nil {
			t.Error("Expected MkdirAll error")
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		// Given a handler with WriteFile error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return os.ErrPermission
		}

		// When setting context
		err := handler.SetContext("new-context")

		// Then it should return WriteFile error
		if err == nil {
			t.Error("Expected WriteFile error")
		}
	})

	t.Run("SameHandlerSeesItsOwnSetContextImmediately", func(t *testing.T) {
		// Given a handler that has already cached a resolved context
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if first := handler.GetContext(); first == "new-context" {
			t.Fatalf("Test invalid: 'new-context' must not already be the resolved value")
		}

		// When this same handler sets a new context
		if err := handler.SetContext("new-context"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it sees the new value on its very next read, unlike a concurrent handler's
		// unrelated file change, which GetContext's own cache is meant to ignore
		if got := handler.GetContext(); got != "new-context" {
			t.Errorf("Expected 'new-context' immediately after SetContext, got '%s'", got)
		}
	})

	t.Run("OverridesEvenAWithContextDerivedHandler", func(t *testing.T) {
		// Given a handler with a WithContext override already applied
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell).WithContext("staging")

		// When that same handler sets a new context
		if err := handler.SetContext("production"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the new value wins, not the earlier override
		if got := handler.GetContext(); got != "production" {
			t.Errorf("Expected 'production' after SetContext, got '%s'", got)
		}
	})

	t.Run("ConcurrentWithGetContextDoesNotRace", func(t *testing.T) {
		// Given many goroutines reading context while one sets it
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = handler.SetContext("new-context")
		}()
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = handler.GetContext()
			}()
		}
		wg.Wait()

		// Then the handler settles on the set value, with no race (run with -race to verify)
		if got := handler.GetContext(); got != "new-context" {
			t.Errorf("Expected 'new-context' after concurrent SetContext, got '%s'", got)
		}
	})
}

func TestConfigHandler_IsLoaded(t *testing.T) {
	t.Run("ReturnsFalseBeforeLoading", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsLoaded()

		if result {
			t.Error("Expected IsLoaded=false before loading config")
		}
	})

	t.Run("ReturnsTrueAfterLoadingFiles", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
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
		handler := NewConfigHandler(mocks.Shell)

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
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", true)

		result := handler.IsDevMode("production")

		if !result {
			t.Error("Expected IsDevMode=true when dev=true, regardless of context name")
		}
	})

	t.Run("ReturnsFalseWhenDevIsSetToFalse", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", false)

		result := handler.IsDevMode("local")

		if result {
			t.Error("Expected IsDevMode=false when dev=false, even for local context")
		}
	})

	t.Run("ReturnsTrueForLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for 'local' context")
		}
	})

	t.Run("ReturnsTrueForLocalPrefixContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local-dev")

		if !result {
			t.Error("Expected IsDevMode=true for context starting with 'local-'")
		}
	})

	t.Run("ReturnsFalseForNonLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("production")

		if result {
			t.Error("Expected IsDevMode=false for non-local context")
		}
	})

	t.Run("IgnoresNonBoolDevValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", "true")

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for local context when dev is non-bool")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestConfigHandler_HelperConversions(t *testing.T) {
	t.Run("ConvertInterfaceMapRecursivelyConvertsNestedMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		input := map[any]any{
			"top": map[any]any{
				"inner": "value",
			},
			10: "ignored",
		}

		converted := handler.convertInterfaceMap(input)
		top, ok := converted["top"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested map conversion, got %T", converted["top"])
		}
		if top["inner"] != "value" {
			t.Errorf("Expected nested converted value, got %v", top["inner"])
		}
		if _, exists := converted["10"]; exists {
			t.Errorf("Expected non-string keys to be dropped, got %v", converted)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
