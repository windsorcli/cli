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
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
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
		talosConfigPath := ""

		// And a mock context is set up
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
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

		// Then the environment variables should be set correctly with an empty TALOSCONFIG
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": talosConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		setup()

		// Given a mock context that returns an error for config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("error retrieving config root")
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("failed to create talos helper: %v", err)
		}

		// And calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = talosHelper.GetEnvVars()

		// Then it should return an error indicating config root retrieval failure
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}
	
func TestTalosHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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
	})

	t.Run("NewTalosHelper", func(t *testing.T) {
		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Given a DI container without registering context
			diContainer := di.NewContainer()

			// When attempting to create TalosHelper
			_, err := NewTalosHelper(diContainer)

			// Then it should return an error indicating context resolution failure
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		setup()

		// Given a mock context
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// When creating TalosHelper
		talosHelper, err := NewTalosHelper(diContainer)
		if err != nil {
			t.Fatalf("NewTalosHelper() error = %v", err)
		}

		t.Run("Success", func(t *testing.T) {
			// When: GetContainerConfig is called
			containerConfig, err := talosHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: the result should be nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
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
	})
}

func TestTalosHelper_GetContainerConfig(t *testing.T) {
	t.Run("SuccessWithDefaultSettings", func(t *testing.T) {
		// Set the necessary environment variable
		os.Setenv("WINDSOR_PROJECT_ROOT", "/mock/project/root")
		defer os.Unsetenv("WINDSOR_PROJECT_ROOT")

		// Given: a mock context and config handler with default settings
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
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) {
			if len(defaultValue) > 0 {
				return defaultValue[0], nil
			}
			return 0, fmt.Errorf("no default value provided for key: %s", key)
		}

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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

		// Define the expected services
		expectedServices := []types.ServiceConfig{
			{
				Name:    "controlplane-1.test",
				Image:   constants.DEFAULT_TALOS_IMAGE,
				Restart: "always",
				Environment: map[string]*string{
					"PLATFORM": strPtr("container"),
					"TALOSSKU": strPtr(fmt.Sprintf("%dCPU-%dRAM", expectedControlPlaneCPUCores, expectedControlPlaneRAMMB)),
				},
				ReadOnly:    true,
				Privileged:  true,
				SecurityOpt: []string{"seccomp=unconfined"},
				Tmpfs:       []string{"/run", "/system", "/tmp"},
				Volumes: []types.ServiceVolumeConfig{
					{Type: "bind", Source: "/run/udev", Target: "/run/udev"},
					{Type: "volume", Source: "system_state", Target: "/system/state"},
					{Type: "volume", Source: "var", Target: "/var"},
					{Type: "volume", Source: "etc_cni", Target: "/etc/cni"},
					{Type: "volume", Source: "etc_kubernetes", Target: "/etc/kubernetes"},
					{Type: "volume", Source: "usr_libexec_kubernetes", Target: "/usr/libexec/kubernetes"},
					{Type: "volume", Source: "usr_etc_udev", Target: "/usr/etc/udev"},
					{Type: "volume", Source: "opt", Target: "/opt"},
				},
			},
			{
				Name:    "worker-1.test",
				Image:   constants.DEFAULT_TALOS_IMAGE,
				Restart: "always",
				Environment: map[string]*string{
					"PLATFORM": strPtr("container"),
					"TALOSSKU": strPtr(fmt.Sprintf("%dCPU-%dRAM", expectedWorkerCPUCores, expectedWorkerRAMMB)),
				},
				ReadOnly:    true,
				Privileged:  true,
				SecurityOpt: []string{"seccomp=unconfined"},
				Tmpfs:       []string{"/run", "/system", "/tmp"},
				Volumes: []types.ServiceVolumeConfig{
					{Type: "bind", Source: "/run/udev", Target: "/run/udev"},
					{Type: "volume", Source: "system_state", Target: "/system/state"},
					{Type: "volume", Source: "var", Target: "/var"},
					{Type: "volume", Source: "etc_cni", Target: "/etc/cni"},
					{Type: "volume", Source: "etc_kubernetes", Target: "/etc/kubernetes"},
					{Type: "volume", Source: "usr_libexec_kubernetes", Target: "/usr/libexec/kubernetes"},
					{Type: "volume", Source: "usr_etc_udev", Target: "/usr/etc/udev"},
					{Type: "volume", Source: "opt", Target: "/opt"},
					// Add the additional volume for the worker
					{Type: "bind", Source: filepath.Join("/mock/project/root", ".volumes"), Target: "/var/local"},
				},
			},
		}

		// Compare using cmp.Diff
		if diff := cmp.Diff(expectedServices, services,
			cmpopts.IgnoreUnexported(types.ServiceConfig{}, types.ServiceVolumeConfig{}),
			cmpopts.SortSlices(func(a, b types.ServiceVolumeConfig) bool {
				return a.Target < b.Target
			}),
		); diff != "" {
			t.Errorf("services mismatch (-expected +got):\n%s", diff)
		}
	})

	t.Run("NonTalosClusterDriver", func(t *testing.T) {
		// Given: a mock context and config handler where cluster driver is not Talos
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			if key == "contexts.test-context.cluster.driver" {
				return "kubernetes", nil
			}
			return "", fmt.Errorf("unexpected key: %s", key)
		}

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
		// Given: a mock context that returns an error
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context error")
		}

		mockConfigHandler := config.NewMockConfigHandler()

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
		// Given: a mock context and config handler that returns error for cluster driver
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", fmt.Errorf("mock cluster driver error")
		}

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
		// Given: a mock context and config handler that returns error for controlplanes.count
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
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

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
		// Given: control plane and worker counts are zero
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
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
			return 0, fmt.Errorf("unexpected key: %s", key)
		}
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
		// Given: custom settings for counts and resources
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "custom-context", nil
		}

		customControlPlanes := 2
		customWorkers := 3
		customCPUCores := 8
		customRAM := 16 // GB

		mockConfigHandler := config.NewMockConfigHandler()
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

		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

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
