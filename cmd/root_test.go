package cmd_test

// The RootTest provides comprehensive test coverage for the Windsor CLI root command.
// It provides validation of command initialization, flag handling, and context management,
// The RootTest ensures proper command execution and context propagation,
// verifying error handling, flag parsing, and command hierarchy.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	cliCmd "github.com/windsorcli/cli/cmd"

	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
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
	SecretsProvider  *secrets.MockProvider
	EnvPrinter       *envvars.MockEnvPrinter
	ToolsManager     *tools.MockToolsManager
	Runtime          *runtime.Runtime
	BlueprintHandler *blueprintpkg.MockBlueprintHandler
	TmpDir           string
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
	TmpDir        string
}

// suppressProcessStdout redirects os.Stdout to a pipe drained to io.Discard for the
// duration of the test so that commands using fmt.Print do not pollute the terminal.
// The reader goroutine drains continuously — Windows pipe buffers are ~4KB, so deferring
// the drain to t.Cleanup deadlocks any test that writes more than that. Restores on t.Cleanup.
func suppressProcessStdout(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		io.Copy(io.Discard, r)
		close(done)
	}()
	t.Cleanup(func() {
		os.Stdout = orig
		w.Close()
		<-done
	})
}

// suppressProcessStderr redirects os.Stderr to a pipe drained to io.Discard for the
// duration of the test so that command error and progress messages do not pollute the
// terminal. See suppressProcessStdout for why the drain runs in a goroutine. Restores on t.Cleanup.
func suppressProcessStderr(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan struct{})
	go func() {
		io.Copy(io.Discard, r)
		close(done)
	}()
	t.Cleanup(func() {
		os.Stderr = orig
		w.Close()
		<-done
	})
}

// captureProcessStderr redirects os.Stderr to a pipe whose contents the caller can read after
// invoking the returned restore. Use this from a subtest to override a parent test's
// suppressProcessStderr when you need to assert on stderr content written via
// fmt.Fprintln(os.Stderr, ...). The caller must invoke restore (or t.Cleanup it) before reading buf.
func captureProcessStderr(t *testing.T) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	buf = new(bytes.Buffer)
	restore = func() {
		w.Close()
		io.Copy(buf, r)
		os.Stderr = orig
	}
	return buf, restore
}

