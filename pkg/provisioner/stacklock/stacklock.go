// The StackLock is a single-writer advisory lock scoped to one (projectRoot, contextName).
// It provides a process-coordination point for windsor operations that mutate infrastructure,
// preventing two concurrent invocations from interleaving before terraform's per-state lock
// can engage. The current implementation is a local flock-backed adapter; s3, kubernetes,
// and azurerm backends slot in behind the same interface.

package stacklock

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

// DefaultTimeout is the lock-wait timeout used by With and by Acquire when the
// caller passes 0. Matches §7.4 of the terraform-lifecycle-hardening spike.
// Declared as a var so tests can shorten it; production callers must not
// reassign at runtime.
var DefaultTimeout = 5 * time.Minute

// acquireRetryInterval is the wait between TryLock attempts under contention.
// Short enough to give the timeout reasonable precision, long enough to avoid
// burning CPU under sustained contention.
const acquireRetryInterval = 50 * time.Millisecond

// lockDirPerm is the mode used when creating the lock-file's parent directory.
const lockDirPerm = 0o755

// stackLockFilename is the basename written under WindsorScratchPath.
const stackLockFilename = ".stacklock"

// =============================================================================
// Types
// =============================================================================

// Mode distinguishes writer (Exclusive) from reader (Shared) acquisition.
// Only Exclusive is supported today; Shared is reserved for read-only plan operations
// (§7.3 of the terraform-lifecycle-hardening spike).
type Mode int

const (
	Exclusive Mode = iota
	Shared
)

// LockInfo records who holds a stack lock and why. Serialised into the lock-file body
// for inspection by future --inspect (§7.6) tooling; not load-bearing for correctness.
type LockInfo struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Mode      Mode      `json:"mode"`
	Who       string    `json:"who"`
	Version   string    `json:"version"`
	ProjectID string    `json:"project_id"`
	Context   string    `json:"context"`
	Created   time.Time `json:"created"`
}

// Release frees a previously-acquired lock. Implementations must be idempotent:
// calls after the first one return nil.
type Release func() error

// LockBusyError is returned by Acquire when the timeout elapses with the lock still held.
// Holder is best-effort and may be nil when the lock-file body is absent or unparseable.
type LockBusyError struct {
	Path   string
	Holder *LockInfo
}

// Error renders a human-readable description of the lock contention; the Holder fields
// are included when known so operators can identify the blocker without opening the file.
func (e *LockBusyError) Error() string {
	if e.Holder == nil {
		return fmt.Sprintf("stack lock at %s is held by another windsor process", e.Path)
	}
	return fmt.Sprintf("stack lock at %s is held by %s (operation=%s, started=%s)",
		e.Path, e.Holder.Who, e.Holder.Operation, e.Holder.Created.Format(time.RFC3339))
}

// =============================================================================
// Interfaces
// =============================================================================

