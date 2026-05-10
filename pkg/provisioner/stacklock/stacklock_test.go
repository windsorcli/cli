package stacklock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

	t.Run("returns a busy error with nil Holder for the local-flock backend", func(t *testing.T) {
		// Given a held lock
		path := filepath.Join(t.TempDir(), ".stacklock")
		first := NewLocalFlockLock(path)
		release1, err := first.Acquire(context.Background(), newTestLockInfo(), time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		t.Cleanup(func() { _ = release1() })

		// When a second instance times out
		second := NewLocalFlockLock(path)
		_, err = second.Acquire(context.Background(), newTestLockInfo(), 100*time.Millisecond)

		// Then a *LockBusyError surfaces with Path set and Holder nil
		// (this backend does not persist holder identity into the lock file)
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if busy.Holder != nil {
			t.Fatalf("expected nil Holder for local-flock backend, got %+v", busy.Holder)
		}
		if busy.Path != path {
			t.Fatalf("expected Path %q, got %q", path, busy.Path)
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

	t.Run("includes who and operation when the holder is known", func(t *testing.T) {
		// Given a busy error with a populated holder
		info := newTestLockInfo()
		info.Who = "ci@runner-7"
		info.Operation = "apply"
		e := &LockBusyError{Path: "/tmp/x.stacklock", Holder: &info}

		// When formatting
		msg := e.Error()

		// Then both fields appear
		for _, want := range []string{"ci@runner-7", "apply"} {
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

		// Then a LockBusyError surfaces (Holder is nil for the local-flock backend)
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
	})
}

func TestNewInfo(t *testing.T) {
	t.Run("populates operation, mode, context and a non-empty ID", func(t *testing.T) {
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