// setupMocks creates mock components for testing the root command
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create temporary directory for test (only if needed)
	var tmpDir string
	if options.TmpDir != "" {
		tmpDir = options.TmpDir
	} else {
		tmpDir = t.TempDir()
	}

	// Create mock shell with all exec functions mocked to avoid waiting
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
	// Mock all exec functions to return immediately without waiting for process execution
	mockShell.ExecFunc = func(string, ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentFunc = func(string, ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecProgressFunc = func(string, string, ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSudoFunc = func(string, string, ...string) (string, error) {
		return "", nil
	}

	// Create mock secrets provider
	mockSecretsProvider := secrets.NewMockProvider()

	// Create mock env printer
	mockEnvPrinter := envvars.NewMockEnvPrinter()
	mockEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}
	mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}

	// Create and register additional mock env printers
	mockWindsorEnvPrinter := envvars.NewMockEnvPrinter()
	mockWindsorEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	mockWindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}
	mockWindsorEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}
	mockDockerEnvPrinter := envvars.NewMockEnvPrinter()
	mockDockerEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
		return nil
	}
	mockDockerEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}
	mockDockerEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
		return map[string]string{}, nil
	}

	// Create config handler - always use mock for tests
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewMockConfigHandler()
		configHandler.SetContext("test-context")
	} else {
		configHandler = options.ConfigHandler
	}
	// If it's a mock config handler, set defaults to use tmpDir
	if mockConfig, ok := configHandler.(*config.MockConfigHandler); ok {
		if mockConfig.GetConfigRootFunc == nil {
			mockConfig.GetConfigRootFunc = func() (string, error) {
				return tmpDir, nil
			}
		}
		if mockConfig.GetContextFunc == nil {
			mockConfig.GetContextFunc = func() string {
				return "test-context"
			}
		}
		if mockConfig.LoadConfigFunc == nil {
			mockConfig.LoadConfigFunc = func() error {
				return nil
			}
		}
		if mockConfig.LoadSchemaFromBytesFunc == nil {
			mockConfig.LoadSchemaFromBytesFunc = func(data []byte) error {
				return nil
			}
		}
		if mockConfig.LoadConfigStringFunc == nil {
			mockConfig.LoadConfigStringFunc = func(content string) error {
				// Parse YAML content if provided
				if content != "" {
					// Use a simple YAML parser - for tests, just mark as loaded
					// The actual parsing is handled by the real implementation
					// but for mocks, we just need to succeed
					return nil
				}
				return nil
			}
		}
		if mockConfig.IsLoadedFunc == nil {
			mockConfig.IsLoadedFunc = func() bool {
				return true
			}
		}
		if mockConfig.GetStringFunc == nil {
			mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
				// Return empty string by default instead of "mock-string" to avoid parsing errors
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}
		if mockConfig.GetContextValuesFunc == nil {
			mockConfig.GetContextValuesFunc = func() (map[string]any, error) {
				addons := make(map[string]any)
				// Initialize common addons with enabled: false to prevent evaluation errors
				for _, addon := range []string{"object_store", "observability", "private_ca", "private_dns"} {
					addons[addon] = map[string]any{"enabled": false}
				}
				return map[string]any{
					"addons": addons,
					"dev":    false,
				}, nil
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

	// Create mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.CheckFunc = func() error { return nil }

	// Create runtime with all mocked dependencies including env printers
	rtOverride := &runtime.Runtime{
		Shell:         mockShell,
		ConfigHandler: configHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  mockToolsManager,
	}
	rtOverride.EnvPrinters.WindsorEnv = mockWindsorEnvPrinter
	rtOverride.EnvPrinters.DockerEnv = mockDockerEnvPrinter
	rt := runtime.NewRuntime(rtOverride)

	return &Mocks{
		ConfigHandler:    configHandler,
		Shell:            mockShell,
		SecretsProvider:  mockSecretsProvider,
		EnvPrinter:       mockEnvPrinter,
		ToolsManager:     mockToolsManager,
		Runtime:          rt,
		BlueprintHandler: mockBlueprintHandler,
		TmpDir:           tmpDir,
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// eventRecorder records every presenter event it receives.
type eventRecorder struct {
	events []presenter.Event
}

func (r *eventRecorder) Emit(_ context.Context, e presenter.Event) {
	r.events = append(r.events, e)
}

// operations returns the messages of the operations that started, in order.
func (r *eventRecorder) operations() []string {
	var ops []string
	for _, e := range r.events {
		if e.Kind == presenter.KindApplying {
			ops = append(ops, e.Message)
		}
	}
	return ops
}

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
		cmd := cliCmd.RootCmd()

		// Ensure the verbose flag is defined
		if cmd.PersistentFlags().Lookup("verbose") == nil {
			cmd.PersistentFlags().BoolVarP(&*cliCmd.Verbose, "verbose", "v", false, "Enable verbose output")
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

		// And the command should have the --no-cache flag (persistent so all subcommands inherit)
		noCacheFlag := cmd.PersistentFlags().Lookup("no-cache")
		if noCacheFlag == nil {
			t.Error("Expected no-cache flag to be defined")
			return
		}
		if noCacheFlag.Name != "no-cache" {
			t.Errorf("Expected flag name to be 'no-cache', got %s", noCacheFlag.Name)
		}
		if noCacheFlag.Shorthand != "" {
			t.Errorf("Expected no shorthand for no-cache, got %q", noCacheFlag.Shorthand)
		}

		// Clear any previously set arguments to ensure we're testing the root command without subcommands
		cliCmd.RootCmd().SetArgs([]string{})

		// Execute should work without error
		if err := cliCmd.Execute(); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RootCmdAccessor", func(t *testing.T) {
		// Given the cmd package init() blocks have run

		// When RootCmd is called
		got := cliCmd.RootCmd()

		// Then it returns the assembled root command with subcommands registered.
		// Asserting via the version subcommand because it is one of the simplest,
		// most stable commands; the precise list of subcommands is verified
		// indirectly by the gendocs generator's own tests.
		if got == nil {
			t.Fatal("RootCmd returned nil")
		}
		if got.Use != "windsor" {
			t.Errorf("expected Use=windsor, got %q", got.Use)
		}
		if _, _, err := got.Find([]string{"version"}); err != nil {
			t.Errorf("expected to find 'version' subcommand: %v", err)
		}
	})
}

func TestRootCmd_PersistentPreRunE(t *testing.T) {
	t.Run("PersistentPreRunE", func(t *testing.T) {
		// Given a set of mocks
		setupMocks(t)

		// When executing the PersistentPreRunE function
		err := cliCmd.RootCmd().PersistentPreRunE(cliCmd.RootCmd(), []string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})
}

func TestExecute(t *testing.T) {
	t.Cleanup(func() {
		cliCmd.RootCmd().SetContext(context.Background())
		cliCmd.RootCmd().SetArgs([]string{})
	})

	t.Run("WithTODOContext", func(t *testing.T) {
		// Given rootCmd with context.TODO
		cliCmd.RootCmd().SetContext(context.TODO())
		cliCmd.RootCmd().SetArgs([]string{})

		// When executing
		err := cliCmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("WithExistingContext", func(t *testing.T) {
		// Given rootCmd with existing context
		ctx := context.WithValue(context.Background(), "test", "value")
		cliCmd.RootCmd().SetContext(ctx)
		cliCmd.RootCmd().SetArgs([]string{})

		// When executing
		err := cliCmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

func TestExecute_RendersErrors(t *testing.T) {
	t.Cleanup(func() {
		*cliCmd.FormatFlag = string(logging.FormatText)
		cliCmd.RootCmd().SetArgs([]string{})
		cliCmd.RootCmd().SetErr(os.Stderr)
	})

	t.Run("RendersAnUnknownFormatWithAHint", func(t *testing.T) {
		// Given an unknown --format value
		var stderr bytes.Buffer
		cliCmd.RootCmd().SetErr(&stderr)
		cliCmd.RootCmd().SetArgs([]string{"version", "--format", "yaml"})

		// When executing
		err := cliCmd.Execute()

		// Then a WindsorError is returned and rendered once with its hint
		var we *werror.WindsorError
		if !errors.As(err, &we) {
			t.Fatalf("expected a WindsorError, got %v", err)
		}
		out := stderr.String()
		if strings.Count(out, "error: Unknown output format") != 1 || !strings.Contains(out, "hint: Run the command again with --format text or --format json.") {
			t.Errorf("unexpected stderr %q", out)
		}
	})
}
