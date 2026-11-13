// The artifact manifest is the CycloneDX BOM that windsor bundle writes at the root of a blueprint artifact.
// It lists each chart and image that an artifact tag names in the bundled files.
// The artifact package scans the tags, and a BOMWriter from the composer encodes them.

package artifact

// =============================================================================
// Constants
// =============================================================================

// BOMFileName is the name of the CycloneDX BOM at the root of a blueprint artifact.
const BOMFileName = "artifact-manifest.cdx.json"

// =============================================================================
// Types
// =============================================================================

// BOMWriter returns the BOM of the artifact tags of a blueprint with the given name and version.
type BOMWriter func(name, version string, tags []ArtifactTag) ([]byte, error)
