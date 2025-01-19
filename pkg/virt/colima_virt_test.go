package virt

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeColimaVmMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the injector
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	mockConfigHandler.LoadConfigFunc = func(path string) error { return nil }

	// Implement GetContextFunc on mock context
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	// Set up the mock config handler to return specific configuration values
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "vm.driver":
			return "colima"
		case "vm.arch":
			return "x86_64"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
		switch key {
		case "vm.cpu":
			return 4 // Assume a realistic CPU count
		case "vm.disk":
			return 60 // Assume a realistic disk size in GB
		case "vm.memory":
			return 8 // Assume a realistic memory size in GB
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
	}

	// Mock realistic responses for ExecSilent
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "colima" && len(args) > 0 && args[0] == "ls" {
			return `{
				"address": "192.168.5.2",
				"arch": "x86_64",
				"cpus": 4,
				"disk": 64424509440,
				"memory": 8589934592,
				"name": "windsor-mock-context",
				"runtime": "docker",
				"status": "Running"
			}`, nil
		}
		return "", fmt.Errorf("command not recognized")
	}

	return &MockComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

// TestColimaVirt_Up tests the Up method of ColimaVirt.
func TestColimaVirt_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// When calling Up
		err := colimaVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorStartingColima", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Up
		err := colimaVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorSettingVMAddress", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate an error when setting the VM address
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "vm.address" {
				return fmt.Errorf("mock set context value error")
			}
			return nil
		}

		// When calling Up
		err := colimaVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "failed to set VM address in config handler: mock set context value error" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})
}

// TestColimaVirt_Down tests the Down method of ColimaVirt.
func TestColimaVirt_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate a successful stop
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "VM stopped", nil
		}

		// When calling Down
		err := colimaVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorExecProgress", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Down
		err := colimaVirt.Down()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVirt_GetVMInfo tests the GetVMInfo method of ColimaVirt.
func TestColimaVirt_GetVMInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate a successful info retrieval
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return `{"address":"192.168.5.2","arch":"x86_64","cpus":4,"disk":64424509440,"memory":8589934592,"name":"test-vm","runtime":"docker","status":"Running"}`, nil
		}

		// When calling GetVMInfo
		info, err := colimaVirt.GetVMInfo()

		// Then no error should be returned and info should be correct
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedInfo := VMInfo{
			Address: "192.168.5.2",
			Arch:    "x86_64",
			CPUs:    4,
			Disk:    60,
			Memory:  8,
			Name:    "test-vm",
		}

		if info != expectedInfo {
			t.Errorf("Expected VMInfo to be %+v, got %+v", expectedInfo, info)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling GetVMInfo
		_, err := colimaVirt.GetVMInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorUnmarshallingColimaInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "invalid json", nil
		}

		// Mock jsonUnmarshal to simulate an error
		originalJsonUnmarshal := jsonUnmarshal
		defer func() { jsonUnmarshal = originalJsonUnmarshal }()
		jsonUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("mock unmarshal error")
		}

		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// When calling GetVMInfo
		_, err := colimaVirt.GetVMInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "mock unmarshal error" {
			t.Errorf("Expected error message 'mock unmarshal error', got %v", err)
		}
	})
}

