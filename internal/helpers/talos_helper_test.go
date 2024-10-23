package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestTalosHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := &shell.MockShell{}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When: Initialize is called
		err = talosHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestTalosHelper_GetEnvVars(t *testing.T) {
	// Common setup for tests
	var (
		mockContext       *context.MockContext
		mockConfigHandler *config.MockConfigHandler
		mockShell         *shell.MockShell
		diContainer       *di.DIContainer
		tempDir           string
	)

	setup := func() {
		tempDir = t.TempDir()
		mockContext = context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.Join(tempDir, "contexts", "test-context"), nil
		}
		mockConfigHandler = config.NewMockConfigHandler()
		mockShell = &shell.MockShell{}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		diContainer = di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
	}

	t.Run("Success", func(t *testing.T) {
		setup()

		// Given: a valid context path
		contextPath := filepath.Join(tempDir, "contexts", "test-context")
		talosConfigPath := filepath.Join(contextPath, ".talos", "config")

		// And the talos config file is created
		err := os.MkdirAll(filepath.Dir(talosConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create talos config directory: %v", err)
		}
		file, err := os.Create(talosConfigPath)
		if err != nil {
			t.Fatalf("Failed to create talos config file: %v", err)
		}
		file.Close() // Close the file to avoid file access issues on Windows

		// And a mock context is set up
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And the cluster driver is set to "talos"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// And calling GetEnvVars
		envVars, err := talosHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": talosConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("TalosConfigFileDoesNotExist", func(t *testing.T) {
		setup()

		// Given: a valid context path where talos config file does not exist
		contextPath := filepath.Join(tempDir, "contexts", "test-context")

		// And a mock context is set up
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And the cluster driver is set to "talos"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling GetEnvVars
		envVars, err := talosHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then the TALOSCONFIG path should be empty
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": "",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given: a mock context that succeeds for GetConfigRoot on the first call and fails on the second call
		mockContext := context.NewMockContext()
		callCount := 0
		mockContext.GetConfigRootFunc = func() (string, error) {
			if callCount == 0 {
				callCount++
				return "", fmt.Errorf("mock error retrieving config root")
			}
			return filepath.Join(tempDir, "path", "to", "config"), nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Mock os.Mkdir to avoid actual directory creation
		originalMkdir := mkdir
		defer func() { mkdir = originalMkdir }()
		mkdir = func(path string, perm os.FileMode) error {
			return nil
		}

		// Create a DI container and register the mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := &shell.MockShell{}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join(tempDir, "path", "to", "project"), nil
		}
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling GetEnvVars
		_, err = talosHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving config root") {
			t.Fatalf("expected error containing 'mock error retrieving config root', got %v", err)
		}
	})
}

func TestTalosHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Common setup for tests
		var (
			mockContext       *context.MockContext
			mockConfigHandler *config.MockConfigHandler
			mockShell         *shell.MockShell
			diContainer       *di.DIContainer
			tempDir           string
		)

		setup := func() {
			tempDir = t.TempDir()
			mockContext = context.NewMockContext()
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(tempDir, "contexts", "test-context"), nil
			}
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockConfigHandler = config.NewMockConfigHandler()
			mockShell = &shell.MockShell{}
			mockShell.GetProjectRootFunc = func() (string, error) {
				return tempDir, nil
			}
			diContainer = di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", mockShell)
		}

		setup()

		// Given a TalosHelper instance
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// When calling PostEnvExec
		err = talosHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestNewTalosHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a DI container without registering cliConfigHandler
		diContainer := di.NewContainer()

		// When attempting to create TalosHelper
		_, err := NewTalosHelper(diContainer)

		// Then it should return an error indicating config handler resolution failure
		if err == nil || !strings.Contains(err.Error(), "error resolving config handler") {
			t.Fatalf("expected error resolving config handler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Given: a DI container with only cliConfigHandler registered
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		// Note: "context" is not registered in the DI container

		// When attempting to create TalosHelper
		_, err := NewTalosHelper(diContainer)

		// Then it should return an error indicating context resolution failure
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		// Given: a mock context that returns an error
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving current context")
		}

		// Create a DI container and register the mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
		diContainer.Register("shell", &shell.MockShell{}) // Registering mock shell to avoid error

		// When: creating a new TalosHelper
		_, err := NewTalosHelper(diContainer)

		// Then: an error should be returned indicating the failure to retrieve the current context
		expectedError := "error retrieving current context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("CreateVolumesDirectory", func(t *testing.T) {
		// Given: a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "expected/volumes/path", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// Create a DI container and register the mock context and config handler
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Mock the os.Stat function to simulate the directory not existing
		originalStat := stat
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		defer func() { stat = originalStat }()

		// Mock the os.Mkdir function to simulate directory creation failure
		originalMkdir := mkdir
		mkdir = func(name string, perm os.FileMode) error {
			if name == filepath.Join("expected", "volumes", "path", ".volumes") {
				return fmt.Errorf("mkdir %s: no such file or directory", name)
			}
			return nil
		}
		defer func() { mkdir = originalMkdir }()

		// Mock the shell's GetProjectRoot function
		mockShell := &shell.MockShell{}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "expected/volumes/path", nil
		}
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		_, err := NewTalosHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error creating .volumes folder") {
			t.Fatalf("expected error creating .volumes folder, got %v", err)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given: a mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// Create a DI container and register the mock context and config handler
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Mock the shell's GetProjectRoot function to return an error
		mockShell := &shell.MockShell{}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		_, err := NewTalosHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Common setup for tests
		var (
			mockContext       *context.MockContext
			mockConfigHandler *config.MockConfigHandler
			diContainer       *di.DIContainer
		)

		setup := func() {
			mockContext = context.NewMockContext()
			mockConfigHandler = config.NewMockConfigHandler()
			diContainer = di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)
		}

		setup()

		// Given: a DI container without the "shell" dependency registered
		// Note: "shell" is not registered here to simulate the error

		// When creating TalosHelper
		_, err := NewTalosHelper(diContainer)

		// Then: an error should be returned indicating the failure to resolve the shell
		expectedError := "error resolving shell"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %q, got %v", expectedError, err)
		}
	})
}

