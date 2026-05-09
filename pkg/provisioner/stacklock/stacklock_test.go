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

	t.Run("surfaces the holder LockInfo in the busy error when readable", func(t *testing.T) {
		// Given a holder with distinctive fields
		path := filepath.Join(t.TempDir(), ".stacklock")
		holderInfo := newTestLockInfo()
		holderInfo.Who = "alice@laptop"
		holderInfo.Operation = "destroy"
		first := NewLocalFlockLock(path)
		release1, err := first.Acquire(context.Background(), holderInfo, time.Second)
		if err != nil {
			t.Fatalf("first acquire: %v", err)
		}
		t.Cleanup(func() { _ = release1() })

		// When a second instance times out
		second := NewLocalFlockLock(path)
		_, err = second.Acquire(context.Background(), newTestLockInfo(), 100*time.Millisecond)

		// Then the busy error names the original holder
		var busy *LockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("expected *LockBusyError, got %T: %v", err, err)
		}
		if busy.Holder == nil {
			t.Fatal("expected busy.Holder to be populated")
		}
		if busy.Holder.Who != "alice@laptop" || busy.Holder.Operation != "destroy" {
			t.Fatalf("holder mismatch: got %+v", busy.Holder)
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

func TestLocalFlockLock_StaleLockFile(t *testing.T) {
	t.Run("acquires when a lock file exists with a malformed body and no holder", func(t *testing.T) {
		// Given a stale lock file with garbage contents and no flock holder
		path := filepath.Join(t.TempDir(), ".stacklock")
		if err := os.WriteFile(path, []byte("not json {{{"), 0o600); err != nil {
			t.Fatalf("seed stale lock: %v", err)
		}

		// When acquiring
		sl := NewLocalFlockLock(path)
		release, err := sl.Acquire(context.Background(), newTestLockInfo(), time.Second)

		// Then acquisition succeeds (the body is informational, not load-bearing)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		t.Cleanup(func() { _ = release() })
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
