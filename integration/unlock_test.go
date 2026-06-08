//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// TestUnlock_ReleasesOrphanedLock verifies that windsor unlock --force clears a
// stuck lock left behind by a holder that died without releasing: the lock file
// and its holder-info sidecar are removed and the command reports success.
func TestUnlock_ReleasesOrphanedLock(t *testing.T) {
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
	// The holder is named, and the release is reported.
	if !strings.Contains(out, "bootstrap") || !strings.Contains(out, "Released stack lock") {
		t.Errorf("expected holder detail and release confirmation, got:\n%s", out)
	}
	// Both lock files are gone afterward.
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Errorf("expected lock file removed, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(lockPath + ".info"); !os.IsNotExist(statErr) {
		t.Errorf("expected sidecar removed, stat err=%v", statErr)
	}
}
