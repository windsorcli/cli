package artifact

import (
	"fmt"
	"os"
	"path/filepath"
)

// The KustomizeBundler handles bundling of kustomize manifests and related files.
// It copies all files from the kustomize directory to the artifact build directory.
// The KustomizeBundler ensures that all kustomize resources are properly bundled
// for distribution with the artifact for use with Flux OCIRegistry.

// =============================================================================
// Types
// =============================================================================

// KustomizeBundler handles bundling of kustomize files
type KustomizeBundler struct {
	BaseBundler
}

// =============================================================================
// Constructor
// =============================================================================

// NewKustomizeBundler creates a new KustomizeBundler instance
func NewKustomizeBundler() *KustomizeBundler {
	return &KustomizeBundler{
		BaseBundler: *NewBaseBundler(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle adds all files from kustomize directory to the artifact by recursively walking the directory tree.
// It checks if the kustomize directory exists and returns nil if not found (graceful handling).
// If the directory exists, it walks through all files preserving the directory structure.
// Each file is read and added to the artifact maintaining the original kustomize path structure.
// Directories are skipped and only regular files are processed for bundling.
func (k *KustomizeBundler) Bundle(artifact Artifact) error {
	kustomizeSource := "kustomize"

	if _, err := k.shims.Stat(kustomizeSource); os.IsNotExist(err) {
		return nil
	}

	return k.shims.Walk(kustomizeSource, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := k.shims.FilepathRel(kustomizeSource, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		data, err := k.shims.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read kustomize file %s: %w", path, err)
		}

		artifactPath := "kustomize/" + filepath.ToSlash(relPath)
		return artifact.AddFile(artifactPath, data, info.Mode())
	})
}

// Ensure KustomizeBundler implements Bundler interface
var _ Bundler = (*KustomizeBundler)(nil)
