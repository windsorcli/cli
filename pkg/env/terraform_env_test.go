package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type TerraformEnvMocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

func setupSafeTerraformEnvMocks(injector ...di.Injector) *TerraformEnvMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Terraform: &terraform.TerraformConfig{
				Backend: &terraform.BackendConfig{
					Type: "local",
				},
			},
		}
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	mockInjector.Register("shell", mockShell)
	mockInjector.Register("configHandler", mockConfigHandler)

	stat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}

	return &TerraformEnvMocks{
		Injector:      mockInjector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
	}
}

func TestTerraformEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         `/mock/config/root/.terraform/project/path`,
			"TF_CLI_ARGS_init":    `-backend=true -backend-config="path=/mock/config/root/.tfstate/project/path/terraform.tfstate"`,
			"TF_CLI_ARGS_plan":    `-out="/mock/config/root/.terraform/project/path/terraform.tfplan" -var-file="/mock/config/root/terraform/project/path.tfvars" -var-file="/mock/config/root/terraform/project/path.tfvars.json"`,
			"TF_CLI_ARGS_apply":   `"/mock/config/root/.terraform/project/path/terraform.tfplan"`,
			"TF_CLI_ARGS_import":  `-var-file="/mock/config/root/terraform/project/path.tfvars" -var-file="/mock/config/root/terraform/project/path.tfvars.json"`,
			"TF_CLI_ARGS_destroy": `-var-file="/mock/config/root/terraform/project/path.tfvars" -var-file="/mock/config/root/terraform/project/path.tfvars.json"`,
			"TF_VAR_context_path": `/mock/config/root`,
		}

		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		// Given a mocked glob function simulating the presence of tf files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"real/terraform/project/path/file1.tf", "real/terraform/project/path/file2.tf"}, nil
			}
			return nil, nil
		}

		// And a mocked getwd function returning a specific path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		// And a mocked stat function simulating file existence with varied tfvars files
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			// Debugging: Print the path being checked
			t.Logf("Checking file: %s", name)
			switch name {
			case filepath.FromSlash("/mock/config/root/terraform/project/path.tfvars"):
				return nil, nil // Simulate file exists
			case filepath.FromSlash("/mock/config/root/terraform/project/path.tfvars.json"):
				return nil, nil // Simulate file exists
			case filepath.FromSlash("/mock/config/root/terraform/project/path_generated.tfvars"):
				return nil, os.ErrNotExist // Simulate file does not exist
			case filepath.FromSlash("/mock/config/root/terraform/project/path_generated.tfvars.json"):
				return nil, os.ErrNotExist // Simulate file does not exist
			default:
				return nil, os.ErrNotExist // Simulate file does not exist
			}
		}

		// When the GetEnvVars function is called
		envVars, err := terraformEnvPrinter.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Debugging: Print the actual envVars on Windows
		for key, value := range envVars {
			t.Logf("envVar[%s] = %s", key, value)
		}

		// Then the expected environment variables should be set
		for key, expectedValue := range expectedEnvVars {
			if value, exists := envVars[key]; !exists || value != expectedValue {
				t.Errorf("Expected %s to be %s, got %s", key, expectedValue, value)
			}
		}
	})

	t.Run("ErrorGettingProjectPath", func(t *testing.T) {
		// Mock the getwd function to simulate an error
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		mocks := setupSafeTerraformEnvMocks()

		// When the GetEnvVars function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		_, err := terraformEnvPrinter.GetEnvVars()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("NoProjectPathFound", func(t *testing.T) {
		// Given a mocked getwd function returning a specific path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root"), nil
		}
		mocks := setupSafeTerraformEnvMocks()

		// When the GetEnvVars function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		envVars, err := terraformEnvPrinter.GetEnvVars()

		// Then it should return nil without an error
		if envVars != nil {
			t.Errorf("Expected nil, got %v", envVars)
		}
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		// When the GetEnvVars function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		_, err := terraformEnvPrinter.GetEnvVars()

		// Then the error should be as expected
		expectedErrorMessage := "error getting config root: mock error getting config root"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorListingTfvarsFiles", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "mockContext"
		}
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		// And a mocked glob function succeeding for *.tf files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"file1.tf", "file2.tf"}, nil
			}
			return nil, nil
		}

		// And a mocked stat function returning an error other than os.IsNotExist
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock error checking file")
		}

		// When the GetEnvVars function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		_, err := terraformEnvPrinter.GetEnvVars()

		// Then the error should be as expected
		expectedErrorMessage := "error checking file: mock error checking file"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("TestWindows", func(t *testing.T) {
		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string {
			return "windows"
		}

		mocks := setupSafeTerraformEnvMocks()
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		// Mock the getwd function to simulate being in a terraform project path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		// Mock the glob function to simulate the presence of *.tf files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"main.tf"}, nil
			}
			return nil, nil
		}

		// Mock the stat function to simulate the existence of tfvars files
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/terraform/project/path.tfvars") {
				return nil, nil // Simulate file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the GetEnvVars function to verify it returns the correct envVars
		envVars, err := terraformEnvPrinter.GetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that GetEnvVars returns the correct envVars
		expectedEnvVars := map[string]string{
			"TF_VAR_os_type": "windows",
		}
		if envVars == nil {
			t.Errorf("envVars is nil, expected %v", expectedEnvVars)
		} else if value, exists := envVars["TF_VAR_os_type"]; !exists || value != expectedEnvVars["TF_VAR_os_type"] {
			t.Errorf("envVars[TF_VAR_os_type] = %v, want %v", value, expectedEnvVars["TF_VAR_os_type"])
		}
	})
}

