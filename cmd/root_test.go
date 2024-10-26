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
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to create a new container and register mock handlers
func setupContainer(
	mockCLIConfigHandler config.ConfigHandler,
	mockProjectConfigHandler config.ConfigHandler,
	mockShell *shell.MockShell,
	mockTerraformHelper helpers.Helper,
	mockAwsHelper helpers.Helper,
	mockColimaHelper helpers.Helper,
	mockDockerHelper helpers.Helper,
) di.ContainerInterface {
	container := di.NewContainer()

	// Create simple mock handlers if not provided
	if mockCLIConfigHandler == nil {
		mockCLIConfigHandler := config.NewMockConfigHandler()
		mockCLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "value", nil
		}
	}
	if mockProjectConfigHandler == nil {
		mockProjectConfigHandler := config.NewMockConfigHandler()
		mockProjectConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "value", nil
		}
	}
	if mockShell == nil {
		mockShell, _ = shell.NewMockShell("unix")
	}
	if mockTerraformHelper == nil {
		mockTerraformHelper = helpers.NewMockHelper()
	}
	if mockAwsHelper == nil {
		mockAwsHelper = helpers.NewMockHelper()
	}
	if mockColimaHelper == nil {
		mockColimaHelper = helpers.NewMockHelper()
	}
	if mockDockerHelper == nil {
		mockDockerHelper = helpers.NewMockHelper()
	}

	container.Register("cliConfigHandler", mockCLIConfigHandler)
	container.Register("projectConfigHandler", mockProjectConfigHandler)
	container.Register("shell", mockShell)
	container.Register("terraformHelper", mockTerraformHelper)
	container.Register("awsHelper", mockAwsHelper)
	container.Register("colimaHelper", mockColimaHelper)
	container.Register("dockerHelper", mockDockerHelper)
	Initialize(container)

	// Ensure handlers are set correctly
	instance, err := container.Resolve("cliConfigHandler")
	if err != nil {
		panic("Error resolving cliConfigHandler: " + err.Error())
	}
	cliConfigHandler, _ = instance.(config.ConfigHandler)

	instance, err = container.Resolve("projectConfigHandler")
	if err != nil {
		panic("Error resolving projectConfigHandler: " + err.Error())
	}
	projectConfigHandler, _ = instance.(config.ConfigHandler)

	instance, err = container.Resolve("shell")
	if err != nil {
		panic("Error resolving shell: " + err.Error())
	}
	shellInstance, _ = instance.(shell.Shell)

	instance, err = container.Resolve("terraformHelper")
	if err != nil {
		panic("Error resolving terraformHelper: " + err.Error())
	}
	terraformHelper, _ = instance.(helpers.Helper)

	instance, err = container.Resolve("awsHelper")
	if err != nil {
		panic("Error resolving awsHelper: " + err.Error())
	}
	awsHelper, _ = instance.(helpers.Helper)

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
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
			projectConfigHandler := config.NewMockConfigHandler()
			projectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			projectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

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

		t.Run("NoProjectConfigHandler", func(t *testing.T) {
			// Given no project config handler is registered
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}
			cliConfigHandler = mockCLIConfigHandler
			projectConfigHandler = nil

			// When preRunLoadConfig is executed
			err := preRunLoadConfig(nil, nil)

			// Then an error should be returned
			if err == nil {
				t.Fatalf("preRunLoadConfig() expected error, got nil")
			}
			expectedError := "projectConfigHandler is not initialized"
			if err.Error() != expectedError {
				t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
			}
		})

		t.Run("CLIConfigLoadError", func(t *testing.T) {
			// Given CLI config handler returns an error on LoadConfig
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return errors.New("mock load error") }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
		t.Run("ProjectConfigLoadError", func(t *testing.T) {
			// Given project config handler returns an error on LoadConfig
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return errors.New("mock load error") }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

			// Ensure the project config path is set
			tempDir := t.TempDir()
			windsorYamlPath := filepath.Join(tempDir, "windsor.yaml")
			file, err := os.Create(windsorYamlPath)
			if err != nil {
				t.Fatalf("Failed to create windsor.yaml: %v", err)
			}
			file.Close()
			mockShell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }

			// When preRunLoadConfig is executed
			err = preRunLoadConfig(nil, nil)

			// Then an error should be returned
			if err == nil {
				t.Fatalf("preRunLoadConfig() expected error, got nil")
			}
			expectedError := "error loading project config: mock load error"
			if err.Error() != expectedError {
				t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
			}
		})
	})

	t.Run("Success", func(t *testing.T) {
		t.Run("ValidConfigHandlers", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
			projectConfigHandler = nil

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
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
		t.Run("GetwdError", func(t *testing.T) {
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

			// Given valid config handlers and shell instance
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) { return "", errors.New("mock error") }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			tempDir := t.TempDir()
			mockShell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			tempDir := t.TempDir()
			mockShell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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

	t.Run("GetProjectConfigPath", func(t *testing.T) {
		t.Run("WindsorYml", func(t *testing.T) {
			// Given valid config handlers and shell instance
			mockCLIConfigHandler := config.NewMockConfigHandler()
			mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockProjectConfigHandler := config.NewMockConfigHandler()
			mockProjectConfigHandler.LoadConfigFunc = func(path string) error { return nil }
			mockProjectConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "value", nil
			}

			mockShell, _ := shell.NewMockShell("unix")
			tempDir := t.TempDir()
			mockShell.GetProjectRootFunc = func() (string, error) { return tempDir, nil }
			setupContainer(mockCLIConfigHandler, mockProjectConfigHandler, mockShell, nil, nil, nil, nil)

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
