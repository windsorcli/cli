// The manifest read is the --all step of the Lister.
// It provides the blueprint-wide artifact set from the artifact-manifest.yaml of each source.
// The set covers everything a blueprint could install, regardless of the context.
// It keeps the docker and helm entries and leaves out release assets and Git tags.

package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Private Methods
// =============================================================================

// listFromManifests returns the entries of the artifact-manifest.yaml in the root of each source.
// An oci:// source reads from the OCI cache, and the blueprint's own source reads from the project
// root. A source with no manifest gives a note.
func (l *BaseLister) listFromManifests(blueprint *blueprintv1alpha1.Blueprint, result *Result) ([]Entry, error) {
	roots := map[string]string{blueprint.Metadata.Name: l.runtime.ProjectRoot}
	order := []string{blueprint.Metadata.Name}
	for _, source := range blueprint.Sources {
		if !strings.HasPrefix(source.Url, "oci://") {
			continue
		}
		registry, repository, tag, err := l.artifactBuilder.ParseOCIRef(source.Url)
		if err != nil {
			return nil, werror.Wrap(err, "failed to parse source %q", source.Name)
		}
		dir, err := l.artifactBuilder.GetCacheDir(registry, repository, tag)
		if err != nil {
			return nil, werror.Wrap(err, "failed to find the cache of source %q", source.Name)
		}
		if _, exists := roots[source.Name]; !exists {
			order = append(order, source.Name)
		}
		roots[source.Name] = dir
	}

	var entries []Entry
	for _, name := range order {
		path := filepath.Join(roots[name], artifact.ManifestFileName)
		data, err := l.shims.ReadFile(path)
		if os.IsNotExist(err) {
			result.Notes = append(result.Notes, fmt.Sprintf("Source %q has no %s.", name, artifact.ManifestFileName))
			continue
		}
		if err != nil {
			return nil, werror.Wrap(err, "failed to read %s of source %q", artifact.ManifestFileName, name)
		}
		var manifest artifact.ArtifactManifest
		if err := l.shims.YamlUnmarshal(data, &manifest); err != nil {
			return nil, werror.Wrap(err, "failed to parse %s of source %q", artifact.ManifestFileName, name)
		}
		for _, m := range manifest.Artifacts {
			if entry, ok := manifestEntry(m); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries, nil
}

// =============================================================================
// Helpers
// =============================================================================

// manifestEntry converts an artifact-manifest.yaml entry to an Entry. A docker entry is an image,
// and a helm entry is a chart in its helmRepo. Every other type returns false.
func manifestEntry(m artifact.ManifestEntry) (Entry, bool) {
	source := Source{File: m.Source.File}
	switch m.Type {
	case artifact.ArtifactTypeDocker:
		ref := parseImageReference(m.Reference)
		digest := m.Digest
		if digest == "" {
			digest = ref.digest
		}
		return Entry{Kind: KindImage, Transport: TransportOCI, Reference: ref.repository, Version: ref.tag, Digest: digest, Source: source}, true
	case artifact.ArtifactTypeHelm:
		entry := Entry{Kind: KindChart, Name: m.Reference, Version: m.Version, Source: source}
		if strings.HasPrefix(m.Repository, "oci://") {
			entry.Transport = TransportOCI
			entry.Reference = strings.TrimSuffix(strings.TrimPrefix(m.Repository, "oci://"), "/") + "/" + m.Reference
		} else {
			entry.Transport = TransportHTTP
			entry.Reference = strings.TrimSuffix(m.Repository, "/")
		}
		return entry, true
	}
	return Entry{}, false
}
