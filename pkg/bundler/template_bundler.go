package bundler

import (
	"fmt"
	"os"
	"path/filepath"
)

// The TemplateBundler handles bundling of jsonnet templates and related template files.
// It copies template files from the contexts/_template directory to the artifact build
// directory. The TemplateBundler ensures that all template dependencies are properly
// bundled for distribution with the artifact.

// =============================================================================
// Types
// =============================================================================

// TemplateBundler handles bundling of template files
type TemplateBundler struct {
	BaseBundler
}

// =============================================================================
// Constructor
// =============================================================================

// NewTemplateBundler creates a new TemplateBundler instance
func NewTemplateBundler() *TemplateBundler {
	return &TemplateBundler{
		BaseBundler: *NewBaseBundler(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle adds template files from contexts/_template directory to the artifact by recursively walking the directory tree.
// It validates that the templates directory exists, then walks through all files preserving the directory structure.
// Each file is read and added to the artifact with the path prefix "_template/" to maintain organization.
// Directories are skipped and only regular files are processed for bundling.
func (t *TemplateBundler) Bundle(artifact Artifact) error {
	templatesSource := filepath.Join("contexts", "_template")

	if _, err := t.shims.Stat(templatesSource); os.IsNotExist(err) {
		return fmt.Errorf("templates directory not found: %s", templatesSource)
	}

	return t.shims.Walk(templatesSource, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := t.shims.FilepathRel(templatesSource, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		data, err := t.shims.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", path, err)
		}

		artifactPath := filepath.Join("_template", relPath)
		return artifact.AddFile(artifactPath, data)
	})
}

// Ensure TemplateBundler implements Bundler interface
var _ Bundler = (*TemplateBundler)(nil)
