// The down command stops the local workstation environment.

package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var downDestroy bool

// downCmd stops the local workstation environment and removes the local artifacts.
// Without --destroy, it requests docker, colima and the secrets tools. The secrets tools are necessary, because Initialize can run sops or op.
// With --destroy, it requests all tools and calls destroyAll before it touches the workstation.
// When the destroy fails, it returns before the VM and the state go, so that the operator can run the destroy again.
// It refuses --confirm and --continue without --destroy, because only the destroy step reads them.
// It skips validation, so that a broken setup can still stop.
// It prints the remaining host changes in a deferred call, so the user sees them when the teardown fails.
// The deferred call reads the list when it runs, after Down fills it.
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the local workstation environment.",
	Long: `Delete the workstation VM, stop the container runtimes, and clear the runtime caches (.terraform, .tfstate) and the cluster credentials (.kube, .talos). Hand-edited tfvars and cloud credentials stay. Workstation contexts only.

Without --destroy, down does not wait for the cluster to release resources outside the VM. Pass --destroy when the cluster manages such resources, for example cloud resources. Down then runs the same steps as 'windsor destroy' first, with the same plan and confirmation. If the destroy fails, down stops and keeps the workstation, so you can run it again.

If 'windsor configure network' installed host network or DNS configuration, down prints a follow-up command at the end to remove it.`,
	Example: `# Delete the workstation environment
windsor down

# Destroy the infrastructure, then delete the workstation environment
windsor down --destroy

# Same, scripted
windsor down --destroy --confirm=local`,
	Annotations: map[string]string{
		"docs.seealso": "[`up`](up.md), [`destroy`](destroy.md), [`configure network`](configure-network.md)",
		"docs.source":  "cmd/down.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !downDestroy && (cmd.Flags().Changed("confirm") || cmd.Flags().Changed("continue")) {
			return werror.New("--confirm and --continue apply only with --destroy", "Add --destroy, or remove --confirm and --continue.", nil)
		}

		reqs := tools.Requirements{Docker: true, Colima: true, Secrets: true}
		if downDestroy {
			reqs = tools.AllRequirements()
		}
		proj, err := prepareProjectSkipValidation(cmd, reqs)
		if err != nil {
			return err
		}

		if proj.Workstation == nil {
			if downDestroy {
				return werror.New("windsor down --destroy needs a workstation context", "Run 'windsor destroy' to tear down this context.", nil)
			}
			fmt.Fprintln(os.Stderr, "windsor down is only applicable when a workstation is enabled; use windsor destroy to tear down live environments")
			return nil
		}

		if downDestroy {
			if err := destroyAll(cmd, proj, proj.Composer.BlueprintHandler.Generate()); err != nil {
				return err
			}
		}

		defer func() {
			printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), runtime.GOOS)
		}()

		if err := proj.Workstation.Down(cmd.Context()); err != nil {
			return werror.Wrap(err, "error tearing down workstation")
		}

		if err := proj.PerformCleanup(); err != nil {
			return werror.Wrap(err, "error performing cleanup")
		}

		return nil
	},
}

// init registers the down command with the root command. --confirm and --continue bind to the
// same variables as the destroy flags, because destroyAll reads them there.
func init() {
	downCmd.Flags().BoolVar(&downDestroy, "destroy", false, "Destroy the live infrastructure of the context before the workstation goes.")
	downCmd.Flags().StringVar(&destroyConfirm, "confirm", "", "Context name to confirm the destroy. Applies only with --destroy.")
	downCmd.Flags().BoolVar(&destroyContinue, "continue", false, "Continue past per-component destroy failures and report a summary. Applies only with --destroy. Down still stops before it deletes the workstation when a component fails.")
	rootCmd.AddCommand(downCmd)
}
