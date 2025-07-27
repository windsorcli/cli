package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupTerraformEnvMocks creates and configures mock objects for Terraform environment tests.
func setupTerraformEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	// Pass the mock config handler to setupMocks
	mocks := setupMocks(t, opts...)

	// Create and register mock blueprint handler
	mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
	mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		return []blueprintv1alpha1.TerraformComponent{}
	}
	mocks.Injector.Register("blueprintHandler", mockBlueprint)

	mocks.Shims.Getwd = func() (string, error) {
		// Use platform-agnostic path
		return filepath.Join("mock", "project", "root", "terraform", "project", "path"), nil
	}

	// Smart Glob function that handles any terraform directory pattern
	mocks.Shims.Glob = func(pattern string) ([]string, error) {
		if strings.Contains(pattern, "*.tf") {
			// Extract directory from pattern and return a main.tf file in that directory
			dir := filepath.Dir(pattern)
			return []string{
				filepath.Join(dir, "main.tf"),
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
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -force-copy -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate"))),
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
		expectedErrorMessage := "error generating terraform args: error checking file: mock error checking file"
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
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -force-copy -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate"))),
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
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string, export bool) {
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
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -force-copy -backend-config="path=%s"`, filepath.Join(configRoot, ".tfstate/project/path/terraform.tfstate")),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan"),
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.Join(configRoot, ".terraform/project/path/terraform.tfplan")),
			"TF_CLI_ARGS_refresh": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s"`,
				filepath.Join(configRoot, "terraform/project/path.tfvars"),
				filepath.Join(configRoot, "terraform/project/path.tfvars.json")),
			"TF_VAR_context_path": configRoot,
			"TF_VAR_context_id":   "",
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
		terraformEnvPrinter.shims = mocks.Shims
		if err := terraformEnvPrinter.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

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

	t.Run("AzureRMBackend", func(t *testing.T) {
		// Given a TerraformEnvPrinter with AzureRM backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "azurerm")

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
  backend "azurerm" {}
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

	t.Run("NoneBackend", func(t *testing.T) {
		// Given a TerraformEnvPrinter with "none" backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "none")

		// Mock Stat and Remove to verify file deletion
		fileExists := true
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "backend_override.tf") {
				if fileExists {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		var fileRemoved bool
		mocks.Shims.Remove = func(name string) error {
			if strings.Contains(name, "backend_override.tf") {
				fileRemoved = true
				fileExists = false
				return nil
			}
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

		// Then no error should occur and the file should be removed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !fileRemoved {
			t.Error("Expected backend_override.tf to be removed")
		}
	})

	t.Run("NoneBackendFileNotExists", func(t *testing.T) {
		// Given a TerraformEnvPrinter with "none" backend configuration and no existing file
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "none")

		// Mock Stat to return file not exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "backend_override.tf") {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		}

		var fileRemoved bool
		mocks.Shims.Remove = func(name string) error {
			if strings.Contains(name, "backend_override.tf") {
				fileRemoved = true
				return nil
			}
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

		// Then no error should occur and Remove should not be called
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if fileRemoved {
			t.Error("Expected Remove to not be called when file doesn't exist")
		}
	})

	t.Run("NoneBackendRemoveError", func(t *testing.T) {
		// Given a TerraformEnvPrinter with "none" backend configuration and failing Remove
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "none")

		// Mock Stat to return file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.Contains(name, "backend_override.tf") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Mock Remove to return error
		mocks.Shims.Remove = func(name string) error {
			if strings.Contains(name, "backend_override.tf") {
				return fmt.Errorf("mock error removing file")
			}
			return nil
		}

		// When generateBackendOverrideTf is called
		err := printer.generateBackendOverrideTf()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error removing backend_override.tf") {
			t.Errorf("Expected error message to contain 'error removing backend_override.tf', got %v", err)
		}
	})

	t.Run("WithSpecificDirectory", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, mocks := setup(t)

		// Track which directory was used for finding terraform files
		var usedDirectory string
		originalGlob := mocks.Shims.Glob
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			// Extract the directory from the pattern
			if strings.Contains(pattern, "*.tf") {
				usedDirectory = filepath.Dir(pattern)
				// Return terraform files in the specified directory
				return []string{
					filepath.Join(usedDirectory, "file1.tf"),
					filepath.Join(usedDirectory, "file2.tf"),
				}, nil
			}
			return originalGlob(pattern)
		}

		// Mock WriteFile to capture the output
		var writtenData []byte
		var writtenPath string
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			writtenPath = filename
			return nil
		}

		// Specify a custom directory
		customDir := filepath.Join("custom", "terraform", "module", "path")

		// When generateBackendOverrideTf is called with a specific directory
		err := printer.generateBackendOverrideTf(customDir)

		// Then no error should occur and the custom directory should be used
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify that the custom directory was used for finding terraform files
		if !strings.Contains(usedDirectory, customDir) {
			t.Errorf("Expected directory %q to be used, but got %q", customDir, usedDirectory)
		}

		// Verify that the backend_override.tf file is written to the custom directory
		if !strings.Contains(writtenPath, customDir) {
			t.Errorf("Expected backend_override.tf to be written to %q, but got %q", customDir, writtenPath)
		}

		// Verify the content is correct
		expectedContent := `terraform {
  backend "local" {}
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=path=/mock/config/root/.tfstate/project/path/terraform.tfstate`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=path=/mock/config/root/.tfstate/mock-prefix/project/path/terraform.tfstate`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=key=mock-prefix/project/path/terraform.tfstate`,
			`-backend-config=bucket=mock-bucket`,
			`-backend-config=region=mock-region`,
			`-backend-config=secret_key=mock-secret-key`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=secret_suffix=mock-prefix-project-path`,
			`-backend-config=namespace=mock-namespace`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=path=/mock/config/root/.tfstate/mock-prefix/project/path/terraform.tfstate`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and backend.tfvars should be included
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			fmt.Sprintf(`-backend-config=%s`, filepath.ToSlash(backendTfvarsPath)),
			`-backend-config=path=/mock/config/root/.tfstate/project/path/terraform.tfstate`,
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
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and backend.tfvars should not be included
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=path=/mock/config/root/.tfstate/project/path/terraform.tfstate`,
		}

		if !reflect.DeepEqual(backendConfigArgs, expectedArgs) {
			t.Errorf("expected %v, got %v", expectedArgs, backendConfigArgs)
		}
	})

	t.Run("AzureRMBackendWithPrefix", func(t *testing.T) {
		// Given a TerraformEnvPrinter with AzureRM backend and prefix configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.SetContextValue("terraform.backend.type", "azurerm")
		mocks.ConfigHandler.SetContextValue("terraform.backend.prefix", "mock-prefix/")
		mocks.ConfigHandler.SetContextValue("terraform.backend.azurerm.storage_account_name", "mock-storage")
		mocks.ConfigHandler.SetContextValue("terraform.backend.azurerm.container_name", "mock-container")
		mocks.ConfigHandler.SetContextValue("terraform.backend.azurerm.use_azuread", true)
		projectPath := "project/path"
		configRoot := "/mock/config/root"

		// When generateBackendConfigArgs is called
		backendConfigArgs, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

		// Then no error should occur and the expected arguments should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedArgs := []string{
			`-backend-config=key=mock-prefix/project/path/terraform.tfstate`,
			`-backend-config=container_name=mock-container`,
			`-backend-config=storage_account_name=mock-storage`,
			`-backend-config=use_azuread=true`,
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
		_, err := printer.generateBackendConfigArgs(projectPath, configRoot, false)

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

func TestTerraformEnv_DependencyResolution(t *testing.T) {
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

	t.Run("ValidDependencyChain", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "vpc",
					FullPath:  "/project/.windsor/.tf_modules/vpc",
					DependsOn: []string{},
				},
				{
					Path:      "subnets",
					FullPath:  "/project/.windsor/.tf_modules/subnets",
					DependsOn: []string{"vpc"},
				},
				{
					Path:      "app",
					FullPath:  "/project/.windsor/.tf_modules/app",
					DependsOn: []string{"subnets"},
				},
			}
		}

		// Mock terraform output for dependencies
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 2 && args[1] == "output" && args[2] == "-json" {
				if strings.Contains(args[0], "vpc") {
					return `{"vpc_id": {"value": "vpc-12345"}, "subnet_cidrs": {"value": ["10.0.1.0/24", "10.0.2.0/24"]}}`, nil
				}
				if strings.Contains(args[0], "subnets") {
					return `{"subnet_ids": {"value": ["subnet-abc", "subnet-def"]}, "vpc_id": {"value": "vpc-12345"}}`, nil
				}
			}
			return "", nil
		}

		// Set up the current working directory to match the "app" component
		workingDir := filepath.Join(string(filepath.Separator), "project", "terraform", "app")
		mocks.Shims.Getwd = func() (string, error) {
			return workingDir, nil
		}

		// When getting environment variables for the "app" component
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And dependency variables should be included (only from direct dependencies)
		expectedVars := map[string]string{
			"TF_VAR_subnet_ids": "[subnet-abc subnet-def]",
			"TF_VAR_vpc_id":     "vpc-12345",
		}

		for expectedKey, expectedValue := range expectedVars {
			if actualValue, exists := envVars[expectedKey]; !exists {
				t.Errorf("Expected environment variable %s to be set", expectedKey)
			} else if actualValue != expectedValue {
				t.Errorf("Expected %s to be %s, got %s", expectedKey, expectedValue, actualValue)
			}
		}

		// Verify that transitive dependencies are NOT included directly
		// Note: With the new naming format, TF_VAR_vpc_id comes from the direct dependency "subnets"
		// The transitive "vpc" component's outputs are not directly accessible
		transitiveVars := []string{
			"TF_VAR_subnet_cidrs", // This should not be present as it's only in the transitive "vpc" dependency
		}

		for _, transitiveVar := range transitiveVars {
			if _, exists := envVars[transitiveVar]; exists {
				t.Errorf("Expected transitive dependency variable %s to NOT be set directly", transitiveVar)
			}
		}
	})

	t.Run("CircularDependencyDetection", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "a",
					FullPath:  "/project/.windsor/.tf_modules/a",
					DependsOn: []string{"b"},
				},
				{
					Path:      "b",
					FullPath:  "/project/.windsor/.tf_modules/b",
					DependsOn: []string{"c"},
				},
				{
					Path:      "c",
					FullPath:  "/project/.windsor/.tf_modules/c",
					DependsOn: []string{"a"},
				},
			}
		}

		// Set up the current working directory to match one of the components
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.Join(string(filepath.Separator), "project", "terraform", "a"), nil
		}

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then it should detect circular dependency
		if err == nil {
			t.Errorf("Expected error for circular dependency, but got nil")
		} else if !strings.Contains(err.Error(), "circular dependency") {
			t.Errorf("Expected error to contain 'circular dependency', got %v", err)
		}
	})

	t.Run("NonExistentDependency", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "app",
					FullPath:  "/project/.windsor/.tf_modules/app",
					DependsOn: []string{"nonexistent"},
				},
			}
		}

		// Set up the current working directory to match the component
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.Join(string(filepath.Separator), "project", "terraform", "app"), nil
		}

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then it should detect missing dependency
		if err == nil {
			t.Errorf("Expected error for non-existent dependency, but got nil")
		} else if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Expected error to contain 'does not exist', got %v", err)
		}
	})

	t.Run("ComponentsWithoutNames", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "vpc/main",
					FullPath:  "/project/.windsor/.tf_modules/vpc/main",
					DependsOn: []string{},
				},
				{
					Path:      "app/frontend",
					FullPath:  "/project/.windsor/.tf_modules/app/frontend",
					DependsOn: []string{"vpc/main"},
				},
			}
		}

		// Mock terraform output
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 2 && args[1] == "output" && args[2] == "-json" {
				return `{"vpc_id": {"value": "vpc-12345"}}`, nil
			}
			return "", nil
		}

		// Set up the current working directory to match the dependent component
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.Join(string(filepath.Separator), "project", "terraform", "app", "frontend"), nil
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And dependency variable should be included
		if actualValue, exists := envVars["TF_VAR_vpc_id"]; !exists {
			t.Errorf("Expected environment variable TF_VAR_vpc_id to be set")
		} else if actualValue != "vpc-12345" {
			t.Errorf("Expected TF_VAR_vpc_id to be vpc-12345, got %s", actualValue)
		}
	})

	t.Run("EmptyTerraformOutput", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "base",
					FullPath:  "/project/.windsor/.tf_modules/base",
					DependsOn: []string{},
				},
				{
					Path:      "app",
					FullPath:  "/project/.windsor/.tf_modules/app",
					DependsOn: []string{"base"},
				},
			}
		}

		// Mock terraform output with empty response
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 2 && args[1] == "output" && args[2] == "-json" {
				return "{}", nil
			}
			return "", nil
		}

		// Set up the current working directory to match the dependent component
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.Join(string(filepath.Separator), "project", "terraform", "app"), nil
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned even with empty output
		if err != nil {
			t.Errorf("Expected no error even with empty output, got %v", err)
		}

		// And standard terraform env vars should still be present
		if _, exists := envVars["TF_VAR_context_path"]; !exists {
			t.Errorf("Expected standard terraform environment variables to be present")
		}
	})

	t.Run("NoCurrentComponent", func(t *testing.T) {
		printer, mocks := setup(t)

		// Get the blueprint handler from the injector and configure it
		blueprintHandler := mocks.Injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		blueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:      "vpc",
					FullPath:  "/project/.windsor/.tf_modules/vpc",
					DependsOn: []string{},
				},
			}
		}

		// Set up the current working directory to not match any component
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.FromSlash("/project/terraform/nonexistent"), nil
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And no dependency variables should be added
		for key := range envVars {
			if strings.HasPrefix(key, "TF_VAR_vpc_") {
				t.Errorf("Expected no dependency variables, but found %s", key)
			}
		}
	})
}

func TestTerraformEnv_GenerateTerraformArgs(t *testing.T) {
	t.Run("GeneratesCorrectArgsWithoutParallelism", func(t *testing.T) {
		mocks := setupTerraformEnvMocks(t)

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Injector),
		}

		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		// When generating terraform args without parallelism
		args, err := printer.GenerateTerraformArgs("test/path", "test/module")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And apply args should contain only the plan file path
		if len(args.ApplyArgs) != 1 {
			t.Errorf("Expected 1 apply arg, got %d: %v", len(args.ApplyArgs), args.ApplyArgs)
		}

		// And destroy args should contain only auto-approve
		expectedDestroyArgs := []string{"-auto-approve"}
		if !reflect.DeepEqual(args.DestroyArgs, expectedDestroyArgs) {
			t.Errorf("Expected destroy args %v, got %v", expectedDestroyArgs, args.DestroyArgs)
		}

		// And environment variables should not contain parallelism
		if strings.Contains(args.TerraformVars["TF_CLI_ARGS_apply"], "parallelism") {
			t.Errorf("Apply args should not contain parallelism: %s", args.TerraformVars["TF_CLI_ARGS_apply"])
		}
		if strings.Contains(args.TerraformVars["TF_CLI_ARGS_destroy"], "parallelism") {
			t.Errorf("Destroy args should not contain parallelism: %s", args.TerraformVars["TF_CLI_ARGS_destroy"])
		}
	})

	t.Run("GeneratesCorrectArgsWithParallelism", func(t *testing.T) {
		mocks := setupTerraformEnvMocks(t)

		// Set up blueprint handler with parallelism component
		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		parallelism := 5
		mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:        "test/path",
					Parallelism: &parallelism,
				},
			}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Injector),
		}

		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		// When generating terraform args with parallelism
		args, err := printer.GenerateTerraformArgs("test/path", "test/module")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And apply args should contain parallelism flag before plan file
		expectedApplyArgs := []string{"-parallelism=5"}
		if len(args.ApplyArgs) < 1 || args.ApplyArgs[0] != expectedApplyArgs[0] {
			t.Errorf("Expected apply args to start with %v, got %v", expectedApplyArgs, args.ApplyArgs)
		}

		// And destroy args should contain parallelism flag
		foundParallelismInDestroy := false
		for _, arg := range args.DestroyArgs {
			if arg == "-parallelism=5" {
				foundParallelismInDestroy = true
				break
			}
		}
		if !foundParallelismInDestroy {
			t.Errorf("Expected destroy args to contain -parallelism=5, got %v", args.DestroyArgs)
		}

		// And environment variables should contain parallelism
		if !strings.Contains(args.TerraformVars["TF_CLI_ARGS_apply"], " -parallelism=5") {
			t.Errorf("Apply env var should contain parallelism: %s", args.TerraformVars["TF_CLI_ARGS_apply"])
		}
		if !strings.Contains(args.TerraformVars["TF_CLI_ARGS_destroy"], "-parallelism=5") {
			t.Errorf("Destroy env var should contain parallelism: %s", args.TerraformVars["TF_CLI_ARGS_destroy"])
		}
	})

	t.Run("ParallelismOnlyAppliedToMatchingComponent", func(t *testing.T) {
		mocks := setupTerraformEnvMocks(t)

		// Set up blueprint handler with parallelism for different component
		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		parallelism := 10
		mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:        "other/path",
					Parallelism: &parallelism,
				},
				{
					Path: "test/path",
					// No parallelism set
				},
			}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Injector),
		}

		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		// When generating terraform args for component without parallelism
		args, err := printer.GenerateTerraformArgs("test/path", "test/module")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And apply args should not contain parallelism flag
		for _, arg := range args.ApplyArgs {
			if strings.Contains(arg, "parallelism") {
				t.Errorf("Apply args should not contain parallelism for non-matching component: %v", args.ApplyArgs)
			}
		}

		// And environment variables should not contain parallelism
		if strings.Contains(args.TerraformVars["TF_CLI_ARGS_apply"], "parallelism") {
			t.Errorf("Apply env var should not contain parallelism: %s", args.TerraformVars["TF_CLI_ARGS_apply"])
		}
	})

	t.Run("HandlesNilBlueprintHandler", func(t *testing.T) {
		mocks := setupTerraformEnvMocks(t)

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Injector),
		}

		// Initialize with the base dependencies but don't fail on missing blueprint handler
		if err := printer.BaseEnvPrinter.Initialize(); err != nil {
			t.Fatalf("Failed to initialize base printer: %v", err)
		}

		// Set blueprint handler to nil explicitly
		printer.blueprintHandler = nil

		// When generating terraform args with nil blueprint handler
		args, err := printer.GenerateTerraformArgs("test/path", "test/module")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And no parallelism should be applied
		for _, arg := range args.ApplyArgs {
			if strings.Contains(arg, "parallelism") {
				t.Errorf("Apply args should not contain parallelism with nil blueprint handler: %v", args.ApplyArgs)
			}
		}
	})

	t.Run("CorrectArgumentOrdering", func(t *testing.T) {
		mocks := setupTerraformEnvMocks(t)

		// Set up blueprint handler with parallelism
		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		parallelism := 3
		mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:        "test/path",
					Parallelism: &parallelism,
				},
			}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Injector),
		}

		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		// When generating terraform args
		args, err := printer.GenerateTerraformArgs("test/path", "test/module")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And apply args should have parallelism flag before plan file path
		if len(args.ApplyArgs) < 2 {
			t.Fatalf("Expected at least 2 apply args, got %d: %v", len(args.ApplyArgs), args.ApplyArgs)
		}

		if args.ApplyArgs[0] != "-parallelism=3" {
			t.Errorf("Expected first apply arg to be -parallelism=3, got %s", args.ApplyArgs[0])
		}

		// Plan file should be last argument
		lastArg := args.ApplyArgs[len(args.ApplyArgs)-1]
		if !strings.Contains(lastArg, "terraform.tfplan") {
			t.Errorf("Expected last apply arg to contain terraform.tfplan, got %s", lastArg)
		}
	})
}
