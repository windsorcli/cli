// The tag reader turns the artifact tags of each blueprint source into inventory entries.
// It provides the source roots of a blueprint and the entry of each chart and image tag.
// The project root is read first, then the cache directory of each oci:// source.
// A source that is not oci:// gives a note, because the CLI has no cache directory for it.

package inventory

import (
	"fmt"
	"os"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Types
// =============================================================================

// sourceRoot is a directory that holds the files of one blueprint source.
type sourceRoot struct {
	name string
	dir  string
}

// =============================================================================
// Private Methods
// =============================================================================

// tagEntries returns the entries of the artifact tags in the project and in each oci:// source of
// blueprint. A source with no cache directory, or that is not oci://, gives a note.
func (l *BaseLister) tagEntries(blueprint *blueprintv1alpha1.Blueprint) ([]Entry, []string, error) {
	roots, notes, err := l.sourceRoots(blueprint)
	if err != nil {
		return nil, nil, err
	}
	var entries []Entry
	for _, root := range roots {
		tags, err := artifact.ScanTagsDir(root.dir)
		if err != nil {
			return nil, nil, werror.Wrap(err, "failed to read the artifact tags of source %q", root.name)
		}
		for _, tag := range tags {
			entries = append(entries, tagEntry(root.name, tag))
		}
	}
	return entries, notes, nil
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
