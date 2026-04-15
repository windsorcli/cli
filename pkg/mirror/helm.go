package mirror

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// The HelmBridge converts a Helm chart fetched from an HTTPS repository into an
// OCI artifact and pushes it to the local mirror registry.
// It provides index parsing, chart and provenance download, and OCI wrapping
// using the canonical Helm media types so consumers pulling the mirrored chart
// see bytes indistinguishable from those produced by `helm push`.
// Provenance (.prov) files are preserved when present so `helm verify` still
// succeeds against the mirrored chart.

// =============================================================================
// Constants
// =============================================================================

const (
	helmConfigMediaType     = "application/vnd.cncf.helm.config.v1+json"
	helmChartMediaType      = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
	helmProvenanceMediaType = "application/vnd.cncf.helm.chart.provenance.v1.prov"
)

// =============================================================================
// Types
// =============================================================================

// helmIndex is a minimal decoding of a Helm repository index.yaml — only the
// fields required to locate a chart .tgz for a specific name and version.
type helmIndex struct {
	Entries map[string][]helmIndexEntry `yaml:"entries"`
}

// helmIndexEntry is a single chart entry in an index.yaml.
type helmIndexEntry struct {
	Name       string   `yaml:"name"`
	Version    string   `yaml:"version"`
	AppVersion string   `yaml:"appVersion"`
	APIVersion string   `yaml:"apiVersion"`
	URLs       []string `yaml:"urls"`
}

