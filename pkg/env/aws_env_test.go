package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupAwsEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if len(opts) == 0 || opts[0].ConfigStr == "" {
		opts = []*SetupOptions{{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    aws:
      aws_profile: default
      aws_endpoint_url: https://aws.endpoint
      s3_hostname: s3.amazonaws.com
      mwaa_endpoint: https://mwaa.endpoint
`,
		}}
	}

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()

	// Set up the GetConfigRoot function to return a mock path
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Set up the GetConfig function to return a mock config
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		// Parse the config string
		var config v1alpha1.Config
		if err := yaml.Unmarshal([]byte(opts[0].ConfigStr), &config); err != nil {
			t.Fatalf("Failed to unmarshal config: %v", err)
		}

		// Return the context for the test-context
		if ctx, ok := config.Contexts["test-context"]; ok {
			return ctx
		}
		return &v1alpha1.Context{}
	}

	// Create mocks with the mock config handler
	mocks := setupMocks(t, &SetupOptions{
		ConfigHandler: mockConfigHandler,
	})

	if err := mocks.ConfigHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
	}
	if err := mocks.ConfigHandler.SetContext("test-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	// Set up shims for AWS config file check
	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		if name == filepath.FromSlash("/mock/config/root/.aws/config") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestAwsEnv_GetEnvVars tests the GetEnvVars method of the AwsEnvPrinter
func TestAwsEnv_GetEnvVars(t *testing.T) {
	setup := func() (*AwsEnvPrinter, *Mocks) {
		mocks := setupAwsEnvMocks(t)
		env := NewAwsEnvPrinter(mocks.Injector)
		if err := env.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		env.shims = mocks.Shims
		return env, mocks
	}

	t.Run("Success", func(t *testing.T) {
		env, _ := setup()

		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}

		expected := map[string]string{
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "https://aws.endpoint",
			"S3_HOSTNAME":      "s3.amazonaws.com",
			"MWAA_ENDPOINT":    "https://mwaa.endpoint",
			"AWS_CONFIG_FILE":  "/mock/config/root/.aws/config",
		}

		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("NonExistentConfigFile", func(t *testing.T) {
		env, _ := setup()

		// Override shims to make AWS config file not exist
		env.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Errorf("GetEnvVars returned an error: %v", err)
		}

		expected := map[string]string{
			"AWS_PROFILE":      "default",
			"AWS_ENDPOINT_URL": "https://aws.endpoint",
			"S3_HOSTNAME":      "s3.amazonaws.com",
			"MWAA_ENDPOINT":    "https://mwaa.endpoint",
		}

		if !reflect.DeepEqual(envVars, expected) {
			t.Errorf("GetEnvVars returned %v, want %v", envVars, expected)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		mocks := setupAwsEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context: {}
`,
		})
		env := NewAwsEnvPrinter(mocks.Injector)
		if err := env.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}

		_, err := env.GetEnvVars()
		if err == nil {
			t.Error("GetEnvVars did not return an error")
		}
		if !strings.Contains(err.Error(), "context configuration or AWS configuration is missing") {
			t.Errorf("GetEnvVars returned error %v, want error containing 'context configuration or AWS configuration is missing'", err)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupAwsEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    aws:
      aws_profile: default
`,
		})

		// Mock the GetConfigRoot function to return an error
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving configuration root directory")
		}

		env := NewAwsEnvPrinter(mocks.Injector)
		if err := env.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}

		_, err := env.GetEnvVars()
		if err == nil {
			t.Error("GetEnvVars did not return an error")
		}
		if !strings.Contains(err.Error(), "error retrieving configuration root directory") {
			t.Errorf("GetEnvVars returned error %v, want error containing 'error retrieving configuration root directory'", err)
		}
	})
}
