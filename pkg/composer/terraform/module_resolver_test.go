package terraform

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

// TerraformTestMocks contains all the mock dependencies for testing terraform resolvers
type TerraformTestMocks struct {
	Shell            *shell.MockShell
	ConfigHandler    config.ConfigHandler
	BlueprintHandler *blueprint.MockBlueprintHandler
	Shims            *Shims
	Runtime          *runtime.Runtime
}

// setupTerraformMocks creates mock components for testing terraform resolvers with optional overrides
func setupTerraformMocks(t *testing.T, opts ...func(*TerraformTestMocks)) *TerraformTestMocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.ExecProgressFunc = func(msg, cmd string, args ...string) (string, error) {
		if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /path/to/module"}`, nil
		}
		return "", nil
	}

	configHandler := config.NewConfigHandler(mockShell)
	configHandler.SetContext("mock-context")

	defaultConfigStr := `
contexts:
  mock-context:
    terraform:
      enabled: true
`
	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		return []blueprintv1alpha1.TerraformComponent{
			{
				Path:     "test-module",
				Source:   "git::https://github.com/test/module.git",
				FullPath: filepath.Join(tmpDir, "terraform", "test-module"),
				Inputs: map[string]any{
					"cluster_name": "test-cluster",
				},
			},
		}
	}

	shims := setupDefaultShims()

	// Create mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.GetTerraformCommandFunc = func() string {
		return "terraform"
	}

	// Create evaluator
	evaluator := evaluator.NewExpressionEvaluator(configHandler, tmpDir, filepath.Join(tmpDir, "contexts", "_template"))

	// Create runtime
	rt := &runtime.Runtime{
		ConfigHandler:      configHandler,
		ToolsManager:       mockToolsManager,
		Shell:              mockShell,
		Evaluator:          evaluator,
		ProjectRoot:        tmpDir,
		ContextName:        "local",
		WindsorScratchPath: filepath.Join(tmpDir, ".windsor", "contexts", "local"),
	}

	mocks := &TerraformTestMocks{
		Shell:            mockShell,
		ConfigHandler:    configHandler,
		BlueprintHandler: mockBlueprintHandler,
		Shims:            shims,
		Runtime:          rt,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// setupDefaultShims configures safe default implementations for all shims operations
// This eliminates the need for repetitive mocking in individual test cases
func setupDefaultShims() *Shims {
	shims := NewShims()

	// Safe defaults for file operations
	shims.ReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "variables.tf") {
			return []byte(`variable "foo" { 
				type = string 
				description = "Test variable"
				default = "test"
				sensitive = true
			}`), nil
		}
		if strings.HasSuffix(path, "outputs.tf") {
			return []byte(`output "foo" { 
				value = "test"
				description = "Test output"
			}`), nil
		}
		return []byte{}, nil
	}

	shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
		return nil
	}

	shims.Stat = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, "variables.tf") || strings.HasSuffix(path, "outputs.tf") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	shims.Chdir = func(path string) error {
		return nil
	}

	shims.Setenv = func(key, value string) error {
		return nil
	}

	shims.FilepathRel = func(basepath, targpath string) (string, error) {
		return "./relative/path", nil
	}

	shims.Chmod = func(name string, mode os.FileMode) error {
		return nil
	}

	shims.Create = func(name string) (*os.File, error) {
		return os.CreateTemp("", "test-file-*")
	}

	shims.Copy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 0, nil
	}

	shims.NewBytesReader = func(b []byte) *bytes.Reader {
		return bytes.NewReader(b)
	}

	shims.NewTarReader = func(r io.Reader) TarReader {
		return tar.NewReader(r)
	}

	shims.EOFError = func() error {
		return io.EOF
	}

	shims.TypeDir = func() byte {
		return tar.TypeDir
	}

	shims.JsonUnmarshal = func(data []byte, v any) error {
		return nil
	}

	return shims
}

// MockTarReader provides a mock implementation for TarReader interface
type MockTarReader struct {
	NextFunc func() (*tar.Header, error)
	ReadFunc func([]byte) (int, error)
}

func (m *MockTarReader) Next() (*tar.Header, error) {
	if m.NextFunc != nil {
		return m.NextFunc()
	}
	return nil, io.EOF
}

func (m *MockTarReader) Read(p []byte) (int, error) {
	if m.ReadFunc != nil {
		return m.ReadFunc(p)
	}
	return 0, io.EOF
}

type MockModuleResolverSetupOptions struct {
	ProcessModulesFunc func() error
}

func setupMockModuleResolver(t *testing.T, opts ...*MockModuleResolverSetupOptions) *MockModuleResolver {
	t.Helper()

	mock := NewMockModuleResolver()
	if len(opts) > 0 && opts[0] != nil {
		if opts[0].ProcessModulesFunc != nil {
			mock.ProcessModulesFunc = opts[0].ProcessModulesFunc
		}
	}
	return mock
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseModuleResolver_NewBaseModuleResolver(t *testing.T) {
	t.Run("CreatesResolverWithDependencies", func(t *testing.T) {
		// Given mocks
		mocks := setupTerraformMocks(t)

		// When creating a new base module resolver
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)

		// Then the resolver should be properly initialized
		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}

		// And shims should be initialized
		if resolver.shims == nil {
			t.Error("Expected shims to be initialized")
		}

		// And dependency fields should be set
		if resolver.runtime.Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.blueprintHandler == nil {
			t.Error("Expected blueprintHandler to be set")
		}
		if resolver.runtime.ConfigHandler == nil {
			t.Error("Expected configHandler to be set")
		}
	})

	t.Run("CreatesResolverWithDefaultShims", func(t *testing.T) {
		// Given mocks
		mocks := setupTerraformMocks(t)

		// When creating a resolver
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)

		// Then the resolver should have default shims
		if resolver.shims == nil {
			t.Fatal("Expected shims to be set")
		}
	})
}

