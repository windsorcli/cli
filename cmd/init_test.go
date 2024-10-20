package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	originalExitFunc := exitFunc
	originalTerraformHelper := terraformHelper
	originalColimaHelper := colimaHelper
	originalContextInstance := contextInstance

	t.Cleanup(func() {
		rootCmd.Args = originalArgs
		exitFunc = originalExitFunc
		terraformHelper = originalTerraformHelper
		colimaHelper = originalColimaHelper
		contextInstance = originalContextInstance
	})

	// Mock the exit function to prevent the test from exiting
	exitFunc = func(code int) {
		panic("exit called")
	}

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("AllFlagsSet", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// Mock the Get function to ensure it is called with the desired object
		mockHandler.GetFunc = func(key string) (interface{}, error) {
			if key != "test-context" {
				t.Errorf("Expected key %q, got %q", "test-context", key)
			}
			return map[string]interface{}{
				"AWS": map[string]interface{}{
					"AWSEndpointURL": "http://localhost:4566",
					"AWSProfile":     "test-profile",
				},
				"Docker": map[string]interface{}{
					"Enabled": true,
				},
				"Terraform": map[string]interface{}{
					"Backend": "s3",
				},
				"VM": map[string]interface{}{
					"Driver": "colima",
					"CPU":    2,
					"Disk":   20,
					"Memory": 4096,
					"Arch":   "x86_64",
				},
			}, nil
		}

		// When: the init command is executed with all flags set
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{
				"init", "test-context",
				"--aws-endpoint-url", "http://localhost:4566",
				"--aws-profile", "test-profile",
				"--docker",
				"--backend", "s3",
				"--vm-driver", "colima",
				"--vm-cpu", "2",
				"--vm-disk", "20",
				"--vm-memory", "4096",
				"--vm-arch", "x86_64",
			})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveProjectConfigSuccess", func(t *testing.T) {
		// Given: a valid project config handler and cli config handler
		mockProjectHandler := config.NewMockConfigHandler()
		mockCLIHandler := config.NewMockConfigHandler()
		saveProjectConfigCalled := false

		mockProjectHandler.SaveConfigFunc = func(path string) error {
			saveProjectConfigCalled = true
			expectedPath := filepath.ToSlash(filepath.Join("/mock/project/root", "windsor.yaml"))
			if filepath.ToSlash(path) != expectedPath {
				t.Errorf("Expected SaveConfig to be called with path %q, got %q", expectedPath, filepath.ToSlash(path))
			}
			return nil
		}

		// Mock shell to return a valid project root
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mockHelper := helpers.NewMockHelper()
		mockDockerHelper := helpers.NewMockHelper()
		setupContainer(mockCLIHandler, mockProjectHandler, mockShell, mockHelper, mockHelper, nil, mockDockerHelper)

		// Mock osStat to return no error for windsor.yaml
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/mock/project/root", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When: the init command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}

		// Verify that SaveConfig was called for the project handler
		if !saveProjectConfigCalled {
			t.Fatalf("Expected SaveConfig to be called for project config, but it was not")
		}
	})

	t.Run("SetContextConfigError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given: a config handler that returns an error on setting the context configuration
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockCliConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == fmt.Sprintf("contexts.%s", "test-context") {
				return errors.New("set context config error")
			}
			return nil
		}
		mockProjectConfigHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		setupContainer(mockCliConfigHandler, mockProjectConfigHandler, mockShell, mockHelper, nil, nil, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting config value: set context config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveProjectConfigError", func(t *testing.T) {
		// Given: a project config handler that returns an error on SaveConfig
		mockProjectHandler := config.NewMockConfigHandler()
		mockCLIHandler := config.NewMockConfigHandler()

		mockProjectHandler.SaveConfigFunc = func(path string) error {
			return fmt.Errorf("mock save config error")
		}

		// Mock shell to return a valid project root
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mockHelper := helpers.NewMockHelper()
		mockDockerHelper := helpers.NewMockHelper()
		setupContainer(mockCLIHandler, mockProjectHandler, mockShell, mockHelper, mockHelper, nil, mockDockerHelper)

		// Mock osStat to return no error for windsor.yaml
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/mock/project/root", "windsor.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving project config file: mock save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("HomeDirError", func(t *testing.T) {
		// Mock cliConfigHandler
		mockHandler := config.NewMockConfigHandler()
		cliConfigHandler = mockHandler

		// Mock os.UserHomeDir to simulate an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving home directory")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// Mock the exit function to prevent the test from exiting
		exitCalled := false
		exitFunc = func(code int) {
			exitCalled = true
		}

		rootCmd.SetArgs([]string{"init", "test-context"})
		err := rootCmd.Execute()

		// Check that the error is as expected
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "error retrieving home directory: mocked error retrieving home directory"
		if err.Error() != expectedError {
			t.Fatalf("Execute() error = %v, expected '%s'", err, expectedError)
		}

		// Check that exit was called
		if !exitCalled {
			t.Fatalf("Expected exit to be called, but it was not")
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set config value error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "set config value error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving config file: save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	// t.Run("ProjectConfigSaveError", func(t *testing.T) {
	// 	// Given: a CLI config handler that succeeds and a project config handler that returns an error on SaveConfig
	// 	mockCLIHandler := config.NewMockConfigHandler()
	// 	mockProjectHandler := config.NewMockConfigHandler()
	// 	mockProjectHandler.SaveConfigFunc = func(path string) error { return errors.New("save project config error") }
	// 	mockShell, err := shell.NewMockShell("cmd")
	// 	if err != nil {
	// 		t.Fatalf("NewMockShell() error = %v", err)
	// 	}
	// 	mockHelper := &helpers.MockHelper{}
	// 	setupContainer(mockCLIHandler, mockProjectHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

	// 	// When: the init command is executed
	// 	output := captureStderr(func() {
	// 		rootCmd.SetArgs([]string{"init", "test-context"})
	// 		err := rootCmd.Execute()
	// 		if err == nil {
	// 			t.Fatalf("Expected error, got nil")
	// 		}
	// 	})

	// 	// Then: the output should indicate the error
	// 	expectedOutput := "Error saving project config file: save project config error"
	// 	if !strings.Contains(output, expectedOutput) {
	// 		t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
	// 	}
	// })

	// t.Run("SetBackendConfigError", func(t *testing.T) {
	// 	// Given: a config handler that returns an error on setting backend config value
	// 	mockHandler := config.NewMockConfigHandler()
	// 	mockHandler.SetFunc = func(key string, value interface{}) error {
	// 		if key == "contexts.test-context.terraform.backend" {
	// 			return errors.New("set backend config error")
	// 		}
	// 		return nil
	// 	}
	// 	mockShell, err := shell.NewMockShell("cmd")
	// 	if err != nil {
	// 		t.Fatalf("NewMockShell() error = %v", err)
	// 	}
	// 	mockHelper := &helpers.MockHelper{}
	// 	setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

	// 	// When: the init command is executed
	// 	output := captureStderr(func() {
	// 		rootCmd.SetArgs([]string{"init", "test-context"})
	// 		err := rootCmd.Execute()
	// 		if err == nil {
	// 			t.Fatalf("Expected error, got nil")
	// 		}
	// 	})

	// 	// Then: the output should indicate the error
	// 	expectedOutput := "Error setting backend value: set backend config error"
	// 	if !strings.Contains(output, expectedOutput) {
	// 		t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
	// 	}
	// })

	t.Run("CLIConfigSaveError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save cli config error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}

		// Replace the global contextInstance with the mock
		originalContextInstance := contextInstance
		defer func() { contextInstance = originalContextInstance }()

		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving config file: save cli config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("WriteConfigError", func(t *testing.T) {
		// Given: a helper that returns an error on WriteConfig
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			WriteConfigFunc: func() error {
				return errors.New("write config error")
			},
		}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, nil, nil, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error writing config for helper: write config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a container that returns an error when resolving helpers
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockProjectConfigHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error")) // Simulate error
		mockContainer.Register("cliConfigHandler", mockCliConfigHandler)
		mockContainer.Register("projectConfigHandler", mockProjectConfigHandler)
		mockContainer.Register("shell", mockShell)
		mockContainer.Register("terraformHelper", mockHelper)
		mockContainer.Register("awsHelper", mockHelper)
		mockContainer.Register("colimaHelper", mockHelper)
		mockContainer.Register("dockerHelper", mockHelper)
		Initialize(mockContainer)

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "error resolving helpers: resolve helpers error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("LocalContextSetsDefault", func(t *testing.T) {
		// Arrange: Create a mock config handler and a flag to check if SetDefault was called
		mockHandler := config.NewMockConfigHandler()
		called := false

		// Set the SetDefaultFunc to update the flag and check the parameters
		mockHandler.SetDefaultFunc = func(context config.Context) {
			called = true
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				t.Errorf("Expected value %v, got %v", expectedValue, context)
			}
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// Act: Call the init command with a local context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Assert: Verify that SetDefault was called
		if !called {
			t.Error("Expected SetDefaultFunc to be called")
		}

		// Assert: Verify the output indicates success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("AWSConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		calledKeys := make(map[string]bool)

		mockHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
			"--aws-profile", "test-profile",
		})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.aws.aws_endpoint_url": true,
			"contexts.test-context.aws.aws_profile":      true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("DockerConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		calledKeys := make(map[string]bool)

		mockHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.docker.enabled": true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("TerraformConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		calledKeys := make(map[string]bool)

		mockHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.terraform.backend": true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("VMConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		calledKeys := make(map[string]bool)

		mockHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper()
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
			"--vm-cpu", "2",
			"--vm-disk", "20",
			"--vm-memory", "4096",
			"--vm-arch", "x86_64",
		})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.vm.driver": true,
			"contexts.test-context.vm.cpu":    true,
			"contexts.test-context.vm.disk":   true,
			"contexts.test-context.vm.memory": true,
			"contexts.test-context.vm.arch":   true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("ErrorSettingAWSEndpointURL", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_endpoint_url" {
				return errors.New("error setting AWS endpoint URL")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting AWS endpoint URL: error setting AWS endpoint URL" {
			t.Fatalf("Expected error setting AWS endpoint URL, got %v", err)
		}
	})

	t.Run("ErrorSettingAWSProfile", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_profile" {
				return errors.New("error setting AWS profile")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-profile", "default",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting AWS profile: error setting AWS profile" {
			t.Fatalf("Expected error setting AWS profile, got %v", err)
		}
	})

	t.Run("ErrorSettingDockerConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.docker.enabled" {
				return errors.New("error setting Docker enabled")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting Docker enabled: error setting Docker enabled" {
			t.Fatalf("Expected error setting Docker enabled, got %v", err)
		}
	})

	t.Run("ErrorSettingTerraformConfiguration", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.terraform.backend" {
				return errors.New("error setting Terraform backend")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting Terraform backend: error setting Terraform backend" {
			t.Fatalf("Expected error setting Terraform backend, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationArch", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.arch" {
				return errors.New("error setting VM architecture")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-arch", "x86_64",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM architecture: error setting VM architecture" {
			t.Fatalf("Expected error setting VM architecture, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDriver", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.driver" {
				return errors.New("error setting VM driver")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM driver: error setting VM driver" {
			t.Fatalf("Expected error setting VM driver, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationCPU", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.cpu" {
				return errors.New("error setting VM CPU")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-cpu", "2",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM CPU: error setting VM CPU" {
			t.Fatalf("Expected error setting VM CPU, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDisk", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.disk" {
				return errors.New("error setting VM disk")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-disk", "20",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM disk: error setting VM disk" {
			t.Fatalf("Expected error setting VM disk, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationMemory", func(t *testing.T) {
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.memory" {
				return errors.New("error setting VM memory")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-memory", "4096",
		})
		err = rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM memory: error setting VM memory" {
			t.Fatalf("Expected error setting VM memory, got %v", err)
		}
	})
}
