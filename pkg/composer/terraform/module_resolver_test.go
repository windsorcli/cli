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

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector         di.Injector
	Shell            *shell.MockShell
	ConfigHandler    config.ConfigHandler
	BlueprintHandler *blueprint.MockBlueprintHandler
	Shims            *Shims
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
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

	injector := di.NewInjector()

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
	injector.Register("shell", mockShell)

	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		configHandler = config.NewConfigHandler(injector)
	}
	injector.Register("configHandler", configHandler)

	if err := configHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
	}
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
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := configHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
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
	injector.Register("blueprintHandler", mockBlueprintHandler)

	// Mock artifact builder for OCI resolver tests
	mockArtifactBuilder := artifact.NewMockArtifact()
	mockArtifactBuilder.PullFunc = func(refs []string) (map[string][]byte, error) {
		artifacts := make(map[string][]byte)
		for _, ref := range refs {
			// Convert OCI ref to cache key format expected by extractOCIModule
			if strings.HasPrefix(ref, "oci://") {
				cacheKey := strings.TrimPrefix(ref, "oci://")
				artifacts[cacheKey] = []byte("mock artifact data")
			} else {
				artifacts[ref] = []byte("mock artifact data")
			}
		}
		return artifacts, nil
	}
	injector.Register("artifactBuilder", mockArtifactBuilder)

	shims := setupShims(t)

	return &Mocks{
		Injector:         injector,
		Shell:            mockShell,
		ConfigHandler:    configHandler,
		BlueprintHandler: mockBlueprintHandler,
		Shims:            shims,
	}
}

// setupShims configures safe default implementations for all shims operations
// This eliminates the need for repetitive mocking in individual test cases
func setupShims(t *testing.T) *Shims {
	t.Helper()

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
		return &RealTarReader{reader: tar.NewReader(r)}
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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseModuleResolver_NewBaseModuleResolver(t *testing.T) {
	t.Run("CreatesResolverWithInjector", func(t *testing.T) {
		// Given an injector
		mocks := setupMocks(t)

		// When creating a new base module resolver
		resolver := NewBaseModuleResolver(mocks.Injector)

		// Then the resolver should be properly initialized
		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}

		// And basic fields should be set
		if resolver.injector == nil {
			t.Error("Expected injector to be set")
		}

		// And shims should be initialized
		if resolver.shims == nil {
			t.Error("Expected shims to be initialized")
		}

		// And dependency fields should be nil until Initialize() is called
		if resolver.shell != nil {
			t.Error("Expected shell to be nil before Initialize()")
		}
		if resolver.blueprintHandler != nil {
			t.Error("Expected blueprintHandler to be nil before Initialize()")
		}
	})
}

func TestBaseModuleResolver_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		resolver := NewBaseModuleResolver(mocks.Injector)
		return resolver, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a resolver
		resolver, _ := setup(t)

		// When calling Initialize
		err := resolver.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// And dependencies should be injected
		if resolver.shell == nil {
			t.Error("Expected shell to be set after Initialize()")
		}
	})

	t.Run("HandlesMissingShellDependency", func(t *testing.T) {
		// Given a resolver with injector that doesn't have shell
		injector := di.NewInjector()
		resolver := NewBaseModuleResolver(injector)

		// When calling Initialize
		err := resolver.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve shell") {
			t.Errorf("Expected error about shell resolution, got: %v", err)
		}
	})
}

func TestBaseModuleResolver_writeShimMainTf(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		resolver := NewBaseModuleResolver(mocks.Injector)
		err := resolver.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize resolver: %v", err)
		}
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
	setup := func(t *testing.T) (*BaseModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		resolver := NewBaseModuleResolver(mocks.Injector)
		err := resolver.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize resolver: %v", err)
		}
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

		// And a temporary directory structure without variables.tf
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

		// Then no error should be returned (missing variables.tf is not an error)
		if err != nil {
			t.Errorf("Expected no error for missing variables.tf, got: %v", err)
		}

		// And an empty variables.tf file should be created
		shimVariablesPath := filepath.Join(moduleDir, "variables.tf")
		info, statErr := resolver.shims.Stat(shimVariablesPath)
		if statErr != nil {
			t.Errorf("Expected variables.tf to be created, got error: %v", statErr)
		}
		if info != nil && info.Size() != 0 {
			t.Errorf("Expected variables.tf to be empty, got size: %d", info.Size())
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

		// And an invalid variables.tf file
		variablesContent := `invalid hcl syntax {`
		variablesPath := filepath.Join(modulePath, "variables.tf")
		if err := resolver.shims.WriteFile(variablesPath, []byte(variablesContent), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When writing the shim variables.tf
		err := resolver.writeShimVariablesTf(moduleDir, modulePath, "test-source")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse variables.tf") {
			t.Errorf("Expected error about parsing variables.tf, got: %v", err)
		}
	})
}

func TestBaseModuleResolver_writeShimOutputsTf(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		resolver := NewBaseModuleResolver(mocks.Injector)
		err := resolver.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize resolver: %v", err)
		}
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
	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		resolver := NewBaseModuleResolver(mocks.Injector)
		resolver.blueprintHandler = mocks.BlueprintHandler
		if err := resolver.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		variablesDir := filepath.Join(projectRoot, ".windsor", ".tf_modules", "test-module")
		if err := os.MkdirAll(variablesDir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "cluster_name" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		err := resolver.GenerateTfvars(false)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

