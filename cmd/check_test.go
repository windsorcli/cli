package cmd

import (
  "bytes"
  "fmt"
  "testing"

  "github.com/windsorcli/cli/pkg/controller"
  "github.com/windsorcli/cli/pkg/tools"
)

func TestCheckCmd(t *testing.T) {
  setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
    t.Helper()

    // Setup mocks with default options
    mocks := setupMocks(t, opts...)

    // Setup command args and output
    rootCmd.SetArgs([]string{"check"})
    stdout, stderr := captureOutput(t)
    rootCmd.SetOut(stdout)
    rootCmd.SetErr(stderr)

    return mocks, stdout, stderr
  }

  t.Run("Success", func(t *testing.T) {
    // Given a set of mocks with proper configuration
    mocks, stdout, stderr := setup(t, &SetupOptions{
      ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
    })

    // And mock tools manager that returns success
    mockToolsManager := tools.NewMockToolsManager()
    mockToolsManager.CheckFunc = func() error {
      return nil
    }
    mocks.Injector.Register("toolsManager", mockToolsManager)

    // When executing the command
    err := Execute(mocks.Controller)

    // Then no error should occur
    if err != nil {
      t.Errorf("Expected success, got error: %v", err)
    }

    // And output should contain success message
    output := stdout.String()
    if output != "All tools are up to date.\n" {
      t.Errorf("Expected 'All tools are up to date.', got: %q", output)
    }
    if stderr.String() != "" {
      t.Error("Expected empty stderr")
    }
  })

  t.Run("ConfigNotLoaded", func(t *testing.T) {
    // Given a set of mocks with no configuration
    mocks, _, _ := setup(t)

    // When executing the command
    err := Execute(mocks.Controller)

    // Then an error should occur
    if err == nil {
      t.Error("Expected error, got nil")
    }

    // And error should contain init message
    expectedError := "Nothing to check. Have you run \033[1mwindsor init\033[0m?"
    if err.Error() != expectedError {
      t.Errorf("Expected error about init, got: %v", err)
    }
  })

  t.Run("ToolsManagerNotFound", func(t *testing.T) {
    // Given a set of mocks with proper configuration
    mocks, _, _ := setup(t, &SetupOptions{
      ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
    })

    // And mock controller that returns nil tools manager
    mocks.Controller.ResolveToolsManagerFunc = func() tools.ToolsManager {
      return nil
    }

    // When executing the command
    err := Execute(mocks.Controller)

    // Then an error should occur
    if err == nil {
      t.Error("Expected error, got nil")
    }

    // And error should contain tools manager message
    if err.Error() != "No tools manager found" {
      t.Errorf("Expected error about tools manager, got: %v", err)
    }
  })

  t.Run("ToolsCheckError", func(t *testing.T) {
    // Given a set of mocks with proper configuration
    mocks, _, _ := setup(t, &SetupOptions{
      ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
    })

    // And mock tools manager that returns error
    mockToolsManager := tools.NewMockToolsManager()
    mockToolsManager.CheckFunc = func() error {
      return fmt.Errorf("tools check failed")
    }
    mocks.Injector.Register("toolsManager", mockToolsManager)

    // When executing the command
    err := Execute(mocks.Controller)

    // Then an error should occur
    if err == nil {
      t.Error("Expected error, got nil")
    }

    // And error should contain tools check message
    if err.Error() != "Error checking tools: tools check failed" {
      t.Errorf("Expected error about tools check, got: %v", err)
    }
  })

  t.Run("InitializeWithRequirementsError", func(t *testing.T) {
    // Given a set of mocks with proper configuration
    mocks, _, _ := setup(t, &SetupOptions{
      ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
    })

    // And mock controller that returns error on initialization
    mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
      return fmt.Errorf("initialization failed")
    }

    // When executing the command
    err := Execute(mocks.Controller)

    // Then an error should occur
    if err == nil {
      t.Error("Expected error, got nil")
    }

    // And error should contain initialization message
    if err.Error() != "Error initializing: initialization failed" {
      t.Errorf("Expected error about initialization, got: %v", err)
    }
  })
}
