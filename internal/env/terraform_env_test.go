package env

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type TerraformEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
	ConfigHandler  *config.MockConfigHandler
}

func setupSafeTerraformEnvMocks(container ...di.ContainerInterface) *TerraformEnvMocks {
	var mockContainer di.ContainerInterface
	if len(container) > 0 {
		mockContainer = container[0]
	} else {
		mockContainer = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}
	mockContext.GetContextFunc = func() (string, error) {
		return "mockContext", nil
	}

	mockShell := shell.NewMockShell()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
		return &config.Context{
			Terraform: &config.TerraformConfig{
				Backend: stringPtr("local"),
			},
		}, nil
	}

	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)
	mockContainer.Register("cliConfigHandler", mockConfigHandler)

	return &TerraformEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
		ConfigHandler:  mockConfigHandler,
	}
}

func TestTerraformEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()

		expectedEnvVars := map[string]string{
			"TF_DATA_DIR":         "/mock/config/root/.terraform/project/path",
			"TF_CLI_ARGS_init":    "-backend=true -backend-config=path=/mock/config/root/.tfstate/project/path/terraform.tfstate",
			"TF_CLI_ARGS_plan":    "-out=/mock/config/root/.terraform/project/path/terraform.tfplan -var-file=/mock/config/root/terraform/project/path.tfvars -var-file=/mock/config/root/terraform/project/path.tfvars.json",
			"TF_CLI_ARGS_apply":   "/mock/config/root/.terraform/project/path/terraform.tfplan",
			"TF_CLI_ARGS_import":  "-var-file=/mock/config/root/terraform/project/path.tfvars -var-file=/mock/config/root/terraform/project/path.tfvars.json",
			"TF_CLI_ARGS_destroy": "-var-file=/mock/config/root/terraform/project/path.tfvars -var-file=/mock/config/root/terraform/project/path.tfvars.json",
			"TF_VAR_context_path": "/mock/config/root",
		}

		env := NewTerraformEnv(mocks.Container)

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
			return "/mock/project/root/terraform/project/path", nil
		}

		// And a mocked stat function simulating file existence with varied tfvars files
		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			switch name {
			case "/mock/config/root/terraform/project/path.tfvars":
				return nil, nil // Simulate file exists
			case "/mock/config/root/terraform/project/path.tfvars.json":
				return nil, nil // Simulate file exists
			case "/mock/config/root/terraform/project/path_generated.tfvars":
				return nil, os.ErrNotExist // Simulate file does not exist
			case "/mock/config/root/terraform/project/path_generated.tfvars.json":
				return nil, os.ErrNotExist // Simulate file does not exist
			default:
				return nil, os.ErrNotExist // Simulate file does not exist
			}
		}

		// When the GetEnvVars function is called
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Then the expected environment variables should be set
		for key, expectedValue := range expectedEnvVars {
			if value, exists := envVars[key]; !exists || value != expectedValue {
				t.Errorf("Expected %s to be %s, got %s", key, expectedValue, value)
			}
		}
	})

	t.Run("NoProjectPathFound", func(t *testing.T) {
		// Given a mocked getwd function returning a specific path
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks := setupSafeTerraformEnvMocks()

		// When the GetEnvVars function is called
		env := NewTerraformEnv(mocks.Container)
		envVars, err := env.GetEnvVars()

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
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}

		// When the GetEnvVars function is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.GetEnvVars()

		// Then the error should be as expected
		expectedErrorMessage := "error getting config root: mock error getting config root"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorListingTfvarsFiles", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "mockContext", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{}, nil
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
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
		env := NewTerraformEnv(mocks.Container)
		_, err := env.GetEnvVars()

		// Then the error should be as expected
		expectedErrorMessage := "error checking file: mock error checking file"
		if err == nil || err.Error() != expectedErrorMessage {
			t.Errorf("Expected error %q, got %v", expectedErrorMessage, err)
		}
	})
}

func TestTerraformEnv_PostEnvHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "mockContext", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("local"),
				},
			}, nil
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
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
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingDependencies", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// When the PostEnvHook function is called
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving") {
			t.Errorf("Expected error message to contain 'error resolving', got %v", err)
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
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

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
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error finding project path") {
			t.Errorf("Expected error message to contain 'error finding project path', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
		}

		// When the PostEnvHook function is called
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error getting config root") {
			t.Errorf("Expected error message to contain 'error getting config root', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context")
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
		}

		// When the PostEnvHook function is called
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving context") {
			t.Errorf("Expected error message to contain 'error retrieving context', got %v", err)
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("unsupported"),
				},
			}, nil
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
		}

		// When the PostEnvHook function is called
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

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
			return "/mock/project/root/terraform/project/path", nil
		}
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
		}

		// When the PostEnvHook function is called
		mocks := setupSafeTerraformEnvMocks()
		env := NewTerraformEnv(mocks.Container)
		err := env.PostEnvHook()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing backend_override.tf file") {
			t.Errorf("Expected error message to contain 'error writing backend_override.tf file', got %v", err)
		}
	})
}

