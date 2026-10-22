// The hook command prints the shell init code for a shell.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

var hookCmd = &cobra.Command{
	Use:   "hook <shell>",
	Short: "Print shell init code for the given shell.",
	Long: `Print init code that wires 'windsor env' into your shell. The hook re-runs 'windsor env --hook' whenever your prompt fires, exporting Windsor's per-context environment variables automatically when you cd into a project.

Supported shells: zsh, bash, powershell.

Add the output to your shell's rc file (or evaluate it directly during shell startup) so the hook installs on every new session.`,
	Example: `# zsh / bash
eval "$(windsor hook zsh)"
eval "$(windsor hook bash)"

# powershell
windsor hook powershell | Out-String | Invoke-Expression`,
	Annotations: map[string]string{
		"docs.seealso": "[`env`](env.md), [`exec`](exec.md)",
		"docs.source":  "cmd/hook.go",
	},
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if err := rt.Shell.InstallHook(args[0]); err != nil {
			return werror.Wrap(err, "error installing hook")
		}

		return nil
	},
}

// init registers the hook command with the root command.
func init() {
	rootCmd.AddCommand(hookCmd)
}