func TestTalosHelper_GetComposeConfig(t *testing.T) {
	// Common setup for tests
	var (
		mockContext       *context.MockContext
		mockConfigHandler *config.MockConfigHandler
		mockShell         *shell.MockShell
		diContainer       *di.DIContainer
		tempDir           string
	)

	setup := func() {
		tempDir = t.TempDir()
		mockContext = context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.Join(tempDir, "contexts", "test-context"), nil
		}
		mockConfigHandler = config.NewMockConfigHandler()
		mockShell = &shell.MockShell{}
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}
		diContainer = di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
	}

	t.Run("Success", func(t *testing.T) {
		setup()

		// Given a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And the cluster driver is set to "talos"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And the number of control planes and workers are set
		customControlPlanes := 1
		customWorkers := 1
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			switch key {
			case "contexts.test-context.cluster.controlplanes.count":
				return customControlPlanes, nil
			case "contexts.test-context.cluster.workers.count":
				return customWorkers, nil
			case "contexts.test-context.cluster.controlplanes.cpu":
				return 2, nil
			case "contexts.test-context.cluster.workers.cpu":
				return 4, nil
			case "contexts.test-context.cluster.controlplanes.memory":
				return 2048, nil
			case "contexts.test-context.cluster.workers.memory":
				return 4096, nil
			default:
				return 0, fmt.Errorf("unexpected key: %s", key)
			}
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When: GetContainerConfig is called
		containerConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should not be nil
		if containerConfig == nil {
			t.Fatalf("expected non-nil containerConfig, got nil")
		}

		// And the number of services should be controlPlanes + workers
		expectedServiceCount := customControlPlanes + customWorkers
		if len(containerConfig.Services) != expectedServiceCount {
			t.Errorf("expected %d services, got %d", expectedServiceCount, len(containerConfig.Services))
		}

		// Validate the services
		for i := 0; i < customControlPlanes; i++ {
			service := containerConfig.Services[i]
			expectedName := fmt.Sprintf("controlplane-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
		}

		for i := 0; i < customWorkers; i++ {
			service := containerConfig.Services[customControlPlanes+i]
			expectedName := fmt.Sprintf("worker-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
		}
	})

	t.Run("SuccessWithDefaultSettings", func(t *testing.T) {
		setup()

		// Set the necessary environment variable
		os.Setenv("WINDSOR_PROJECT_ROOT", tempDir)
		defer os.Unsetenv("WINDSOR_PROJECT_ROOT")

		// Given: a mock context and config handler with default settings
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return "", nil
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling GetComposeConfig
		composeConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Define expected values
		expectedControlPlaneCPUCores := 2
		expectedControlPlaneRAMGB := 2
		expectedControlPlaneRAMMB := expectedControlPlaneRAMGB * 1024

		expectedWorkerCPUCores := 4
		expectedWorkerRAMGB := 4
		expectedWorkerRAMMB := expectedWorkerRAMGB * 1024

		// Validate the services
		if len(composeConfig.Services) != 2 {
			t.Fatalf("expected 2 services, got %d", len(composeConfig.Services))
		}

		controlPlaneService := composeConfig.Services[0]
		if controlPlaneService.Name != "controlplane-1.test" {
			t.Errorf("expected controlplane-1.test, got %s", controlPlaneService.Name)
		}
		if *controlPlaneService.Environment["PLATFORM"] != "container" {
			t.Errorf("expected PLATFORM=container, got %s", *controlPlaneService.Environment["PLATFORM"])
		}
		if *controlPlaneService.Environment["TALOSSKU"] != fmt.Sprintf("%dCPU-%dRAM", expectedControlPlaneCPUCores, expectedControlPlaneRAMMB) {
			t.Errorf("expected TALOSSKU=%dCPU-%dRAM, got %s", expectedControlPlaneCPUCores, expectedControlPlaneRAMMB, *controlPlaneService.Environment["TALOSSKU"])
		}

		workerService := composeConfig.Services[1]
		if workerService.Name != "worker-1.test" {
			t.Errorf("expected worker-1.test, got %s", workerService.Name)
		}
		if *workerService.Environment["PLATFORM"] != "container" {
			t.Errorf("expected PLATFORM=container, got %s", *workerService.Environment["PLATFORM"])
		}
		if *workerService.Environment["TALOSSKU"] != fmt.Sprintf("%dCPU-%dRAM", expectedWorkerCPUCores, expectedWorkerRAMMB) {
			t.Errorf("expected TALOSSKU=%dCPU-%dRAM, got %s", expectedWorkerCPUCores, expectedWorkerRAMMB, *workerService.Environment["TALOSSKU"])
		}
		if len(workerService.Volumes) != 9 {
			t.Errorf("expected 9 volumes, got %d", len(workerService.Volumes))
		}
		if workerService.Volumes[8].Source != "${WINDSOR_PROJECT_ROOT}/.volumes" {
			t.Errorf("expected volume source ${WINDSOR_PROJECT_ROOT}/.volumes, got %s", workerService.Volumes[8].Source)
		}
		if workerService.Volumes[8].Target != "/var/local" {
			t.Errorf("expected volume target /var/local, got %s", workerService.Volumes[8].Target)
		}
	})

	t.Run("NonTalosClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context and config handler where cluster driver is not Talos
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "kubernetes", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling GetComposeConfig
		composeConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then services should be nil
		if composeConfig != nil {
			t.Errorf("expected composeConfig to be nil when cluster driver is not Talos, got: %v", composeConfig)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		setup()

		// Given: a mock context that succeeds first and then returns an error
		callCount := 0
		mockContext.GetContextFunc = func() (string, error) {
			if callCount == 0 {
				callCount++
				return "test-context", nil
			}
			return "", fmt.Errorf("mock context error")
		}

		// And the cluster driver is set to "talos"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving current context") {
			t.Errorf("expected error retrieving current context, got: %v", err)
		}
	})

	t.Run("EmptyClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context and config handler that returns an empty string for the cluster driver
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "", nil
			}
			return "", nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// When calling GetComposeConfig
		config, err := talosHelper.GetComposeConfig()
		// Then the config should be nil
		if config != nil {
			t.Errorf("expected nil config, got: %v", config)
		}
	})

	t.Run("ErrorRetrievingNumberOfControlPlanes", func(t *testing.T) {
		setup()

		// Given: a mock context and config handler that returns error for controlplanes.count
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", fmt.Errorf("mock error retrieving string value for key: %s", key)
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.controlplanes.count" {
				return 0, fmt.Errorf("mock error retrieving number of control planes")
			}
			return 0, fmt.Errorf("mock error retrieving int value for key: %s", key)
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving number of control planes") {
			t.Errorf("expected error retrieving number of control planes, got: %v", err)
		}
	})

	t.Run("ZeroControlPlanesAndWorkers", func(t *testing.T) {
		setup()

		// Given: control plane and worker counts are zero
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.controlplanes.count" {
				return 0, nil
			}
			if key == "contexts.test-context.cluster.workers.count" {
				return 0, nil
			}
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, fmt.Errorf("unexpected key: %s", key)
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		composeConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		// Then: services should be an empty slice
		if len(composeConfig.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(composeConfig.Services))
		}
	})

	t.Run("CustomSettings", func(t *testing.T) {
		setup()

		// Given: custom settings for counts and resources
		mockContext.GetContextFunc = func() (string, error) {
			return "custom-context", nil
		}

		customControlPlanes := 2
		customWorkers := 3
		customCPUCores := 8
		customRAM := 16 // GB

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.custom-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			switch key {
			case "contexts.custom-context.cluster.controlplanes.count":
				return customControlPlanes, nil
			case "contexts.custom-context.cluster.workers.count":
				return customWorkers, nil
			case "contexts.custom-context.cluster.controlplanes.cpu":
				return customCPUCores, nil
			case "contexts.custom-context.cluster.controlplanes.memory":
				return customRAM, nil
			case "contexts.custom-context.cluster.workers.cpu":
				return customCPUCores, nil
			case "contexts.custom-context.cluster.workers.memory":
				return customRAM, nil
			default:
				return 0, fmt.Errorf("unexpected key: %s", key)
			}
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling GetComposeConfig
		composeConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then the number of services should be controlPlanes + workers
		expectedServiceCount := customControlPlanes + customWorkers
		if len(composeConfig.Services) != expectedServiceCount {
			t.Errorf("expected %d services, got %d", expectedServiceCount, len(composeConfig.Services))
		}

		// Validate the services
		for i := 0; i < customControlPlanes; i++ {
			service := composeConfig.Services[i]
			expectedName := fmt.Sprintf("controlplane-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
			expectedSKU := fmt.Sprintf("%dCPU-%dRAM", customCPUCores, customRAM*1024)
			if sku := *service.Environment["TALOSSKU"]; sku != expectedSKU {
				t.Errorf("expected TALOSSKU %s, got %s", expectedSKU, sku)
			}
		}

		for i := 0; i < customWorkers; i++ {
			service := composeConfig.Services[customControlPlanes+i]
			expectedName := fmt.Sprintf("worker-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
			expectedSKU := fmt.Sprintf("%dCPU-%dRAM", customCPUCores, customRAM*1024)
			if sku := *service.Environment["TALOSSKU"]; sku != expectedSKU {
				t.Errorf("expected TALOSSKU %s, got %s", expectedSKU, sku)
			}
		}
	})

	t.Run("ErrorRetrievingNumberOfWorkers", func(t *testing.T) {
		setup()

		// Given: a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And the cluster driver is 'talos'
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And mockConfigHandler.GetInt returns an error when retrieving number of workers
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.workers.count" {
				return 0, fmt.Errorf("mock error retrieving number of workers")
			}
			// Provide default values for other keys to proceed
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		expectedError := "error retrieving number of workers"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingControlPlaneCPU", func(t *testing.T) {
		setup()

		// Given: a mock context and cluster driver set to "talos"
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And mockConfigHandler.GetInt returns an error when retrieving control plane CPU
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.controlplanes.cpu" {
				return 0, fmt.Errorf("mock error retrieving control plane CPU setting")
			}
			// Provide default values for other keys to proceed
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		expectedError := "error retrieving control plane CPU setting"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingControlPlaneRAM", func(t *testing.T) {
		setup()

		// Given: a mock context and cluster driver set to "talos"
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And mockConfigHandler.GetInt returns an error when retrieving control plane RAM
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.controlplanes.memory" {
				return 0, fmt.Errorf("mock error retrieving control plane RAM setting")
			}
			// Provide default values for other keys to proceed
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		expectedError := "error retrieving control plane RAM setting"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingWorkerCPU", func(t *testing.T) {
		setup()

		// Given: a mock context and cluster driver set to "talos"
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And mockConfigHandler.GetInt returns an error when retrieving worker CPU
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.workers.cpu" {
				return 0, fmt.Errorf("mock error retrieving worker CPU setting")
			}
			// Provide default values for other keys to proceed
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		expectedError := "error retrieving worker CPU setting"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingWorkerRAM", func(t *testing.T) {
		setup()

		// Given: a mock context and cluster driver set to "talos"
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "talos", nil
			}
			return "", nil
		}

		// And mockConfigHandler.GetInt returns an error when retrieving worker RAM
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if key == "contexts.test-context.cluster.workers.memory" {
				return 0, fmt.Errorf("mock error retrieving worker RAM setting")
			}
			// Provide default values for other keys to proceed
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When calling GetComposeConfig
		_, err = talosHelper.GetComposeConfig()
		// Then an error should be returned
		expectedError := "error retrieving worker RAM setting"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})
}

func TestTalosHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Common setup for tests
		var (
			mockContext       *context.MockContext
			mockConfigHandler *config.MockConfigHandler
			mockShell         *shell.MockShell
			diContainer       *di.DIContainer
		)

		setup := func() {
			mockContext = context.NewMockContext()
			mockConfigHandler = config.NewMockConfigHandler()
			mockShell = &shell.MockShell{}
			diContainer = di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", mockShell)
		}

		setup()

		// Given: a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/path/to/config", nil
		}

		// And: a mock shell
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/path/to/project", nil
		}

		// When: WriteConfig is called
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}
		err = talosHelper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// Custom comparison function for ServiceConfig slices
func compareServiceConfigs(actual, expected []types.ServiceConfig) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if !compareServiceConfig(actual[i], expected[i]) {
			return false
		}
	}
	return true
}

func compareServiceConfig(actual, expected types.ServiceConfig) bool {
	// Compare fields directly
	if actual.Name != expected.Name ||
		actual.Image != expected.Image ||
		actual.Restart != expected.Restart ||
		actual.ReadOnly != expected.ReadOnly ||
		actual.Privileged != expected.Privileged ||
		!reflect.DeepEqual(actual.SecurityOpt, expected.SecurityOpt) ||
		!reflect.DeepEqual(actual.Tmpfs, expected.Tmpfs) {
		return false
	}

	// Compare Environment maps
	if !compareStringPointerMaps(actual.Environment, expected.Environment) {
		return false
	}

	// Compare Volumes
	if !reflect.DeepEqual(actual.Volumes, expected.Volumes) {
		return false
	}

	return true
}

func compareStringPointerMaps(actual, expected map[string]*string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, val := range actual {
		expectedVal, exists := expected[key]
		if !exists {
			return false
		}
		if val == nil && expectedVal == nil {
			continue
		}
		if val == nil || expectedVal == nil {
			return false
		}
		if *val != *expectedVal {
			return false
		}
	}
	return true
}
