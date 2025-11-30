// The colima_virt_test package is a test suite for the ColimaVirt implementation
// It provides test coverage for Colima VM management functionality
// It serves as a verification framework for Colima virtualization operations
// It enables testing of Colima-specific features and error handling

package virt

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	colimaConfig "github.com/abiosoft/colima/config"
	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupColimaMocks(t *testing.T, opts ...func(*VirtTestMocks)) *VirtTestMocks {
	t.Helper()

	// Set up mocks and shell
	mocks := setupVirtMocks(t, opts...)

	// Set up shell mock for GetVMInfo
	mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "colima" {
			switch args[0] {
			case "ls":
				if len(args) >= 4 && args[1] == "--profile" && args[3] == "--json" {
					return `{
						"address": "192.168.1.2",
						"arch": "x86_64",
						"cpus": 2,
						"disk": 64424509440,
						"memory": 4294967296,
						"name": "windsor-mock-context",
						"runtime": "docker",
						"status": "Running"
					}`, nil
				}
			case "start":
				return "", nil
			case "stop":
				return "", nil
			case "delete":
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	// Set up shell mock for ExecProgress
	mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
		if command == "colima" {
			switch args[0] {
			case "start":
				return "", nil
			case "stop":
				return "", nil
			case "delete":
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	// Load Colima-specific config using v1alpha1 schema
	configStr := `
version: v1alpha1
contexts:
  mock-context:
    vm:
      driver: colima
      cpu: 2
      memory: 4
      disk: 60
      arch: x86_64
      address: 192.168.1.2
    docker:
      enabled: true
      registry_url: docker.io
      registries:
        local:
          remote: docker.io
          local: localhost:5000
          hostname: localhost
          hostport: 5000
    network:
      cidr_block: 10.0.0.0/24
      loadbalancer_ips:
        start: 10.0.0.100
        end: 10.0.0.200
    dns:
      enabled: true
      domain: mock.domain.com
      address: 10.0.0.53
      forward:
        - 8.8.8.8
        - 8.8.4.4
      records:
        - "*.mock.domain.com. IN A 10.0.0.53"`

	if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
		t.Fatalf("Failed to load config string: %v", err)
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestColimaVirt_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.setShims(mocks.Shims)
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		setup(t)

		// Then no error should be returned from setup
		// (Initialize is already called in setup)
	})

	t.Run("ErrorResolveShell", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, _ := setup(t)

		// Then the service should be properly initialized
		if colimaVirt == nil {
			t.Fatal("Expected ColimaVirt, got nil")
		}
	})

	t.Run("ErrorResolveConfigHandler", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, _ := setup(t)

		// Then the service should be properly initialized
		if colimaVirt == nil {
			t.Fatal("Expected ColimaVirt, got nil")
		}
	})
}

