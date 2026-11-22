// The op_call file holds the shared 1Password call guard of the secrets package.
// It runs one op CLI or SDK call under a timeout, an interrupt watch, and an approval hint.
// It also turns the failure text of op into a message that names the fix.

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// opTimeoutEnv is the environment variable that sets the timeout of one 1Password call, in
// Go duration syntax such as "45s" or "2m".
const opTimeoutEnv = "WINDSOR_OP_TIMEOUT"

// defaultOpTimeout is the timeout of one 1Password call when opTimeoutEnv is not set.
const defaultOpTimeout = 30 * time.Second

// opApprovalHintDelay is a variable so tests can shorten it. It is the wait after which a 1Password call prints the approval hint.
var opApprovalHintDelay = 3 * time.Second

// opStderrLimit is the most characters of op stderr that an error message includes.
const opStderrLimit = 300

// =============================================================================
// Private Helpers
// =============================================================================

// opTimeout returns the timeout of one 1Password call. An unset, invalid, or non-positive
// value of opTimeoutEnv gives defaultOpTimeout.
func opTimeout(getenv func(string) string) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(opTimeoutEnv))); err == nil && d > 0 {
		return d
	}
	return defaultOpTimeout
}

// runOnePasswordCall runs fn with a context that ends at the timeout or at an interrupt.
// After opApprovalHintDelay it prints a hint that 1Password may ask for approval.
// A timeout returns an error with the account and the fix. An interrupt returns an error that
// names what the call was resolving. what is a short phrase such as `secret "op://V/I/F"`.
// Any other error from fn is returned unchanged. The call returns at the timeout or interrupt
// even when fn does not honor its context.
func runOnePasswordCall(shims *Shims, account, what string, fn func(ctx context.Context) error) error {
	timeout := opTimeout(shims.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	interrupted, stopWatch := shims.WatchInterrupt()
	defer stopWatch()
	release := presenter.GuardInterrupts()
	defer release()

	hint := time.AfterFunc(opApprovalHintDelay, func() {
		if shims.HintWriter != nil {
			fmt.Fprintf(shims.HintWriter, "Waiting for 1Password (%s) to resolve %s. Approve the request in the 1Password app if it is asking.\n", account, what)
		}
	})
	defer hint.Stop()

	go func() {
		select {
		case <-interrupted:
			cancel()
		case <-ctx.Done():
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- fn(ctx) }()
	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return werror.New(fmt.Sprintf("Timed out after %s waiting for 1Password (%s) to resolve %s.", timeout, account, what),
			fmt.Sprintf("Unlock 1Password, check Settings > Developer > Integrate with 1Password CLI, then run `op whoami --account %s`. Set %s to wait longer.", account, opTimeoutEnv), err)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return werror.New(fmt.Sprintf("Interrupted while resolving %s from 1Password (%s).", what, account), "", err)
	}
	return err
}

// classifyOpError turns a failed op call into an error. The message holds the stderr of op and
// the remediation names the fix for the common failures: no account, not signed in, desktop
// app unreachable, item not found, and field not found. Other failures keep the stderr and the
// cause. what is a short phrase such as `secret "op://V/I/F"`.
func classifyOpError(cause error, stderr []byte, account, what string) error {
	text := strings.Join(strings.Fields(string(stderr)), " ")
	if text == "" && cause != nil {
		text = cause.Error()
	}
	if len(text) > opStderrLimit {
		text = text[:opStderrLimit] + "..."
	}
	lower := strings.ToLower(text)

	var summary, fix string
	switch {
	case strings.Contains(lower, "no account found") || strings.Contains(lower, "account not found") || strings.Contains(lower, "found no account"):
		summary = fmt.Sprintf("1Password has no account %s.", account)
		fix = fmt.Sprintf("Check the vault url in secrets.onepassword.vaults, or run `op account add --address %s`.", account)
	case strings.Contains(lower, "desktop app"):
		summary = fmt.Sprintf("Couldn't reach the 1Password desktop app for %s.", account)
		fix = fmt.Sprintf("Unlock 1Password, check Settings > Developer > Integrate with 1Password CLI, then run `op whoami --account %s`.", account)
	case strings.Contains(lower, "not signed in") || strings.Contains(lower, "no accounts configured") || strings.Contains(lower, "signin") || strings.Contains(lower, "sign in"):
		summary = fmt.Sprintf("1Password isn't signed in for %s.", account)
		fix = fmt.Sprintf("Run `op signin --account %s`.", account)
	case strings.Contains(lower, "isn't a field") || strings.Contains(lower, "no field") || strings.Contains(lower, "field not found") || strings.Contains(lower, "does not have a field"):
		summary = fmt.Sprintf("1Password has no such field for %s.", what)
		fix = "Check the field name in secret()."
	case strings.Contains(lower, "isn't an item") || strings.Contains(lower, "item not found") || strings.Contains(lower, "could not find item") || strings.Contains(lower, "isn't a vault"):
		summary = fmt.Sprintf("1Password has no such item or vault for %s.", what)
		fix = "Check the vault name and the item name in secret()."
	default:
		summary = fmt.Sprintf("Failed to read %s from 1Password (%s).", what, account)
	}
	if text != "" {
		summary += " op said: " + text
	}
	return werror.New(summary, fix, cause)
}
