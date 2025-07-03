package generators

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/bundler"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/zclconf/go-cty/cty"
)

// =============================================================================
// Test Setup
// =============================================================================

type simpleDirEntry struct {
	name  string
	isDir bool
}

func (s *simpleDirEntry) Name() string {
	return s.name
}

func (s *simpleDirEntry) IsDir() bool {
	return s.isDir
}

func (s *simpleDirEntry) Type() fs.FileMode {
	if s.isDir {
		return fs.ModeDir
	}
	return 0
}

func (s *simpleDirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name  string
	isDir bool
	mode  os.FileMode
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

// mockDirEntry implements os.DirEntry for testing
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string      { return m.name }
func (m *mockDirEntry) IsDir() bool       { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{name: m.name, isDir: m.isDir}, nil
}

// createMockFile creates a temporary file that can be used in tests
func createMockFile() (*os.File, error) {
	return os.CreateTemp("", "mock-test-file-*")
}

// setupTerraformGeneratorMocks extends base mocks with terraform generator specific mocking
func setupTerraformGeneratorMocks(mocks *Mocks) {
	// Tar extraction mocks
	mocks.Shims.NewBytesReader = func(data []byte) io.Reader {
		return bytes.NewReader(data)
	}
	mocks.Shims.NewTarReader = func(r io.Reader) *tar.Reader {
		return tar.NewReader(r)
	}
	mocks.Shims.EOFError = func() error {
		return io.EOF
	}
	mocks.Shims.TypeDir = func() byte {
		return tar.TypeDir
	}
	mocks.Shims.Create = func(path string) (*os.File, error) {
		return os.Create(path)
	}
	mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
		return io.Copy(dst, src)
	}
	mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
		return nil // Default to successful chmod
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{component}
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to process terraform templates: failed to get project root: error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create terraform directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get config root: mock error getting config root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("DeletesTerraformDirOnReset", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And .terraform directory exists
		var removedPath string
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			if strings.HasSuffix(path, ".terraform") {
				return nil, nil // exists
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.RemoveAll = func(path string) error {
			removedPath = path
			return nil
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error { return nil }
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error { return nil }
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) { return []byte{}, nil }
		mocks.Shims.Chdir = func(_ string) error { return nil }

		// When Write is called with reset=true
		err := generator.Write(true)

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the .terraform directory should be removed
		want := filepath.Join("/mock/context", ".terraform")
		if removedPath != want {
			t.Errorf("expected RemoveAll called with %q, got %q", want, removedPath)
		}
	})

	t.Run("ErrorRemovingTerraformDir", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And .terraform directory exists but RemoveAll fails
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			if strings.HasSuffix(path, ".terraform") {
				return nil, nil // exists
			}
			return nil, os.ErrNotExist
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("mock error removing directory")
		}
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/mock/context", nil
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		}

		// When Write is called with reset=true
		err := generator.Write(true)

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// And the error should match the expected message
		expectedError := "failed to remove .terraform directory: mock error removing directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorFromGenerateModuleShim", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source that will cause generateModuleShim to fail
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "test-component",
			FullPath: "/tmp/terraform/test-component",
		}
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{component}
		}

		// Mock processTemplates to succeed
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "/tmp/context", nil
		}

		mocks.Shims.MkdirAll = func(path string, _ fs.FileMode) error {
			if strings.Contains(path, "terraform") && !strings.Contains(path, "test-component") {
				return nil // Allow terraform folder creation
			}
			return fmt.Errorf("mock error creating module directory")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to generate module shim") {
			t.Errorf("expected error to contain 'failed to generate module shim', got %v", err)
		}
	})

}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformGenerator_processTemplates(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("TemplateDirectoryNotExists", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return os.ErrNotExist for template directory
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then no error should occur and result should be nil
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("ErrorCheckingTemplateDirectory", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return an error for template directory
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, fmt.Errorf("permission denied")
			}
			return nil, nil
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to check template directory") {
			t.Errorf("expected error to contain 'failed to check template directory', got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Shell.GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("expected error to contain 'failed to get project root', got %v", err)
		}
	})

	t.Run("SuccessWithEmptyDirectory", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success for template directory
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, nil
			}
			return nil, nil
		}

		// And ReadDir is mocked to return empty directory
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{}, nil
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And result should be an empty map
		if result == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("ProcessesJsonnetFiles", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Create a simple mock for fs.DirEntry with jsonnet file
		mockEntry := &simpleDirEntry{name: "test.jsonnet", isDir: false}

		// And ReadDir is mocked to return a jsonnet file
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{mockEntry}, nil
		}

		// Mock all dependencies for processJsonnetTemplate
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  test_var: "test_value"
}`), nil
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{"test_var": "test_value"}
			}
			return nil
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/tmp/contexts/_template/terraform", "/context/path", "test-context", false, templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And template values should contain the processed template
		if len(templateValues) != 1 {
			t.Errorf("expected 1 template value, got %d", len(templateValues))
		}
	})

	t.Run("ErrorProcessingJsonnetTemplate", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Create a simple mock for fs.DirEntry with jsonnet file
		mockEntry := &simpleDirEntry{name: "test.jsonnet", isDir: false}

		// And ReadDir is mocked to return a jsonnet file
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{mockEntry}, nil
		}

		// And ReadFile is mocked to return an error for processJsonnetTemplate
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("file read error")
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error reading template file") {
			t.Errorf("expected error to contain 'error reading template file', got %v", err)
		}
	})

	t.Run("RecursivelyProcessesDirectories", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		callCount := 0
		// Create mock directory entry
		mockDirEntry := &simpleDirEntry{name: "subdir", isDir: true}

		// And ReadDir is mocked to return a directory on first call, empty on second
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return []fs.DirEntry{mockDirEntry}, nil
			}
			return []fs.DirEntry{}, nil // Empty subdirectory
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And ReadDir should have been called twice (once for root, once for subdir)
		if callCount != 2 {
			t.Errorf("expected ReadDir to be called 2 times, got %d", callCount)
		}
	})

	t.Run("ErrorInRecursiveDirectoryCall", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		callCount := 0
		// Create mock directory entry
		mockDirEntry := &simpleDirEntry{name: "subdir", isDir: true}

		// And ReadDir is mocked to return a directory on first call, error on second
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return []fs.DirEntry{mockDirEntry}, nil
			}
			return nil, fmt.Errorf("subdirectory read error")
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to read template directory") {
			t.Errorf("expected error to contain 'failed to read template directory', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success for template directory
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, nil
			}
			return nil, nil
		}

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to get config root") {
			t.Errorf("expected error to contain 'failed to get config root', got %v", err)
		}
	})

	t.Run("UsesEnvironmentVariableForContextName", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success for template directory
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, nil
			}
			return nil, nil
		}

		// And ReadDir is mocked to return empty directory
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{}, nil
		}

		// And GetString is mocked to return empty string (no context configured)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "context" {
				return "" // No context configured
			}
			return ""
		}

		// Mock environment variable
		originalEnv := os.Getenv("WINDSOR_CONTEXT")
		os.Setenv("WINDSOR_CONTEXT", "env-context")
		defer os.Setenv("WINDSOR_CONTEXT", originalEnv)

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And result should be an empty map
		if result == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}

		// Note: The environment variable usage is tested by the fact that
		// the function completes successfully and calls walkTemplateDirectory
		// with the environment context name
	})
}

func TestTerraformGenerator_walkTemplateDirectory(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ErrorReadingDirectory", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadDir is mocked to return an error
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return nil, fmt.Errorf("permission denied")
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to read template directory") {
			t.Errorf("expected error to contain 'failed to read template directory', got %v", err)
		}
	})

	t.Run("IgnoresNonJsonnetFiles", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Create a simple mock for fs.DirEntry
		mockEntry := &simpleDirEntry{name: "test.txt", isDir: false}

		// And ReadDir is mocked to return a non-jsonnet file
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{mockEntry}, nil
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And template values should be empty
		if len(templateValues) != 0 {
			t.Errorf("expected 0 template values, got %d", len(templateValues))
		}
	})

	t.Run("ProcessesJsonnetFiles", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Create a simple mock for fs.DirEntry with jsonnet file
		mockEntry := &simpleDirEntry{name: "test.jsonnet", isDir: false}

		// And ReadDir is mocked to return a jsonnet file
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{mockEntry}, nil
		}

		// Mock all dependencies for processJsonnetTemplate
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  test_var: "test_value"
}`), nil
		}

		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{"test_var": "test_value"}
			}
			return nil
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/tmp/contexts/_template/terraform", "/context/path", "test-context", false, templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And template values should contain the processed template
		if len(templateValues) != 1 {
			t.Errorf("expected 1 template value, got %d", len(templateValues))
		}
	})

	t.Run("ErrorProcessingJsonnetTemplate", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// Create a simple mock for fs.DirEntry with jsonnet file
		mockEntry := &simpleDirEntry{name: "test.jsonnet", isDir: false}

		// And ReadDir is mocked to return a jsonnet file
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{mockEntry}, nil
		}

		// And ReadFile is mocked to return an error for processJsonnetTemplate
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("file read error")
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error reading template file") {
			t.Errorf("expected error to contain 'error reading template file', got %v", err)
		}
	})

	t.Run("RecursivelyProcessesDirectories", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		callCount := 0
		// Create mock directory entry
		mockDirEntry := &simpleDirEntry{name: "subdir", isDir: true}

		// And ReadDir is mocked to return a directory on first call, empty on second
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return []fs.DirEntry{mockDirEntry}, nil
			}
			return []fs.DirEntry{}, nil // Empty subdirectory
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And ReadDir should have been called twice (once for root, once for subdir)
		if callCount != 2 {
			t.Errorf("expected ReadDir to be called 2 times, got %d", callCount)
		}
	})

	t.Run("ErrorInRecursiveDirectoryCall", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		callCount := 0
		// Create mock directory entry
		mockDirEntry := &simpleDirEntry{name: "subdir", isDir: true}

		// And ReadDir is mocked to return a directory on first call, error on second
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return []fs.DirEntry{mockDirEntry}, nil
			}
			return nil, fmt.Errorf("subdirectory read error")
		}

		templateValues := make(map[string]map[string]any)

		// When walkTemplateDirectory is called
		err := generator.walkTemplateDirectory("/template/dir", "/context/path", "test-context", false, templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to read template directory") {
			t.Errorf("expected error to contain 'failed to read template directory', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And template directory exists
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, nil
			}
			return nil, nil
		}

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to get config root") {
			t.Errorf("expected error to contain 'failed to get config root', got %v", err)
		}
	})

	t.Run("ContextNameFromEnvironment", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And template directory exists
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "_template") {
				return nil, nil
			}
			return nil, nil
		}

		// And GetString returns empty string (no context configured)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "context" {
				return ""
			}
			return ""
		}

		// And environment variable is set
		originalEnv := os.Getenv("WINDSOR_CONTEXT")
		defer func() {
			if originalEnv == "" {
				os.Unsetenv("WINDSOR_CONTEXT")
			} else {
				os.Setenv("WINDSOR_CONTEXT", originalEnv)
			}
		}()
		os.Setenv("WINDSOR_CONTEXT", "env-context")

		// And ReadDir is mocked to return empty directory
		mocks.Shims.ReadDir = func(path string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{}, nil
		}

		// When processTemplates is called
		result, err := generator.processTemplates(false)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And result should be an empty map
		if result == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})
}

