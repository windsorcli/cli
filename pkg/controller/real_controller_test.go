package controller

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
)

func TestNewRealController(t *testing.T) {
	t.Run("NewRealController", func(t *testing.T) {
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
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

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
		if injector.Resolve("shell") == nil {
			t.Fatalf("expected shell to be registered, got error")
		}

		t.Logf("Success: common components created and registered")
	})
}

func TestRealController_CreateProjectComponents(t *testing.T) {
	// t.Run("Success", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// When creating project components
	// 	err := controller.CreateProjectComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And the components should be registered in the injector
	// 	if injector.Resolve("gitGenerator") == nil {
	// 		t.Fatalf("expected gitGenerator to be registered, got error")
	// 	}
	// 	if injector.Resolve("blueprintHandler") == nil {
	// 		t.Fatalf("expected blueprintHandler to be registered, got error")
	// 	}
	// 	if injector.Resolve("terraformGenerator") == nil {
	// 		t.Fatalf("expected terraformGenerator to be registered, got error")
	// 	}

	// 	t.Logf("Success: project components created and registered")
	// })

	t.Run("DefaultToolsManagerCreation", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "toolsManager" {
				return ""
			}
			return ""
		}
		injector.Register("configHandler", mockConfigHandler)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the default tools manager should be registered
		if injector.Resolve("toolsManager") == nil {
			t.Fatalf("expected default toolsManager to be registered, got error")
		}
	})
}

func TestRealController_CreateEnvComponents(t *testing.T) {
	// t.Run("Success", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// Override the existing configHandler with a mock configHandler
	// 	mockConfigHandler := config.NewMockConfigHandler()

	// 	// Configure the mock to return necessary values for testing CreateEnvComponents
	// 	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
	// 		switch key {
	// 		case "aws.enabled":
	// 			return true
	// 		case "docker.enabled":
	// 			return true
	// 		default:
	// 			return false
	// 		}
	// 	}
	// 	injector.Register("configHandler", mockConfigHandler)

	// 	// When creating env components
	// 	err := controller.CreateEnvComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And the components should be registered in the injector
	// 	if injector.Resolve("awsEnv") == nil {
	// 		t.Fatalf("expected awsEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("dockerEnv") == nil {
	// 		t.Fatalf("expected dockerEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("kubeEnv") == nil {
	// 		t.Fatalf("expected kubeEnv to be registered, got error: %v", err)
	// 	}
	// 	if injector.Resolve("omniEnv") == nil {
	// 		t.Fatalf("expected omniEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("sopsEnv") == nil {
	// 		t.Fatalf("expected sopsEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("talosEnv") == nil {
	// 		t.Fatalf("expected talosEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("terraformEnv") == nil {
	// 		t.Fatalf("expected terraformEnv to be registered, got error")
	// 	}
	// 	if injector.Resolve("windsorEnv") == nil {
	// 		t.Fatalf("expected windsorEnv to be registered, got error")
	// 	}

	// 	t.Logf("Success: env components created and registered")
	// })
}

func TestRealController_CreateServiceComponents(t *testing.T) {
	// t.Run("Success", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// Override the existing configHandler with a mock configHandler
	// 	mockConfigHandler := config.NewMockConfigHandler()

	// 	// Configure the mock to return necessary values for testing CreateServiceComponents
	// 	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
	// 		switch key {
	// 		case "docker.enabled":
	// 			return true
	// 		case "dns.enabled":
	// 			return true
	// 		case "git.livereload.enabled":
	// 			return true
	// 		case "aws.localstack.enabled":
	// 			return true
	// 		case "cluster.enabled":
	// 			return true
	// 		default:
	// 			return false
	// 		}
	// 	}
	// 	injector.Register("configHandler", mockConfigHandler)

	// 	// When creating service components
	// 	err := controller.CreateServiceComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And at least one service should be registered in the injector
	// 	if injector.Resolve("dnsService") == nil &&
	// 		injector.Resolve("gitLivereloadService") == nil &&
	// 		injector.Resolve("localstackService") == nil {
	// 		t.Fatalf("expected at least one service to be registered, got none")
	// 	}

	// 	t.Logf("Success: service components created and registered")
	// })

	// t.Run("DockerDisabled", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// And a mock config handler with GetBool("docker.enabled") returning false
	// 	mockConfigHandler := config.NewMockConfigHandler()
	// 	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
	// 		if key == "docker.enabled" {
	// 			return false
	// 		}
	// 		return true
	// 	}
	// 	injector.Register("configHandler", mockConfigHandler)

	// 	// When creating service components
	// 	err := controller.CreateServiceComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And no services should be registered in the injector
	// 	if injector.Resolve("dnsService") != nil {
	// 		t.Fatalf("expected dnsService not to be registered")
	// 	}
	// 	if injector.Resolve("gitLivereloadService") != nil {
	// 		t.Fatalf("expected gitLivereloadService not to be registered")
	// 	}
	// 	if injector.Resolve("localstackService") != nil {
	// 		t.Fatalf("expected localstackService not to be registered")
	// 	}

	// 	t.Logf("Success: no service components created or registered")
	// })
}

func TestRealController_CreateVirtualizationComponents(t *testing.T) {
	// t.Run("SuccessWithColimaDriver", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// And a mock config handler with GetString("vm.driver") returning "colima"
	// 	mockConfigHandler := config.NewMockConfigHandler()
	// 	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
	// 		if key == "vm.driver" {
	// 			return "colima"
	// 		}
	// 		return ""
	// 	}
	// 	injector.Register("configHandler", mockConfigHandler)

	// 	// When creating virtualization components
	// 	err := controller.CreateVirtualizationComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And the colima virtual machine should be registered in the injector
	// 	if injector.Resolve("virtualMachine") == nil {
	// 		t.Fatalf("expected virtualMachine to be registered, got error")
	// 	}

	// 	// And the network interface provider should be registered in the injector
	// 	if injector.Resolve("networkInterfaceProvider") == nil {
	// 		t.Fatalf("expected networkInterfaceProvider to be registered, got error")
	// 	}

	// 	// And the ssh client should be registered in the injector
	// 	if injector.Resolve("sshClient") == nil {
	// 		t.Fatalf("expected sshClient to be registered, got error")
	// 	}

	// 	// And the secure shell should be registered in the injector
	// 	if injector.Resolve("secureShell") == nil {
	// 		t.Fatalf("expected secureShell to be registered, got error")
	// 	}

	// 	t.Logf("Success: virtualization components created and registered")
	// })

	// t.Run("DockerDisabled", func(t *testing.T) {
	// 	// Given a new injector and a new real controller using mocks
	// 	injector := di.NewInjector()
	// 	controller := NewRealController(injector)

	// 	// Initialize the controller
	// 	if err := controller.Initialize(); err != nil {
	// 		t.Fatalf("failed to initialize controller: %v", err)
	// 	}

	// 	// And a mock config handler with GetBool("docker.enabled") returning false
	// 	mockConfigHandler := config.NewMockConfigHandler()
	// 	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
	// 		if key == "docker.enabled" {
	// 			return false
	// 		}
	// 		return false
	// 	}
	// 	injector.Register("configHandler", mockConfigHandler)

	// 	// When creating virtualization components
	// 	err := controller.CreateVirtualizationComponents()

	// 	// Then there should be no error
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}

	// 	// And the container runtime should not be registered in the injector
	// 	if injector.Resolve("containerRuntime") != nil {
	// 		t.Fatalf("expected containerRuntime not to be registered")
	// 	}

	// 	t.Logf("Success: no container runtime created or registered")
	// })
}

func TestRealController_CreateStackComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

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
