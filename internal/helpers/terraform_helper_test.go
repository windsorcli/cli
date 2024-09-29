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

func TestTerraformHelper_FindRelativeTerraformProjectPath(t *testing.T) {
	// Override getwd and glob for the test
	originalGetwd := getwd
	originalGlob := glob
	defer func() {
		getwd = originalGetwd
		glob = originalGlob
	}() // Restore original functions after test

	t.Run("ValidProjectPath", func(t *testing.T) {
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
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(tempDir, "project"))
		})

		// When: FindRelativeTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		relativePath, err := helper.FindRelativeTerraformProjectPath()

		// Then: it should return the correct relative path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedPath := "something"
		if relativePath != expectedPath {
			t.Errorf("Expected %s, got %s", expectedPath, relativePath)
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		// Given: a directory without Terraform files
		tempDir := t.TempDir()
		noTfDir := filepath.Join(tempDir, "project/root/nodir")

		// Mock getwd to return the directory without Terraform files
		originalGetwd := getwd
		getwd = func() (string, error) {
			return noTfDir, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create the directory
		err := os.MkdirAll(noTfDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(tempDir, "project"))
		})

		// When: FindRelativeTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		relativePath, err := helper.FindRelativeTerraformProjectPath()

		// Then: it should return no error and an empty string
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if relativePath != "" {
			t.Errorf("Expected empty string, got %s", relativePath)
		}
	})

	t.Run("NoTerraformDirectory", func(t *testing.T) {
		// Given: a directory with Terraform files but no "terraform" directory in the path
		tempDir := os.TempDir()
		noTfDir := filepath.Join(tempDir, "project/root/nodir")
		getwd = func() (string, error) {
			return noTfDir, nil
		}

		err := os.MkdirAll(noTfDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(filepath.Join(noTfDir, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(tempDir, "project"))
		})

		// When: FindRelativeTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		_, err = helper.FindRelativeTerraformProjectPath()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "no 'terraform' directory found in the current path" {
			t.Fatalf("Expected error 'no 'terraform' directory found in the current path', got %v", err)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		// Given: an error occurs when finding the Terraform project path
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")

		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		// Create the directory
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(tempDir, "project"))
		})

		// When: GetEnvVars is called
		mockContext := &context.MockContext{}
		mockShell := &shell.MockShell{}
		helper := NewTerraformHelper(nil, mockShell, mockContext)
		envVars, err := helper.GetEnvVars()

		// Then: it should return an error and empty environment variables
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error getting current directory") {
			t.Fatalf("Expected error message to contain 'mock error getting current directory', got %v", err)
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty environment variables, got %v", envVars)
		}
	})
}

func TestTerraformHelper_GenerateTerraformTfvarsFlags(t *testing.T) {
	// Given: a mock context and config handler
	projectRoot := t.TempDir()
	configRoot := filepath.Join(projectRoot, "contexts/local")
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return configRoot, nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		nil,
	)
	helper := NewTerraformHelper(mockConfigHandler, nil, mockContext)

	t.Run("ValidProjectPath", func(t *testing.T) {
		// Given: a valid project path
		terraformProjectPath := filepath.Join(projectRoot, "terraform/windsor/blueprint")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return terraformProjectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create mock tfvars files
		err := os.MkdirAll(terraformProjectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(terraformProjectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfvarsDir := filepath.Join(configRoot, "windsor")
		if err := os.MkdirAll(tfvarsDir, os.ModePerm); err != nil {
			t.Fatalf("Failed to create tfvars directory: %v", err)
		}
		tfvarsFiles := []string{
			"blueprint.tfvars",
			"blueprint_generated.tfvars.json",
		}
		for _, file := range tfvarsFiles {
			if err := os.WriteFile(filepath.Join(tfvarsDir, file), []byte(""), os.ModePerm); err != nil {
				t.Fatalf("Failed to create tfvars file: %v", err)
			}
		}

		// When: GenerateTerraformTfvarsFlags is called
		flags, err := helper.GenerateTerraformTfvarsFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := []string{
			"-var-file=" + filepath.Join(tfvarsDir, "blueprint.tfvars"),
			"-var-file=" + filepath.Join(tfvarsDir, "blueprint_generated.tfvars.json"),
		}
		sort.Strings(expectedFlags)
		actualFlags := strings.Split(flags, " ")
		sort.Strings(actualFlags)
		if !reflect.DeepEqual(expectedFlags, actualFlags) {
			t.Errorf("Expected %v, got %v", expectedFlags, actualFlags)
		}
	})

	t.Run("InvalidProjectPath", func(t *testing.T) {
		// Given: an invalid project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("project path not found")
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateTerraformTfvarsFlags is called
		_, err := helper.GenerateTerraformTfvarsFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		// Given: a valid project path but an error occurs when finding Terraform files
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")

		// Mock getwd and glob functions
		originalGetwd := getwd
		originalGlob := glob
		getwd = func() (string, error) {
			return projectPath, nil
		}
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() {
			getwd = originalGetwd
			glob = originalGlob
		}()

		// Create the directory
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// When: GenerateTerraformTfvarsFlags is called
		helper := NewTerraformHelper(nil, nil, nil)
		_, err = helper.GenerateTerraformTfvarsFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "error finding project path: mock error finding project path" {
			t.Fatalf("Expected error 'error finding project path: mock error finding project path', got %v", err)
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

		// Mock context to return an error when GetConfigRoot is called
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", fmt.Errorf("error getting config root")
			},
		}

		// When: GenerateTerraformTfvarsFlags is called
		helper := NewTerraformHelper(nil, nil, mockContext)
		_, err = helper.GenerateTerraformTfvarsFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "error getting config root" {
			t.Fatalf("Expected error 'error getting config root', got %v", err)
		}
	})
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

		// Mock glob to return a valid result for FindRelativeTerraformProjectPath
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
}