func TestTerraformGenerator_processJsonnetTemplate(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ErrorReadingTemplateFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error reading template file") {
			t.Errorf("expected error to contain 'error reading template file', got %v", err)
		}
	})

	t.Run("ErrorMarshallingContextToYAML", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`local context = std.extVar("context");
{
  test_var: context.test
}`), nil
		}

		// And YamlMarshalWithDefinedPaths is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return nil, fmt.Errorf("yaml marshal error")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error marshalling context to YAML") {
			t.Errorf("expected error to contain 'error marshalling context to YAML', got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingContextYAML", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`local context = std.extVar("context");
{
  test_var: context.test
}`), nil
		}

		// And YamlMarshalWithDefinedPaths is mocked
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		// And YamlUnmarshal is mocked to return an error
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml unmarshal error")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error unmarshalling context YAML") {
			t.Errorf("expected error to contain 'error unmarshalling context YAML', got %v", err)
		}
	})

	t.Run("ErrorMarshallingContextToJSON", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`local context = std.extVar("context");
{
  test_var: context.test
}`), nil
		}

		// And YamlMarshalWithDefinedPaths is mocked
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		// And YamlUnmarshal is mocked
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		// And JsonMarshal is mocked to return an error
		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("json marshal error")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error marshalling context map to JSON") {
			t.Errorf("expected error to contain 'error marshalling context map to JSON', got %v", err)
		}
	})

	t.Run("ErrorEvaluatingJsonnet", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return invalid jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`invalid jsonnet syntax {`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error evaluating jsonnet template") {
			t.Errorf("expected error to contain 'error evaluating jsonnet template', got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingJsonnetResult", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return jsonnet that produces valid output
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  test_var: "test_value"
}`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		// And JsonUnmarshal is mocked to return an error
		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("json unmarshal error")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "jsonnet template must output valid JSON") {
			t.Errorf("expected error to contain 'jsonnet template must output valid JSON', got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Shell.GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// And ReadFile is mocked to return valid jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  test_var: "test_value"
}`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{"test_var": "test_value"}
			}
			return nil
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to get project root") {
			t.Errorf("expected error to contain 'failed to get project root', got %v", err)
		}
	})

	t.Run("ErrorCalculatingRelativePath", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Shell.GetProjectRoot is mocked to return expected path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		// And ReadFile is mocked to return valid jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  test_var: "test_value"
}`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{"test_var": "test_value"}
			}
			return nil
		}

		// And FilepathRel is mocked to return an error
		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("relative path calculation error")
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/template/test.jsonnet", "test-context", templateValues)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to calculate relative path") {
			t.Errorf("expected error to contain 'failed to calculate relative path', got %v", err)
		}
	})

	t.Run("SuccessProcessingTemplate", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Shell.GetProjectRoot is mocked to return expected path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		// And ReadFile is mocked to return valid jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`local context = std.extVar("context");
{
  test_var: context.test,
  context_name: context.name
}`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{
					"test_var":     "config",
					"context_name": "test-context",
				}
			}
			return nil
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called
		err := generator.processJsonnetTemplate("/tmp/contexts/_template/terraform/test.jsonnet", "test-context", templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And template values should contain the processed template
		if len(templateValues) != 1 {
			t.Errorf("expected 1 template value, got %d", len(templateValues))
		}
		if _, exists := templateValues["test"]; !exists {
			t.Errorf("expected template values to contain 'test' key")
		}

		// And the values should match expected content
		values := templateValues["test"]
		if values["test_var"] != "config" {
			t.Errorf("expected test_var to be 'config', got %v", values["test_var"])
		}
		if values["context_name"] != "test-context" {
			t.Errorf("expected context_name to be 'test-context', got %v", values["context_name"])
		}
	})

	t.Run("SuccessWithNestedPath", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Shell.GetProjectRoot is mocked to return expected path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp", nil
		}

		// And ReadFile is mocked to return valid jsonnet content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{
  nested_var: "nested_value"
}`), nil
		}

		// Mock the required dependencies
		mocks.ConfigHandler.(*config.MockConfigHandler).YamlMarshalWithDefinedPathsFunc = func(config any) ([]byte, error) {
			return []byte("test: config"), nil
		}

		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			if configMap, ok := v.(*map[string]any); ok {
				*configMap = map[string]any{"test": "config"}
			}
			return nil
		}

		mocks.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`{"test": "config", "name": "test-context"}`), nil
		}

		mocks.Shims.JsonUnmarshal = func(data []byte, v any) error {
			if values, ok := v.(*map[string]any); ok {
				*values = map[string]any{"nested_var": "nested_value"}
			}
			return nil
		}

		templateValues := make(map[string]map[string]any)

		// When processJsonnetTemplate is called with nested path
		err := generator.processJsonnetTemplate("/tmp/contexts/_template/terraform/nested/path/component.jsonnet", "test-context", templateValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And template values should contain the processed template with nested key
		if len(templateValues) != 1 {
			t.Errorf("expected 1 template value, got %d", len(templateValues))
		}
		if _, exists := templateValues["nested/path/component"]; !exists {
			t.Errorf("expected template values to contain 'nested/path/component' key")
		}
	})
}

