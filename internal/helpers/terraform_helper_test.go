package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to sort space-separated strings
func sortString(s string) string {
	parts := strings.Split(s, " ")
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func TestTerraformHelper_GenerateTerraformInitBackendFlags(t *testing.T) {
	// Given: a mock context and config handler
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return "/mock/config/root", nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		func(key string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"backend": "local",
			}, nil
		},
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidBackend", func(t *testing.T) {
		// Given: a valid backend
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// When: GenerateTerraformInitBackendFlags is called
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-backend=true -backend-config=path=" + filepath.Join("/mock/config/root/.tfstate/something/terraform.tfstate")
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
		}
	})

	t.Run("InvalidBackend", func(t *testing.T) {
		// Given: a valid project path with .tf files
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// Mock config handler to return an error when getting the backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return nil, errors.New("backend not found")
			},
		)

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)
		_, err = helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "backend not found" {
			t.Fatalf("Expected error 'backend not found', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given: a valid project path with .tf files
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		getwd = func() (string, error) {
			return projectPath, nil
		}

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// Mock context to return an error when getting the config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error getting config root")
			},
		}

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)
		_, err = helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "error getting config root: error getting config root" {
			t.Fatalf("Expected error 'error getting config root: error getting config root', got %v", err)
		}
	})

	t.Run("S3Backend", func(t *testing.T) {
		// Given: a valid project path with .tf files and S3 backend
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		getwd = func() (string, error) {
			return projectPath, nil
		}

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// Mock config handler to return S3 backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return map[string]interface{}{
					"backend": "s3",
				}, nil
			},
		)

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-backend=true -backend-config=key=something/terraform.tfstate"
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
		}
	})

	t.Run("KubernetesBackend", func(t *testing.T) {
		// Create a temporary directory for the mock project
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/subdir")
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}

		// Create a mock Terraform file to ensure the directory is recognized as a Terraform project
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}

		// Mock getwd to return the project path
		getwd = func() (string, error) {
			return projectPath, nil
		}

		// Mock ConfigHandler to return "kubernetes" as the backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			},
			func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context" {
					return map[string]interface{}{
						"backend": "kubernetes",
					}, nil
				}
				return nil, fmt.Errorf("unexpected key: %s", key)
			},
		)

		// Mock Context to return a valid config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
		}

		// Create a new TerraformHelper with the mocked context and config handler
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)

		// Call GenerateTerraformInitBackendFlags
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Ensure no error is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Ensure the flags contain the correct backend configuration for kubernetes
		expectedFlag := "-backend-config=secret_suffix=subdir"
		if !strings.Contains(flags, expectedFlag) {
			t.Errorf("Expected flags to contain %s, got %s", expectedFlag, flags)
		}
	})

	t.Run("BackendConfigFileExists", func(t *testing.T) {
		// Given: a valid project path with .tf files and a backend configuration file
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		configRoot := filepath.Join(tempDir, "mock/config/root")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create mock tfvars file and backend configuration file
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		err = os.MkdirAll(configRoot, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create config root directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(configRoot, "backend.tfvars"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create backend configuration file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// Mock context to return the correct config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return configRoot, nil
			},
		}

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return the correct flags including the backend configuration file
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-backend=true -backend-config=" + filepath.Join(configRoot, "backend.tfvars") + " -backend-config=path=" + filepath.Join(configRoot, ".tfstate/something/terraform.tfstate")
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
		}
	})

	t.Run("UnrecognizedBackend", func(t *testing.T) {
		// Given: a valid project path with .tf files and an unrecognized backend
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// Mock config handler to return an unrecognized backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return map[string]interface{}{
					"backend": "unrecognized",
				}, nil
			},
		)

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an empty string and no error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if flags != "" {
			t.Errorf("Expected empty flags, got %s", flags)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return an error to simulate failure in finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error finding project path") {
			t.Fatalf("Expected error message to contain 'mock error finding project path', got %v", err)
		}
		if flags != "" {
			t.Fatalf("Expected empty flags, got %v", flags)
		}
	})

	t.Run("EmptyProjectPath", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return no matches to simulate no Terraform files found
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return []string{}, nil
		}
		defer func() { glob = originalGlob }()

		// When: GenerateTerraformInitBackendFlags is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an empty string and no error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if flags != "" {
			t.Fatalf("Expected empty flags, got %v", flags)
		}
	})
}

