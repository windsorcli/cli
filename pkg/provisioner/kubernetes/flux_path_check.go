// The flux path check finds kustomization paths and components that do not exist in the project.
// It resolves each kustomization with FluxKustomization, so the check agrees with the path that Flux reads.
// It also finds kustomize directories under contexts/, which no kustomization can reach.

package kubernetes

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Public Methods
// =============================================================================

// CheckKustomizePaths returns one message for each kustomization of bp whose directory or component
// directory is missing from the project. It checks only kustomizations that read the project itself
// (the default source, a GitRepository named bp.Metadata.Name). It skips every other source, because
// a remote OCI or Git source holds its own files. The skipped list names the kustomizations it skipped.
// projectRoot is the project root and basePrefix is the base directory relative to it, as in FluxKustomization.
// An empty component is a conditional slot with no component, and the check ignores it.
func CheckKustomizePaths(bp *blueprintv1alpha1.Blueprint, projectRoot, basePrefix string) (problems []string, skipped []string) {
	defaultSourceName := bp.Metadata.Name
	for _, k := range bp.AllKustomizations() {
		k := k
		flux := FluxKustomization(&k, basePrefix, "", defaultSourceName, bp.Sources, constants.GitopsMode(""), bp.ConfigMaps)
		if flux.Spec.SourceRef.Kind != "GitRepository" || flux.Spec.SourceRef.Name != defaultSourceName {
			skipped = append(skipped, fmt.Sprintf("kustomization %q reads source %q", k.Name, flux.Spec.SourceRef.Name))
			continue
		}
		sourceName := flux.Spec.SourceRef.Name
		if sourceName == "" {
			sourceName = "project"
		}
		dir := filepath.Join(projectRoot, filepath.FromSlash(flux.Spec.Path))
		if !isDir(dir) {
			problems = append(problems, fmt.Sprintf(
				"kustomization %q: %s not found in source %q (resolved against %s); manifests belong in <project>/%s",
				k.Name, flux.Spec.Path, sourceName, projectRoot, path.Join(basePrefix, "kustomize")))
			continue
		}
		for _, component := range flux.Spec.Components {
			if component == "" {
				continue
			}
			if !isDir(filepath.Join(dir, filepath.FromSlash(component))) {
				problems = append(problems, fmt.Sprintf(
					"kustomization %q: component %q not found at %s/%s in source %q (resolved against %s)",
					k.Name, component, flux.Spec.Path, component, sourceName, projectRoot))
			}
		}
	}
	return problems, skipped
}

// FindMisplacedKustomizeDirs returns the kustomize directories under <base>/contexts, relative to the
// project root. Flux reads <base>/kustomize only, so manifests under contexts/_template/kustomize
// never resolve. It returns nil when contexts/ does not exist.
func FindMisplacedKustomizeDirs(projectRoot, basePrefix string) []string {
	contexts := filepath.Join(projectRoot, filepath.FromSlash(basePrefix), "contexts")
	var found []string
	_ = filepath.WalkDir(contexts, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == "kustomize" {
			if rel, relErr := filepath.Rel(projectRoot, p); relErr == nil {
				found = append(found, filepath.ToSlash(rel))
			}
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// MisplacedKustomizeMessage formats the warning for a kustomize directory that Flux cannot reach.
func MisplacedKustomizeMessage(dir, basePrefix string) string {
	return fmt.Sprintf("%s is not reachable by any kustomization; move its manifests to <project>/%s",
		dir, strings.TrimPrefix(path.Join(basePrefix, "kustomize"), "/"))
}

// =============================================================================
// Helpers
// =============================================================================

// isDir reports whether p is an existing directory.
func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
