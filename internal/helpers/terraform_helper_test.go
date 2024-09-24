package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestTerraformHelper_FindTerraformProjectPath(t *testing.T) {
	// Override getwd for the test
	originalGetwd := getwd
	defer func() { getwd = originalGetwd }() // Restore original function after test

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
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// When: FindTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		relativePath, err := helper.FindTerraformProjectPath()

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
		tempDir := os.TempDir()
		noTfDir := filepath.Join(tempDir, "project/root/nodir")
		getwd = func() (string, error) {
			return noTfDir, nil
		}

		err := os.MkdirAll(noTfDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// When: FindTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		_, err = helper.FindTerraformProjectPath()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "no Terraform files found in the current directory" {
			t.Fatalf("Expected error 'no Terraform files found in the current directory', got %v", err)
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
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		// When: FindTerraformProjectPath is called
		helper := NewTerraformHelper(nil, nil, nil)
		_, err = helper.FindTerraformProjectPath()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "no 'terraform' directory found in the current path" {
			t.Fatalf("Expected error 'no 'terraform' directory found in the current path', got %v", err)
		}
	})
}

func TestTerraformHelper_GenerateTerraformTfvarsFlags(t *testing.T) {
	// Given: a mock context and config handler
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			tempDir := os.TempDir()
			return filepath.Join(tempDir, "mock/config/root"), nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		nil,
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidProjectPath", func(t *testing.T) {
		// Given: a valid project path
		tempDir := os.TempDir()
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")
		configRoot := filepath.Join(tempDir, "mock/config/root")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create mock tfvars file
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
		err = os.WriteFile(filepath.Join(configRoot, "something.tfvars"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create tfvars file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// When: GenerateTerraformTfvarsFlags is called
		flags, err := helper.GenerateTerraformTfvarsFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-var-file=" + filepath.Join(configRoot, "something.tfvars")
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
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
		// Given: a directory without a "terraform" directory in the path
		tempDir := os.TempDir()
		noTfDir := filepath.Join(tempDir, "project/root/nodir")
		getwd = func() (string, error) {
			return noTfDir, nil
		}

		err := os.MkdirAll(noTfDir, os.ModePerm)
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
		expectedFlags := "-backend=true -backend-config=key=something/terraform.tfstate -backend-config=secret_suffix=something"
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
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

		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error finding project path")
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateTerraformInitBackendFlags is called
		_, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "error getting current directory: mock error finding project path" {
			t.Fatalf("Expected error 'error getting current directory: mock error finding project path', got %v", err)
		}
	})
}

