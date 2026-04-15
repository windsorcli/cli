package mirror

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// The Resolver walks the OCI blueprint graph rooted at a set of seed blueprint
// references and accumulates every downstream artifact into a CopyPlan.
// It provides recursive expansion of blueprint `sources[]` entries with
// `oci://` transport, plus aggregation of leaf entries (docker images, helm
// charts) declared in each blueprint's bundled artifact-manifest.yaml.
// Visited blueprints are deduplicated by canonical reference so cycles and
// diamond dependencies do not produce repeated work.

// =============================================================================
// Types
// =============================================================================

// CopyPlan is the deduplicated output of blueprint graph resolution. Blueprints
// and DockerImages are canonical OCI references (no oci:// prefix); HelmHTTPS
// entries carry the repository URL, chart name, and version required to fetch
// a chart from an HTTPS Helm index.
type CopyPlan struct {
	Blueprints   []string
	DockerImages []string
	HelmHTTPS    []HelmHTTPSEntry
	HelmOCI      []string
	Skipped      []SkippedEntry
}

// HelmHTTPSEntry captures the information needed to pull a chart from an
// HTTPS-only Helm repository and re-host it as an OCI artifact.
type HelmHTTPSEntry struct {
	Repository string
	ChartName  string
	Version    string
}

// SkippedEntry records manifest entries that cannot be mirrored through an OCI
// registry (github-release, git-tag) so the CLI can report them to the user.
type SkippedEntry struct {
	Reference string
	Type      string
	Reason    string
}

// Resolver performs the graph walk and produces a CopyPlan.
type Resolver struct {
	shims    *Shims
	seen     map[string]bool
	OnStatus func(message string)
}

// =============================================================================
// Constructor
// =============================================================================