// helmChartConfig is the JSON config blob embedded in a Helm OCI artifact.
// Shape matches what `helm push` writes so downstream consumers (including
// helm itself) treat a mirrored chart identically to one pushed by helm.
type helmChartConfig struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	APIVersion string `json:"apiVersion,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
}

// helmImageCore implements partial.CompressedImageCore so the assembled OCI
// image exposes a Helm-format config blob and an explicit manifest whose
// descriptors reference the chart (and optional provenance) layers directly.
type helmImageCore struct {
	rawConfig   []byte
	rawManifest []byte
	layersByDig map[v1.Hash]*helmCompressedLayer
}

// helmCompressedLayer implements partial.CompressedLayer for a helm chart or
// provenance layer. The underlying bytes are treated as already-compressed
// for registry transport purposes, matching the wire format used by helm.
type helmCompressedLayer struct {
	mediaType types.MediaType
	content   []byte
	digest    v1.Hash
	size      int64
}

// =============================================================================
// Private Methods
// =============================================================================

// MediaType returns the manifest media type for the helm OCI image.
func (c *helmImageCore) MediaType() (types.MediaType, error) {
	return types.OCIManifestSchema1, nil
}

// RawConfigFile returns the raw JSON bytes of the Helm chart config blob.
func (c *helmImageCore) RawConfigFile() ([]byte, error) {
	return c.rawConfig, nil
}

// RawManifest returns the precomputed JSON manifest bytes.
func (c *helmImageCore) RawManifest() ([]byte, error) {
	return c.rawManifest, nil
}

// LayerByDigest returns the layer with the supplied content digest, or an
// error when no such layer was registered on construction.
func (c *helmImageCore) LayerByDigest(h v1.Hash) (partial.CompressedLayer, error) {
	layer, ok := c.layersByDig[h]
	if !ok {
		return nil, fmt.Errorf("helm layer with digest %s not found", h)
	}
	return layer, nil
}

// Digest reports the sha256 content digest of the layer.
func (l *helmCompressedLayer) Digest() (v1.Hash, error) { return l.digest, nil }

// Compressed returns a reader over the layer's on-wire bytes.
func (l *helmCompressedLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

// Size reports the byte length of the layer content.
func (l *helmCompressedLayer) Size() (int64, error) { return l.size, nil }

// MediaType reports the canonical Helm media type associated with the layer.
func (l *helmCompressedLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }

// =============================================================================
// Helpers
// =============================================================================

// resolveHelmChart fetches and parses the repository index at repoURL and
// returns the first .tgz download URL for the named chart at the requested
// version. Returns an error when the chart or version is not indexed.
func resolveHelmChart(shims *Shims, repoURL, chartName, version string) (tgzURL string, entry helmIndexEntry, err error) {
	idxURL := strings.TrimRight(repoURL, "/") + "/index.yaml"
	resp, err := shims.HttpGet(idxURL)
	if err != nil {
		return "", helmIndexEntry{}, fmt.Errorf("failed to fetch helm index %s: %w", idxURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", helmIndexEntry{}, fmt.Errorf("helm index %s returned status %d", idxURL, resp.StatusCode)
	}
	body, err := shims.ReadAll(resp.Body)
	if err != nil {
		return "", helmIndexEntry{}, fmt.Errorf("failed to read helm index: %w", err)
	}
	var idx helmIndex
	if err := shims.YamlUnmarshal(body, &idx); err != nil {
		return "", helmIndexEntry{}, fmt.Errorf("failed to parse helm index: %w", err)
	}
	candidates := []string{version, "v" + version, strings.TrimPrefix(version, "v")}
	for _, want := range candidates {
		for _, e := range idx.Entries[chartName] {
			if e.Version == want {
				if len(e.URLs) == 0 {
					return "", helmIndexEntry{}, fmt.Errorf("helm chart %s %s has no download URLs", chartName, want)
				}
				return absoluteURL(repoURL, e.URLs[0]), e, nil
			}
		}
	}
	return "", helmIndexEntry{}, fmt.Errorf("helm chart %s version %s not found in %s (also tried v-prefix variants)", chartName, version, idxURL)
}

// absoluteURL resolves chartURL against repoURL, returning chartURL unchanged
// when it is already absolute. Helm indexes may contain relative URLs.
func absoluteURL(repoURL, chartURL string) string {
	if strings.HasPrefix(chartURL, "http://") || strings.HasPrefix(chartURL, "https://") {
		return chartURL
	}
	base, err := url.Parse(strings.TrimRight(repoURL, "/") + "/")
	if err != nil {
		return chartURL
	}
	rel, err := url.Parse(chartURL)
	if err != nil {
		return chartURL
	}
	return base.ResolveReference(rel).String()
}

// fetchBytes issues an HTTP GET and returns the response body bytes. Non-2xx
// statuses are reported as errors. The caller owns the returned slice.
func fetchBytes(shims *Shims, rawURL string) ([]byte, error) {
	resp, err := shims.HttpGet(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s returned status %d", rawURL, resp.StatusCode)
	}
	return shims.ReadAll(resp.Body)
}

// fetchProvenance attempts to download a <chart>.tgz.prov alongside the chart
// archive. A missing provenance file is not an error — the returned byte slice
// will be nil and callers should treat it as optional.
func fetchProvenance(shims *Shims, chartURL string) ([]byte, error) {
	resp, err := shims.HttpGet(chartURL + ".prov")
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	return shims.ReadAll(resp.Body)
}

// sha256Digest computes the sha256 hash of content and returns it in the v1
// digest form used by registry descriptors.
func sha256Digest(content []byte) v1.Hash {
	sum := sha256.Sum256(content)
	return v1.Hash{Algorithm: "sha256", Hex: hex.EncodeToString(sum[:])}
}

// buildHelmOCIImage wraps raw chart bytes (and optional provenance bytes) in an
// OCI image whose config blob contains Helm's canonical chart metadata JSON.
// The resulting image is byte-compatible with the output of `helm push` so
// helm clients consuming the mirrored chart behave identically to those
// pulling from the upstream OCI host.
func buildHelmOCIImage(entry helmIndexEntry, chartBytes, provBytes []byte) (v1.Image, error) {
	cfg := helmChartConfig{
		Name:       entry.Name,
		Version:    entry.Version,
		APIVersion: entry.APIVersion,
		AppVersion: entry.AppVersion,
	}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal helm config: %w", err)
	}

	layersByDig := map[v1.Hash]*helmCompressedLayer{}
	mkLayer := func(content []byte, mt types.MediaType) *helmCompressedLayer {
		digest := sha256Digest(content)
		layer := &helmCompressedLayer{
			mediaType: mt,
			content:   content,
			digest:    digest,
			size:      int64(len(content)),
		}
		layersByDig[digest] = layer
		return layer
	}

	chartLayer := mkLayer(chartBytes, helmChartMediaType)
	var manifestLayers []v1.Descriptor
	manifestLayers = append(manifestLayers, v1.Descriptor{
		MediaType: chartLayer.mediaType,
		Size:      chartLayer.size,
		Digest:    chartLayer.digest,
	})
	if len(provBytes) > 0 {
		provLayer := mkLayer(provBytes, helmProvenanceMediaType)
		manifestLayers = append(manifestLayers, v1.Descriptor{
			MediaType: provLayer.mediaType,
			Size:      provLayer.size,
			Digest:    provLayer.digest,
		})
	}

	manifest := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config: v1.Descriptor{
			MediaType: helmConfigMediaType,
			Size:      int64(len(cfgBytes)),
			Digest:    sha256Digest(cfgBytes),
		},
		Layers: manifestLayers,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal helm manifest: %w", err)
	}

	core := &helmImageCore{
		rawConfig:   cfgBytes,
		rawManifest: manifestBytes,
		layersByDig: layersByDig,
	}
	img, err := partial.CompressedToImage(core)
	if err != nil {
		return nil, fmt.Errorf("failed to build helm OCI image: %w", err)
	}
	return img, nil
}
