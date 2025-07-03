package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

// The TerraformBundler handles bundling of terraform manifests and related files.
// It copies all files from the terraform directory to the artifact build directory.
// The TerraformBundler ensures that all terraform resources are properly bundled
// for distribution with the artifact for infrastructure deployment.

// =============================================================================
// Types
// =============================================================================

// TerraformBundler handles bundling of terraform files
type TerraformBundler struct {
	BaseBundler
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformBundler creates a new TerraformBundler instance
func NewTerraformBundler() *TerraformBundler {
	return &TerraformBundler{
		BaseBundler: *NewBaseBundler(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle adds all files from terraform directory to the artifact by recursively walking the directory tree.
// It checks if the terraform directory exists and returns nil if not found (graceful handling).
// If the directory exists, it walks through all files preserving the directory structure.
// Each file is read and added to the artifact maintaining the original terraform path structure.
// Directories are skipped and only regular files are processed for bundling.
func (t *TerraformBundler) Bundle(artifact Artifact) error {
	terraformSource := "terraform"

	if _, err := t.shims.Stat(terraformSource); os.IsNotExist(err) {
		return nil
	}

	return t.shims.Walk(terraformSource, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := t.shims.FilepathRel(terraformSource, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		data, err := t.shims.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read terraform file %s: %w", path, err)
		}

		artifactPath := "terraform/" + filepath.ToSlash(relPath)
		return artifact.AddFile(artifactPath, data)
	})
}

// Ensure TerraformBundler implements Bundler interface
var _ Bundler = (*TerraformBundler)(nil)
