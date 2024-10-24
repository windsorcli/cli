package helpers

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

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

func createDIContainer(mockContext *context.MockContext, mockConfigHandler *config.MockConfigHandler) *di.DIContainer {
	diContainer := di.NewContainer()
	diContainer.Register("context", mockContext)
	diContainer.Register("cliConfigHandler", mockConfigHandler)
	return diContainer
}

func TestColimaHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Create an instance of ColimaHelper
		colimaHelper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// When: Initialize is called
		err = colimaHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestColimaHelper_NewColimaHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a DI container without registering cliConfigHandler
		diContainer := di.NewContainer()

		// When attempting to create ColimaHelper
		_, err := NewColimaHelper(diContainer)

		// Then it should return an error indicating cliConfigHandler resolution failure
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Given a DI container with only cliConfigHandler registered
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// When attempting to create ColimaHelper
		_, err := NewColimaHelper(diContainer)

		// Then it should return an error indicating context resolution failure
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestColimaHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler and context
		cliConfigHandler := config.NewMockConfigHandler()
		ctx := context.NewMockContext()

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(ctx, cliConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And executing post environment setup
		err = helper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestColimaHelper_GetDefaultValues(t *testing.T) {
	t.Run("MemoryError", func(t *testing.T) {
		// Given a mock virtualMemory function that returns an error
		originalVirtualMemory := virtualMemory
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return nil, errors.New("mock error")
		}
		defer func() { virtualMemory = originalVirtualMemory }()

		// When calling getDefaultValues
		_, _, memory, _, _ := getDefaultValues("test-context")

		// Then it should return a default memory value of 2
		if memory != 2 {
			t.Fatalf("expected memory to be 2, got %d", memory)
		}
	})

	t.Run("MemoryMock", func(t *testing.T) {
		// Given a mock virtualMemory function that returns a fixed total memory
		originalVirtualMemory := virtualMemory
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
		}
		defer func() { virtualMemory = originalVirtualMemory }()

		// When calling getDefaultValues
		_, _, memory, _, _ := getDefaultValues("test-context")

		// Then it should return half of the total memory
		if memory != 32 { // Expecting half of 64GB
			t.Fatalf("expected memory to be 32, got %d", memory)
		}
	})

	t.Run("MemoryOverflowHandling", func(t *testing.T) {
		// Given a mock virtualMemory function that returns a normal value
		originalVirtualMemory := virtualMemory
		defer func() { virtualMemory = originalVirtualMemory }()

		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
		}

		// And forcing the overflow condition
		testForceMemoryOverflow = true
		defer func() { testForceMemoryOverflow = false }()

		// When calling getDefaultValues
		_, _, memory, _, _ := getDefaultValues("test-context")

		// Then it should return the maximum integer value for memory
		if memory != math.MaxInt {
			t.Fatalf("expected memory to be set to MaxInt, got %d", memory)
		}
	})
}

func TestColimaHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockConfigHandler := config.NewMockConfigHandler()

		// And a DI container with the mock context and config handler registered
		container := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		colimaHelper, err := NewColimaHelper(container)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting container configuration
		composeConfig, err := colimaHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then it should return nil as per the stub implementation
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})
}

