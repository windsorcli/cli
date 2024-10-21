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
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string) (string, error) {
		switch key {
		case "context":
			return "local", nil
		case "contexts.local.terraform.backend":
			return backend, nil
		default:
			return "", fmt.Errorf("unexpected key: %s", key)
		}
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return configRoot, nil
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

func TestTerraformHelper(t *testing.T) {
	t.Run("NewTerraformHelper", func(t *testing.T) {
		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Given a DI container without context registered
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new TerraformHelper
			_, err := NewTerraformHelper(diContainer)

			// Then an error should be returned
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})

		t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
			// Given a DI container without cliConfigHandler registered
			diContainer := di.NewContainer()

			// When creating a new TerraformHelper
			_, err := NewTerraformHelper(diContainer)

			// Then an error should be returned
			if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
				t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
			}
		})

		t.Run("ConfigHandlerTypeError", func(t *testing.T) {
			// Given a DI container with an incorrect type for cliConfigHandler
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", "not a config handler")

			// And a valid context
			mockContext := context.NewMockContext()
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			_, err := NewTerraformHelper(diContainer)

			// Then an error about cliConfigHandler type should be returned
			if err == nil || !strings.Contains(err.Error(), "resolved cliConfigHandler is not of type ConfigHandler") {
				t.Fatalf("expected error about cliConfigHandler type, got %v", err)
			}
		})

		t.Run("ContextTypeError", func(t *testing.T) {
			// Given a DI container with a valid config handler
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// And an incorrect type for context
			diContainer.Register("context", "not a context interface")

			// When creating a new TerraformHelper
			_, err := NewTerraformHelper(diContainer)

			// Then an error about context type should be returned
			if err == nil || !strings.Contains(err.Error(), "resolved context is not of type ContextInterface") {
				t.Fatalf("expected error about context type, got %v", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("ValidTfvarsFiles", func(t *testing.T) {
			// Given a valid project and tfvars files
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

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return configRoot, nil
			}
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// When GetEnvVars is called
			envVars, err := helper.GetEnvVars()

			// Then it should return no error and the environment variables should include the var-file arguments
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
			// Given an invalid project path
			tempDir := t.TempDir()
			configRoot := filepath.Join(tempDir, "mock/config/root")
			invalidProjectPath := filepath.Join(tempDir, "invalid/path")

			// Mock getwd to return the invalid project path
			originalGetwd := getwd
			getwd = func() (string, error) {
				return invalidProjectPath, nil
			}
			defer func() { getwd = originalGetwd }()

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return configRoot, nil
			}
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// When GetEnvVars is called
			envVars, err := helper.GetEnvVars()

			// Then it should return no error and empty environment variables
			expectedEnvVars := map[string]string{}

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if !reflect.DeepEqual(envVars, expectedEnvVars) {
				t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
			}
		})

		t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
			// Given a valid project path but an error when getting the config root
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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", fmt.Errorf("error getting config root")
			}
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// When GetEnvVars is called
			_, err = helper.GetEnvVars()

			// Then it should return an error
			if err == nil {
				t.Fatalf("Expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "error getting config root") {
				t.Errorf("Expected error message to contain 'error getting config root', got %v", err)
			}
		})

		t.Run("ErrorGlobbingTfvarsFiles", func(t *testing.T) {
			// Given a valid project path and config root
			tempDir := t.TempDir()
			projectPath := filepath.Join(tempDir, "terraform/windsor")
			configRoot := filepath.Join(tempDir, "contexts/local")

			// Create the necessary directory structure
			err := mkdirAll(projectPath, os.ModePerm)
			if err != nil {
				t.Fatalf("Failed to create project directories: %v", err)
			}

			// Mock getwd to return the terraform project directory
			originalGetwd := getwd
			getwd = func() (string, error) {
				return projectPath, nil
			}
			defer func() { getwd = originalGetwd }()

			// Mock glob to return an error
			originalGlob := glob
			glob = func(pattern string) ([]string, error) {
				return nil, fmt.Errorf("glob error")
			}
			defer func() { glob = originalGlob }()

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "local", nil
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return configRoot, nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			terraformHelper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetEnvVars
			_, err = terraformHelper.GetEnvVars()

			// Then it should return an error
			if err == nil {
				t.Fatalf("Expected an error, got nil")
			}
			expectedErrMsg := "glob error"
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Errorf("Expected error message to contain %s, got %s", expectedErrMsg, err.Error())
			}
		})

		t.Run("NoTfvarsFiles", func(t *testing.T) {
			// Given a valid project path and config root
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

			// Mock context to return the config root
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return configRoot, nil
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetEnvVars
			envVars, err := helper.GetEnvVars()

			// Then it should return no error and the environment variables should not include the var-file arguments
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
			// Given a path without a "terraform" directory
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

			// Mock context to return the config root
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/path/to/config", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			terraformHelper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetEnvVars
			envVars, err := terraformHelper.GetEnvVars()

			// Then it should return an error indicating no "terraform" directory found
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
			// Given an error when getting the current directory
			originalGetwd := getwd
			getwd = func() (string, error) {
				return "", fmt.Errorf("mock error getting current directory")
			}
			defer func() { getwd = originalGetwd }()

			// Mock context to return the config root
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetEnvVars
			_, err = helper.GetEnvVars()

			// Then it should return an error
			if err == nil {
				t.Fatalf("Expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "error getting current directory") {
				t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
			}
		})

		t.Run("ErrorFindingProjectPath", func(t *testing.T) {
			// Given a valid project path
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

			// Mock context to return the config root
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetEnvVars
			_, err = helper.GetEnvVars()

			// Then it should return an error
			if err == nil {
				t.Fatalf("Expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "error finding project path") {
				t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
			}
		})

		t.Run("ErrorCheckingFile", func(t *testing.T) {
			// Given a valid project path and config root
			tempDir := t.TempDir()
			projectPath := filepath.Join(tempDir, "terraform/windsor")
			configRoot := filepath.Join(tempDir, "contexts/local")

			// Create the necessary directory structure
			err := mkdirAll(projectPath, os.ModePerm)
			if err != nil {
				t.Fatalf("Failed to create project directories: %v", err)
			}

			// Create the necessary directory structure for tfvars files
			tfvarsFiles := map[string]string{
				"contexts/local/terraform/windsor/blueprint.tfvars":                "",
				"contexts/local/terraform/windsor/blueprint_generated.tfvars.json": "",
			}
			for path, content := range tfvarsFiles {
				dir := filepath.Dir(filepath.Join(tempDir, path))
				err = mkdirAll(dir, os.ModePerm)
				if err != nil {
					t.Fatalf("Failed to create tfvars directories: %v", err)
				}
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
			defer func() { getwd = originalGetwd }()

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
			defer func() { glob = originalGlob }()

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "local", nil
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return configRoot, nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create TerraformHelper
			terraformHelper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// Ensure the file does not exist
			fileToCheck := filepath.Join(projectPath, "non_existent_file.tfvars")
			if _, err := os.Stat(fileToCheck); !os.IsNotExist(err) {
				t.Fatalf("Expected file to not exist, but it does")
			}

			// Mock stat to return an error other than os.ErrNotExist
			mockStatWithError(fmt.Errorf("mock error"))
			defer restoreStat()

			// When calling GetEnvVars
			_, err = terraformHelper.GetEnvVars()

			// Then it should return an error
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
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "local", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "kubernetes", nil // Ensure this matches the expected backend
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
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
	})
	t.Run("GetAlias", func(t *testing.T) {
		t.Run("LocalContext", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetAlias
			alias, err := helper.GetAlias()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			// And the alias should match the expected alias
			expectedAlias := map[string]string{"terraform": "tflocal"}
			if !reflect.DeepEqual(alias, expectedAlias) {
				t.Errorf("Expected alias: %v, got: %v", expectedAlias, alias)
			}
		})

		t.Run("NonLocalContext", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "remote", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetAlias
			alias, err := helper.GetAlias()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			// And the alias should match the expected alias
			expectedAlias := map[string]string{"terraform": ""}
			if !reflect.DeepEqual(alias, expectedAlias) {
				t.Errorf("Expected alias: %v, got: %v", expectedAlias, alias)
			}
		})

		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock context and config handler that returns an error for context
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "", fmt.Errorf("error retrieving context")
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// When creating a new TerraformHelper
			helper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("Failed to create TerraformHelper: %v", err)
			}

			// And calling GetAlias
			alias, err := helper.GetAlias()

			// Then an error should be returned
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}

			// And the alias should be nil
			if alias != nil {
				t.Errorf("Expected alias to be nil, got: %v", alias)
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a temporary directory and project path
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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return projectPath, nil
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				switch key {
				case "context":
					return "local", nil
				case "contexts.local.terraform.backend":
					return "local", nil // Return "local" instead of "kubernetes"
				default:
					return "", fmt.Errorf("unexpected key: %s", key)
				}
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
			// Given a mock getwd that returns an error
			originalGetwd := getwd
			getwd = func() (string, error) {
				return "", fmt.Errorf("mock error getting current directory")
			}
			defer func() { getwd = originalGetwd }()

			// Mock the context and config handler
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()

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
			// Given a temporary directory and project path
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

			// Mock the context and config handler
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()

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

			// Verify that no backend_override.tf file was created
			backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
			if _, err := os.Stat(backendOverridePath); err == nil {
				t.Fatalf("expected no backend_override.tf file to be created")
			}
		})

		t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
			// Given a temporary directory and project path
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
			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", fmt.Errorf("mock error getting config root")
			}
			mockConfigHandler := config.NewMockConfigHandler()

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
			// Given a temporary directory and project path
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

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()

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
			// Given a temporary directory and project path
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
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local" {
					return "", fmt.Errorf("mock error getting backend configuration")
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
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
			// Given a temporary directory and project path
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
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "", fmt.Errorf("mock error retrieving context")
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
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
			// Given a setup test environment with "local" backend
			projectPath, cleanup, terraformHelper := setupTestEnv(t, "local", nil)
			defer cleanup()

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local.terraform.backend" {
					return "local", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return projectPath, nil
			}

			// Set up DI container
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
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
		t.Run("BackendS3", func(t *testing.T) {
			// Given a setup test environment with "s3" backend
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
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local.terraform.backend" {
					return "s3", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return projectPath, nil
			}

			// Set up DI container
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
			// Given a setup test environment with "kubernetes" backend
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

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local.terraform.backend" {
					return "kubernetes", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return longProjectPath, nil
			}

			// Set up DI container
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

		t.Run("UnsupportedBackend", func(t *testing.T) {
			// Given a setup test environment with an unsupported backend
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

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local.terraform.backend" {
					return "unsupported", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return projectPath, nil
			}

			// Set up DI container
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

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
			expectedErrorMsg := "unsupported backend: unsupported"
			if !strings.Contains(err.Error(), expectedErrorMsg) {
				t.Fatalf("expected error message to contain '%s', got %v", expectedErrorMsg, err)
			}
		})
		t.Run("ErrorWritingBackendOverride", func(t *testing.T) {
			// Given a setup test environment with a local backend
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

			// Mock config handler to return a context configuration with the specified backend
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string) (string, error) {
				if key == "context" {
					return "local", nil
				}
				if key == "contexts.local.terraform.backend" {
					return "local", nil
				}
				return "", fmt.Errorf("unexpected key: %s", key)
			}

			mockContext := context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return projectPath, nil
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
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context and mock config handler
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()

			container := di.NewContainer()
			container.Register("context", mockContext)
			container.Register("cliConfigHandler", mockConfigHandler)

			// Create TerraformHelper
			terraformHelper, err := NewTerraformHelper(container)
			if err != nil {
				t.Fatalf("NewTerraformHelper() error = %v", err)
			}

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
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/path/to/config", nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Create an instance of TerraformHelper
			terraformHelper, err := NewTerraformHelper(diContainer)
			if err != nil {
				t.Fatalf("NewTerraformHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = terraformHelper.WriteConfig()
			if err != nil {
				t.Fatalf("WriteConfig() error = %v", err)
			}

			// Then: no error should be returned
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	})
}
