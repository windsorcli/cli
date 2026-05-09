// The StackLock is a single-writer advisory lock scoped to one (projectRoot, contextName).
// It provides a process-coordination point for windsor operations that mutate infrastructure,
// preventing two concurrent invocations from interleaving before terraform's per-state lock
// can engage. Phase 1 ships only the local flock-backed adapter; future phases add s3,
// kubernetes, and azurerm backends behind the same interface.

package stacklock

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// =============================================================================
// Constants
// =============================================================================

// DefaultTimeout is how long Acquire waits for a held lock before returning a LockBusyError.
const DefaultTimeout = 5 * time.Minute

// =============================================================================
// Types
// =============================================================================

// Mode distinguishes writer (Exclusive) from reader (Shared) acquisition.
// Phase 1 supports only Exclusive; Shared is reserved for the plan-side extension in Phase 4
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
// The lock file's parent directory is created on first Acquire. Phase 1 returns a stub
// that fails every call with errNotImplemented; Phase 2 replaces with the real adapter.
func NewLocalFlockLock(lockPath string) StackLock {
	return &localFlockLockStub{path: lockPath}
}

// =============================================================================
// Private Methods
// =============================================================================

// errNotImplemented is the placeholder returned by the Phase 1 stub constructor so that
// the failing test suite drives Phase 2 implementation. Removed when the real adapter lands.
var errNotImplemented = errors.New("stacklock: not implemented (phase 1 stub)")

// localFlockLockStub satisfies StackLock for the contract baseline; every method returns
// errNotImplemented. Replaced wholesale by localFlockLock in Phase 2.
type localFlockLockStub struct {
	path string
}

func (s *localFlockLockStub) Acquire(ctx context.Context, info LockInfo, timeout time.Duration) (Release, error) {
	return nil, errNotImplemented
}

func (s *localFlockLockStub) Inspect(ctx context.Context) (*LockInfo, error) {
	return nil, errNotImplemented
}

func (s *localFlockLockStub) ForceRelease(ctx context.Context, lockID string, reason string) error {
	return errNotImplemented
}
