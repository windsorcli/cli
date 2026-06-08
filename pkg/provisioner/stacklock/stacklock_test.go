package stacklock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Test Setup
// =============================================================================

// newTestLockInfo returns a fully-populated LockInfo suitable for any test
// that needs to acquire a lock. Tests that assert on specific holder fields
// override Who/Operation directly on the returned value.
func newTestLockInfo() LockInfo {
	return LockInfo{
		ID:        "test-id-1",
		Operation: "up",
		Mode:      Exclusive,
		Who:       "tester@host",
		Version:   "0.0.0-test",
		ProjectID: "proj-hash",
		Context:   "test-ctx",
		Created:   time.Now().UTC(),
	}
}

// newTestRuntime returns a minimal *runtime.Runtime sufficient to drive With.
// WindsorScratchPath points at a per-test temp directory so concurrent tests
// do not contend on the same lock file.
func newTestRuntime(t *testing.T) *runtime.Runtime {
	t.Helper()
	return &runtime.Runtime{
		WindsorScratchPath: t.TempDir(),
		ProjectRoot:        "/tmp/windsor-test-proj",
		ContextName:        "test-ctx",
	}
}

// withDefaultTimeout swaps DefaultTimeout for the duration of the test. Tests
// that override must not run in parallel — the var is package-global. Restored
// on cleanup.
func withDefaultTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := DefaultTimeout
	DefaultTimeout = d
	t.Cleanup(func() { DefaultTimeout = prev })
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewLocalFlockLock(t *testing.T) {
	t.Run("returns a non-nil StackLock for any path", func(t *testing.T) {
		// Given a candidate lock-file path under a temp directory
		path := filepath.Join(t.TempDir(), ".stacklock")

		// When constructing a local flock lock
		sl := NewLocalFlockLock(path)

		// Then a usable StackLock instance is returned
		if sl == nil {
			t.Fatal("expected non-nil StackLock")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLocalFlockLock_Acquire(t *testing.T) {
	t.Run("succeeds when the lock file does not exist", func(t *testing.T) {
		// Given a fresh lock path with no prior file
		path := filepath.Join(t.TempDir(), ".stacklock")
		sl := NewLocalFlockLock(path)

		// When acquiring with a short timeout
		release, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)

		// Then acquisition succeeds and yields a non-nil Release
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if release == nil {
			t.Fatal("expected non-nil Release")
		}
		if err := release(); err != nil {
			t.Fatalf("release: %v", err)
		}
	})

	t.Run("creates the lock file's parent directory on demand", func(t *testing.T) {
		// Given a lock path under a not-yet-existing nested directory
		root := t.TempDir()
		path := filepath.Join(root, "nested", "deeper", ".stacklock")
		sl := NewLocalFlockLock(path)

		// When acquiring
		release, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)

		// Then the parent directory is created and acquisition succeeds
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		t.Cleanup(func() { _ = release() })
		if _, err := os.Stat(filepath.Dir(path)); err != nil {
			t.Fatalf("expected parent dir created, got %v", err)
		}
	})

	t.Run("blocks then times out when held by a separate instance", func(t *testing.T) {
		// Given a lock already held by one instance against a shared path
		path := filepath.Join(t.TempDir(), ".stacklock")
		first := NewLocalFlockLock(path)
		release1, err := first.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		t.Cleanup(func() { _ = release1() })

		// When a second instance attempts to acquire with a short timeout
		second := NewLocalFlockLock(path)
		start := time.Now()
		_, err = second.Acquire(context.Background(), newTestLockInfo(), 200*time.Millisecond)
		elapsed := time.Since(start)

		// Then a LockBusyError is returned and the wait honoured the timeout
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if elapsed < 150*time.Millisecond {
			t.Fatalf("expected ~200ms wait, got %v", elapsed)
		}
	})

	t.Run("populates Holder from the sidecar info file on contention", func(t *testing.T) {
		// Given a held lock whose holder wrote a populated LockInfo to the sidecar
		path := filepath.Join(t.TempDir(), ".stacklock")
		first := NewLocalFlockLock(path)
		holderInfo := newTestLockInfo()
		holderInfo.Who = "ci@runner-9"
		holderInfo.Operation = "bootstrap"
		holderInfo.PID = 4242
		release1, err := first.Acquire(context.Background(), holderInfo, time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		t.Cleanup(func() { _ = release1() })

		// When a second instance times out
		second := NewLocalFlockLock(path)
		_, err = second.Acquire(context.Background(), newTestLockInfo(), 100*time.Millisecond)

		// Then a *LockBusyError surfaces with Path set and Holder populated from the sidecar
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if busy.Path != path {
			t.Fatalf("expected Path %q, got %q", path, busy.Path)
		}
		if busy.Holder == nil {
			t.Fatal("expected non-nil Holder populated from sidecar")
		}
		if busy.Holder.Who != "ci@runner-9" || busy.Holder.Operation != "bootstrap" || busy.Holder.PID != 4242 {
			t.Fatalf("expected holder ci@runner-9/bootstrap/PID=4242, got %+v", busy.Holder)
		}
	})

	t.Run("returns a busy error with nil Holder when no sidecar is present", func(t *testing.T) {
		// Given a lock file held by an external flock with no sidecar written
		// (simulates a non-windsor flock holder or a holder that died mid-write)
		path := filepath.Join(t.TempDir(), ".stacklock")
		external := flock.New(path)
		locked, err := external.TryLock()
		if err != nil || !locked {
			t.Fatalf("external flock: locked=%v err=%v", locked, err)
		}
		t.Cleanup(func() { _ = external.Unlock() })

		// When a windsor instance times out
		sl := NewLocalFlockLock(path)
		_, err = sl.Acquire(context.Background(), newTestLockInfo(), 100*time.Millisecond)

		// Then a *LockBusyError surfaces with Holder nil — no diagnostic data available
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if busy.Holder != nil {
			t.Fatalf("expected nil Holder without sidecar, got %+v", busy.Holder)
		}
	})

	t.Run("writes the holder-info sidecar on acquire and removes it on release", func(t *testing.T) {
		// Given an acquired lock
		path := filepath.Join(t.TempDir(), ".stacklock")
		sidecar := path + ".info"
		sl := NewLocalFlockLock(path)
		info := newTestLockInfo()
		info.PID = 9999
		release, err := sl.Acquire(context.Background(), info, time.Second)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}

		// Then the sidecar exists and decodes to the holder's LockInfo
		data, err := os.ReadFile(sidecar)
		if err != nil {
			t.Fatalf("read sidecar: %v", err)
		}
		var got LockInfo
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode sidecar: %v", err)
		}
		if got.Who != info.Who || got.PID != 9999 {
			t.Fatalf("sidecar contents: got %+v want Who=%s PID=9999", got, info.Who)
		}

		// When the lock is released
		if err := release(); err != nil {
			t.Fatalf("release: %v", err)
		}

		// Then the sidecar is removed
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("expected sidecar removed, stat err=%v", err)
		}
	})

	t.Run("populates PID with os.Getpid when caller passes 0", func(t *testing.T) {
		// Given a lock acquired with info.PID unset (the With code path doesn't
		// always go through NewInfo, so Acquire defends against a zero PID)
		path := filepath.Join(t.TempDir(), ".stacklock")
		sl := NewLocalFlockLock(path)
		info := newTestLockInfo()
		info.PID = 0
		release, err := sl.Acquire(context.Background(), info, time.Second)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		t.Cleanup(func() { _ = release() })

		// When the sidecar is read back
		data, err := os.ReadFile(path + ".info")
		if err != nil {
			t.Fatalf("read sidecar: %v", err)
		}
		var got LockInfo
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode sidecar: %v", err)
		}

		// Then PID reflects the running process
		if got.PID != os.Getpid() {
			t.Fatalf("expected PID=%d, got %d", os.Getpid(), got.PID)
		}
	})

	t.Run("succeeds for a second acquirer after the holder releases", func(t *testing.T) {
		// Given a held-then-released lock
		path := filepath.Join(t.TempDir(), ".stacklock")
		first := NewLocalFlockLock(path)
		release1, err := first.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		if err := release1(); err != nil {
			t.Fatalf("release: %v", err)
		}

		// When a second instance acquires the same path
		second := NewLocalFlockLock(path)
		release2, err := second.Acquire(context.Background(), newTestLockInfo(), time.Second)

		// Then acquisition succeeds
		if err != nil {
			t.Fatalf("re-acquire: %v", err)
		}
		t.Cleanup(func() { _ = release2() })
	})

	t.Run("aborts the wait when the context is cancelled", func(t *testing.T) {
		// Given a held lock and a cancellable context with a long timeout
		path := filepath.Join(t.TempDir(), ".stacklock")
		held := NewLocalFlockLock(path)
		release1, err := held.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		t.Cleanup(func() { _ = release1() })
		ctx, cancel := context.WithCancel(context.Background())

		// When a goroutine starts a second acquire and the context is cancelled
		var (
			secondErr error
			wg        sync.WaitGroup
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			second := NewLocalFlockLock(path)
			_, secondErr = second.Acquire(ctx, newTestLockInfo(), time.Hour)
		}()
		time.Sleep(50 * time.Millisecond)
		cancel()
		wg.Wait()

		// Then the second acquire returns context.Canceled
		if !errors.Is(secondErr, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", secondErr)
		}
	})
}

