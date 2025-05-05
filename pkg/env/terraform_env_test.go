package env

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupTerraformEnvMocks creates and configures mock objects for Terraform environment tests.
func setupTerraformEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	// Pass the mock config handler to setupMocks
	mocks := setupMocks(t, opts...)

	mocks.Shims.Getwd = func() (string, error) {
		// Use platform-agnostic path
		return filepath.Join("mock", "project", "root", "terraform", "project", "path"), nil
	}

	mocks.Shims.Glob = func(pattern string) ([]string, error) {
		if strings.Contains(pattern, "*.tf") {
			return []string{
				filepath.Join("real", "terraform", "project", "path", "file1.tf"),
				filepath.Join("real", "terraform", "project", "path", "file2.tf"),
			}, nil
		}
		return nil, nil
	}

	mocks.ConfigHandler.SetContextValue("terraform.backend.type", "local")

	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		// Convert paths to slash format for consistent comparison
		nameSlash := filepath.ToSlash(name)

		// Check for tfvars files in the expected paths
		if strings.Contains(nameSlash, "project/path.tfvars") ||
			strings.Contains(nameSlash, "project/path.tfvars.json") ||
			strings.Contains(nameSlash, "project\\path.tfvars") ||
			strings.Contains(nameSlash, "project\\path.tfvars.json") {
			return nil, nil
		}
		if strings.Contains(nameSlash, "project/path_generated.tfvars") {
			return nil, os.ErrNotExist
		}
		return nil, os.ErrNotExist
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestTerraformEnv_GetEnvVars tests the GetEnvVars method of the TerraformEnvPrinter
func TestTerraformEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new TerraformEnvPrinter with mock configuration
		printer, mocks := setup(t)

		// Mock the OS type
		osType := "unix"
		if mocks.Shims.Goos() == "windows" {
			osType = "windows"
		}

		// Get the actual config root
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path")),
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate"))),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan"))),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_VAR_context_path": filepath.ToSlash(configRoot),
			"TF_VAR_os_type":      osType,
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And environment variables should be set correctly
		for key, expectedValue := range expectedEnvVars {
			if value, exists := envVars[key]; !exists || value != expectedValue {
				t.Errorf("Expected %s to be %s, got %s", key, expectedValue, value)
			}
		}
	})

	t.Run("ErrorGettingProjectPath", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock Getwd to return an error
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("NoProjectPathFound", func(t *testing.T) {
		// Given a new TerraformEnvPrinter with no Terraform project path
		printer, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root"), nil
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And an empty map should be returned
		if envVars == nil {
			t.Errorf("Expected an empty map, got nil")
		}
		if len(envVars) != 0 {
			t.Errorf("Expected empty map, got %v", envVars)
		}
	})

	t.Run("ResetEnvVarsWhenNoProjectPathFound", func(t *testing.T) {
		// Given a new TerraformEnvPrinter with existing environment variables
		printer, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root"), nil
		}

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "TF_DATA_DIR" || key == "TF_CLI_ARGS_init" {
				return "some-value", true
			}
			return "", false
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And environment variables should be reset
		if envVars == nil {
			t.Errorf("Expected a map with reset variables, got nil")
		}
		if len(envVars) != 2 {
			t.Errorf("Expected map with 2 entries, got %v with %d entries", envVars, len(envVars))
		}
		if val, exists := envVars["TF_DATA_DIR"]; !exists || val != "" {
			t.Errorf("Expected TF_DATA_DIR to be empty string, got %v", val)
		}
		if val, exists := envVars["TF_CLI_ARGS_init"]; !exists || val != "" {
			t.Errorf("Expected TF_CLI_ARGS_init to be empty string, got %v", val)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}
		mocks := setupTerraformEnvMocks(t, &SetupOptions{
			ConfigHandler: configHandler,
		})
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		// When GetEnvVars is called
		_, err := printer.GetEnvVars()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected error message to contain 'error getting config root', got %v", err)
		}
	})

	t.Run("ErrorListingTfvarsFiles", func(t *testing.T) {
		// Given a new TerraformEnvPrinter with failing file stat
		printer, mocks := setup(t)

		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"file1.tf", "file2.tf"}, nil
			}
			return nil, nil
		}

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, fmt.Errorf("mock error checking file")
		}

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		expectedErrorMessage := "error checking file: mock error checking file"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("TestWindows", func(t *testing.T) {
		// Given a new TerraformEnvPrinter on Windows
		printer, mocks := setup(t)

		// Mock Windows OS
		mocks.Shims.Goos = func() string {
			return "windows"
		}

		// Mock filesystem operations
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.FromSlash("/mock/project/root/terraform/project/path"), nil
		}

		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{"main.tf"}, nil
			}
			return nil, nil
		}

		// Get the actual config root
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		// Mock Stat to handle both tfvars files
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			// Convert paths to slash format for consistent comparison
			nameSlash := filepath.ToSlash(name)

			// Check for tfvars files in the expected paths
			if strings.Contains(nameSlash, "project/path.tfvars") ||
				strings.Contains(nameSlash, "project/path.tfvars.json") ||
				strings.Contains(nameSlash, "project\\path.tfvars") ||
				strings.Contains(nameSlash, "project\\path.tfvars.json") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path")),
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate"))),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.ToSlash(filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan"))),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_VAR_context_path": filepath.ToSlash(configRoot),
			"TF_VAR_os_type":      "windows",
		}

		if envVars == nil {
			t.Errorf("envVars is nil, expected %v", expectedEnvVars)
		} else {
			for key, expectedValue := range expectedEnvVars {
				if value, exists := envVars[key]; !exists || value != expectedValue {
					t.Errorf("Expected %s to be %s, got %s", key, expectedValue, value)
				}
			}
		}
	})
}

