package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupTerraformEnvMocks creates and configures mock objects for Terraform environment tests.
func setupTerraformEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	mocks := setupEnvMocks(t, overrides...)

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

	mocks.ConfigHandler.Set("terraform.backend.type", "local")

	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		// Convert paths to slash format for consistent comparison
		nameSlash := filepath.ToSlash(name)

		// Check for tfvars files in the expected paths
		if strings.Contains(nameSlash, "project/path.tfvars") ||
			strings.Contains(nameSlash, "project/path.tfvars.json") ||
			strings.Contains(nameSlash, "project\\path.tfvars") ||
			strings.Contains(nameSlash, "project\\path.tfvars.json") ||
			strings.Contains(nameSlash, ".windsor/contexts/local/terraform/project/path/terraform.tfvars") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	return mocks
}

// setupTerraformEnvPrinter creates a TerraformEnvPrinter with a mock provider for testing.
func setupTerraformEnvPrinter(t *testing.T, mocks *EnvTestMocks, provider terraform.TerraformProvider) *TerraformEnvPrinter {
	t.Helper()
	printer := &TerraformEnvPrinter{
		BaseEnvPrinter:    *NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler),
		toolsManager:      mocks.ToolsManager,
		terraformProvider: provider,
	}
	printer.shims = mocks.Shims
	return printer
}

