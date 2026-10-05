package cmd

import (
	"errors"
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
	Short: "Clear stale stack lock information.",
	Long: `Clear stale stack lock information for the current context.

The lock frees itself when its holder exits, including after a crash, an OOM kill, or a CI cancellation. A crash can leave holder details behind, and this command removes them. If a running process still holds the lock, the command names it and exits with an error. Stop that process to free the lock.`,
	Example: `# Clear stale lock information interactively
windsor unlock
# → prompts: Type "local" to confirm:

# Scripted recovery
windsor unlock --force`,
	Annotations: map[string]string{
		"docs.seealso": "[`destroy`](destroy.md), [`up`](up.md)\n" +
			"[Global flags](../global-flags.md) — `--lock-timeout` waits for a lock instead of failing immediately",
		"docs.source":  "cmd/unlock.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		held, err := lock.IsHeld(cmd.Context())
		if err != nil {
			return fmt.Errorf("error checking stack lock: %w", err)
		}
		holder, inspectErr := lock.Inspect(cmd.Context())
		if held {
			return heldLockError(contextName, holder)
		}
		switch {
		case inspectErr != nil:
			fmt.Fprintf(w, "Stack lock for context %q has unreadable holder info (%v); clearing it.\n", contextName, inspectErr)
		case holder == nil:
			fmt.Fprintf(w, "No stack lock held for context %q; nothing to release.\n", contextName)
			return nil
		default:
			fmt.Fprintf(w, "No process holds the stack lock for context %q. Stale holder info remains from %s (PID=%d, operation=%s, started=%s).\n",
				contextName, holder.Who, holder.PID, holder.Operation, holder.Created.Format(time.RFC3339))
		}

		if !unlockForce {
			desc := fmt.Sprintf("This will clear the stale stack lock information for context %q.", contextName)
			if err := confirmDestroy(cmd.InOrStdin(), w, desc, contextName); err != nil {
				return err
			}
		}

		if err := lock.ClearStale(cmd.Context(), "windsor unlock"); err != nil {
			var busy *stacklock.LockBusyError
			if errors.As(err, &busy) {
				return heldLockError(contextName, busy.Holder)
			}
			return fmt.Errorf("error clearing stack lock: %w", err)
		}
		fmt.Fprintf(w, "Cleared stale stack lock information for context %q.\n", contextName)
		return nil
	},
}

// heldLockError builds the error unlock returns when a live process holds the lock. It names
// the holder when its details are known and tells the operator to stop that process.
func heldLockError(contextName string, holder *stacklock.LockInfo) error {
	if holder == nil {
		return fmt.Errorf("a running windsor process holds the stack lock for context %q; stop it to free the lock", contextName)
	}
	return fmt.Errorf("a running windsor process holds the stack lock for context %q (%s, PID=%d, operation=%s, started=%s); stop it to free the lock",
		contextName, holder.Who, holder.PID, holder.Operation, holder.Created.Format(time.RFC3339))
}

// init registers the unlock command and its --force flag, which skips the
// type-the-context confirmation for scripted recovery.
func init() {
	unlockCmd.Flags().BoolVar(&unlockForce, "force", false, "Skip the confirmation prompt (for scripted recovery).")
	rootCmd.AddCommand(unlockCmd)
}
