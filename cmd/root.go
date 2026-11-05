// The root command is the windsor command and holds the persistent flags.
// It provides the project setup, the preflight checks and the error output that all commands use.

package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/debug"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// verbose is a flag for verbose output
var verbose bool

// debugFlag turns on internal diagnostic logging via the debug package.
// WINDSOR_DEBUG=true enables it too, for runs that cannot pass a flag.
var debugFlag bool

// noCache is a flag for bypassing the OCI artifact cache. When true, every
// command's preflight sets NO_CACHE=true so ArtifactBuilder.Pull skips the
// disk cache and re-downloads (and atomically overwrites) the cached entry.
var noCache bool

// lockTimeout is a flag for how long a command waits to acquire the stack lock before
// failing. Defaults to 0 (fail immediately on contention, matching terraform's own
// -lock-timeout default) rather than silently blocking; pass a duration to wait instead.
var lockTimeout time.Duration

// contextFlag overrides the windsor context for this invocation. See setupGlobalContext.
var contextFlag string

// formatFlag selects the output format for progress, logs, and errors: text or json.
var formatFlag string

// closeOutput restores the terminal after the command. withOutput sets it.
var closeOutput func()

// Define a custom type for context keys
type contextKey string

const projectOverridesKey = contextKey("projectOverrides")
const composerOverridesKey = contextKey("composerOverrides")
const runtimeOverridesKey = contextKey("runtimeOverrides")
const testRunnerOverridesKey = contextKey("testRunnerOverrides")

// trustDirectoryHint tells the operator how to trust the current directory.
const trustDirectoryHint = "Run windsor init to trust this directory, if it is a Windsor project."

// initProjectHint tells the operator how to create the configuration of a project.
const initProjectHint = "Run windsor init, then run the command again."

// setFormatHint tells the operator the format of a --set entry.
const setFormatHint = "Set --set in the format key=value."

// Execute is the main entry point for the Windsor CLI application.
// It executes the root command with the provided context or a new background context.
// Sets the root command's context before execution so cmd.Root().Context() is correct
// when RunE runs (Cobra does not always propagate context to root on subsequent runs).
// It restores the terminal, then renders a returned error once, unless the failing command
// already reported it.
func Execute() error {
	ctx := rootCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rootCmd.SetContext(ctx)
	executed, err := rootCmd.ExecuteContextC(ctx)
	if closeOutput != nil {
		closeOutput()
		closeOutput = nil
	}
	if err != nil && (executed == nil || !executed.SilenceErrors) {
		renderError(rootCmd.ErrOrStderr(), err)
	}
	return err
}

// RootCmd exposes the assembled cobra command tree for tooling that needs to
// introspect commands without executing them — currently the reference-doc
// generator under internal/gendocs. Importing the cmd package triggers the
// init() blocks that register every subcommand, so this accessor reflects the
// full tree by the time any caller invokes it.
func RootCmd() *cobra.Command {
	return rootCmd
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "windsor",
	Short:             "CLI for cloud-native development workflows",
	Long:              "CLI for cloud-native development workflows.",
	PersistentPreRunE: commandPreflight,
	SilenceErrors:     true,
}

// init registers the persistent flags of the root command.
// All subcommands inherit these flags.
// The --no-cache flag sets NO_CACHE, and ArtifactBuilder.Pull reads that variable.
// The --lock-timeout flag applies to each command that acquires the stack lock.
func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Bypass the OCI artifact cache and force re-download of remote sources")
	rootCmd.PersistentFlags().DurationVar(&lockTimeout, "lock-timeout", 0, "Duration to wait for the stack lock before failing (e.g. 30s, 5m). Defaults to 0 (fail immediately).")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Enable internal debug logging to stderr")
	rootCmd.PersistentFlags().StringVarP(&contextFlag, "context", "c", "", "Override the windsor context for this command")
	rootCmd.PersistentFlags().StringVar(&formatFlag, "format", string(logging.FormatText), "Output format for progress, logs, and errors: text or json")
}

