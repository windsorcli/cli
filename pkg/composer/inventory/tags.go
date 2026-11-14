// The tag reader turns the artifact tags of each blueprint source into inventory entries.
// It provides the source roots of a blueprint, the scope of each root, and the entry of each chart and image tag.
// The project root is read first, then the cache directory of each oci:// source.
// By default it reads only the files that the composed blueprint uses.
// A source that is not oci:// gives a note, because the CLI has no cache directory for it.

package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Types
// =============================================================================

// sourceRoot is a directory that holds the files of one blueprint source.
// scope lists the paths of the source that the composed blueprint uses.
type sourceRoot struct {
	name  string
	dir   string
	scope artifact.TagScope
}

// =============================================================================
// Private Methods
// =============================================================================

// tagEntries returns the entries of the artifact tags in the project and in each oci:// source of
// blueprint. By default it reads the files that blueprint uses. When all is true, it reads whole
// sources. A source with no cache directory, or that is not oci://, gives a note. With a trace, it
// leaves out each facet tag on a value that blueprint does not use. It also returns the roots that it read.
func (l *BaseLister) tagEntries(blueprint *blueprintv1alpha1.Blueprint, opts Options) ([]Entry, []sourceRoot, []string, error) {
	roots, notes, err := l.sourceRoots(blueprint)
	if err != nil {
		return nil, nil, nil, err
	}
	if !opts.All {
		notes = append(notes, addScopes(blueprint, roots)...)
	}
	var entries []Entry
	for _, root := range roots {
		tags, rootNotes, err := scanRoot(root, opts.All)
		if err != nil {
			return nil, nil, nil, werror.Wrap(err, "failed to read the artifact tags of source %q", root.name)
		}
		for _, note := range rootNotes {
			notes = append(notes, fmt.Sprintf("Source %q: %s", root.name, note))
		}
		if !opts.All && opts.Trace != nil {
			var traceNotes []string
			tags, traceNotes = usedFacetTags(root.dir, tags, opts.Trace)
			notes = appendNewNotes(notes, traceNotes)
		}
		for _, tag := range tags {
			entries = append(entries, tagEntry(root.name, tag))
		}
	}
	return entries, roots, notes, nil
}

// sourceRoots returns the project root, named for the blueprint, and the cache directory of each
// oci:// source. The local template source is the project itself. A source that is not oci://, or
// whose cache directory is missing, gives a note.
func (l *BaseLister) sourceRoots(blueprint *blueprintv1alpha1.Blueprint) ([]sourceRoot, []string, error) {
	roots := []sourceRoot{{name: blueprint.Metadata.Name, dir: l.runtime.ProjectRoot}}
	var notes []string
	for _, source := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(source) {
			continue
		}
		if !strings.HasPrefix(source.Url, "oci://") {
			notes = append(notes, fmt.Sprintf("Source %q is not an oci:// source, so its artifact tags are not read.", source.Name))
			continue
		}
		registry, repository, tag, err := l.artifactBuilder.ParseOCIRef(source.Url)
		if err != nil {
			return nil, nil, werror.Wrap(err, "failed to parse source %q", source.Name)
		}
		dir, err := l.artifactBuilder.GetCacheDir(registry, repository, tag)
		if err != nil {
			return nil, nil, werror.Wrap(err, "failed to find the cache of source %q", source.Name)
		}
		if _, err := os.Stat(dir); err != nil {
			notes = append(notes, fmt.Sprintf("Source %q is not in the cache, so its artifact tags are not read.", source.Name))
			continue
		}
		roots = append(roots, sourceRoot{name: source.Name, dir: dir})
	}
	return roots, notes, nil
}

// =============================================================================
// Helpers
// =============================================================================

// scanRoot returns the artifact tags of root: the tags of the whole source when all is true, and the
// tags of the scope of root when all is false.
func scanRoot(root sourceRoot, all bool) ([]artifact.ArtifactTag, []string, error) {
	if all {
		tags, err := artifact.ScanTagsDir(root.dir)
		return tags, nil, err
	}
	return artifact.ScanTagsScoped(root.dir, root.scope)
}

