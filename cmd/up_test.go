package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
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
		Initialize(mocks.Injector)

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
		Initialize(mocks.Injector)

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
		Initialize(mocks.Injector)

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
		Initialize(mocks.Injector)

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
}