func TestLocalFlockLock_ReusedInstance(t *testing.T) {
	t.Run("produces independent Release closures across sequential Acquires", func(t *testing.T) {
		// Given a single localFlockLock instance reused across two Acquires
		path := filepath.Join(t.TempDir(), ".stacklock")
		sl := NewLocalFlockLock(path)

		release1, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		if err := release1(); err != nil {
			t.Fatalf("release1: %v", err)
		}

		release2, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("second acquire: %v", err)
		}
		if err := release2(); err != nil {
			t.Fatalf("release2: %v", err)
		}

		// When a third acquirer takes the same path
		// Then it succeeds — proving release2 actually unlocked the second flock
		// (would time out with LockBusyError if release2 had been a no-op)
		other := NewLocalFlockLock(path)
		release3, err := other.Acquire(context.Background(), newTestLockInfo(), 200*time.Millisecond)
		if err != nil {
			t.Fatalf("third acquire should succeed if release2 unlocked: %v", err)
		}
		t.Cleanup(func() { _ = release3() })
	})
}

func TestLocalFlockLock_Release(t *testing.T) {
	t.Run("is idempotent", func(t *testing.T) {
		// Given an acquired lock
		path := filepath.Join(t.TempDir(), ".stacklock")
		sl := NewLocalFlockLock(path)
		release, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}

		// When release is called more than once
		if err := release(); err != nil {
			t.Fatalf("first release: %v", err)
		}
		// Then subsequent calls return nil
		if err := release(); err != nil {
			t.Fatalf("second release: %v", err)
		}
	})
}

