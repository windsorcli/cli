package helpers

import (
	"fmt"
	"os"
	"path/filepath"
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
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
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
	diContainer.Register("contextHandler", mockContext)

	terraformHelper, err := NewTerraformHelper(diContainer)
	if err != nil {
		t.Fatalf("Failed to create TerraformHelper: %v", err)
	}

	return projectPath, func() {}, terraformHelper
}

func TestTerraformHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// When: Initialize is called
		err = terraformHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestTerraformHelper_NewTerraformHelper(t *testing.T) {
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
		diContainer.Register("contextHandler", mockContext)

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
		diContainer.Register("contextHandler", "not a context interface")

		// When creating a new TerraformHelper
		_, err := NewTerraformHelper(diContainer)

		// Then an error about context type should be returned
		if err == nil || !strings.Contains(err.Error(), "resolved context is not of type ContextInterface") {
			t.Fatalf("expected error about context type, got %v", err)
		}
	})
}

func TestTerraformHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context and mock config handler
		mockContext := context.NewMockContext()
		mockConfigHandler := config.NewMockConfigHandler()

		container := di.NewContainer()
		container.Register("contextHandler", mockContext)
		container.Register("cliConfigHandler", mockConfigHandler)

		// Create TerraformHelper
		terraformHelper, err := NewTerraformHelper(container)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := terraformHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the result should be nil as per the stub implementation
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})
}

func TestTerraformHelper_WriteConfig(t *testing.T) {
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
		diContainer.Register("contextHandler", mockContext)

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
}

func TestTerraformHelper_generateBackendOverrideTf(t *testing.T) {
	// Common setup for both tests
	// Mock getwd function
	originalGetwd := getwd
	defer func() { getwd = originalGetwd }()
	getwd = func() (string, error) {
		return "/mock/path/terraform/project", nil
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "terraform_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a mock Terraform file in the temp directory
	tfFilePath := filepath.Join(tempDir, "main.tf")
	err = os.WriteFile(tfFilePath, []byte(`resource "null_resource" "test" {}`), 0644)
	if err != nil {
		t.Fatalf("Failed to write mock Terraform file: %v", err)
	}

	// Mock glob function to return the mock Terraform file
	originalGlob := glob
	defer func() { glob = originalGlob }()
	glob = func(pattern string) ([]string, error) {
		return []string{tfFilePath}, nil
	}

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock context that returns an error for GetConfigRoot
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// And a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: ptrString("local"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create a temporary directory to simulate the Terraform project path
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform", "project")
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// When creating a new TerraformHelper
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// And calling generateBackendOverrideTf
		err = generateBackendOverrideTf(helper)

		// Then it should return an error indicating config root retrieval failure
		if err == nil || !strings.Contains(err.Error(), "error getting config root") {
			t.Fatalf("expected error getting config root, got %v", err)
		}
	})

	t.Run("ErrorRetrievingConfigForContext", func(t *testing.T) {
		// Given a mock context that returns a valid config root
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// And a mock config handler that returns an error for GetConfig
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving config for context")
		}

		// And a DI container with the mock context and config handler registered
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create a temporary directory to simulate the Terraform project path
		tempDir := t.TempDir()
		projectPath := filepath.Join(tempDir, "terraform", "project")
		err := os.MkdirAll(projectPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Mock getwd to return the project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return projectPath, nil
		}
		defer func() { getwd = originalGetwd }()

		// When creating a new TerraformHelper
		helper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// And calling generateBackendOverrideTf
		err = generateBackendOverrideTf(helper)

		// Then it should return an error indicating a problem with retrieving the config for context
		if err == nil || !strings.Contains(err.Error(), "error retrieving config for context") {
			t.Fatalf("expected error retrieving config for context, got %v", err)
		}
	})
}

func TestTerraformHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// When: Up is called
		err = terraformHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestTerraformHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of TerraformHelper
		terraformHelper, err := NewTerraformHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTerraformHelper() error = %v", err)
		}

		// When: Info is called
		info, err := terraformHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