func TestTerraformEnv_PostEnvHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "mockContext"
		}
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "local",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("mock/project/root/terraform/project/path"), nil
		}

		// And a mocked glob function succeeding for *.tf files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"file1.tf", "file2.tf"}, nil
			}
			return nil, nil
		}

		// And a mocked writeFile function simulating successful file writing
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		// When the PostEnvHook function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.PostEnvHook()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a mocked getwd function returning an error
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When the PostEnvHook function is called
		mocks := setupSafeTerraformEnvMocks()
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		// Given a mocked glob function returning an error
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}

		// When the PostEnvHook function is called
		mocks := setupSafeTerraformEnvMocks()
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "unsupported",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("mock/project/root/terraform/project/path"), nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{filepath.FromSlash("mock/project/root/terraform/project/path/main.tf")}, nil
		}

		// When the PostEnvHook function is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported backend") {
			t.Errorf("Expected error message to contain 'unsupported backend', got %v", err)
		}
	})

	t.Run("ErrorWritingBackendOverrideFile", func(t *testing.T) {
		// Given a mocked writeFile function returning an error
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend_override.tf file")
		}

		// And a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("mock/project/root/terraform/project/path"), nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{filepath.FromSlash("mock/project/root/terraform/project/path/main.tf")}, nil
		}

		// When the PostEnvHook function is called
		mocks := setupSafeTerraformEnvMocks()
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing backend_override.tf file") {
			t.Errorf("Expected error message to contain 'error writing backend_override.tf file', got %v", err)
		}
	})
}

func TestTerraformEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeTerraformEnvMocks to create mocks
		mocks := setupSafeTerraformEnvMocks()
		mockInjector := mocks.Injector
		terraformEnvPrinter := NewTerraformEnvPrinter(mockInjector)
		terraformEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the terraform config file
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.terraform/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the glob function to simulate the presence of *.tf files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"main.tf"}, nil // Simulate that tf files exist
			}
			return nil, nil
		}

		// Mock the getwd function to return a path that includes "terraform"
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := terraformEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Determine the expected OS type
		expectedOSType := "unix"
		if goos() == "windows" {
			expectedOSType = "windows"
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         "/mock/config/root/.terraform/project/path",
			"TF_CLI_ARGS_init":    "-backend=true -backend-config=\"path=/mock/config/root/.tfstate/project/path/terraform.tfstate\"",
			"TF_CLI_ARGS_plan":    `-out="/mock/config/root/.terraform/project/path/terraform.tfplan"`,
			"TF_CLI_ARGS_apply":   `"/mock/config/root/.terraform/project/path/terraform.tfplan"`,
			"TF_CLI_ARGS_import":  "",
			"TF_CLI_ARGS_destroy": "",
			"TF_VAR_context_path": "/mock/config/root",
			"TF_VAR_os_type":      expectedOSType,
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeTerraformEnvMocks to create mocks
		mocks := setupSafeTerraformEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock config error")
		}

		mockInjector := mocks.Injector

		terraformEnvPrinter := NewTerraformEnvPrinter(mockInjector)
		terraformEnvPrinter.Initialize()

		// Call Print and check for errors
		err := terraformEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestTerraformEnv_getAlias(t *testing.T) {
	t.Run("SuccessLocalstackEnabled", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "aws.localstack.create" {
				return true
			}
			return false
		}

		// When getAlias is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		aliases, err := terraformEnvPrinter.getAlias()

		// Then no error should occur and the expected alias should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedAlias := map[string]string{"terraform": "tflocal"}
		if !reflect.DeepEqual(aliases, expectedAlias) {
			t.Errorf("Expected aliases %v, got %v", expectedAlias, aliases)
		}
	})

	t.Run("SuccessLocalstackDisabled", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "local"
		}
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				AWS: &aws.AWSConfig{
					Localstack: &aws.LocalstackConfig{
						Enabled: boolPtr(false),
					},
				},
			}
		}

		// When getAlias is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		aliases, err := terraformEnvPrinter.getAlias()

		// Then no error should occur and the expected alias should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedAlias := map[string]string{"terraform": ""}
		if !reflect.DeepEqual(aliases, expectedAlias) {
			t.Errorf("Expected aliases %v, got %v", expectedAlias, aliases)
		}
	})
}