func TestLockBusyError_Error(t *testing.T) {
	t.Run("includes the path when holder is unknown", func(t *testing.T) {
		// Given a busy error with no holder
		e := &LockBusyError{Path: "/tmp/x.stacklock"}

		// When formatting
		msg := e.Error()

		// Then the message names the path
		if !strings.Contains(msg, "/tmp/x.stacklock") {
			t.Fatalf("expected path in message, got %q", msg)
		}
	})

	t.Run("includes who, PID, and operation when the holder is known", func(t *testing.T) {
		// Given a busy error with a populated holder
		info := newTestLockInfo()
		info.Who = "ci@runner-7"
		info.Operation = "apply"
		info.PID = 12345
		e := &LockBusyError{Path: "/tmp/x.stacklock", Holder: &info}

		// When formatting
		msg := e.Error()

		// Then who, PID, and operation all appear so operators can identify the blocker
		for _, want := range []string{"ci@runner-7", "PID=12345", "apply"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("expected %q in message, got %q", want, msg)
			}
		}
	})
}

func TestWith(t *testing.T) {
	t.Run("returns an error when runtime is nil", func(t *testing.T) {
		// Given a nil runtime
		// When invoking With
		err := With(context.Background(), nil, "up", func() error { return nil })
		// Then an error is returned (fn would panic on nil access if it ran)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns an error when WindsorScratchPath is empty", func(t *testing.T) {
		// Given a runtime with no scratch path populated
		rt := &runtime.Runtime{}

		// When invoking With
		err := With(context.Background(), rt, "up", func() error { return nil })

		// Then an error is returned identifying the missing path
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invokes fn while holding the lock and returns its nil result", func(t *testing.T) {
		// Given a runtime with a usable scratch path
		rt := newTestRuntime(t)
		called := false

		// When invoking With
		err := With(context.Background(), rt, "up", func() error {
			called = true
			return nil
		})

		// Then fn is invoked and the call returns nil
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !called {
			t.Fatal("fn was not invoked")
		}
	})

	t.Run("propagates the error returned by fn", func(t *testing.T) {
		// Given a runtime and an fn that returns a sentinel error
		rt := newTestRuntime(t)
		sentinel := errors.New("from fn")

		// When invoking With
		err := With(context.Background(), rt, "up", func() error { return sentinel })

		// Then the sentinel surfaces unchanged
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected sentinel, got %v", err)
		}
	})

	t.Run("releases the lock so a second invocation can re-acquire", func(t *testing.T) {
		// Given two sequential invocations against the same scratch path
		rt := newTestRuntime(t)

		// When the first call completes
		if err := With(context.Background(), rt, "up", func() error { return nil }); err != nil {
			t.Fatalf("first: %v", err)
		}

		// Then a second call against the same path acquires successfully
		if err := With(context.Background(), rt, "apply", func() error { return nil }); err != nil {
			t.Fatalf("second: %v", err)
		}
	})

	t.Run("releases the lock when fn returns an error", func(t *testing.T) {
		// Given fn that errors out
		rt := newTestRuntime(t)
		_ = With(context.Background(), rt, "up", func() error { return errors.New("boom") })

		// When a second invocation runs against the same path
		err := With(context.Background(), rt, "apply", func() error { return nil })

		// Then the second invocation succeeds, proving the first released
		if err != nil {
			t.Fatalf("second: %v", err)
		}
	})

	t.Run("returns LockBusyError when a peer holds the lock", func(t *testing.T) {
		// Given a peer holding the lock against the same scratch path
		rt := newTestRuntime(t)
		peer := NewLocalFlockLock(filepath.Join(rt.WindsorScratchPath, stackLockFilename))
		peerInfo := LockInfo{
			ID:        "peer-id",
			Operation: "destroy",
			Mode:      Exclusive,
			Who:       "peer@host",
			Created:   time.Now().UTC(),
		}
		peerRelease, err := peer.Acquire(context.Background(), peerInfo, time.Second)
		if err != nil {
			t.Fatalf("peer acquire: %v", err)
		}
		t.Cleanup(func() { _ = peerRelease() })

		// When With runs against the same path with a short timeout
		withDefaultTimeout(t, 100*time.Millisecond)
		err = With(context.Background(), rt, "up", func() error {
			t.Fatal("fn must not run when acquire fails")
			return nil
		})

		// Then a LockBusyError surfaces with Holder populated from the peer's sidecar
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if busy.Holder == nil || busy.Holder.Who != "peer@host" || busy.Holder.Operation != "destroy" {
			t.Fatalf("expected holder peer@host/destroy, got %+v", busy.Holder)
		}
	})
}

