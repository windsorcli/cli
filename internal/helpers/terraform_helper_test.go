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
	"github.com/windsor-hotel/cli/internal/di"
)

// Mock stat to return a specific error
var originalStat = stat

func mockStatWithError(err error) {
	stat = func(name string) (os.FileInfo, error) {
		return nil, err
	}
}

func restoreStat() {
	stat = originalStat
}

// Helper function to sort space-separated strings
func sortString(s string) string {
	parts := strings.Split(s, " ")
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// Helper function to set up the test environment
func setupTestEnv(t *testing.T, backend string, tfvarsFiles map[string]string) (string, func(), *TerraformHelper) {
	tempDir := t.TempDir()
	projectPath := filepath.Join(tempDir, "terraform/windsor")
	configRoot := filepath.Join(tempDir, "contexts/local")

	// Create the necessary directory structure
	err := mkdirAll(projectPath, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create project directories: %v", err)
	}

	// Create the necessary directory structure for tfvars files
	for path := range tfvarsFiles {
		dir := filepath.Dir(filepath.Join(tempDir, path))
		err = mkdirAll(dir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create tfvars directories: %v", err)
		}
	}

	// Create .tfvars and .tfvars.json files in the config root directory
	for path, content := range tfvarsFiles {
		err = os.WriteFile(filepath.Join(tempDir, path), []byte(content), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create tfvars file: %v", err)
		}
	}

	// Mock getwd to return the terraform project directory
	originalGetwd := getwd
	getwd = func() (string, error) {
		return projectPath, nil
	}
	t.Cleanup(func() { getwd = originalGetwd })

	// Mock glob to return a valid result for findRelativeTerraformProjectPath
	originalGlob := glob
	glob = func(pattern string) ([]string, error) {
		if strings.Contains(pattern, "*.tf") {
			return []string{filepath.Join(projectPath, "main.tf")}, nil
		}
		if strings.Contains(pattern, "*.tfvars") {
			var matches []string
			for path := range tfvarsFiles {
				if strings.HasSuffix(path, ".tfvars") || strings.HasSuffix(path, ".tfvars.json") {
					matches = append(matches, filepath.Join(tempDir, path))
				}
			}
			return matches, nil
		}
		return nil, fmt.Errorf("error globbing files")
	}
	t.Cleanup(func() { glob = originalGlob })

	// Mock config handler to return a context configuration with the specified backend
	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
		switch key {
		case "context":
			return "local", nil
		case "contexts.local.terraform.backend":
			return backend, nil
		default:
			return "", fmt.Errorf("unexpected key: %s", key)
		}
	}

	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return configRoot, nil
		},
	}

	// Set up DI container
	diContainer := di.NewContainer()
	diContainer.Register("cliConfigHandler", mockConfigHandler)
	diContainer.Register("context", mockContext)

	terraformHelper, err := NewTerraformHelper(diContainer)
	if err != nil {
		t.Fatalf("Failed to create TerraformHelper: %v", err)
	}

	return projectPath, func() {}, terraformHelper
}

func TestNewTerraformHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container without registering context
		diContainer := di.NewContainer()
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Attempt to create TerraformHelper
		_, err := NewTerraformHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create DI container without registering configHandler
		diContainer := di.NewContainer()

		// Attempt to create TerraformHelper
		_, err := NewTerraformHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ConfigHandlerTypeError", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Register a wrong type for cliConfigHandler
		diContainer.Register("cliConfigHandler", "not a config handler")

		// Register a valid context
		mockContext := &context.MockContext{}
		diContainer.Register("context", mockContext)

		_, err := NewTerraformHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "resolved configHandler is not of type ConfigHandler") {
			t.Fatalf("expected error about configHandler type, got %v", err)
		}
	})

	t.Run("ContextTypeError", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Register a valid config handler
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Register a wrong type for context
		diContainer.Register("context", "not a context interface")

		_, err := NewTerraformHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "resolved context is not of type ContextInterface") {
			t.Fatalf("expected error about context type, got %v", err)
		}
	})
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
		err := mkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create the necessary directory structure for tfvars files
		tfvarsDir := filepath.Join(configRoot, "terraform/windsor")
		err = mkdirAll(tfvarsDir, os.ModePerm)
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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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
		err := mkdirAll(projectPath, os.ModePerm)
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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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
		tfvarsFiles := map[string]string{}
		_, cleanup, helper := setupTestEnv(t, "local", tfvarsFiles)
		defer cleanup()

		// Mock glob to return an error
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("glob error")
		}
		defer func() { glob = originalGlob }()

		// When: GetEnvVars is called
		_, err := helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedErrMsg := "glob error"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Expected error message to contain %s, got %s", expectedErrMsg, err.Error())
		}
	})

	t.Run("NoTfvarsFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configRoot := filepath.Join(tempDir, "mock/config/root")
		projectPath := filepath.Join(tempDir, "terraform/windsor/blueprint")

		// Create the necessary directory structure
		err := mkdirAll(projectPath, os.ModePerm)
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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: GetEnvVars is called
		_, err = helper.GetEnvVars()

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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: GetEnvVars is called
		_, err = helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("ErrorCheckingFile", func(t *testing.T) {
		tfvarsFiles := map[string]string{
			"contexts/local/terraform/windsor/blueprint.tfvars":                "",
			"contexts/local/terraform/windsor/blueprint_generated.tfvars.json": "",
		}
		projectPath, cleanup, helper := setupTestEnv(t, "local", tfvarsFiles)
		defer cleanup()

		// Ensure the file does not exist
		fileToCheck := filepath.Join(projectPath, "non_existent_file.tfvars")
		if _, err := os.Stat(fileToCheck); !os.IsNotExist(err) {
			t.Fatalf("Expected file to not exist, but it does")
		}

		// Mock stat to return an error other than os.ErrNotExist
		mockStatWithError(fmt.Errorf("mock error"))
		defer restoreStat()

		// When: GetEnvVars is called
		_, err := helper.GetEnvVars()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedErrMsg := "error checking file"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Expected error message to contain %s, got %s", expectedErrMsg, err.Error())
		}
	})

	t.Run("TruncateLongStringInKubernetesBackend", func(t *testing.T) {
		tfvarsFiles := map[string]string{}
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "kubernetes", tfvarsFiles)
		defer cleanup()

		// Mock a long project path that exceeds 63 characters
		longProjectPath := filepath.Join(projectPath, "terraform", "supercalifragilisticexpialidocioussupercalifragilisticexpialidocious")
		expected := "supercalifragilisticexpialidocioussupercalifragilisticexpialido"

		// Ensure the long project path exists
		err := mkdirAll(longProjectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create long project path: %v", err)
		}

		// Ensure the project path is set to a long path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return longProjectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock the context and config handler responses
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "kubernetes", nil // Ensure this matches the expected backend
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
			},
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		terraformHelper, err = NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created with the correct content
		backendOverridePath := filepath.Join(longProjectPath, "backend_override.tf")
		content, err := os.ReadFile(backendOverridePath)
		if err != nil {
			t.Fatalf("failed to read backend_override.tf: %v", err)
		}

		expectedContent := fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix = "%s"
  }
}`, expected)

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
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
			mockContext := &context.MockContext{}
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

			// Set up DI container
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create a new TerraformHelper with the mocked config handler
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

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
	mockContext := &context.MockContext{}
	diContainer := di.NewContainer()
	diContainer.Register("cliConfigHandler", mockConfigHandler)
	diContainer.Register("context", mockContext)
	helper, err := NewTerraformHelper(diContainer)
	if err != nil {
		t.Fatalf("Failed to create TerraformHelper: %v", err)
	}

	t.Run("SetBackend", func(t *testing.T) {
		// Mock SetConfigValue to return no error
		mockConfigHandler.SetConfigValueFunc = func(key, value string) error {
			if key == "contexts.test-context.terraform.backend" {
				return nil
			}
			return fmt.Errorf("unexpected key: %s", key)
		}

		// Mock GetContext to return "test-context"
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: SetConfig is called with "backend" key
		err = helper.SetConfig("backend", "s3")

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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: SetConfig is called with "backend" key
		err = helper.SetConfig("backend", "s3")

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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: SetConfig is called with "backend" key
		err = helper.SetConfig("backend", "s3")

		// Then: it should return an error
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: mock error retrieving context" {
			t.Fatalf("expected error 'error retrieving context: mock error retrieving context', got %v", err)
		}
	})

	t.Run("EmptyValue", func(t *testing.T) {
		// Given: a mock config handler, and context
		mockConfigHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return "/path/to/config", nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When: SetConfig is called with an empty value
		err = terraformHelper.SetConfig("backend", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestTerraformHelper_PostEnvExec(t *testing.T) {
	// Mock dependencies
	mockConfigHandler := &config.MockConfigHandler{}
	mockContext := &context.MockContext{}

	t.Run("Success", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/windsor")

		// Create the necessary directory structure
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}

		// Mock getwd global function
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock the context and config handler
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return projectPath, nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "local", nil // Return "local" instead of "kubernetes"
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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

		// Dynamically construct the expected path based on the temporary directory structure
		expectedPath := filepath.Join(projectPath, ".tfstate/windsor/terraform.tfstate")
		expectedContent := fmt.Sprintf(`