func TestTerraformEnv_findRelativeTerraformProjectPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mocked getwd function returning a specific directory path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/path/to/terraform/project"), nil
		}
		defer func() { getwd = originalGetwd }()

		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/path/to/terraform/project/*.tf") {
				return []string{filepath.FromSlash("/mock/path/to/terraform/project/main.tf")}, nil
			}
			return nil, nil
		}
		defer func() { glob = originalGlob }()

		// When findRelativeTerraformProjectPath is called
		projectPath, err := findRelativeTerraformProjectPath()

		// Then no error should occur and the expected project path should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedPath := "project"
		if projectPath != expectedPath {
			t.Errorf("Expected project path %v, got %v", expectedPath, projectPath)
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		// Given a mocked getwd function returning a specific directory path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/path/to/terraform/project"), nil
		}
		defer func() { getwd = originalGetwd }()

		// And a mocked glob function simulating no Terraform files found
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			return nil, nil
		}
		defer func() { glob = originalGlob }()

		// When findRelativeTerraformProjectPath is called
		projectPath, err := findRelativeTerraformProjectPath()

		// Then no error should occur and the project path should be empty
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if projectPath != "" {
			t.Errorf("Expected empty project path, got %v", projectPath)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a mocked getwd function returning an error
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		defer func() { getwd = originalGetwd }()

		// When findRelativeTerraformProjectPath is called
		_, err := findRelativeTerraformProjectPath()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("NoTerraformDirectoryFound", func(t *testing.T) {
		// Given a mocked getwd function returning a specific directory path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/path/to/project"), nil
		}
		defer func() { getwd = originalGetwd }()

		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/path/to/project/*.tf") {
				return []string{filepath.FromSlash("/mock/path/to/project/main.tf")}, nil
			}
			return nil, nil
		}
		defer func() { glob = originalGlob }()

		// When findRelativeTerraformProjectPath is called
		projectPath, err := findRelativeTerraformProjectPath()

		// Then no error should occur and the project path should be empty
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if projectPath != "" {
			t.Errorf("Expected empty project path, got %v", projectPath)
		}
	})
}

func TestTerraformEnv_sanitizeForK8s(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase and valid characters",
			input:    "valid-name",
			expected: "valid-name",
		},
		{
			name:     "Uppercase characters",
			input:    "VALID-NAME",
			expected: "valid-name",
		},
		{
			name:     "Underscores to hyphens",
			input:    "valid_name",
			expected: "valid-name",
		},
		{
			name:     "Invalid characters",
			input:    "valid@name!",
			expected: "valid-name",
		},
		{
			name:     "Consecutive hyphens",
			input:    "valid--name",
			expected: "valid-name",
		},
		{
			name:     "Leading and trailing hyphens",
			input:    "-valid-name-",
			expected: "valid-name",
		},
		{
			name:     "Exceeds max length",
			input:    "a-very-long-name-that-exceeds-the-sixty-three-character-limit-should-be-truncated",
			expected: "a-very-long-name-that-exceeds-the-sixty-three-character-limit-s",
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When sanitizeForK8s is called
			result := sanitizeForK8s(tt.input)

			// Then the result should match the expected output
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestTerraformEnv_generateBackendOverrideTf(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "local",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}
		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			expectedPattern := filepath.FromSlash("/mock/project/root/terraform/project/path/*.tf")
			if pattern == expectedPattern {
				return []string{filepath.FromSlash("/mock/project/root/terraform/project/path/main.tf")}, nil
			}
			return nil, nil
		}

		// And a mocked writeFile function to capture the output
		var writtenData []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `terraform {
  backend "local" {}
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("S3Backend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "s3",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}
		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/project/root/terraform/project/path/*.tf") {
				return []string{filepath.FromSlash("/mock/project/root/terraform/project/path/main.tf")}, nil
			}
			return nil, nil
		}

		// And a mocked writeFile function to capture the output
		var writtenData []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `terraform {
  backend "s3" {}
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("KubernetesBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "kubernetes",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}
		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/project/root/terraform/project/path/*.tf") {
				return []string{filepath.FromSlash("/mock/project/root/terraform/project/path/main.tf")}, nil
			}
			return nil, nil
		}

		// And a mocked writeFile function to capture the output
		var writtenData []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `terraform {
  backend "kubernetes" {}
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "unsupported",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}
		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/project/root/terraform/project/path/*.tf") {
				return []string{filepath.FromSlash("/mock/project/root/terraform/project/path/main.tf")}, nil
			}
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.generateBackendOverrideTf()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported backend: unsupported") {
			t.Errorf("Expected error message to contain 'unsupported backend: unsupported', got %v", err)
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "local",
					},
				},
			}
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}
		// And a mocked glob function simulating no Terraform files found
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()
		err := terraformEnvPrinter.generateBackendOverrideTf()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestTerraformEnv_generateBackendConfigArgs(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		backendConfigArgs, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.Join(configRoot, "terraform", "backend.tfvars")),
			fmt.Sprintf(`-backend-config="path=%s"`, filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")),
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("LocalBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Local: &terraform.LocalBackend{
							Path: stringPtr(filepath.FromSlash("/mock/config/root/.tfstate/project/path/terraform.tfstate")),
						},
					},
				},
			}
		}
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		backendConfigArgs, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.Join(configRoot, "terraform", "backend.tfvars")),
			fmt.Sprintf(`-backend-config="path=%s"`, filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")),
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("S3Backend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						S3: &terraform.S3Backend{
							Bucket:                    stringPtr("mock-bucket"),
							Region:                    stringPtr("mock-region"),
							AccessKey:                 stringPtr("mock-access-key"),
							SecretKey:                 stringPtr("mock-secret-key"),
							MaxRetries:                intPtr(5),
							SkipCredentialsValidation: boolPtr(true),
						},
					},
				},
			}
		}
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		backendConfigArgs, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.Join(configRoot, "terraform", "backend.tfvars")),
			`-backend-config="key=project/path/terraform.tfstate"`,
			`-backend-config="access_key=mock-access-key"`,
			`-backend-config="bucket=mock-bucket"`,
			`-backend-config="max_retries=5"`,
			`-backend-config="region=mock-region"`,
			`-backend-config="secret_key=mock-secret-key"`,
			`-backend-config="skip_credentials_validation=true"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("KubernetesBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Kubernetes: &terraform.KubernetesBackend{
							SecretSuffix: stringPtr("mock-secret-suffix"),
							Namespace:    stringPtr("mock-namespace"),
						},
					},
				},
			}
		}
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		backendConfigArgs, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.Join(configRoot, "terraform", "backend.tfvars")),
			`-backend-config="secret_suffix=project-path"`,
			`-backend-config="namespace=mock-namespace"`,
			`-backend-config="secret_suffix=mock-secret-suffix"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("BackendTfvarsFileExists", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		backendConfigArgs, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.Join(configRoot, "terraform", "backend.tfvars")),
			fmt.Sprintf(`-backend-config="path=%s"`, filepath.Join(configRoot, ".tfstate", projectPath, "terraform.tfstate")),
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("ErrorMarshallingBackendConfig", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "s3",
						S3:   &terraform.S3Backend{},
					},
				},
			}
		}

		// Mock yamlMarshal to return an error
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock marshalling error")
		}

		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		_, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "error marshalling backend to YAML: mock marshalling error") {
			t.Errorf("expected error to contain %v, got %v", "error marshalling backend to YAML: mock marshalling error", err.Error())
		}
	})

	t.Run("ErrorProcessingKubernetesBackendConfig", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type:       "kubernetes",
						Kubernetes: &terraform.KubernetesBackend{},
					},
				},
			}
		}

		// Mock processBackendConfig to return an error
		originalProcessBackendConfig := processBackendConfig
		defer func() { processBackendConfig = originalProcessBackendConfig }()
		processBackendConfig = func(backendConfig interface{}, addArg func(key, value string)) error {
			return fmt.Errorf("mock processing error")
		}

		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		_, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "error processing Kubernetes backend config: mock processing error") {
			t.Errorf("expected error to contain %v, got %v", "error processing Kubernetes backend config: mock processing error", err.Error())
		}
	})

	t.Run("UnsupportedBackendType", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Terraform: &terraform.TerraformConfig{
					Backend: &terraform.BackendConfig{
						Type: "unsupported",
					},
				},
			}
		}

		// Mock GetString to return "unsupported" for the backend type
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "unsupported"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		projectPath := "project/path"
		configRoot := filepath.FromSlash("/mock/config/root")

		_, err := terraformEnvPrinter.generateBackendConfigArgs(projectPath, configRoot)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "unsupported backend: unsupported") {
			t.Errorf("expected error to contain %v, got %v", "unsupported backend: unsupported", err.Error())
		}
	})
}

func TestTerraformEnv_processBackendConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		backendConfig := map[string]interface{}{
			"key1": "value1",
			"key2": true,
			"key3": 123,
			"key4": []interface{}{"item1", "item2"},
			"key5": map[string]interface{}{
				"nestedKey1": "nestedValue1",
				"nestedKey2": "nestedValue2",
			},
		}

		var args []string
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		err := processBackendConfig(backendConfig, addArg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			"key1=value1",
			"key2=true",
			"key3=123",
			"key4=item1",
			"key4=item2",
			"key5.nestedKey1=nestedValue1",
			"key5.nestedKey2=nestedValue2",
		}

		sort.Strings(args)
		sort.Strings(expectedArgs)

		if !reflect.DeepEqual(args, expectedArgs) {
			t.Errorf("expected args %v, got %v", expectedArgs, args)
		}
	})

	t.Run("ErrorUnmarshallingBackendConfig", func(t *testing.T) {
		originalYamlUnmarshal := yamlUnmarshal
		defer func() { yamlUnmarshal = originalYamlUnmarshal }()

		yamlUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mocked error")
		}

		backendConfig := map[string]interface{}{
			"key1": "value1",
		}

		var args []string
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		err := processBackendConfig(backendConfig, addArg)
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		expectedError := "mocked error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error to contain %v, got %v", expectedError, err.Error())
		}
	})
}
