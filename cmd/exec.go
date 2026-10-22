package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/runtime"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec [--] <command> [args...]",
	Short: "Run a command with project env vars injected.",
	Long: `Run a command with the project's environment variables and decrypted secrets injected. Useful for one-off commands that need the full Windsor environment without sourcing it into your shell.

exec is implicitly --decrypt: 1Password and SOPS secrets are dereferenced before the child process starts.

If the command you're running takes flags of its own — long (--foo) or short (-x) — pass '--' first so they aren't parsed as 'windsor' flags. Without it, Cobra intercepts the flag and aborts with 'unknown flag'. The '--' is unnecessary only when the inner command takes no flags or only positional arguments.`,
	Example: `# Inner command has its own flags — separate with --
windsor exec -- terraform plan --var-file=staging.tfvars
windsor exec -- kubectl logs my-pod --tail=50
windsor exec -- helm install my-app ./chart --namespace=apps

# A wrapper script that takes no flags itself
windsor exec ./scripts/deploy.sh`,
	Annotations: map[string]string{
		"docs.seealso": "[`env`](env.md), [`hook`](hook.md)",
		"docs.source":  "cmd/exec.go",
	},
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no command provided")
		}

		verbose, _ := cmd.Flags().GetBool("verbose")

		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		rt.Shell.SetVerbosity(verbose)

		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		if err := rt.HandleSessionReset(); err != nil {
			return err
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			return err
		}

		if err := rt.InitializeComponents(); err != nil {
			return werror.Wrap(err, "failed to initialize components")
		}

		if err := composer.LoadBlueprintIfTerraformProject(rt, true); err != nil {
			return werror.Wrap(err, "failed to load blueprint")
		}

		if err := rt.LoadEnvironment(true); err != nil {
			return werror.Wrap(err, "failed to load environment")
		}

		for key, value := range rt.GetEnvVars() {
			if err := os.Setenv(key, value); err != nil {
				return werror.Wrap(err, "failed to set environment variable %s", key)
			}
		}

		command := args[0]
		var commandArgs []string
		if len(args) > 1 {
			commandArgs = args[1:]
		}

		if _, err := rt.Shell.Exec(command, commandArgs...); err != nil {
			return werror.Wrap(err, "failed to execute command")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
