package artifact

// The ArtifactManifest is a declarative inventory of every external artifact a
// Windsor blueprint depends on — container images, Helm charts, OCI bundles,
// and other versioned references. It is produced by scanning Renovate
// annotations across the blueprint source tree and bundled into the blueprint
// OCI artifact so downstream consumers can hydrate a local mirror without
// re-evaluating adaptive composition logic or rendering Helm charts.

// =============================================================================
// Constants
// =============================================================================

// ManifestVersion is the current schema version for ArtifactManifest documents.
const ManifestVersion = "v1alpha1"

// ManifestFileName is the canonical filename for the manifest inside a
// blueprint OCI bundle and at the blueprint source root.
const ManifestFileName = "artifact-manifest.yaml"

// ArtifactType classifies an artifact by the semantic role it plays in the
// blueprint. It is derived from the Renovate datasource that surfaced it and
// is independent of transport — a Helm chart is a Helm chart whether it ships
// over HTTP or OCI.
type ArtifactType string

const (
	// ArtifactTypeDocker identifies a container image (Renovate datasource
	// "docker").
	ArtifactTypeDocker ArtifactType = "docker"

	// ArtifactTypeHelm identifies a Helm chart (Renovate datasource "helm"),
	// regardless of whether the upstream is a traditional HTTP repository or
	// an OCI registry. Hydrate consumers always publish these to OCI.
	ArtifactTypeHelm ArtifactType = "helm"

	// ArtifactTypeGitHubRelease identifies a release asset downloaded from a
	// GitHub release (Renovate datasource "github-releases").
	ArtifactTypeGitHubRelease ArtifactType = "github-release"

	// ArtifactTypeGitTag identifies a git tag reference (Renovate datasource
	// "git-tags"), typically used for CLI tool versions.
	ArtifactTypeGitTag ArtifactType = "git-tag"
)

// ArtifactTransport classifies how a consumer must retrieve an artifact from
// its upstream source. Hydrate dispatches on this field to select the
// appropriate client (OCI registry, raw HTTP download, git mirror).
type ArtifactTransport string

const (
	// ArtifactTransportOCI identifies artifacts fetched from — and published
	// to — an OCI registry. Applies to container images and all Helm charts
	// (Windsor always re-hosts Helm via OCI).
	ArtifactTransportOCI ArtifactTransport = "oci"

	// ArtifactTransportTarball identifies artifacts downloaded as raw HTTP
	// archives (GitHub release assets, chart tarballs when shipped verbatim).
	ArtifactTransportTarball ArtifactTransport = "tarball"

	// ArtifactTransportGit identifies artifacts retrieved by cloning a git
	// repository at a specific tag.
	ArtifactTransportGit ArtifactTransport = "git"
)

// =============================================================================
// Types
// =============================================================================

// ArtifactManifest is the top-level document written as artifact-manifest.yaml.
type ArtifactManifest struct {
	Version   string         `yaml:"version"`
	Blueprint Provenance     `yaml:"blueprint,omitempty"`
	Artifacts []ManifestEntry `yaml:"artifacts"`
}

// Provenance records which blueprint produced the manifest and at which
// revision. All fields are optional; absence indicates the manifest was
// generated outside a recognized blueprint build.
type Provenance struct {
	Name    string `yaml:"name,omitempty"`
	Version string `yaml:"version,omitempty"`
	Commit  string `yaml:"commit,omitempty"`
}

// ManifestEntry describes a single external dependency. Reference is the
// canonical pull string a consumer would use against the upstream source;
// Digest, when present, pins the artifact to an immutable content hash;
// Transport tells the hydrate consumer which client is required to fetch it.
type ManifestEntry struct {
	Type       ArtifactType      `yaml:"type"`
	Transport  ArtifactTransport `yaml:"transport"`
	Reference  string            `yaml:"reference"`
	Version    string            `yaml:"version,omitempty"`
	Digest     string            `yaml:"digest,omitempty"`
	Repository string            `yaml:"repository,omitempty"`
	Source     ArtifactSource    `yaml:"source"`
}

// ArtifactSource records where in the blueprint tree an artifact was declared
// and which Renovate annotation surfaced it. File paths are blueprint-relative
// so manifests remain stable across checkout locations.
type ArtifactSource struct {
	File       string `yaml:"file"`
	Line       int    `yaml:"line,omitempty"`
	Datasource string `yaml:"datasource"`
	DepName    string `yaml:"depName,omitempty"`
	Package    string `yaml:"package,omitempty"`
	HelmRepo   string `yaml:"helmRepo,omitempty"`
}
