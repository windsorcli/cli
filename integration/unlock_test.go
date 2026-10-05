//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/windsorcli/cli/integration/helpers"
)

// TestUnlock_NoLockHeld verifies that windsor unlock reports there is nothing to
// release (and exits 0) when no stack lock is present for the context.
func TestUnlock_NoLockHeld(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"unlock"}, env)
	if err != nil {
		t.Fatalf("unlock: %v\nstderr: %s", err, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "nothing to release") {
		t.Errorf("expected 'nothing to release', got:\n%s", out)
	}
}

// TestUnlock_ClearsStaleHolderInfo verifies that windsor unlock --force clears the
// holder info left behind by a holder that died without releasing: the sidecar is
// removed, the lock file stays, and the command reports success.
func TestUnlock_ClearsStaleHolderInfo(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")

	// Plant an orphaned lock: lock file + a valid holder-info sidecar naming a PID
	// that is no longer running, mirroring a SIGKILL'd holder.
	scratch := filepath.Join(dir, ".windsor", "contexts", "default")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	lockPath := filepath.Join(scratch, ".stacklock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
	sidecar := `{"id":"orphan-1","operation":"bootstrap","mode":0,"who":"ci@runner","version":"0.0.0","project_id":"p","context":"default","created":"2026-06-08T03:02:31Z","pid":8016}`
	if err := os.WriteFile(lockPath+".info", []byte(sidecar), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"unlock", "--force"}, env)
	if err != nil {
		t.Fatalf("unlock --force: %v\nstderr: %s", err, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "bootstrap") || !strings.Contains(out, "Cleared stale stack lock information") {
		t.Errorf("expected holder detail and clear confirmation, got:\n%s", out)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Errorf("expected lock file kept, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(lockPath + ".info"); !os.IsNotExist(statErr) {
		t.Errorf("expected sidecar removed, stat err=%v", statErr)
	}
}

// TestUnlock_ClearsCorruptSidecar verifies that a torn/partial holder-info sidecar
// (a holder killed mid-write) is treated as clearable debris — unlock --force warns
// that the holder info is unreadable and still removes the sidecar, rather than
// reporting "nothing to release" and leaving the operator stuck.
func TestUnlock_ClearsCorruptSidecar(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")

	scratch := filepath.Join(dir, ".windsor", "contexts", "default")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	lockPath := filepath.Join(scratch, ".stacklock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
	// A partial JSON write — what a SIGKILL mid-sidecar-write leaves behind.
	if err := os.WriteFile(lockPath+".info", []byte(`{"id":"orphan`), 0o644); err != nil {
		t.Fatalf("write corrupt sidecar: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"unlock", "--force"}, env)
	if err != nil {
		t.Fatalf("unlock --force: %v\nstderr: %s", err, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "unreadable") || !strings.Contains(out, "Cleared stale stack lock information") {
		t.Errorf("expected unreadable-holder warning and clear confirmation, got:\n%s", out)
	}
	if _, statErr := os.Stat(lockPath + ".info"); !os.IsNotExist(statErr) {
		t.Errorf("expected corrupt sidecar removed, stat err=%v", statErr)
	}
}

// TestUnlock_RefusesLiveHolder verifies that windsor unlock refuses when a running process
// holds the lock: it exits non-zero, names the holder, and leaves the lock file and the
// sidecar in place so the holder keeps its exclusive lock.
func TestUnlock_RefusesLiveHolder(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")

	scratch := filepath.Join(dir, ".windsor", "contexts", "default")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	lockPath := filepath.Join(scratch, ".stacklock")
	holder := flock.New(lockPath)
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold lock: locked=%v err=%v", locked, err)
	}
	t.Cleanup(func() { _ = holder.Unlock() })
	sidecar := `{"id":"live-1","operation":"apply","mode":0,"who":"ci@runner","version":"0.0.0","project_id":"p","context":"default","created":"2026-06-08T03:02:31Z","pid":4242}`
	if err := os.WriteFile(lockPath+".info", []byte(sidecar), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"unlock", "--force"}, env)

	if err == nil {
		t.Fatalf("expected unlock to refuse a live holder, got success\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PID=4242") || !strings.Contains(out, "apply") {
		t.Errorf("expected the holder to be named, got:\n%s", out)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Errorf("expected lock file kept, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(lockPath + ".info"); statErr != nil {
		t.Errorf("expected sidecar kept, stat err=%v", statErr)
	}
}