func TestTerraformGenerator_generateModuleShim(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And Stat is mocked to return success for the module path
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if path == "/path/to/module" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create module directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorChdir", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And Chdir is mocked to return an error
		mocks.Shims.Chdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to change to module directory: mock error changing directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorSetenv", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And Setenv is mocked to return an error
		mocks.Shims.Setenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to set TF_DATA_DIR: mock error setting environment variable"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorExecSilent", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecProgress is mocked to return an error
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error running terraform init")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to initialize terraform: mock error running terraform init"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And GetConfigRoot is mocked to return an error
		mocks.ConfigHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get config root: mock error getting config root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And Stat is mocked to return success for the module path
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if path == "/path/to/module" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return an error for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return nil, fmt.Errorf("mock error reading variables.tf")
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read variables.tf: mock error reading variables.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingMainTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And Stat is mocked to return success for the module path
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if path == "/path/to/module" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for main.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return fmt.Errorf("mock error writing main.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write main.tf: mock error writing main.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingVariablesTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And Stat is mocked to return success for the module path
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if path == "/path/to/module" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for variables.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				return fmt.Errorf("mock error writing variables.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write shim variables.tf: mock error writing variables.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingOutputsTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output with module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}

		// And Stat is mocked to return success for the module path and outputs.tf
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if path == "/path/to/module" || strings.HasSuffix(path, "outputs.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf and outputs.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			if strings.HasSuffix(path, "outputs.tf") {
				return []byte(`output "test" {
  value = "test"
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for outputs.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "outputs.tf") {
				return fmt.Errorf("mock error writing outputs.tf")
			}
			return nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write shim outputs.tf: mock error writing outputs.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ModulePathResolution", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output without module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z"}`, nil
		}

		// And Stat is mocked to return success for .tf_modules/variables.tf
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			// Convert path to forward slashes for consistent comparison
			path = filepath.ToSlash(path)
			if strings.HasSuffix(path, ".tf_modules/variables.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			// Convert path to forward slashes for consistent comparison
			path = filepath.ToSlash(path)
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ModulePathResolutionFailure", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ExecSilent is mocked to return output without module path
		mocks.Shell.ExecSilentFunc = func(_ string, _ ...string) (string, error) {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z"}`, nil
		}

		// And Stat is mocked to return success for the standard path
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorInTerraformInit", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: filepath.Join(t.TempDir(), "module/path1"),
		}

		// Mock the WriteFile method to succeed
		originalWriteFile := generator.shims.WriteFile
		generator.shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			return nil
		}
		// Restore original WriteFile after test
		defer func() {
			generator.shims.WriteFile = originalWriteFile
		}()

		// And ExecProgress is mocked to return an error
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", fmt.Errorf("terraform init failed")
			}
			return "", nil
		}

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should indicate terraform init failure
		expectedError := "failed to initialize terraform"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoValidModulePath", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: filepath.Join(t.TempDir(), "module/path1"),
		}

		// Mock WriteFile to succeed
		originalWriteFile := generator.shims.WriteFile
		generator.shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			return nil
		}
		defer func() {
			generator.shims.WriteFile = originalWriteFile
		}()

		// Mock ExecProgress to return output without a valid module path
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z"}
{"@level":"info","@message":"No modules to initialize","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
			}
			return "", nil
		}

		// And mock Stat to return not exist for all paths
		originalStat := generator.shims.Stat
		generator.shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		defer func() {
			generator.shims.Stat = originalStat
		}()

		// When generateModuleShim is called
		err := generator.generateModuleShim(component, make(map[string][]byte))

		// Then no error should be returned because the function handles missing files
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("BlankOutputLine", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return "\n", nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("MalformedJSONOutput", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return "not-json-line", nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("LogLineWithoutMainIn", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return `{"@level":"info","@message":"No main here","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("MainInAtEndOfString", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return `{"@level":"info","@message":"- main in","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("EmptyPathAfterMainIn", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return `{"@level":"info","@message":"- main in   ","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("StatFailsForDetectedPath", func(t *testing.T) {
		generator, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}
		mocks.Shell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
			return `{"@level":"info","@message":"- main in /not/a/real/path","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = "string" }`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}
		err := generator.generateModuleShim(component, make(map[string][]byte))
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithOCISource", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with OCI source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "core-oci",
			Path:     "cluster/talos",
			FullPath: "/project/terraform/cluster/talos",
		}

		// And blueprint handler returns OCI sources
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{
					Name: "core-oci",
					Url:  "oci://ghcr.io/windsorcli/core:v0.0.1",
				},
			}
		}

		// And shell GetProjectRoot returns project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/project", nil
		}

		// And FilepathRel calculates relative path
		mocks.Shims.FilepathRel = func(basepath, targpath string) (string, error) {
			return "../../../.windsor/.oci_extracted/ghcr.io-windsorcli_core-v0.0.1/terraform/cluster/talos", nil
		}

		// And the extracted module path exists
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, ".oci_extracted") {
				return &mockFileInfo{name: "talos", isDir: true}, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile returns content from extracted module
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "cluster_name" {
  description = "Name of the Talos cluster"
  type        = string
}`), nil
			}
			if strings.HasSuffix(path, "outputs.tf") {
				return []byte(`output "cluster_endpoint" {
  description = "Cluster endpoint"
  value       = "https://cluster.example.com"
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And MkdirAll is mocked to avoid filesystem operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// And WriteFile is mocked to avoid filesystem operations
		mocks.Shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
			return nil
		}

		// And ociArtifacts contains pre-extracted data
		ociArtifacts := map[string][]byte{
			"ghcr.io/windsorcli/core:v0.0.1": []byte("mock-artifact-data"),
		}

		// When generateModuleShim is called with OCI source
		err := generator.generateModuleShim(component, ociArtifacts)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

}

func TestTerraformGenerator_writeModuleFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorReadFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read variables.tf: mock error reading file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writeModuleFile is called
		err := generator.writeModuleFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "mock error writing file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_writeTfvarsFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to create directory: mock error creating directory"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with source
		component := blueprintv1alpha1.TerraformComponent{
			Source:   "fake-source",
			Path:     "module/path1",
			FullPath: "original/full/path",
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test_dir", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckExistingTfvarsFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path: "module/path1",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return an error
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("mock error checking existing tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error checking tfvars file: mock error checking existing tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path: "module/path1",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("mock error reading existing tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read existing tfvars file: mock error reading existing tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteComponentValues", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "module/path1",
			FullPath: "original/full/path",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return not exist
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error for component values
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing component values")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing component values"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorWriteFileAfterValues", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a component with values
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "module/path1",
			FullPath: "original/full/path",
			Values: map[string]interface{}{
				"test": "value",
			},
		}

		// And Stat is mocked to return not exist
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing final tfvars file")
		}

		// When writeTfvarsFile is called
		err := generator.writeTfvarsFile("test.tfvars", component)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "error writing tfvars file: mock error writing final tfvars file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_checkExistingTfvarsFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("FileDoesNotExist", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return not found
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("FileExists", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte("test content"), nil
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then os.ErrExist should be returned
		if err != os.ErrExist {
			t.Errorf("expected os.ErrExist, got %v", err)
		}
	})

	t.Run("ErrorReadingFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading file")
		}

		// When checkExistingTfvarsFile is called
		err := generator.checkExistingTfvarsFile("test.tfvars")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read existing tfvars file: mock error reading file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_addTfvarsHeader(t *testing.T) {
	t.Run("WithSource", func(t *testing.T) {
		// Given a body and source
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		source := "fake-source"

		// When addTfvarsHeader is called
		addTfvarsHeader(body, source)

		// Then the header should be written with source
		expected := `# Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
# Module source: fake-source
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("WithoutSource", func(t *testing.T) {
		// Given a body without source
		file := hclwrite.NewEmptyFile()
		body := file.Body()

		// When addTfvarsHeader is called
		addTfvarsHeader(body, "")

		// Then the header should be written without source
		expected := `# Managed by Windsor CLI: This file is partially managed by the windsor CLI. Your changes will not be overwritten.
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})
}

