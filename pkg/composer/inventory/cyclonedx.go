// The CycloneDX encoder turns an Inventory into a CycloneDX 1.6 BOM.
// It provides the BOM that windsor show artifacts prints with -o cyclonedx and that windsor bundle writes.
// Each artifact is one component with a package URL, and a digest is a SHA-256 hash.
// The BOM has no serial number and no timestamp, so the same inventory always gives the same bytes.

package inventory

import (
	"bytes"
	"sort"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/package-url/packageurl-go"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Public Methods
// =============================================================================

// CycloneDX returns inv as a CycloneDX 1.6 BOM in JSON. An image is a container component, a chart is
// an application component, and a blueprint is a data component. An OCI artifact has a pkg:oci package
// URL, with the digest as the version when the entry has one. An HTTP chart has a pkg:generic package
// URL with the repository as its download_url. Each package URL is a unique bom-ref, so CycloneDX keeps
// the first entry for a package URL and drops the next ones. When name is not empty, the BOM metadata
// names the blueprint with name and version.
func CycloneDX(inv Inventory, name, version string) ([]byte, error) {
	bom := cdx.NewBOM()
	if name != "" {
		bom.Metadata = &cdx.Metadata{Component: &cdx.Component{Type: cdx.ComponentTypeData, Name: name, Version: version}}
	}
	components := make([]cdx.Component, 0, len(inv.Artifacts))
	seen := make(map[string]bool)
	for _, e := range inv.Artifacts {
		component := bomComponent(e)
		if seen[component.BOMRef] {
			continue
		}
		seen[component.BOMRef] = true
		components = append(components, component)
	}
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

// BundleBOM returns the CycloneDX BOM of the artifact tags of one blueprint source. It gives the same
// components as CycloneDX for the inventory of those tags. The artifact builder of windsor bundle
// calls it with the name and version of the blueprint.
func BundleBOM(name, version string, tags []artifact.ArtifactTag) ([]byte, error) {
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].File != tags[j].File {
			return tags[i].File < tags[j].File
		}
		return tags[i].Line < tags[j].Line
	})
	entries := make([]Entry, 0, len(tags))
	for _, tag := range tags {
		entries = append(entries, tagEntry(name, tag))
	}
	inv := Inventory{Version: InventoryVersion, Artifacts: sortEntries(dedupeEntries(entries))}
	return CycloneDX(inv, name, version)
}

// =============================================================================
// Helpers
// =============================================================================

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