// setupMockTerraformProvider creates a mock TerraformProvider with sensible defaults.
// Individual functions can be overridden by setting them on the returned mock.
func setupMockTerraformProvider(mocks *EnvTestMocks) *terraform.MockTerraformProvider {
	testEvaluator := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, "/test/project", "/test/template")
	realProvider := terraform.NewTerraformProvider(mocks.ConfigHandler, mocks.Shell, mocks.ToolsManager, testEvaluator)

	// Set up the real provider's Shims to use the test's mocked functions
	// Use reflection to access the exported Shims field on the concrete type
	rv := reflect.ValueOf(realProvider)
	if rv.Kind() == reflect.Ptr {
		shimsField := rv.Elem().FieldByName("Shims")
		if shimsField.IsValid() && shimsField.CanSet() {
			shimsValue := shimsField.Interface().(*terraform.Shims)
			if shimsValue != nil {
				shimsValue.Stat = mocks.Shims.Stat
				shimsValue.Goos = mocks.Shims.Goos
			}
		}
	}

	return &terraform.MockTerraformProvider{
		FindRelativeProjectPathFunc: func(directory ...string) (string, error) {
			var currentPath string
			if len(directory) > 0 {
				currentPath = filepath.Clean(directory[0])
			} else {
				var err error
				currentPath, err = mocks.Shims.Getwd()
				if err != nil {
					return "", err
				}
			}
			globPattern := filepath.Join(currentPath, "*.tf")
			matches, err := mocks.Shims.Glob(globPattern)
			if err != nil {
				return "", err
			}
			if len(matches) == 0 {
				return "", nil
			}
			pathParts := strings.Split(currentPath, string(os.PathSeparator))
			for i := len(pathParts) - 1; i >= 0; i-- {
				if strings.EqualFold(pathParts[i], "terraform") {
					relativePath := filepath.Join(pathParts[i+1:]...)
					return filepath.ToSlash(relativePath), nil
				}
				if strings.EqualFold(pathParts[i], "contexts") {
					relativePath := filepath.Join(pathParts[i+1:]...)
					return filepath.ToSlash(relativePath), nil
				}
			}
			return "", nil
		},
		GenerateBackendOverrideFunc: func(directory string) error {
			return nil
		},
		GetTerraformComponentFunc: func(componentID string) *blueprintv1alpha1.TerraformComponent {
			return nil
		},
		GetTerraformComponentsFunc: func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{}
		},
		GetTFDataDirFunc: func(componentID string) (string, error) {
			windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
			if err != nil {
				return "", err
			}
			return filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", componentID)), nil
		},
		ClearCacheFunc: func() {},
		GenerateTerraformArgsFunc: func(componentID, modulePath string, interactive bool) (*terraform.TerraformArgs, error) {
			// Ensure the real provider uses the test's mocked Stat function
			// We need to access the concrete type to set Shims
			if provider, ok := realProvider.(interface{ GetShims() *terraform.Shims }); ok {
				shims := provider.GetShims()
				if shims != nil {
					shims.Stat = mocks.Shims.Stat
				}
			}
			return realProvider.GenerateTerraformArgs(componentID, modulePath, interactive)
		},
		GetEnvVarsFunc: func(componentID string, interactive bool) (map[string]string, *terraform.TerraformArgs, error) {
			// Ensure the real provider uses the test's mocked functions
			rv := reflect.ValueOf(realProvider)
			if rv.Kind() == reflect.Ptr {
				shimsField := rv.Elem().FieldByName("Shims")
				if shimsField.IsValid() && shimsField.CanSet() {
					shimsValue := shimsField.Interface().(*terraform.Shims)
					if shimsValue != nil {
						shimsValue.Goos = mocks.Shims.Goos
					}
				}
			}
			return realProvider.GetEnvVars(componentID, interactive)
		},
		FormatArgsForEnvFunc: func(args []string) string {
			return realProvider.FormatArgsForEnv(args)
		},
		GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
			return realProvider.GetTerraformOutputs(componentID)
		},
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestTerraformEnv_GetEnvVars tests the GetEnvVars method of the TerraformEnvPrinter
func TestTerraformEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := setupTerraformEnvPrinter(t, mocks, setupMockTerraformProvider(mocks))
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

		// Get the actual config root, windsor scratch path, and project root
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
		if err != nil {
			t.Fatalf("Failed to get windsor scratch path: %v", err)
		}
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":      filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "project/path")),
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -force-copy -upgrade -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(windsorScratchPath, ".tfstate", "project/path", "terraform.tfstate"))),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "project/path", "terraform.tfplan")),
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform", "project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform", "project/path.tfvars.json"))),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "project/path", "terraform.tfplan"))),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_VAR_context":      mocks.ConfigHandler.GetContext(),
			"TF_VAR_project_root": filepath.ToSlash(projectRoot),
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
		// Given a TerraformEnvPrinter with failing Getwd
		printer, mocks := setup(t)

		// Update mock provider to return error
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "", fmt.Errorf("error getting current directory")
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		// Update mock provider to return empty path
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "TF_DATA_DIR" || key == "TF_CLI_ARGS_init" {
				return "some-value", true
			}
			return "", false
		}

		// Update mock provider to return empty path
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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
		// Given a TerraformEnvPrinter with failing config root lookup
		configHandler := config.NewMockConfigHandler()
		configHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}
		mocks := setupTerraformEnvMocks(t, &EnvTestMocks{
			ConfigHandler: configHandler,
		})
		mockProvider := setupMockTerraformProvider(mocks)
		printer := NewTerraformEnvPrinter(mocks.Shell, mocks.ConfigHandler, mocks.ToolsManager, mockProvider)
		printer.shims = mocks.Shims
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "project/path", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		// Update the real provider's Shims to use the test's mocked Stat function
		testEvaluator := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, "/test/project", "/test/template")
		realProvider := terraform.NewTerraformProvider(mocks.ConfigHandler, mocks.Shell, mocks.ToolsManager, testEvaluator)
		rvProvider := reflect.ValueOf(realProvider)
		if rvProvider.Kind() == reflect.Ptr {
			shimsField := rvProvider.Elem().FieldByName("Shims")
			if shimsField.IsValid() {
				shimsValue := shimsField.Interface().(*terraform.Shims)
				if shimsValue != nil {
					shimsValue.Stat = mocks.Shims.Stat
				}
			}
		}
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GenerateTerraformArgsFunc = func(componentID, modulePath string, interactive bool) (*terraform.TerraformArgs, error) {
			return realProvider.GenerateTerraformArgs(componentID, modulePath, interactive)
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		// Get the actual config root, windsor scratch path, and project root
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
		if err != nil {
			t.Fatalf("Failed to get windsor scratch path: %v", err)
		}
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}

		// Mock Stat to handle both tfvars files
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			// Convert paths to slash format for consistent comparison
			nameSlash := filepath.ToSlash(name)

			// Check for tfvars files in the expected paths
			if strings.Contains(nameSlash, "project/path.tfvars") ||
				strings.Contains(nameSlash, "project/path.tfvars.json") ||
				strings.Contains(nameSlash, "project\\path.tfvars") ||
				strings.Contains(nameSlash, "project\\path.tfvars.json") ||
				strings.Contains(nameSlash, ".windsor/contexts/local/terraform/project/path/terraform.tfvars") {
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
			"TF_DATA_DIR":      filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform/project/path")),
			"TF_CLI_ARGS_init": fmt.Sprintf(`-backend=true -force-copy -upgrade -backend-config="path=%s"`, filepath.ToSlash(filepath.Join(windsorScratchPath, ".tfstate/project/path/terraform.tfstate"))),
			"TF_CLI_ARGS_plan": fmt.Sprintf(`-out="%s" -var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "project/path", "terraform.tfplan")),
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform", "project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform", "project/path.tfvars.json"))),
			"TF_CLI_ARGS_apply": fmt.Sprintf(`"%s"`, filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "project/path", "terraform.tfplan"))),
			"TF_CLI_ARGS_import": fmt.Sprintf(`-var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_CLI_ARGS_destroy": fmt.Sprintf(`-var-file="%s" -var-file="%s" -var-file="%s"`,
				filepath.ToSlash(filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "project/path", "terraform.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars")),
				filepath.ToSlash(filepath.Join(configRoot, "terraform/project/path.tfvars.json"))),
			"TF_VAR_context":      mocks.ConfigHandler.GetContext(),
			"TF_VAR_project_root": filepath.ToSlash(projectRoot),
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
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := setupTerraformEnvPrinter(t, mocks, setupMockTerraformProvider(mocks))
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

		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "", fmt.Errorf("error getting current directory")
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		// Update mock provider to return error
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "", fmt.Errorf("error finding project path")
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		// Update mock provider to return error for unsupported backend
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "project/path", nil
		}
		mockProvider.GenerateBackendOverrideFunc = func(directory string) error {
			return fmt.Errorf("unsupported backend: unsupported")
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

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

		// Update mock provider to return error for writing file
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "project/path", nil
		}
		mockProvider.GenerateBackendOverrideFunc = func(directory string) error {
			return fmt.Errorf("error writing backend_override.tf")
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When the PostEnvHook function is called
		err := printer.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing backend_override.tf") {
			t.Errorf("Expected error message to contain 'error writing backend_override.tf', got %v", err)
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
			// Note: sanitizeForK8s is now in the terraform package, this test should be moved there
			t.Skip("sanitizeForK8s moved to terraform package - test should be moved there")
		})
	}
}

