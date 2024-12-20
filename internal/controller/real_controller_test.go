package controller

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
)

func TestNewRealController(t *testing.T) {
	t.Run("NewRealController", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// When creating a new real controller
		controller := NewRealController(injector)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatalf("expected controller, got nil")
		} else {
			t.Logf("Success: controller created")
		}
	})
}

func TestRealController_CreateCommonComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if injector.Resolve("configHandler") == nil {
			t.Fatalf("expected configHandler to be registered, got error")
		}
		if injector.Resolve("contextHandler") == nil {
			t.Fatalf("expected contextHandler to be registered, got error")
		}
		if injector.Resolve("shell") == nil {
			t.Fatalf("expected shell to be registered, got error")
		}

		t.Logf("Success: common components created and registered")
	})
}

func TestRealController_CreateEnvComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if injector.Resolve("awsEnv") == nil {
			t.Fatalf("expected awsEnv to be registered, got error")
		}
		if injector.Resolve("dockerEnv") == nil {
			t.Fatalf("expected dockerEnv to be registered, got error")
		}
		if injector.Resolve("kubeEnv") == nil {
			t.Fatalf("expected kubeEnv to be registered, got error: %v", err)
		}
		if injector.Resolve("omniEnv") == nil {
			t.Fatalf("expected omniEnv to be registered, got error")
		}
		if injector.Resolve("sopsEnv") == nil {
			t.Fatalf("expected sopsEnv to be registered, got error")
		}
		if injector.Resolve("talosEnv") == nil {
			t.Fatalf("expected talosEnv to be registered, got error")
		}
		if injector.Resolve("terraformEnv") == nil {
			t.Fatalf("expected terraformEnv to be registered, got error")
		}
		if injector.Resolve("windsorEnv") == nil {
			t.Fatalf("expected windsorEnv to be registered, got error: %v", err)
		}

		t.Logf("Success: env components created and registered")
	})
}

func TestRealController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()

		// Configure the mock to return necessary values for testing CreateServiceComponents
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "docker.enabled":
				return true
			case "dns.enabled":
				return true
			case "git.livereload.enabled":
				return true
			case "aws.localstack.enabled":
				return true
			case "cluster.enabled":
				return true
			default:
				return false
			}
		}
		mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
			switch key {
			case "cluster.controlplanes.count":
				return 2
			case "cluster.workers.count":
				return 3
			default:
				return 0
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the services should be registered in the injector
		if injector.Resolve("dnsService") == nil {
			t.Fatalf("expected dnsService to be registered, got error")
		}
		if injector.Resolve("gitLivereloadService") == nil {
			t.Fatalf("expected gitLivereloadService to be registered, got error")
		}
		if injector.Resolve("localstackService") == nil {
			t.Fatalf("expected localstackService to be registered, got error: %v", err)
		}
		for i := 1; i <= 2; i++ {
			serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
			if injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}
		for i := 1; i <= 3; i++ {
			serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
			if injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}
		for _, registry := range []string{"registry1", "registry2"} {
			serviceName := fmt.Sprintf("registryService.%s", registry)
			if injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}

		t.Logf("Success: service components created and registered")
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And a mock config handler with GetBool("docker.enabled") returning false
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return true
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And no services should be registered in the injector
		if injector.Resolve("dnsService") != nil {
			t.Fatalf("expected dnsService not to be registered")
		}
		if injector.Resolve("gitLivereloadService") != nil {
			t.Fatalf("expected gitLivereloadService not to be registered")
		}
		if injector.Resolve("localstackService") != nil {
			t.Fatalf("expected localstackService not to be registered")
		}

		t.Logf("Success: no service components created or registered")
	})
}

func TestRealController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And a mock config handler with GetString("vm.driver") returning "colima"
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the colima virtual machine should be registered in the injector
		if injector.Resolve("virtualMachine") == nil {
			t.Fatalf("expected virtualMachine to be registered, got error")
		}

		// And the colima network manager should be registered in the injector
		if injector.Resolve("networkManager") == nil {
			t.Fatalf("expected networkManager to be registered, got error")
		}

		t.Logf("Success: virtualization components created and registered")
	})

	t.Run("EmptyDriver", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And a mock config handler with GetString("vm.driver") returning an empty string
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return ""
			}
			return ""
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the virtual machine should not be registered in the injector
		if injector.Resolve("virtualMachine") != nil {
			t.Fatalf("expected virtualMachine not to be registered")
		}

		// And the network manager should not be registered in the injector
		if injector.Resolve("networkManager") != nil {
			t.Fatalf("expected networkManager not to be registered")
		}

		t.Logf("Success: no virtualization components created or registered")
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And a mock config handler with GetBool("docker.enabled") returning false
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the container runtime should not be registered in the injector
		if injector.Resolve("containerRuntime") != nil {
			t.Fatalf("expected containerRuntime not to be registered")
		}

		t.Logf("Success: no container runtime created or registered")
	})
}

func TestRealController_CreateStackComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// When creating stack components
		err := controller.CreateStackComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the stack should be registered in the injector
		if injector.Resolve("stack") == nil {
			t.Fatalf("expected stack to be registered, got error")
		}

		t.Logf("Success: stack components created and registered")
	})
}
