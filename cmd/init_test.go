package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/shell"
)

// setupInitMocks creates mock components specifically for testing the init command
func setupInitMocks(t *testing.T, opts *SetupOptions) *Mocks {
	t.Helper()

	// Create a temporary directory for config
	tmpDir := t.TempDir()
	if opts == nil {
		opts = &SetupOptions{}
	}

	// Use the existing setupMocks function as a base
	mocks := setupMocks(t, opts)

	// Set up mock shell functions specific to init command
	mockShell := mocks.Shell
	mockShell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	mockShell.WriteResetTokenFunc = func() (string, error) { return "reset-token", nil }
	mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }

	// Set up mock controller functions specific to init command
	mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
		return nil
	}
	mocks.Controller.ResolveShellFunc = func() shell.Shell {
		return mockShell
	}
	mocks.Controller.SetEnvironmentVariablesFunc = func() error {
		return nil
	}
	mocks.Controller.WriteConfigurationFilesFunc = func() error {
		return nil
	}

	return mocks
}

func TestInitCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Close the writer and restore os.Stderr
		w.Close()

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Create a pipe to capture os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		// Set up command arguments
		rootCmd.SetArgs([]string{"init", "test-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Close the writer and restore os.Stderr
		w.Close()

		// Read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should contain success message
		expectedMessage := "Initialization successful\n"
		if buf.String() != expectedMessage {
			t.Errorf("Expected message %q, got %q", expectedMessage, buf.String())
		}
	})

	t.Run("AddCurrentDirToTrustedFileError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override shell to return error for AddCurrentDirToTrustedFile
		mocks.Shell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("trusted file error")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain trusted file error message
		expectedError := "Error adding current directory to trusted file: trusted file error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("WriteResetTokenError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override shell to return error for WriteResetToken
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("reset token error")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain reset token error message
		expectedError := "Error writing reset token: reset token error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override config handler to return error for SetContext
		// We need to create a new mock config handler with the error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mockConfigHandler.SetContextFunc = func(context string) error {
			return fmt.Errorf("set context error")
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(config v1alpha1.Context) error {
			return nil
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return nil
		}
		mockConfigHandler.SaveConfigFunc = func(path string) error {
			return nil
		}
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mockConfigHandler
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain set context error message
		expectedError := "Error setting context value: set context error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override config handler to return error for SaveConfig
		// We need to create a new mock config handler with the error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string {
			return ""
		}
		mockConfigHandler.SetContextFunc = func(context string) error {
			return nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(config v1alpha1.Context) error {
			return nil
		}
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			return nil
		}
		mockConfigHandler.SaveConfigFunc = func(path string) error {
			return fmt.Errorf("save config error")
		}
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mockConfigHandler
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain save config error message
		expectedError := "Error saving config file: save config error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetEnvironmentVariablesError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override controller to return error for SetEnvironmentVariables
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("set env vars error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain set env vars error message
		expectedError := "Error setting environment variables: set env vars error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("WriteConfigurationFilesError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override controller to return error for WriteConfigurationFiles
		mocks.Controller.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("write config files error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain write config files error message
		expectedError := "Error writing configuration files: write config files error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("VMDriverConfiguration", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Set up command arguments with VM driver
		rootCmd.SetArgs([]string{"init", "--vm-driver", "colima"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And VM driver should be set in config
		vmDriver := mocks.ConfigHandler.GetString("vm.driver")
		if vmDriver != "colima" {
			t.Errorf("Expected VM driver to be 'colima', got '%s'", vmDriver)
		}
	})

	t.Run("DefaultConfigSelection", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Set up command arguments with docker-desktop VM driver
		rootCmd.SetArgs([]string{"init", "--vm-driver", "docker-desktop"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And docker should be enabled in config
		dockerEnabled := mocks.ConfigHandler.GetBool("docker.enabled")
		if !dockerEnabled {
			t.Error("Expected docker to be enabled for docker-desktop VM driver")
		}
	})

	t.Run("FlagToConfigMapping", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Set up command arguments with various flags
		rootCmd.SetArgs([]string{
			"init",
			"--aws-profile", "test-profile",
			"--aws-endpoint-url", "https://test.endpoint",
			"--docker",
			"--vm-cpu", "4",
			"--vm-memory", "8192",
			"--vm-disk", "100",
			"--vm-arch", "arm64",
			"--platform", "local",
			"--endpoint", "https://test.endpoint:6443",
		})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And all flag values should be set in config
		stringChecks := map[string]string{
			"aws.aws_profile":      "test-profile",
			"aws.aws_endpoint_url": "https://test.endpoint",
			"vm.arch":              "arm64",
			"cluster.platform":     "local",
			"cluster.endpoint":     "https://test.endpoint:6443",
		}

		for path, expected := range stringChecks {
			value := mocks.ConfigHandler.GetString(path)
			if value != expected {
				t.Errorf("Expected %s to be %v, got %v", path, expected, value)
			}
		}

		// Check boolean values
		if !mocks.ConfigHandler.GetBool("docker.enabled") {
			t.Error("Expected docker.enabled to be true")
		}

		// Check integer values
		intChecks := map[string]int{
			"vm.cpu":    4,
			"vm.memory": 8192,
			"vm.disk":   100,
		}

		for path, expected := range intChecks {
			value := mocks.ConfigHandler.GetInt(path)
			if value != expected {
				t.Errorf("Expected %s to be %d, got %d", path, expected, value)
			}
		}
	})

	t.Run("CLIConfigPathDetermination", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Create a temporary directory for the test
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }

		// Create both .yaml and .yml files
		yamlPath := filepath.Join(tmpDir, "windsor.yaml")
		ymlPath := filepath.Join(tmpDir, "windsor.yml")
		if err := os.WriteFile(yamlPath, []byte("test: data"), 0644); err != nil {
			t.Fatalf("Failed to create yaml file: %v", err)
		}
		if err := os.WriteFile(ymlPath, []byte("test: data"), 0644); err != nil {
			t.Fatalf("Failed to create yml file: %v", err)
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And config should be saved to .yaml file (preferred extension)
		if _, err := os.Stat(yamlPath); err != nil {
			t.Errorf("Expected config to be saved to %s, got error: %v", yamlPath, err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks := setupInitMocks(t, nil)

		// Override shell to return error for GetProjectRoot
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// Set up command arguments
		rootCmd.SetArgs([]string{"init"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain project root error message
		expectedError := "Error setting context value: error getting project root: project root error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetDefaultConfigError", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(config v1alpha1.Context) error {
			return fmt.Errorf("failed to set default config")
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupInitMocks(t, opts)

		rootCmd.SetArgs([]string{"init"})
		err := Execute(mocks.Controller)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to set default config") {
			t.Errorf("Expected error containing 'failed to set default config', got: %v", err)
		}
	})

	t.Run("SetVMDriverError", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "vm.driver" {
				return fmt.Errorf("failed to set vm driver")
			}
			return nil
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupInitMocks(t, opts)

		rootCmd.SetArgs([]string{"init", "--vm-driver", "docker-desktop"})
		err := Execute(mocks.Controller)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to set vm driver") {
			t.Errorf("Expected error containing 'failed to set vm driver', got: %v", err)
		}
	})

	t.Run("AutomaticVMDriverSelectionDarwin", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupInitMocks(t, opts)

		rootCmd.SetArgs([]string{"init", "local"})
		err := Execute(mocks.Controller)

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		vmDriver := mocks.ConfigHandler.GetString("vm.driver")
		if vmDriver != "docker-desktop" {
			t.Errorf("Expected VM driver to be 'docker-desktop', got '%s'", vmDriver)
		}
	})

	t.Run("AutomaticVMDriverSelectionLinux", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker"
			}
			return ""
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupInitMocks(t, opts)

		rootCmd.SetArgs([]string{"init", "local"})
		err := Execute(mocks.Controller)

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		vmDriver := mocks.ConfigHandler.GetString("vm.driver")
		if vmDriver != "docker" {
			t.Errorf("Expected VM driver to be 'docker', got '%s'", vmDriver)
		}
	})

	t.Run("AutomaticVMDriverSelectionNonLocal", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return ""
			}
			return ""
		}
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupInitMocks(t, opts)

		rootCmd.SetArgs([]string{"init", "prod"})
		err := Execute(mocks.Controller)

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		vmDriver := mocks.ConfigHandler.GetString("vm.driver")
		if vmDriver != "" {
			t.Errorf("Expected VM driver to be empty, got '%s'", vmDriver)
		}
	})
}
