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
)

func TestTalosHelper_GetEnvVars(t *testing.T) {
	// Common setup for tests
	var (
		mockContext       *context.MockContext
		mockConfigHandler *config.MockConfigHandler
		diContainer       *di.DIContainer
	)

	setup := func() {
		mockContext = context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler = config.NewMockConfigHandler()
		diContainer = di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)
	}

	t.Run("Success", func(t *testing.T) {
		setup()

		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		talosConfigPath := filepath.Join(contextPath, ".talos", "config")

		// And the talos config file is created
		err := os.MkdirAll(filepath.Dir(talosConfigPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create talos config directory: %v", err)
		}
		_, err = os.Create(talosConfigPath)
		if err != nil {
			t.Fatalf("Failed to create talos config file: %v", err)
		}

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

	t.Run("FileNotExist", func(t *testing.T) {
		setup()

		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")

		// And a mock context is set up
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And the cluster driver is set to "talos"
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.non-existent-context.cluster.driver" {
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

		// Then the environment variables should be empty
		if envVars != nil {
			t.Errorf("expected nil, got %v", envVars)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		setup()

		// Given a mock context that returns an error for GetContext
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("error retrieving current context")
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// And calling GetEnvVars
		expectedError := "error retrieving current context"

		_, err = talosHelper.GetEnvVars()

		// Then it should return an error indicating current context retrieval failure
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And a mock config handler that returns an error for cluster driver
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "", errors.New("mock error retrieving cluster driver")
			}
			return "", nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// And calling GetEnvVars
		_, err = talosHelper.GetEnvVars()

		// Then it should return an error indicating cluster driver retrieval failure
		expectedError := "error retrieving cluster driver"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("NonTalosClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// And the cluster driver is set to something other than 'talos'
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "kubernetes", nil
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

		// Then it should return nil
		if envVars != nil {
			t.Errorf("expected nil envVars when cluster driver is not 'talos', got %v", envVars)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
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

		// And mockContext.GetConfigRoot returns an error
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock error retrieving config root")
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// And calling GetEnvVars
		_, err = talosHelper.GetEnvVars()

		// Then it should return an error indicating config root retrieval failure
		expectedError := "error retrieving context config path"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("TalosConfigFileDoesNotExist", func(t *testing.T) {
		setup()

		// Given: a valid context path where talos config file does not exist
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		talosConfigPath := filepath.Join(contextPath, ".talos", "config")

		// Ensure the talos config file does not exist
		os.Remove(talosConfigPath)

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
}

func TestTalosHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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
}

func TestTalosHelper_GetContainerConfig(t *testing.T) {
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
		containerConfig, err := talosHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should not be nil
		if containerConfig == nil {
			t.Fatalf("expected non-nil containerConfig, got nil")
		}

		// And the number of services should be controlPlanes + workers
		expectedServiceCount := customControlPlanes + customWorkers
		if len(containerConfig) != expectedServiceCount {
			t.Errorf("expected %d services, got %d", expectedServiceCount, len(containerConfig))
		}

		// Validate the services
		for i := 0; i < customControlPlanes; i++ {
			service := containerConfig[i]
			expectedName := fmt.Sprintf("controlplane-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
		}

		for i := 0; i < customWorkers; i++ {
			service := containerConfig[customControlPlanes+i]
			expectedName := fmt.Sprintf("worker-%d.test", i+1)
			if service.Name != expectedName {
				t.Errorf("expected service name %s, got %s", expectedName, service.Name)
			}
		}
	})

	t.Run("SuccessWithDefaultSettings", func(t *testing.T) {
		setup()

		// Set the necessary environment variable
		os.Setenv("WINDSOR_PROJECT_ROOT", "/mock/project/root")
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

		// And calling GetContainerConfig
		services, err := talosHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Define expected values
		expectedControlPlaneCPUCores := 2
		expectedControlPlaneRAMGB := 2
		expectedControlPlaneRAMMB := expectedControlPlaneRAMGB * 1024

		expectedWorkerCPUCores := 4
		expectedWorkerRAMGB := 4
		expectedWorkerRAMMB := expectedWorkerRAMGB * 1024

		// Validate the services
		if len(services) != 2 {
			t.Fatalf("expected 2 services, got %d", len(services))
		}

		controlPlaneService := services[0]
		if controlPlaneService.Name != "controlplane-1.test" {
			t.Errorf("expected controlplane-1.test, got %s", controlPlaneService.Name)
		}
		if *controlPlaneService.Environment["PLATFORM"] != "container" {
			t.Errorf("expected PLATFORM=container, got %s", *controlPlaneService.Environment["PLATFORM"])
		}
		if *controlPlaneService.Environment["TALOSSKU"] != fmt.Sprintf("%dCPU-%dRAM", expectedControlPlaneCPUCores, expectedControlPlaneRAMMB) {
			t.Errorf("expected TALOSSKU=%dCPU-%dRAM, got %s", expectedControlPlaneCPUCores, expectedControlPlaneRAMMB, *controlPlaneService.Environment["TALOSSKU"])
		}

		workerService := services[1]
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
		if workerService.Volumes[8].Source != filepath.Join("/mock/project/root", ".volumes") {
			t.Errorf("expected volume source /mock/project/root/.volumes, got %s", workerService.Volumes[8].Source)
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

		// And calling GetContainerConfig
		services, err := talosHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then services should be nil
		if services != nil {
			t.Errorf("expected services to be nil when cluster driver is not Talos, got: %v", services)
		}
	})

	t.Run("ErrorRetrievingCurrentContext", func(t *testing.T) {
		setup()

		// Given: a mock context that returns an error
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context error")
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving current context") {
			t.Errorf("expected error retrieving current context, got: %v", err)
		}
	})

	t.Run("ErrorRetrievingClusterDriver", func(t *testing.T) {
		setup()

		// Given: a mock context and config handler that returns error for cluster driver
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", fmt.Errorf("mock cluster driver error")
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create TalosHelper: %v", err)
		}

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving cluster driver") {
			t.Errorf("expected error retrieving cluster driver, got: %v", err)
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		services, err := talosHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}
		// Then: services should be an empty slice
		if len(services) != 0 {
			t.Errorf("expected 0 services, got %d", len(services))
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

		// And calling GetContainerConfig
		services, err := talosHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then the number of services should be controlPlanes + workers
		expectedServiceCount := customControlPlanes + customWorkers
		if len(services) != expectedServiceCount {
			t.Errorf("expected %d services, got %d", expectedServiceCount, len(services))
		}

		// Validate the services
		for i := 0; i < customControlPlanes; i++ {
			service := services[i]
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
			service := services[customControlPlanes+i]
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		// When calling GetContainerConfig
		_, err = talosHelper.GetContainerConfig()
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

		// Given: a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/path/to/config", nil
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
