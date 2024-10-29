package cmd

import (
	"bytes"
	"errors"
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

		// Given a valid config handler, shell, and helper
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: func(s string) *string { return &s }("colima"),
				},
				Docker: &config.DockerConfig{
					Enabled:     func(b bool) *bool { return &b }(true),
					NetworkCIDR: func(s string) *string { return &s }("192.168.5.0/24"),
				},
				DNS: &config.DNSConfig{
					Name: func(s string) *string { return &s }("test.local"),
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
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string]map[string]string{
					"dns.test": {"ip": "192.168.5.1"},
				},
			}, nil
		}
		mocks.NetworkManager.ConfigureFunc = func(config *network.NetworkConfig) (*network.NetworkConfig, error) {
			return config, nil
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
		expectedOutput := "Welcome to the Windsor Environment 📐"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given no context configuration
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("error getting config")
		}
		Initialize(mocks.Container)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Then the error should be as expected
		expectedError := "Error getting context configuration: error getting config"
		if err == nil || err.Error() != expectedError {
			t.Errorf("Expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("NoConfig", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given no context configuration
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, nil
		}
		Initialize(mocks.Container)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()

		// Then there should be no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ColimaUpError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a ColimaHelper that returns an error on Up
		mocks := mocks.CreateSuperMocks()
		mocks.ColimaHelper.UpFunc = func() error {
			return errors.New("colima up error")
		}
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: func(s string) *string { return &s }("colima"),
				},
			}, nil
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error running ColimaHelper Up command: colima up error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ColimaInfoError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a ColimaHelper that returns an error on Info
		mocks := mocks.CreateSuperMocks()
		mocks.ColimaHelper.UpFunc = func() error {
			return nil
		}
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return nil, errors.New("colima info error")
		}
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: func(s string) *string { return &s }("colima"),
				},
			}, nil
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error getting ColimaHelper info: colima info error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("DockerUpError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a DockerHelper that returns an error on Up
		mocks := mocks.CreateSuperMocks()
		mocks.DockerHelper.UpFunc = func() error {
			return errors.New("docker up error")
		}
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: func(b bool) *bool { return &b }(true),
				},
			}, nil
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error running DockerHelper Up command: docker up error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("DockerInfoError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a DockerHelper that returns an error on Info
		mocks := mocks.CreateSuperMocks()
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return nil, errors.New("docker info error")
		}
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: func(b bool) *bool { return &b }(true),
				},
			}, nil
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error getting DockerHelper info: docker info error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("DNSIPIsDefined", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helper with DNS IP defined
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
				Docker: &config.DockerConfig{
					Enabled:     ptrBool(true),
					NetworkCIDR: ptrString("192.168.5.0/24"),
				},
				DNS: &config.DNSConfig{
					Name: ptrString("example.com"),
					IP:   ptrString("192.168.5.3"),
				},
			}, nil
		}
		mocks.ColimaHelper.UpFunc = func() error {
			return nil
		}
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.ColimaInfo{
				Address: "192.168.5.2",
			}, nil
		}
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string]map[string]string{
					"dns.test": {
						"role": "dns",
						"ip":   "192.168.5.3",
					},
				},
			}, nil
		}
		mocks.NetworkManager.ConfigureFunc = func(config *network.NetworkConfig) (*network.NetworkConfig, error) {
			return config, nil
		}
		Initialize(mocks.Container)

		// Capture stdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should indicate the DNS IP from the Docker info
		expectedSubstring := "192.168.5.3"
		if !strings.Contains(output, expectedSubstring) {
			t.Errorf("Expected output to contain %q, got %q", expectedSubstring, output)
		}
	})

	t.Run("DNSNotDefined", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a valid config handler, shell, and helper with DNS not defined
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: func(s string) *string { return &s }("colima"),
				},
				Docker: &config.DockerConfig{
					Enabled:     func(b bool) *bool { return &b }(true),
					NetworkCIDR: func(s string) *string { return &s }("192.168.5.0/24"),
				},
				DNS: nil,
			}, nil
		}
		mocks.ColimaHelper.UpFunc = func() error {
			return nil
		}
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.ColimaInfo{
				Address: "192.168.5.2",
			}, nil
		}
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string]map[string]string{
					"dns.test": {
						"ip": "192.168.5.3",
					},
				},
			}, nil
		}
		mocks.NetworkManager.ConfigureFunc = func(config *network.NetworkConfig) (*network.NetworkConfig, error) {
			return config, nil
		}
		Initialize(mocks.Container)

		// Capture stdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should indicate the DNS is not defined
		expectedOutput := "Welcome to the Windsor Environment 📐"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NetworkConfigureError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a NetworkManager that returns an error on Configure
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: func(s string) *string { return &s }("colima"),
				},
				Docker: &config.DockerConfig{
					Enabled:     func(b bool) *bool { return &b }(true),
					NetworkCIDR: func(s string) *string { return &s }("192.168.5.0/24"),
				},
				DNS: &config.DNSConfig{
					Name: func(s string) *string { return &s }("test.local"),
				},
			}, nil
		}
		mocks.ColimaHelper.UpFunc = func() error {
			return nil
		}
		mocks.ColimaHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.ColimaInfo{
				Address: "192.168.5.2",
			}, nil
		}
		mocks.DockerHelper.UpFunc = func() error {
			return nil
		}
		mocks.DockerHelper.InfoFunc = func() (interface{}, error) {
			return &helpers.DockerInfo{
				Services: map[string]map[string]string{
					"dns.test": {
						"ip": "192.168.5.3",
					},
				},
			}, nil
		}
		mocks.NetworkManager.ConfigureFunc = func(config *network.NetworkConfig) (*network.NetworkConfig, error) {
			return nil, errors.New("network configure error")
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error configuring network: network configure error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
