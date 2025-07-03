package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// The bundler ignores .terraform directories and filters out common terraform files that should not be bundled:
// - *_override.tf and *.tf.json override files
// - *.tfstate and *.tfstate.* state files
// - *.tfvars and *.tfvars.json variable files (often contain sensitive data)
// - crash.log and crash.*.log files
// - .terraformrc and terraform.rc CLI config files
// - *.tfplan plan output files
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
			if info.Name() == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}

		if t.shouldSkipFile(info.Name()) {
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
		return artifact.AddFile(artifactPath, data, info.Mode())
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// shouldSkipFile determines if a file should be excluded from bundling.
// Files are skipped to avoid including sensitive data, temporary files, and configuration overrides.
// This includes state files, variable files, plan files, override files, and crash logs.
func (t *TerraformBundler) shouldSkipFile(filename string) bool {
	if strings.HasSuffix(filename, "_override.tf") ||
		strings.HasSuffix(filename, "_override.tf.json") ||
		filename == "override.tf" ||
		filename == "override.tf.json" {
		return true
	}

	if strings.HasSuffix(filename, ".tfstate") ||
		strings.Contains(filename, ".tfstate.") {
		return true
	}

	if strings.HasSuffix(filename, ".tfvars") ||
		strings.HasSuffix(filename, ".tfvars.json") {
		return true
	}

	if strings.HasSuffix(filename, ".tfplan") {
		return true
	}

	if filename == ".terraformrc" || filename == "terraform.rc" {
		return true
	}

	if filename == "crash.log" || strings.HasPrefix(filename, "crash.") && strings.HasSuffix(filename, ".log") {
		return true
	}

	return false
}

// Ensure TerraformBundler implements Bundler interface
var _ Bundler = (*TerraformBundler)(nil)
