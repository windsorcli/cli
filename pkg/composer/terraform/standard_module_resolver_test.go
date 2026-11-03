package terraform_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStandardModuleResolver_NewStandardModuleResolver(t *testing.T) {
	t.Run("CreatesStandardModuleResolver", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		resolver := terraform.NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		if resolver == nil {
			t.Fatal("Expected non-nil standard module resolver")
		}
		if resolver.BaseModuleResolver == nil {
			t.Error("Expected BaseModuleResolver to be set")
		}
	})
}

func TestStandardModuleResolver_NewStandardModuleResolverWithDependencies(t *testing.T) {
	setup := func(t *testing.T) (*terraform.StandardModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := terraform.NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a standard module resolver with valid dependencies
		resolver, _ := setup(t)

		// Then all handlers are set
		if resolver.BaseModuleResolver.BlueprintHandler() == nil {
			t.Error("Expected blueprintHandler to be set")
		}
		if resolver.BaseModuleResolver.Runtime().Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.BaseModuleResolver.Runtime().ConfigHandler == nil {
			t.Error("Expected configHandler to be set")
		}
	})
}

func TestStandardModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T) (*terraform.StandardModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := terraform.NewStandardModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		resolver.BaseModuleResolver.SetShims(mocks.Shims)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with proper JSON unmarshaling for complete path coverage
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"

		// Use real JSON unmarshaling to exercise the parsing logic
		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				// Return terraform init output with detected path different from standard
				return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /detected/module/path"}`, nil
			}
			return "", nil
		}

		// When processing modules
		err := resolver.ProcessModules()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("PanicsWhenBlueprintHandlerIsNil", func(t *testing.T) {
		// Given a resolver with nil blueprint handler
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.SetBlueprintHandler(nil)

		// When ProcessModules is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when blueprint handler is nil")
			}
		}()
		_ = resolver.ProcessModules()
	})

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		// Given a resolver with MkdirAll shim returning error
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Shims().MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to create module directory
		if err == nil || !strings.Contains(err.Error(), "failed to create module directory") {
			t.Errorf("Expected mkdir error, got: %v", err)
		}
	})

	t.Run("HandlesChdirError", func(t *testing.T) {
		// Given a resolver with Chdir shim returning error
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Shims().Chdir = func(path string) error {
			return errors.New("chdir error")
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to change to module directory
		if err == nil || !strings.Contains(err.Error(), "failed to change to module directory") {
			t.Errorf("Expected chdir error, got: %v", err)
		}
	})

	t.Run("HandlesGetConfigRootError", func(t *testing.T) {
		// Given a resolver with empty WindsorScratchPath
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Runtime().WindsorScratchPath = ""

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to get windsor scratch path
		if err == nil || !strings.Contains(err.Error(), "windsor scratch path is empty") {
			t.Errorf("Expected windsor scratch path error, got: %v", err)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		// Given a resolver with Setenv shim returning error for TF_DATA_DIR
		resolver, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		_ = mocks
		resolver.BaseModuleResolver.Runtime().ConfigHandler = mockConfigHandler
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/mock/config/root"
		resolver.BaseModuleResolver.Shims().Setenv = func(key, value string) error {
			if key == "TF_DATA_DIR" {
				return errors.New("setenv error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to set TF_DATA_DIR
		if err == nil || !strings.Contains(err.Error(), "failed to set TF_DATA_DIR") {
			t.Errorf("Expected setenv error, got: %v", err)
		}
	})

	t.Run("HandlesTerraformInitError", func(t *testing.T) {
		// Given a resolver with Shell.ExecProgressFunc returning error for terraform init
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"
		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", errors.New("terraform init error")
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to initialize terraform
		if err == nil || !strings.Contains(err.Error(), "failed to initialize terraform") {
			t.Errorf("Expected terraform init error, got: %v", err)
		}
	})

	t.Run("HandlesTerraformNotAvailable", func(t *testing.T) {
		// Given a resolver whose ToolsManager reports terraform as missing
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"
		mockToolsManager := mocks.Runtime.ToolsManager.(*tools.MockToolsManager)
		mockToolsManager.CheckTerraformFunc = func() error {
			return errors.New("Terraform >= 1.0.0 is required but was not found on PATH.\n  Install: https://developer.hashicorp.com/terraform/install")
		}
		execCalled := false
		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				execCalled = true
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then the friendly missing-tool error is returned without ever shelling out to terraform
		if err == nil || !strings.Contains(err.Error(), "was not found on PATH") {
			t.Errorf("Expected missing-tool error, got: %v", err)
		}
		if execCalled {
			t.Error("Expected terraform init to be skipped when the tool check fails")
		}
	})

	t.Run("ChecksTerraformOnceAcrossMultipleComponents", func(t *testing.T) {
		// Given a resolver processing two standard-source components
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "module-one",
					Source:   "git::https://github.com/test/module-one.git",
					FullPath: filepath.Join("/test", "terraform", "module-one"),
				},
				{
					Path:     "module-two",
					Source:   "git::https://github.com/test/module-two.git",
					FullPath: filepath.Join("/test", "terraform", "module-two"),
				},
			}
		}
		checkTerraformCalls := 0
		mockToolsManager := mocks.Runtime.ToolsManager.(*tools.MockToolsManager)
		mockToolsManager.CheckTerraformFunc = func() error {
			checkTerraformCalls++
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then CheckTerraform runs exactly once despite two matching components
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if checkTerraformCalls != 1 {
			t.Errorf("Expected CheckTerraform to be called once, got %d calls", checkTerraformCalls)
		}
	})

	t.Run("HandlesWriteShimMainTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for main.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Shims().WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "main.tf") {
				return errors.New("write main.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write main.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected write main.tf error, got: %v", err)
		}
	})

	t.Run("UsesWindsorScratchPathForTFDataDir", func(t *testing.T) {
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		expectedWindsorScratchPath := filepath.Join(tmpDir, ".windsor", "contexts", "local")
		resolver.BaseModuleResolver.Runtime().WindsorScratchPath = expectedWindsorScratchPath

		var actualTFDataDir string
		resolver.BaseModuleResolver.Shims().Setenv = func(key, value string) error {
			if key == "TF_DATA_DIR" {
				actualTFDataDir = value
			}
			return nil
		}

		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /path/to/module"}`, nil
			}
			return "", nil
		}

		err := resolver.ProcessModules()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedTFDataDir := filepath.Join(expectedWindsorScratchPath, ".terraform", "test-module")
		if actualTFDataDir != expectedTFDataDir {
			t.Errorf("Expected TF_DATA_DIR to be %s, got %s", expectedTFDataDir, actualTFDataDir)
		}
	})

	t.Run("UsesWindsorScratchPathForModulePath", func(t *testing.T) {
		resolver, mocks := setup(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		expectedWindsorScratchPath := filepath.Join(tmpDir, ".windsor", "contexts", "local")
		resolver.BaseModuleResolver.Runtime().WindsorScratchPath = expectedWindsorScratchPath

		var modulePathUsed string
		resolver.BaseModuleResolver.Shims().Stat = func(path string) (os.FileInfo, error) {
			if strings.Contains(path, "variables.tf") && strings.Contains(path, ".terraform") {
				modulePathUsed = filepath.Dir(path)
			}
			return nil, os.ErrNotExist
		}

		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /path/to/module"}`, nil
			}
			return "", nil
		}

		err := resolver.ProcessModules()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedModulePath := filepath.Join(expectedWindsorScratchPath, ".terraform", "test-module", "modules", "main", "terraform", "test-module")
		if modulePathUsed != "" && modulePathUsed != expectedModulePath {
			t.Errorf("Expected module path to be %s, got %s", expectedModulePath, modulePathUsed)
		}
	})

	t.Run("HandlesEmptyWindsorScratchPath", func(t *testing.T) {
		resolver, mocks := setup(t)
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "terraform", "test-module")
		os.MkdirAll(moduleDir, 0755)

		resolver.BaseModuleResolver.Runtime().WindsorScratchPath = ""

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test-module",
					Source:   "git::https://github.com/test/module.git",
					FullPath: moduleDir,
				},
			}
		}

		resolver.BaseModuleResolver.Shims().Chdir = func(path string) error {
			return nil
		}

		err := resolver.ProcessModules()

		if err == nil {
			t.Error("Expected error when WindsorScratchPath is empty")
		}
		if err != nil && !strings.Contains(err.Error(), "windsor scratch path is empty") {
			t.Errorf("Expected error about empty windsor scratch path, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimVariablesTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for variables.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"

		// Mock Glob to return a variables.tf file so writeShimVariablesTf will try to process it
		originalGlob := resolver.BaseModuleResolver.Shims().Glob
		resolver.BaseModuleResolver.Shims().Glob = func(pattern string) ([]string, error) {
			if strings.HasSuffix(pattern, "*.tf") {
				// Return a variables.tf file for any .tf pattern
				moduleDir := strings.TrimSuffix(pattern, "/*.tf")
				return []string{filepath.Join(moduleDir, "variables.tf")}, nil
			}
			return originalGlob(pattern)
		}

		// Mock ReadFile to return variable content
		originalReadFile := resolver.BaseModuleResolver.Shims().ReadFile
		resolver.BaseModuleResolver.Shims().ReadFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "variables.tf") {
				return []byte(`variable "test" { type = string }`), nil
			}
			return originalReadFile(path)
		}

		resolver.BaseModuleResolver.Shims().WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "variables.tf") {
				return errors.New("write variables.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write variables.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write variables.tf") {
			t.Errorf("Expected write variables.tf error, got: %v", err)
		}
	})

	t.Run("HandlesWriteShimOutputsTfError", func(t *testing.T) {
		// Given a resolver with WriteFile shim returning error for outputs.tf
		resolver, _ := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"
		resolver.BaseModuleResolver.Shims().WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "outputs.tf") {
				return errors.New("write outputs.tf error")
			}
			return nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then an error is returned indicating failure to write outputs.tf
		if err == nil || !strings.Contains(err.Error(), "failed to write outputs.tf") {
			t.Errorf("Expected write outputs.tf error, got: %v", err)
		}
	})

	// Edge cases for component filtering
	t.Run("HandlesEmptySourceComponents", func(t *testing.T) {
		// Given a resolver with a component having empty source
		resolver, mocks := setup(t)
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{{
				Path:     "test-module",
				Source:   "",
				FullPath: "/mock/project/terraform/test-module",
			}}
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (component is skipped)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesTerraformInitOutputParsing", func(t *testing.T) {
		// Given a resolver with custom JsonUnmarshal and Stat shims for output parsing edge cases
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"
		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			if initOutput, ok := v.(*terraform.TerraformInitOutput); ok {
				jsonStr := string(data)
				if strings.Contains(jsonStr, `"invalid_json"`) {
					return errors.New("invalid JSON")
				}
				if strings.Contains(jsonStr, `"non_log_type"`) {
					initOutput.Type = "info"
					initOutput.Message = "some info message"
					return nil
				}
				if strings.Contains(jsonStr, `"no_main_in"`) {
					initOutput.Type = "log"
					initOutput.Message = "some other message"
					return nil
				}
				if strings.Contains(jsonStr, `"main_in_at_end"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in"
					return nil
				}
				if strings.Contains(jsonStr, `"empty_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in   "
					return nil
				}
				if strings.Contains(jsonStr, `"nonexistent_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in /nonexistent/path"
					return nil
				}
				if strings.Contains(jsonStr, `"valid_path"`) {
					initOutput.Type = "log"
					initOutput.Message = "- main in /valid/path"
					return nil
				}
			}
			return json.Unmarshal(data, v)
		}

		// And Stat shim only succeeds for /valid/path
		resolver.BaseModuleResolver.Shims().Stat = func(path string) (os.FileInfo, error) {
			if path == "/valid/path" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And Shell.ExecProgressFunc returns all edge case lines
		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `invalid json line
{"@type":"info","@message":"some info message","non_log_type":true}
{"@type":"log","@message":"some other message","no_main_in":true}
{"@type":"log","@message":"- main in","main_in_at_end":true}
{"@type":"log","@message":"- main in   ","empty_path":true}
{"@type":"log","@message":"- main in /nonexistent/path","nonexistent_path":true}
{"@type":"log","@message":"- main in /valid/path","valid_path":true}`, nil
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (all output parsing edge cases handled)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesMultipleLogLinesWithValidPaths", func(t *testing.T) {
		// Given a resolver that receives multiple log lines with valid paths
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"

		firstPath := "/first/path"
		secondPath := "/second/path"

		detectedPathStatCalls := 0
		resolver.BaseModuleResolver.Shims().Stat = func(path string) (os.FileInfo, error) {
			if path == firstPath {
				detectedPathStatCalls++
				return nil, nil
			}
			if path == secondPath {
				detectedPathStatCalls++
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `{"@type":"log","@message":"- main in /first/path"}
{"@type":"log","@message":"- main in /second/path"}`, nil
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And Stat should only be called once for detected paths (breaks after first valid path)
		if detectedPathStatCalls != 1 {
			t.Errorf("Expected Stat to be called once for detected paths (breaks after first valid path), got %d calls", detectedPathStatCalls)
		}
	})

	t.Run("HandlesJsonUnmarshalErrors", func(t *testing.T) {
		// Given a resolver with JsonUnmarshal returning errors for some lines
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"

		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			jsonStr := string(data)
			if strings.Contains(jsonStr, "invalid") {
				return errors.New("invalid JSON")
			}
			return json.Unmarshal(data, v)
		}

		resolver.BaseModuleResolver.Shims().Stat = func(path string) (os.FileInfo, error) {
			if path == "/valid/path" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `invalid json line
{"@type":"log","@message":"- main in /valid/path"}`, nil
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (invalid JSON lines are skipped)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("HandlesEmptyLinesInOutput", func(t *testing.T) {
		// Given a resolver with empty lines in terraform init output
		resolver, mocks := setup(t)
		resolver.BaseModuleResolver.Runtime().ConfigRoot = "/test/config"

		resolver.BaseModuleResolver.Shims().JsonUnmarshal = func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		}

		resolver.BaseModuleResolver.Shims().Stat = func(path string) (os.FileInfo, error) {
			if path == "/valid/path" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
				return `


{"@type":"log","@message":"- main in /valid/path"}


`, nil
			}
			return "", nil
		}

		// When ProcessModules is called
		err := resolver.ProcessModules()

		// Then no error is returned (empty lines are skipped)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}