// commandPreflight orchestrates global CLI preflight checks and context initialization for all commands.
// Intended for use as cobra.Command.PersistentPreRunE, it ensures the command context is configured
// prior to command execution. Trust checking is now handled by individual commands through the runtime.
func commandPreflight(cmd *cobra.Command, args []string) error {
	if err := setupGlobalContext(cmd); err != nil {
		return err
	}
	return nil
}

// configureProject creates the project for the command and configures it.
// It reads test overrides from the command context, sets the shell verbosity and checks that the directory is trusted.
// Commands that need more steps after Configure, such as ComposeBlueprint, call this function directly.
func configureProject(cmd *cobra.Command) (*project.Project, error) {
	var opts []*project.Project
	if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
		opts = []*project.Project{overridesVal.(*project.Project)}
	}
	proj := project.NewProject("", opts...)
	proj.Runtime.Shell.SetVerbosity(verbose)
	if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
		return nil, werror.New("not in a trusted directory", trustDirectoryHint, err)
	}
	if err := proj.Configure(nil); err != nil {
		return nil, err
	}
	return proj, nil
}

// prepareProject configures a project through configureProject and runs Initialize with reqs.
// Each caller declares only the tools its codepath uses, so down does not check for terraform.
// Pass tools.AllRequirements() when the codepath depends on the blueprint.
// A command that needs a step between Configure and Initialize calls configureProject directly.
// A teardown or read command uses prepareProjectSkipValidation.
func prepareProject(cmd *cobra.Command, reqs tools.Requirements) (*project.Project, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, err
	}
	proj.SetToolRequirements(reqs)
	if err := proj.Initialize(false); err != nil {
		return nil, err
	}
	return proj, nil
}

// prepareProjectSkipValidation is prepareProject without blueprint structural validation.
// Teardown and read commands (destroy, down, env) use it, so an operator can destroy a deployed blueprint
// that the validator rejects. reqs still sets the tool requirements of each command.
func prepareProjectSkipValidation(cmd *cobra.Command, reqs tools.Requirements) (*project.Project, error) {
	proj, err := configureProject(cmd)
	if err != nil {
		return nil, err
	}
	proj.Composer.BlueprintHandler.SetSkipValidation(true)
	proj.SetToolRequirements(reqs)
	if err := proj.Initialize(false); err != nil {
		return nil, err
	}
	return proj, nil
}

// requireCloudAuth runs the cloud credential preflight for the platform of the current context.
// Every command that runs terraform (apply, plan, destroy, up, bootstrap, upgrade) calls it before init and
// state migration. A command that does not run terraform must not call it. On failure, it prints the
// CheckAuth message to stderr, then the remediation of its WindsorError when it has one. It silences the
// cobra "Error:" prefix on the parent commands and returns the error so the process exits non-zero.
func requireCloudAuth(cmd *cobra.Command, proj *project.Project) error {
	if err := proj.Runtime.ToolsManager.CheckAuth(); err != nil {
		silenceErrorsOnAncestors(cmd)
		if we := werror.Find(err); we != nil && we.Remediation != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s\n%s\n", we.Message, we.Remediation)
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
		}
		return err
	}
	return nil
}

// renderError writes err to w once, in the format that --format selects. Text output prints the
// message after "error:". A WindsorError that werror.Find returns prints its message and, on the
// last line, its remediation after "hint:". A joined error prints its full text, so no cause is hidden. Under --verbose, the breadcrumbs that do not end the message print before
// the hint, with each continuation line indented. JSON output is one failed event line without a trace.
func renderError(w io.Writer, err error) {
	if formatFlag == string(logging.FormatJSON) {
		presenter.NewJSON(w).Emit(context.Background(), presenter.Event{Kind: presenter.KindFailed, Err: err})
		return
	}
	message, remediation := err.Error(), ""
	if we := werror.Find(err); we != nil {
		message, remediation = we.Message, we.Remediation
	}
	fmt.Fprintf(w, "error: %s\n", message)
	if verbose {
		var trace []string
		for _, crumb := range werror.Breadcrumbs(err) {
			if !strings.HasSuffix(message, crumb) {
				trace = append(trace, crumb)
			}
		}
		if len(trace) > 0 {
			fmt.Fprintln(w, "trace:")
			for _, crumb := range trace {
				fmt.Fprintf(w, "  %s\n", strings.ReplaceAll(crumb, "\n", "\n  "))
			}
		}
	}
	if remediation != "" {
		fmt.Fprintf(w, "hint: %s\n", remediation)
	}
}

