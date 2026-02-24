//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

const coverPkg = "./cmd/...,./pkg/..."

// TestMain locates the repo root, builds the CLI binary once, sets helpers.BinaryPath
// and helpers.RepoRoot, then runs all integration tests. Tests run the binary as a
// subprocess against fixture project trees. When INTEGRATION_COVER_DIR is set, the
// binary is built with -cover and subprocess coverage is written there; after tests
// it is converted to COVER_OUT_PATH (or coverage_integration_subprocess.out) for merge.
func TestMain(m *testing.M) {
	origWd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration TestMain: getwd: %v\n", err)
		os.Exit(1)
	}
	root, err := helpers.FindRepoRoot(origWd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration TestMain: find repo root: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(root); err != nil {
		fmt.Fprintf(os.Stderr, "integration TestMain: chdir to root: %v\n", err)
		os.Exit(1)
	}

	helpers.RepoRoot = root
	binName := "windsor-integration-test-binary"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(root, "integration", binName)
	coverDir := os.Getenv("INTEGRATION_COVER_DIR")
	if coverDir != "" {
		if err := os.MkdirAll(coverDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "integration TestMain: mkdir GOCOVERDIR: %v\n", err)
			os.Exit(1)
		}
		if err := helpers.BuildBinaryWithCover(root, bin, coverPkg); err != nil {
			fmt.Fprintf(os.Stderr, "integration TestMain: build with cover: %v\n", err)
			os.Exit(1)
		}
		helpers.CoverDir = coverDir
	} else {
		if err := helpers.BuildBinary(root, bin); err != nil {
			fmt.Fprintf(os.Stderr, "integration TestMain: build: %v\n", err)
			os.Exit(1)
		}
	}
	helpers.BinaryPath = bin
	code := m.Run()
	if coverDir != "" {
		outPath := os.Getenv("COVER_OUT_PATH")
		if outPath == "" {
			outPath = filepath.Join(root, "coverage_integration_subprocess.out")
		}
		cov := exec.Command("go", "tool", "covdata", "textfmt", "-i="+coverDir, "-o="+outPath)
		cov.Stdout = os.Stdout
		cov.Stderr = os.Stderr
		_ = cov.Run()
	}
	_ = os.Chdir(origWd)
	os.Exit(code)
}
