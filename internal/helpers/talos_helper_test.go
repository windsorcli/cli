package helpers

import (
	"errors"
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
			mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
				return &config.Context{
					Cluster: &config.ClusterConfig{
						Driver: ptrString("talos"),
					},
				}, nil
			}
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

		// When calling Initialize
		err = talosHelper.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessCreatingVolumesDirectory", func(t *testing.T) {
		// Given: a mock context and config handler with "talos" as the cluster driver
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigFunc: func() (*config.Context, error) {
				return &config.Context{
					Cluster: &config.ClusterConfig{
						Driver: ptrString("talos"),
					},
				}, nil
			},
		}
		mockShell := &shell.MockShell{
			GetProjectRootFunc: func() (string, error) {
				return "/mock/project/root", nil
			},
		}

		// Mock the stat and mkdir functions
		originalStat := stat
		originalMkdir := mkdir
		defer func() {
			stat = originalStat
			mkdir = originalMkdir
		}()

		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mkdir = func(name string, perm os.FileMode) error {
			return nil
		}

		// Setup DI container with mock components
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling Initialize
		err = talosHelper.Initialize()

		// Then: no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given: a mock context and config handler with "talos" as the cluster driver
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigFunc: func() (*config.Context, error) {
				return &config.Context{
					Cluster: &config.ClusterConfig{
						Driver: ptrString("talos"),
					},
				}, nil
			},
		}
		mockShell := &shell.MockShell{
			GetProjectRootFunc: func() (string, error) {
				return "", errors.New("mock error retrieving project root")
			},
		}

		// Setup DI container with mock components
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling Initialize
		err = talosHelper.Initialize()

		// Then: an error should be returned
		expectedError := "error retrieving project root"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorCreatingVolumesDirectory", func(t *testing.T) {
		// Given: a mock context and config handler with "talos" as the cluster driver
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigFunc: func() (*config.Context, error) {
				return &config.Context{
					Cluster: &config.ClusterConfig{
						Driver: ptrString("talos"),
					},
				}, nil
			},
		}
		mockShell := &shell.MockShell{
			GetProjectRootFunc: func() (string, error) {
				return "/mock/project/root", nil
			},
		}

		// Mock the stat and mkdir functions
		originalStat := stat
		originalMkdir := mkdir
		defer func() {
			stat = originalStat
			mkdir = originalMkdir
		}()

		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		mkdir = func(name string, perm os.FileMode) error {
			return errors.New("mock error creating .volumes folder")
		}

		// Setup DI container with mock components
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// When: creating a new TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// And calling Initialize
		err = talosHelper.Initialize()

		// Then: an error should be returned
		expectedError := "error creating .volumes folder"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error containing %q, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingContextConfiguration", func(t *testing.T) {
		// Given a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context configuration")
		}

		// And a mock context and shell
		mockContext := context.NewMockContext()
		mockShell := &shell.MockShell{}

		// And a DI container with the mock config handler, context, and shell registered
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// When creating a new TalosHelper
		helper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// And calling Initialize
		err = helper.Initialize()

		// Then it should return an error indicating context configuration retrieval failure
		if err == nil || !strings.Contains(err.Error(), "error retrieving context configuration") {
			t.Fatalf("expected error retrieving context configuration, got %v", err)
		}
	})

	t.Run("NonTalosClusterDriver", func(t *testing.T) {
		// Given a mock config handler with a non-Talos cluster driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("kubernetes"),
				},
			}, nil
		}

		// And a mock context and shell
		mockContext := context.NewMockContext()
		mockShell := &shell.MockShell{}

		// And a DI container with the mock config handler, context, and shell registered
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// When creating a new TalosHelper
		helper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// And calling Initialize
		err = helper.Initialize()

		// Then it should return nil, indicating no action was taken
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
				},
			}, nil
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
				},
			}, nil
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
				},
			}, nil
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(1), CPU: ptrInt(2), Memory: ptrInt(2048)},
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(1), CPU: ptrInt(4), Memory: ptrInt(4096)},
				},
			}, nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		containerConfig, err := talosHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the result should not be nil
		if containerConfig == nil {
			t.Fatalf("expected non-nil containerConfig, got nil")
		}

		// And the number of services should be controlPlanes + workers
		expectedServiceCount := 2
		if len(containerConfig.Services) != expectedServiceCount {
			t.Errorf("expected %d services, got %d", expectedServiceCount, len(containerConfig.Services))
		}

		// Validate the services
		for i := 0; i < 1; i++ {
			service := containerConfig.Services[i]
			expectedName := fmt.Sprintf("controlplane-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
		}

		for i := 0; i < 1; i++ {
			service := containerConfig.Services[1+i]
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(1), CPU: ptrInt(2), Memory: ptrInt(2)}, // Memory in GB
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(1), CPU: ptrInt(4), Memory: ptrInt(4)}, // Memory in GB
				},
			}, nil
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("kubernetes"),
				},
			}, nil
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

	t.Run("EmptyClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context and config handler that returns an empty string for the cluster driver
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString(""),
				},
			}, nil
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: nil},
				},
			}, fmt.Errorf("error retrieving number of control planes")
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(0)},
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(0)},
				},
			}, nil
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
		customRAMGB := 16 // RAM in GB

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(customControlPlanes), CPU: ptrInt(customCPUCores), Memory: ptrInt(customRAMGB)},
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: ptrInt(customWorkers), CPU: ptrInt(customCPUCores), Memory: ptrInt(customRAMGB)},
				},
			}, nil
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

		// Convert RAM from GB to MB for expected values
		expectedRAMMB := customRAMGB * 1024

		// Validate the services
		for i := 0; i < customControlPlanes; i++ {
			service := composeConfig.Services[i]
			expectedName := fmt.Sprintf("controlplane-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
			expectedSKU := fmt.Sprintf("%dCPU-%dRAM", customCPUCores, expectedRAMMB)
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
			expectedSKU := fmt.Sprintf("%dCPU-%dRAM", customCPUCores, expectedRAMMB)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Count: nil},
				},
			}, fmt.Errorf("error retrieving number of workers")
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{CPU: nil},
				},
			}, fmt.Errorf("mock error retrieving control plane CPU setting")
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					ControlPlanes: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Memory: nil},
				},
			}, fmt.Errorf("mock error retrieving control plane RAM setting")
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{CPU: nil},
				},
			}, fmt.Errorf("mock error retrieving worker CPU setting")
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

		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Cluster: &config.ClusterConfig{
					Driver: ptrString("talos"),
					Workers: struct {
						Count  *int `yaml:"count"`
						CPU    *int `yaml:"cpu"`
						Memory *int `yaml:"memory"`
					}{Memory: nil},
				},
			}, fmt.Errorf("mock error retrieving worker RAM setting")
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