func TestTerraformEnv_PostEnvHook(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, _ := setup(t)

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		printer, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("ErrorFindingProjectPath", func(t *testing.T) {
		printer, mocks := setup(t)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, fmt.Errorf("mock error finding project path")
		}

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "unsupported")

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported backend") {
			t.Errorf("Expected error message to contain 'unsupported backend', got %v", err)
		}
	})

	t.Run("ErrorWritingBackendOverrideFile", func(t *testing.T) {
		printer, mocks := setup(t)
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing backend_override.tf file")
		}

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

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
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, mocks := setup(t)

		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When Print is called
		err := printer.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Then the expected environment variables should be set
		expectedOSType := "unix"
		if mocks.Shims.Goos() == "windows" {
			expectedOSType = "windows"
		}

		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.Join(configRoot, ".terraform/project/path"),
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -backend-config="path=%s"`, filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate")),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan"),
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan")),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_VAR_context_path": configRoot,
			"TF_VAR_os_type":      expectedOSType,
		}

		for k, v := range expectedEnvVars {
			expectedEnvVars[k] = filepath.ToSlash(v)
		}
		for k, v := range capturedEnvVars {
			capturedEnvVars[k] = filepath.ToSlash(v)
		}

		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Given a TerraformEnvPrinter with a failing config handler
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock config error")
		}
		mocks := setupTerraformEnvMocks(t, &SetupOptions{
			ConfigHandler: configHandler,
		})
		terraformEnvPrinter := NewTerraformEnvPrinter(mocks.Injector)
		terraformEnvPrinter.Initialize()

		// When Print is called
		err := terraformEnvPrinter.Print()

		// Then an error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestTerraformEnv_findRelativeTerraformProjectPath(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, _ := setup(t)

		// When findRelativeTerraformProjectPath is called
		projectPath, err := printer.findRelativeTerraformProjectPath()

		// Then no error should occur and the expected project path should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedPath := "project/path"
		if projectPath != expectedPath {
			t.Errorf("Expected project path %v, got %v", expectedPath, projectPath)
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		// Given a TerraformEnvPrinter with no Terraform files
		printer, mocks := setup(t)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When findRelativeTerraformProjectPath is called
		projectPath, err := printer.findRelativeTerraformProjectPath()

		// Then no error should occur and the project path should be empty
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if projectPath != "" {
			t.Errorf("Expected empty project path, got %v", projectPath)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a TerraformEnvPrinter with a failing Getwd function
		printer, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// When findRelativeTerraformProjectPath is called
		_, err := printer.findRelativeTerraformProjectPath()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Errorf("Expected error message to contain 'error getting current directory', got %v", err)
		}
	})

	t.Run("NoTerraformDirectoryFound", func(t *testing.T) {
		// Given a TerraformEnvPrinter with no Terraform directory
		printer, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.FromSlash("/mock/path/to/project"), nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.FromSlash("/mock/path/to/project/*.tf") {
				return []string{filepath.FromSlash("/mock/path/to/project/main.tf")}, nil
			}
			return nil, nil
		}

		// When findRelativeTerraformProjectPath is called
		projectPath, err := printer.findRelativeTerraformProjectPath()

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
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, mocks := setup(t)

		// Mock WriteFile to capture the output
		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

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
		// Given a TerraformEnvPrinter with S3 backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "s3")

		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

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
		// Given a TerraformEnvPrinter with Kubernetes backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "kubernetes")

		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

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
		// Given a TerraformEnvPrinter with unsupported backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "unsupported")

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported backend: unsupported") {
			t.Errorf("Expected error message to contain 'unsupported backend: unsupported', got %v", err)
		}
	})

	t.Run("NoTerraformFiles", func(t *testing.T) {
		// Given a TerraformEnvPrinter with no Terraform files
		printer, mocks := setup(t)
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestTerraformEnv_generateBackendConfigArgs(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, _ := setup(t)
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="path=/mock/config/root/.tfstate/project/path/terraform.tfstate"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("LocalBackendWithPrefix", func(t *testing.T) {
		// Given a TerraformEnvPrinter with local backend and prefix configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.prefix", "mock-prefix/")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="path=/mock/config/root/.tfstate/mock-prefix/project/path/terraform.tfstate"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("S3BackendWithPrefix", func(t *testing.T) {
		// Given a TerraformEnvPrinter with S3 backend and prefix configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "s3")
		mocks.ConfigHandler.SetContextValue("terraform.backend.prefix", "mock-prefix/")
		mocks.ConfigHandler.SetContextValue("terraform.backend.s3.bucket", "mock-bucket")
		mocks.ConfigHandler.SetContextValue("terraform.backend.s3.region", "mock-region")
		mocks.ConfigHandler.SetContextValue("terraform.backend.s3.secret_key", "mock-secret-key")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="key=mock-prefix/project/path/terraform.tfstate"`,
			`-backend-config="bucket=mock-bucket"`,
			`-backend-config="region=mock-region"`,
			`-backend-config="secret_key=mock-secret-key"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("KubernetesBackendWithPrefix", func(t *testing.T) {
		// Given a TerraformEnvPrinter with Kubernetes backend and prefix configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "kubernetes")
		mocks.ConfigHandler.SetContextValue("terraform.backend.prefix", "mock-prefix")
		mocks.ConfigHandler.SetContextValue("terraform.backend.kubernetes.namespace", "mock-namespace")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="secret_suffix=mock-prefix-project-path"`,
			`-backend-config="namespace=mock-namespace"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("BackendTfvarsFileExistsWithPrefix", func(t *testing.T) {
		// Given a TerraformEnvPrinter with backend tfvars file and prefix configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.prefix", "mock-prefix/")
		mocks.ConfigHandler.SetContextValue("context", "mock-context")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="path=/mock/config/root/.tfstate/mock-prefix/project/path/terraform.tfstate"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("BackendTfvarsFileExists", func(t *testing.T) {
		// Given a TerraformEnvPrinter with a backend.tfvars file
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("context", "mock-context")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// Mock Stat to return nil error for backend.tfvars
		backendTfvarsPath := filepath.Join(configRoot, "terraform", "backend.tfvars")
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == backendTfvarsPath {
				return nil, nil
			}
			return nil, fmt.Errorf("unexpected path: %s", path)
		}

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and backend.tfvars should be included
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config="%s"`, filepath.ToSlash(backendTfvarsPath)),
			`-backend-config="path=/mock/config/root/.tfstate/project/path/terraform.tfstate"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("BackendTfvarsFileDoesNotExist", func(t *testing.T) {
		// Given a TerraformEnvPrinter without a backend.tfvars file
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("context", "mock-context")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// Mock Stat to return error for backend.tfvars
		backendTfvarsPath := filepath.Join(configRoot, "terraform", "backend.tfvars")
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == backendTfvarsPath {
				return nil, fmt.Errorf("file not found")
			}
			return nil, fmt.Errorf("unexpected path: %s", path)
		}

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then no error should occur and backend.tfvars should not be included
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config="path=/mock/config/root/.tfstate/project/path/terraform.tfstate"`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("UnsupportedBackendType", func(t *testing.T) {
		// Given a TerraformEnvPrinter with unsupported backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "unsupported")
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		_, err := printer.generateBackendConfigArgs(projectPath, configRoot)

		// Then an error should be returned
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "unsupported backend: unsupported") {
			t.Errorf("expected error to contain %v, got %v", "unsupported backend: unsupported", err.Error())
		}
	})
}

