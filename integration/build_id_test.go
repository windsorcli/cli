//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

func TestBuildID_ReturnsEmptyWhenNoFile(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"build-id"}, env)
	if err != nil {
		t.Fatalf("build-id: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(string(stdout)) != "" {
		t.Errorf("expected empty build ID when no file, got %q", stdout)
	}
}

func TestBuildID_NewGeneratesFormat(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"build-id", "--new"}, env)
	if err != nil {
		t.Fatalf("build-id --new: %v\nstderr: %s", err, stderr)
	}
	out := strings.TrimSpace(string(stdout))
	if out == "" {
		t.Fatal("expected non-empty build ID from --new")
	}
	re := regexp.MustCompile(`^\d{6}\.\d{3}\.\d+$`)
	if !re.MatchString(out) {
		t.Errorf("expected build ID format YYMMDD.XXX.N, got %q", out)
	}
}

func TestBuildID_GetAfterNew(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, _, err := helpers.RunCLI(dir, []string{"build-id", "--new"}, env)
	if err != nil {
		t.Fatalf("build-id --new: %v", err)
	}
	stdout, stderr, err := helpers.RunCLI(dir, []string{"build-id"}, env)
	if err != nil {
		t.Fatalf("build-id: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(string(stdout)) == "" {
		t.Error("expected build ID to be printed after previous --new")
	}
	if _, err := os.Stat(filepath.Join(dir, ".windsor", ".build-id")); err != nil {
		t.Errorf("expected .windsor/.build-id to exist: %v", err)
	}
}

func TestBuildID_FailsWhenBuildIDUnreadable(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	windsorDir := filepath.Join(dir, ".windsor")
	if err := os.MkdirAll(windsorDir, 0750); err != nil {
		t.Fatalf("create .windsor: %v", err)
	}
	buildIDPath := filepath.Join(windsorDir, ".build-id")
	if err := os.MkdirAll(buildIDPath, 0750); err != nil {
		t.Fatalf("create .build-id as dir: %v", err)
	}
	_, stderr, err := helpers.RunCLI(dir, []string{"build-id"}, env)
	if err == nil {
		t.Fatal("expected build-id to fail when .build-id is unreadable (dir)")
	}
	if !strings.Contains(string(stderr), "build ID") && !strings.Contains(string(stderr), "failed") {
		t.Errorf("expected error about build ID, got stderr: %s", stderr)
	}
}