func TestTerraformEnv_resolveDependencies(t *testing.T) {
	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error resolving contextHandler") {
			t.Errorf("Expected error message to contain 'mock error resolving contextHandler', got %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)

		// Given a mock object that does not implement ContextInterface
		mockContainer.Register("contextHandler", "invalidContextHandler")

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "contextHandler is not of type ContextInterface") {
			t.Errorf("Expected error message to contain 'contextHandler is not of type ContextInterface', got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error resolving shell") {
			t.Errorf("Expected error message to contain 'mock error resolving shell', got %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)

		// Given a mock object that does not implement Shell
		mockContainer.Register("shell", "invalidShell")

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "shell is not of type Shell") {
			t.Errorf("Expected error message to contain 'shell is not of type Shell', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("mock error resolving cliConfigHandler"))

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error resolving cliConfigHandler") {
			t.Errorf("Expected error message to contain 'mock error resolving cliConfigHandler', got %v", err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)

		// Given a mock object that does not implement ConfigHandler
		mockContainer.Register("cliConfigHandler", "invalidConfigHandler")

		// When resolveDependencies is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.resolveDependencies()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "cliConfigHandler is not of type ConfigHandler") {
			t.Errorf("Expected error message to contain 'cliConfigHandler is not of type ConfigHandler', got %v", err)
		}
	})
}

func TestTerraformEnv_getAlias(t *testing.T) {
	t.Run("ErrorResolvingDependencies", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// When getAlias is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.getAlias()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving") {
			t.Errorf("Expected error message to contain 'error resolving', got %v", err)
		}
	})

	t.Run("SuccessLocalstackEnabled", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "local", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create: boolPtr(true),
					},
				},
			}, nil
		}

		// When getAlias is called
		env := NewTerraformEnv(mocks.Container)
		aliases, err := env.getAlias()

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
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "local", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				AWS: &config.AWSConfig{
					Localstack: &config.LocalstackConfig{
						Create: boolPtr(false),
					},
				},
			}, nil
		}

		// When getAlias is called
		env := NewTerraformEnv(mocks.Container)
		aliases, err := env.getAlias()

		// Then no error should occur and the expected alias should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		expectedAlias := map[string]string{"terraform": ""}
		if !reflect.DeepEqual(aliases, expectedAlias) {
			t.Errorf("Expected aliases %v, got %v", expectedAlias, aliases)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context")
		}

		// When getAlias is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.getAlias()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving context") {
			t.Errorf("Expected error message to contain 'error retrieving context', got %v", err)
		}
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context config")
		}

		// When getAlias is called
		env := NewTerraformEnv(mocks.Container)
		_, err := env.getAlias()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving context config") {
			t.Errorf("Expected error message to contain 'error retrieving context config', got %v", err)
		}
	})
}

func TestTerraformEnv_findRelativeTerraformProjectPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mocked getwd function returning a specific directory path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/path/to/terraform/project", nil
		}
		defer func() { getwd = originalGetwd }()

		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			if pattern == "/mock/path/to/terraform/project/*.tf" {
				return []string{"/mock/path/to/terraform/project/main.tf"}, nil
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
			return "/mock/path/to/terraform/project", nil
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
			return "/mock/path/to/project", nil
		}
		defer func() { getwd = originalGetwd }()

		// And a mocked glob function simulating finding Terraform files
		originalGlob := glob
		glob = func(pattern string) ([]string, error) {
			if pattern == "/mock/path/to/project/*.tf" {
				return []string{"/mock/path/to/project/main.tf"}, nil
			}
			return nil, nil
		}
		defer func() { glob = originalGlob }()

		// When findRelativeTerraformProjectPath is called
		_, err := findRelativeTerraformProjectPath()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no 'terraform' directory found in the current path") {
			t.Errorf("Expected error message to contain 'no 'terraform' directory found in the current path', got %v", err)
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
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("local"),
				},
			}, nil
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
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
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `
terraform {
  backend "local" {
    path = "/mock/config/root/.tfstate/project/path/terraform.tfstate"
  }
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("S3Backend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("s3"),
				},
			}, nil
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
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
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `
terraform {
  backend "s3" {
    key = "project/path/terraform.tfstate"
  }
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("KubernetesBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("kubernetes"),
				},
			}, nil
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
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
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then no error should occur and the expected backend config should be written
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedContent := `
terraform {
  backend "kubernetes" {
    secret_suffix = "project-path"
  }
}`
		if string(writtenData) != expectedContent {
			t.Errorf("Expected backend config %q, got %q", expectedContent, string(writtenData))
		}
	})

	t.Run("ErrorResolvingDependencies", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		mocks := setupSafeTerraformEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving contextHandler"))

		// When generateBackendOverrideTf is called
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error resolving contextHandler") {
			t.Errorf("Expected error message to contain 'mock error resolving contextHandler', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
			}
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error getting config root") {
			t.Errorf("Expected error message to contain 'mock error getting config root', got %v", err)
		}
	})

	t.Run("ErrorGettingConfig", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context")
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
			}
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then the error should contain the expected message
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error retrieving context") {
			t.Errorf("Expected error message to contain 'mock error retrieving context', got %v", err)
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		mocks := setupSafeTerraformEnvMocks()
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("unsupported"),
				},
			}, nil
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
			if pattern == "/mock/project/root/terraform/project/path/*.tf" {
				return []string{"/mock/project/root/terraform/project/path/main.tf"}, nil
			}
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

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
		mocks.ConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Terraform: &config.TerraformConfig{
					Backend: stringPtr("local"),
				},
			}, nil
		}

		// Given a mocked getwd function simulating being in a terraform project root
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()
		getwd = func() (string, error) {
			return "/mock/project/root/terraform/project/path", nil
		}
		// And a mocked glob function simulating no Terraform files found
		originalGlob := glob
		defer func() { glob = originalGlob }()
		glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When generateBackendOverrideTf is called
		env := NewTerraformEnv(mocks.Container)
		err := env.generateBackendOverrideTf()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
