package cmd

// The RootTest provides comprehensive test coverage for the Windsor CLI root command.
// It provides validation of command initialization, flag handling, and context management,
// The RootTest ensures proper command execution and context propagation,
// verifying error handling, flag parsing, and command hierarchy.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	blueprintpkg "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
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
		rootCmd.SetArgs([]string{})

		// Execute should work without error
		if err := Execute(); err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RootCmdAccessor", func(t *testing.T) {
		// Given the cmd package init() blocks have run

		// When RootCmd is called
		got := RootCmd()

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
		err := rootCmd.PersistentPreRunE(rootCmd, []string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})
}

func TestExecute(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
		rootCmd.SetArgs([]string{})
	})

	t.Run("WithTODOContext", func(t *testing.T) {
		// Given rootCmd with context.TODO
		rootCmd.SetContext(context.TODO())
		rootCmd.SetArgs([]string{})

		// When executing
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("WithExistingContext", func(t *testing.T) {
		// Given rootCmd with existing context
		ctx := context.WithValue(context.Background(), "test", "value")
		rootCmd.SetContext(ctx)
		rootCmd.SetArgs([]string{})

		// When executing
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

func TestCommandPreflight(t *testing.T) {
	// Cleanup: reset rootCmd context and globals after all subtests complete.
	// noCache is reset alongside verbose because both are package-level flag
	// vars that leak across subtests if not cleared.
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
		verbose = false
		noCache = false
	})

	// Set up mocks for all tests
	setupMocks(t)

	t.Run("SucceedsForInitCommand", func(t *testing.T) {
		// Given an init command attached to root
		cmd := &cobra.Command{Use: "init"}
		rootCmd.AddCommand(cmd)

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (preflight only sets up global context)
		if err != nil {
			t.Errorf("Expected no error for init command, got: %v", err)
		}
	})

	t.Run("SucceedsForEnvCommandWithHookFlag", func(t *testing.T) {
		// Given an env command with hook flag attached to root
		cmd := &cobra.Command{Use: "env"}
		cmd.Flags().Bool("hook", false, "hook flag")
		cmd.Flags().Set("hook", "true")
		rootCmd.AddCommand(cmd)

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (preflight only sets up global context)
		if err != nil {
			t.Errorf("Expected no error for env --hook, got: %v", err)
		}
	})

	t.Run("SetsUpGlobalContext", func(t *testing.T) {
		// Given any command attached to root
		cmd := &cobra.Command{Use: "test"}
		rootCmd.AddCommand(cmd)

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

	t.Run("EnablesDebugLoggingUnderVerbose", func(t *testing.T) {
		// Given verbose flag is set
		verbose = true
		cmd := &cobra.Command{Use: "test"}
		rootCmd.AddCommand(cmd)

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error for preflight, got: %v", err)
		}

		// And the context logger should accept debug records
		if cmd.Context() == nil {
			t.Fatal("Expected command context to be set")
		}
		if !logging.FromContext(cmd.Context()).Enabled(cmd.Context(), slog.LevelDebug) {
			t.Error("Expected the context logger to be enabled at debug level")
		}
	})

	t.Run("HandlesSetupGlobalContextWithNilRootContext", func(t *testing.T) {
		// Given a command with root that has a nil context, ensure we pass a non-nil Context
		rootCmd.SetContext(context.TODO())
		cmd := &cobra.Command{Use: "test"}
		rootCmd.AddCommand(cmd)

		// When running preflight
		err := commandPreflight(cmd, []string{})

		// Then no error should occur (setupGlobalContext doesn't return errors currently)
		if err != nil {
			t.Errorf("Expected no error for preflight, got: %v", err)
		}
	})

	t.Run("SetsNoCacheEnvWhenFlagTrue", func(t *testing.T) {
		// Given the --no-cache flag is set, and NO_CACHE is unset in the environment,
		// preflight must propagate the flag to NO_CACHE=true so ArtifactBuilder.Pull
		// (which only reads the env var, not the flag) bypasses the disk cache.
		original, hadOriginal := os.LookupEnv("NO_CACHE")
		os.Unsetenv("NO_CACHE")
		t.Cleanup(func() {
			noCache = false
			if hadOriginal {
				os.Setenv("NO_CACHE", original)
			} else {
				os.Unsetenv("NO_CACHE")
			}
		})

		noCache = true
		cmd := &cobra.Command{Use: "test"}
		rootCmd.AddCommand(cmd)
		t.Cleanup(func() { rootCmd.RemoveCommand(cmd) })

		// When running preflight
		if err := commandPreflight(cmd, []string{}); err != nil {
			t.Fatalf("Expected no error for preflight, got: %v", err)
		}

		// Then NO_CACHE should be set to "true" so the artifact layer bypasses the cache
		if got := os.Getenv("NO_CACHE"); got != "true" {
			t.Errorf("Expected NO_CACHE=true after --no-cache preflight, got %q", got)
		}
	})

	t.Run("DoesNotTouchNoCacheEnvWhenFlagFalse", func(t *testing.T) {
		// Given --no-cache is NOT set but the operator already exported NO_CACHE in their
		// shell, preflight must not clobber it. The flag is purely additive: explicit
		// flag wins, but a quiet preflight leaves the operator's environment alone.
		original, hadOriginal := os.LookupEnv("NO_CACHE")
		os.Setenv("NO_CACHE", "preexisting")
		t.Cleanup(func() {
			noCache = false
			if hadOriginal {
				os.Setenv("NO_CACHE", original)
			} else {
				os.Unsetenv("NO_CACHE")
			}
		})

		noCache = false
		cmd := &cobra.Command{Use: "test"}
		rootCmd.AddCommand(cmd)
		t.Cleanup(func() { rootCmd.RemoveCommand(cmd) })

		// When running preflight
		if err := commandPreflight(cmd, []string{}); err != nil {
			t.Fatalf("Expected no error for preflight, got: %v", err)
		}

		// Then NO_CACHE should be untouched — preflight has no business overwriting an
		// operator-supplied value when the flag is off
		if got := os.Getenv("NO_CACHE"); got != "preexisting" {
			t.Errorf("Expected NO_CACHE preserved at %q, got %q", "preexisting", got)
		}
	})
}

func TestRequireCloudAuth(t *testing.T) {
	t.Run("PrintsHintToStderrAndSilencesCobraOnFailure", func(t *testing.T) {
		// Given CheckAuth returns the per-platform hint as the error (the awsAuthHint shape:
		// "AWS SSO session for X has likely expired. Run: aws sso login --profile X"),
		// requireCloudAuth must (a) print that hint to the command's stderr verbatim so the
		// operator sees the actionable next step, (b) set SilenceErrors on the command tree
		// so cobra does not double-print "Error: AWS SSO session..." on top — credential
		// failures are flow-guidance moments, not exceptions, and the "Error:" framing reads
		// as panic, and (c) still return the error so the parent command exits non-zero
		// (preserving CI / scripted exit-code semantics).
		mocks := setupMocks(t)
		hint := "AWS SSO session for \"test-context\" has likely expired. Run:\n  aws sso login --profile test-context"
		mocks.ToolsManager.CheckAuthFunc = func() error { return fmt.Errorf("%s", hint) }
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		root := &cobra.Command{Use: "windsor"}
		leaf := &cobra.Command{Use: "destroy"}
		root.AddCommand(leaf)

		var stderr strings.Builder
		leaf.SetErr(&stderr)

		err := requireCloudAuth(leaf, proj)
		if err == nil {
			t.Fatal("Expected requireCloudAuth to return CheckAuth's error, got nil")
		}
		if !strings.Contains(stderr.String(), "aws sso login --profile test-context") {
			t.Errorf("Expected hint printed to stderr, got: %q", stderr.String())
		}
		// Both leaf and root must be silenced — cobra walks the chain when deciding whether
		// to print "Error: ...", and a missed parent re-introduces the prefix.
		if !leaf.SilenceErrors {
			t.Error("Expected leaf cmd's SilenceErrors to be set to suppress cobra's Error: prefix")
		}
		if !root.SilenceErrors {
			t.Error("Expected root cmd's SilenceErrors to be set so cobra's chain check does not re-add Error: prefix")
		}
	})

	t.Run("SuccessIsSilent", func(t *testing.T) {
		// Given CheckAuth passes, requireCloudAuth must not touch stderr or fiddle with
		// SilenceErrors — those side effects belong only on the failure path.
		mocks := setupMocks(t)
		// Default mock CheckAuth returns nil.
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		leaf := &cobra.Command{Use: "destroy"}
		var stderr strings.Builder
		leaf.SetErr(&stderr)

		if err := requireCloudAuth(leaf, proj); err != nil {
			t.Fatalf("Expected no error on success, got %v", err)
		}
		if stderr.String() != "" {
			t.Errorf("Expected no stderr output on success, got: %q", stderr.String())
		}
		if leaf.SilenceErrors {
			t.Error("Expected SilenceErrors to remain unset on success — only the failure path should touch it")
		}
	})
}

func TestExecute_RendersErrors(t *testing.T) {
	t.Cleanup(func() {
		formatFlag = string(logging.FormatText)
		rootCmd.SetArgs([]string{})
		rootCmd.SetErr(os.Stderr)
	})

	t.Run("RendersAnUnknownFormatAsACodedError", func(t *testing.T) {
		// Given an unknown --format value
		var stderr bytes.Buffer
		rootCmd.SetErr(&stderr)
		rootCmd.SetArgs([]string{"version", "--format", "yaml"})

		// When executing
		err := Execute()

		// Then a CLI-001 error is returned and rendered once with its remediation
		if !werror.IsCode(err, "CLI-001") {
			t.Fatalf("expected CLI-001, got %v", err)
		}
		out := stderr.String()
		if strings.Count(out, "Error [CLI-001]") != 1 || !strings.Contains(out, "--format text or --format json") {
			t.Errorf("unexpected stderr %q", out)
		}
	})
}

func TestRenderError(t *testing.T) {
	t.Cleanup(func() {
		formatFlag = string(logging.FormatText)
		verbose = false
	})

	t.Run("PrintsAnUntypedErrorWithThePlainPrefix", func(t *testing.T) {
		// Given text format and an untyped error
		formatFlag, verbose = string(logging.FormatText), false
		var buf bytes.Buffer

		// When the error is rendered
		renderError(&buf, fmt.Errorf("context %q not found", "x"))

		// Then the output matches the plain error line
		if buf.String() != "Error: context \"x\" not found\n" {
			t.Errorf("unexpected output %q", buf.String())
		}
	})

	t.Run("PrintsACodedErrorWithItsRemediation", func(t *testing.T) {
		// Given text format and a coded error under a frame
		formatFlag, verbose = string(logging.FormatText), false
		var buf bytes.Buffer
		err := werror.Wrap(werror.New("CONFIG-002", "Context value is malformed.", "Run windsor set to fix it.", nil), "loading")

		// When the error is rendered
		renderError(&buf, err)

		// Then the code, message, and remediation print without a trace
		want := "Error [CONFIG-002]: Context value is malformed.\n\nRun windsor set to fix it.\n"
		if buf.String() != want {
			t.Errorf("expected %q, got %q", want, buf.String())
		}
	})

	t.Run("AddsTheTraceUnderVerbose", func(t *testing.T) {
		// Given verbose text output and a coded error with a cause under a frame
		formatFlag, verbose = string(logging.FormatText), true
		var buf bytes.Buffer
		err := werror.Wrap(werror.New("TERRAFORM-002", "Apply failed.", "", fmt.Errorf("exit status 1")), "applying cluster")

		// When the error is rendered
		renderError(&buf, err)

		// Then each breadcrumb prints under a trace heading
		if !strings.Contains(buf.String(), "\nTrace:\n  applying cluster\n  Apply failed.\n  exit status 1\n") {
			t.Errorf("expected trace, got %q", buf.String())
		}
	})

	t.Run("WritesOneJSONLineInJSONFormat", func(t *testing.T) {
		// Given JSON format and a coded error
		formatFlag, verbose = string(logging.FormatJSON), false
		var buf bytes.Buffer

		// When the error is rendered
		renderError(&buf, werror.New("CLI-001", "Unknown output format.", "Use text or json.", nil))

		// Then one failed event line holds the error fields
		var line struct {
			Kind  string            `json:"kind"`
			Error map[string]string `json:"error"`
		}
		if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
			t.Fatalf("expected one JSON line, got %q: %v", buf.String(), err)
		}
		if line.Kind != "failed" || line.Error["code"] != "CLI-001" || line.Error["remediation"] != "Use text or json." {
			t.Errorf("unexpected line %+v", line)
		}
	})
}

func TestWithOutput(t *testing.T) {
	t.Cleanup(func() { verbose = false })

	t.Run("InjectsAPresenterAndALoggerThatRoutesIntoIt", func(t *testing.T) {
		// Given JSON output to a buffer
		verbose = false
		var buf bytes.Buffer

		// When output is wired and a warning is logged through the context logger
		ctx := withOutput(context.Background(), &buf, logging.FormatJSON)
		logging.FromContext(ctx).Warn("slow apply")

		// Then the presenter is in the context and the log reached it as a JSON log event
		if _, ok := ctx.Value(presenterKey).(presenter.Presenter); !ok {
			t.Error("expected a presenter in the context")
		}
		if !strings.Contains(buf.String(), `"kind":"log"`) || !strings.Contains(buf.String(), "slow apply") {
			t.Errorf("expected a JSON log event, got %q", buf.String())
		}
	})

	t.Run("ShowsDebugLogsOnlyUnderVerbose", func(t *testing.T) {
		// Given text output with and without --verbose
		var quiet, loud bytes.Buffer
		verbose = false
		quietCtx := withOutput(context.Background(), &quiet, logging.FormatText)
		verbose = true
		loudCtx := withOutput(context.Background(), &loud, logging.FormatText)

		// When a debug record is logged through each logger
		logging.FromContext(quietCtx).Debug("resolving values")
		logging.FromContext(loudCtx).Debug("resolving values")

		// Then only the verbose logger writes it
		if quiet.Len() != 0 || !strings.Contains(loud.String(), "resolving values") {
			t.Errorf("unexpected output quiet=%q loud=%q", quiet.String(), loud.String())
		}
	})
}
