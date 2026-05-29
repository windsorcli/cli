package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the local workstation environment.",
	Long: `Tear down the workstation VM, stop container runtimes, and clear local context artifacts (.kube, .talos, generated terraform stubs, etc.). Live infrastructure is NOT destroyed by down — run 'windsor destroy' first if you need to remove cloud resources. Workstation contexts only.

If any host-side network or DNS configuration was previously installed by 'windsor configure network', down prints a follow-up command at the end so the operator can clean up leftover host state.`,
	Example: `# Standard teardown
windsor down

# Full teardown including cloud infrastructure
windsor destroy --confirm=local
windsor down`,
	Annotations: map[string]string{
		"docs.seealso": "[`up`](up.md), [`destroy`](destroy.md), [`configure network`](configure-network.md)",
		"docs.source": "cmd/down.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `windsor down` stops the local workstation (container runtime + colima VM if
		// applicable) and cleans up local artifacts. Terraform and kubelogin are not
		// exercised here. Secrets is requested because prepareProject → Initialize calls
		// LoadEnvironment, which shells out to sops / op when those backends are configured;
		// without this, a sops-enabled context with sops missing on PATH would surface a raw
		// exec error instead of the registry-formatted install hint. Config gates skip the
		// actual binary check for contexts that haven't enabled either backend. Skip-validation
		// tolerates a deployed-but-misordered blueprint so teardown stays runnable.
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Docker: true, Colima: true, Secrets: true})
		if err != nil {
			return err
		}

		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "windsor down is only applicable when a workstation is enabled; use windsor destroy to tear down live environments")
			return nil
		}

		// Down records leftover-host-config items on the workstation register BEFORE attempting
		// teardown, so render the summary via defer to survive a teardown or cleanup failure —
		// otherwise an operator who hits a container-runtime or VM stop error never learns the
		// host route / DNS resolver entries still need 'windsor configure network --revert'.
		// Wrapped in a closure so DeferredWork() is read at defer-execution time, after Down
		// has populated the register, rather than at defer-registration time.
		defer func() {
			printDeferredWork(os.Stderr, proj.Workstation.DeferredWork(), runtime.GOOS)
		}()

		if err := proj.Workstation.Down(); err != nil {
			return fmt.Errorf("error tearing down workstation: %w", err)
		}

		if err := proj.PerformCleanup(); err != nil {
			return fmt.Errorf("error performing cleanup: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