func TestTerraformGenerator_writeShimVariablesTf(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to return an error
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write shim variables.tf: mock error writing file"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("CopiesSensitiveAttribute", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return variables.tf with sensitive attribute
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "sensitive_var" {
  description = "Sensitive variable"
  type        = string
  sensitive   = true
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to capture the content
		var writtenContent []byte
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				writtenContent = content
			}
			return nil
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the sensitive attribute should be copied
		file, diags := hclwrite.ParseConfig(writtenContent, "variables.tf", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			t.Fatalf("failed to parse HCL: %v", diags)
		}
		body := file.Body()
		block := body.Blocks()[0]
		attr := block.Body().GetAttribute("sensitive")
		if attr == nil {
			t.Error("expected sensitive attribute to be present")
		} else {
			tokens := attr.Expr().BuildTokens(nil)
			if len(tokens) < 1 || tokens[0].Type != hclsyntax.TokenIdent || string(tokens[0].Bytes) != "true" {
				t.Error("expected sensitive attribute to be true")
			}
		}
	})

	t.Run("ErrorReadingVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return nil, fmt.Errorf("mock error reading variables.tf")
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to read variables.tf") {
			t.Errorf("expected error to contain 'failed to read variables.tf', got %v", err)
		}
	})

	t.Run("ErrorParsingVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return invalid HCL content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`invalid hcl syntax {`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to parse variables.tf") {
			t.Errorf("expected error to contain 'failed to parse variables.tf', got %v", err)
		}
	})

	t.Run("ErrorWritingMainTf", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return content for variables.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" {
  description = "Test variable"
  type        = string
}`), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to fail only for main.tf
		mocks.Shims.WriteFile = func(path string, _ []byte, _ fs.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return fmt.Errorf("mock error writing main.tf")
			}
			return nil // Success for variables.tf
		}

		// When writeShimVariablesTf is called
		err := generator.writeShimVariablesTf("test_dir", "module_path", "fake-source")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to write shim main.tf") {
			t.Errorf("expected error to contain 'failed to write shim main.tf', got %v", err)
		}
	})
}

func TestTerraformGenerator_writeShimOutputsTf(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ErrorReadingOutputs", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to return success for outputs.tf
		mocks.Shims.Stat = func(path string) (fs.FileInfo, error) {
			if strings.HasSuffix(path, "outputs.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And ReadFile is mocked to return an error for outputs.tf
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "outputs.tf") {
				return nil, fmt.Errorf("mock error reading outputs.tf")
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// When writeShimOutputsTf is called
		err := generator.writeShimOutputsTf("test_dir", "module_path")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read outputs.tf: mock error reading outputs.tf"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestTerraformGenerator_isOCISource(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("DirectOCIURL", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, _ := setup(t)

		// When isOCISource is called with a direct OCI URL
		result := generator.isOCISource("oci://registry.example.com/repo:tag")

		// Then it should return true
		if !result {
			t.Error("expected isOCISource to return true for direct OCI URL")
		}
	})

	t.Run("NamedOCISource", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)
		mockBH := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBH.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{
					Name: "oci-source",
					Url:  "oci://registry.example.com/terraform-modules:v1.0.0",
				},
			}
		}

		// When isOCISource is called with a named OCI source
		result := generator.isOCISource("oci-source")

		// Then it should return true
		if !result {
			t.Error("expected isOCISource to return true for named OCI source")
		}
	})

	t.Run("NonOCISources", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)
		mockBH := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBH.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{
					Name: "git-source",
					Url:  "git::https://github.com/example/repo.git",
				},
			}
		}

		testCases := []string{
			"/path/to/.oci_extracted/module",
			"git::https://github.com/example/repo.git",
			"./local/path",
			"git-source",
			"unknown-source",
			"",
		}

		for _, source := range testCases {
			// When isOCISource is called
			result := generator.isOCISource(source)

			// Then it should return false
			if result {
				t.Errorf("expected isOCISource to return false for %s", source)
			}
		}
	})
}

func TestTerraformGenerator_extractOCIModule(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ModuleAlreadyExtracted", func(t *testing.T) {
		// Given a TerraformGenerator with existing extracted module
		generator, mocks := setup(t)
		mockBH := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBH.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "oci://registry.example.com/modules:v1.0.0"},
			}
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp/project", nil
		}

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil // Module already exists
		}

		// When extractOCIModule is called
		result, err := generator.extractOCIModule("test-source", "test/module", make(map[string][]byte))

		// Then it should return the existing path without error
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		expectedPath := "/tmp/project/.windsor/.oci_extracted/registry.example.com-modules-v1.0.0/terraform/test/module"
		if filepath.ToSlash(result) != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, filepath.ToSlash(result))
		}
	})

	t.Run("SourceNotFound", func(t *testing.T) {
		// Given a TerraformGenerator with no matching source
		generator, mocks := setup(t)
		mockBH := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBH.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{}
		}

		// When extractOCIModule is called with unknown source
		_, err := generator.extractOCIModule("unknown-source", "test/module", make(map[string][]byte))

		// Then it should return an error
		if err == nil {
			t.Error("expected error for unknown source")
		}
		if !strings.Contains(err.Error(), "source unknown-source not found") {
			t.Errorf("expected 'source not found' error, got %v", err)
		}
	})

	t.Run("UsesCachedArtifact", func(t *testing.T) {
		// Given a TerraformGenerator with cached OCI artifact
		generator, mocks := setup(t)
		mockBH := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBH.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "test-source", Url: "oci://registry.example.com/modules:v1.0.0"},
			}
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/tmp/project", nil
		}

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // Module doesn't exist yet
		}

		// Mock tar extraction - create a minimal valid tar stream
		mocks.Shims.NewBytesReader = func(data []byte) io.Reader {
			// Create a minimal tar file with just an EOF
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			tw.Close() // This creates a valid empty tar file
			return bytes.NewReader(buf.Bytes())
		}

		mocks.Shims.NewTarReader = func(r io.Reader) *tar.Reader {
			return tar.NewReader(r)
		}

		mocks.Shims.EOFError = func() error { return io.EOF }

		// And cached artifact data
		cachedData := []byte("mock tar data")
		ociArtifacts := map[string][]byte{
			"registry.example.com/modules:v1.0.0": cachedData,
		}

		// When extractOCIModule is called
		result, err := generator.extractOCIModule("test-source", "test/module", ociArtifacts)

		// Then it should succeed and return correct path (even with empty tar)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		expectedPath := "/tmp/project/.windsor/.oci_extracted/registry.example.com-modules-v1.0.0/terraform/test/module"
		if filepath.ToSlash(result) != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, filepath.ToSlash(result))
		}
	})
}

func TestTerraformGenerator_parseVariablesFile(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("AllVariableTypes", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return variables with all types and attributes
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`
variable "string_var" {
  description = "String variable"
  type        = string
  default     = "default_value"
  sensitive   = false
}

variable "number_var" {
  description = "Number variable"
  type        = number
  default     = 42
  sensitive   = false
}

variable "bool_var" {
  description = "Boolean variable"
  type        = bool
  default     = true
  sensitive   = true
}

variable "list_var" {
  description = "List variable"
  type        = list(string)
  default     = ["item1", "item2"]
}

variable "map_var" {
  description = "Map variable"
  type        = map(string)
  default     = { key = "value" }
}

variable "no_default" {
  description = "Variable without default"
  type        = string
}

variable "no_description" {
  type    = string
  default = "value"
}

variable "invalid_default" {
  description = "Variable with invalid default"
  type        = string
  default     = invalid
}

variable "invalid_sensitive" {
  description = "Variable with invalid sensitive"
  type        = string
  sensitive   = invalid
}`), nil
		}

		// When parseVariablesFile is called
		variables, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And all variables should be parsed correctly
		expectedVars := map[string]VariableInfo{
			"string_var": {
				Name:        "string_var",
				Description: "String variable",
				Default:     "default_value",
				Sensitive:   false,
			},
			"number_var": {
				Name:        "number_var",
				Description: "Number variable",
				Default:     int64(42),
				Sensitive:   false,
			},
			"bool_var": {
				Name:        "bool_var",
				Description: "Boolean variable",
				Default:     true,
				Sensitive:   true,
			},
			"list_var": {
				Name:        "list_var",
				Description: "List variable",
				Default:     []any{"item1", "item2"},
			},
			"map_var": {
				Name:        "map_var",
				Description: "Map variable",
				Default:     map[string]any{"key": "value"},
			},
			"no_default": {
				Name:        "no_default",
				Description: "Variable without default",
			},
			"no_description": {
				Name:    "no_description",
				Default: "value",
			},
			"invalid_default": {
				Name:        "invalid_default",
				Description: "Variable with invalid default",
			},
			"invalid_sensitive": {
				Name:        "invalid_sensitive",
				Description: "Variable with invalid sensitive",
			},
		}

		// Verify each variable
		if len(variables) != len(expectedVars) {
			t.Errorf("expected %d variables, got %d", len(expectedVars), len(variables))
		}

		for _, v := range variables {
			expected, exists := expectedVars[v.Name]
			if !exists {
				t.Errorf("unexpected variable %s", v.Name)
				continue
			}

			if v.Description != expected.Description {
				t.Errorf("variable %s: expected description %q, got %q", v.Name, expected.Description, v.Description)
			}
			if !reflect.DeepEqual(v.Default, expected.Default) {
				t.Errorf("variable %s: expected default %v (%T), got %v (%T)", v.Name, expected.Default, expected.Default, v.Default, v.Default)
			}
			if v.Sensitive != expected.Sensitive {
				t.Errorf("variable %s: expected sensitive %v, got %v", v.Name, expected.Sensitive, v.Sensitive)
			}
		}
	})

	t.Run("ProtectedVariables", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return variables
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`
variable "protected_var" {
  description = "Protected variable"
  type        = string
}

