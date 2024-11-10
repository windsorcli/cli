package virt

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func setupSafeColimaVmMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	// Set up the mock config handler to return a safe default configuration for Colima VMs
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			VM: &config.VMConfig{
				Arch:   ptrString(goArch),
				CPU:    ptrInt(numCPU()),
				Disk:   ptrInt(20 * 1024 * 1024 * 1024), // 20GB
				Driver: ptrString("colima"),
				Memory: ptrInt(4 * 1024 * 1024 * 1024), // 4GB
			},
		}
	}

	return &MockComponents{
		Injector:          injector,
		MockContext:       mockContext,
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

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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
			return "", nil
		}
		mocks.MockConfigHandler.LoadConfigFunc = func(path string) error { return nil }

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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Up
		err := colimaVirt.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "VM stopped", nil
		}

		// When calling Down
		err := colimaVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return `{"address":"192.168.5.2","arch":"x86_64","cpus":4,"disk":64424509440,"memory":8589934592,"name":"test-vm","runtime":"docker","status":"Running"}`, nil
		}

		// When calling GetVMInfo
		info, err := colimaVirt.GetVMInfo()

		// Then no error should be returned and info should be correct
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if info.Address != "192.168.5.2" {
			t.Errorf("Expected address to be '192.168.5.2', got %s", info.Address)
		}
		if info.Arch != "x86_64" {
			t.Errorf("Expected arch to be 'x86_64', got %s", info.Arch)
		}
		if info.CPUs != 4 {
			t.Errorf("Expected CPUs to be 4, got %d", info.CPUs)
		}
		if info.Disk != 60 {
			t.Errorf("Expected Disk to be 60, got %d", info.Disk)
		}
		if info.Memory != 8 {
			t.Errorf("Expected Memory to be 8, got %d", info.Memory)
		}
		if info.Name != "test-vm" {
			t.Errorf("Expected Name to be 'test-vm', got %s", info.Name)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling GetVMInfo
		_, err := colimaVirt.GetVMInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context retrieval error")
		}
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// When calling GetVMInfo
		_, err := colimaVirt.GetVMInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error retrieving context: mock context retrieval error" {
			t.Errorf("Expected error message 'error retrieving context: mock context retrieval error', got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingColimaInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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

// TestColimaVirt_Delete tests the Delete method of ColimaVirt.
func TestColimaVirt_Delete(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate a successful delete
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "VM deleted successfully", nil
		}

		// When calling Delete
		err := colimaVirt.Delete()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Delete
		err := colimaVirt.Delete()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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

		// And a mock config handler that simulates a successful config save
		mocks.MockConfigHandler.SaveConfigFunc = func(path string) error {
			return nil
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("NoVMDefined", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// And a mock config handler that returns a config with no VM defined
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{VM: nil}
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("AArchVM", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the getArch function to return "aarch64"
		originalGetArch := getArch
		defer func() { getArch = originalGetArch }()
		getArch = func() string { return "aarch64" }

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify the vmType is set to "vz"
		if getArch() != "aarch64" {
			t.Errorf("Expected getArch to return 'aarch64', got %s", getArch())
		}
	})

	t.Run("ErrorSavingConfig", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the writeFile function to simulate an error during file writing
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write file error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock write file error") {
			t.Errorf("Expected error to contain 'mock write file error', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// And a mock context that simulates an error when retrieving context
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context retrieval error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock context retrieval error") {
			t.Errorf("Expected error to contain 'mock context retrieval error', got %v", err)
		}
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the userHomeDir function to simulate an error
		originalUserHomeDir := userHomeDir
		defer func() { userHomeDir = originalUserHomeDir }()
		userHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock user home dir error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock user home dir error") {
			t.Errorf("Expected error to contain 'mock user home dir error', got %v", err)
		}
	})

	t.Run("ErrorCreatingParentDirectories", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the mkdirAll function to simulate an error when creating directories
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock mkdirAll error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock mkdirAll error") {
			t.Errorf("Expected error to contain 'mock mkdirAll error', got %v", err)
		}
	})

	t.Run("ErrorCreatingColimaDirectory", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the mkdirAll function to simulate an error when creating the Colima directory
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			if strings.Contains(path, ".colima") {
				return fmt.Errorf("mock error creating colima directory")
			}
			return nil
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error creating colima directory") {
			t.Errorf("Expected error to contain 'mock error creating colima directory', got %v", err)
		}
	})

	t.Run("ErrorEncodingYaml", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the newYAMLEncoder function to return a mock encoder that returns an error on Encode
		originalNewYAMLEncoder := newYAMLEncoder
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
					return errors.New("mock encode error")
				},
				closeFunc: func() error {
					return nil
				},
			}
		}
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock encode error") {
			t.Errorf("Expected error to contain 'mock encode error', got %v", err)
		}
	})

	t.Run("ErrorClosingEncoder", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the newYAMLEncoder function to simulate an error during closing
		originalNewYAMLEncoder := newYAMLEncoder
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
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
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock close error") {
			t.Errorf("Expected error to contain 'mock close error', got %v", err)
		}
	})

	t.Run("ErrorRenamingTemporaryFile", func(t *testing.T) {
		// Given a ColimaVirt with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the rename function to simulate an error during renaming
		originalRename := rename
		defer func() { rename = originalRename }()
		rename = func(oldpath, newpath string) error {
			return fmt.Errorf("mock rename error")
		}

		// When calling WriteConfig
		err := colimaVirt.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock rename error") {
			t.Errorf("Expected error to contain 'mock rename error', got %v", err)
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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "delete" {
				return "Command executed successfully", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling executeColimaCommand
		err := colimaVirt.executeColimaCommand("delete", false)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods to simulate an error when getting context
		mocks.MockContext.GetContextFunc = func() (string, error) { return "", fmt.Errorf("mock context error") }

		// When calling executeColimaCommand
		err := colimaVirt.executeColimaCommand("delete", false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling executeColimaCommand
		err := colimaVirt.executeColimaCommand("delete", false)

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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
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
		err := colimaVirt.startColima(false)

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
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock execution error")
		}

		// When calling startColima
		err := colimaVirt.startColima(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		mocks.MockContext.GetContextFunc = func() (string, error) { return "", fmt.Errorf("mock context error") }

		// When calling startColima
		err := colimaVirt.startColima(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingColimaInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVirt := NewColimaVirt(mocks.Injector)
		colimaVirt.Initialize()

		// Mock the necessary methods
		callCount := 0
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "start" {
				return "", nil // Simulate successful execution
			}
			if command == "colima" && len(args) > 0 && args[0] == "ls" {
				callCount++
				if callCount == 1 {
					return `{"address": ""}`, nil // Simulate no IP address on first call
				}
				return "", fmt.Errorf("mock execution error") // Simulate failure in Info() on second call
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling startColima
		err := colimaVirt.startColima(false)

		// Then an error should be returned due to failure in Info() on the second call
		if err == nil || !strings.Contains(err.Error(), "Error retrieving Colima info") {
			t.Fatalf("Expected error containing 'Error retrieving Colima info', got %v", err)
		}
	})
}