func TestTerraformHelper_GenerateBackendOverrideTf(t *testing.T) {
	t.Run("ValidBackend", func(t *testing.T) {
		// Given: a valid backend
		tempDir := os.TempDir()
		workingDir := filepath.Join(tempDir, "mock/working/dir/terraform")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return workingDir, nil
		}
		t.Cleanup(func() { getwd = originalGetwd }) // Ensure getwd is reset after the test

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(workingDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(workingDir, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// Mock context to return a valid config root
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

		// When: GenerateBackendOverrideTf is called
		err = helper.GenerateBackendOverrideTf()

		// Then: it should create the backend_override.tf file
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedPath := filepath.Join(workingDir, "backend_override.tf")
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Fatalf("Expected file %s to be created", expectedPath)
		}
	})

	t.Run("InvalidBackend", func(t *testing.T) {
		// Given: an invalid backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return nil, errors.New("backend not found")
			},
		)
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/config/root", nil
			},
		}
		mockShell := createMockShell(func() (string, error) {
			return "/mock/project/root", nil
		})
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting backend") || !strings.Contains(err.Error(), "backend not found") {
			t.Fatalf("Expected error to contain 'error getting backend' and 'backend not found', got '%v'", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given: a valid project path with .tf files
		tempDir := os.TempDir()
		workingDir := filepath.Join(tempDir, "mock/working/dir/terraform")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return workingDir, nil
		}
		t.Cleanup(func() { getwd = originalGetwd }) // Ensure getwd is reset after the test

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(workingDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(workingDir, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// Mock context to return an error when GetConfigRoot is called
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", fmt.Errorf("error getting config root")
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

		// When: GenerateBackendOverrideTf is called
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error getting config root"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("ErrorWritingBackendOverrideTf", func(t *testing.T) {
		// Given: a valid project path with .tf files
		tempDir := os.TempDir()
		workingDir := filepath.Join(tempDir, "mock/working/dir/terraform")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return workingDir, nil
		}
		t.Cleanup(func() { getwd = originalGetwd }) // Ensure getwd is reset after the test

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(workingDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(workingDir, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// Mock writeFile to return an error
		originalWriteFile := writeFile
		writeFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend_override.tf")
		}
		t.Cleanup(func() { writeFile = originalWriteFile }) // Ensure writeFile is reset after the test

		// Mock context to return a valid config root
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

		// When: GenerateBackendOverrideTf is called
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error writing backend_override.tf: mock error writing backend_override.tf"
		if err.Error() != expectedError {
			t.Fatalf("Expected error '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
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

		// Mock getwd to return an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error getting current directory: mock error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
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

		// Mock getwd to return a valid directory
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/something", nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return an error
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error finding project path: mock error finding project path"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}
	})
}

func TestTerraformHelper_GetCurrentBackend(t *testing.T) {
	// Given: a mock config handler that returns an error when retrieving the context
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "", errors.New("error retrieving context")
		},
		nil,
	)
	helper := NewTerraformHelper(mockConfigHandler, nil, nil)

	// When: GetCurrentBackend is called
	_, err := helper.GetCurrentBackend()

	// Then: it should return an error
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if err.Error() != "error retrieving context: error retrieving context" {
		t.Fatalf("Expected error 'error retrieving context: error retrieving context', got %v", err)
	}

	// Given: a mock config handler that returns an error when retrieving the config for the context
	mockConfigHandler = createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		func(key string) (map[string]interface{}, error) {
			return nil, errors.New("error retrieving config for context")
		},
	)
	helper = NewTerraformHelper(mockConfigHandler, nil, nil)

	// When: GetCurrentBackend is called
	_, err = helper.GetCurrentBackend()

	// Then: it should return an error
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if err.Error() != "error retrieving config for context: error retrieving config for context" {
		t.Fatalf("Expected error 'error retrieving config for context: error retrieving config for context', got %v", err)
	}

	// Given: a mock config handler that returns a config without a backend
	mockConfigHandler = createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		func(key string) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		},
	)
	helper = NewTerraformHelper(mockConfigHandler, nil, nil)

	// When: GetCurrentBackend is called
	backend, err := helper.GetCurrentBackend()

	// Then: it should return an empty backend
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if backend != "" {
		t.Fatalf("Expected empty backend, got %v", backend)
	}
}

func TestTerraformHelper_SanitizeForK8s(t *testing.T) {
	helper := NewTerraformHelper(nil, nil, nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"My_Test_String", "my-test-string"},
		{"My__Test__String", "my-test-string"},
		{"My!Test@String#", "my-test-string"},
		{"My--Test--String", "my-test-string"},
		{"-My-Test-String-", "my-test-string"},
		{"MyTestStringThatIsWayTooLongAndShouldBeTrimmedToSixtyThreeCharacters", "myteststringthatiswaytoolongandshouldbetrimmedtosixtythreechara"},
		{"MyTestStringWith123Numbers", "myteststringwith123numbers"},
		{"MyTestStringWithSpecialChars!@#$%^&*()", "myteststringwithspecialchars"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := helper.SanitizeForK8s(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestTerraformHelper_GetAlias(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "local", nil
			}
			return "", fmt.Errorf("key not found")
		},
	}

	helper := NewTerraformHelper(mockConfigHandler, nil, nil)

	t.Run("LocalContext", func(t *testing.T) {
		alias, err := helper.GetAlias()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedAlias := map[string]string{"terraform": "tflocal"}
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("Expected %v, got %v", expectedAlias, alias)
		}
	})

	t.Run("NonLocalContext", func(t *testing.T) {
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			if key == "context" {
				return "remote", nil
			}
			return "", fmt.Errorf("key not found")
		}
		alias, err := helper.GetAlias()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedAlias := map[string]string{"terraform": ""}
		if !reflect.DeepEqual(alias, expectedAlias) {
			t.Errorf("Expected %v, got %v", expectedAlias, alias)
		}
	})

	t.Run("ErrorGettingContext", func(t *testing.T) {
		mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
			return "", fmt.Errorf("error retrieving context")
		}
		_, err := helper.GetAlias()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error retrieving context: error retrieving context"
		if err.Error() != expectedError {
			t.Fatalf("Expected error '%s', got '%v'", expectedError, err)
		}
	})
}

