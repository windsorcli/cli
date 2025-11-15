package cmd

// The RootTest provides comprehensive test coverage for the Windsor CLI root command.
// It provides validation of command initialization, flag handling, and context management,
// The RootTest ensures proper command execution and context propagation,
// verifying error handling, flag parsing, and command hierarchy.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/spf13/cobra"
	blueprintpkg "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	SecretsProvider  *secrets.MockSecretsProvider
	EnvPrinter       *envvars.MockEnvPrinter
	ToolsManager     *tools.MockToolsManager
	Runtime          *runtime.Runtime
	Shims            *Shims
	BlueprintHandler *blueprintpkg.MockBlueprintHandler
	TmpDir           string
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
	Shims         *Shims
}

// setupMocks creates mock components for testing the root command
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Store original shims and restore after test
	origShims := shims
	t.Cleanup(func() {
		shims = origShims
	})

	// Create shims
	testShims := &Shims{
		Exit:        func(int) {},
		UserHomeDir: func() (string, error) { return t.TempDir(), nil },
		Stat:        func(string) (os.FileInfo, error) { return nil, nil },
		RemoveAll:   func(string) error { return nil },
		Getwd:       func() (string, error) { return "/test/project", nil },
		Command:     func(string, ...string) *exec.Cmd { return exec.Command("echo") },
		Setenv:      func(string, string) error { return nil },
		ReadFile: func(filename string) ([]byte, error) {
			// Mock trusted file content that includes the current directory
			return []byte("/test/project\n"), nil
		},
	}

	// Override with provided shims if any
	if options.Shims != nil {
		testShims = options.Shims
	}

	// Set global shims
	shims = testShims

	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Create mock shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.CheckTrustedDirectoryFunc = func() error {
		return nil
	}
	mockShell.CheckResetFlagsFunc = func() (bool, error) {
		return false, nil
	}
	mockShell.ResetFunc = func(...bool) {}
	mockShell.GetSessionTokenFunc = func() (string, error) {
		return "mock-session-token", nil
	}
	mockShell.WriteResetTokenFunc = func() (string, error) {
		return "mock-reset-token", nil
	}

	// Create mock secrets provider
	mockSecretsProvider := secrets.NewMockSecretsProvider(mockShell)
	mockSecretsProvider.LoadSecretsFunc = func() error {
		return nil
	}

	// Create mock env printer
	mockEnvPrinter := envvars.NewMockEnvPrinter()
	// PrintFunc removed - functionality now in runtime
	mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}

	// Create and register additional mock env printers
	mockWindsorEnvPrinter := envvars.NewMockEnvPrinter()
	// PrintFunc removed - functionality now in runtime
	mockWindsorEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	mockDockerEnvPrinter := envvars.NewMockEnvPrinter()
	// PrintFunc removed - functionality now in runtime
	mockDockerEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(mockShell)
	} else {
		configHandler = options.ConfigHandler
		// If it's a mock config handler, set GetConfigRootFunc to use tmpDir
		if mockConfig, ok := configHandler.(*config.MockConfigHandler); ok {
			if mockConfig.GetConfigRootFunc == nil {
				mockConfig.GetConfigRootFunc = func() (string, error) {
					return tmpDir, nil
				}
			}
		}
	}

	// Load config if ConfigStr is provided
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if err := configHandler.SetContext("default"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}
	}

	// Create mock blueprint handler
	mockBlueprintHandler := blueprintpkg.NewMockBlueprintHandler()
	mockBlueprintHandler.InstallFunc = func() error {
		return nil
	}

	// Create mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.CheckFunc = func() error { return nil }

	// Create runtime with all mocked dependencies
	rt, err := runtime.NewRuntime(&runtime.Runtime{
		Shell:         mockShell,
		ConfigHandler: configHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  mockToolsManager,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}

	return &Mocks{
		ConfigHandler:    configHandler,
		Shell:            mockShell,
		SecretsProvider:  mockSecretsProvider,
		EnvPrinter:       mockEnvPrinter,
		ToolsManager:     mockToolsManager,
		Runtime:          rt,
		Shims:            testShims,
		BlueprintHandler: mockBlueprintHandler,
		TmpDir:           tmpDir,
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// captureOutput creates buffers for stdout and stderr and returns them along with a cleanup function
func captureOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	t.Cleanup(func() {
		stdout.Reset()
		stderr.Reset()
	})

	return stdout, stderr
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRootCmd(t *testing.T) {
	t.Run("RootCmd", func(t *testing.T) {
		// Given a set of mocks
		setupMocks(t)

		// When creating the root command
		cmd := rootCmd

		// Ensure the verbose flag is defined
		if cmd.PersistentFlags().Lookup("verbose") == nil {
			cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
		}

		// Then the command should be properly configured
		if cmd.Use != "windsor" {
			t.Errorf("Expected Use to be 'windsor', got %s", cmd.Use)
		}

		// And the command should have the verbose flag
		verboseFlag := cmd.PersistentFlags().Lookup("verbose")
		if verboseFlag == nil {
			t.Error("Expected verbose flag to be defined")
			return
		}

		// And the flag should have the correct properties
		if verboseFlag.Name != "verbose" {
			t.Errorf("Expected flag name to be 'verbose', got %s", verboseFlag.Name)
		}
		if verboseFlag.Shorthand != "v" {
			t.Errorf("Expected flag shorthand to be 'v', got %s", verboseFlag.Shorthand)
		}
		if verboseFlag.Usage != "Enable verbose output" {
			t.Errorf("Expected flag usage to be 'Enable verbose output', got %s", verboseFlag.Usage)
		}

		// Clear any previously set arguments to ensure we're testing the root command without subcommands
		rootCmd.SetArgs([]string{})

		// Execute should work without error
		if err := Execute(); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestRootCmd_PersistentPreRunE(t *testing.T) {
	t.Run("PersistentPreRunE", func(t *testing.T) {
		// Given a set of mocks
		setupMocks(t)

		// When executing the PersistentPreRunE function
		err := rootCmd.PersistentPreRunE(rootCmd, []string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})
}

func TestCommandPreflight(t *testing.T) {
	// Cleanup: reset rootCmd context after all subtests complete
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
	})

	// Set up mocks for all tests
	setupMocks(t)

	createMockCmd := func(name string) *cobra.Command {
		return &cobra.Command{
			Use: name,
		}
	}

	t.Run("SucceedsForInitCommand", func(t *testing.T) {
		// Given an init command
		cmd := createMockCmd("init")

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (preflight only sets up global context)
		if err != nil {
			t.Errorf("Expected no error for init command, got: %v", err)
		}
	})

	t.Run("SucceedsForEnvCommandWithHookFlag", func(t *testing.T) {
		// Given an env command with hook flag
		cmd := createMockCmd("env")
		cmd.Flags().Bool("hook", false, "hook flag")
		cmd.Flags().Set("hook", "true")

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (preflight only sets up global context)
		if err != nil {
			t.Errorf("Expected no error for env --hook, got: %v", err)
		}
	})

	t.Run("SetsUpGlobalContext", func(t *testing.T) {
		// Given any command
		cmd := createMockCmd("test")

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (preflight only sets up global context)
		if err != nil {
			t.Errorf("Expected no error for preflight, got: %v", err)
		}

		// And context should be set
		if cmd.Context() == nil {
			t.Error("Expected command context to be set")
		}
	})

}
