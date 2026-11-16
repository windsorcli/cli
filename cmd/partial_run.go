// The partial run helpers of the cmd package serve the --to and --continue flags of up and bootstrap.
// It provides the flag registration and the end-of-run handling of the saved record.
// The provisioner owns the plan and the record. These helpers only connect them to the commands.

package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
)

// =============================================================================
// Helpers
// =============================================================================

// addPartialRunFlags registers --to and --continue on c.
func addPartialRunFlags(c *pflag.FlagSet, to *string, cont *bool) {
	c.StringVar(to, "to", "", "Stop after this terraform component or kustomization. Use terraform/<id> or kustomize/<name> when a name matches both.")
	c.BoolVar(cont, "continue", false, "Continue a run that --to stopped, and skip the terraform components that it finished.")
}

// finishPartialRun saves or clears the record after a run that was not halted, and reports a stop.
// A run that --to stopped saves the record and prints the command that continues it.
// A run to the end removes the record. It returns true when the run stopped at a --to target.
func finishPartialRun(w io.Writer, proj *project.Project, plan *provisioner.RunPlan, blueprint *blueprintv1alpha1.Blueprint) (bool, error) {
	if !plan.Stops() {
		return false, proj.Provisioner.ClearPartialRun()
	}
	if err := proj.Provisioner.RecordPartialRun(blueprint, plan); err != nil {
		return false, err
	}
	fmt.Fprintf(w, "Stopped after %s. Run 'windsor %s --continue' to continue.\n", plan.StopLabel(), plan.Command)
	return true, nil
}
