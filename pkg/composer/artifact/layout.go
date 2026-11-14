// The layout file joins the source directory names with a base directory for the artifact code.
// It provides ProjectBase and the scan roots for a base directory.
// A bundle and every consumed source have an empty base. A project has the base of its root file.
// Every directory name is joined with the base, so one prefix describes both layouts.

package artifact

import (
	"path"

	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Public Methods
// =============================================================================

// ProjectBase returns the base directory of the project that resolver describes, relative to the
// project root with forward slashes. It is empty when the base is the project root.
func ProjectBase(resolver *paths.Resolver) string {
	rel := resolver.RelSlash(resolver.Base())
	if rel == "." {
		return ""
	}
	return rel
}

// TerraformDir returns the terraform directory under base, relative to the tree root.
func TerraformDir(base string) string {
	return path.Join(base, paths.TerraformDirName)
}

// =============================================================================
// Helpers
// =============================================================================

// scanRoots returns the directories under base that the tag scan reads. The contexts directory holds
// the facets and values files of a project. The _template directory holds them in a bundled source.
func scanRoots(base string) []string {
	return []string{
		path.Join(base, paths.KustomizeDirName),
		path.Join(base, paths.TerraformDirName),
		path.Join(base, paths.ContextsDirName),
		path.Join(base, paths.TemplateDirName),
	}
}

// wholeDirs returns the directories under base that the scoped scan always reads in full. The
// kustomize/crds directory is the CRD catalog. A cluster can use a catalog entry with no kustomization.
func wholeDirs(base string) []string {
	return []string{
		path.Join(base, paths.ContextsDirName),
		path.Join(base, paths.TemplateDirName),
		path.Join(base, paths.KustomizeDirName, "crds"),
	}
}