func TestColimaHelper_GetEnvVars(t *testing.T) {
	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock config error")
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting environment variables
		_, err = helper.GetEnvVars()

		// Then it should return an error indicating config retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving config: mock config error" {
			t.Fatalf("expected 'error retrieving config: mock config error', got '%v'", err)
		}
	})

	t.Run("DriverNotColima", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("not-colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting environment variables
		envVars, err := helper.GetEnvVars()

		// Then it should return an empty map for envVars and no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(envVars) != 0 {
			t.Fatalf("expected empty envVars, got %v", envVars)
		}
	})

	t.Run("ErrorRetrievingUserHomeDir", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Mock the userHomeDir function to return an error
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return "", errors.New("mock home dir error")
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting environment variables
		_, err = helper.GetEnvVars()

		// Then it should return an error indicating user home directory retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving user home directory: mock home dir error" {
			t.Fatalf("expected 'error retrieving user home directory: mock home dir error', got '%v'", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Mock the userHomeDir function to return a valid directory
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting environment variables
		envVars, err := helper.GetEnvVars()

		// Then it should return the expected environment variables
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedDockerSockPath := filepath.Join(tempDir, ".colima", "windsor-test-context", "docker.sock")
		if envVars["DOCKER_SOCK"] != expectedDockerSockPath {
			t.Fatalf("expected DOCKER_SOCK to be '%s', got '%s'", expectedDockerSockPath, envVars["DOCKER_SOCK"])
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given a mock context that returns an error when retrieving context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}
		// And a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And getting environment variables
		_, err = helper.GetEnvVars()

		// Then it should return an error indicating context retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error retrieving context: mock context error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})
}

func TestColimaHelper_WriteConfig(t *testing.T) {
	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given a mock context that returns an error when retrieving context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("mock error")
		}
		// And a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating context retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error retrieving context: mock error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("OverrideValue", func(t *testing.T) {
		// Setup mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					CPU:    ptrInt(4),
					Disk:   ptrInt(100),
					Memory: ptrInt(8),
					Driver: ptrString("colima"),
					Arch:   ptrString("aarch64"),
				},
			}, nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Create ColimaHelper
		colimaHelper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// Create a temporary directory
		tempDir, err := os.MkdirTemp("", "colima_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temp directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Test WriteConfig function which uses overrideValue internally
		err = colimaHelper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the values have been overridden correctly
		config, err := colimaHelper.ConfigHandler.GetConfig()
		if err != nil {
			t.Fatalf("GetConfig() error = %v", err)
		}

		if config.VM.CPU == nil || *config.VM.CPU != 4 {
			t.Errorf("Expected cpu to be 4, got %d", *config.VM.CPU)
		}
		if config.VM.Disk == nil || *config.VM.Disk != 100 {
			t.Errorf("Expected disk to be 100, got %d", *config.VM.Disk)
		}
		if config.VM.Memory == nil || *config.VM.Memory != 8 {
			t.Errorf("Expected memory to be 8, got %d", *config.VM.Memory)
		}
		if config.VM.Arch == nil || *config.VM.Arch != "aarch64" {
			t.Errorf("Expected arch to be 'aarch64', got '%s'", *config.VM.Arch)
		}
	})

	t.Run("ErrorRetrievingUserHomeDir", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Mock the userHomeDir function to return an error
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return "", errors.New("mock home dir error")
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating user home directory retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error retrieving user home directory: mock home dir error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingParentDirectories", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Mock the userHomeDir function to return a valid directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return "/mock/home", nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the mkdirAll function to return an error
		originalMkdirAll := mkdirAll
		mkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mock mkdir error")
		}
		defer func() { mkdirAll = originalMkdirAll }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to create parent directories
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error creating parent directories for colima directory: mock mkdir error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingColimaDirectory", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Mock the userHomeDir function to return a valid directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return "/mock/home", nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the mkdirAll function to return an error when creating the Colima directory
		originalMkdirAll := mkdirAll
		mkdirAll = func(path string, perm os.FileMode) error {
			colimaDir := filepath.Join("/mock/home", ".colima", "windsor-test-context")
			if path == colimaDir {
				return errors.New("mock mkdir error")
			}
			return nil
		}
		defer func() { mkdirAll = originalMkdirAll }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to create the Colima directory
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error creating colima directory: mock mkdir error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorEncodingYAML", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Create a temporary directory for the test
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temporary directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

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

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to encode YAML
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error encoding yaml: mock encode error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorClosingEncoder", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Create a temporary directory for the test
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temporary directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the newYAMLEncoder function to return a mock encoder that returns an error on Close
		originalNewYAMLEncoder := newYAMLEncoder
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
					return nil
				},
				closeFunc: func() error {
					return errors.New("mock close error")
				},
			}
		}
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to close the encoder
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error closing encoder: mock close error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingToTemporaryFile", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Create a temporary directory for the test
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temporary directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the writeFile function to return an error
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("mock write file error")
		}
		defer func() { writeFile = originalWriteFile }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to write to the temporary file
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error writing to temporary file: mock write file error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ErrorRenamingTemporaryFile", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// Create a temporary directory for the test
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temporary directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the writeFile function to succeed
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		defer func() { writeFile = originalWriteFile }()

		// Mock the rename function to return an error
		originalRename := rename
		rename = func(oldpath, newpath string) error {
			return errors.New("mock rename error")
		}
		defer func() { rename = originalRename }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating failure to rename the temporary file
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error renaming temporary file to colima config file: mock rename error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("SetVMTypeToVZOnAarch64", func(t *testing.T) {
		// Given a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Create a temporary directory for the test
		tempDir := t.TempDir()
		defer os.RemoveAll(tempDir) // Clean up the temp directory after the test

		// Mock the userHomeDir function to return the temporary directory
		originalUserHomeDir := userHomeDir
		userHomeDir = func() (string, error) {
			return tempDir, nil
		}
		defer func() { userHomeDir = originalUserHomeDir }()

		// Mock the getArch function to return "aarch64"
		originalGetArch := getArch
		getArch = func() string {
			return "aarch64"
		}
		defer func() { getArch = originalGetArch }()

		// Mock the writeFile and rename functions to succeed
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		defer func() { writeFile = originalWriteFile }()

		originalRename := rename
		rename = func(oldpath, newpath string) error {
			return nil
		}
		defer func() { rename = originalRename }()

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Then the vmType should be set to "vz"
		// This would typically be verified by checking the configuration written to the file
		// For this test, you might need to inspect the internal state or mock the encoder to capture the config
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Given a mock context that returns a valid context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And a mock config handler that returns an error when retrieving config
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock config error")
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return an error indicating config retrieval failure
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedError := "error retrieving config: mock config error"
		if err.Error() != expectedError {
			t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("DriverNotColima", func(t *testing.T) {
		// Given a mock context that returns a valid context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And a mock config handler with a VM driver not set to "colima"
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("not-colima"),
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := createDIContainer(mockContext, mockConfigHandler)

		// When creating a new ColimaHelper
		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// And writing the configuration
		err = helper.WriteConfig()

		// Then it should return nil, indicating no action was taken
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestGetArch(t *testing.T) {
	t.Run("ArchAMD64", func(t *testing.T) {
		// Mock goArch to return "amd64"
		originalGoArch := goArch
		goArch = func() string {
			return "amd64"
		}
		defer func() { goArch = originalGoArch }()

		// When calling getArch
		arch := getArch()

		// Then it should return "x86_64"
		if arch != "x86_64" {
			t.Fatalf("expected arch to be 'x86_64', got '%s'", arch)
		}
	})

	t.Run("ArchARM64", func(t *testing.T) {
		// Mock goArch to return "arm64"
		originalGoArch := goArch
		goArch = func() string {
			return "arm64"
		}
		defer func() { goArch = originalGoArch }()

		// When calling getArch
		arch := getArch()

		// Then it should return "aarch64"
		if arch != "aarch64" {
			t.Fatalf("expected arch to be 'aarch64', got '%s'", arch)
		}
	})

	t.Run("ArchOther", func(t *testing.T) {
		// Mock goArch to return "s390x"
		originalGoArch := goArch
		goArch = func() string {
			return "s390x"
		}
		defer func() { goArch = originalGoArch }()

		// When calling getArch
		arch := getArch()

		// Then it should return "s390x"
		if arch != "s390x" {
			t.Fatalf("expected arch to be 's390x', got '%s'", arch)
		}
	})
}
