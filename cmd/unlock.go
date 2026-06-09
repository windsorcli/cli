package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Unlock Command
// =============================================================================

var unlockForce bool

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Release a stuck stack lock.",
	Long: `Force-release a stuck stack lock for the current context.

A holder killed before it could release (CI cancellation, OOM, crash) leaves the lock behind, so later commands block until timeout and then fail. This clears it. It does not check whether the holder is still alive, so only run it when no other windsor process is using this context.`,
	Example: `# Clear a stuck lock interactively
windsor unlock
# → prompts: Type "local" to confirm:

# Scripted recovery
windsor unlock --force`,
	Annotations: map[string]string{
		"docs.seealso": "[`destroy`](destroy.md), [`up`](up.md)",
		"docs.source":  "cmd/unlock.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// unlock only touches the local lock files; no terraform/k8s/docker tools are
		// needed. Skip-validation so a deployed-but-misordered blueprint can't block
		// the recovery path that exists precisely to unstick such a context.
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{})
		if err != nil {
			return err
		}

		lock, err := stacklock.ForRuntime(proj.Runtime)
		if err != nil {
			return err
		}

		contextName := proj.Runtime.ContextName
		w := cmd.ErrOrStderr()

		// A missing sidecar means nothing is held. A corrupt/unreadable one (e.g. a
		// partial write from a killed holder) is exactly the debris unlock clears, so
		// warn and proceed rather than treating it as "nothing to release".
		holder, inspectErr := lock.Inspect(cmd.Context())
		lockID := ""
		switch {
		case inspectErr != nil:
			fmt.Fprintf(w, "Stack lock for context %q has unreadable holder info (%v); clearing it.\n", contextName, inspectErr)
		case holder == nil:
			fmt.Fprintf(w, "No stack lock held for context %q; nothing to release.\n", contextName)
			return nil
		default:
			lockID = holder.ID
			fmt.Fprintf(w, "Stack lock for context %q is held by %s (PID=%d, operation=%s, started=%s).\n",
				contextName, holder.Who, holder.PID, holder.Operation, holder.Created.Format(time.RFC3339))
		}

		if !unlockForce {
			desc := fmt.Sprintf("This will force-release the stack lock for context %q. Only proceed if no other windsor process is operating on it.", contextName)
			if err := confirmDestroy(cmd.InOrStdin(), w, desc, contextName); err != nil {
				return err
			}
		}

		if err := lock.ForceRelease(cmd.Context(), lockID, "windsor unlock"); err != nil {
			return fmt.Errorf("error releasing stack lock: %w", err)
		}
		fmt.Fprintf(w, "Released stack lock for context %q.\n", contextName)
		return nil
	},
}

// init registers the unlock command and its --force flag, which skips the
// type-the-context confirmation for scripted recovery.
func init() {
	unlockCmd.Flags().BoolVar(&unlockForce, "force", false, "Skip the confirmation prompt (for scripted recovery).")
	rootCmd.AddCommand(unlockCmd)
}
