package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Stop the local workstation environment",
	Long:         "Stop the local workstation environment by tearing down the VM, stopping container runtimes, and cleaning up context artifacts.",
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