func TestNewInfo(t *testing.T) {
	t.Run("populates operation, mode, context, PID, and a non-empty ID", func(t *testing.T) {
		// Given a runtime with a context and project root
		rt := &runtime.Runtime{ContextName: "ctx-A", ProjectRoot: "/some/root"}

		// When constructing a LockInfo
		info := NewInfo(rt, "destroy")

		// Then the diagnostic fields are populated
		if info.Operation != "destroy" {
			t.Fatalf("operation: got %q want destroy", info.Operation)
		}
		if info.Mode != Exclusive {
			t.Fatalf("mode: got %v want Exclusive", info.Mode)
		}
		if info.Context != "ctx-A" {
			t.Fatalf("context: got %q want ctx-A", info.Context)
		}
		if info.ID == "" {
			t.Fatal("expected non-empty ID")
		}
		if info.Who == "" {
			t.Fatal("expected non-empty Who")
		}
		if info.ProjectID == "" {
			t.Fatal("expected non-empty ProjectID")
		}
		if info.PID != os.Getpid() {
			t.Fatalf("expected PID=%d, got %d", os.Getpid(), info.PID)
		}
	})

	t.Run("generates distinct IDs across calls", func(t *testing.T) {
		// Given the same runtime
		rt := &runtime.Runtime{ContextName: "ctx", ProjectRoot: "/x"}

		// When constructing two LockInfos
		a := NewInfo(rt, "up")
		b := NewInfo(rt, "up")

		// Then the IDs differ
		if a.ID == b.ID {
			t.Fatalf("expected distinct IDs, both got %q", a.ID)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestHashProjectRoot(t *testing.T) {
	t.Run("returns the same hash for the same input", func(t *testing.T) {
		// Given the same path twice
		// When hashing
		// Then the results match
		if hashProjectRoot("/a/b") != hashProjectRoot("/a/b") {
			t.Fatal("expected stable hash across calls")
		}
	})

	t.Run("returns different hashes for different inputs", func(t *testing.T) {
		// Given two distinct paths
		// When hashing
		// Then the results differ
		if hashProjectRoot("/a/b") == hashProjectRoot("/a/c") {
			t.Fatal("expected distinct hashes for distinct paths")
		}
	})
}

// =============================================================================
// Test ForRuntime
// =============================================================================

func TestForRuntime(t *testing.T) {
	t.Run("returns a usable lock for a configured runtime", func(t *testing.T) {
		// Given a runtime with a scratch path
		rt := newTestRuntime(t)

		// When deriving the lock
		lock, err := ForRuntime(rt)

		// Then a usable lock is returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if lock == nil {
			t.Fatal("expected non-nil StackLock")
		}
	})

	t.Run("errors when the runtime is nil", func(t *testing.T) {
		// When deriving a lock from a nil runtime
		_, err := ForRuntime(nil)

		// Then it reports the missing runtime
		if err == nil || !strings.Contains(err.Error(), "runtime is required") {
			t.Fatalf("expected runtime-required error, got %v", err)
		}
	})

	t.Run("errors when the scratch path is empty", func(t *testing.T) {
		// Given a runtime that has not been configured yet
		rt := &runtime.Runtime{}

		// When deriving the lock
		_, err := ForRuntime(rt)

		// Then it reports the unconfigured scratch path
		if err == nil || !strings.Contains(err.Error(), "scratch path is empty") {
			t.Fatalf("expected scratch-path error, got %v", err)
		}
	})
}

// =============================================================================
// Test Inspect
// =============================================================================

func TestLocalFlockLock_Inspect(t *testing.T) {
	t.Run("returns nil when no lock is held", func(t *testing.T) {
		// Given a fresh lock path with no sidecar
		path := filepath.Join(t.TempDir(), ".stacklock")
		sl := NewLocalFlockLock(path)

		// When inspecting
		info, err := sl.Inspect(context.Background())

		// Then there is no holder and no error
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if info != nil {
			t.Fatalf("expected nil holder, got %+v", info)
		}
	})

	t.Run("returns the holder recorded in the sidecar", func(t *testing.T) {
		// Given a sidecar describing the current holder
		path := filepath.Join(t.TempDir(), ".stacklock")
		holder := newTestLockInfo()
		holder.Who = "ci@runner-9"
		holder.Operation = "bootstrap"
		holder.PID = 4242
		writeHolderInfo(path+stackLockInfoSuffix, holder)

		// When inspecting
		info, err := NewLocalFlockLock(path).Inspect(context.Background())

		// Then the recorded holder is returned
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if info == nil || info.PID != 4242 || info.Operation != "bootstrap" {
			t.Fatalf("expected populated holder, got %+v", info)
		}
	})
}

// =============================================================================
// Test ForceRelease
// =============================================================================

func TestLocalFlockLock_ForceRelease(t *testing.T) {
	// orphanedLock writes a lock file and sidecar with no process holding the flock,
	// mirroring a holder that was SIGKILL'd before it could release. Files are created
	// directly (not via Acquire) so no open fd lingers — os.Remove of an fd-held file
	// fails on Windows, and the recovery scenario is precisely a dead holder.
	orphanedLock := func(t *testing.T, info LockInfo) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), ".stacklock")
		if err := os.WriteFile(path, nil, lockInfoPerm); err != nil {
			t.Fatalf("write lock file: %v", err)
		}
		writeHolderInfo(path+stackLockInfoSuffix, info)
		return path
	}

	t.Run("removes the lock file and sidecar", func(t *testing.T) {
		// Given an orphaned lock with both files present
		path := orphanedLock(t, newTestLockInfo())

		// When force-releasing without an ID guard
		if err := NewLocalFlockLock(path).ForceRelease(context.Background(), "", "test recovery"); err != nil {
			t.Fatalf("force-release: %v", err)
		}

		// Then both the lock file and the sidecar are gone
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected lock file removed, stat err=%v", err)
		}
		if _, err := os.Stat(path + stackLockInfoSuffix); !os.IsNotExist(err) {
			t.Fatalf("expected sidecar removed, stat err=%v", err)
		}
	})

	t.Run("is a no-op when no lock files exist", func(t *testing.T) {
		// Given a path with no lock files
		path := filepath.Join(t.TempDir(), ".stacklock")

		// When force-releasing, then it succeeds (already clear)
		if err := NewLocalFlockLock(path).ForceRelease(context.Background(), "", "test"); err != nil {
			t.Fatalf("expected nil error for absent files, got %v", err)
		}
	})

	t.Run("refuses when a different holder has acquired the lock", func(t *testing.T) {
		// Given a lock now held by a holder whose ID differs from the caller's target
		current := newTestLockInfo()
		current.ID = "new-holder-id"
		path := orphanedLock(t, current)

		// When force-releasing against a stale target ID
		err := NewLocalFlockLock(path).ForceRelease(context.Background(), "stale-target-id", "test")

		// Then it refuses and leaves the files intact
		if err == nil || !strings.Contains(err.Error(), "different holder") {
			t.Fatalf("expected refusal error, got %v", err)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected lock file preserved on refusal, got %v", statErr)
		}
	})
}