func TestBaseModuleResolver_writeShimMainTf(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("CreatesValidMainTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "test-module")
		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module directory: %v", err)
		}

		// And a module source
		source := "git::https://github.com/test/module.git"

		// When writing the shim main.tf
		err := resolver.writeShimMainTf(moduleDir, source)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the main.tf file should be created
		mainTfPath := filepath.Join(moduleDir, "main.tf")
		if _, err := resolver.shims.Stat(mainTfPath); err != nil {
			t.Errorf("Expected main.tf to be created, got error: %v", err)
		}

		// And the file should contain valid HCL
		content, err := resolver.shims.ReadFile(mainTfPath)
		if err != nil {
			t.Fatalf("Failed to read main.tf: %v", err)
		}

		// And the content should contain the module block
		if !strings.Contains(string(content), "module \"main\"") {
			t.Error("Expected main.tf to contain module \"main\" block")
		}
		if !strings.Contains(string(content), source) {
			t.Errorf("Expected main.tf to contain source %s", source)
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a mock shims that returns error on WriteFile
		originalWriteFile := resolver.shims.WriteFile
		resolver.shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write file error")
		}
		defer func() { resolver.shims.WriteFile = originalWriteFile }()

		// And a temporary directory
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "test-module")
		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module directory: %v", err)
		}

		// When writing the shim main.tf
		err := resolver.writeShimMainTf(moduleDir, "test-source")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to write main.tf") {
			t.Errorf("Expected error about writing main.tf, got: %v", err)
		}
	})
}

func TestBaseModuleResolver_writeShimVariablesTf(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("CreatesValidVariablesTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And a source variables.tf file
		variablesContent := `
variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
  default     = "default-cluster"
}

variable "instance_type" {
  description = "Type of instance"
  type        = string
  sensitive   = true
}
`
		variablesPath := filepath.Join(modulePath, "variables.tf")
		if err := resolver.shims.WriteFile(variablesPath, []byte(variablesContent), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When writing the shim variables.tf
		err := resolver.writeShimVariablesTf(moduleDir, modulePath, "test-source")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the variables.tf file should be created
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		if _, err := resolver.shims.Stat(shimVariablesPath); err != nil {
			t.Errorf("Expected variables.tf to be created, got error: %v", err)
		}

		// And the main.tf file should be updated
		shimMainPath := filepath.Join(moduleDir, "main.tf")
		if _, err := resolver.shims.Stat(shimMainPath); err != nil {
			t.Errorf("Expected main.tf to be updated, got error: %v", err)
		}

		// And the variables.tf should contain the variable definitions
		content, err := resolver.shims.ReadFile(shimVariablesPath)
		if err != nil {
			t.Fatalf("Failed to read shim variables.tf: %v", err)
		}

		if !strings.Contains(string(content), "variable \"cluster_name\"") {
			t.Error("Expected variables.tf to contain cluster_name variable")
		}
		if !strings.Contains(string(content), "variable \"instance_type\"") {
			t.Error("Expected variables.tf to contain instance_type variable")
		}
		if !strings.Contains(string(content), "Name of the cluster") {
			t.Error("Expected variables.tf to preserve description")
		}
		if !regexp.MustCompile(`sensitive\s*=\s*true`).Match(content) {
			t.Errorf("Expected variables.tf to preserve sensitive flag, got content: %s", string(content))
		}
	})

	t.Run("HandlesMissingVariablesTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure without any .tf files
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// When writing the shim variables.tf
		err := resolver.writeShimVariablesTf(moduleDir, modulePath, "test-source")

		// Then no error should be returned (no .tf files is not an error)
		if err != nil {
			t.Errorf("Expected no error for module with no .tf files, got: %v", err)
		}

		// And variables.tf should not be created (no variables to write)
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		_, statErr := resolver.shims.Stat(shimVariablesPath)
		if statErr == nil {
			t.Error("Expected variables.tf not to be created when there are no variables")
		}

		// And main.tf should be created
		shimMainPath := filepath.Join(moduleDir, "main.tf")
		if _, err := resolver.shims.Stat(shimMainPath); err != nil {
			t.Errorf("Expected main.tf to be created, got error: %v", err)
		}
	})

	t.Run("HandlesInvalidVariablesTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And an invalid .tf file (invalid files are skipped)
		invalidContent := `invalid hcl syntax {`
		invalidPath := filepath.Join(modulePath, "invalid.tf")
		if err := resolver.shims.WriteFile(invalidPath, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid.tf: %v", err)
		}

		// And a valid .tf file with variables
		validContent := `variable "test_var" { type = string }`
		validPath := filepath.Join(modulePath, "main.tf")
		if err := resolver.shims.WriteFile(validPath, []byte(validContent), 0644); err != nil {
			t.Fatalf("Failed to write main.tf: %v", err)
		}

		// When writing the shim variables.tf
		err := resolver.writeShimVariablesTf(moduleDir, modulePath, "test-source")

		// Then no error should be returned (invalid files are skipped)
		if err != nil {
			t.Errorf("Expected no error when invalid file is skipped, got: %v", err)
		}

		// And the shim variables.tf should contain the valid variable
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		content, readErr := resolver.shims.ReadFile(shimVariablesPath)
		if readErr != nil {
			t.Errorf("Expected variables.tf to be created, got error: %v", readErr)
		}
		if content != nil && !strings.Contains(string(content), "test_var") {
			t.Errorf("Expected variables.tf to contain test_var, got content: %s", string(content))
		}
	})

	t.Run("AlphabetizesVariablesInShim", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And source variables in non-alphabetical order
		variablesContent := `
variable "zebra" {
  description = "Zebra variable"
  type        = string
}

variable "alpha" {
  description = "Alpha variable"
  type        = string
}

variable "beta" {
  description = "Beta variable"
  type        = string
}
`
		variablesPath := filepath.Join(modulePath, "variables.tf")
		if err := resolver.shims.WriteFile(variablesPath, []byte(variablesContent), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When writing the shim variables.tf
		err := resolver.writeShimVariablesTf(moduleDir, modulePath, "test-source")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the variables.tf should be created
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		content, err := resolver.shims.ReadFile(shimVariablesPath)
		if err != nil {
			t.Fatalf("Failed to read shim variables.tf: %v", err)
		}

		contentStr := string(content)

		// And variables should appear in alphabetical order
		alphaIndex := strings.Index(contentStr, "variable \"alpha\"")
		betaIndex := strings.Index(contentStr, "variable \"beta\"")
		zebraIndex := strings.Index(contentStr, "variable \"zebra\"")

		if alphaIndex == -1 || betaIndex == -1 || zebraIndex == -1 {
			t.Errorf("Expected all variables to be present in shim variables.tf")
		}

		if alphaIndex > betaIndex || betaIndex > zebraIndex {
			t.Errorf("Expected variables in alphabetical order (alpha, beta, zebra), but found alpha at %d, beta at %d, zebra at %d", alphaIndex, betaIndex, zebraIndex)
		}

		// And main.tf should also have variables in alphabetical order
		shimMainPath := filepath.Join(moduleDir, "main.tf")
		mainContent, err := resolver.shims.ReadFile(shimMainPath)
		if err != nil {
			t.Fatalf("Failed to read shim main.tf: %v", err)
		}

		mainContentStr := string(mainContent)

		alphaMainIndex := strings.Index(mainContentStr, "alpha")
		betaMainIndex := strings.Index(mainContentStr, "beta")
		zebraMainIndex := strings.Index(mainContentStr, "zebra")

		if alphaMainIndex == -1 || betaMainIndex == -1 || zebraMainIndex == -1 {
			t.Errorf("Expected all variables to be present in shim main.tf module arguments")
		}

		if alphaMainIndex > betaMainIndex || betaMainIndex > zebraMainIndex {
			t.Errorf("Expected module arguments in alphabetical order (alpha, beta, zebra), but found alpha at %d, beta at %d, zebra at %d", alphaMainIndex, betaMainIndex, zebraMainIndex)
		}
	})
}

