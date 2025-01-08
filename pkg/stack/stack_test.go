package stack

import (
	"fmt"
	"os"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockSafeComponents struct {
	Injector         di.Injector
	BlueprintHandler *blueprint.MockBlueprintHandler
	EnvPrinter       *env.MockEnvPrinter
	Shell            *shell.MockShell
}

// setupSafeMocks creates mock components for testing the stack
func setupSafeMocks(injector ...di.Injector) MockSafeComponents {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mockInjector)
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		// Define common components
		remoteComponent := blueprintv1alpha1.TerraformComponent{
			Source:   "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
			Path:     "remote/path",
			FullPath: "/mock/project/root/.windsor/.tf_modules/remote/path",
			Values: map[string]interface{}{
				"remote_variable1": "default_value",
			},
		}
		localComponent := blueprintv1alpha1.TerraformComponent{
			Source:   "",
			Path:     "local/path",
			FullPath: "/mock/project/root/terraform/local/path",
			Values: map[string]interface{}{
				"local_variable1": "default_value",
			},
		}

		return []blueprintv1alpha1.TerraformComponent{remoteComponent, localComponent}
	}
	mockInjector.Register("blueprintHandler", mockBlueprintHandler)

	// Create a mock env printer
	mockEnvPrinter := env.NewMockEnvPrinter()
	mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{
			"MOCK_ENV_VAR": "mock_value",
		}, nil
	}
	mockInjector.Register("envPrinter", mockEnvPrinter)

	// Create a mock shell
	mockShell := shell.NewMockShell()
	mockInjector.Register("shell", mockShell)

	// Mock osStat and osChdir functions
	osStat = func(_ string) (os.FileInfo, error) {
		return nil, nil
	}
	osChdir = func(_ string) error {
		return nil
	}
	osRemove = func(_ string) error {
		return nil
	}

	return MockSafeComponents{
		Injector:         mockInjector,
		BlueprintHandler: mockBlueprintHandler,
		EnvPrinter:       mockEnvPrinter,
		Shell:            mockShell,
	}
}

func TestStack_NewStack(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// When a new BaseStack is created
		stack := NewBaseStack(injector)

		// Then the stack should be non-nil
		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestStack_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		mocks := setupSafeMocks()

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)
		if err := stack.Initialize(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Initialize to return nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given safe mock components
		mocks := setupSafeMocks()

		// And the shell is unregistered to simulate an error
		mocks.Injector.Register("shell", nil)

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving shell"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		// Given safe mock components
		mocks := setupSafeMocks()

		// And the blueprintHandler is unregistered to simulate an error
		mocks.Injector.Register("blueprintHandler", nil)

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)

		// Then an error should occur
		if err := stack.Initialize(); err == nil {
			t.Errorf("Expected Initialize to return an error")
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		// Given safe mock components
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError((*env.EnvPrinter)(nil), fmt.Errorf("mock error resolving envPrinters"))
		mocks := setupSafeMocks(mockInjector)

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving envPrinters"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})
}

func TestStack_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		mocks := setupSafeMocks()

		// When a new BaseStack is brought up
		stack := NewBaseStack(mocks.Injector)
		if err := stack.Up(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})
}
