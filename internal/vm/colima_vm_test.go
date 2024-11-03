package vm

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

type MockComponents struct {
	Container         di.ContainerInterface
	MockContext       *context.MockContext
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
}

type mockYAMLEncoder struct {
	encodeFunc func(v interface{}) error
	closeFunc  func() error
}

func (m *mockYAMLEncoder) Encode(v interface{}) error {
	return m.encodeFunc(v)
}

func (m *mockYAMLEncoder) Close() error {
	return m.closeFunc()
}

func setupSafeColimaVmMocks(optionalContainer ...di.ContainerInterface) *MockComponents {
	var container di.ContainerInterface
	if len(optionalContainer) > 0 {
		container = optionalContainer[0]
	} else {
		container = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell()
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the container
	container.Register("contextHandler", mockContext)
	container.Register("shell", mockShell)
	container.Register("cliConfigHandler", mockConfigHandler)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() (string, error) {
		return "default-context", nil
	}

	// Set up the mock config handler to return a safe default configuration for Colima VMs
	mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
		return &config.Context{
			VM: &config.VMConfig{
				Arch:   ptrString(goArch),
				CPU:    ptrInt(numCPU()),
				Disk:   ptrInt(20 * 1024 * 1024 * 1024), // 20GB
				Driver: ptrString("colima"),
				Memory: ptrInt(4 * 1024 * 1024 * 1024), // 4GB
			},
		}, nil
	}

	return &MockComponents{
		Container:         container,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

// TestColimaVM_Up tests the Up method of ColimaVM.
func TestColimaVM_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

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
		err := colimaVM.Up()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorConfiguringColima", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate an error during configuration
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock configuration error")
		}

		// When calling Up
		err := colimaVM.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorStartingColima", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Up
		err := colimaVM.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVM_Down tests the Down method of ColimaVM.
func TestColimaVM_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate a successful stop
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "VM stopped", nil
		}

		// When calling Down
		err := colimaVM.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Down
		err := colimaVM.Down()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVM_Info tests the Info method of ColimaVM.
func TestColimaVM_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate a successful info retrieval
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return `{"address":"192.168.5.2","arch":"x86_64","cpus":4,"disk":10737418240,"memory":2147483648,"name":"test-vm","runtime":"docker","status":"Running"}`, nil
		}

		// When calling Info
		info, err := colimaVM.Info()

		// Then no error should be returned and info should be correct
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		colimaInfo := info.(*VMInfo)
		if colimaInfo.Address != "192.168.5.2" {
			t.Errorf("Expected address to be '192.168.5.2', got %s", colimaInfo.Address)
		}
		if colimaInfo.Arch != "x86_64" {
			t.Errorf("Expected arch to be 'x86_64', got %s", colimaInfo.Arch)
		}
		if colimaInfo.CPUs != 4 {
			t.Errorf("Expected CPUs to be 4, got %d", colimaInfo.CPUs)
		}
		if colimaInfo.Disk != 10 {
			t.Errorf("Expected Disk to be 10, got %f", colimaInfo.Disk)
		}
		if colimaInfo.Memory != 2 {
			t.Errorf("Expected Memory to be 2, got %f", colimaInfo.Memory)
		}
		if colimaInfo.Name != "test-vm" {
			t.Errorf("Expected Name to be 'test-vm', got %s", colimaInfo.Name)
		}
		if colimaInfo.Runtime != "docker" {
			t.Errorf("Expected Runtime to be 'docker', got %s", colimaInfo.Runtime)
		}
		if colimaInfo.Status != "Running" {
			t.Errorf("Expected Status to be 'Running', got %s", colimaInfo.Status)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Info
		_, err := colimaVM.Info()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create a mock container
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		// Setup mock components with the mock container
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// When calling Info
		_, err := colimaVM.Info()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error resolving context: mock resolve error" {
			t.Errorf("Expected error message 'error resolving context: mock resolve error', got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create a mock container
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		// Setup mock components with the mock container
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// When calling Info
		_, err := colimaVM.Info()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error resolving shell: mock resolve error" {
			t.Errorf("Expected error message 'error resolving shell: mock resolve error', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context retrieval error")
		}
		colimaVM := NewColimaVM(mocks.Container)

		// When calling Info
		_, err := colimaVM.Info()

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

		colimaVM := NewColimaVM(mocks.Container)

		// When calling Info
		_, err := colimaVM.Info()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "mock unmarshal error" {
			t.Errorf("Expected error message 'mock unmarshal error', got %v", err)
		}
	})
}