func TestColimaVirt_PrintInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate a successful info retrieval
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return `{"address":"192.168.5.2","arch":"x86_64","cpus":4,"disk":64424509440,"memory":8589934592,"name":"test-vm","runtime":"docker","status":"Running"}`, nil
		}

		// Capture the output
		output := captureStdout(func() {
			err := colimaVirt.PrintInfo()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Verify some contents of the output
		if !strings.Contains(output, "VM NAME") || !strings.Contains(output, "test-vm") || !strings.Contains(output, "192.168.5.2") {
			t.Errorf("Output does not contain expected contents. Got %q", output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// Capture the output
		err := colimaVirt.PrintInfo()
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}

		// Verify the error message
		expectedError := "error retrieving Colima info: mock error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}

// TestColimaVirt_WriteConfig tests the WriteConfig method of ColimaVirt.
func TestColimaVirt_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate a successful config save
		mocks.MockConfigHandler.SaveConfigFunc = func(path string) error {
			return nil
		}

		// Mock the userHomeDir function to return a valid directory
		userHomeDir = func() (string, error) {
			return "/mock/home/dir", nil
		}

		// Mock the mkdirAll function to simulate directory creation
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Mock the writeFile function to simulate file writing
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		// Mock the rename function to simulate file renaming
		rename = func(_, _ string) error {
			return nil
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ColimaNotDriver", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the vm.driver to be something other than "colima"
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "other-driver"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ArchSet", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the vm.arch to be an empty string
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.arch" {
				return "aarch64"
			}
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingHomeDir", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the userHomeDir function to return an error
		originalUserHomeDir := userHomeDir
		defer func() { userHomeDir = originalUserHomeDir }()
		userHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving home directory")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error retrieving user home directory: mock error retrieving home directory" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("ErrorCreatingColimaDir", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the mkdirAll function to return an error
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating colima directory")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error creating colima directory: mock error creating colima directory" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("ErrorEncodingYaml", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the newYAMLEncoder function to return an error
		originalNewYAMLEncoder := newYAMLEncoder
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
					return fmt.Errorf("mock error encoding yaml")
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
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error encoding yaml: mock error encoding yaml" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("ErrorClosingEncoder", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the newYAMLEncoder function to simulate an error when closing the encoder
		originalNewYAMLEncoder := newYAMLEncoder
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
					return nil
				},
				closeFunc: func() error {
					return fmt.Errorf("mock error closing encoder")
				},
			}
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error closing encoder: mock error closing encoder" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("ErrorWritingToTemporaryFile", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the writeFile function to simulate an error when writing to the temporary file
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing to temporary file")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error writing to temporary file: mock error writing to temporary file" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})

	t.Run("ErrorRenamingTemporaryFile", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the rename function to simulate an error during file renaming
		originalRename := rename
		defer func() { rename = originalRename }()
		rename = func(_, _ string) error {
			return fmt.Errorf("mock error renaming temporary file")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if err.Error() != "error renaming temporary file to colima config file: mock error renaming temporary file" {
			t.Fatalf("Unexpected error message: %v", err)
		}
	})
}