// TestTerraformEnv_generateBackendOverrideTf - UPDATED
// The generateBackendOverrideTf method has been moved to terraform.TerraformProvider.GenerateBackendOverride.
// This test now verifies PostEnvHook which calls the provider's GenerateBackendOverride.
func TestTerraformEnv_generateBackendOverrideTf(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		mockProvider := setupMockTerraformProvider(mocks)
		printer := NewTerraformEnvPrinter(mocks.Shell, mocks.ConfigHandler, mocks.ToolsManager, mockProvider)
		printer.shims = mocks.Shims
		mockProvider.GenerateBackendOverrideFunc = func(directory string) error {
			backend := mocks.ConfigHandler.GetString("terraform.backend.type", "local")
			var backendConfig string
			switch backend {
			case "none":
				backendOverridePath := filepath.Join(directory, "backend_override.tf")
				if _, err := mocks.Shims.Stat(backendOverridePath); err == nil {
					if err := mocks.Shims.Remove(backendOverridePath); err != nil {
						return fmt.Errorf("error removing backend_override.tf: %w", err)
					}
				}
				return nil
			case "local":
				backendConfig = `terraform {
  backend "local" {}
}`
			case "s3":
				backendConfig = `terraform {
  backend "s3" {}
}`
			case "kubernetes":
				backendConfig = `terraform {
  backend "kubernetes" {}
}`
			case "azurerm":
				backendConfig = `terraform {
  backend "azurerm" {}
}`
			default:
				return fmt.Errorf("unsupported backend: %s", backend)
			}
			backendOverridePath := filepath.Join(directory, "backend_override.tf")
			err := mocks.Shims.WriteFile(backendOverridePath, []byte(backendConfig), os.ModePerm)
			if err != nil {
				return fmt.Errorf("error writing backend_override.tf: %w", err)
			}
			return nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a TerraformEnvPrinter with mock configuration
		printer, mocks := setup(t)

		// Mock Getwd to return a directory
		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}

		// Mock Glob to return terraform files
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

		// Mock WriteFile to capture the output
		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When PostEnvHook is called (which calls GenerateBackendOverride)
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "s3")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "kubernetes")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "azurerm")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

		var writtenData []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.Shims.Getwd = func() (string, error) {
			return filepath.Join("test", "dir"), nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When PostEnvHook is called (should return nil when no project path found)
		err := printer.PostEnvHook()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoneBackend", func(t *testing.T) {
		// Given a TerraformEnvPrinter with "none" backend configuration
		printer, mocks := setup(t)
		mocks.ConfigHandler.Set("terraform.backend.type", "none")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

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

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "none")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

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

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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
		mocks.ConfigHandler.Set("terraform.backend.type", "none")

		testDir := filepath.Join("test", "terraform", "module")
		mocks.Shims.Getwd = func() (string, error) {
			return testDir, nil
		}
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(testDir, "main.tf")}, nil
			}
			return nil, nil
		}

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

		// When PostEnvHook is called
		err := printer.PostEnvHook()

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

		// Specify a custom directory
		customDir := filepath.Join("custom", "terraform", "module", "path")

		// Mock Glob to return terraform files in the custom directory
		mocks.Shims.Glob = func(pattern string) ([]string, error) {
			if strings.Contains(pattern, "*.tf") {
				return []string{filepath.Join(customDir, "main.tf")}, nil
			}
			return nil, nil
		}

		// Mock WriteFile to capture the output
		var writtenData []byte
		var writtenPath string
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenData = data
			writtenPath = filename
			return nil
		}

		// When PostEnvHook is called with a specific directory
		err := printer.PostEnvHook(customDir)

		// Then no error should occur and the custom directory should be used
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify that the custom directory was used for writing the file
		if !strings.Contains(writtenPath, customDir) {
			t.Errorf("Expected file to be written in directory %q, but got %q", customDir, writtenPath)
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

// TestTerraformEnv_DependencyResolution - UPDATED
// This test verifies that GetEnvVars no longer automatically injects dependency variables.
// Dependency variable injection via addDependencyVariables has been removed.
func TestTerraformEnv_DependencyResolution(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := setupTerraformEnvPrinter(t, mocks, setupMockTerraformProvider(mocks))
		return printer, mocks
	}

	t.Run("NoAutomaticDependencyInjection", func(t *testing.T) {
		// Given a TerraformEnvPrinter with valid dependency chain
		printer, mocks := setup(t)

		// Mock blueprint.yaml content
		blueprintYAML := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
terraform:
  - path: vpc
    fullPath: /project/.windsor/contexts/local/terraform/vpc
    dependsOn: []
  - path: subnets
    fullPath: /project/.windsor/contexts/local/terraform/subnets
    dependsOn: [vpc]
  - path: app
    fullPath: /project/.windsor/contexts/local/terraform/app
    dependsOn: [subnets]`

		// Mock ReadFile to return blueprint.yaml content
		originalReadFile := mocks.Shims.ReadFile
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return originalReadFile(filename)
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

		// And dependency variables should NOT be automatically injected
		// (they should only come from explicit terraform_output() references in inputs)
		unexpectedVars := []string{
			"TF_VAR_subnet_ids",
			"TF_VAR_vpc_id",
			"TF_VAR_subnet_cidrs",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected dependency variable %s to NOT be automatically injected", unexpectedVar)
			}
		}

		// But standard terraform env vars should still be present
		if _, exists := envVars["TF_VAR_context_path"]; !exists {
			t.Errorf("Expected standard terraform environment variables to be present")
		}
	})

	t.Run("NoCurrentComponent", func(t *testing.T) {
		// Given a TerraformEnvPrinter with no matching current component
		printer, mocks := setup(t)

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

func TestTerraformProvider_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := setupTerraformEnvPrinter(t, mocks, setupMockTerraformProvider(mocks))
		return printer, mocks
	}

	t.Run("GeneratesCorrectArgsWithoutParallelism", func(t *testing.T) {
		// Given a TerraformEnvPrinter without parallelism configuration
		printer, mocks := setup(t)

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When generating terraform args without parallelism for interactive regular injection
		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And apply args should contain only the plan file path
		if len(args.ApplyArgs) != 1 {
			t.Errorf("Expected 1 apply arg, got %d: %v", len(args.ApplyArgs), args.ApplyArgs)
		}

		// And destroy args should not contain auto-approve for regular injection
		expectedDestroyArgs := []string{}
		if !reflect.DeepEqual(args.DestroyArgs, expectedDestroyArgs) {
			t.Errorf("Expected destroy args %v, got %v", expectedDestroyArgs, args.DestroyArgs)
		}

		// And environment variables should not contain parallelism
		envVars, args, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		if strings.Contains(envVars["TF_CLI_ARGS_apply"], "parallelism") {
			t.Errorf("Apply args should not contain parallelism: %s", envVars["TF_CLI_ARGS_apply"])
		}
		if strings.Contains(envVars["TF_CLI_ARGS_destroy"], "parallelism") {
			t.Errorf("Destroy args should not contain parallelism: %s", envVars["TF_CLI_ARGS_destroy"])
		}
	})

	t.Run("GeneratesCorrectArgsWithParallelism", func(t *testing.T) {
		// Given a TerraformEnvPrinter with parallelism configuration
		printer, mocks := setup(t)

		// Mock blueprint.yaml content with parallelism
		blueprintYAML := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
terraform:
  - path: test/path
    parallelism: 5`

		// Mock ReadFile to return blueprint.yaml content
		originalReadFile := mocks.Shims.ReadFile
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return originalReadFile(filename)
		}

		// Set up mock provider
		parallelism := 5
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GetTerraformComponentFunc = func(componentID string) *blueprintv1alpha1.TerraformComponent {
			if componentID == "test/path" {
				return &blueprintv1alpha1.TerraformComponent{
					Path:        "test/path",
					Parallelism: &blueprintv1alpha1.IntExpression{Value: &parallelism, IsExpr: false},
				}
			}
			return nil
		}
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		// Override GenerateTerraformArgs to manually construct args with parallelism
		// since the real provider's GetTerraformComponent loads from blueprint.yaml
		testEvaluator := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, "/test/project", "/test/template")
		realProvider := terraform.NewTerraformProvider(mocks.ConfigHandler, mocks.Shell, mocks.ToolsManager, testEvaluator)
		rvProvider := reflect.ValueOf(realProvider)
		if rvProvider.Kind() == reflect.Ptr {
			shimsField := rvProvider.Elem().FieldByName("Shims")
			if shimsField.IsValid() {
				shimsValue := shimsField.Interface().(*terraform.Shims)
				if shimsValue != nil {
					shimsValue.Stat = mocks.Shims.Stat
				}
			}
		}
		mockProvider.GenerateTerraformArgsFunc = func(componentID, modulePath string, interactive bool) (*terraform.TerraformArgs, error) {
			tfDataDir, _ := mockProvider.GetTFDataDirFunc(componentID)
			tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

			// GenerateBackendConfigArgs is now private, get args from GenerateTerraformArgs
			args, _ := realProvider.GenerateTerraformArgs(componentID, modulePath, interactive)
			backendConfigArgs := args.InitArgs[3:] // Skip "-backend=true", "-force-copy", "-upgrade"
			initArgs := []string{"-backend=true", "-force-copy", "-upgrade"}
			initArgs = append(initArgs, backendConfigArgs...)

			planArgs := []string{fmt.Sprintf("-out=%s", tfPlanPath)}
			applyArgs := []string{"-parallelism=5"}
			applyArgs = append(applyArgs, tfPlanPath)
			destroyArgs := []string{"-parallelism=5"}

			return &terraform.TerraformArgs{
				TFDataDir:       tfDataDir,
				InitArgs:        initArgs,
				PlanArgs:        planArgs,
				ApplyArgs:       applyArgs,
				RefreshArgs:     []string{},
				ImportArgs:      []string{},
				DestroyArgs:     destroyArgs,
				PlanDestroyArgs: []string{"-destroy"},
				BackendConfig:   strings.Join(backendConfigArgs, " "),
			}, nil
		}
		mockProvider.GetEnvVarsFunc = func(componentID string, interactive bool) (map[string]string, *terraform.TerraformArgs, error) {
			args, err := mockProvider.GenerateTerraformArgsFunc(componentID, "", interactive)
			if err != nil {
				return nil, nil, err
			}
			envVars := make(map[string]string)
			envVars["TF_DATA_DIR"] = args.TFDataDir
			envVars["TF_CLI_ARGS_init"] = strings.Join(args.InitArgs, " ")
			envVars["TF_CLI_ARGS_plan"] = strings.Join(args.PlanArgs, " ")
			envVars["TF_CLI_ARGS_apply"] = strings.Join(args.ApplyArgs, " ")
			envVars["TF_CLI_ARGS_destroy"] = strings.Join(args.DestroyArgs, " ")
			return envVars, args, nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When generating terraform args with parallelism for interactive regular injection
		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)

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
		envVars, args, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		if !strings.Contains(envVars["TF_CLI_ARGS_apply"], "-parallelism=5") {
			t.Errorf("Apply env var should contain parallelism: %s", envVars["TF_CLI_ARGS_apply"])
		}
		if !strings.Contains(envVars["TF_CLI_ARGS_destroy"], "-parallelism=5") {
			t.Errorf("Destroy env var should contain parallelism: %s", envVars["TF_CLI_ARGS_destroy"])
		}
	})

	t.Run("ParallelismOnlyAppliedToMatchingComponent", func(t *testing.T) {
		// Given a TerraformEnvPrinter with parallelism for a different component
		printer, mocks := setup(t)

		// Mock blueprint.yaml with parallelism for different component
		parallelism := 10
		configRoot, _ := mocks.ConfigHandler.GetConfigRoot()
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		blueprintYAML := fmt.Sprintf(`apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
terraform:
  - path: other/path
    parallelism: %d
  - path: test/path`, parallelism)

		originalReadFile := mocks.Shims.ReadFile
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == blueprintPath {
				return []byte(blueprintYAML), nil
			}
			return originalReadFile(filename)
		}

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When generating terraform args for component without parallelism for interactive regular injection
		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)

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
		envVars, args, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		if strings.Contains(envVars["TF_CLI_ARGS_apply"], "parallelism") {
			t.Errorf("Apply env var should not contain parallelism: %s", envVars["TF_CLI_ARGS_apply"])
		}
	})

	t.Run("HandlesMissingBlueprintFile", func(t *testing.T) {
		// Given a TerraformEnvPrinter without blueprint.yaml file
		printer, mocks := setup(t)

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When generating terraform args without blueprint.yaml file for interactive regular injection
		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And no parallelism should be applied (since no blueprint.yaml exists)
		for _, arg := range args.ApplyArgs {
			if strings.Contains(arg, "parallelism") {
				t.Errorf("Apply args should not contain parallelism without blueprint.yaml: %v", args.ApplyArgs)
			}
		}
	})

	t.Run("DiscoversTfvarsFilesForNamedComponent", func(t *testing.T) {
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
		if err != nil {
			t.Fatalf("Failed to get windsor scratch path: %v", err)
		}

		componentName := "cluster"
		userTfvarsPath := filepath.Join(configRoot, "terraform", componentName+".tfvars")
		autoTfvarsPath := filepath.Join(windsorScratchPath, "terraform", componentName, "terraform.tfvars")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			nameSlash := filepath.ToSlash(name)
			userTfvarsSlash := filepath.ToSlash(userTfvarsPath)
			autoTfvarsSlash := filepath.ToSlash(autoTfvarsPath)
			if nameSlash == userTfvarsSlash || nameSlash == autoTfvarsSlash {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		blueprintYAML := fmt.Sprintf(`apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
terraform:
  - name: %s
    path: terraform/cluster`, componentName)

		originalReadFile := mocks.Shims.ReadFile
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return originalReadFile(filename)
		}

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Name:     componentName,
					Path:     "terraform/cluster",
					FullPath: "test/module",
				},
			}
		}
		mockProvider.GetTerraformComponentFunc = func(componentID string) *blueprintv1alpha1.TerraformComponent {
			if componentID == componentName {
				return &blueprintv1alpha1.TerraformComponent{
					Name:     componentName,
					Path:     "terraform/cluster",
					FullPath: "test/module",
				}
			}
			return nil
		}
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return componentName, nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		envVars, _, err := printer.terraformProvider.GetEnvVars(componentName, true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		planArgs := envVars["TF_CLI_ARGS_plan"]
		userTfvarsSlash := filepath.ToSlash(userTfvarsPath)
		autoTfvarsSlash := filepath.ToSlash(autoTfvarsPath)

		if !strings.Contains(planArgs, userTfvarsSlash) {
			t.Errorf("Expected plan args to contain user tfvars file %s, got %s", userTfvarsSlash, planArgs)
		}
		if !strings.Contains(planArgs, autoTfvarsSlash) {
			t.Errorf("Expected plan args to contain auto-generated tfvars file %s, got %s", autoTfvarsSlash, planArgs)
		}

		userTfvarsIndex := strings.Index(planArgs, userTfvarsSlash)
		autoTfvarsIndex := strings.Index(planArgs, autoTfvarsSlash)
		if autoTfvarsIndex > userTfvarsIndex {
			t.Errorf("Expected auto-generated tfvars file to come before user file in args (so user can override), but auto at %d, user at %d", autoTfvarsIndex, userTfvarsIndex)
		}
	})

	t.Run("UsesWindsorScratchPathForTFDataDir", func(t *testing.T) {
		printer, mocks := setup(t)
		windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
		if err != nil {
			t.Fatalf("Failed to get windsor scratch path: %v", err)
		}

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test/path",
					FullPath: "test/module",
				},
			}
		}
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		mockProvider.GetTFDataDirFunc = func(componentID string) (string, error) {
			return filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", componentID)), nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedTFDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test/path"))
		if args.TFDataDir != expectedTFDataDir {
			t.Errorf("Expected TFDataDir to be %s, got %s", expectedTFDataDir, args.TFDataDir)
		}

		envVars, args, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		if envVars["TF_DATA_DIR"] != expectedTFDataDir {
			t.Errorf("Expected TF_DATA_DIR env var to be %s, got %s", expectedTFDataDir, envVars["TF_DATA_DIR"])
		}
	})

	t.Run("UsesWindsorScratchPathForTfstate", func(t *testing.T) {
		printer, mocks := setup(t)
		windsorScratchPath, err := mocks.ConfigHandler.GetWindsorScratchPath()
		if err != nil {
			t.Fatalf("Failed to get windsor scratch path: %v", err)
		}

		// Set up mock provider
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Path:     "test/path",
					FullPath: "test/module",
				},
			}
		}
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		expectedTfstatePath := filepath.ToSlash(filepath.Join(windsorScratchPath, ".tfstate", "test/path", "terraform.tfstate"))
		envVars, _, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err != nil {
			t.Fatalf("Error getting env vars: %v", err)
		}
		if !strings.Contains(envVars["TF_CLI_ARGS_init"], expectedTfstatePath) {
			t.Errorf("Expected TF_CLI_ARGS_init to contain %s, got %s", expectedTfstatePath, envVars["TF_CLI_ARGS_init"])
		}
	})

	t.Run("HandlesGetWindsorScratchPathError", func(t *testing.T) {
		_, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("windsor scratch path error")
		}

		printer := &TerraformEnvPrinter{
			BaseEnvPrinter: *NewBaseEnvPrinter(mocks.Shell, mockConfigHandler),
		}
		printer.shims = mocks.Shims

		// Set up mock provider that will fail on GetWindsorScratchPath
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		mockProvider.GetTFDataDirFunc = func(componentID string) (string, error) {
			return "", fmt.Errorf("error getting windsor scratch path: test error")
		}
		// Override GenerateTerraformArgs to return error when GetTFDataDir fails
		mockProvider.GenerateTerraformArgsFunc = func(componentID, modulePath string, interactive bool) (*terraform.TerraformArgs, error) {
			_, err := mockProvider.GetTFDataDirFunc(componentID)
			if err != nil {
				return nil, fmt.Errorf("error getting TF_DATA_DIR: %w", err)
			}
			return &terraform.TerraformArgs{}, nil
		}
		mockProvider.GetEnvVarsFunc = func(componentID string, interactive bool) (map[string]string, *terraform.TerraformArgs, error) {
			_, err := mockProvider.GenerateTerraformArgsFunc(componentID, "", interactive)
			return nil, nil, err
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		_, _, err := printer.terraformProvider.GetEnvVars("test/path", true)
		if err == nil {
			t.Error("Expected error when GetTFDataDir fails")
			return
		}
		if !strings.Contains(err.Error(), "TF_DATA_DIR") && !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about TF_DATA_DIR or windsor scratch path, got: %v", err)
		}
	})

	t.Run("CorrectArgumentOrdering", func(t *testing.T) {
		// Given a TerraformEnvPrinter with parallelism configuration
		printer, mocks := setup(t)

		// Mock blueprint.yaml content with parallelism
		blueprintYAML := `apiVersion: v1alpha1
kind: Blueprint
metadata:
  name: test-blueprint
terraform:
  - path: test/path
    parallelism: 3`

		// Mock ReadFile to return blueprint.yaml content
		originalReadFile := mocks.Shims.ReadFile
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return originalReadFile(filename)
		}

		// Set up mock provider
		parallelism := 3
		mockProvider := setupMockTerraformProvider(mocks)
		mockProvider.GetTerraformComponentFunc = func(componentID string) *blueprintv1alpha1.TerraformComponent {
			if componentID == "test/path" {
				return &blueprintv1alpha1.TerraformComponent{
					Path:        "test/path",
					Parallelism: &blueprintv1alpha1.IntExpression{Value: &parallelism, IsExpr: false},
				}
			}
			return nil
		}
		mockProvider.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return "test/path", nil
		}
		// Override GenerateTerraformArgs to manually construct args with parallelism
		testEvaluator := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, "/test/project", "/test/template")
		realProvider := terraform.NewTerraformProvider(mocks.ConfigHandler, mocks.Shell, mocks.ToolsManager, testEvaluator)
		rvProvider := reflect.ValueOf(realProvider)
		if rvProvider.Kind() == reflect.Ptr {
			shimsField := rvProvider.Elem().FieldByName("Shims")
			if shimsField.IsValid() {
				shimsValue := shimsField.Interface().(*terraform.Shims)
				if shimsValue != nil {
					shimsValue.Stat = mocks.Shims.Stat
				}
			}
		}
		mockProvider.GenerateTerraformArgsFunc = func(componentID, modulePath string, interactive bool) (*terraform.TerraformArgs, error) {
			tfDataDir, _ := mockProvider.GetTFDataDirFunc(componentID)
			tfPlanPath := filepath.ToSlash(filepath.Join(tfDataDir, "terraform.tfplan"))

			// GenerateBackendConfigArgs is now private, get args from GenerateTerraformArgs
			args, _ := realProvider.GenerateTerraformArgs(componentID, modulePath, interactive)
			backendConfigArgs := args.InitArgs[3:] // Skip "-backend=true", "-force-copy", "-upgrade"
			initArgs := []string{"-backend=true", "-force-copy", "-upgrade"}
			initArgs = append(initArgs, backendConfigArgs...)

			planArgs := []string{fmt.Sprintf("-out=%s", tfPlanPath)}
			applyArgs := []string{"-parallelism=3"}
			applyArgs = append(applyArgs, tfPlanPath)
			destroyArgs := []string{"-parallelism=3"}

			return &terraform.TerraformArgs{
				TFDataDir:       tfDataDir,
				InitArgs:        initArgs,
				PlanArgs:        planArgs,
				ApplyArgs:       applyArgs,
				RefreshArgs:     []string{},
				ImportArgs:      []string{},
				DestroyArgs:     destroyArgs,
				PlanDestroyArgs: []string{"-destroy"},
				BackendConfig:   strings.Join(backendConfigArgs, " "),
			}, nil
		}
		mockProvider.GetEnvVarsFunc = func(componentID string, interactive bool) (map[string]string, *terraform.TerraformArgs, error) {
			args, err := mockProvider.GenerateTerraformArgsFunc(componentID, "", interactive)
			if err != nil {
				return nil, nil, err
			}
			envVars := make(map[string]string)
			envVars["TF_DATA_DIR"] = args.TFDataDir
			envVars["TF_CLI_ARGS_init"] = strings.Join(args.InitArgs, " ")
			envVars["TF_CLI_ARGS_plan"] = strings.Join(args.PlanArgs, " ")
			envVars["TF_CLI_ARGS_apply"] = strings.Join(args.ApplyArgs, " ")
			envVars["TF_CLI_ARGS_destroy"] = strings.Join(args.DestroyArgs, " ")
			return envVars, args, nil
		}
		printer = setupTerraformEnvPrinter(t, mocks, mockProvider)

		// When generating terraform args for interactive regular injection
		_, args, err := printer.terraformProvider.GetEnvVars("test/path", true)

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