// usedFacetTags returns tags without each tag on a facet value that trace proves is unused. It also
// returns the notes of trace for the values that it cannot resolve.
func usedFacetTags(dir string, tags []artifact.ArtifactTag, trace FacetTrace) ([]artifact.ArtifactTag, []string) {
	var used []artifact.ArtifactTag
	var notes []string
	for _, tag := range tags {
		unused, note := trace.UnusedFacetValue(filepath.Join(dir, filepath.FromSlash(tag.File)), tag.Line)
		if note != "" {
			notes = append(notes, note)
		}
		if !unused {
			used = append(used, tag)
		}
	}
	return used, notes
}

// appendNewNotes appends each note of added that notes does not hold.
func appendNewNotes(notes, added []string) []string {
	for _, note := range added {
		if !slices.Contains(notes, note) {
			notes = append(notes, note)
		}
	}
	return notes
}

// addScopes sets the scope of each root from the kustomizations and the Terraform components of
// blueprint. It leaves out an entry that is disabled. It returns a note for an entry whose source
// the blueprint does not define. A source in blueprint.Sources with no root already has a note.
func addScopes(blueprint *blueprintv1alpha1.Blueprint, roots []sourceRoot) []string {
	var notes []string
	for _, k := range blueprint.AllKustomizations() {
		if !k.Enabled.IsEnabled() {
			continue
		}
		root, note := rootFor(blueprint, roots, k.Source, "Kustomization", k.Name)
		if note != "" {
			notes = append(notes, note)
		}
		if root != nil {
			root.scope.Kustomizations = append(root.scope.Kustomizations, artifact.KustomizationScope{Name: k.Name, Dir: k.FluxPath(), Components: k.Components})
		}
	}
	for _, t := range blueprint.TerraformComponents {
		if !t.Enabled.IsEnabled() {
			continue
		}
		root, note := rootFor(blueprint, roots, t.Source, "Terraform component", t.GetID())
		if note != "" {
			notes = append(notes, note)
		}
		if root != nil {
			root.scope.Terraform = append(root.scope.Terraform, artifact.TerraformScope{Name: t.GetID(), Root: terraformRoot(blueprint, t.Source), Path: strings.ReplaceAll(t.Path, "\\", "/")})
		}
	}
	return notes
}

// rootFor returns the root of the source that an entry names. An empty name, the name of the
// blueprint, and a template source with no URL are the project root, which is the first root. It
// returns a note and no root for a name that the blueprint does not define.
func rootFor(blueprint *blueprintv1alpha1.Blueprint, roots []sourceRoot, source, kind, name string) (*sourceRoot, string) {
	if source == "" || source == blueprint.Metadata.Name || (source == "template" && !blueprintv1alpha1.HasRemoteTemplateSource(blueprint.Sources)) {
		return &roots[0], ""
	}
	for i := range roots {
		if roots[i].name == source {
			return &roots[i], ""
		}
	}
	for _, declared := range blueprint.Sources {
		if declared.Name == source {
			return nil, ""
		}
	}
	return nil, fmt.Sprintf("%s %q names the source %q, which the blueprint does not define, so its artifact tags are not read.", kind, name, source)
}

// terraformRoot returns the directory that holds the Terraform components of a source. It is the
// path prefix of the source, or terraform when the source sets none.
func terraformRoot(blueprint *blueprintv1alpha1.Blueprint, source string) string {
	for _, declared := range blueprint.Sources {
		if declared.Name == source && declared.PathPrefix != "" {
			return declared.PathPrefix
		}
	}
	return "terraform"
}

// tagEntry converts an artifact tag to an Entry. An image reference is normalized like any other
// image. A chart in an oci:// repository has the reference <repository>/<name>, and a chart in an
// HTTP repository has the repository URL as its reference.
func tagEntry(sourceName string, tag artifact.ArtifactTag) Entry {
	source := Source{Object: "source/" + sourceName, File: tag.File}
	if tag.Type == artifact.ArtifactTagImage {
		ref := parseImageReference(tag.Name)
		return Entry{Kind: KindImage, Transport: TransportOCI, Reference: ref.repository, Version: tag.Version, Digest: tag.Digest, NoReferrers: tag.NoReferrers, Source: source}
	}
	entry := Entry{Kind: KindChart, Name: tag.Name, Version: tag.Version, Source: source}
	if strings.HasPrefix(tag.Repo, "oci://") {
		entry.Transport = TransportOCI
		entry.Reference = strings.TrimSuffix(strings.TrimPrefix(tag.Repo, "oci://"), "/") + "/" + tag.Name
	} else {
		entry.Transport = TransportHTTP
		entry.Reference = strings.TrimSuffix(tag.Repo, "/")
	}
	return entry
}