// TestTerraformHelper_GetEnvVars tests the GetEnvVars method
func TestTerraformHelper_GetEnvVars(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}

	t.Run("ValidTfvarsFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configRoot := filepath.Join(tempDir, "mock/config/root")
		projectPath := filepath.Join(tempDir, "terraform/windsor/blueprint")

		// Create the necessary directory structure
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create the necessary directory structure for tfvars files
		tfvarsDir := filepath.Join(configRoot, "terraform/windsor")
		err = os.MkdirAll(tfvarsDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create tfvars directories: %v", err)
		}

		// Create a .tf file in the project directory
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}

		// Create .tfvars files in the config root directory
		err = os.WriteFile(filepath.Join(tfvarsDir, "blueprint.tfvars"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tfvars file: %v", err)
		}
		err = os.WriteFile(filepath.Join(tfvarsDir, "blueprint_generated.tfvars"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create generated .tfvars file: %v", err)
		}

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return configRoot, nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		envVars, err := helper.GetEnvVars()

		// Then: it should return no error and the environment variables should include the var-file arguments
		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.Join(configRoot, ".terraform/windsor/blueprint"),
			"TF_CLI_ARGS_init": fmt.Sprintf("-backend=true -backend-config=path=%s", filepath.Join(configRoot, ".tfstate/windsor/blueprint/terraform.tfstate")),
			"TF_CLI_ARGS_plan": fmt.Sprintf("-out=%s -var-file=%s -var-file=%s",
				filepath.Join(configRoot, ".terraform/windsor/blueprint/terraform.tfplan"),
				filepath.Join(tfvarsDir, "blueprint.tfvars"),
				filepath.Join(tfvarsDir, "blueprint_generated.tfvars")),
			"TF_CLI_ARGS_apply": filepath.Join(configRoot, ".terraform/windsor/blueprint/terraform.tfplan"),
			"TF_CLI_ARGS_import": fmt.Sprintf("-var-file=%s -var-file=%s",
				filepath.Join(tfvarsDir, "blueprint.tfvars"),
				filepath.Join(tfvarsDir, "blueprint_generated.tfvars")),
			"TF_CLI_ARGS_destroy": fmt.Sprintf("-var-file=%s -var-file=%s",
				filepath.Join(tfvarsDir, "blueprint.tfvars"),
				filepath.Join(tfvarsDir, "blueprint_generated.tfvars")),
			"TF_VAR_context_path": configRoot,
		}

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("InvalidProjectPath", func(t *testing.T) {
		tempDir := t.TempDir()
		configRoot := filepath.Join(tempDir, "mock/config/root")
		invalidProjectPath := filepath.Join(tempDir, "invalid/path")

		// Mock getwd to return the invalid project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return invalidProjectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return configRoot, nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		envVars, err := helper.GetEnvVars()

		// Then: it should return no error and empty environment variables
		expectedEnvVars := map[string]string{}

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/windsor/blueprint")

		// Create the necessary directory structure
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create a .tf file in the project directory to simulate a valid Terraform project
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock context to return an error when getting the config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", fmt.Errorf("error getting config root")
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		_, err = helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected error message to contain 'error getting config root', got %v", err)
		}
	})

	t.Run("ErrorGlobbingTfvarsFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configRoot := filepath.Join(tempDir, "mock/config/root")
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return configRoot, nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// Given: a valid project path
		projectPath := filepath.Join(tempDir, "terraform/windsor/blueprint")

		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return a valid result for findRelativeTerraformProjectPath
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(projectPath, "main.tf")}, nil
			}
			return nil, fmt.Errorf("error globbing files")
		}
		defer func() { glob = originalGlob }()

		// When: GetEnvVars is called
		_, err := helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedErrMsg := "error globbing files"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Expected error message to contain %s, got %s", expectedErrMsg, err.Error())
		}
	})

	t.Run("NoTfvarsFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configRoot := filepath.Join(tempDir, "mock/config/root")
		projectPath := filepath.Join(tempDir, "terraform/windsor/blueprint")

		// Create the necessary directory structure
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create a .tf file in the project directory
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return configRoot, nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		envVars, err := helper.GetEnvVars()

		// Then: it should return no error and the environment variables should not include the var-file arguments
		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         filepath.Join(configRoot, ".terraform/windsor/blueprint"),
			"TF_CLI_ARGS_init":    fmt.Sprintf("-backend=true -backend-config=path=%s", filepath.Join(configRoot, ".tfstate/windsor/blueprint/terraform.tfstate")),
			"TF_CLI_ARGS_plan":    fmt.Sprintf("-out=%s", filepath.Join(configRoot, ".terraform/windsor/blueprint/terraform.tfplan")),
			"TF_CLI_ARGS_apply":   filepath.Join(configRoot, ".terraform/windsor/blueprint/terraform.tfplan"),
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": configRoot,
		}

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("NoTerraformDir", func(t *testing.T) {
		// Mock getwd to return a path without a "terraform" directory
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/path/to/project", nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return matches
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return []string{"/path/to/project/main.tf"}, nil
		}
		defer func() { glob = originalGlob }()

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Create TerraformHelper
		terraformHelper := NewTerraformHelper(nil, nil, mockContext)

		// When: GetEnvVars is called
		envVars, err := terraformHelper.GetEnvVars()

		// Then: it should return an error indicating no "terraform" directory found
		expectedError := "no 'terraform' directory found in the current path"
		if err == nil || err.Error() != expectedError {
			t.Errorf("Expected error %q, got %v", expectedError, err)
		}

		// Ensure envVars is empty
		if len(envVars) != 0 {
			t.Errorf("Expected empty envVars, got %v", envVars)
		}
	})
}

