package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/virt"
)

func TestUpCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	// Common success configuration
	successConfig := func() mocks.SuperMocks {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled:     ptrBool(true),
					NetworkCIDR: ptrString("192.168.5.0/24"),
				},
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
					CPU:    ptrInt(2),
					Memory: ptrInt(4),
					Disk:   ptrInt(10),
				},
			}, nil
		}
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string]helpers.ServiceInfo{
					"web": {Role: "web", IP: "192.168.1.2"},
					"db":  {Role: "db", IP: "192.168.1.2"},
				},
			}, nil
		}
		mocks.ColimaVirt.UpFunc = func(verbose ...bool) error {
			return nil
		}
		mocks.ColimaVirt.GetContainerInfoFunc = func() ([]virt.ContainerInfo, error) {
			return []virt.ContainerInfo{
				{
					Name: "mock-vm",
				},
			}, nil
		}
		mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
				return "", fmt.Errorf("Bad rule")
			}
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-mock-interface", nil
			}
			return "Executed: " + command, nil
		}
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "route" {
				return "mock-route-output", nil
			}
			return "Executed: " + command, nil
		}
		mocks.SSHClient.SetClientConfigFileFunc = func(configStr, hostname string) error {
			return nil
		}
		return mocks
	}

	t.Run("ErrorGettingContext", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a context instance that returns an error
		mocks := successConfig()
		mocks.ContextInstance.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting context")
		}
		Initialize(mocks.Container)

		// When the up command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error getting context: mock error getting context"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingContextConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)
		// Given a config handler that returns an error
		mocks := successConfig()
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
		mocks := successConfig()
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
		mocks := successConfig()
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

	t.Run("ErrorRunningDockerHelperUp", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a config handler that returns a valid context config with Docker enabled
		mocks := successConfig()
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
		mocks := successConfig()
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
}
