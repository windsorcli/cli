package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/network"
)

func TestUpCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})
	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string][]string{
					"web": {"service1"},
					"db":  {"service2"},
				},
			}, nil
		}
		mocks.ColimaHelper.UpFunc = func() error {
			return nil
		}
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.ColimaInfo{
				Address: "192.168.5.2",
				Arch:    "x86_64",
				CPUs:    4,
				Disk:    20.0,
				Memory:  8.0,
				Name:    "colima",
				Runtime: "docker",
				Status:  "Running",
			}, nil
		}
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", nil
		}
		Initialize(mocks.Container)

		// When the up command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should contain the welcome message
		expectedOutput := "Welcome to the Windsor Environment"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingContextConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)
		// Given a config handler that returns an error
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error getting context config")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error getting context configuration: mock error getting context config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingContextConfigNonVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns an error
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error getting context config")
		}
		Initialize(mocks.Container)

		// When the up command is executed without verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should be empty
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoContextConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns nil context config
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, nil
		}
		Initialize(mocks.Container)

		// When the up command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should be empty
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorRunningColimaHelperUp", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a ColimaHelper that returns an error when running Up
		mocks.ColimaHelper.UpFunc = func() error {
			return fmt.Errorf("mock error running ColimaHelper Up")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error running ColimaHelper Up command: mock error running ColimaHelper Up"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingColimaInfo", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a ColimaHelper that returns an error when getting Info
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return nil, fmt.Errorf("mock error retrieving Colima info")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error retrieving Colima info: mock error retrieving Colima info"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorRunningDockerHelperUp", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config with Docker enabled
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}

		// And a DockerHelper that returns an error when running Up
		mocks.DockerHelper.UpFunc = func() error {
			return fmt.Errorf("mock error running DockerHelper Up")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error running DockerHelper Up command: mock error running DockerHelper Up"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingDockerInfo", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config with Docker enabled
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}

		// And a DockerHelper that returns an error when retrieving Docker info
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return nil, fmt.Errorf("mock error retrieving Docker info")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error retrieving Docker info: mock error retrieving Docker info"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorConfiguringNetwork", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config with Docker enabled
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}

		// And a DockerHelper that returns valid Docker info
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string][]string{
					"web": {"service1"},
					"db":  {"service2"},
				},
			}, nil
		}

		// And a networkManager that returns an error when configuring the network
		mocks.NetworkManager.ConfigureFunc = func(networkConfig *network.NetworkConfig) (*network.NetworkConfig, error) {
			return nil, fmt.Errorf("mock error configuring network")
		}
		Initialize(mocks.Container)

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error configuring network: mock error configuring network"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