// StackLock coordinates exclusive access to a single (projectRoot, contextName) tuple
// across windsor invocations. A given StackLock instance is constructed per operation;
// implementations are not required to be safe for concurrent in-process reuse of one instance.
type StackLock interface {
	Acquire(ctx context.Context, info LockInfo, timeout time.Duration) (Release, error)
	Inspect(ctx context.Context) (*LockInfo, error)
	ForceRelease(ctx context.Context, lockID string, reason string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewLocalFlockLock returns a StackLock backed by a local advisory file lock at lockPath.
// The lock file's parent directory is created on first Acquire. Single-host coverage only;
// network filesystems where flock semantics are unreliable (NFS) will degrade silently.
func NewLocalFlockLock(lockPath string) StackLock {
	return &localFlockLock{path: lockPath}
}

// =============================================================================
// Public Methods
// =============================================================================

// With acquires an exclusive stack lock against the runtime's context and runs
// fn while holding it. The lock is released on return — including on panic —
// via deferred Release. operation labels what the caller is doing ("up",
// "apply", "destroy", "bootstrap", "plan") so concurrent operators see the
// holder's intent in busy-error messages.
func With(ctx context.Context, rt *runtime.Runtime, operation string, fn func() error) error {
	if rt == nil {
		return errors.New("stacklock: runtime is required")
	}
	if rt.WindsorScratchPath == "" {
		return errors.New("stacklock: scratch path is empty (Configure must run first)")
	}
	lock := NewLocalFlockLock(filepath.Join(rt.WindsorScratchPath, stackLockFilename))
	release, err := lock.Acquire(ctx, NewInfo(rt, operation), DefaultTimeout)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// NewInfo constructs the LockInfo persisted into the lock-file body for a given
// runtime and operation label. The result is diagnostic only — Acquire writes it
// into the body so the next contender's busy error can name the holder.
func NewInfo(rt *runtime.Runtime, operation string) LockInfo {
	return LockInfo{
		ID:        newLockID(),
		Operation: operation,
		Mode:      Exclusive,
		Who:       holderIdentity(),
		Version:   constants.Version,
		ProjectID: hashProjectRoot(rt.ProjectRoot),
		Context:   rt.ContextName,
		Created:   time.Now().UTC(),
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// errOperationNotSupported is returned by Inspect and ForceRelease, which are
// reserved for the operator-facing `windsor stack lock --inspect`/`--force`
// commands that are not yet wired up.
var errOperationNotSupported = errors.New("stacklock: operation not supported")

// localFlockLock is the single-host implementation of StackLock. The struct
// itself owns no per-Acquire state — flk and the once-guard are scoped to each
// Release closure so that a future caller reusing one instance across multiple
// Acquires gets independent release semantics for each lock held.
type localFlockLock struct {
	path string
}

// Acquire takes the lock, retrying every acquireRetryInterval until it is held,
// the timeout elapses, or ctx is cancelled. Cancellation returns ctx.Err();
// timeout returns *LockBusyError with Holder=nil — the local-flock backend does
// not persist holder identity (the body-write would race with the flock on
// Windows). Future backends with their own metadata stores can populate Holder.
// info is accepted for interface stability but is ignored by this implementation.
func (s *localFlockLock) Acquire(ctx context.Context, info LockInfo, timeout time.Duration) (Release, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if err := os.MkdirAll(filepath.Dir(s.path), lockDirPerm); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	flk := flock.New(s.path)
	deadline := time.Now().Add(timeout)
	for {
		locked, err := flk.TryLock()
		if err != nil {
			return nil, fmt.Errorf("flock acquire: %w", err)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			return nil, &LockBusyError{Path: s.path}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(acquireRetryInterval):
		}
	}
	return makeRelease(flk), nil
}

// Inspect is reserved for the operator-facing `windsor stack lock --inspect`
// command; the CLI entry point is not yet wired up.
func (s *localFlockLock) Inspect(ctx context.Context) (*LockInfo, error) {
	return nil, errOperationNotSupported
}

// ForceRelease is reserved for the operator-facing `windsor stack lock --force`
// command; the CLI entry point is not yet wired up.
func (s *localFlockLock) ForceRelease(ctx context.Context, lockID string, reason string) error {
	return errOperationNotSupported
}

// makeRelease returns the closure handed back from Acquire. flk and the once
// guard are scoped to this Release, not to the localFlockLock receiver, so
// reusing the same lock instance for a second Acquire produces an independent
// Release that does not interfere with this one. The first call returns the
// underlying Unlock result; subsequent calls return nil so callers may safely
// defer release alongside an explicit earlier release without double-unlocking.
func makeRelease(flk *flock.Flock) Release {
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() {
			err = flk.Unlock()
		})
		return err
	}
}

// =============================================================================
// Helpers
// =============================================================================

// newLockID generates a 128-bit random ID rendered as 32 hex characters.
// Used only to identify lock holders in audit messages; collision risk is negligible.
func newLockID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// holderIdentity returns "<user>@<host>" with safe fallbacks so the lock
// always carries some identifier even on hosts where the lookups fail.
func holderIdentity() string {
	u := "unknown"
	if usr, err := user.Current(); err == nil && usr.Username != "" {
		u = usr.Username
	}
	h, err := os.Hostname()
	if err != nil || h == "" {
		h = "unknown"
	}
	return fmt.Sprintf("%s@%s", u, h)
}

// hashProjectRoot returns a stable short hash of the project root path so
// LockInfo can identify the project without persisting absolute paths into
// what may end up shared across teammates' machines.
func hashProjectRoot(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:8])
}
