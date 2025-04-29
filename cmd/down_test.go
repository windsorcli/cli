package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Types
// =============================================================================

// Extend Mocks with additional fields needed for down command tests
type DownMocks struct {
	*Mocks
	ContainerRuntime *virt.MockVirt
}

// =============================================================================
// Helpers
// =============================================================================

func setupDownMocks(t *testing.T, opts ...*SetupOptions) *DownMocks {
	t.Helper()

	// Process options with defaults
	options := &SetupOptions{
		ConfigStr: `
contexts:
  default:
    docker:
      enabled: true`,
	}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	mocks := setupMocks(t, options)

	// Store original shims and restore after test
	originalShims := shims
	newShims := NewShims()
	shims = newShims
	t.Cleanup(func() {
		shims = originalShims
	})

	containerRuntime := virt.NewMockVirt()
	containerRuntime.DownFunc = func() error { return nil }
	mocks.Injector.Register("containerRuntime", containerRuntime)

	mocks.Controller.SetEnvironmentVariablesFunc = func() error { return nil }
	mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error { return nil }

	return &DownMocks{
		Mocks:            mocks,
		ContainerRuntime: containerRuntime,
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestDownCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return nil
		}
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return nil
		}

		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingWithRequirements", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorNilContainerRuntime", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.Controller.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No container runtime found" {
			t.Errorf("Expected 'No container runtime found', got '%v'", err)
		}
	})

	t.Run("ErrorContainerRuntimeDown", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.ContainerRuntime.DownFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorCleaningConfig", func(t *testing.T) {
		// Create a mock config handler with a failing Clean method
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error { return nil }
		mockConfigHandler.LoadConfigStringFunc = func(content string) error { return nil }
		mockConfigHandler.SetContextFunc = func(context string) error { return nil }
		mockConfigHandler.CleanFunc = func() error { return fmt.Errorf("test error") }

		// Set up mocks with the mock config handler
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
			ConfigStr: `
contexts:
  default:
    docker:
      enabled: true`,
		}
		mocks := setupDownMocks(t, opts)

		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.CleanFunc = func() error { return nil }
		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
			ConfigStr: `
contexts:
  default:
    docker:
      enabled: true`,
		}
		mocks := setupDownMocks(t, opts)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error retrieving project root") {
			t.Errorf("Expected error to contain 'Error retrieving project root', got: %v", err)
		}
	})

	t.Run("ErrorRemovingVolumes", func(t *testing.T) {
		mocks := setupDownMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/path", nil
		}
		shims.RemoveAll = func(path string) error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error deleting .volumes folder") {
			t.Errorf("Expected error to contain 'Error deleting .volumes folder', got: %v", err)
		}
	})
}
