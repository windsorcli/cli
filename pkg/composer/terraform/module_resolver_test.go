package terraform_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
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
	Shims            *terraform.Shims
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
func setupDefaultShims() *terraform.Shims {
	shims := terraform.NewShims()

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

	shims.JsonUnmarshal = func(data []byte, v any) error {
		return nil
	}

	return shims
}

type MockModuleResolverSetupOptions struct {
	ProcessModulesFunc func() error
}

func setupMockModuleResolver(t *testing.T, opts ...*MockModuleResolverSetupOptions) *terraform.MockModuleResolver {
	t.Helper()

	mock := terraform.NewMockModuleResolver()
	if len(opts) > 0 && opts[0] != nil {
		if opts[0].ProcessModulesFunc != nil {
			mock.ProcessModulesFunc = opts[0].ProcessModulesFunc
		}
	}
	return mock
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestBaseModuleResolver_NewBaseModuleResolver(t *testing.T) {
	t.Run("CreatesResolverWithDependencies", func(t *testing.T) {
		// Given mocks
		mocks := setupTerraformMocks(t)

		// When creating a new base module resolver
		resolver := terraform.NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)

		// Then the resolver should be properly initialized
		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}

		// And shims should be initialized
		if resolver.Shims() == nil {
			t.Error("Expected shims to be initialized")
		}

		// And dependency fields should be set
		if resolver.Runtime().Shell == nil {
			t.Error("Expected shell to be set")
		}
		if resolver.BlueprintHandler() == nil {
			t.Error("Expected blueprintHandler to be set")
		}
		if resolver.Runtime().ConfigHandler == nil {
			t.Error("Expected configHandler to be set")
		}
	})

	t.Run("CreatesResolverWithDefaultShims", func(t *testing.T) {
		// Given mocks
		mocks := setupTerraformMocks(t)

		// When creating a resolver
		resolver := terraform.NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)

		// Then the resolver should have default shims
		if resolver.Shims() == nil {
			t.Fatal("Expected shims to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseModuleResolver_GenerateTfvars(t *testing.T) {
	setup := func(t *testing.T) (*terraform.BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := terraform.NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
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

	t.Run("HeaderStatesFullRegenerationAndOverridePath", func(t *testing.T) {
		// cli#3150: the header used to claim "your changes will not be overwritten"
		// while the implementation always fully regenerates the file — a direct
		// contradiction that invited hand-edits the next run silently discarded.
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
		if err := resolver.GenerateTfvars(false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the written header states the file is regenerated and names the
		// actual override path, and never claims edits are preserved
		tfvarsPath := filepath.Join(variablesDir, "terraform.tfvars")
		data, err := os.ReadFile(tfvarsPath)
		if err != nil {
			t.Fatalf("Failed to read generated tfvars: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "regenerated on every run") {
			t.Errorf("expected header to state the file is fully regenerated, got:\n%s", content)
		}
		if !strings.Contains(content, "`inputs:` in blueprint.yaml") {
			t.Errorf("expected header to name the inputs: override path, got:\n%s", content)
		}
		if strings.Contains(content, "will not be overwritten") {
			t.Errorf("expected the old, contradicted claim to be gone, got:\n%s", content)
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
		if err := resolver.GenerateTfvars(false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the heredoc keeps each line of the string
		data, err := os.ReadFile(filepath.Join(variablesDir, "terraform.tfvars"))
		if err != nil {
			t.Fatalf("Failed to read generated tfvars: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "config = <<EOF\nline1\nline2\nline3\nEOF") {
			t.Errorf("Expected heredoc with each line, got:\n%s", content)
		}
	})

	t.Run("KeepsEveryDocumentOfMultiDocumentString", func(t *testing.T) {
		// Given a resolver with a string input that holds two YAML documents
		resolver, mocks := setup(t)

		patches := "machine:\n  install:\n    disk: /dev/sda\n---\napiVersion: v1alpha1\nkind: RegistryMirrorConfig\nname: docker.io"
		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			projectRoot, _ := mocks.Shell.GetProjectRootFunc()
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "test-module",
					Source: "git::https://github.com/test/module.git",
					Inputs: map[string]any{
						"config_patches": patches,
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
		if err := os.WriteFile(filepath.Join(variablesDir, "variables.tf"), []byte(`variable "config_patches" { type = string }`), 0644); err != nil {
			t.Fatalf("Failed to write variables.tf: %v", err)
		}

		// When generating tfvars
		if err := resolver.GenerateTfvars(false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the heredoc holds both documents and the document marker
		data, err := os.ReadFile(filepath.Join(variablesDir, "terraform.tfvars"))
		if err != nil {
			t.Fatalf("Failed to read generated tfvars: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "config_patches = <<EOF\n"+patches+"\nEOF") {
			t.Errorf("Expected heredoc with both documents, got:\n%s", content)
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
		originalReadDir := resolver.Shims().ReadDir
		resolver.Shims().ReadDir = func(path string) ([]os.DirEntry, error) {
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
		originalStat := resolver.Shims().Stat
		resolver.Shims().Stat = func(path string) (os.FileInfo, error) {
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
		resolver.Shims().ReadDir = func(name string) ([]os.DirEntry, error) {
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
		resolver.Shims().RemoveAll = func(path string) error {
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
		originalMkdirAll := resolver.Shims().MkdirAll
		resolver.Shims().MkdirAll = func(path string, perm os.FileMode) error {
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
		resolver.Shims().WriteFile = func(path string, data []byte, perm os.FileMode) error {
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
		originalStat := resolver.Shims().Stat
		callCount := 0
		resolver.Shims().Stat = func(path string) (os.FileInfo, error) {
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

	t.Run("MissingLocalModuleHasRemediation", func(t *testing.T) {
		// Given a local component without a name whose module directory does not exist
		resolver, mocks := setup(t)

		mocks.BlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{Path: "missing-local", Inputs: map[string]any{"name": "test"}},
			}
		}

		// When generating tfvars
		err := resolver.GenerateTfvars(false)

		// Then the error should carry a remediation hint about the component path
		if err == nil {
			t.Fatal("Expected error when local module directory is not found")
		}
		werr := werror.Find(err)
		if werr == nil {
			t.Fatalf("Expected a WindsorError, got: %v", err)
		}
		if !strings.Contains(werr.Message, "module directory not found for component missing-local") {
			t.Errorf("Expected message about missing module directory, got: %q", werr.Message)
		}
		if !strings.Contains(werr.Remediation, "component path") {
			t.Errorf("Expected remediation about the component path, got: %q", werr.Remediation)
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
		resolver.SetEvaluator(mockEvaluator)

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

		resolver.Shims().Stat = os.Stat
		resolver.Shims().ReadFile = os.ReadFile
		resolver.Shims().MkdirAll = os.MkdirAll
		resolver.Shims().WriteFile = os.WriteFile

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