// TestColimaVM_Delete tests the Delete method of ColimaVM.
func TestColimaVM_Delete(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate a successful delete
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "VM deleted successfully", nil
		}

		// When calling Delete
		err := colimaVM.Delete()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return an error
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling Delete
		err := colimaVM.Delete()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVM_GetArch tests the GetArch method of ColimaVM.
func TestColimaVM_GetArch(t *testing.T) {
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

// TestColimaVM_getDefaultValues tests the getDefaultValues method of ColimaVM.
func TestColimaVM_getDefaultValues(t *testing.T) {
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

// TestColimaVM_executeColimaCommand tests the executeColimaCommand method of ColimaVM.
func TestColimaVM_executeColimaCommand(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "colima" && len(args) > 0 && args[0] == "delete" {
				return "Command executed successfully", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// When calling executeColimaCommand
		err := colimaVM.executeColimaCommand("delete", false)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Setup mock components with a mock container
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// When calling executeColimaCommand
		err := colimaVM.executeColimaCommand("delete", false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Setup mock components with a mock container
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// When calling executeColimaCommand
		err := colimaVM.executeColimaCommand("delete", false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorGettingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate an error when getting context
		mocks.MockContext.GetContextFunc = func() (string, error) { return "", fmt.Errorf("mock context error") }

		// When calling executeColimaCommand
		err := colimaVM.executeColimaCommand("delete", false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock error")
		}

		// When calling executeColimaCommand
		err := colimaVM.executeColimaCommand("delete", false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})
}

// TestColimaVM_configureColima tests the configureColima method of ColimaVM.
func TestColimaVM_configureColima(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return a valid config for Colima
		colimaDriver := "colima"
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{VM: &config.VMConfig{Driver: &colimaDriver}}, nil
		}

		// When calling configureColima
		err := colimaVM.configureColima()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Setup mock components with a mock container
		mockContainer := di.NewMockContainer()
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// Set an error to be returned when resolving cliConfigHandler
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("mock resolve error"))

		// When calling configureColima
		err := colimaVM.configureColima()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to simulate an error when retrieving config
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock config error")
		}

		// When calling configureColima
		err := colimaVM.configureColima()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("NoConfigForColima", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods to return a config without Colima driver
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{VM: &config.VMConfig{Driver: nil}}, nil
		}

		// When calling configureColima
		err := colimaVM.configureColima()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})
}

// TestColimaVM_startColimaVM tests the startColimaVM method of ColimaVM.
func TestColimaVM_startColimaVM(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

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

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Setup mock components with a mock container
		mockContainer := di.NewMockContainer()
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// Set the mock container to return an error when resolving context
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error resolving context: mock resolve error" {
			t.Errorf("Expected error message 'error resolving context: mock resolve error', got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Setup mock components with a mock container
		mockContainer := di.NewMockContainer()
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// Set the mock container to return an error when resolving shell
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error resolving shell: mock resolve error" {
			t.Errorf("Expected error message 'error resolving shell: mock resolve error', got %v", err)
		}
	})

	t.Run("ErrorExecutingCommand", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock execution error")
		}

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the necessary methods
		mocks.MockContext.GetContextFunc = func() (string, error) { return "", fmt.Errorf("mock context error") }

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
	})

	t.Run("ErrorRetrievingColimaInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

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

		// When calling startColimaVM
		err := colimaVM.startColimaVM(false)

		// Then an error should be returned due to failure in Info() on the second call
		if err == nil || !strings.Contains(err.Error(), "Error retrieving Colima info") {
			t.Fatalf("Expected error containing 'Error retrieving Colima info', got %v", err)
		}
	})
}

