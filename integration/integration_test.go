//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// TestMain locates the repo root, builds the CLI binary once, sets helpers.BinaryPath
// and helpers.RepoRoot, then runs all integration tests. Tests run the binary as a
// subprocess against fixture project trees.
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
	defer func() { _ = os.Chdir(origWd) }()

	helpers.RepoRoot = root
	binName := "windsor-integration-test-binary"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(root, "integration", binName)
	if err := helpers.BuildBinary(root, bin); err != nil {
		fmt.Fprintf(os.Stderr, "integration TestMain: build: %v\n", err)
		os.Exit(1)
	}
	helpers.BinaryPath = bin
	m.Run()
}