// NewResolver constructs a Resolver using the provided shims for all network
// and OCI registry access.
func NewResolver(shims *Shims) *Resolver {
	return &Resolver{
		shims: shims,
		seen:  make(map[string]bool),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Resolve walks the blueprint graph starting from every seed URL and returns a
// CopyPlan containing the dedeuplicated set of artifacts that need to be
// mirrored. Seed URLs are expected in `oci://host/repo:tag` form.
func (r *Resolver) Resolve(seedURLs []string) (*CopyPlan, error) {
	plan := &CopyPlan{}
	queue := make([]string, 0, len(seedURLs))
	for _, s := range seedURLs {
		if canon := canonicalOCI(s); canon != "" {
			queue = append(queue, canon)
		}
	}

	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		if r.seen[ref] {
			continue
		}
		r.seen[ref] = true
		plan.Blueprints = append(plan.Blueprints, ref)
		if r.OnStatus != nil {
			r.OnStatus(fmt.Sprintf("resolving %s", ref))
		}

		bpYAML, manifest, err := r.fetchBlueprintContents(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch blueprint %s: %w", ref, err)
		}

		if len(bpYAML) > 0 {
			children, err := r.extractOCISources(bpYAML)
			if err != nil {
				return nil, fmt.Errorf("failed to parse blueprint yaml from %s: %w", ref, err)
			}
			queue = append(queue, children...)
		}

		if manifest != nil {
			r.addManifestEntries(plan, manifest)
		}
	}

	return plan, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// fetchBlueprintContents pulls the OCI artifact at ref, decompresses its
// first layer, and returns the embedded `_template/blueprint.yaml` and parsed
// `artifact-manifest.yaml`. Missing files yield zero values rather than
// errors so older blueprints lacking a bundled manifest still traverse.
func (r *Resolver) fetchBlueprintContents(ref string) ([]byte, *artifact.ArtifactManifest, error) {
	parsed, err := r.shims.ParseReference(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("parse reference: %w", err)
	}
	img, err := r.shims.RemoteImage(parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("pull image: %w", err)
	}
	layers, err := r.shims.ImageLayers(img)
	if err != nil {
		return nil, nil, fmt.Errorf("read layers: %w", err)
	}
	if len(layers) == 0 {
		return nil, nil, fmt.Errorf("blueprint artifact has no layers")
	}
	rc, err := r.shims.LayerUncompressed(layers[0])
	if err != nil {
		return nil, nil, fmt.Errorf("uncompress layer: %w", err)
	}
	defer rc.Close()
	raw, err := r.shims.ReadAll(rc)
	if err != nil {
		return nil, nil, fmt.Errorf("read layer: %w", err)
	}

	return extractBlueprintArtifactFiles(raw, r.shims)
}

// extractOCISources parses blueprint YAML and returns the canonical OCI refs
// for every source entry whose URL uses the oci:// scheme.
func (r *Resolver) extractOCISources(bpYAML []byte) ([]string, error) {
	var bp blueprintv1alpha1.Blueprint
	if err := r.shims.YamlUnmarshal(bpYAML, &bp); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(bp.Sources))
	for _, src := range bp.Sources {
		if canon := canonicalOCI(src.Url); canon != "" {
			out = append(out, canon)
		}
	}
	return out, nil
}

// IngestManifest merges the entries from an externally-supplied artifact
// manifest (typically the local project's scan) into the plan. Used to
// hydrate mirrors for base blueprints that have no OCI source dependencies
// but still publish their own images and charts.
func (r *Resolver) IngestManifest(plan *CopyPlan, manifest *artifact.ArtifactManifest) {
	if manifest == nil {
		return
	}
	r.addManifestEntries(plan, manifest)
}

// addManifestEntries classifies each entry in the blueprint's bundled manifest
// into the appropriate CopyPlan slice.
func (r *Resolver) addManifestEntries(plan *CopyPlan, manifest *artifact.ArtifactManifest) {
	for _, entry := range manifest.Artifacts {
		switch entry.Type {
		case artifact.ArtifactTypeDocker:
			ref := canonicalDockerImage(entry.Reference)
			if ref == "" {
				continue
			}
			if !containsString(plan.DockerImages, ref) {
				plan.DockerImages = append(plan.DockerImages, ref)
			}
		case artifact.ArtifactTypeHelm:
			chart := helmChartName(entry)
			if strings.HasPrefix(entry.Repository, "oci://") {
				ociRef := canonicalOCI(entry.Repository + "/" + chart + ":" + entry.Version)
				if ociRef != "" && !containsString(plan.HelmOCI, ociRef) {
					plan.HelmOCI = append(plan.HelmOCI, ociRef)
				}
				continue
			}
			if entry.Repository == "" || chart == "" || entry.Version == "" {
				continue
			}
			h := HelmHTTPSEntry{Repository: entry.Repository, ChartName: chart, Version: entry.Version}
			if !containsHelmHTTPS(plan.HelmHTTPS, h) {
				plan.HelmHTTPS = append(plan.HelmHTTPS, h)
			}
		case artifact.ArtifactTypeGitHubRelease, artifact.ArtifactTypeGitTag:
			// Non-OCI datasources are version pins for tools or companion
			// downloads whose real container artifacts are already captured
			// by separate docker/helm annotations. They cannot live in an
			// OCI registry, and surfacing them in the mirror report produces
			// unactionable noise, so they are silently ignored.
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// extractBlueprintArtifactFiles scans the decompressed artifact tar for the
// two files the resolver cares about: the bundled blueprint.yaml (source of
// transitive OCI source references) and artifact-manifest.yaml (source of
// leaf artifact entries).
func extractBlueprintArtifactFiles(raw []byte, shims *Shims) ([]byte, *artifact.ArtifactManifest, error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var blueprintYAML []byte
	var manifestYAML []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		switch {
		case name == "_template/blueprint.yaml":
			blueprintYAML, err = io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("read blueprint.yaml: %w", err)
			}
		case name == artifact.ManifestFileName:
			manifestYAML, err = io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("read manifest: %w", err)
			}
		}
	}

	var manifest *artifact.ArtifactManifest
	if len(manifestYAML) > 0 {
		manifest = &artifact.ArtifactManifest{}
		if err := shims.YamlUnmarshal(manifestYAML, manifest); err != nil {
			return nil, nil, fmt.Errorf("parse manifest: %w", err)
		}
	}
	return blueprintYAML, manifest, nil
}

// helmChartName returns the chart name to look up in a Helm index for the
// supplied manifest entry. Renovate annotations may declare `package` (the
// actual chart name) distinct from `depName` (a semantic alias); this helper
// prefers package when present so the lookup matches the real index entry.
func helmChartName(entry artifact.ManifestEntry) string {
	if entry.Source.Package != "" {
		return entry.Source.Package
	}
	return entry.Reference
}

// canonicalOCI strips an oci:// prefix and returns the remainder, or the empty
// string for references that are not OCI.
func canonicalOCI(ref string) string {
	if strings.HasPrefix(ref, "oci://") {
		return strings.TrimPrefix(ref, "oci://")
	}
	return ""
}

// canonicalDockerImage returns a reference suitable for mirroring. Bare image
// references without a registry host are expanded to docker.io/library/<name>
// so the resulting path matches the Talos mirror-key convention.
func canonicalDockerImage(ref string) string {
	if ref == "" {
		return ""
	}
	ref = strings.TrimPrefix(ref, "docker://")
	name := strings.TrimSpace(ref)
	// Split off tag/digest for inspection, reattach at the end.
	tagSep := strings.LastIndex(name, ":")
	digestSep := strings.Index(name, "@")
	pathEnd := len(name)
	if digestSep > 0 {
		pathEnd = digestSep
	} else if tagSep > 0 && !strings.Contains(name[tagSep:], "/") {
		pathEnd = tagSep
	}
	path := name[:pathEnd]
	suffix := name[pathEnd:]
	firstSlash := strings.Index(path, "/")
	if firstSlash == -1 {
		path = "docker.io/library/" + path
	} else {
		firstSegment := path[:firstSlash]
		if !strings.ContainsAny(firstSegment, ".:") && firstSegment != "localhost" {
			path = "docker.io/" + path
		}
	}
	return path + suffix
}

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// containsHelmHTTPS reports whether haystack already contains the same chart
// coordinates. Equality requires matching repository, chart, and version.
func containsHelmHTTPS(haystack []HelmHTTPSEntry, entry HelmHTTPSEntry) bool {
	for _, h := range haystack {
		if h == entry {
			return true
		}
	}
	return false
}
