package cmd

// The RootTest provides comprehensive test coverage for the Windsor CLI root command.
// It provides validation of command initialization, flag handling, and context management,
// The RootTest ensures proper command execution and context propagation,
// verifying error handling, flag parsing, and command hierarchy.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	blueprintpkg "github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector         di.Injector
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	SecretsProvider  *secrets.MockSecretsProvider
	EnvPrinter       *env.MockEnvPrinter
	Shims            *Shims
	BlueprintHandler *blueprintpkg.MockBlueprintHandler
}

type SetupOptions struct {
	Injector      di.Injector
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
		Getwd:       func() (string, error) { return t.TempDir(), nil },
		Command:     func(string, ...string) *exec.Cmd { return exec.Command("echo") },
		Setenv:      func(string, string) error { return nil },
	}

	// Override with provided shims if any
	if options.Shims != nil {
		testShims = options.Shims
	}

	// Set global shims
	shims = testShims

	// Create injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	// Create and register mock shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return t.TempDir(), nil
	}
	mockShell.CheckTrustedDirectoryFunc = func() error {
		return nil
	}
	mockShell.CheckResetFlagsFunc = func() (bool, error) {
		return false, nil
	}
	mockShell.ResetFunc = func(...bool) {}
	injector.Register("shell", mockShell)

	// Create and register mock secrets provider
	mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
	mockSecretsProvider.LoadSecretsFunc = func() error {
		return nil
	}
	injector.Register("secretsProvider", mockSecretsProvider)

	// Create and register mock env printer
	mockEnvPrinter := env.NewMockEnvPrinter()
	mockEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	injector.Register("envPrinter", mockEnvPrinter)

	// Create and register additional mock env printers
	mockWindsorEnvPrinter := env.NewMockEnvPrinter()
	mockWindsorEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockWindsorEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	injector.Register("windsorEnvPrinter", mockWindsorEnvPrinter)

	mockDockerEnvPrinter := env.NewMockEnvPrinter()
	mockDockerEnvPrinter.PrintFunc = func() error {
		return nil
	}
	mockDockerEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	injector.Register("dockerEnvPrinter", mockDockerEnvPrinter)

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewYamlConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}

	// Register config handler
	injector.Register("configHandler", configHandler)

	// Initialize config handler
	if err := configHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
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
	mockBlueprintHandler := blueprintpkg.NewMockBlueprintHandler(injector)
	mockBlueprintHandler.InstallFunc = func() error {
		return nil
	}
	injector.Register("blueprintHandler", mockBlueprintHandler)

	return &Mocks{
		Injector:         injector,
		ConfigHandler:    configHandler,
		Shell:            mockShell,
		SecretsProvider:  mockSecretsProvider,
		EnvPrinter:       mockEnvPrinter,
		Shims:            testShims,
		BlueprintHandler: mockBlueprintHandler,
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

func TestCheckTrust(t *testing.T) {
	createMockCmd := func(name string) *cobra.Command {
		return &cobra.Command{
			Use: name,
		}
	}

	t.Run("SkipsTrustCheckForInitCommand", func(t *testing.T) {
		// Given an init command
		cmd := createMockCmd("init")

		// When checking trust
		err := checkTrust(cmd, []string{})

		// Then no error should occur (trust check is skipped)
		if err != nil {
			t.Errorf("Expected no error for init command, got: %v", err)
		}
	})

	t.Run("SkipsTrustCheckForEnvCommandWithHookFlag", func(t *testing.T) {
		// Given an env command with hook flag
		cmd := createMockCmd("env")
		cmd.Flags().Bool("hook", false, "hook flag")
		cmd.Flags().Set("hook", "true")

		// When checking trust
		err := checkTrust(cmd, []string{})

		// Then no error should occur (trust check is skipped for env --hook)
		if err != nil {
			t.Errorf("Expected no error for env --hook, got: %v", err)
		}
	})

	t.Run("ChecksTrustForEnvCommandWithoutHookFlag", func(t *testing.T) {
		// Given an env command without hook flag in an untrusted directory
		cmd := createMockCmd("env")
		cmd.Flags().Bool("hook", false, "hook flag")

		// Set up a temporary directory that's not trusted
		tmpDir := t.TempDir()
		originalDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}
		defer os.Chdir(originalDir)

		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		// When checking trust
		err = checkTrust(cmd, []string{})

		// Then an error should occur about untrusted directory
		if err == nil {
			t.Error("Expected error for untrusted directory, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trust error message, got: %v", err)
		}
	})

	t.Run("PassesTrustCheckForTrustedDirectory", func(t *testing.T) {
		// Given a command in a trusted directory
		cmd := createMockCmd("down")

		// Set up a temporary directory structure with trusted file
		tmpDir := t.TempDir()
		testDir := filepath.Join(tmpDir, "project")
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		// Create trusted file
		trustedDir := filepath.Join(tmpDir, ".config", "windsor")
		if err := os.MkdirAll(trustedDir, 0755); err != nil {
			t.Fatalf("Failed to create trusted directory: %v", err)
		}

		trustedFile := filepath.Join(trustedDir, ".trusted")
		realTestDir, _ := filepath.EvalSymlinks(testDir)
		trustedContent := realTestDir + "\n"
		if err := os.WriteFile(trustedFile, []byte(trustedContent), 0644); err != nil {
			t.Fatalf("Failed to create trusted file: %v", err)
		}

		// Change to test directory
		originalDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}
		defer os.Chdir(originalDir)

		if err := os.Chdir(testDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		// Mock home directory
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		// When checking trust
		err = checkTrust(cmd, []string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error for trusted directory, got: %v", err)
		}
	})

}