terraform {
  backend "local" {
    path = "%s"
  }
}`, expectedPath)

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}
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

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

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

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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
			if key == "contexts.local" {
				return "", fmt.Errorf("mock error getting backend configuration")
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving context, defaulting to 'local'") {
			t.Fatalf("expected error message to contain 'error retrieving context, defaulting to 'local'', got %v", err)
		}
	})

	t.Run("BackendLocal", func(t *testing.T) {
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "local", nil)
		defer cleanup()

		// Mock config handler to return a context configuration with the specified backend
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			if key == "contexts.local.terraform.backend" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return projectPath, nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// Mock the GetConfigRootFunc to return the actual project path
		mockContext.GetConfigRootFunc = func() (string, error) {
			return projectPath, nil
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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

		// Dynamically construct the expected path based on the temporary directory structure
		expectedPath := filepath.Join(projectPath, ".tfstate/windsor/terraform.tfstate")
		expectedContent := fmt.Sprintf(`
terraform {
  backend "local" {
    path = "%s"
  }
}`, expectedPath)

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("BackendS3", func(t *testing.T) {
		tfvarsFiles := map[string]string{}
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "s3", tfvarsFiles)
		defer cleanup()

		// Create the necessary directory structure
		err := mkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}
		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
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

		// Mock config handler to return a context configuration with the specified backend
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			if key == "contexts.local.terraform.backend" {
				return "s3", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return projectPath, nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		terraformHelper, err = NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// Mock the GetConfigRootFunc to return the actual project path
		mockContext.GetConfigRootFunc = func() (string, error) {
			return projectPath, nil
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

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
    key = "windsor/terraform.tfstate"
  }
}`)

		// Normalize path separators to Unix-style
		normalizedContent := strings.ReplaceAll(string(content), "\\", "/")

		if strings.TrimSpace(normalizedContent) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, normalizedContent)
		}
	})

	t.Run("BackendKubernetes", func(t *testing.T) {
		tfvarsFiles := map[string]string{}
		projectPath, cleanup, terraformHelper := setupTestEnv(t, "kubernetes", tfvarsFiles)
		defer cleanup()

		// Mock a long project path that exceeds 63 characters
		longProjectPath := filepath.Join(projectPath, "terraform", "supercalifragilisticexpialidocioussupercalifragilisticexpialidocious")
		expected := "supercalifragilisticexpialidocioussupercalifragilisticexpialido"

		// Ensure the long project path exists
		err := mkdirAll(longProjectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create long project path: %v", err)
		}

		// Ensure the project path is set to a long path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return longProjectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Use the helper function to set up the mock context and config handler
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			if key == "contexts.local.terraform.backend" {
				return "kubernetes", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return longProjectPath, nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		terraformHelper, err = NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// Mock the GetConfigRootFunc to return the actual project path
		mockContext.GetConfigRootFunc = func() (string, error) {
			return longProjectPath, nil
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify that the backend_override.tf file was created with the correct content
		backendOverridePath := filepath.Join(longProjectPath, "backend_override.tf")
		content, err := os.ReadFile(backendOverridePath)
		if err != nil {
			t.Fatalf("failed to read backend_override.tf: %v", err)
		}

		expectedContent := fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix = "%s"
  }
}`, expected)

		if strings.TrimSpace(string(content)) != strings.TrimSpace(expectedContent) {
			t.Errorf("expected %s, got %s", expectedContent, string(content))
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Create the necessary directory structure
		err := mkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
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

		// Use the helper function to set up the mock context and config handler
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "unsupported", nil // Return "local" instead of "kubernetes"
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
			},
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return projectPath, nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// Mock the GetConfigRootFunc to return the actual project path
		mockContext.GetConfigRootFunc = func() (string, error) {
			return projectPath, nil
		}

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedErrorMsg := "unsupported backend: unsupported"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain '%s', got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorWritingBackendOverride", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Create the necessary directory structure
		err := mkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create project directories: %v", err)
		}

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
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

		// Use the helper function to set up the mock context and config handler
		mockConfigHandler := &config.MockConfigHandler{}
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			if key == "contexts.local.terraform.backend" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return projectPath, nil
			},
		}

		// Set up DI container
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("Failed to create TerraformHelper: %v", err)
		}

		// Mock the writeFile function to simulate an error
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}
		defer func() { writeFile = originalWriteFile }()

		// When calling PostEnvExec
		err = terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedErrorMsg := "error writing backend_override.tf: mock error writing file"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain '%s', got %v", expectedErrorMsg, err)
		}
	})
}

func TestTerraformHelper_GetContainerConfig(t *testing.T) {
	// Given a mock context and mock config handler
	mockContext := &context.MockContext{}
	mockConfigHandler := &config.MockConfigHandler{}
	container := di.NewContainer()
	container.Register("context", mockContext)
	container.Register("cliConfigHandler", mockConfigHandler)

	// Create TerraformHelper
	terraformHelper, err := NewTerraformHelper(container)
	if err != nil {
		t.Fatalf("NewTerraformHelper() error = %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		// When: GetContainerConfig is called
		containerConfig, err := terraformHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should be nil as per the stub implementation
		if containerConfig != nil {
			t.Errorf("expected nil, got %v", containerConfig)
		}
	})
}
