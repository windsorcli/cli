package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Struct to hold optional mock handlers and helpers
type MockDependencies struct {
	CLIConfigHandler config.ConfigHandler
	Shell            shell.Shell
	TerraformHelper  helpers.Helper
	AwsHelper        helpers.Helper
	ColimaHelper     helpers.Helper
	DockerHelper     helpers.Helper
	ContextInstance  context.ContextInterface
}

// Helper function to create a new container and register mock handlers
func setupContainer(deps MockDependencies) di.ContainerInterface {
	container := di.NewContainer()

	if deps.CLIConfigHandler == nil {
		deps.CLIConfigHandler = config.NewMockConfigHandler()
	}
	if deps.Shell == nil {
		deps.Shell = shell.NewMockShell()
	}
	if deps.TerraformHelper == nil {
		deps.TerraformHelper = helpers.NewMockHelper()
	}
	if deps.AwsHelper == nil {
		deps.AwsHelper = helpers.NewMockHelper()
	}
	if deps.ColimaHelper == nil {
		deps.ColimaHelper = helpers.NewMockHelper()
	}
	if deps.DockerHelper == nil {
		deps.DockerHelper = helpers.NewMockHelper()
	}
	if deps.ContextInstance == nil {
		deps.ContextInstance = context.NewMockContext()
	}

	container.Register("cliConfigHandler", deps.CLIConfigHandler)
	container.Register("shell", deps.Shell)
	container.Register("terraformHelper", deps.TerraformHelper)
	container.Register("awsHelper", deps.AwsHelper)
	container.Register("colimaHelper", deps.ColimaHelper)
	container.Register("dockerHelper", deps.DockerHelper)
	container.Register("contextInstance", deps.ContextInstance)
	Initialize(container)

	return container
}

// Helper function to capture stdout output
func captureStdout(f func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	return buf.String()
}

// Helper function to capture stderr output
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	return buf.String()
}

// Mock exit function to capture exit code and message
var exitCode int
var exitMessage string

func mockExit(code int, message string) {
	exitCode = code
	exitMessage = message
	panic(fmt.Sprintf("exit code: %d", code))
}

