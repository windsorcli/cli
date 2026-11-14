// The Layout names the source directories of a tree that the artifact code reads or writes.
// It provides ArchiveLayout, the fixed layout of a bundle, and ProjectLayout, the layout of a project.
// A bundle always uses ArchiveLayout, so every consumer of an artifact reads one layout.
// A project uses the directories that its root file configures.

package artifact

import "github.com/windsorcli/cli/pkg/runtime/paths"

// =============================================================================
// Constants
// =============================================================================

const (
	archiveKustomizeDir = "kustomize"
	archiveTerraformDir = "terraform"
	archiveContextsDir  = "contexts"
	archiveTemplateDir  = "_template"
)

// =============================================================================
// Types
// =============================================================================

// Layout holds the source directories of a tree. Each is relative to the tree root with forward slashes.
// An empty field means the tree has no such directory.
type Layout struct {
	Kustomize string
	Terraform string
	Contexts  string
	Template  string
}

// ArchiveLayout is the fixed layout of a bundle and of every source that a project consumes.
var ArchiveLayout = Layout{
	Kustomize: archiveKustomizeDir,
	Terraform: archiveTerraformDir,
	Contexts:  archiveContextsDir,
	Template:  archiveTemplateDir,
}

// =============================================================================
// Constructor
// =============================================================================

// ProjectLayout returns the layout of the project that resolver describes. It has no Template entry,
// because the template directory sits inside the contexts directory.
func ProjectLayout(resolver *paths.Resolver) Layout {
	return Layout{
		Kustomize: resolver.RelSlash(resolver.Kustomize()),
		Terraform: resolver.RelSlash(resolver.Terraform()),
		Contexts:  resolver.RelSlash(resolver.Contexts()),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Roots returns the directories that the tag scan reads, in a fixed order. It skips an empty entry.
func (l Layout) Roots() []string {
	var roots []string
	for _, dir := range []string{l.Kustomize, l.Terraform, l.Contexts, l.Template} {
		if dir != "" {
			roots = append(roots, dir)
		}
	}
	return roots
}

// WholeDirs returns the directories that the scoped scan always reads in full. The kustomize/crds
// directory is the CRD catalog. A cluster can use a catalog entry with no kustomization.
func (l Layout) WholeDirs() []string {
	var dirs []string
	for _, dir := range []string{l.Contexts, l.Template} {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	if l.Kustomize != "" {
		dirs = append(dirs, l.Kustomize+"/crds")
	}
	return dirs
}