func TestTerraformEnv_processBackendConfig(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with valid backend configuration
		printer, _ := setup(t)

		backendConfig := map[string]any{
			"key1": "value1",
			"key2": true,
			"key3": 123,
			"key4": []any{"item1", "item2"},
			"key5": map[string]any{
				"nestedKey1": "nestedValue1",
				"nestedKey2": "nestedValue2",
			},
		}

		var args []string
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		// When processing the backend configuration
		err := printer.processBackendConfig(backendConfig, addArg)

		// Then no error should occur
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And all configuration values should be properly formatted
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
		// Given a TerraformEnvPrinter with failing YAML unmarshalling
		printer, mocks := setup(t)

		mocks.Shims.YamlMarshal = func(v any) ([]byte, error) {
			return []byte("valid yaml"), nil
		}
		mocks.Shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("mock unmarshal error")
		}

		var args []string
		addArg := func(key, value string) {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}

		// When processing the backend configuration
		err := printer.processBackendConfig(map[string]any{"key1": "value1"}, addArg)

		// Then an error should be returned
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		// And the error should contain the expected message
		expectedError := "error unmarshalling backend YAML: mock unmarshal error"
		if err.Error() != expectedError {
			t.Errorf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestTerraformEnv_getTerraformOutputs(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := NewTerraformEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock terraform output
		printer, mocks := setup(t)
		projectPath := "project/path"

		// Mock terraform output command
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", `{
				"instance_id": {"value": "i-123456", "type": "string"},
				"port": {"value": 8080, "type": "number"},
				"tags": {"value": ["prod", "web"], "type": "list"}
			}`)
			return cmd
		}

		// When getTerraformOutputs is called
		outputs, err := printer.getTerraformOutputs(projectPath)

		// Then no error should occur and outputs should be correctly formatted
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedOutputs := map[string]string{
			"TF_VAR_instance_id": "i-123456",
			"TF_VAR_port":        "8080",
			"TF_VAR_tags":        "prod,web",
		}

		if !reflect.DeepEqual(outputs, expectedOutputs) {
			t.Errorf("Expected outputs %v, got %v", expectedOutputs, outputs)
		}
	})

	t.Run("CommandError", func(t *testing.T) {
		// Given a TerraformEnvPrinter with failing terraform command
		printer, mocks := setup(t)
		projectPath := "project/path"

		// Mock failing terraform command
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("false")
			return cmd
		}

		// When getTerraformOutputs is called
		_, err := printer.getTerraformOutputs(projectPath)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to run terraform output") {
			t.Errorf("Expected error message to contain 'failed to run terraform output', got %v", err)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		// Given a TerraformEnvPrinter with invalid JSON output
		printer, mocks := setup(t)
		projectPath := "project/path"

		// Mock terraform command with invalid JSON
		mocks.Shims.Command = func(name string, arg ...string) *exec.Cmd {
			cmd := exec.Command("echo", `invalid json`)
			return cmd
		}

		// When getTerraformOutputs is called
		_, err := printer.getTerraformOutputs(projectPath)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse terraform output") {
			t.Errorf("Expected error message to contain 'failed to parse terraform output', got %v", err)
		}
	})
}

func TestTerraformEnv_formatTerraformValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "String value",
			input:    "test-string",
			expected: "test-string",
		},
		{
			name:     "Integer value",
			input:    42,
			expected: "42",
		},
		{
			name:     "Float value",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "Boolean value",
			input:    true,
			expected: "true",
		},
		{
			name:     "String array",
			input:    []interface{}{"item1", "item2", "item3"},
			expected: "item1,item2,item3",
		},
		{
			name:     "Mixed array",
			input:    []interface{}{"item1", 42, true},
			expected: "item1,42,true",
		},
		{
			name:     "Map value",
			input:    map[string]interface{}{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "Nil value",
			input:    nil,
			expected: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When formatTerraformValue is called
			result := formatTerraformValue(tt.input)

			// Then the result should match the expected output
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