func TestTerraformHelper_GetCurrentBackend(t *testing.T) {
	t.Run("ValidBackend", func(t *testing.T) {
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
						"backend": "s3",
					}, nil
				}
				return nil, fmt.Errorf("unexpected key: %s", key)
			},
		)
		helper := NewTerraformHelper(mockConfigHandler, nil, nil)

		backend, err := helper.GetCurrentBackend()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if backend != "s3" {
			t.Errorf("Expected backend 's3', got '%s'", backend)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "", fmt.Errorf("error retrieving context")
			},
			nil,
		)
		helper := NewTerraformHelper(mockConfigHandler, nil, nil)

		backend, err := helper.GetCurrentBackend()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if backend != "local" {
			t.Errorf("Expected backend 'local', got '%s'", backend)
		}
	})

	t.Run("BackendNotString", func(t *testing.T) {
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
						"backend": 123, // Non-string backend
					}, nil
				}
				return nil, fmt.Errorf("unexpected key: %s", key)
			},
		)
		helper := NewTerraformHelper(mockConfigHandler, nil, nil)

		backend, err := helper.GetCurrentBackend()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if backend != "local" {
			t.Errorf("Expected backend 'local', got '%s'", backend)
		}
	})
}

func TestTerraformHelper_FindTerraformProjectRoot(t *testing.T) {
	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		helper := &TerraformHelper{}
		_, err := helper.FindTerraformProjectRoot()

		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error getting current directory: mock error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("NoTerraformDirectoryFound", func(t *testing.T) {
		// Mock getwd to return a path without a "terraform" directory
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root", nil
		}
		defer func() { getwd = originalGetwd }()

		helper := &TerraformHelper{}
		_, err := helper.FindTerraformProjectRoot()

		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "no 'terraform' directory found in the current path"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("TerraformDirectoryFound", func(t *testing.T) {
		// Mock getwd to return a path with a "terraform" directory
		originalGetwd := getwd
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/subdir"), nil
		}
		defer func() { getwd = originalGetwd }()

		helper := &TerraformHelper{}
		projectRoot, err := helper.FindTerraformProjectRoot()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedRoot := filepath.FromSlash("/mock/project/root/terraform")
		projectRoot = filepath.Clean(projectRoot)
		expectedRoot = filepath.Clean(expectedRoot)

		// Ensure both paths are absolute
		if !filepath.IsAbs(projectRoot) {
			projectRoot = filepath.Join("/", projectRoot)
		}
		if !filepath.IsAbs(expectedRoot) {
			expectedRoot = filepath.Join("/", expectedRoot)
		}

		if projectRoot != expectedRoot {
			t.Fatalf("Expected project root '%s', got '%s'", expectedRoot, projectRoot)
		}
	})
}

