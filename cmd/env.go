// The env command prints the shell commands that export the project environment variables.

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print the project's environment variables.",
	Long: `Print the project's environment variables as KEY=value lines.
Secrets show as ******** unless you pass --decrypt.

The output is for display. To set the variables in your shell, run 'windsor hook'
or use 'eval' with --hook. To run one command with them, use 'windsor exec'.`,
	Example: `# Show the variables
windsor env

# Show the variables with secrets decrypted (1Password / SOPS)
windsor env --decrypt

# Set the variables in the current bash or zsh shell
eval "$(windsor env --decrypt --hook)"

# tcsh
eval "` + "`" + `windsor env --shell tcsh` + "`" + `"`,
	Annotations: map[string]string{
		migrationNoticeAnnotation: "true",
		"docs.seealso":            "[`hook`](hook.md), [`exec`](exec.md)",
		"docs.source":             "cmd/env.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		hook, _ := cmd.Flags().GetBool("hook")
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		shellName, _ := cmd.Flags().GetString("shell")
		if shellName != "" && shellName != "tcsh" {
			return werror.New(fmt.Sprintf("Windsor cannot print env for the shell %q.", shellName),
				"Use --shell tcsh, or omit --shell for POSIX output.", nil)
		}

		if !hook && os.Getenv("NO_CACHE") == "" {
			if err := os.Setenv("NO_CACHE", "true"); err != nil {
				return werror.Wrap(err, "failed to set NO_CACHE environment variable")
			}
		}
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)
		if hook && rt.Global {
			printManagedRemovals(cmd, rt, shellName == "tcsh")
			return nil
		}
		if hook {
			stderrNullFile, stderrNullErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0600)
			if stderrNullErr == nil {
				originalStderr := os.Stderr
				os.Stderr = stderrNullFile
				defer func() {
					os.Stderr = originalStderr
					_ = stderrNullFile.Close()
				}()
			}
		}
		if err := rt.Shell.CheckTrustedDirectory(); err != nil {
			if hook {
				printManagedRemovals(cmd, rt, shellName == "tcsh")
				return nil
			}
			return werror.New("not in a trusted directory", trustDirectoryHint, err)
		}

		staleEnvs, staleAliases := rt.ManagedNames()
		if err := rt.HandleSessionReset(shellName == "tcsh"); err != nil {
			if hook {
				return nil
			}
			return err
		}

		if err := rt.ConfigHandler.LoadConfig(); err != nil {
			if hook {
				return nil
			}
			return err
		}

		if err := rt.InitializeComponents(); err != nil {
			if hook {
				return nil
			}
			return werror.Wrap(err, "failed to initialize components")
		}

		if err := composer.LoadBlueprintIfTerraformProject(rt, true); err != nil {
			if hook {
				return nil
			}
			return werror.Wrap(err, "failed to load blueprint")
		}

		if err := rt.LoadEnvironment(decrypt); err != nil {
			if hook {
				return nil
			}
			return werror.Wrap(err, "failed to load environment")
		}

		outputFunc := func(output string) {
			if output != "" {
				fmt.Fprint(cmd.OutOrStdout(), output)
			}
		}

		if shellName == "tcsh" {
			var aliases map[string]string
			if hook {
				aliases = rt.GetAliases()
			}
			outputFunc(shell.RenderCsh(shell.WithRemoved(rt.GetEnvVars(), staleEnvs), shell.WithRemoved(aliases, staleAliases)))
		} else if hook {
			if rt.Shell != nil && len(rt.GetEnvVars()) > 0 {
				outputFunc(rt.Shell.RenderEnvVars(rt.GetEnvVars(), true))
			}
			if rt.Shell != nil && len(rt.GetAliases()) > 0 {
				outputFunc(rt.Shell.RenderAliases(rt.GetAliases()))
			}
		} else {
			if rt.Shell != nil && len(rt.GetEnvVars()) > 0 {
				outputFunc(rt.Shell.RenderEnvVars(rt.GetEnvVars(), false))
			}
		}

		return nil
	},
}

// init registers the env command and its flags with the root command.
func init() {
	envCmd.Flags().Bool("decrypt", false, "Decrypt secrets before printing env vars.")
	envCmd.Flags().String("shell", "", "Print setenv commands for the given shell instead of POSIX export commands. Only tcsh is supported.")
	envCmd.Flags().Bool("hook", false, "Print shell commands that export the variables. Suppress warnings and exit 0 on errors so a misconfigured project never breaks the prompt. The shell hook installed by 'windsor hook' invokes 'windsor env --decrypt --hook' automatically.")
	rootCmd.AddCommand(envCmd)
}

// =============================================================================
// Helpers
// =============================================================================

// printManagedRemovals prints unset commands for the env vars and aliases that the last hook run set.
// The hook calls it outside a trusted project, so the shell does not keep the previous project's values.
func printManagedRemovals(cmd *cobra.Command, rt *runtime.Runtime, tcsh bool) {
	envVars, aliases := rt.ManagedRemovals()
	if len(envVars) == 0 && len(aliases) == 0 {
		return
	}
	if tcsh {
		fmt.Fprint(cmd.OutOrStdout(), shell.RenderCsh(envVars, aliases))
		return
	}
	if rt.Shell == nil {
		return
	}
	fmt.Fprint(cmd.OutOrStdout(), rt.Shell.RenderEnvVars(envVars, true))
	if len(aliases) > 0 {
		fmt.Fprint(cmd.OutOrStdout(), rt.Shell.RenderAliases(aliases))
	}
}