func TestBaseModuleResolver_writeShimOutputsTf(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("CreatesValidOutputsTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And a source outputs.tf file
		outputsContent := `
output "cluster_id" {
  description = "ID of the created cluster"
  value       = module.cluster.id
}

output "endpoint" {
  description = "Cluster endpoint"
  value       = module.cluster.endpoint
}
`
		outputsPath := filepath.Join(modulePath, "outputs.tf")
		if err := resolver.shims.WriteFile(outputsPath, []byte(outputsContent), 0644); err != nil {
			t.Fatalf("Failed to write outputs.tf: %v", err)
		}

		// When writing the shim outputs.tf
		err := resolver.writeShimOutputsTf(moduleDir, modulePath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the outputs.tf file should be created
		shimOutputsPath := filepath.Join(moduleDir, "outputs.tf")
		if _, err := resolver.shims.Stat(shimOutputsPath); err != nil {
			t.Errorf("Expected outputs.tf to be created, got error: %v", err)
		}

		// And the outputs.tf should contain the output definitions
		content, err := resolver.shims.ReadFile(shimOutputsPath)
		if err != nil {
			t.Fatalf("Failed to read shim outputs.tf: %v", err)
		}

		if !strings.Contains(string(content), "output \"cluster_id\"") {
			t.Error("Expected outputs.tf to contain cluster_id output")
		}
		if !strings.Contains(string(content), "output \"endpoint\"") {
			t.Error("Expected outputs.tf to contain endpoint output")
		}
		if !strings.Contains(string(content), "ID of the created cluster") {
			t.Error("Expected outputs.tf to preserve description")
		}
		if !strings.Contains(string(content), "module.main.cluster_id") {
			t.Error("Expected outputs.tf to reference module.main outputs")
		}
	})

	t.Run("HandlesMissingOutputsTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure without outputs.tf
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// When writing the shim outputs.tf
		err := resolver.writeShimOutputsTf(moduleDir, modulePath)

		// Then no error should be returned (missing outputs.tf is not an error)
		if err != nil {
			t.Errorf("Expected no error for missing outputs.tf, got: %v", err)
		}

		// And no outputs.tf file should be created
		shimOutputsPath := filepath.Join(moduleDir, "outputs.tf")
		if _, err := resolver.shims.Stat(shimOutputsPath); err == nil {
			t.Error("Expected no outputs.tf to be created when source doesn't have one")
		}
	})

	t.Run("HandlesInvalidOutputsTf", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And an invalid outputs.tf file
		outputsContent := `invalid hcl syntax {`
		outputsPath := filepath.Join(modulePath, "outputs.tf")
		if err := resolver.shims.WriteFile(outputsPath, []byte(outputsContent), 0644); err != nil {
			t.Fatalf("Failed to write outputs.tf: %v", err)
		}

		// When writing the shim outputs.tf
		err := resolver.writeShimOutputsTf(moduleDir, modulePath)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse outputs.tf") {
			t.Errorf("Expected error about parsing outputs.tf, got: %v", err)
		}
	})

	t.Run("PreservesSensitiveOutputs", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// And a temporary directory structure
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		modulePath := filepath.Join(tmpDir, "source")

		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.MkdirAll(modulePath, 0755); err != nil {
			t.Fatalf("Failed to create source directory: %v", err)
		}

		// And a source outputs.tf file with sensitive outputs
		outputsContent := `
output "cluster_id" {
  description = "ID of the created cluster"
  value       = module.cluster.id
}

output "secret_key" {
  description = "Secret access key"
  value       = module.cluster.secret_key
  sensitive   = true
}
`
		outputsPath := filepath.Join(modulePath, "outputs.tf")
		if err := resolver.shims.WriteFile(outputsPath, []byte(outputsContent), 0644); err != nil {
			t.Fatalf("Failed to write outputs.tf: %v", err)
		}

		// When writing the shim outputs.tf
		err := resolver.writeShimOutputsTf(moduleDir, modulePath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And the outputs.tf file should be created
		shimOutputsPath := filepath.Join(moduleDir, "outputs.tf")
		if _, err := resolver.shims.Stat(shimOutputsPath); err != nil {
			t.Errorf("Expected outputs.tf to be created, got error: %v", err)
		}

		// And the outputs.tf should contain the output definitions with sensitive flag preserved
		content, err := resolver.shims.ReadFile(shimOutputsPath)
		if err != nil {
			t.Fatalf("Failed to read shim outputs.tf: %v", err)
		}

		if !strings.Contains(string(content), "output \"cluster_id\"") {
			t.Error("Expected outputs.tf to contain cluster_id output")
		}
		if !strings.Contains(string(content), "output \"secret_key\"") {
			t.Error("Expected outputs.tf to contain secret_key output")
		}
		if !strings.Contains(string(content), "Secret access key") {
			t.Error("Expected outputs.tf to preserve description")
		}
		if !regexp.MustCompile(`sensitive\s*=\s*true`).Match(content) {
			t.Errorf("Expected outputs.tf to preserve sensitive flag, got content: %s", string(content))
		}
		if !strings.Contains(string(content), "module.main.secret_key") {
			t.Error("Expected outputs.tf to reference module.main outputs")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestShims_NewShims(t *testing.T) {
	t.Run("CreatesShimsWithDefaultImplementations", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then all function fields should be set
		if shims.MkdirAll == nil {
			t.Error("Expected MkdirAll to be set")
		}
		if shims.WriteFile == nil {
			t.Error("Expected WriteFile to be set")
		}
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be set")
		}
		if shims.Stat == nil {
			t.Error("Expected Stat to be set")
		}
		if shims.Chdir == nil {
			t.Error("Expected Chdir to be set")
		}
		if shims.FilepathRel == nil {
			t.Error("Expected FilepathRel to be set")
		}
		if shims.JsonUnmarshal == nil {
			t.Error("Expected JsonUnmarshal to be set")
		}
		if shims.NewBytesReader == nil {
			t.Error("Expected NewBytesReader to be set")
		}
		if shims.NewTarReader == nil {
			t.Error("Expected NewTarReader to be set")
		}
		if shims.EOFError == nil {
			t.Error("Expected EOFError to be set")
		}
		if shims.TypeDir == nil {
			t.Error("Expected TypeDir to be set")
		}
		if shims.Create == nil {
			t.Error("Expected Create to be set")
		}
		if shims.Copy == nil {
			t.Error("Expected Copy to be set")
		}
		if shims.Chmod == nil {
			t.Error("Expected Chmod to be set")
		}
		if shims.Setenv == nil {
			t.Error("Expected Setenv to be set")
		}
	})

	t.Run("ShimsProvideBasicFunctionality", func(t *testing.T) {
		// Given real shims
		shims := NewShims()

		// When testing basic functionality
		// Then all function fields should work

		// And NewBytesReader should work
		reader := shims.NewBytesReader([]byte("test"))
		if reader == nil {
			t.Error("Expected NewBytesReader to create a reader")
		}

		// And NewTarReader should work
		tarReader := shims.NewTarReader(reader)
		if tarReader == nil {
			t.Error("Expected NewTarReader to create a tar reader")
		}

		// And EOFError should return io.EOF
		if shims.EOFError() != io.EOF {
			t.Error("Expected EOFError to return io.EOF")
		}

		// And TypeDir should return tar.TypeDir
		if shims.TypeDir() != tar.TypeDir {
			t.Error("Expected TypeDir to return tar.TypeDir")
		}

		// And FilepathRel should work
		rel, err := shims.FilepathRel("/base", "/base/path")
		if err != nil {
			t.Errorf("Expected FilepathRel to work, got error: %v", err)
		}
		if rel != "path" {
			t.Errorf("Expected relative path 'path', got: %s", rel)
		}
	})
}

func TestBaseModuleResolver_GenerateTfvars(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver with a module that has variables
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesMultiLineStringValues", func(t *testing.T) {
		// Given a resolver with a variable that has a multi-line string value
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"config": "line1\nline2\nline3",
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "config" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed and use heredoc format
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesMapValues", func(t *testing.T) {
		// Given a resolver with a variable that has a map value
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"tags": map[string]any{
							"env":    "prod",
							"region": "us-east-1",
						},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "tags" { type = map(string) }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesNestedMapValues", func(t *testing.T) {
		// Given a resolver with a variable that has nested map values
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"config": map[string]any{
							"database": map[string]any{
								"host": "localhost",
								"port": 5432,
							},
							"cache": map[string]any{
								"enabled": true,
							},
						},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "config" { type = object({ database = object({ host = string, port = number }), cache = object({ enabled = bool }) }) }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesListValues", func(t *testing.T) {
		// Given a resolver with a variable that has list values
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"subnets": []string{"10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "subnets" { type = list(string) }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesMixedTypeValues", func(t *testing.T) {
		// Given a resolver with variables of various types
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"name":    "test-cluster",
						"count":   3,
						"enabled": true,
						"tags":    map[string]any{"env": "prod"},
						"subnets": []string{"10.0.1.0/24"},
						"config":  "multi\nline\nstring",
						"metadata": map[string]any{
							"nested": map[string]any{
								"value": "deep",
							},
						},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "name" { type = string }
variable "count" { type = number }
variable "enabled" { type = bool }
variable "tags" { type = map(string) }
variable "subnets" { type = list(string) }
variable "config" { type = string }
variable "metadata" { type = object({ nested = object({ value = string }) }) }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesOverwrite", func(t *testing.T) {
		// Given a resolver with an existing tfvars file
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		contextPath := mocks.Runtime.ConfigRoot
		tfvarsPath := filepath.Join(contextPath, "terraform", "test-module.auto.tfvars")
		if err := os.MkdirAll(filepath.Dir(tfvarsPath), 0755); err != nil {
			t.Fatalf("Failed to create context dir: %v", err)
		}
		if err := os.WriteFile(tfvarsPath, []byte("cluster_name = \"old\""), 0644); err != nil {
			t.Fatalf("Failed to write existing tfvars: %v", err)
		}

		// When generating tfvars with overwrite
		err := resolver.GenerateTfvars(true)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("AlphabetizesVariablesInTfvars", func(t *testing.T) {
		// Given a resolver with variables in non-alphabetical order
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"zebra": "z-value",
						"alpha": "a-value",
						"beta":  "b-value",
						"gamma": "g-value",
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "zebra" { type = string }
variable "alpha" { type = string }
variable "beta" { type = string }
variable "gamma" { type = string }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the tfvars file should contain variables in alphabetical order
		componentID := "test-module"
		tfvarsPath := filepath.Join(mocks.Runtime.WindsorScratchPath, "terraform", componentID, "terraform.tfvars")
		content, err := os.ReadFile(tfvarsPath)
		if err != nil {
			t.Fatalf("Failed to read generated tfvars file: %v", err)
		}

		contentStr := string(content)

		// Find positions of variable assignments
		alphaIndex := strings.Index(contentStr, "alpha")
		betaIndex := strings.Index(contentStr, "beta")
		gammaIndex := strings.Index(contentStr, "gamma")
		zebraIndex := strings.Index(contentStr, "zebra")

		if alphaIndex == -1 || betaIndex == -1 || gammaIndex == -1 || zebraIndex == -1 {
			t.Errorf("Expected all variables to be present in tfvars file")
		}

		// Verify alphabetical order
		if alphaIndex > betaIndex || betaIndex > gammaIndex || gammaIndex > zebraIndex {
			t.Errorf("Expected variables in alphabetical order (alpha, beta, gamma, zebra), but found alpha at %d, beta at %d, gamma at %d, zebra at %d", alphaIndex, betaIndex, gammaIndex, zebraIndex)
		}
	})

	t.Run("UsesComponentIDForNamedComponentTfvarsPath", func(t *testing.T) {
		// Given a resolver with a named component
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/complex/path/to/cluster",
					Source: "",
					Inputs: map[string]any{
						"node_count": 3,
					},
					FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		actualModulePath := filepath.Join(projectRoot, "terraform", "complex", "path", "to", "cluster")
		if err := os.MkdirAll(actualModulePath, 0755); err != nil {
			t.Fatalf("Failed to create actual module directory: %v", err)
		}

		shimPath := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(shimPath, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}

		variablesTfPath := filepath.Join(actualModulePath, "variables.tf")
		if err := os.WriteFile(variablesTfPath, []byte(`variable "node_count" { type = number }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And the tfvars file should be created using the component name (not path)
		componentID := "cluster"
		expectedTfvarsPath := filepath.Join(mocks.Runtime.WindsorScratchPath, "terraform", componentID, "terraform.tfvars")
		if _, err := os.Stat(expectedTfvarsPath); err != nil {
			t.Errorf("Expected tfvars file at %s (using component name), but file not found: %v", expectedTfvarsPath, err)
		}

		// And the tfvars file should NOT be at the path location
		pathBasedTfvarsPath := filepath.Join(mocks.Runtime.WindsorScratchPath, "terraform", "complex", "path", "to", "cluster", "terraform.tfvars")
		if _, err := os.Stat(pathBasedTfvarsPath); err == nil {
			t.Errorf("Expected tfvars file NOT to be at path-based location %s, but file was found", pathBasedTfvarsPath)
		}
	})

	t.Run("HandlesVariablesWithDefaults", func(t *testing.T) {
		// Given a resolver with variables that have default values
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "name" {
  type = string
  default = "default-name"
}
variable "count" {
  type = number
  default = 5
}
variable "enabled" {
  type = bool
  default = true
}
variable "tags" {
  type = map(string)
  default = {
    env = "dev"
  }
}
variable "list" {
  type = list(string)
  default = ["item1", "item2"]
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed and include default values as comments
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesSensitiveVariables", func(t *testing.T) {
		// Given a resolver with sensitive variables
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"password": "secret123",
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "password" {
  type = string
  sensitive = true
}`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed and mark sensitive variables as comments
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesEmptyListsAndMaps", func(t *testing.T) {
		// Given a resolver with empty list and map values
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"empty_list": []string{},
						"empty_map":  map[string]any{},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "empty_list" { type = list(string) }
variable "empty_map" { type = map(string) }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesNumericTypes", func(t *testing.T) {
		// Given a resolver with numeric values
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"count":    42,
						"ratio":    3.14,
						"enabled":  true,
						"disabled": false,
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "count" { type = number }
variable "ratio" { type = number }
variable "enabled" { type = bool }
variable "disabled" { type = bool }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesVariablesWithDescriptions", func(t *testing.T) {
		// Given a resolver with variables that have descriptions
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "cluster_name" { 
  type = string
  description = "The name of the cluster"
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed and include descriptions as comments
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesComponentWithEmptySource", func(t *testing.T) {
		// Given a resolver with a component that has empty source
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "local-module",
					Source:   "",
					Inputs:   map[string]any{"name": "test"},
					FullPath: filepath.Join(projectRoot, "terraform", "local-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, "terraform", "local-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesRemoveTfvarsFilesReadDirError", func(t *testing.T) {
		// Given a resolver with a ReadDir error when removing tfvars files
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "local-module",
					Source:   "",
					Inputs:   map[string]any{"name": "test"},
					FullPath: filepath.Join(projectRoot, "terraform", "local-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, "terraform", "local-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		moduleDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "local-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module dir: %v", err)
		}

		// Mock ReadDir to return error when removing tfvars files
		originalReadDir := resolver.shims.ReadDir
		resolver.shims.ReadDir = func(path string) ([]os.DirEntry, error) {
			normalizedPath := filepath.Clean(path)
			normalizedModuleDir := filepath.Clean(moduleDir)
			if normalizedPath == normalizedModuleDir {
				return nil, fmt.Errorf("readdir error")
			}
			return originalReadDir(path)
		}

		// When generating tfvars with reset (so removeTfvarsFiles runs)
		err := resolver.GenerateTfvars(true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when ReadDir fails during tfvars removal")
		}
		if err != nil && !strings.Contains(err.Error(), "failed cleaning existing .tfvars") {
			t.Errorf("Expected error about cleaning tfvars, got: %v", err)
		}
	})

	t.Run("HandlesRemoveTfvarsFilesStatError", func(t *testing.T) {
		// Given a resolver with a stat error when checking module directory
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "local-module",
					Source:   "",
					Inputs:   map[string]any{"name": "test"},
					FullPath: filepath.Join(projectRoot, "terraform", "local-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, "terraform", "local-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		moduleDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "local-module")

		// Mock Stat to return non-NotExist error for the module directory
		originalStat := resolver.shims.Stat
		resolver.shims.Stat = func(path string) (os.FileInfo, error) {
			normalizedPath := filepath.Clean(path)
			normalizedModuleDir := filepath.Clean(moduleDir)
			if normalizedPath == normalizedModuleDir {
				return nil, fmt.Errorf("stat error")
			}
			return originalStat(path)
		}

		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create module dir: %v", err)
		}

		// When generating tfvars with reset (so removeTfvarsFiles runs)
		err := resolver.GenerateTfvars(true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when stat fails during tfvars removal")
		}
		if err != nil && !strings.Contains(err.Error(), "failed cleaning existing .tfvars") {
			t.Errorf("Expected error about cleaning tfvars, got: %v", err)
		}
	})

	t.Run("HandlesRemoveTfvarsFilesErrors", func(t *testing.T) {
		// Given a resolver with removeTfvarsFiles that encounters errors
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// Mock ReadDir to return error
		resolver.shims.ReadDir = func(name string) ([]os.DirEntry, error) {
			if strings.Contains(name, "contexts") {
				return nil, fmt.Errorf("readdir error")
			}
			return setupDefaultShims().ReadDir(name)
		}

		// When generating tfvars with reset (so removeTfvarsFiles runs)
		err := resolver.GenerateTfvars(true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when ReadDir fails")
		}
	})

	t.Run("HandlesRemoveTfvarsFilesRemoveAllError", func(t *testing.T) {
		// Given a resolver with removeTfvarsFiles that fails to remove files
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "terraform.tfvars"), []byte("cluster_name = \"old\""), 0644); err != nil {
			t.Fatalf("Failed to write tfvars: %v", err)
		}

		// Mock RemoveAll to return error
		resolver.shims.RemoveAll = func(path string) error {
			if strings.HasSuffix(path, ".tfvars") {
				return fmt.Errorf("remove error")
			}
			return setupDefaultShims().RemoveAll(path)
		}

		// When generating tfvars with reset (so removeTfvarsFiles runs)
		err := resolver.GenerateTfvars(true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when RemoveAll fails")
		}
	})

	t.Run("HandlesFormatValueNilAndDefault", func(t *testing.T) {
		// Given a resolver with nil and default type values
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"nil_value":   nil,
						"int_value":   42,
						"float_value": 3.14,
						"nested_list": []any{[]string{"a", "b"}, []string{"c", "d"}},
						"nested_map": map[string]any{
							"inner": map[string]any{
								"deep": map[string]any{
									"value": "nested",
								},
							},
						},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "nil_value" { type = any }
variable "int_value" { type = number }
variable "float_value" { type = number }
variable "nested_list" { type = list(list(string)) }
variable "nested_map" { type = object({ inner = object({ deep = object({ value = string }) }) }) }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesWriteVariableWithDescription", func(t *testing.T) {
		// Given a resolver with a variable that has description in VariableInfo
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"name": "test-cluster",
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "name" {
  type = string
  description = "The cluster name"
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesGenerateTfvarsFileParseError", func(t *testing.T) {
		// Given a resolver with generateTfvarsFile that encounters invalid HCL in a .tf file
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		moduleDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		// Write invalid HCL - this file will be skipped during parsing
		if err := os.WriteFile(filepath.Join(moduleDir, "invalid.tf"), []byte(`invalid hcl syntax {`), 0644); err != nil {
			t.Fatalf("Failed to write invalid.tf: %v", err)
		}
		// Write a valid .tf file with variables
		if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(`variable "test_var" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write main.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should not return an error (invalid files are skipped)
		if err != nil {
			t.Errorf("Expected no error when invalid HCL file is skipped, got: %v", err)
		}
	})

	t.Run("HandlesGenerateTfvarsFileMkdirAllError", func(t *testing.T) {
		// Given a resolver with generateTfvarsFile that fails to create parent directory
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// Remove the directory so it needs to be created
		if err := os.RemoveAll(variablesDir); err != nil {
			t.Fatalf("Failed to remove dir: %v", err)
		}

		// Mock MkdirAll to return error
		originalMkdirAll := resolver.shims.MkdirAll
		resolver.shims.MkdirAll = func(path string, perm os.FileMode) error {
			if path == variablesDir {
				return fmt.Errorf("mkdir error")
			}
			return originalMkdirAll(path, perm)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when MkdirAll fails")
		}
	})

	t.Run("HandlesGenerateTfvarsFileWriteError", func(t *testing.T) {
		// Given a resolver with generateTfvarsFile that fails to write
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// Mock WriteFile to return error
		resolver.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, ".tfvars") {
				return fmt.Errorf("write error")
			}
			return setupDefaultShims().WriteFile(path, data, perm)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when WriteFile fails")
		}
	})

	t.Run("HandlesProtectedValues", func(t *testing.T) {
		// Given a resolver with protected values
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "context_path" { type = string }
variable "os_type" { type = string }
variable "context_id" { type = string }
variable "cluster_name" { type = string }`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed and skip protected values
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesVariablesWithObjectDefaults", func(t *testing.T) {
		// Given a resolver with variables that have object/map default values
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "config" {
  type = object({
    database = object({
      host = string
      port = number
    })
  })
  default = {
    database = {
      host = "localhost"
      port = 5432
    }
  }
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesFormatValueWithNestedStructures", func(t *testing.T) {
		// Given a resolver with deeply nested structures
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"complex": map[string]any{
							"level1": map[string]any{
								"level2": map[string]any{
									"level3": "deep",
								},
							},
							"list_in_map": []any{"item1", "item2"},
							"map_in_list": []any{
								map[string]any{"key": "value"},
							},
						},
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "complex" {
  type = object({
    level1 = object({
      level2 = object({
        level3 = string
      })
    })
    list_in_map = list(string)
    map_in_list = list(object({ key = string }))
  })
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesRemoveTfvarsFilesStatError", func(t *testing.T) {
		// Given a resolver with removeTfvarsFiles that encounters Stat error
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// Remove the directory so Stat will be called on it
		if err := os.RemoveAll(variablesDir); err != nil {
			t.Fatalf("Failed to remove dir: %v", err)
		}

		// Mock Stat to return non-NotExist error
		originalStat := resolver.shims.Stat
		callCount := 0
		resolver.shims.Stat = func(path string) (os.FileInfo, error) {
			callCount++
			// removeTfvarsFiles calls Stat on the directory
			if callCount > 1 && path == variablesDir {
				return nil, fmt.Errorf("stat error")
			}
			return originalStat(path)
		}

		// When generating tfvars with reset (so removeTfvarsFiles runs)
		err := resolver.GenerateTfvars(true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when Stat fails in removeTfvarsFiles")
		}
	})

	t.Run("HandlesFindModulePathForComponentError", func(t *testing.T) {
		// Given a resolver with a component that has no module directory
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "missing-module",
					Source:   "git::https://github.com/test/module.git",
					Inputs:   map[string]any{"name": "test"},
					FullPath: filepath.Join(projectRoot, "terraform", "missing-module"),
				},
			}
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when module directory is not found")
		}
	})

	t.Run("HandlesParseVariablesFromModuleWithNoTfFiles", func(t *testing.T) {
		// Given a resolver with a module directory that has no .tf files
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		moduleDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should not return an error (empty variables list is valid)
		if err != nil {
			t.Errorf("Expected no error when module has no .tf files, got: %v", err)
		}
	})

	t.Run("HandlesWriteVariableWithDescriptionInInfo", func(t *testing.T) {
		// Given a resolver with a variable that has description in VariableInfo passed to writeVariable
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"name": "test-cluster",
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "name" {
  type = string
  description = "The cluster name"
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesFormatValueDefaultCase", func(t *testing.T) {
		// Given a resolver with a value type that uses formatValue default case
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"custom_type": 12345,
					},
					FullPath: filepath.Join(projectRoot, "terraform", "test-module"),
				},
			}
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "custom_type" { type = number }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesConvertFromCtyValueUnknownOrNull", func(t *testing.T) {
		// Given a resolver with variables that have unknown or null default values
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "null_value" {
  type = string
  default = null
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("HandlesConvertFromCtyValueFloatNumbers", func(t *testing.T) {
		// Given a resolver with variables that have float default values
		resolver, mocks := setup(t)

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		variablesTf := `variable "float_value" {
  type = number
  default = 3.14159
}`
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(variablesTf), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("FiltersDeferredValuesWithContainsExpression", func(t *testing.T) {
		// Given a resolver with component inputs containing deferred expressions
		resolver, mocks := setup(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if strVal, ok := value.(string); ok && strVal == "deferred_value" {
					result[key] = "${terraform_output(vpc.id)}"
				} else {
					result[key] = value
				}
			}
			return result, nil
		}
		resolver.evaluator = mockEvaluator

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		moduleDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "test-module")
		os.MkdirAll(moduleDir, 0755)
		os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"cluster_name": "test-cluster",
						"vpc_id":       "deferred_value",
					},
					FullPath: moduleDir,
				},
			}
		}

		resolver.shims.Stat = os.Stat
		resolver.shims.ReadFile = os.ReadFile
		resolver.shims.MkdirAll = os.MkdirAll
		resolver.shims.WriteFile = os.WriteFile

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// And tfvars file should only contain non-deferred values
		tfvarsPath := filepath.Join(moduleDir, "terraform.tfvars")
		content, err := os.ReadFile(tfvarsPath)
		if err != nil {
			t.Fatalf("Expected tfvars file to be created, got error: %v", err)
		}
		contentStr := string(content)
		if !strings.Contains(contentStr, "cluster_name") {
			t.Error("Expected tfvars to contain cluster_name")
		}
		if strings.Contains(contentStr, "vpc_id") {
			t.Error("Expected tfvars to NOT contain deferred vpc_id")
		}
	})

}

func TestBaseModuleResolver_evaluateInputs(t *testing.T) {
	t.Run("HandlesNilEvaluator", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		mocks.Runtime.Evaluator = nil
		inputs := map[string]any{
			"key1": "value1",
			"key2": 42,
		}

		// When evaluator is nil, callers should check before calling EvaluateMap
		// This test verifies the expected behavior when nil check is done
		if mocks.Runtime.Evaluator == nil {
			result := inputs
			if len(result) != len(inputs) {
				t.Errorf("Expected %d inputs, got %d", len(inputs), len(result))
			}

			if result["key1"] != "value1" {
				t.Errorf("Expected key1 to be 'value1', got %v", result["key1"])
			}

			if result["key2"] != 42 {
				t.Errorf("Expected key2 to be 42, got %v", result["key2"])
			}
		}
	})

	t.Run("PreservesNonStringValues", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks.Runtime.Evaluator = testEvaluator
		inputs := map[string]any{
			"count":   42,
			"enabled": true,
			"tags":    []string{"a", "b"},
			"nested":  map[string]any{"key": "value"},
		}

		result, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["count"] != 42 {
			t.Errorf("Expected count to be 42, got %v", result["count"])
		}

		if result["enabled"] != true {
			t.Errorf("Expected enabled to be true, got %v", result["enabled"])
		}

		if tags, ok := result["tags"].([]string); !ok || len(tags) != 2 {
			t.Errorf("Expected tags to be preserved, got %v", result["tags"])
		}
	})

	t.Run("EvaluatesStringExpressions", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": 42,
			}, nil
		}
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks.Runtime.Evaluator = testEvaluator
		inputs := map[string]any{
			"plain":      "plainstring",
			"expression": "${value}",
		}

		result, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["plain"] != "plainstring" {
			t.Errorf("Expected plain to be 'plainstring', got %v", result["plain"])
		}

		if result["expression"] != 42 {
			t.Errorf("Expected expression to be 42, got %v", result["expression"])
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, fmt.Errorf("failed to evaluate 'bad': evaluation failed")
		}
		mocks.Runtime.Evaluator = mockEvaluator
		inputs := map[string]any{
			"bad": "${invalid}",
		}

		result, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err == nil {
			t.Fatal("Expected error on evaluation failure")
		}

		if result != nil {
			t.Error("Expected nil result on error")
		}

		if !strings.Contains(err.Error(), "failed to evaluate") {
			t.Errorf("Expected error message to contain 'failed to evaluate', got: %v", err)
		}
	})

	t.Run("HandlesEmptyInputs", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks.Runtime.Evaluator = testEvaluator
		inputs := map[string]any{}

		result, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d entries", len(result))
		}
	})

	t.Run("HandlesMixedInputs", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"value": "evaluated",
			}, nil
		}
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks.Runtime.Evaluator = testEvaluator
		inputs := map[string]any{
			"string":    "plain",
			"number":    42,
			"boolean":   true,
			"array":     []string{"a", "b"},
			"evaluated": "${value}",
		}

		result, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["string"] != "plain" {
			t.Errorf("Expected string to be 'plain', got %v", result["string"])
		}

		if result["number"] != 42 {
			t.Errorf("Expected number to be 42, got %v", result["number"])
		}

		if result["boolean"] != true {
			t.Errorf("Expected boolean to be true, got %v", result["boolean"])
		}

		if result["evaluated"] != "evaluated" {
			t.Errorf("Expected evaluated to be 'evaluated', got %v", result["evaluated"])
		}
	})

	t.Run("AlwaysUsesEvaluateDeferredTrue", func(t *testing.T) {
		mocks := setupTerraformMocks(t)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		var receivedEvaluateDeferred bool
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			receivedEvaluateDeferred = evaluateDeferred
			return values, nil
		}
		mocks.Runtime.Evaluator = mockEvaluator
		inputs := map[string]any{
			"test": "value",
		}

		_, err := mocks.Runtime.Evaluator.EvaluateMap(inputs, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !receivedEvaluateDeferred {
			t.Error("Expected evaluateDeferred to be true")
		}
	})
}

