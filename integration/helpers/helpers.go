//go:build integration
// +build integration

package helpers

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BinaryPath and RepoRoot are set by integration TestMain after building the CLI.
var (
	BinaryPath string
	RepoRoot   string
)

// FindRepoRoot walks up from dir until it finds a directory containing go.mod.
func FindRepoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", filepath.ErrBadPattern
		}
		abs = parent
	}
}

// BuildBinary runs `go build -o out ./cmd/windsor` from repo root.
func BuildBinary(repoRoot, outPath string) error {
	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/windsor")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CopyFixture copies the fixture directory at src to dst recursively. dst must not exist.
func CopyFixture(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

// RunCLI runs the built binary in dir with args and optional env (e.g. WINDSOR_CONTEXT=default).
// Returns stdout, stderr, and any run error.
func RunCLI(dir string, args []string, env []string) (stdout, stderr []byte, err error) {
	cmd := exec.Command(BinaryPath, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = outBuf.Bytes()
	stderr = errBuf.Bytes()
	if runErr != nil {
		return stdout, stderr, runErr
	}
	return stdout, stderr, nil
}

// FixturePath returns the absolute path to the named fixture under integration/fixtures/.
func FixturePath(name string) string {
	return filepath.Join(RepoRoot, "integration", "fixtures", name)
}

// envForHome returns env vars so the subprocess uses homeDir for home (Unix HOME,
// Windows USERPROFILE). Use when running the CLI so trusted-file and config paths are isolated.
// Forwards PATH so tools installed on the host (e.g. docker, terraform) are found.
func envForHome(homeDir string) []string {
	return []string{
		"HOME=" + homeDir,
		"USERPROFILE=" + homeDir,
		"PATH=" + os.Getenv("PATH"),
	}
}

// CopyFixtureOnly copies the named fixture into a temp dir and returns workDir and env
// (HOME/USERPROFILE set to a temp dir). Does not run windsor init, so the dir is not trusted.
func CopyFixtureOnly(t *testing.T, name string) (workDir string, env []string) {
	t.Helper()
	src := FixturePath(name)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	workDir = t.TempDir()
	if err := CopyFixture(src, workDir); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	env = envForHome(t.TempDir())
	return workDir, env
}

// PrepareFixture copies the named fixture into a temp dir, runs windsor init with a temp
// HOME so the dir is trusted, and returns workDir and env. Include env in RunCLI so the
// trusted check passes.
func PrepareFixture(t *testing.T, name string) (workDir string, env []string) {
	t.Helper()
	workDir, env = CopyFixtureOnly(t, name)
	stdout, stderr, err := RunCLI(workDir, []string{"init"}, env)
	if err != nil {
		t.Fatalf("windsor init: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return workDir, env
}
