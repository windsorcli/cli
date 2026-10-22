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

// downCmd stops the local workstation environment and removes the local artifacts.
// It requests docker, colima and the secrets tools. The secrets tools are necessary, because Initialize can run sops or op.
// It skips validation, so that a broken setup can still stop.
// It prints the remaining host changes in a deferred call, so the user sees them when the teardown fails.
// The deferred call reads the list when it runs, after Down fills it.
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the local workstation environment.",
	Long: `Tear down the workstation VM, stop container runtimes, and clear the runtime caches (.terraform, .tfstate) and stale cluster credentials (.kube, .talos). Hand-edited tfvars and cloud credentials are preserved. Live infrastructure is NOT destroyed by down — run 'windsor destroy' first if you need to remove cloud resources. Workstation contexts only.

If any host-side network or DNS configuration was previously installed by 'windsor configure network', down prints a follow-up command at the end so the operator can clean up leftover host state.`,
	Example: `# Standard teardown
windsor down

# Full teardown including cloud infrastructure
windsor destroy --confirm=local
windsor down`,
	Annotations: map[string]string{
		"docs.seealso": "[`up`](up.md), [`destroy`](destroy.md), [`configure network`](configure-network.md)",
		"docs.source":  "cmd/down.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Docker: true, Colima: true, Secrets: true})
		if err != nil {
			return err
		}

		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "windsor down is only applicable when a workstation is enabled; use windsor destroy to tear down live environments")
			return nil
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

// init registers the down command with the root command.
func init() {
	rootCmd.AddCommand(downCmd)
}