// TestColimaVirt_getArch tests the getArch method of ColimaVirt.
func TestColimaVirt_getArch(t *testing.T) {
	originalGoArch := goArch
	defer func() { goArch = originalGoArch }()

	tests := []struct {
		name     string
		mockArch string
		expected string
	}{
		{
			name:     "Test x86_64 architecture",
			mockArch: "amd64",
			expected: "x86_64",
		},
		{
			name:     "Test aarch64 architecture",
			mockArch: "arm64",
			expected: "aarch64",
		},
		{
			name:     "Test other architecture",
			mockArch: "386",
			expected: "386",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goArch = tt.mockArch

			result := getArch()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestColimaVirt_getDefaultValues tests the getDefaultValues method of ColimaVirt.
func TestColimaVirt_getDefaultValues(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		context := "success-context"
		expectedMemory := 4
		expectedCPU := 2
		expectedDisk := 60

		// Mock the necessary functions to simulate the environment
		mockVirtualMemory := func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: uint64(expectedMemory * 2 * 1024 * 1024 * 1024)}, nil
		}
		mockNumCPU := func() int {
			return expectedCPU * 2
		}

		originalVirtualMemory := virtualMemory
		originalNumCPU := numCPU
		defer func() {
			virtualMemory = originalVirtualMemory
			numCPU = originalNumCPU
		}()
		virtualMemory = mockVirtualMemory
		numCPU = mockNumCPU

		cpu, disk, memory, _, _ := getDefaultValues(context)

		if memory != expectedMemory {
			t.Errorf("Expected Memory %d, got %d", expectedMemory, memory)
		}

		if cpu != expectedCPU {
			t.Errorf("Expected CPU %d, got %d", expectedCPU, cpu)
		}

		if disk != expectedDisk {
			t.Errorf("Expected Disk %d, got %d", expectedDisk, disk)
		}
	})

	t.Run("MemoryOverflowHandling", func(t *testing.T) {
		context := "overflow-context"
		expectedMemory := math.MaxInt // Max int value
		expectedCPU := 2
		expectedDisk := 60

		// Mock the necessary functions to simulate the environment
		mockVirtualMemory := func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: uint64(expectedMemory+1) * 2 * 1024 * 1024 * 1024}, nil
		}
		mockNumCPU := func() int {
			return expectedCPU * 2
		}

		originalVirtualMemory := virtualMemory
		originalNumCPU := numCPU
		defer func() {
			virtualMemory = originalVirtualMemory
			numCPU = originalNumCPU
		}()
		virtualMemory = mockVirtualMemory
		numCPU = mockNumCPU

		// Force the overflow condition
		testForceMemoryOverflow = true
		defer func() { testForceMemoryOverflow = false }()

		cpu, disk, memory, _, _ := getDefaultValues(context)

		if memory != expectedMemory {
			t.Errorf("Expected Memory %d, got %d", expectedMemory, memory)
		}

		if cpu != expectedCPU {
			t.Errorf("Expected CPU %d, got %d", expectedCPU, cpu)
		}

		if disk != expectedDisk {
			t.Errorf("Expected Disk %d, got %d", expectedDisk, disk)
		}
	})

	t.Run("MemoryRetrievalError", func(t *testing.T) {
		context := "error-context"
		expectedMemory := 2
		expectedCPU := 2
		expectedDisk := 60

		// Mock the necessary functions to simulate the environment
		mockVirtualMemory := func() (*mem.VirtualMemoryStat, error) {
			return nil, fmt.Errorf("mock error")
		}
		mockNumCPU := func() int {
			return expectedCPU * 2
		}

		originalVirtualMemory := virtualMemory
		originalNumCPU := numCPU
		defer func() {
			virtualMemory = originalVirtualMemory
			numCPU = originalNumCPU
		}()
		virtualMemory = mockVirtualMemory
		numCPU = mockNumCPU

		cpu, disk, memory, _, _ := getDefaultValues(context)

		if memory != expectedMemory {
			t.Errorf("Expected Memory %d, got %d", expectedMemory, memory)
		}

		if cpu != expectedCPU {
			t.Errorf("Expected CPU %d, got %d", expectedCPU, cpu)
		}

		if disk != expectedDisk {
			t.Errorf("Expected Disk %d, got %d", expectedDisk, disk)
		}
	})
}

// TestColimaVirt_executeColimaCommand tests the executeColimaCommand method of ColimaVirt.
func TestColimaVirt_executeColimaCommand(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "delete" {
				return "Command executed successfully", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling executeColimaCommand
		err := colimaVirt.executeColimaCommand("delete")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling executeColimaCommand
		err := colimaVirt.executeColimaCommand("delete")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVirt_startColima tests the startColima method of ColimaVirt.
func TestColimaVirt_startColima(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "start" {
				return "", nil
			}
			if command == "colima" && len(args) > 0 && args[0] == "ls" {
				return `{
					"address": "192.168.5.2",
					"arch": "x86_64",
					"cpus": 4,
					"disk": 64424509440,
					"memory": 8589934592,
					"name": "windsor-test-context",
					"runtime": "docker",
					"status": "Running"
				}`, nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling startColima
		_, err := colimaVirt.startColima()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock execution error")
		}

		// When calling startColima
		_, err := colimaVirt.startColima()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("FailedToRetrieveVMInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "start" {
				return "", nil // Simulate successful execution
			}
			if command == "colima" && len(args) > 0 && args[0] == "ls" {
				return `{"address": ""}`, nil // Simulate no IP address
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling startColima
		_, err := colimaVirt.startColima()

		// Then an error should be returned due to failure to retrieve VM info with a valid address
		if err == nil || !strings.Contains(err.Error(), "Failed to retrieve VM info with a valid address") {
			t.Fatalf("Expected error containing 'Failed to retrieve VM info with a valid address', got %v", err)
		}
	})
}