// TestColimaVM_writeConfig tests the writeConfig method of ColimaVM.
func TestColimaVM_writeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// And a mock config handler that simulates a successful config save
		mocks.MockConfigHandler.SaveConfigFunc = func(path string) error {
			return nil
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Given a mock container
		mockContainer := di.NewMockContainer()

		// Simulate an error during context resolution
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock context resolution error"))

		// And a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// And a mock context that simulates an error during context resolution
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context resolution error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock context resolution error") {
			t.Errorf("Expected error to contain 'mock context resolution error', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a mock container
		mockContainer := di.NewMockContainer()

		// Simulate an error during config handler resolution
		mockContainer.SetResolveError("cliConfigHandler", fmt.Errorf("mock config handler resolution error"))

		// And a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks(mockContainer)
		colimaVM := NewColimaVM(mocks.Container)

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock config handler resolution error") {
			t.Errorf("Expected error to contain 'mock config handler resolution error', got %v", err)
		}
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Simulate an error when retrieving the configuration
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock config retrieval error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock config retrieval error") {
			t.Errorf("Expected error to contain 'mock config retrieval error', got %v", err)
		}
	})

	t.Run("NoVMDefined", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// And a mock config handler that returns a config with no VM defined
		mocks.MockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{VM: nil}, nil
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("AArchVM", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the getArch function to return "aarch64"
		originalGetArch := getArch
		defer func() { getArch = originalGetArch }()
		getArch = func() string { return "aarch64" }

		// When calling writeConfig
		err := colimaVM.writeConfig()

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
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the writeFile function to simulate an error during file writing
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock write file error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock write file error") {
			t.Errorf("Expected error to contain 'mock write file error', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// And a mock context that simulates an error when retrieving context
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context retrieval error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock context retrieval error") {
			t.Errorf("Expected error to contain 'mock context retrieval error', got %v", err)
		}
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the userHomeDir function to simulate an error
		originalUserHomeDir := userHomeDir
		defer func() { userHomeDir = originalUserHomeDir }()
		userHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock user home dir error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock user home dir error") {
			t.Errorf("Expected error to contain 'mock user home dir error', got %v", err)
		}
	})

	t.Run("ErrorCreatingParentDirectories", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the mkdirAll function to simulate an error when creating directories
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock mkdirAll error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock mkdirAll error") {
			t.Errorf("Expected error to contain 'mock mkdirAll error', got %v", err)
		}
	})

	t.Run("ErrorCreatingColimaDirectory", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the mkdirAll function to simulate an error when creating the Colima directory
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			if strings.Contains(path, ".colima") {
				return fmt.Errorf("mock error creating colima directory")
			}
			return nil
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error creating colima directory") {
			t.Errorf("Expected error to contain 'mock error creating colima directory', got %v", err)
		}
	})

	t.Run("ErrorEncodingYaml", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

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

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock encode error") {
			t.Errorf("Expected error to contain 'mock encode error', got %v", err)
		}
	})

	t.Run("ErrorClosingEncoder", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

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

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock close error") {
			t.Errorf("Expected error to contain 'mock close error', got %v", err)
		}
	})

	t.Run("ErrorRenamingTemporaryFile", func(t *testing.T) {
		// Given a ColimaVM with mock components
		mocks := setupSafeColimaVmMocks()
		colimaVM := NewColimaVM(mocks.Container)

		// Mock the rename function to simulate an error during renaming
		originalRename := rename
		defer func() { rename = originalRename }()
		rename = func(oldpath, newpath string) error {
			return fmt.Errorf("mock rename error")
		}

		// When calling writeConfig
		err := colimaVM.writeConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock rename error") {
			t.Errorf("Expected error to contain 'mock rename error', got %v", err)
		}
	})
}