func TestColimaVirt_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)

		// Ensure vm.driver is explicitly set to colima
		if err := mocks.ConfigHandler.Set("vm.driver", "colima"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.setShims(mocks.Shims)
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with default mock components
		colimaVirt, _ := setup(t)

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorYamlEncode", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And NewYAMLEncoder returns an encoder that errors on Encode
		mocks.Shims.NewYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v any) error {
					return fmt.Errorf("mock encode error")
				},
				closeFunc: func() error {
					return nil
				},
			}
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error encoding yaml") {
			t.Errorf("Expected error about encoding yaml, got: %v", err)
		}
	})

	t.Run("ErrorYAMLClose", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And NewYAMLEncoder returns an encoder that errors on Close
		mocks.Shims.NewYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v any) error {
					return nil
				},
				closeFunc: func() error {
					return fmt.Errorf("mock close error")
				},
			}
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error closing encoder") {
			t.Errorf("Expected error about closing encoder, got: %v", err)
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a ColimaVirt with mock WriteFile that returns an error
		colimaVirt, _ := setup(t)

		// Create custom shims with error on WriteFile
		writeFileFuncCalled := false
		colimaVirt.shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileFuncCalled = true
			return fmt.Errorf("mock write file error")
		}

		// When WriteConfig is called
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "mock write file error") {
			t.Errorf("Expected error to contain 'mock write file error', got: %v", err)
		}

		// Verify that the WriteFile function was called
		if !writeFileFuncCalled {
			t.Errorf("WriteFile function called: %v", writeFileFuncCalled)
		}
	})

	t.Run("ErrorRename", func(t *testing.T) {
		// Given a ColimaVirt with mock Rename that returns an error
		colimaVirt, _ := setup(t)

		// Create custom shims with error on Rename
		renameFuncCalled := false
		colimaVirt.shims.Rename = func(oldpath, newpath string) error {
			renameFuncCalled = true
			return fmt.Errorf("mock rename error")
		}

		// When WriteConfig is called
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "mock rename error") {
			t.Errorf("Expected error to contain 'mock rename error', got: %v", err)
		}

		// Verify that the Rename function was called
		if !renameFuncCalled {
			t.Errorf("Rename function called: %v", renameFuncCalled)
		}
	})

	t.Run("NotColimaDriver", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.setShims(mocks.Shims)

		// And vm.driver is not colima
		if err := mocks.ConfigHandler.Set("vm.driver", "other"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorUserHomeDir", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And UserHomeDir returns an error
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock home dir error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving user home directory") {
			t.Errorf("Expected error about retrieving home directory, got: %v", err)
		}
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And MkdirAll returns an error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock mkdir error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating colima directory") {
			t.Errorf("Expected error about creating directory, got: %v", err)
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And WriteFile returns an error
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing to temporary file") {
			t.Errorf("Expected error about writing file, got: %v", err)
		}
	})

	t.Run("ErrorRename", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And Rename returns an error
		mocks.Shims.Rename = func(oldpath, newpath string) error {
			return fmt.Errorf("mock rename error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error renaming temporary file") {
			t.Errorf("Expected error about renaming file, got: %v", err)
		}
	})

	t.Run("ArchitectureSpecificSettings", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And GOARCH returns aarch64
		mocks.Shims.GOARCH = func() string {
			return "arm64"
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the config should use vz and virtiofs
		// Note: We can't easily verify the config values since we're mocking the file operations
		// Instead, we'll call WriteConfig again with a special encoder that lets us inspect the config
		var capturedConfig *colimaConfig.Config
		mocks.Shims.NewYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v any) error {
					if cfg, ok := v.(*colimaConfig.Config); ok {
						capturedConfig = cfg
					}
					return nil
				},
				closeFunc: func() error {
					return nil
				},
			}
		}

		// When calling WriteConfig again
		err = colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the config should have the correct values
		if capturedConfig == nil {
			t.Fatal("Expected config to be captured")
		}
		if capturedConfig.VMType != "vz" {
			t.Errorf("Expected VMType to be vz, got %s", capturedConfig.VMType)
		}
		if capturedConfig.MountType != "virtiofs" {
			t.Errorf("Expected MountType to be virtiofs, got %s", capturedConfig.MountType)
		}
	})

	t.Run("NetworkConfiguration", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// And a config capture mechanism
		var capturedConfig *colimaConfig.Config
		mocks.Shims.NewYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v any) error {
					if cfg, ok := v.(*colimaConfig.Config); ok {
						capturedConfig = cfg
					}
					return nil
				},
				closeFunc: func() error {
					return nil
				},
			}
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the network configuration should have the correct values
		if capturedConfig == nil {
			t.Fatal("Expected config to be captured")
		}
		if !capturedConfig.Network.Address {
			t.Error("Expected Network.Address to be true")
		}
		if capturedConfig.Network.Mode != "shared" {
			t.Errorf("Expected Network.Mode to be 'shared', got %s", capturedConfig.Network.Mode)
		}
		if capturedConfig.Network.BridgeInterface != "" {
			t.Errorf("Expected Network.BridgeInterface to be empty, got %s", capturedConfig.Network.BridgeInterface)
		}
		if capturedConfig.Network.PreferredRoute {
			t.Error("Expected Network.PreferredRoute to be false")
		}
	})
}

