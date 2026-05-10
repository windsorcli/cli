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
	"encoding/json"
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

// lockFilePerm is the mode used when creating the lock-file body. 0o600 keeps
// the operator identity in the body unreadable by other accounts on shared hosts.
const lockFilePerm = 0o600

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

// localFlockLock is the single-host implementation of StackLock. It serialises a
// LockInfo into the lock-file body so peers can identify the holder, but the body
// is informational only — correctness rides on flock(2) alone.
type localFlockLock struct {
	path        string
	flk         *flock.Flock
	releaseOnce sync.Once
}

// Acquire takes the lock, retrying every acquireRetryInterval until it is held,
// the timeout elapses, or ctx is cancelled. Cancellation returns ctx.Err();
// timeout returns *LockBusyError with the holder's LockInfo when the body is
// readable. On success the caller's LockInfo is written into the file body.
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
			holder, _ := readLockBody(s.path)
			return nil, &LockBusyError{Path: s.path, Holder: holder}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(acquireRetryInterval):
		}
	}
	if err := writeLockBody(s.path, info); err != nil {
		_ = flk.Unlock()
		return nil, err
	}
	s.flk = flk
	return s.makeRelease(), nil
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

// makeRelease returns the closure handed back from Acquire. The unlock runs at
// most once; callers may safely defer release in the same scope as a `defer
// release()` even when an explicit release earlier in the path has already fired.
func (s *localFlockLock) makeRelease() Release {
	return func() error {
		var firstErr error
		s.releaseOnce.Do(func() {
			if s.flk != nil {
				firstErr = s.flk.Unlock()
			}
		})
		return firstErr
	}
}

// =============================================================================
// Helpers
// =============================================================================

// writeLockBody serialises info as JSON and truncates the lock file with the
// result. The flock holder must already own the lock; callers that fail must
// release the flock before returning.
func writeLockBody(path string, info LockInfo) error {
	body, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal lock info: %w", err)
	}
	if err := os.WriteFile(path, body, lockFilePerm); err != nil {
		return fmt.Errorf("write lock body: %w", err)
	}
	return nil
}

// readLockBody returns the holder LockInfo when the file is present and valid.
// Reads happen without holding the flock, so partial writes during a peer's
// in-flight Acquire surface as parse errors — callers treat any non-nil error
// as "unknown holder" and proceed.
func readLockBody(path string) (*LockInfo, error) {
	// #nosec G304 - path is built from rt.WindsorScratchPath inside With, never user input
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, errors.New("empty lock body")
	}
	var info LockInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

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