// silenceErrorsOnAncestors walks up the cobra command tree setting SilenceErrors=true on
// each ancestor. Cobra prints "Error: <msg>" by checking SilenceErrors on the command that
// returned the error AND walking upward; setting it only on the leaf is not enough when
// the error bubbles past intermediate commands (root → destroy → terraform). Walking the
// chain mirrors what cobra does internally for the same flag.
func silenceErrorsOnAncestors(cmd *cobra.Command) {
	for c := cmd; c != nil; c = c.Parent() {
		c.SilenceErrors = true
	}
}

// setupGlobalContext injects the logger and the presenter for --format into the command context.
// --no-cache sets NO_CACHE for ArtifactBuilder.Pull, and a NO_CACHE already in the environment stays
// when the flag is not set. --debug or WINDSOR_DEBUG=true enables debug.Log output for the run.
// --context builds a runtime through NewRuntime and injects it under both override keys.
func setupGlobalContext(cmd *cobra.Command) error {
	ctx := cmd.Root().Context()
	if ctx == nil {
		ctx = context.Background()
	}
	format, err := logging.ParseFormat(formatFlag)
	if err != nil {
		return werror.New(fmt.Sprintf("Unknown output format %q.", formatFlag), "Run the command again with --format text or --format json.", err)
	}
	ctx = withOutput(ctx, cmd.ErrOrStderr(), format)
	if noCache {
		if err := os.Setenv("NO_CACHE", "true"); err != nil {
			return werror.Wrap(err, "failed to set NO_CACHE environment variable")
		}
	}
	if contextFlag != "" {
		sh := shell.NewDefaultShell()
		rtOverride := runtime.NewRuntime(&runtime.Runtime{
			Shell:         sh,
			ConfigHandler: config.NewConfigHandler(sh).WithContext(contextFlag),
			ContextName:   contextFlag,
		})
		if ctx.Value(runtimeOverridesKey) == nil {
			ctx = context.WithValue(ctx, runtimeOverridesKey, rtOverride)
		}
		if ctx.Value(projectOverridesKey) == nil {
			ctx = context.WithValue(ctx, projectOverridesKey, &project.Project{Runtime: rtOverride})
		}
	}
	debug.Init(debugFlag || os.Getenv("WINDSOR_DEBUG") == "true")
	debug.Log("debug logging enabled")
	cmd.SetContext(ctx)
	return nil
}

// withOutput returns ctx with a presenter for format that writes to w, and a logger that routes
// into it. Logs show at debug level under --verbose and at warn level otherwise. The console
// animates only when w is a terminal and --verbose is off, because verbose runs stream
// subprocess output to the same terminal. The presenter becomes the process default, which owns
// the terminal for handoffs such as sudo prompts. An interrupt restores the terminal before the
// process exits, and Execute restores it after the command returns.
func withOutput(ctx context.Context, w io.Writer, format logging.Format) context.Context {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	p := presenter.New(format, w, presenter.ConsoleOptions{Level: level, Interactive: !verbose})
	presenter.SetDefault(p)
	if c, ok := p.(*presenter.Console); ok {
		stopWatch := c.WatchInterrupts(presenter.InterruptsGuarded)
		closeOutput = func() {
			stopWatch()
			c.Close()
		}
	}
	ctx = presenter.NewContext(ctx, p)
	return logging.NewContext(ctx, slog.New(presenter.NewLogHandler(p, level)))
}

// addDeprecatedWaitFlag registers --wait on c as a hidden no-op, since installs always wait.
func addDeprecatedWaitFlag(c *cobra.Command) {
	c.Flags().Bool("wait", true, "Wait for kustomization resources to be ready.")
	_ = c.Flags().MarkDeprecated("wait", "kustomizations are always waited on; drop the flag")
}
