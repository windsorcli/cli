package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestHeldLockError(t *testing.T) {
	t.Run("NamesContextWhenHolderIsUnknown", func(t *testing.T) {
		// Given no details about the holder of the lock

		// When the error is built
		err := heldLockError("local", nil)

		// Then the message names the context and the hint tells the operator to stop the process
		if !strings.Contains(err.Error(), `stack lock for context "local"`) {
			t.Errorf("Expected the message to name the context, got %q", err.Error())
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "Stop that process") {
			t.Errorf("Expected a hint to stop the process, got %+v", werr)
		}
	})

	t.Run("NamesHolderWhenDetailsAreKnown", func(t *testing.T) {
		// Given the details of the holder of the lock
		holder := &stacklock.LockInfo{Who: "dev@host", PID: 4242, Operation: "apply", Created: time.Unix(0, 0).UTC()}

		// When the error is built
		err := heldLockError("local", holder)

		// Then the message names the holder and the hint tells the operator to stop the process
		if !strings.Contains(err.Error(), "dev@host, PID=4242, operation=apply") {
			t.Errorf("Expected the message to name the holder, got %q", err.Error())
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "Stop that process") {
			t.Errorf("Expected a hint to stop the process, got %+v", werr)
		}
	})
}
