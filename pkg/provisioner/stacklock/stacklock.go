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

// lockDirPerm is the mode used when creating the lock-file's parent directory.
const lockDirPerm = 0o755

// stackLockFilename is the basename written under WindsorScratchPath.
const stackLockFilename = ".stacklock"

// stackLockInfoSuffix is appended to the lock-file path to produce the
// sidecar holder-info file. Kept as a sidecar (rather than written into the
// flock'd file body) to avoid interacting with byte-range locking on Windows.
const stackLockInfoSuffix = ".info"

// lockInfoPerm is the mode used when writing the holder-info sidecar.
const lockInfoPerm = 0o644

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

// LockInfo records who holds a stack lock and why. Persisted into a sidecar
// file next to the lock so a blocked contender can name the holder in its
// busy error and operators can identify the holding process.
type LockInfo struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Mode      Mode      `json:"mode"`
	Who       string    `json:"who"`
	Version   string    `json:"version"`
	ProjectID string    `json:"project_id"`
	Context   string    `json:"context"`
	Created   time.Time `json:"created"`
	PID       int       `json:"pid"`
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
	return fmt.Sprintf("stack lock at %s is held by %s (PID=%d, operation=%s, started=%s)",
		e.Path, e.Holder.Who, e.Holder.PID, e.Holder.Operation, e.Holder.Created.Format(time.RFC3339))
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
	lock, err := ForRuntime(rt)
	if err != nil {
		return err
	}
	release, err := lock.Acquire(ctx, NewInfo(rt, operation), DefaultTimeout)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// ForRuntime returns the StackLock for the runtime's context — the same lock that
// With acquires. It is exposed so operator-facing recovery (windsor unlock) can
// inspect and force-release a stuck lock without duplicating the path derivation.
// Returns an error when the runtime is nil or has not been configured yet (empty
// scratch path).
func ForRuntime(rt *runtime.Runtime) (StackLock, error) {
	if rt == nil {
		return nil, errors.New("stacklock: runtime is required")
	}
	if rt.WindsorScratchPath == "" {
		return nil, errors.New("stacklock: scratch path is empty (Configure must run first)")
	}
	return NewLocalFlockLock(filepath.Join(rt.WindsorScratchPath, stackLockFilename)), nil
}

// NewInfo constructs the LockInfo persisted into the holder-info sidecar for a
// given runtime and operation label. The result is diagnostic only — Acquire
// writes it next to the lock so the next contender's busy error can name the
// holder and the holding PID.
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
		PID:       os.Getpid(),
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// localFlockLock is the single-host implementation of StackLock. The struct
// itself owns no per-Acquire state — flk and the once-guard are scoped to each
// Release closure so that a future caller reusing one instance across multiple
// Acquires gets independent release semantics for each lock held.
type localFlockLock struct {
	path string
}

// Acquire takes the lock, retrying every acquireRetryInterval until it is held,
// the timeout elapses, or ctx is cancelled. Cancellation returns ctx.Err();
// timeout returns *LockBusyError, populating Holder from the sidecar info file
// when one is present. After flock succeeds, info is persisted to a sidecar
// (<path>.info) via atomic temp+rename so a future contender's busy error can
// name the holder and PID. The sidecar is removed by Release; it is diagnostic
// only and a missing or partial file is not load-bearing for correctness.
func (s *localFlockLock) Acquire(ctx context.Context, info LockInfo, timeout time.Duration) (Release, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if err := os.MkdirAll(filepath.Dir(s.path), lockDirPerm); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	flk := flock.New(s.path)
	infoPath := s.path + stackLockInfoSuffix
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
			return nil, &LockBusyError{Path: s.path, Holder: readHolderInfo(infoPath)}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(acquireRetryInterval):
		}
	}
	if info.PID == 0 {
		info.PID = os.Getpid()
	}
	writeHolderInfo(infoPath, info)
	return makeRelease(flk, infoPath), nil
}

// Inspect returns the current holder recorded in the sidecar, or (nil, nil) when
// no lock is held (or the sidecar is absent/unreadable). It backs windsor unlock's
// "who holds this?" report. The sidecar is diagnostic, so a missing file is not an
// error — it simply means there is nothing to release.
func (s *localFlockLock) Inspect(ctx context.Context) (*LockInfo, error) {
	return readHolderInfo(s.path + stackLockInfoSuffix), nil
}

// ForceRelease clears a stuck lock by removing the lock file and its holder-info
// sidecar, so the next Acquire starts from a clean slate. It is the operator-facing
// recovery path (windsor unlock) for a holder that died without releasing. When
// lockID is non-empty it guards against a race: if a different holder has acquired
// the lock since the caller inspected it, the release is refused rather than yanking
// a lock that is now legitimately held. reason is included in any failure message
// for diagnostics. Missing files are not an error — the lock is already clear.
func (s *localFlockLock) ForceRelease(ctx context.Context, lockID string, reason string) error {
	infoPath := s.path + stackLockInfoSuffix
	if lockID != "" {
		if info := readHolderInfo(infoPath); info != nil && info.ID != lockID {
			return fmt.Errorf("stacklock: refusing to force-release: lock is now held by a different holder (%q, not %q) — a new windsor process acquired it", info.ID, lockID)
		}
	}
	var errs []error
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if err := os.Remove(infoPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("stacklock: force-release (%s): %w", reason, errors.Join(errs...))
	}
	return nil
}

// makeRelease returns the closure handed back from Acquire. flk, infoPath,
// and the once guard are scoped to this Release, not to the localFlockLock
// receiver, so reusing the same lock instance for a second Acquire produces
// an independent Release that does not interfere with this one. The first
// call removes the holder-info sidecar (best-effort) and returns the
// underlying Unlock result; subsequent calls return nil so callers may safely
// defer release alongside an explicit earlier release without double-unlocking.
func makeRelease(flk *flock.Flock, infoPath string) Release {
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() {
			if infoPath != "" {
				_ = os.Remove(infoPath)
			}
			err = flk.Unlock()
		})
		return err
	}
}

// =============================================================================
// Helpers
// =============================================================================

// writeHolderInfo persists the holder LockInfo to the sidecar path via an
// atomic temp+rename so a concurrent reader never observes a partial file.
// Failures are swallowed because the sidecar is diagnostic only — losing it
// degrades busy-error detail but does not affect lock correctness.
func writeHolderInfo(path string, info LockInfo) {
	data, err := json.Marshal(info)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, lockInfoPerm); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

// readHolderInfo returns the LockInfo recorded by the current holder, or nil
// when the sidecar is absent or unparseable. Used by Acquire to populate
// LockBusyError.Holder on contention; callers must not depend on a non-nil
// result for correctness.
func readHolderInfo(path string) *LockInfo {
	// #nosec G304 - path is the lock's sidecar location configured by the runtime, not user-supplied at call time
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	return &info
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