func TestRootCommand(t *testing.T) {
	originalExitFunc := exitFunc
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("PreRunLoadConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			Initialize(mocks.Container)

			// When preRunLoadConfig is executed
			err := preRunLoadConfig(nil, nil)

			// Then no error should be returned
			if err != nil {
				t.Fatalf("preRunLoadConfig() error = %v, expected no error", err)
			}
		})

		t.Run("NoCLIConfigHandler", func(t *testing.T) {
			// Given no CLI config handler is registered
			cliConfigHandler = nil

			// When preRunLoadConfig is executed
			err := preRunLoadConfig(nil, nil)

			// Then an error should be returned
			if err == nil {
				t.Fatalf("preRunLoadConfig() expected error, got nil")
			}
			expectedError := "cliConfigHandler is not initialized"
			if err.Error() != expectedError {
				t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
			}
		})

		t.Run("CLIConfigLoadError", func(t *testing.T) {
			// Given CLI config handler returns an error on LoadConfig
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return errors.New("mock load error") }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			Initialize(mocks.Container)

			// When preRunLoadConfig is executed
			err := preRunLoadConfig(nil, nil)

			// Then an error should be returned
			if err == nil {
				t.Fatalf("preRunLoadConfig() expected error, got nil")
			}
			expectedError := "error loading CLI config: mock load error"
			if err.Error() != expectedError {
				t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
			}
		})
	})

	t.Run("Success", func(t *testing.T) {
		t.Run("ValidConfigHandlers", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			Initialize(mocks.Container)

			// Mock exitFunc to capture the exit code
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			// Add a dummy subcommand to trigger PersistentPreRunE
			dummyCmd := &cobra.Command{
				Use: "dummy",
				Run: func(cmd *cobra.Command, args []string) {},
			}
			rootCmd.AddCommand(dummyCmd)
			rootCmd.SetArgs([]string{"dummy"})

			// When the command is executed
			err := rootCmd.Execute()

			// Then no error should be returned and exitFunc should not be called
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if exitCode != 0 {
				t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
			}

			// Cleanup
			rootCmd.RemoveCommand(dummyCmd)
		})
	})

	t.Run("Error", func(t *testing.T) {
		t.Run("NoConfigHandlers", func(t *testing.T) {
			// Given no config handlers are registered
			cliConfigHandler = nil

			// Mock exitFunc to capture the exit code
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			// Add a dummy subcommand to trigger PersistentPreRunE
			dummyCmd := &cobra.Command{
				Use: "dummy",
				Run: func(cmd *cobra.Command, args []string) {},
			}
			rootCmd.AddCommand(dummyCmd)
			rootCmd.SetArgs([]string{"dummy"})

			// Capture stderr output
			actualErrorMsg := captureStderr(func() {
				Execute()
			})

			// Then exitFunc should be called with code 1 and the error message should be printed to stderr
			if exitCode != 1 {
				t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
			}
			expectedErrorMsg := "cliConfigHandler is not initialized\n"
			if !strings.Contains(actualErrorMsg, expectedErrorMsg) {
				t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
			}

			// Cleanup
			rootCmd.RemoveCommand(dummyCmd)
		})
	})

	t.Run("Initialize", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }

			setupContainer(MockDependencies{
				CLIConfigHandler: mocks.CLIConfigHandler,
				Shell:            mocks.Shell,
			})

			// Mock exitFunc to capture the exit code
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			// When the cmd package is initialized and stderr is captured
			actualErrorMsg := captureStderr(func() {
				Initialize(container)
			})

			// Then exitFunc should not be called and no error message should be printed to stderr
			if exitCode != 0 {
				t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
			}
			if actualErrorMsg != "" {
				t.Errorf("Expected no error message, got '%s'", actualErrorMsg)
			}
		})

		t.Run("Error", func(t *testing.T) {
			// Given no config handlers are registered
			container := di.NewContainer()

			// Mock exitFunc to capture the exit code
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			// When the cmd package is initialized and stderr is captured
			actualErrorMsg := captureStderr(func() {
				Initialize(container)
			})

			// Then exitFunc should be called with code 1 and the error message should be printed to stderr
			if exitCode != 1 {
				t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
			}
			expectedErrorMsg := "Error resolving cliConfigHandler: no instance registered with name cliConfigHandler\n"
			if !strings.Contains(actualErrorMsg, expectedErrorMsg) {
				t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
			}
		})
	})

	t.Run("GetCLIConfigPath", func(t *testing.T) {
		t.Run("UserHomeDirError", func(t *testing.T) {
			// Save the original functions
			originalOsUserHomeDir := osUserHomeDir
			originalExitFunc := exitFunc

			// Restore the original functions after the test
			t.Cleanup(func() {
				osUserHomeDir = originalOsUserHomeDir
				exitFunc = originalExitFunc
			})

			// Mock osUserHomeDir to return an error
			osUserHomeDir = func() (string, error) {
				return "", errors.New("mock error")
			}

			// Mock exitFunc to capture the exit code
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}

			// Capture the output to os.Stderr
			stderr := captureStderr(func() {
				getCLIConfigPath()
			})

			// Verify the error message
			expectedErrorMessage := "error finding home directory, mock error\n"
			if stderr != expectedErrorMessage {
				t.Errorf("expected error message %q, got %q", expectedErrorMessage, stderr)
			}

			// Verify the exit code
			if exitCode != 1 {
				t.Errorf("expected exit code 1, got %d", exitCode)
			}
		})
	})
	t.Run("GetProjectConfigPath", func(t *testing.T) {
		// Save the original functions
		originalGetwd := getwd
		originalExitFunc := exitFunc

		// Restore the original functions after the test
		t.Cleanup(func() {
			getwd = originalGetwd
			exitFunc = originalExitFunc
		})

		// Mock getwd to return an error
		getwd = func() (string, error) {
			return "", errors.New("mock error")
		}

		// Mock exitFunc to capture the exit code
		var exitCode int
		exitFunc = func(code int) {
			exitCode = code
		}

		t.Run("GetwdError", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return "", errors.New("mock error") }
			Initialize(mocks.Container)

			// Capture the output to os.Stderr
			stderr := captureStderr(func() {
				getProjectConfigPath()
			})

			// Verify the error message
			expectedErrorMessage := "error getting current working directory, mock error\n"
			if stderr != expectedErrorMessage {
				t.Errorf("expected error message %q, got %q", expectedErrorMessage, stderr)
			}

			// Verify the exit code
			if exitCode != 1 {
				t.Errorf("expected exit code 1, got %d", exitCode)
			}
		})

		t.Run("WindsorYaml", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			tempDir := t.TempDir()
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }
			Initialize(mocks.Container)

			// Create a temporary windsor.yaml file in the project root
			windsorYamlPath := filepath.Join(tempDir, "windsor.yaml")
			file, err := os.Create(windsorYamlPath)
			if err != nil {
				t.Fatalf("Failed to create windsor.yaml: %v", err)
			}
			file.Close()

			// When getProjectConfigPath is called
			projectConfigPath := getProjectConfigPath()

			// Then projectConfigPath should be set to windsor.yaml
			if projectConfigPath != windsorYamlPath {
				t.Errorf("expected projectConfigPath to be %s, got %s", windsorYamlPath, projectConfigPath)
			}
		})

		t.Run("WindsorYml", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mocks := mocks.CreateSuperMocks()
			mocks.CLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			tempDir := t.TempDir()
			mocks.Shell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }
			Initialize(mocks.Container)

			// Create a temporary windsor.yml file in the project root
			windsorYmlPath := filepath.Join(tempDir, "windsor.yml")
			file, err := os.Create(windsorYmlPath)
			if err != nil {
				t.Fatalf("Failed to create windsor.yml: %v", err)
			}
			file.Close()

			// When getProjectConfigPath is called
			projectConfigPath := getProjectConfigPath()

			// Then projectConfigPath should be set to windsor.yml
			if projectConfigPath != windsorYmlPath {
				t.Errorf("expected projectConfigPath to be %s, got %s", windsorYmlPath, projectConfigPath)
			}
		})
	})
}