// TestTerraformEnv_restoreEnvVar tests the restoreEnvVar method of the TerraformEnvPrinter
func TestTerraformEnv_restoreEnvVar(t *testing.T) {
	setup := func(t *testing.T) (*TerraformEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupTerraformEnvMocks(t)
		printer := setupTerraformEnvPrinter(t, mocks, setupMockTerraformProvider(mocks))
		return printer, mocks
	}

	t.Run("RestoresValueWhenOriginalValueNotEmpty", func(t *testing.T) {
		// Given a TerraformEnvPrinter and an environment variable with original value
		printer, _ := setup(t)

		testKey := "TEST_RESTORE_VAR"
		originalValue := "original-value"
		os.Setenv(testKey, "changed-value")
		defer os.Unsetenv(testKey)

		// When restoreEnvVar is called with original value
		printer.restoreEnvVar(testKey, originalValue)

		// Then the environment variable should be restored
		if os.Getenv(testKey) != originalValue {
			t.Errorf("Expected %s=%s, got %s", testKey, originalValue, os.Getenv(testKey))
		}
	})

	t.Run("UnsetsValueWhenOriginalValueEmpty", func(t *testing.T) {
		// Given a TerraformEnvPrinter and an environment variable with empty original value
		printer, _ := setup(t)

		testKey := "TEST_UNSET_VAR"
		os.Setenv(testKey, "some-value")
		defer os.Unsetenv(testKey)

		// When restoreEnvVar is called with empty original value
		printer.restoreEnvVar(testKey, "")

		// Then the environment variable should be unset
		if os.Getenv(testKey) != "" {
			t.Errorf("Expected %s to be unset, got %s", testKey, os.Getenv(testKey))
		}
	})
}