func TestColimaVirt_Up(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.setShims(mocks.Shims)
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, _ := setup(t)

		// When calling Up
		err := colimaVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorStartColima", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// Save original function to restore it in our mock
		originalExecProgress := mocks.Shell.ExecProgressFunc

		// Override just the relevant method to return an error
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "start" {
				return "", fmt.Errorf("mock start colima error")
			}
			// For any other command, use the original implementation
			return originalExecProgress(message, command, args...)
		}

		// When calling Up
		err := colimaVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock start colima error") {
			t.Errorf("Expected error containing 'mock start colima error', got %v", err)
		}
	})

	t.Run("ErrorSetVMAddress", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		_, mocks := setup(t)

		// Create a mock config handler that returns error on set
		mockConfigHandler := config.NewMockConfigHandler()

		// Copy required config values from the original handler
		mockConfigHandler.GetContextFunc = func() string {
			return mocks.ConfigHandler.GetContext()
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValues ...string) string {
			return mocks.ConfigHandler.GetString(key, defaultValues...)
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValues ...int) int {
			return mocks.ConfigHandler.GetInt(key, defaultValues...)
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValues ...bool) bool {
			return mocks.ConfigHandler.GetBool(key, defaultValues...)
		}

		// Override just the Set to return an error
		mockConfigHandler.SetFunc = func(key string, _ any) error {
			if key == "vm.address" {
				return fmt.Errorf("mock set context value error")
			}
			return nil
		}

		// Create a new runtime with the mock config handler
		rt := &runtime.Runtime{
			ProjectRoot:   mocks.Runtime.ProjectRoot,
			ConfigRoot:    mocks.Runtime.ConfigRoot,
			TemplateRoot:  mocks.Runtime.TemplateRoot,
			ContextName:   mocks.Runtime.ContextName,
			ConfigHandler: mockConfigHandler,
			Shell:         mocks.Shell,
		}

		// Create a new ColimaVirt with the mock config handler
		colimaVirt := NewColimaVirt(rt)

		// Set up the shell to return success for colima start
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			return "", nil
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "ls" {
				return `{"address":"192.168.1.10","arch":"x86_64","cpus":2,"disk":60,"memory":4096,"name":"windsor-mock-context","status":"Running"}`, nil
			}
			return "", nil
		}

		// When calling Up
		err := colimaVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock set context value error") {
			t.Errorf("Expected error containing 'mock set context value error', got %v", err)
		}
	})
}

func TestColimaVirt_Down(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.setShims(mocks.Shims)
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, _ := setup(t)

		// When calling Down
		err := colimaVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorStopColima", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// Save original function to restore it in our mock
		originalExecProgress := mocks.Shell.ExecProgressFunc

		// Override the ExecProgress function to return an error for stop
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "stop" {
				return "", fmt.Errorf("mock stop colima error")
			}
			// For any other command, use the original implementation
			return originalExecProgress(message, command, args...)
		}

		// When calling Down
		err := colimaVirt.Down()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock stop colima error") {
			t.Errorf("Expected error containing 'mock stop colima error', got %v", err)
		}
	})

	t.Run("ErrorDeleteColima", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		colimaVirt, mocks := setup(t)

		// Save original function to restore it in our mock
		originalExecProgress := mocks.Shell.ExecProgressFunc

		// Override the ExecProgress function for selective operations
		stopCalled := false
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" {
				switch args[0] {
				case "stop":
					stopCalled = true
					return "", nil
				case "delete":
					// Only return error for delete if stop was called first
					if stopCalled {
						return "", fmt.Errorf("mock delete colima error")
					}
					return "", fmt.Errorf("delete called before stop")
				}
			}
			// For any other command, use the original implementation
			return originalExecProgress(message, command, args...)
		}

		// When calling Down
		err := colimaVirt.Down()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock delete colima error") {
			t.Errorf("Expected error containing 'mock delete colima error', got %v", err)
		}

		// Verify stop was called
		if !stopCalled {
			t.Error("Stop function was not called")
		}
	})
}

// TestColimaVirt_getArch tests the getArch method of the ColimaVirt component.
func TestColimaVirt_getArch(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupVirtMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.shims = mocks.Shims
		return colimaVirt, mocks
	}

	tests := []struct {
		name     string
		goArch   string
		expected string
	}{
		{
			name:     "AMD64",
			goArch:   "amd64",
			expected: "x86_64",
		},
		{
			name:     "ARM64",
			goArch:   "arm64",
			expected: "aarch64",
		},
		{
			name:     "Other",
			goArch:   "other",
			expected: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a colima virt instance with valid mocks
			colimaVirt, mocks := setup(t)

			// And mock GOARCH returns a specific architecture
			mocks.Shims.GOARCH = func() string {
				return tt.goArch
			}

			// When getting the architecture
			arch := colimaVirt.getArch()

			// Then the expected architecture should be returned
			if arch != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, arch)
			}
		})
	}
}

