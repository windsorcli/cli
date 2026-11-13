// The source tree is the artifacts of a blueprint, grouped by the oci:// source that declares them.
// It provides the nesting of the CycloneDX BOM, in which each source holds the artifacts of its files.
// The bundle reader builds the tree from the BOM in each source artifact, or from a tag scan of an older artifact.
// The Lister builds the tree from the cached _template/blueprint.yaml of each source.

package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Constants
// =============================================================================

// maxSourceDepth is the deepest nesting of oci:// sources that windsor bundle reads.
const maxSourceDepth = 16

// =============================================================================
// Types
// =============================================================================

// SourceTree is the artifacts of one blueprint source and the trees of the oci:// sources that it names.
// The root of a tree is the blueprint itself, with no name and no entry.
type SourceTree struct {
	Name      string
	Entry     Entry
	Artifacts []Entry
	Sources   []SourceTree
	bom       *[]cdx.Component
}

// bundleReader builds the source trees of windsor bundle and pulls each source that is not in the cache.
// pulled holds the cache directory of each URL that it pulled, and built holds the tree of each URL.
type bundleReader struct {
	builder artifact.Artifact
	pulled  map[string]string
	built   map[string]SourceTree
}

// sourceList is the part of a blueprint file that names its sources.
type sourceList struct {
	Sources []struct {
		Name string `yaml:"name"`
		Url  string `yaml:"url"`
	} `yaml:"sources"`
}

// =============================================================================
// Private Methods
// =============================================================================

// sourceTrees returns a tree for each oci:// source in sources. A source whose artifact holds a BOM
// gets the components of that BOM, and a tree for each of its own sources that the BOM does not hold.
// Another source gets the tags of the whole cached source and the trees of its own sources. A source
// whose URL is in path gets no children, so a loop stops. It reuses the tree of a URL that it built
// before, and fails when the nesting is deeper than maxSourceDepth.
func (r bundleReader) sourceTrees(sources []blueprintv1alpha1.Source, path []string) ([]SourceTree, error) {
	var trees []SourceTree
	for _, source := range ociSources(sources) {
		tree := SourceTree{Name: source.Name, Entry: sourceEntry(source)}
		if slices.Contains(path, source.Url) {
			trees = append(trees, tree)
			continue
		}
		if built, ok := r.built[source.Url]; ok {
			built.Name = source.Name
			trees = append(trees, built)
			continue
		}
		if len(path) >= maxSourceDepth {
			return nil, fmt.Errorf("source %q is nested more than %d sources deep", source.Name, maxSourceDepth)
		}
		dir, err := r.pull(source)
		if err != nil {
			return nil, err
		}
		bom, err := artifact.ReadCachedBOM(dir)
		if err != nil {
			return nil, werror.Wrap(err, "failed to read the BOM of source %q", source.Name)
		}
		nested, err := readSources(dir)
		if err != nil {
			return nil, werror.Wrap(err, "failed to read the sources of source %q", source.Name)
		}
		if bom != nil {
			components, err := decodeComponents(bom)
			if err != nil {
				return nil, werror.Wrap(err, "failed to decode the BOM of source %q", source.Name)
			}
			tree.bom = &components
			nested = sourcesNotIn(nested, components)
		} else {
			tags, err := artifact.ScanTagsDir(dir)
			if err != nil {
				return nil, werror.Wrap(err, "failed to read the artifact tags of source %q", source.Name)
			}
			tree.Artifacts = tagArtifacts(source.Name, tags)
		}
		if tree.Sources, err = r.sourceTrees(nested, append(slices.Clone(path), source.Url)); err != nil {
			return nil, err
		}
		r.built[source.Url] = tree
		trees = append(trees, tree)
	}
	return trees, nil
}

// pull puts source in the OCI cache, when it is not there yet, and returns its cache directory.
// It pulls each URL once for the life of r.
func (r bundleReader) pull(source blueprintv1alpha1.Source) (string, error) {
	if dir, ok := r.pulled[source.Url]; ok {
		return dir, nil
	}
	if _, err := r.builder.Pull([]string{source.Url}); err != nil {
		return "", werror.Wrap(err, "failed to pull source %q from %s", source.Name, source.Url)
	}
	registry, repository, tag, err := r.builder.ParseOCIRef(source.Url)
	if err != nil {
		return "", werror.Wrap(err, "failed to parse source %q", source.Name)
	}
	dir, err := r.builder.GetCacheDir(registry, repository, tag)
	if err != nil {
		return "", werror.Wrap(err, "failed to find the cache of source %q", source.Name)
	}
	r.pulled[source.Url] = dir
	return dir, nil
}