variable "normal_var" {
  description = "Normal variable"
  type        = string
}`), nil
		}

		// And protected values are set
		protectedValues := map[string]bool{
			"protected_var": true,
		}

		// When parseVariablesFile is called
		variables, err := generator.parseVariablesFile("test.tf", protectedValues)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And only non-protected variables should be included
		if len(variables) != 1 {
			t.Errorf("expected 1 variable, got %d", len(variables))
		}
		if variables[0].Name != "normal_var" {
			t.Errorf("expected variable normal_var, got %s", variables[0].Name)
		}
	})

	t.Run("InvalidHCL", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return invalid HCL
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`invalid hcl content`), nil
		}

		// When parseVariablesFile is called
		_, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, fmt.Errorf("read error")
		}

		// When parseVariablesFile is called
		_, err := generator.parseVariablesFile("test.tf", map[string]bool{})

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got nil")
		}
		expectedError := "failed to read variables.tf: read error"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestConvertToCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected cty.Value
	}{
		{
			name:     "String",
			input:    "test",
			expected: cty.StringVal("test"),
		},
		{
			name:     "Int",
			input:    42,
			expected: cty.NumberIntVal(42),
		},
		{
			name:     "Float64",
			input:    42.5,
			expected: cty.NumberFloatVal(42.5),
		},
		{
			name:     "Bool",
			input:    true,
			expected: cty.BoolVal(true),
		},
		{
			name:     "EmptyList",
			input:    []any{},
			expected: cty.ListValEmpty(cty.DynamicPseudoType),
		},
		{
			name:     "List",
			input:    []any{"item1", "item2"},
			expected: cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
		},
		{
			name:     "Map",
			input:    map[string]any{"key": "value"},
			expected: cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("value")}),
		},
		{
			name:     "Unsupported",
			input:    struct{}{},
			expected: cty.NilVal,
		},
		{
			name:     "Nil",
			input:    nil,
			expected: cty.NilVal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToCtyValue(tt.input)
			if !result.RawEquals(tt.expected) {
				t.Errorf("expected %#v, got %#v", tt.expected, result)
			}
		})
	}
}

func TestConvertFromCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    cty.Value
		expected any
	}{
		{
			name:     "String",
			input:    cty.StringVal("test"),
			expected: "test",
		},
		{
			name:     "Int",
			input:    cty.NumberIntVal(42),
			expected: int64(42),
		},
		{
			name:     "Float",
			input:    cty.NumberFloatVal(42.5),
			expected: float64(42.5),
		},
		{
			name:     "NumberBigFloat",
			input:    cty.NumberVal(big.NewFloat(42.5)),
			expected: float64(42.5),
		},
		{
			name:     "Bool",
			input:    cty.BoolVal(true),
			expected: true,
		},
		{
			name:     "List",
			input:    cty.ListVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
			expected: []any{"item1", "item2"},
		},
		{
			name:     "EmptyList",
			input:    cty.ListValEmpty(cty.String),
			expected: []any(nil),
		},
		{
			name:     "Map",
			input:    cty.MapVal(map[string]cty.Value{"key": cty.StringVal("value")}),
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "EmptyMap",
			input:    cty.MapValEmpty(cty.String),
			expected: map[string]any{},
		},
		{
			name:     "Object",
			input:    cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("value")}),
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "Null",
			input:    cty.NullVal(cty.String),
			expected: nil,
		},
		{
			name:     "Unknown",
			input:    cty.UnknownVal(cty.String),
			expected: nil,
		},
		{
			name:     "Set",
			input:    cty.SetVal([]cty.Value{cty.StringVal("item1"), cty.StringVal("item2")}),
			expected: []any{"item1", "item2"},
		},
		{
			name:     "Tuple",
			input:    cty.TupleVal([]cty.Value{cty.StringVal("item1"), cty.NumberIntVal(42)}),
			expected: []any{"item1", int64(42)},
		},
		{
			name:     "UnsupportedType",
			input:    cty.DynamicVal,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFromCtyValue(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %#v (%T), got %#v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	t.Run("EmptyArray", func(t *testing.T) {
		result := formatValue([]any{})
		if result != "[]" {
			t.Errorf("expected [] got %q", result)
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		result := formatValue(map[string]any{})
		if result != "{}" {
			t.Errorf("expected {} got %q", result)
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		result := formatValue(nil)
		if result != "null" {
			t.Errorf("expected null got %q", result)
		}
	})

	t.Run("ComplexNestedObject", func(t *testing.T) {
		input := map[string]any{
			"node_groups": map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}
		expected := `{
  node_groups = {
    default = {
      desired_size = 2
      instance_types = ["t3.medium"]
      max_size = 3
      min_size = 1
    }
  }
}`
		result := formatValue(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("EmptyAddons", func(t *testing.T) {
		input := map[string]any{
			"addons": map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}
		expected := `{
  addons = {
    aws-ebs-csi-driver = {}
    aws-efs-csi-driver = {}
    coredns = {}
    eks-pod-identity-agent = {}
    external-dns = {}
    vpc-cni = {}
  }
}`
		result := formatValue(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestWriteComponentValues(t *testing.T) {
	t.Run("BasicComponentValues", func(t *testing.T) {
		// Given a body and variables with component values
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{
				Name:        "var1",
				Description: "Variable 1",
				Sensitive:   true,
				Default:     "default1",
			},
			{
				Name:        "var2",
				Description: "Variable 2",
				Sensitive:   false,
				Default:     "default2",
			},
			{
				Name:        "var3",
				Description: "Variable 3",
				Default:     "default3",
			},
		}
		values := map[string]any{
			"var2": "pinned_value",
		}
		protectedValues := map[string]bool{}

		// When writeComponentValues is called
		writeComponentValues(body, values, protectedValues, variables)

		// Then the variables should be written in order with proper handling of sensitive values
		expected := `
# Variable 1
# var1 = "(sensitive)"

# Variable 2
var2 = "pinned_value"