func TestTerraformHelper_GetEnvVars(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	tempDir := os.TempDir()
	configRoot := filepath.Join(tempDir, "mock/config/root")
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return configRoot, nil
		},
	}
	mockShell := &shell.MockShell{}

	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidProjectPath", func(t *testing.T) {
		projectPath := filepath.Join(tempDir, "project/root/terraform/something")

		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// Create mock Terraform files in the project directory
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(projectPath, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "project"))

		envVars, err := helper.GetEnvVars()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.Join(configRoot, ".terraform/something"),
			"TF_CLI_ARGS_init": fmt.Sprintf("-backend=true -backend-config=path=%s", filepath.Join(configRoot, ".tfstate/something/terraform.tfstate")),
			"TF_CLI_ARGS_plan": fmt.Sprintf("-out=%s -var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
				filepath.Join(configRoot, ".terraform/something/terraform.tfplan"), "something", "something", "something", "something", "something"),
			"TF_CLI_ARGS_apply": filepath.Join(configRoot, ".terraform/something/terraform.tfplan"),
			"TF_CLI_ARGS_import": fmt.Sprintf("-var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
				"something", "something", "something", "something", "something"),
			"TF_CLI_ARGS_destroy": fmt.Sprintf("-var-file=%s.tfvars -var-file=%s.json -var-file=%s_generated.tfvars -var-file=%s_generated.json -var-file=%s_generated.tfvars.json",
				"something", "something", "something", "something", "something"),
			"TF_VAR_context_path": configRoot,
		}

		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		envVars, err := helper.GetEnvVars()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "error getting current directory: mock error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         "",
			"TF_CLI_ARGS_init":    "",
			"TF_CLI_ARGS_plan":    "",
			"TF_CLI_ARGS_apply":   "",
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": "",
		}

		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root", nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock the behavior of FindTerraformProjectPath by simulating the absence of Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		envVars, err := helper.GetEnvVars()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "mock error finding project path"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         "",
			"TF_CLI_ARGS_init":    "",
			"TF_CLI_ARGS_plan":    "",
			"TF_CLI_ARGS_apply":   "",
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": "",
		}

		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root", nil
		}
		defer func() { getwd = originalGetwd }()

		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting config root")
		}

		envVars, err := helper.GetEnvVars()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "error getting config root"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         "",
			"TF_CLI_ARGS_init":    "",
			"TF_CLI_ARGS_plan":    "",
			"TF_CLI_ARGS_apply":   "",
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": "",
		}

		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("Expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorWritingBackendOverrideTf", func(t *testing.T) {
		// Given: a valid project path with .tf files
		tempDir := os.TempDir()
		workingDir := filepath.Join(tempDir, "mock/working/dir/terraform")
		originalGetwd := getwd
		getwd = func() (string, error) {
			return workingDir, nil
		}
		t.Cleanup(func() { getwd = originalGetwd }) // Ensure getwd is reset after the test

		// Create a mock .tf file to ensure the directory is recognized as a Terraform project
		err := os.MkdirAll(workingDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		err = os.WriteFile(filepath.Join(workingDir, "main.tf"), []byte(""), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create .tf file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(tempDir, "mock"))

		// Mock writeFile to return an error
		originalWriteFile := writeFile
		writeFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend_override.tf")
		}
		t.Cleanup(func() { writeFile = originalWriteFile }) // Ensure writeFile is reset after the test

		// Mock context to return a valid config root
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

		// When: GenerateBackendOverrideTf is called
		err = helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "error writing backend_override.tf: mock error writing backend_override.tf"
		if err.Error() != expectedError {
			t.Fatalf("Expected error '%s', got '%v'", expectedError, err)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
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

		// Mock getwd to return a valid directory
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/something", nil
		}
		defer func() { getwd = originalGetwd }()

		// Mock glob to return an error
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}
		defer func() { glob = originalGlob }()

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "mock error finding project path"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s', got '%v'", expectedError, err)
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
			return "/mock/project/root/terraform/subdir", nil
		}
		defer func() { getwd = originalGetwd }()

		helper := &TerraformHelper{}
		projectRoot, err := helper.FindTerraformProjectRoot()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedRoot := filepath.Clean("/mock/project/root/terraform")
		if !strings.HasPrefix(projectRoot, "/") {
			projectRoot = "/" + projectRoot
		}
		if projectRoot != expectedRoot {
			t.Fatalf("Expected project root '%s', got '%s'", expectedRoot, projectRoot)
		}
	})
}
