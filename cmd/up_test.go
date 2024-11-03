package cmd

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/vm"
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
		mocks.ColimaVM.UpFunc = func(verbose ...bool) error {
			return nil
		}
		mocks.ColimaVM.InfoFunc = func() (interface{}, error) {
			return &vm.VMInfo{
				Address: "192.168.5.2",
				Arch:    "x86_64",
				CPUs:    2,
				Disk:    10.0,
				Memory:  4.0,
				Name:    "mock-vm",
				Runtime: "docker",
				Status:  "Running",
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

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
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

	t.Run("ErrorSettingSSHClientConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.SSHClient.SetClientConfigFileFunc = func(configStr, hostname string) error {
			return fmt.Errorf("mock error setting SSH client config")
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
		expectedOutput := "Error setting SSH client config: mock error setting SSH client config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorListingNetworkInterfaces", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "", fmt.Errorf("mock error executing command to list network interfaces")
			}
			return "Executed: " + command, nil
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
		expectedOutput := "Error executing command to list network interfaces: mock error executing command to list network interfaces"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoDockerBridgeInterfaceFound", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "eth0\nlo", nil // No interface starting with "br-"
			}
			return "Executed: " + command, nil
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
		expectedOutput := "Error: No interface starting with 'br-' found"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingNetworkInterfaceIP", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "ls" && args[0] == "/sys/class/net" {
				return "br-mock-interface", nil
			}
			return "Executed: " + command, nil
		}
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "Executed: " + command, nil
		}
		Initialize(mocks.Container)

		// Mock netInterfaces to return an error
		originalNetInterfaces := netInterfaces
		netInterfaces = func() ([]net.Interface, error) {
			return nil, fmt.Errorf("mock error getting network interfaces")
		}
		t.Cleanup(func() {
			netInterfaces = originalNetInterfaces
		})

		// When the up command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error getting network interfaces: mock error getting network interfaces"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorSettingIPTablesRule", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" {
				return "", fmt.Errorf("mock error setting iptables rule")
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
		expectedOutput := "Error checking iptables rule: mock error setting iptables rule"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorAddingRoute", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helpers
		mocks := successConfig()
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "sudo" && args[0] == "route" {
				return "", fmt.Errorf("mock error adding route")
			}
			return "Executed: " + command, nil
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
		expectedOutput := "failed to add route: mock error adding route"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