# Variable 3
# var3 = "default3"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("ComplexDefaultsNodeGroups", func(t *testing.T) {
		// Given a body and variables with complex nested default for node groups
		file := hclwrite.NewEmptyFile()
		variable := VariableInfo{
			Name:        "node_groups",
			Description: "Map of EKS managed node group definitions to create.",
			Default: map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}

		// When writeComponentValues is called
		writeComponentValues(file.Body(), map[string]any{}, map[string]bool{}, []VariableInfo{variable})

		// Then the complex default should be commented out correctly
		expected := `
# Map of EKS managed node group definitions to create.
# node_groups = {
#   default = {
#     desired_size = 2
#     instance_types = ["t3.medium"]
#     max_size = 3
#     min_size = 1
#   }
# }`
		result := string(file.Bytes())
		// Check that every line is commented
		for _, line := range strings.Split(result, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if !strings.HasPrefix(line, "#") {
				t.Errorf("uncommented line found: %q", line)
			}
		}
		// Check that the output matches expected ignoring leading/trailing whitespace
		if strings.TrimSpace(result) != strings.TrimSpace(expected) {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ComplexDefaultsEmptyAddons", func(t *testing.T) {
		// Given a body and variables with complex empty map defaults for addons
		file := hclwrite.NewEmptyFile()
		variable := VariableInfo{
			Name:        "addons",
			Description: "Map of EKS add-ons",
			Default: map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}

		// When writeComponentValues is called
		writeComponentValues(file.Body(), map[string]any{}, map[string]bool{}, []VariableInfo{variable})

		// Then the complex empty map defaults should be commented correctly
		expected := "\n# Map of EKS add-ons\n# addons = {\n#   aws-ebs-csi-driver = {}\n#   aws-efs-csi-driver = {}\n#   coredns = {}\n#   eks-pod-identity-agent = {}\n#   external-dns = {}\n#   vpc-cni = {}\n# }\n"
		result := string(file.Bytes())
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestWriteDefaultValues(t *testing.T) {
	// Given a body and variables with default values
	file := hclwrite.NewEmptyFile()
	body := file.Body()
	variables := []VariableInfo{
		{
			Name:        "var1",
			Description: "Variable 1",
			Sensitive:   true,
			Default:     "default1",
		},
		{
			Name:        "var2",
			Description: "Variable 2",
			Sensitive:   false,
			Default:     "default2",
		},
		{
			Name:        "var3",
			Description: "Variable 3",
			Default:     "default3",
		},
	}

	// When writeDefaultValues is called
	writeDefaultValues(body, variables, nil)

	// Then the variables should be written in order with proper handling of sensitive values
	expected := `
# Variable 1
# var1 = "(sensitive)"

# Variable 2
# var2 = "default2"

# Variable 3
# var3 = "default3"
`
	if string(file.Bytes()) != expected {
		t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
	}
}

func TestWriteVariable(t *testing.T) {
	t.Run("SensitiveVariable", func(t *testing.T) {
		// Given a body and variables with a sensitive variable
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{
				Name:        "test_var",
				Description: "Test variable",
				Sensitive:   true,
			},
		}

		// When writeVariable is called
		writeVariable(body, "test_var", "value", variables)

		// Then the variable should be commented out with (sensitive)
		expected := `# Test variable
# test_var = "(sensitive)"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("NonSensitiveVariable", func(t *testing.T) {
		// Given a body and variables with a non-sensitive variable
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{
				Name:        "test_var",
				Description: "Test variable",
				Sensitive:   false,
			},
		}

		// When writeVariable is called
		writeVariable(body, "test_var", "value", variables)

		// Then the variable should be written with its value
		expected := `# Test variable
test_var = "value"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("VariableWithComment", func(t *testing.T) {
		// Given a body and variables with a variable with comment
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{
				Name:        "test_var",
				Description: "Test variable description",
			},
		}

		// When writeVariable is called
		writeVariable(body, "test_var", "value", variables)

		// Then the variable should be written with its comment
		expected := `# Test variable description
test_var = "value"
`
		if string(file.Bytes()) != expected {
			t.Errorf("expected %q, got %q", expected, string(file.Bytes()))
		}
	})

	t.Run("YAMLMultilineValue", func(t *testing.T) {
		// Given a body and variables with a YAML multiline value
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{
				Name:        "worker_config_patches",
				Description: "Worker configuration patches",
			},
		}

		// When writeVariable is called with a YAML multiline string
		yamlValue := `machine:
  kubelet:
    extraMounts:
    - destination: /var/local
      options:
      - rbind
      - rw
      source: /var/local
      type: bind`
		writeVariable(body, "worker_config_patches", yamlValue, variables)

		// Then the variable should be written as a heredoc with valid YAML
		actual := string(file.Bytes())

		// Extract the YAML content from the heredoc
		lines := strings.Split(actual, "\n")
		var yamlContent strings.Builder
		inYAML := false
		for _, line := range lines {
			if strings.Contains(line, "<<EOF") {
				inYAML = true
				continue
			}
			if strings.Contains(line, "EOF") {
				inYAML = false
				continue
			}
			if inYAML {
				yamlContent.WriteString(line + "\n")
			}
		}

		// Parse both expected and actual YAML
		var expectedData, actualData any
		expectedYAML := `machine:
  kubelet:
    extraMounts:
    - destination: /var/local
      options:
      - rbind
      - rw
      source: /var/local
      type: bind`

		if err := yaml.UnmarshalWithOptions([]byte(expectedYAML), &expectedData); err != nil {
			t.Fatalf("failed to parse expected YAML: %v", err)
		}
		if err := yaml.UnmarshalWithOptions([]byte(yamlContent.String()), &actualData); err != nil {
			t.Fatalf("failed to parse actual YAML: %v", err)
		}

		// Compare the YAML data structures
		var expectedBytes, actualBytes []byte
		expectedBytes, _ = yaml.MarshalWithOptions(expectedData)
		actualBytes, _ = yaml.MarshalWithOptions(actualData)
		if string(expectedBytes) != string(actualBytes) {
			t.Errorf("YAML content does not match.\nExpected:\n%s\nGot:\n%s", expectedBytes, actualBytes)
		}

		// Verify the comment and heredoc syntax
		if !strings.Contains(actual, "# Worker configuration patches") {
			t.Error("missing description comment")
		}
		if !strings.Contains(actual, "worker_config_patches = <<EOF") {
			t.Error("missing heredoc start")
		}
		if !strings.Contains(actual, "EOF") {
			t.Error("missing heredoc end")
		}
	})

	t.Run("MultilineString", func(t *testing.T) {
		// Given a multiline string value
		file := hclwrite.NewEmptyFile()
		value := `first line
  indented line
    more indented
last line`

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the content should be written as heredoc
		content := string(file.Bytes())
		lines := strings.Split(content, "\n")
		var actualLines []string
		inHeredoc := false
		for _, line := range lines {
			if strings.Contains(line, "<<EOF") {
				inHeredoc = true
				continue
			}
			if strings.Contains(line, "EOF") {
				break
			}
			if inHeredoc {
				actualLines = append(actualLines, line)
			}
		}
		actual := strings.Join(actualLines, "\n")
		if actual != value {
			t.Errorf("content does not match.\nExpected:\n%s\nGot:\n%s", value, actual)
		}
	})

	t.Run("MultilineStringWithTabs", func(t *testing.T) {
		// Given a multiline string with tabs
		file := hclwrite.NewEmptyFile()
		value := `first line
	tabbed line
		more tabbed
last line`

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the content should preserve tabs
		content := string(file.Bytes())
		lines := strings.Split(content, "\n")
		var actualLines []string
		inHeredoc := false
		for _, line := range lines {
			if strings.Contains(line, "<<EOF") {
				inHeredoc = true
				continue
			}
			if strings.Contains(line, "EOF") {
				break
			}
			if inHeredoc {
				actualLines = append(actualLines, line)
			}
		}
		actual := strings.Join(actualLines, "\n")
		if actual != value {
			t.Errorf("content does not match.\nExpected:\n%s\nGot:\n%s", value, actual)
		}
	})

	t.Run("MapValue", func(t *testing.T) {
		// Given a map value
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"string": "value",
			"number": 42,
			"bool":   true,
			"nested": map[string]any{
				"key": "value",
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "test_var", value, nil)

		// Then the map should be formatted correctly
		content := string(file.Bytes())
		assignmentPrefix := "test_var = "
		idx := strings.Index(content, assignmentPrefix)
		if idx == -1 {
			t.Fatalf("assignment not found in output: %q", content)
		}
		actual := strings.TrimSpace(content[idx+len(assignmentPrefix):])
		expected := `{
  bool = true
  nested = {
    key = "value"
  }
  number = 42
  string = "value"
}`
		if actual != expected {
			t.Errorf("map content does not match.\nExpected:\n%s\nGot:\n%s", expected, actual)
		}
	})

	t.Run("ComplexObjectNodeGroups", func(t *testing.T) {
		// Given a complex nested object for node groups
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"node_groups": map[string]any{
				"default": map[string]any{
					"instance_types": []any{"t3.medium"},
					"min_size":       1,
					"max_size":       3,
					"desired_size":   2,
				},
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "node_groups", value, nil)

		// Then the complex object should be formatted correctly
		expected := "node_groups = {\n  node_groups = {\n    default = {\n      desired_size = 2\n      instance_types = [\"t3.medium\"]\n      max_size = 3\n      min_size = 1\n    }\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ComplexObjectAddons", func(t *testing.T) {
		// Given a complex object with empty maps for addons
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"addons": map[string]any{
				"vpc-cni":                map[string]any{},
				"aws-efs-csi-driver":     map[string]any{},
				"aws-ebs-csi-driver":     map[string]any{},
				"eks-pod-identity-agent": map[string]any{},
				"coredns":                map[string]any{},
				"external-dns":           map[string]any{},
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "addons", value, nil)

		// Then the addons object should be formatted correctly with sorted keys
		expected := "addons = {\n  addons = {\n    aws-ebs-csi-driver = {}\n    aws-efs-csi-driver = {}\n    coredns = {}\n    eks-pod-identity-agent = {}\n    external-dns = {}\n    vpc-cni = {}\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})

	t.Run("ObjectAssignmentNoHeredoc", func(t *testing.T) {
		// Given a nested object value without requiring heredoc
		file := hclwrite.NewEmptyFile()
		value := map[string]any{
			"default": map[string]any{
				"desired_size":   2,
				"instance_types": []any{"t3.medium"},
				"max_size":       3,
				"min_size":       1,
			},
		}

		// When writeVariable is called
		writeVariable(file.Body(), "node_groups", value, nil)

		// Then the object should be assigned directly without heredoc
		expected := "node_groups = {\n  default = {\n    desired_size = 2\n    instance_types = [\"t3.medium\"]\n    max_size = 3\n    min_size = 1\n  }\n}"
		result := strings.TrimSpace(string(file.Bytes()))
		if result != expected {
			t.Errorf("expected\n%s\ngot\n%s", expected, result)
		}
	})
}

func TestTerraformGenerator_parseOCIRef(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("ValidOCIReference", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, _ := setup(t)

		// When parseOCIRef is called with valid OCI reference
		registry, repository, tag, err := generator.parseOCIRef("oci://registry.example.com/my-repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the components should be parsed correctly
		if registry != "registry.example.com" {
			t.Errorf("expected registry 'registry.example.com', got %s", registry)
		}
		if repository != "my-repo" {
			t.Errorf("expected repository 'my-repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})

	t.Run("InvalidOCIPrefix", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, _ := setup(t)

		// When parseOCIRef is called with invalid prefix
		_, _, _, err := generator.parseOCIRef("https://registry.example.com/my-repo:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format: https://registry.example.com/my-repo:v1.0.0"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MissingTag", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, _ := setup(t)

		// When parseOCIRef is called with missing tag
		_, _, _, err := generator.parseOCIRef("oci://registry.example.com/my-repo")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format, expected registry/repository:tag: oci://registry.example.com/my-repo"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("MissingRepository", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, _ := setup(t)

		// When parseOCIRef is called with missing repository
		_, _, _, err := generator.parseOCIRef("oci://registry.example.com:v1.0.0")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "invalid OCI reference format, expected registry/repository:tag: oci://registry.example.com:v1.0.0"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NestedRepositoryPath", func(t *testing.T) {
		// Given a TerraformGenerator
		generator, _ := setup(t)

		// When parseOCIRef is called with nested repository path
		registry, repository, tag, err := generator.parseOCIRef("oci://registry.example.com/organization/my-repo:v1.0.0")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the components should be parsed correctly
		if registry != "registry.example.com" {
			t.Errorf("expected registry 'registry.example.com', got %s", registry)
		}
		if repository != "organization/my-repo" {
			t.Errorf("expected repository 'organization/my-repo', got %s", repository)
		}
		if tag != "v1.0.0" {
			t.Errorf("expected tag 'v1.0.0', got %s", tag)
		}
	})
}

func TestTerraformGenerator_extractModuleFromArtifact(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		setupTerraformGeneratorMocks(mocks) // Add terraform-specific mocks
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims

		// Mock GetProjectRoot to return /project for tests
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/project", nil
		}

		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	// Helper function to create a tar archive with test data
	createTestTarData := func(files map[string]string, dirs []string) []byte {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		defer tw.Close()

		// Add directories first
		for _, dir := range dirs {
			header := &tar.Header{
				Name:     dir,
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			if err := tw.WriteHeader(header); err != nil {
				panic(err)
			}
		}

		// Add files
		for filename, content := range files {
			header := &tar.Header{
				Name: filename,
				Mode: 0644,
				Size: int64(len(content)),
			}
			if err := tw.WriteHeader(header); err != nil {
				panic(err)
			}
			if _, err := tw.Write([]byte(content)); err != nil {
				panic(err)
			}
		}

		return buf.Bytes()
	}

	t.Run("SuccessfulExtraction", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with test files
		testFiles := map[string]string{
			"terraform/test/module/main.tf":      "module content",
			"terraform/test/module/variables.tf": "variables content",
			"terraform/test/module/outputs.tf":   "outputs content",
		}
		testDirs := []string{
			"terraform/test/module",
		}
		tarData := createTestTarData(testFiles, testDirs)

		// And tracking variables for mocked operations
		var extractedFiles []string
		var createdDirs []string

		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			createdDirs = append(createdDirs, path)
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			extractedFiles = append(extractedFiles, path)
			// Return a mock file that doesn't actually write to disk
			return createMockFile()
		}
		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 100, nil // Simulate successful copy
		}
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return nil // Simulate successful chmod
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the correct files should be extracted
		expectedFiles := map[string]bool{
			"/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module/main.tf":      true,
			"/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module/variables.tf": true,
			"/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module/outputs.tf":   true,
		}
		if len(extractedFiles) != len(expectedFiles) {
			t.Errorf("expected %d files extracted, got %d", len(expectedFiles), len(extractedFiles))
		}
		for _, extractedFile := range extractedFiles {
			normalizedPath := filepath.ToSlash(extractedFile)
			if !expectedFiles[normalizedPath] {
				t.Errorf("unexpected file extracted: %q", normalizedPath)
			}
		}

		// And the correct directory should be created
		expectedDir := "/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module"
		found := false
		for _, dir := range createdDirs {
			if filepath.ToSlash(dir) == expectedDir {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected directory %q to be created", expectedDir)
		}
	})

	t.Run("PathFiltering", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with files that should be filtered out
		testFiles := map[string]string{
			"terraform/test/module/main.tf":  "module content",
			"terraform/other/module/main.tf": "other content",  // Should be filtered out
			"docs/README.md":                 "readme content", // Should be filtered out
		}
		tarData := createTestTarData(testFiles, []string{})

		var extractedFiles []string

		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			extractedFiles = append(extractedFiles, path)
			return createMockFile()
		}
		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 100, nil
		}
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return nil // Simulate successful chmod
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And only matching files should be extracted
		if len(extractedFiles) != 1 {
			t.Errorf("expected 1 file extracted, got %d", len(extractedFiles))
		}
		if len(extractedFiles) > 0 && filepath.ToSlash(extractedFiles[0]) != "/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module/main.tf" {
			t.Errorf("expected only test/module/main.tf to be extracted, got %v", extractedFiles)
		}
	})

	t.Run("DirectoryCreationError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with directory entries
		testDirs := []string{"terraform/test/module"}
		tarData := createTestTarData(map[string]string{}, testDirs)

		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return fmt.Errorf("permission denied")
		}
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return nil // Won't be called due to directory creation error
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to create directory") {
			t.Errorf("expected error to contain 'failed to create directory', got %v", err)
		}
	})

	t.Run("FileCreationError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with file entries
		testFiles := map[string]string{
			"terraform/test/module/main.tf": "module content",
		}
		tarData := createTestTarData(testFiles, []string{})

		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			return nil, fmt.Errorf("permission denied")
		}
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return nil // Won't be called due to file creation error
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to create file") {
			t.Errorf("expected error to contain 'failed to create file', got %v", err)
		}
	})

	t.Run("FileCopyError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with file entries
		testFiles := map[string]string{
			"terraform/test/module/main.tf": "module content",
		}
		tarData := createTestTarData(testFiles, []string{})

		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			return createMockFile()
		}
		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, fmt.Errorf("write error")
		}
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return nil // Won't be called due to copy error
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to write file") {
			t.Errorf("expected error to contain 'failed to write file', got %v", err)
		}
	})

	t.Run("EmptyTarArchive", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, _ := setup(t)

		// And an empty tar archive
		tarData := createTestTarData(map[string]string{}, []string{})

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then no error should occur (empty extraction is valid)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("InvalidTarData", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, _ := setup(t)

		// And invalid tar data
		invalidTarData := []byte("invalid tar data")

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(invalidTarData, "test/module", "/project")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to read tar header") {
			t.Errorf("expected error to contain 'failed to read tar header', got %v", err)
		}
	})

	t.Run("FilePermissionsPreserved", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with executable files
		testFiles := map[string]string{
			"terraform/test/module/main.tf":        "module content",
			"terraform/test/module/scripts/run.sh": "#!/bin/bash\necho 'hello'",
		}
		tarData := createTestTarData(testFiles, []string{})

		// And mocks for file operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			return createMockFile()
		}
		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 100, nil
		}

		// And track chmod calls
		chmodCalls := make(map[string]os.FileMode)
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			chmodCalls[name] = mode
			return nil
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And chmod should have been called for all files
		if len(chmodCalls) == 0 {
			t.Errorf("expected chmod to be called for extracted files")
		}

		// And permissions should be preserved (the test tar data creates files with 0644)
		for path, mode := range chmodCalls {
			normalizedPath := filepath.ToSlash(path)
			if !strings.Contains(normalizedPath, "/project/.windsor/.oci_extracted/test-extraction-key/terraform/test/module/") {
				t.Errorf("unexpected file path in chmod call: %s", normalizedPath)
			}
			// createTestTarData creates regular files with 0644 mode
			// but .sh files get execute permissions added (0755)
			if strings.HasSuffix(path, ".sh") {
				if mode != 0755 {
					t.Errorf("expected file mode 0755 for .sh file, got %o for file %s", mode, path)
				}
			} else {
				if mode != 0644 {
					t.Errorf("expected file mode 0644, got %o for file %s", mode, path)
				}
			}
		}
	})

	t.Run("ChmodError", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And a tar archive with files
		testFiles := map[string]string{
			"terraform/test/module/main.tf": "module content",
		}
		tarData := createTestTarData(testFiles, []string{})

		// And mocks for file operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}
		mocks.Shims.Create = func(path string) (*os.File, error) {
			return createMockFile()
		}
		mocks.Shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
			return 100, nil
		}

		// And chmod returns an error
		mocks.Shims.Chmod = func(name string, mode os.FileMode) error {
			return fmt.Errorf("permission denied")
		}

		// When extractModuleFromArtifact is called
		err := generator.extractModuleFromArtifact(tarData, "test/module", "test-extraction-key")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "failed to set file permissions") {
			t.Errorf("expected error to contain 'failed to set file permissions', got %v", err)
		}
	})
}