// sourceTree returns the tree of blueprint from the tag entries of each root. A source is a child of each
// source whose cached _template/blueprint.yaml names it, and a source that no other source names is at the
// top. A source that the walk does not reach from the top, as in a loop, is also at the top.
func (l *BaseLister) sourceTree(blueprint *blueprintv1alpha1.Blueprint, roots []sourceRoot, entries []Entry) (SourceTree, []string) {
	byObject := make(map[string][]Entry)
	for _, e := range entries {
		if e.Kind != KindBlueprint {
			byObject[e.Source.Object] = append(byObject[e.Source.Object], e)
		}
	}
	byName := make(map[string]blueprintv1alpha1.Source)
	var order []string
	for _, source := range ociSources(blueprint.Sources) {
		if _, ok := byName[source.Name]; !ok {
			byName[source.Name] = source
			order = append(order, source.Name)
		}
	}
	children := make(map[string][]string)
	named := make(map[string]bool)
	var notes []string
	for _, root := range roots[1:] {
		nested, err := readSources(root.dir)
		if err != nil {
			notes = append(notes, fmt.Sprintf("Source %q: the BOM does not nest its sources, because %v.", root.name, err))
			continue
		}
		for _, source := range ociSources(nested) {
			if _, ok := byName[source.Name]; ok && source.Name != root.name && !slices.Contains(children[root.name], source.Name) {
				children[root.name] = append(children[root.name], source.Name)
				named[source.Name] = true
			}
		}
	}

	visited := make(map[string]bool)
	var build func(name string, path []string) SourceTree
	build = func(name string, path []string) SourceTree {
		visited[name] = true
		tree := SourceTree{Name: name, Entry: sourceEntry(byName[name]), Artifacts: groupArtifacts(byObject["source/"+name])}
		if slices.Contains(path, name) {
			return tree
		}
		for _, child := range children[name] {
			tree.Sources = append(tree.Sources, build(child, append(slices.Clone(path), name)))
		}
		return tree
	}
	tree := SourceTree{Artifacts: groupArtifacts(byObject["source/"+blueprint.Metadata.Name])}
	for _, name := range order {
		if !named[name] {
			tree.Sources = append(tree.Sources, build(name, nil))
		}
	}
	for _, name := range order {
		if !visited[name] {
			tree.Sources = append(tree.Sources, build(name, nil))
		}
	}
	return tree, notes
}

// =============================================================================
// Helpers
// =============================================================================

// readSources returns the sources that _template/blueprint.yaml in dir names. A missing file names no sources.
func readSources(dir string) ([]blueprintv1alpha1.Source, error) {
	data, err := os.ReadFile(filepath.Join(dir, "_template", "blueprint.yaml")) // #nosec G304 -- blueprint file in the OCI cache directory of a source
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSources(data)
}

// parseSources returns the name and URL of each source that the blueprint file in data names. It reads
// no other field, so an expression in another field does not fail the parse. Empty data names no sources.
func parseSources(data []byte) ([]blueprintv1alpha1.Source, error) {
	var list sourceList
	if len(data) == 0 {
		return nil, nil
	}
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	sources := make([]blueprintv1alpha1.Source, 0, len(list.Sources))
	for _, s := range list.Sources {
		sources = append(sources, blueprintv1alpha1.Source{Name: s.Name, Url: s.Url})
	}
	return sources, nil
}

// sourcesNotIn returns the oci:// sources in sources that have no data component in components.
// A BOM that does not nest the sources of its blueprint thus gets a tree for each of them.
func sourcesNotIn(sources []blueprintv1alpha1.Source, components []cdx.Component) []blueprintv1alpha1.Source {
	held := make(map[string]bool)
	for _, c := range components {
		if c.Type == cdx.ComponentTypeData {
			held[c.PackageURL] = true
		}
	}
	var out []blueprintv1alpha1.Source
	for _, source := range ociSources(sources) {
		if !held[packageURL(sourceEntry(source))] {
			out = append(out, source)
		}
	}
	return out
}

// ociSources returns the sources in sources that have a name and an oci:// URL.
func ociSources(sources []blueprintv1alpha1.Source) []blueprintv1alpha1.Source {
	var out []blueprintv1alpha1.Source
	for _, source := range sources {
		if source.Name != "" && strings.HasPrefix(source.Url, "oci://") {
			out = append(out, source)
		}
	}
	return out
}

// sourceEntry returns the blueprint entry of an oci:// source, with the tag and the digest of its URL.
func sourceEntry(source blueprintv1alpha1.Source) Entry {
	ref := parseImageReference(strings.TrimPrefix(source.Url, "oci://"))
	return Entry{
		Kind:      KindBlueprint,
		Transport: TransportOCI,
		Reference: ref.repository,
		Version:   ref.tag,
		Digest:    ref.digest,
		Source:    Source{Object: "source/" + source.Name},
	}
}

// tagArtifacts returns the entries of the tags of the source sourceName, unique and sorted.
func tagArtifacts(sourceName string, tags []artifact.ArtifactTag) []Entry {
	sorted := slices.Clone(tags)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})
	entries := make([]Entry, 0, len(sorted))
	for _, tag := range sorted {
		entries = append(entries, tagEntry(sourceName, tag))
	}
	return groupArtifacts(entries)
}

// groupArtifacts returns a sorted copy of entries with one entry for each artifact.
func groupArtifacts(entries []Entry) []Entry {
	return sortEntries(dedupeEntries(slices.Clone(entries)))
}

// sortTrees returns a copy of trees sorted by the reference, version and name of each source.
func sortTrees(trees []SourceTree) []SourceTree {
	sorted := slices.Clone(trees)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Entry.Reference != b.Entry.Reference {
			return a.Entry.Reference < b.Entry.Reference
		}
		if a.Entry.Version != b.Entry.Version {
			return a.Entry.Version < b.Entry.Version
		}
		return a.Name < b.Name
	})
	return sorted
}
