// The CycloneDX encoder turns a SourceTree into a CycloneDX 1.6 BOM.
// It provides the BOM that windsor show artifacts prints with -o cyclonedx and that windsor bundle writes.
// Each artifact is one component with a package URL, and each oci:// source nests the components of its files.
// The BOM has no serial number and no timestamp, so the same tree always gives the same bytes.

package inventory

import (
	"bytes"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/package-url/packageurl-go"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Public Methods
// =============================================================================

// CycloneDX returns tree as a CycloneDX 1.6 BOM in JSON. An image is a container component, a chart is
// an application component, and a source is a data component that holds the components of the source.
// A nested component has the source names and a slash in front of its bom-ref. When name is not empty,
// the BOM metadata names the blueprint with name and version.
func CycloneDX(tree SourceTree, name, version string) ([]byte, error) {
	bom := cdx.NewBOM()
	if name != "" {
		bom.Metadata = &cdx.Metadata{Component: &cdx.Component{Type: cdx.ComponentTypeData, Name: name, Version: version}}
	}
	components := treeComponents(tree)
	bom.Components = &components

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
// bundle and nests each oci:// source of the bundled blueprint, with the components of that source.
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
// Helpers
// =============================================================================

// treeComponents returns the components of tree: a data component for each source, then a component
// for each artifact. The components of a source are its children, with the source name in front of
// each bom-ref. A source read from the BOM of its artifact gives the components of that BOM after its sources.
// Of two sources with one package URL, only the first is a component, so each bom-ref stays unique.
func treeComponents(tree SourceTree) []cdx.Component {
	components := make([]cdx.Component, 0, len(tree.Sources)+len(tree.Artifacts))
	seen := make(map[string]bool)
	for _, source := range sortTrees(tree.Sources) {
		component := bomComponent(source.Entry)
		if seen[component.BOMRef] {
			continue
		}
		seen[component.BOMRef] = true
		if children := prefixRefs(treeComponents(source), source.Name); len(children) > 0 {
			component.Components = &children
		}
		components = append(components, component)
	}
	if tree.bom != nil {
		return append(components, *tree.bom...)
	}
	for _, e := range tree.Artifacts {
		components = append(components, bomComponent(e))
	}
	return components
}

// prefixRefs returns a copy of components in which each bom-ref, at every depth, starts with prefix and a slash.
func prefixRefs(components []cdx.Component, prefix string) []cdx.Component {
	out := make([]cdx.Component, len(components))
	for i, c := range components {
		if c.BOMRef != "" {
			c.BOMRef = prefix + "/" + c.BOMRef
		}
		if c.Components != nil {
			children := prefixRefs(*c.Components, prefix)
			c.Components = &children
		}
		out[i] = c
	}
	return out
}

// decodeComponents returns the components of the CycloneDX BOM in data.
func decodeComponents(data []byte) ([]cdx.Component, error) {
	var bom cdx.BOM
	if err := cdx.NewBOMDecoder(bytes.NewReader(data), cdx.BOMFileFormatJSON).Decode(&bom); err != nil {
		return nil, err
	}
	if bom.Components == nil {
		return []cdx.Component{}, nil
	}
	return *bom.Components, nil
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