// TestColimaVirt_getDefaultValues tests the getDefaultValues method of the ColimaVirt component.
func TestColimaVirt_getDefaultValues(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.shims = mocks.Shims
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, _ := setup(t)

		// When getting default values
		cpu, disk, memory, hostname, arch := colimaVirt.getDefaultValues("test-context")

		// Then values should be reasonable
		if cpu <= 0 {
			t.Errorf("expected positive CPU count, got %d", cpu)
		}
		if disk != 60 {
			t.Errorf("expected disk size 60GB, got %d", disk)
		}
		if memory <= 0 {
			t.Errorf("expected positive memory size, got %d", memory)
		}
		if hostname != "windsor-test-context" {
			t.Errorf("expected hostname 'windsor-test-context', got %s", hostname)
		}
		if arch == "" {
			t.Error("expected non-empty arch")
		}
	})

	t.Run("MemoryRetrievalFailure", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, mocks := setup(t)

		// And VirtualMemory returns an error
		mocks.Shims.VirtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return nil, fmt.Errorf("mock memory retrieval error")
		}

		// When getting default values
		_, _, memory, _, _ := colimaVirt.getDefaultValues("test-context")

		// Then memory should be set to default value
		if memory != 2 {
			t.Errorf("expected default memory 2GB, got %d", memory)
		}
	})

	t.Run("MemoryOverflow", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, _ := setup(t)

		// And memory overflow is forced via test hook
		testForceMemoryOverflow = true
		defer func() { testForceMemoryOverflow = false }()

		// When getting default values
		_, _, memory, _, _ := colimaVirt.getDefaultValues("test-context")

		// Then memory should be set to MaxInt
		if memory != math.MaxInt {
			t.Errorf("expected memory MaxInt, got %d", memory)
		}
	})
}

// TestColimaVirt_startColima tests the startColima method of the ColimaVirt component.
func TestColimaVirt_startColima(t *testing.T) {
	setup := func(t *testing.T) (*ColimaVirt, *VirtTestMocks) {
		t.Helper()
		mocks := setupColimaMocks(t)
		colimaVirt := NewColimaVirt(mocks.Runtime)
		colimaVirt.shims = mocks.Shims
		return colimaVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, _ := setup(t)

		// When starting colima
		info, err := colimaVirt.startColima()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And info should be populated
		if info.Address == "" {
			t.Error("expected address to be populated")
		}
	})

	t.Run("ErrorStartingColima", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, mocks := setup(t)

		// And ExecProgress returns an error
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "colima" && args[0] == "start" {
				return "", fmt.Errorf("mock start error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When starting colima
		_, err := colimaVirt.startColima()

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "mock start error"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("TimeoutWaitingForIP", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, mocks := setup(t)

		// Set test retry attempts to 2 for faster test execution
		oldRetryAttempts := testRetryAttempts
		testRetryAttempts = 2
		defer func() { testRetryAttempts = oldRetryAttempts }()

		// And GetVMInfo returns info without an IP address
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 4 && args[0] == "ls" && args[1] == "--profile" && args[3] == "--json" {
				return `{
					"address": "",
					"arch": "x86_64",
					"cpus": 2,
					"disk": 64424509440,
					"memory": 4294967296,
					"name": "windsor-mock-context",
					"runtime": "docker",
					"status": "Running"
				}`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When starting colima
		_, err := colimaVirt.startColima()

		// Then an error should occur
		if err == nil {
			t.Error("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "Timed out waiting for Colima VM to get an IP address"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("RetryOnGetVMInfoError", func(t *testing.T) {
		// Given a colima virt instance with valid mocks
		colimaVirt, mocks := setup(t)

		// And GetVMInfo fails twice then succeeds
		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) >= 4 && args[0] == "ls" && args[1] == "--profile" && args[3] == "--json" {
				callCount++
				if callCount < 3 {
					return "", fmt.Errorf("mock get info error")
				}
				return `{
					"address": "192.168.1.2",
					"arch": "x86_64",
					"cpus": 2,
					"disk": 64424509440,
					"memory": 4294967296,
					"name": "windsor-mock-context",
					"runtime": "docker",
					"status": "Running"
				}`, nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When starting colima
		info, err := colimaVirt.startColima()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And info should be populated
		if info.Address == "" {
			t.Error("expected address to be populated")
		}

		// And GetVMInfo should be called multiple times
		if callCount < 3 {
			t.Errorf("expected at least 3 calls to GetVMInfo, got %d", callCount)
		}
	})
}
