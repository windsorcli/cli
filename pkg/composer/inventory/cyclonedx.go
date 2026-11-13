// The CycloneDX encoder turns a SourceTree into a CycloneDX 1.6 BOM.
// It provides the BOM that windsor show artifacts prints with -o cyclonedx and that windsor bundle writes.
// Each artifact and each oci:// source is one component, and the dependencies graph tells which source pulls it.
// The BOM has no serial number and no timestamp, so the same tree always gives the same bytes.

package inventory

import (
	"bytes"
	"slices"
	"sort"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/package-url/packageurl-go"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Types
// =============================================================================

// bomBuilder collects the components and the dependency edges of one BOM.
// root is the bom-ref of the metadata component. owned marks the components of the walked tree, which
// win over components read from a source BOM. expanded marks each source that the walk has expanded.
// imported holds the edges read from source BOMs, which apply only to a component that is not owned.
type bomBuilder struct {
	root       string
	components []cdx.Component
	index      map[string]int
	owned      map[string]bool
	expanded   map[string]bool
	edges      map[string]map[string]bool
	imported   map[string]map[string]bool
}

// =============================================================================
// Public Methods
// =============================================================================

// CycloneDX returns tree as a CycloneDX 1.6 BOM in JSON. An image is a container component, a chart is
// an application component, and a source is a data component. The BOM has one component for each
// package URL, and its dependencies graph names the components that each source pulls. When name is not
// empty, the metadata names the blueprint, with name as its bom-ref, and the blueprint depends on its
// own artifacts and sources.
func CycloneDX(tree SourceTree, name, version string) ([]byte, error) {
	b := &bomBuilder{
		root:     name,
		index:    make(map[string]int),
		owned:    make(map[string]bool),
		expanded: make(map[string]bool),
		edges:    make(map[string]map[string]bool),
		imported: make(map[string]map[string]bool),
	}
	direct := b.addTree(tree)

	bom := cdx.NewBOM()
	if name != "" {
		bom.Metadata = &cdx.Metadata{Component: &cdx.Component{BOMRef: name, Type: cdx.ComponentTypeData, Name: name, Version: version}}
		b.depend(name, direct...)
	}
	components := b.sortedComponents()
	bom.Components = &components
	if dependencies := b.dependencies(name); len(dependencies) > 0 {
		bom.Dependencies = &dependencies
	}

	var buf bytes.Buffer
	encoder := cdx.NewBOMEncoder(&buf, cdx.BOMFileFormatJSON)
	encoder.SetPretty(true)
	encoder.SetEscapeHTML(false)
	if err := encoder.EncodeVersion(bom, cdx.SpecVersion1_6); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BundleBOMWriter returns the BOM writer of windsor bundle. The writer lists the artifact tags of the
// bundle, each oci:// source of the bundled blueprint, and the components of each source.
// It pulls each source that is not in the cache through builder, once for all calls, and fails when a pull fails.
func BundleBOMWriter(builder artifact.Artifact) artifact.BOMWriter {
	reader := bundleReader{builder: builder, pulled: make(map[string]string), built: make(map[string]SourceTree)}
	return func(name, version string, tags []artifact.ArtifactTag, blueprint []byte) ([]byte, error) {
		sources, err := parseSources(blueprint)
		if err != nil {
			return nil, werror.Wrap(err, "failed to read the sources of the bundled blueprint")
		}
		tree := SourceTree{Artifacts: tagArtifacts(name, tags)}
		if tree.Sources, err = reader.sourceTrees(sources, nil); err != nil {
			return nil, err
		}
		return CycloneDX(tree, name, version)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// addTree adds the sources and the artifacts of tree, and the edges from each source to what it pulls.
// It returns the bom-refs that tree pulls directly. It expands each source once, so a source that many
// sources name costs one walk. A source read from the BOM of its artifact adds what that BOM lists.
func (b *bomBuilder) addTree(tree SourceTree) []string {
	var direct []string
	for _, source := range sortTrees(tree.Sources) {
		ref := b.add(bomComponent(source.Entry), true)
		direct = append(direct, ref)
		if b.expanded[ref] {
			continue
		}
		b.expanded[ref] = true
		b.depend(ref, b.addTree(source)...)
	}
	if tree.bom != nil {
		return append(direct, b.addBOM(*tree.bom)...)
	}
	for _, e := range tree.Artifacts {
		direct = append(direct, b.add(bomComponent(e), true))
	}
	return direct
}

// addBOM adds the components and the edges of a source BOM, and returns the bom-refs that its blueprint
// pulls directly: the edges of its metadata component, or else its top-level components. It keeps only
// edges between components of that BOM, and the graph uses them only for a component that the walked
// tree does not own. A nested component gets an edge from its parent. A component with no bom-ref uses
// its package URL.
func (b *bomBuilder) addBOM(bom cdx.BOM) []string {
	local := make(map[string]bool)
	var top []string
	if bom.Components != nil {
		for _, c := range *bom.Components {
			if ref := b.addImported(c, local); ref != "" {
				top = append(top, ref)
			}
		}
	}
	root := ""
	if bom.Metadata != nil && bom.Metadata.Component != nil {
		root = bom.Metadata.Component.BOMRef
	}
	var direct []string
	hasRoot := false
	if bom.Dependencies != nil {
		for _, d := range *bom.Dependencies {
			var dependsOn []string
			if d.Dependencies != nil {
				for _, target := range *d.Dependencies {
					if local[target] {
						dependsOn = append(dependsOn, target)
					}
				}
			}
			switch {
			case root != "" && d.Ref == root:
				direct, hasRoot = append(direct, dependsOn...), true
			case local[d.Ref]:
				addEdges(b.imported, d.Ref, dependsOn...)
			}
		}
	}
	if hasRoot {
		return direct
	}
	return top
}

// addImported adds c from a source BOM, with each component nested in it and an edge from c to each one.
// It records each added bom-ref in local, and returns the bom-ref of c. It returns "" for a component with
// no bom-ref and no package URL, or whose bom-ref is the root of this BOM.
func (b *bomBuilder) addImported(c cdx.Component, local map[string]bool) string {
	var children []cdx.Component
	if c.Components != nil {
		children = *c.Components
		c.Components = nil
	}
	if c.BOMRef == "" {
		c.BOMRef = c.PackageURL
	}
	if c.BOMRef == "" || c.BOMRef == b.root {
		return ""
	}
	ref := b.add(c, false)
	local[ref] = true
	for _, child := range children {
		if childRef := b.addImported(child, local); childRef != "" {
			addEdges(b.imported, ref, childRef)
		}
	}
	return ref
}

// add adds c and returns its bom-ref. It keeps one component for each bom-ref. An owned component
// replaces a component from a source BOM. Between two owned or two read components, a chart replaces
// an image, and otherwise the first component stays.
func (b *bomBuilder) add(c cdx.Component, owned bool) string {
	if i, ok := b.index[c.BOMRef]; ok {
		replace := owned && !b.owned[c.BOMRef]
		if owned == b.owned[c.BOMRef] && typeRank(c.Type) < typeRank(b.components[i].Type) {
			replace = true
		}
		if replace {
			b.components[i] = c
			b.owned[c.BOMRef] = owned
		}
		return c.BOMRef
	}
	b.index[c.BOMRef] = len(b.components)
	b.owned[c.BOMRef] = owned
	b.components = append(b.components, c)
	return c.BOMRef
}

// depend adds an edge from ref to each bom-ref in dependsOn, except ref itself.
func (b *bomBuilder) depend(ref string, dependsOn ...string) {
	addEdges(b.edges, ref, dependsOn...)
}

// sortedComponents returns the components sorted by type, sources first, then by the order that they were added.
func (b *bomBuilder) sortedComponents() []cdx.Component {
	components := slices.Clone(b.components)
	sort.SliceStable(components, func(i, j int) bool {
		return typeRank(components[i].Type) < typeRank(components[j].Type)
	})
	return components
}

// dependencies returns the dependencies graph: one entry for root when it is not empty, and one entry
// for each component, with an empty list for a component that pulls nothing. It leaves out an edge to a
// bom-ref that is not a component. The entries and their lists are sorted, so the order of the walk does
// not change the graph.
func (b *bomBuilder) dependencies(root string) []cdx.Dependency {
	refs := make([]string, 0, len(b.components)+1)
	if root != "" {
		refs = append(refs, root)
	}
	for _, c := range b.components {
		refs = append(refs, c.BOMRef)
	}
	sort.Strings(refs)
	out := make([]cdx.Dependency, 0, len(refs))
	for _, ref := range refs {
		dependency := cdx.Dependency{Ref: ref}
		targets := b.edges[ref]
		if !b.owned[ref] {
			targets = mergeEdges(targets, b.imported[ref])
		}
		var edges []string
		for target := range targets {
			if _, ok := b.index[target]; ok {
				edges = append(edges, target)
			}
		}
		if len(edges) > 0 {
			sort.Strings(edges)
			dependency.Dependencies = &edges
		}
		out = append(out, dependency)
	}
	return out
}

// =============================================================================
// Helpers
// =============================================================================

// addEdges adds an edge from ref to each bom-ref in dependsOn, except ref itself, to graph.
func addEdges(graph map[string]map[string]bool, ref string, dependsOn ...string) {
	edges := graph[ref]
	if edges == nil {
		edges = make(map[string]bool)
		graph[ref] = edges
	}
	for _, d := range dependsOn {
		if d != ref {
			edges[d] = true
		}
	}
}

// mergeEdges returns the union of two edge sets.
func mergeEdges(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for ref := range a {
		out[ref] = true
	}
	for ref := range b {
		out[ref] = true
	}
	return out
}

// typeRank orders the component types of a BOM: a source, then a chart, then an image.
func typeRank(t cdx.ComponentType) int {
	switch t {
	case cdx.ComponentTypeData:
		return 0
	case cdx.ComponentTypeApplication:
		return 1
	default:
		return 2
	}
}

// decodeBOM returns the CycloneDX BOM in data.
func decodeBOM(data []byte) (cdx.BOM, error) {
	var bom cdx.BOM
	err := cdx.NewBOMDecoder(bytes.NewReader(data), cdx.BOMFileFormatJSON).Decode(&bom)
	return bom, err
}

// flatComponents returns components and each component nested in them, at every depth.
func flatComponents(components []cdx.Component) []cdx.Component {
	var out []cdx.Component
	for _, c := range components {
		out = append(out, c)
		if c.Components != nil {
			out = append(out, flatComponents(*c.Components)...)
		}
	}
	return out
}

// bomComponent returns the CycloneDX component of e. The package URL is also the bom-ref.
// An entry with a digest and no version has the digest as its component version.
func bomComponent(e Entry) cdx.Component {
	componentType := cdx.ComponentTypeContainer
	componentName := e.Reference
	switch e.Kind {
	case KindChart:
		componentType = cdx.ComponentTypeApplication
		componentName = e.Name
	case KindBlueprint:
		componentType = cdx.ComponentTypeData
	}
	componentVersion := e.Version
	if componentVersion == "" {
		componentVersion = e.Digest
	}
	purl := packageURL(e)
	component := cdx.Component{
		BOMRef:     purl,
		Type:       componentType,
		Name:       componentName,
		Version:    componentVersion,
		PackageURL: purl,
	}
	if hex, ok := strings.CutPrefix(e.Digest, "sha256:"); ok {
		component.Hashes = &[]cdx.Hash{{Algorithm: cdx.HashAlgoSHA256, Value: hex}}
	}
	return component
}

// packageURL returns the package URL of e. An OCI artifact is pkg:oci/<name> with repository_url and
// tag qualifiers, and the digest as the version. An HTTP or Git chart is pkg:generic/<name>@<version>
// with the repository as its download_url, because the package URL specification has no chart type.
func packageURL(e Entry) string {
	if e.Transport != TransportOCI {
		qualifiers := packageurl.QualifiersFromMap(map[string]string{"download_url": e.Reference})
		return packageurl.NewPackageURL(packageurl.TypeGeneric, "", e.Name, e.Version, qualifiers, "").ToString()
	}
	name := strings.ToLower(e.Reference[strings.LastIndex(e.Reference, "/")+1:])
	values := map[string]string{"repository_url": e.Reference}
	if e.Version != "" {
		values["tag"] = e.Version
	}
	return packageurl.NewPackageURL(packageurl.TypeOCI, "", name, e.Digest, packageurl.QualifiersFromMap(values), "").ToString()
}
