package shell

import (
	"fmt"
	"os"
	"path/filepath"
)

// The anchor.go file provides pure filesystem-walk helpers used by `windsor
// init` to verify project identity before any runtime is constructed:
//   - EnsureGitRepository asserts the cwd is inside a git repository, since
//     Windsor relies on git for trusted-directory ownership and blueprint refs.
//   - EnsureProjectAnchor writes a minimal windsor.yaml in the cwd when no
//     project file exists anywhere up the walk, preventing init from falling
//     back to the global $HOME config and silently operating against it.
// Both functions are stateless and exported so cmd/ stays a thin cobra layer.

// =============================================================================
// Public Functions
// =============================================================================

// EnsureGitRepository returns an actionable error when no .git entry exists at
// or above the current working directory. Windsor relies on git for project-
// state ownership (trusted directories, blueprint refs, terraform state
// placement); running init in a bare directory produces opaque errors
// downstream, so the missing-repo case is surfaced here with a one-line
// operator hint. The walk caps at MaxFolderSearchDepth to match GetProjectRoot.
func EnsureGitRepository() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	dir := cwd
	for i := 0; i <= MaxFolderSearchDepth; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return fmt.Errorf("windsor init must run inside a git repository. Run 'git init' first")
}

// EnsureProjectAnchor writes a minimal windsor.yaml in the current working
// directory when no project file is found anywhere in the walk-up path. This
// anchors `windsor init` to the cwd so subsequent runtime resolution does not
// fall back to global mode and silently operate against $HOME/.config/windsor.
// If a project file already exists at or above the cwd, this is a no-op.
func EnsureProjectAnchor() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	dir := cwd
	for i := 0; i <= MaxFolderSearchDepth; i++ {
		if _, err := os.Stat(filepath.Join(dir, "windsor.yaml")); err == nil {
			return nil
		}
		if _, err := os.Stat(filepath.Join(dir, "windsor.yml")); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// windsor.yaml is user-readable project config, 0644 matches the project
	// convention used by typed_source.EnsureRoot.
	return os.WriteFile(filepath.Join(cwd, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644) // #nosec G306
}
