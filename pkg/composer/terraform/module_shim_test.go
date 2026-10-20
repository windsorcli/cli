package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseModuleResolver_clearShimDirTfFiles(t *testing.T) {
	setup := func(t *testing.T) (*BaseModuleResolver, *TerraformTestMocks) {
		t.Helper()
		mocks := setupTerraformMocks(t)
		resolver := NewBaseModuleResolver(mocks.Runtime, mocks.BlueprintHandler)
		return resolver, mocks
	}

	t.Run("RemovesAllTfFilesFromShimDir", func(t *testing.T) {
		resolver, _ := setup(t)
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "shim")
		if err := resolver.shims.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("Failed to create shim directory: %v", err)
		}
		if err := resolver.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte("old"), 0644); err != nil {
			t.Fatalf("Failed to write main.tf: %v", err)
		}
		if err := resolver.shims.WriteFile(filepath.Join(moduleDir, "output.tf"), []byte("stale"), 0644); err != nil {
			t.Fatalf("Failed to write output.tf: %v", err)
		}
		if err := resolver.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), []byte("stale"), 0644); err != nil {
			t.Fatalf("Failed to write outputs.tf: %v", err)
		}
		if err := resolver.shims.WriteFile(filepath.Join(moduleDir, "keep.txt"), []byte("keep"), 0644); err != nil {
			t.Fatalf("Failed to write keep.txt: %v", err)
		}

		err := resolver.clearShimDirTfFiles(moduleDir)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if _, err := resolver.shims.Stat(filepath.Join(moduleDir, "main.tf")); err == nil {
			t.Error("Expected main.tf to be removed")
		}
		if _, err := resolver.shims.Stat(filepath.Join(moduleDir, "output.tf")); err == nil {
			t.Error("Expected output.tf to be removed")
		}
		if _, err := resolver.shims.Stat(filepath.Join(moduleDir, "outputs.tf")); err == nil {
			t.Error("Expected outputs.tf to be removed")
		}
		keepContent, err := resolver.shims.ReadFile(filepath.Join(moduleDir, "keep.txt"))
		if err != nil || string(keepContent) != "keep" {
			t.Error("Expected keep.txt to remain unchanged")
		}
	})

	t.Run("SucceedsWhenDirDoesNotExist", func(t *testing.T) {
		resolver, _ := setup(t)
		err := resolver.clearShimDirTfFiles(filepath.Join(t.TempDir(), "nonexistent"))
		if err != nil {
			t.Errorf("Expected nil when dir does not exist, got: %v", err)
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

		// Then no error should be returned (missing source outputs.tf writes empty shim file)
		if err != nil {
			t.Errorf("Expected no error for missing outputs.tf, got: %v", err)
		}

		// And an empty outputs.tf file should be created (overwrites any stale outputs.tf)
		shimOutputsPath := filepath.Join(moduleDir, "outputs.tf")
		content, err := resolver.shims.ReadFile(shimOutputsPath)
		if err != nil {
			t.Errorf("Expected outputs.tf to be created when source has none (empty file), got: %v", err)
		}
		if len(content) != 0 {
			t.Errorf("Expected empty outputs.tf when source has none, got %d bytes", len(content))
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