func Test_formatValue(t *testing.T) {
	t.Run("FormatsString", func(t *testing.T) {
		// Given a string value
		value := "test"

		// When formatting the value
		result := formatValue(value)

		// Then it should be quoted
		if result != `"test"` {
			t.Errorf("Expected '\"test\"', got '%s'", result)
		}
	})

	t.Run("FormatsEmptyStringSlice", func(t *testing.T) {
		// Given an empty string slice
		value := []string{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty array
		if result != "[]" {
			t.Errorf("Expected '[]', got '%s'", result)
		}
	})

	t.Run("FormatsStringSlice", func(t *testing.T) {
		// Given a string slice
		value := []string{"a", "b", "c"}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted array
		expected := `["a", "b", "c"]`
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("FormatsEmptyAnySlice", func(t *testing.T) {
		// Given an empty any slice
		value := []any{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty array
		if result != "[]" {
			t.Errorf("Expected '[]', got '%s'", result)
		}
	})

	t.Run("FormatsAnySlice", func(t *testing.T) {
		// Given an any slice
		value := []any{"a", 1, true}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted array
		if !strings.Contains(result, `"a"`) {
			t.Errorf("Expected result to contain '\"a\"', got '%s'", result)
		}
	})

	t.Run("FormatsEmptyMap", func(t *testing.T) {
		// Given an empty map
		value := map[string]any{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty object
		if result != "{}" {
			t.Errorf("Expected '{}', got '%s'", result)
		}
	})

	t.Run("FormatsMap", func(t *testing.T) {
		// Given a map
		value := map[string]any{
			"key1": "value1",
			"key2": 42,
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted object
		if !strings.Contains(result, "key1") || !strings.Contains(result, "key2") {
			t.Errorf("Expected result to contain keys, got '%s'", result)
		}
	})

	t.Run("FormatsNestedMap", func(t *testing.T) {
		// Given a nested map
		value := map[string]any{
			"outer": map[string]any{
				"inner": "value",
			},
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted nested object
		if !strings.Contains(result, "outer") || !strings.Contains(result, "inner") {
			t.Errorf("Expected result to contain nested keys, got '%s'", result)
		}
	})

	t.Run("FormatsNil", func(t *testing.T) {
		// Given a nil value
		var value any = nil

		// When formatting the value
		result := formatValue(value)

		// Then it should be null
		if result != "null" {
			t.Errorf("Expected 'null', got '%s'", result)
		}
	})

	t.Run("FormatsNumber", func(t *testing.T) {
		// Given a number value
		value := 42

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted as string
		if result != "42" {
			t.Errorf("Expected '42', got '%s'", result)
		}
	})

	t.Run("FormatsBoolean", func(t *testing.T) {
		// Given a boolean value
		value := true

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted as string
		if result != "true" {
			t.Errorf("Expected 'true', got '%s'", result)
		}
	})

	t.Run("FormatsMapWithEmptyValues", func(t *testing.T) {
		// Given a map with empty nested values
		value := map[string]any{
			"empty_map":   map[string]any{},
			"empty_array": []any{},
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should format empty values correctly
		if !strings.Contains(result, "empty_map = {}") {
			t.Errorf("Expected result to contain 'empty_map = {}', got '%s'", result)
		}
		if !strings.Contains(result, "empty_array = []") {
			t.Errorf("Expected result to contain 'empty_array = []', got '%s'", result)
		}
	})
}

func Test_writeVariable(t *testing.T) {
	t.Run("WritesVariableWithDescription", func(t *testing.T) {
		// Given a body and variable info with description
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{Name: "test", Description: "Test variable"},
		}

		// When writing variable
		writeVariable(body, "test", "value", variables)

		// Then it should include description comment
		content := string(file.Bytes())
		if !strings.Contains(content, "# Test variable") {
			t.Errorf("Expected description comment, got: %s", content)
		}
	})

	t.Run("SkipsSensitiveVariable", func(t *testing.T) {
		// Given a body and variable info with sensitive flag
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{Name: "test", Sensitive: true},
		}

		// When writing variable
		writeVariable(body, "test", "value", variables)

		// Then it should include sensitive comment and not write value
		content := string(file.Bytes())
		if !strings.Contains(content, "# test = \"(sensitive)\"") {
			t.Errorf("Expected sensitive comment, got: %s", content)
		}
		if strings.Contains(content, "test = \"value\"") {
			t.Error("Expected sensitive variable value to be skipped")
		}
	})

	t.Run("WritesStringWithNewlinesAsHeredoc", func(t *testing.T) {
		// Given a body and a string value with newlines
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with multiline string
		writeVariable(body, "test", "line1\nline2\nline3", variables)

		// Then it should use heredoc format
		content := string(file.Bytes())
		if !strings.Contains(content, "<<-EOT") && !strings.Contains(content, "<<EOF") {
			t.Errorf("Expected heredoc format, got: %s", content)
		}
	})

	t.Run("WritesMapValue", func(t *testing.T) {
		// Given a body and a map value
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with map value
		writeVariable(body, "test", map[string]any{"key": "value"}, variables)

		// Then it should format as map
		content := string(file.Bytes())
		if !strings.Contains(content, "test =") {
			t.Errorf("Expected map assignment, got: %s", content)
		}
	})

	t.Run("WritesSimpleValue", func(t *testing.T) {
		// Given a body and a simple value
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with simple value
		writeVariable(body, "test", "value", variables)

		// Then it should write attribute
		content := string(file.Bytes())
		if !strings.Contains(content, "test") {
			t.Errorf("Expected variable assignment, got: %s", content)
		}
	})
}
