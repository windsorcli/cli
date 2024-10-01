package helpers

import (
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

// Helper function to set up the test environment
func setupTestEnv(t *testing.T, backend string) (string, func(), *TerraformHelper) {
	tempDir := t.TempDir()
	projectPath := filepath.Join(tempDir, "terraform/project")

	// Create the necessary directory structure
	err := os.MkdirAll(projectPath, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create project directories: %v", err)
	}

	// Mock getwd to return the project path
	originalGetwd := getwd
	getwd = func() (string, error) {
		return projectPath, nil
	}

	// Mock glob to return a valid result for findRelativeTerraformProjectPath
	originalGlob := glob
	glob = func(pattern string) ([]string, error) {
		if strings.Contains(pattern, "*.tf") {
			return []string{filepath.Join(projectPath, "main.tf")}, nil
		}
		return nil, fmt.Errorf("error globbing files")
	}

	// Mock config handler to return a context configuration with the specified backend
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
		if key == "context" {
			return "local", nil
		}
		return "", fmt.Errorf("unexpected key: %s", key)
	}
	mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
		if key == "contexts.local" {
			return map[string]interface{}{"backend": backend}, nil
		}
		return nil, fmt.Errorf("unexpected key: %s", key)
	}

	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return "/mock/config/root", nil
		},
	}
	mockShell := &shell.MockShell{}
	terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	// Cleanup function to restore original functions
	cleanup := func() {
		getwd = originalGetwd
		glob = originalGlob
	}

	return projectPath, cleanup, terraformHelper
}

// TestTerraformHelper_GetEnvVars tests the GetEnvVars method
func TestTerraformHelper_GetEnvVars(t *testing.T) {
	// Mock dependencies
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

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		_, err := helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
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

		// Mock glob to return an error
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
		}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GetEnvVars is called
		_, err := helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
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

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		if _, err := os.Stat(backendOverridePath); os.IsNotExist(err) {
			t.Fatalf("expected backend_override.tf file to be created")
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("NoTerraformProjectPath", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "non-terraform/project")

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create a directory without .tf files to simulate an invalid project path
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "non-terraform"))

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that no backend_override.tf file was created
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		if _, err := os.Stat(backendOverridePath); err == nil {
			t.Fatalf("expected no backend_override.tf file to be created")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
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

		// Mock context to return an error when getting the config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Fatalf("expected error message to contain 'error getting config root', got %v", err)
		}
	})

	t.Run("ErrorWritingBackendOverrideTf", func(t *testing.T) {
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

		// Mock writeFile to return an error
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend override tf")
		}
		defer func() { writeFile = originalWriteFile }()

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

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing backend_override.tf") {
			t.Fatalf("expected error message to contain 'error writing backend_override.tf', got %v", err)
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

		// Mock glob to return an error
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Fatalf("expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("ErrorGettingBackend", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
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

		// Mock config handler to return an error when getting the backend configuration
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			if key == "contexts.local" {
				return nil, fmt.Errorf("mock error getting backend configuration")
			}
			return nil, fmt.Errorf("unexpected key: %s", key)
		}

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting backend") {
			t.Fatalf("expected error message to contain 'error getting backend', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return the project path
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

		// Mock config handler to return an error when getting the context
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "", fmt.Errorf("mock error retrieving context")
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving context, defaulting to 'local'") {
			t.Fatalf("expected error message to contain 'error retrieving context, defaulting to 'local'', got %v", err)
		}
	})

	t.Run("BackendNotInConfig", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Create the necessary directory structure
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Mock getwd to return the project path
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

		// Mock config handler to return a context configuration without a backend
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			if key == "contexts.local" {
				return map[string]interface{}{}, nil
			}
			return nil, fmt.Errorf("unexpected key: %s", key)
		}

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then no error should be returned and the backend should default to "local"
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("BackendLocal", func(t *testing.T) {
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "local")
		defer cleanup()

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created with the correct content
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		content, err := os.ReadFile(backendOverridePath)
		if err != nil {
			t.Fatalf("failed to read backend_override.tf: %v", err)
		}

		expectedContent := fmt.Sprintf(`
terraform {
  backend "local" {
    path = "%s"
  }
}`, filepath.Join("/mock/config/root", ".tfstate", "project", "terraform.tfstate"))

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("BackendS3", func(t *testing.T) {
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "s3")
		defer cleanup()

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created with the correct content
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		content, err := os.ReadFile(backendOverridePath)
		if err != nil {
			t.Fatalf("failed to read backend_override.tf: %v", err)
		}

		expectedContent := fmt.Sprintf(`
terraform {
  backend "s3" {
    key = "%s"
  }
}`, filepath.Join("project", "terraform.tfstate"))

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("BackendKubernetes", func(t *testing.T) {
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "kubernetes")
		defer cleanup()

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created with the correct content
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		content, err := os.ReadFile(backendOverridePath)
		if err != nil {
			t.Fatalf("failed to read backend_override.tf: %v", err)
		}

		expectedContent := fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix = "%s"
  }
}`, "project")

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		_, cleanup, terraformHelper := setupTestEnv(t, "unsupported")
		defer cleanup()

		// When calling PostEnvExec
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedErrorMsg := "unsupported backend: unsupported"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain '%s', got %v", expectedErrorMsg, err)
		}
	})
}
