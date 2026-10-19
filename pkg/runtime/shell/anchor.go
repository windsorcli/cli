// The project anchor checks are the filesystem checks that windsor init runs before the runtime exists.
// It provides EnsureGitRepository, which requires a git repository at or above the current directory.
// It provides EnsureProjectAnchor, which writes a windsor.yaml when no project file is found.
// Without the anchor, init falls back to the global config in $HOME and changes it.

package shell

import (
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/internal/werror"
)

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
		return werror.Wrap(err, "failed to get working directory")
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
	return werror.New("windsor init must run inside a git repository.", "Run 'git init' in the project directory, then run 'windsor init' again.", nil)
}

// EnsureProjectAnchor writes a minimal windsor.yaml in the current working
// directory when no project file is found anywhere in the walk-up path. This
// anchors `windsor init` to the cwd so subsequent runtime resolution does not
// fall back to global mode and silently operate against $HOME/.config/windsor.
// If a project file already exists at or above the cwd, this is a no-op.
// Returns the directory holding the project file: cwd when this call created
// it, or the ancestor directory when one already existed there.
func EnsureProjectAnchor() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", werror.Wrap(err, "failed to get working directory")
	}
	dir := cwd
	for i := 0; i <= MaxFolderSearchDepth; i++ {
		if _, err := os.Stat(filepath.Join(dir, "windsor.yaml")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "windsor.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if err := os.WriteFile(filepath.Join(cwd, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644); err != nil { // #nosec G306
		return "", err
	}
	return cwd, nil
}