func TestTerraformGenerator_preloadOCIArtifacts(t *testing.T) {
	setup := func(t *testing.T) (*TerraformGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewTerraformGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize TerraformGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("NoOCISources", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns no OCI sources
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "git-source", Url: "https://github.com/example/repo.git"},
				{Name: "local-source", Url: "file:///local/path"},
			}
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And an empty map should be returned
		if len(artifacts) != 0 {
			t.Errorf("expected empty artifacts map, got %d items", len(artifacts))
		}
	})

	t.Run("EmptySourcesList", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns empty list
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{}
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And an empty map should be returned
		if len(artifacts) != 0 {
			t.Errorf("expected empty artifacts map, got %d items", len(artifacts))
		}
	})

	t.Run("SingleOCISourceSuccess", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns one OCI source
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "oci-source", Url: "oci://registry.example.com/my-repo:v1.0.0"},
			}
		}

		// And the artifact builder is set up to return expected data
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			if len(ociRefs) != 1 || ociRefs[0] != "oci://registry.example.com/my-repo:v1.0.0" {
				return nil, fmt.Errorf("unexpected OCI refs: %v", ociRefs)
			}
			return map[string][]byte{
				"registry.example.com/my-repo:v1.0.0": []byte("test artifact data"),
			}, nil
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain the cached data
		expectedKey := "registry.example.com/my-repo:v1.0.0"
		if len(artifacts) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts))
		}
		expectedData := []byte("test artifact data")
		if !bytes.Equal(artifacts[expectedKey], expectedData) {
			t.Errorf("expected artifact data to match test data")
		}
	})

	t.Run("MultipleOCISourcesDifferentArtifacts", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns multiple different OCI sources
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "oci-source1", Url: "oci://registry.example.com/repo1:v1.0.0"},
				{Name: "oci-source2", Url: "oci://registry.example.com/repo2:v2.0.0"},
			}
		}

		// And the artifact builder is set up to return data for both artifacts
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			expected := []string{
				"oci://registry.example.com/repo1:v1.0.0",
				"oci://registry.example.com/repo2:v2.0.0",
			}
			if len(ociRefs) != 2 {
				return nil, fmt.Errorf("expected 2 OCI refs, got %d", len(ociRefs))
			}
			for _, expected := range expected {
				found := false
				for _, actual := range ociRefs {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("expected OCI ref %s not found", expected)
				}
			}
			return map[string][]byte{
				"registry.example.com/repo1:v1.0.0": []byte("test artifact data 1"),
				"registry.example.com/repo2:v2.0.0": []byte("test artifact data 2"),
			}, nil
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain both entries
		if len(artifacts) != 2 {
			t.Errorf("expected 2 artifacts, got %d", len(artifacts))
		}

		// And both artifacts should be present
		key1 := "registry.example.com/repo1:v1.0.0"
		key2 := "registry.example.com/repo2:v2.0.0"
		if _, exists := artifacts[key1]; !exists {
			t.Errorf("expected artifact %s to exist", key1)
		}
		if _, exists := artifacts[key2]; !exists {
			t.Errorf("expected artifact %s to exist", key2)
		}
	})

	t.Run("MultipleOCISourcesSameArtifact", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns multiple sources pointing to same OCI artifact
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "oci-source1", Url: "oci://registry.example.com/my-repo:v1.0.0"},
				{Name: "oci-source2", Url: "oci://registry.example.com/my-repo:v1.0.0"},
				{Name: "git-source", Url: "https://github.com/example/repo.git"},
			}
		}

		// And the artifact builder is set up to return data for the unique OCI artifact
		testData := []byte("test artifact data")
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			if len(ociRefs) != 2 {
				return nil, fmt.Errorf("expected 2 OCI refs (with duplicates), got %d", len(ociRefs))
			}
			for _, ref := range ociRefs {
				if ref != "oci://registry.example.com/my-repo:v1.0.0" {
					return nil, fmt.Errorf("unexpected OCI ref: %s", ref)
				}
			}
			// The bundler handles deduplication, so return just one artifact
			return map[string][]byte{
				"registry.example.com/my-repo:v1.0.0": testData,
			}, nil
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the artifacts map should contain only one entry (deduplicated by bundler)
		if len(artifacts) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts))
		}

		expectedKey := "registry.example.com/my-repo:v1.0.0"
		if !bytes.Equal(artifacts[expectedKey], testData) {
			t.Errorf("expected artifact data to match test data")
		}
	})

	t.Run("ErrorParsingOCIReference", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns an invalid OCI source (missing repository part)
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "invalid-oci", Url: "oci://registry.example.com:v1.0.0"},
			}
		}

		// And the artifact builder is set up to return an error for invalid OCI reference
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, fmt.Errorf("invalid OCI reference format")
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And artifacts should be nil
		if artifacts != nil {
			t.Errorf("expected nil artifacts, got %v", artifacts)
		}
	})

	t.Run("ErrorDownloadingOCIArtifact", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns a valid OCI source
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "oci-source", Url: "oci://registry.example.com/my-repo:v1.0.0"},
			}
		}

		// And the artifact builder is set up to return a download error
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, fmt.Errorf("mock download error")
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And artifacts should be nil
		if artifacts != nil {
			t.Errorf("expected nil artifacts, got %v", artifacts)
		}
	})

	t.Run("ErrorFromRemoteImage", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns a valid OCI source
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "oci-source", Url: "oci://registry.example.com/my-repo:v1.0.0"},
			}
		}

		// And the artifact builder is set up to return a remote image error
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			return nil, fmt.Errorf("mock remote image error")
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And artifacts should be nil
		if artifacts != nil {
			t.Errorf("expected nil artifacts, got %v", artifacts)
		}
	})

	t.Run("MixedSourceTypesWithOCI", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns mixed source types including one OCI source
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "git-source", Url: "https://github.com/example/repo.git"},
				{Name: "oci-source", Url: "oci://registry.example.com/my-repo:v1.0.0"},
				{Name: "local-source", Url: "file:///local/path"},
			}
		}

		// And the artifact builder is set up to return data for only the OCI source
		mockArtifact := mocks.Injector.Resolve("artifactBuilder").(*bundler.MockArtifact)
		mockArtifact.PullFunc = func(ociRefs []string) (map[string][]byte, error) {
			if len(ociRefs) != 1 || ociRefs[0] != "oci://registry.example.com/my-repo:v1.0.0" {
				return nil, fmt.Errorf("unexpected OCI refs: %v", ociRefs)
			}
			return map[string][]byte{
				"registry.example.com/my-repo:v1.0.0": []byte("test artifact data"),
			}, nil
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And only the OCI artifact should be cached (non-OCI sources ignored)
		if len(artifacts) != 1 {
			t.Errorf("expected 1 artifact, got %d", len(artifacts))
		}

		expectedKey := "registry.example.com/my-repo:v1.0.0"
		expectedData := []byte("test artifact data")
		if !bytes.Equal(artifacts[expectedKey], expectedData) {
			t.Errorf("expected artifact data to match test data")
		}
	})

	t.Run("MixedSourceTypesNoOCI", func(t *testing.T) {
		// Given a TerraformGenerator with mocks
		generator, mocks := setup(t)

		// And GetSources returns mixed source types with no OCI sources
		mocks.BlueprintHandler.GetSourcesFunc = func() []blueprintv1alpha1.Source {
			return []blueprintv1alpha1.Source{
				{Name: "git-source", Url: "https://github.com/example/repo.git"},
				{Name: "local-source", Url: "file:///local/path"},
				{Name: "http-source", Url: "https://releases.example.com/module.tar.gz"},
			}
		}

		// When preloadOCIArtifacts is called
		artifacts, err := generator.preloadOCIArtifacts()

		// Then no error should occur (non-OCI sources should be ignored)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And artifacts map should be empty (no OCI sources to process)
		if len(artifacts) != 0 {
			t.Errorf("expected 0 artifacts, got %d", len(artifacts))
		}
	})
}