// TestTerraformHelper_GetAlias tests the GetAlias method
func TestTerraformHelper_GetAlias(t *testing.T) {
	tests := []struct {
		name          string
		contextValue  string
		contextError  error
		expectedAlias map[string]string
		expectedError bool
	}{
		{
			name:          "LocalContext",
			contextValue:  "local",
			contextError:  nil,
			expectedAlias: map[string]string{"terraform": "tflocal"},
			expectedError: false,
		},
		{
			name:          "NonLocalContext",
			contextValue:  "remote",
			contextError:  nil,
			expectedAlias: map[string]string{"terraform": ""},
			expectedError: false,
		},
		{
			name:          "ErrorRetrievingContext",
			contextValue:  "",
			contextError:  fmt.Errorf("error retrieving context"),
			expectedAlias: nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ConfigHandler to return the specified context value and error
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					if key == "context" {
						return tt.contextValue, tt.contextError
					}
					return "", fmt.Errorf("unexpected key: %s", key)
				},
				nil,
			)

			// Create a new TerraformHelper with the mocked config handler
			helper := NewTerraformHelper(mockConfigHandler, nil, nil)

			// Call GetAlias
			alias, err := helper.GetAlias()

			// Check for expected error
			if (err != nil) != tt.expectedError {
				t.Fatalf("Expected error: %v, got: %v", tt.expectedError, err)
			}

			// Check for expected alias
			if !reflect.DeepEqual(alias, tt.expectedAlias) {
				t.Errorf("Expected alias: %v, got: %v", tt.expectedAlias, alias)
			}
		})
	}
}

func TestTerraformHelper_SetConfig(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	helper := NewTerraformHelper(mockConfigHandler, nil, nil)

	t.Run("SetBackend", func(t *testing.T) {
		// Mock SetConfigValue to return no error
		mockConfigHandler.SetConfigValueFunc = func(key, value string) error {
			if key == "contexts.test-context.terraform.backend" {
				return nil
			}
			return fmt.Errorf("unexpected key: %s", key)
		}

		// Mock GetContext to return "test-context"
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)

		// When: SetConfig is called with "backend" key
		err := helper.SetConfig("backend", "s3")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("UnsupportedKey", func(t *testing.T) {
		// When: SetConfig is called with an unsupported key
		err := helper.SetConfig("unsupported_key", "value")

		// Then: it should return an error
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "unsupported config key: unsupported_key" {
			t.Fatalf("expected error 'unsupported config key: unsupported_key', got %v", err)
		}
	})

	t.Run("ErrorSettingBackend", func(t *testing.T) {
		// Mock SetConfigValue to return an error
		mockConfigHandler.SetConfigValueFunc = func(key, value string) error {
			if key == "contexts.test-context.terraform.backend" {
				return fmt.Errorf("mock error setting backend")
			}
			return nil
		}

		// Mock GetContext to return "test-context"
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)

		// When: SetConfig is called with "backend" key
		err := helper.SetConfig("backend", "s3")

		// Then: it should return an error
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting backend: mock error setting backend" {
			t.Fatalf("expected error 'error setting backend: mock error setting backend', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Mock GetContext to return an error
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", fmt.Errorf("mock error retrieving context")
			},
		}
		helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)

		// When: SetConfig is called with "backend" key
		err := helper.SetConfig("backend", "s3")

		// Then: it should return an error
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: mock error retrieving context" {
			t.Fatalf("expected error 'error retrieving context: mock error retrieving context', got %v", err)
		}
	})
}

func TestTerraformHelper_PostEnvExec(t *testing.T) {
	// Mock dependencies
	mockConfigHandler := &config.MockConfigHandler{}
	mockShell := &shell.MockShell{}
	mockContext := &context.MockContext{}

	t.Run("Success", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock context to return the config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Mock GetCurrentBackend to return "local"
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			if key == "contexts.local" {
				return map[string]interface{}{"backend": "local"}, nil
			}
			return nil, fmt.Errorf("unexpected key: %s", key)
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FallbackToDefaultContext", func(t *testing.T) {
		// Given a TerraformHelper instance with an error retrieving context
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			return "", fmt.Errorf("error retrieving context")
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			if key == "contexts.local" {
				return map[string]interface{}{"backend": "local"}, nil
			}
			return nil, fmt.Errorf("unexpected key: %s", key)
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned and it should fall back to default context
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FallbackToDefaultBackend", func(t *testing.T) {
		// Given a TerraformHelper instance with a missing context configuration
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "nonexistent", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			return nil, fmt.Errorf("key %s not found", key)
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned and it should fall back to default backend
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