// TestTerraformHelper_SanitizeForK8s tests the SanitizeForK8s method
func TestTerraformHelper_SanitizeForK8s(t *testing.T) {
	helper := &TerraformHelper{}

	tests := []struct {
		input    string
		expected string
	}{
		{"My_Test_String", "my-test-string"},
		{"My--Test--String", "my-test-string"},
		{"My__Test__String", "my-test-string"},
		{"My!!Test!!String", "my-test-string"},
		{"MyTestStringWithMoreThanSixtyThreeCharactersWhichShouldBeTrimmed", "myteststringwithmorethansixtythreecharacterswhichshouldbetrimme"},
		{"MyTestStringWithExactlySixtyThreeCharactersWhichShouldNotBeTrimmed", "myteststringwithexactlysixtythreecharacterswhichshouldnotbetrim"},
		{"MyTestStringWithSpecialChars!@#$%^&*()", "myteststringwithspecialchars"},
		{"MyTestStringWithLeadingAndTrailingHyphens-", "myteststringwithleadingandtrailinghyphens"},
		{"-MyTestStringWithLeadingAndTrailingHyphens", "myteststringwithleadingandtrailinghyphens"},
		{"-MyTestStringWithLeadingAndTrailingHyphens-", "myteststringwithleadingandtrailinghyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := helper.SanitizeForK8s(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s (length: %d)", tt.expected, result, len(result))
			}
		})
	}
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

func TestTerraformHelper_GenerateBackendOverrideTf(t *testing.T) {
	// Mock dependencies
	mockConfigHandler := &config.MockConfigHandler{}
	mockShell := &shell.MockShell{}
	mockContext := &context.MockContext{}

	t.Run("Success", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform", "project")

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

		// Create the directory and a .tf file to simulate a valid Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		tfFile, err := os.Create(filepath.Join(projectPath, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfFile.Close() // Ensure the file is closed before cleanup
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check if the backend_override.tf file was created
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		if _, err := os.Stat(backendOverridePath); os.IsNotExist(err) {
			t.Fatalf("Expected backend_override.tf to be created, but it does not exist")
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")
		configRoot := filepath.Join(tempDir, "config/root")

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock context to return the config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
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

		// Create the directory without any .tf files
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should not generate the backend_override.tf file and return no error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check if the backend_override.tf file was not created
		backendOverridePath := filepath.Join(projectPath, "backend_override.tf")
		if _, err := os.Stat(backendOverridePath); !os.IsNotExist(err) {
			t.Fatalf("Expected backend_override.tf to not be created, but it exists")
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

		// Mock context to return the config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Mock GetCurrentBackend to return an error
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			return "", fmt.Errorf("mock error getting backend")
		}

		// Create the directory and a .tf file to simulate a valid Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		tfFile, err := os.Create(filepath.Join(projectPath, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfFile.Close() // Ensure the file is closed before cleanup
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting backend") {
			t.Fatalf("Expected error message to contain 'error getting backend', got %v", err)
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

		// Mock context to return an error when getting the config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
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

		// Create the directory and a .tf file to simulate a valid Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		tfFile, err := os.Create(filepath.Join(projectPath, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfFile.Close() // Ensure the file is closed before cleanup
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Fatalf("Expected error message to contain 'error getting config root', got %v", err)
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

		// Mock WriteFile to return an error
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend override tf")
		}
		defer func() { writeFile = originalWriteFile }()

		// Create the directory and a .tf file to simulate a valid Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		tfFile, err := os.Create(filepath.Join(projectPath, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfFile.Close() // Ensure the file is closed before cleanup
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing backend override tf") {
			t.Fatalf("Expected error message to contain 'error writing backend override tf', got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform/project")

		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
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

		// Create the directory and a .tf file to simulate a valid Terraform project
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		tfFile, err := os.Create(filepath.Join(projectPath, "main.tf"))
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		tfFile.Close() // Ensure the file is closed before cleanup
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("Expected error message to contain 'error getting current directory', got %v", err)
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

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Fatalf("Expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("ErrorGeneratingBackendOverrideTf", func(t *testing.T) {
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

		// Mock glob to return an error to simulate failure in finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		// When: GenerateBackendOverrideTf is called
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Fatalf("Expected error message to contain 'error finding project path', got %v", err)
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

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Given a TerraformHelper instance with an error retrieving config
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
			return nil, nil // Simulate backendConfig being nil
		}

		terraformHelper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		expectedError := fmt.Errorf("error retrieving config for context: backendConfig is nil")
		err := terraformHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}
