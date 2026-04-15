package mirror

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// The Copier mirrors OCI artifacts from upstream registries into the local
// distribution registry hosting the air-gapped cache.
// It provides canonical-reference copy for blueprint and docker artifacts
// (manifest + layers transferred byte-for-byte) and HTTPS-to-OCI bridging for
// Helm charts fetched from traditional chart repositories.
// The destination path layout embeds the upstream host as the first path
// segment so a single registry can serve many upstreams without collision.

// =============================================================================
// Types
// =============================================================================

// Copier transfers artifacts from upstream registries to a local destination
// registry at destHost (e.g. "localhost:5000"). Skipped counts the artifacts
// bypassed because the destination already holds the same content; concurrent
// goroutines share the counter via atomic operations.
type Copier struct {
	shims    *Shims
	destHost string
	Skipped  atomic.Int64
}

// =============================================================================
// Constructor
// =============================================================================

// NewCopier constructs a Copier targeting destHost. destHost must be the
// registry authority (host:port) with no scheme or trailing slash.
func NewCopier(shims *Shims, destHost string) *Copier {
	return &Copier{shims: shims, destHost: destHost}
}

// =============================================================================
// Public Methods
// =============================================================================

// CopyOCI transfers an OCI artifact at srcRef into the destination registry
// under a host-prefixed path. It uses crane.Copy so multi-architecture image
// indexes are preserved bit-for-bit and blobs already present at the
// destination are mounted-from-source rather than re-uploaded. When the
// destination tag already resolves to the same digest as the upstream, the
// copy is skipped entirely so reruns are near-instant.
func (c *Copier) CopyOCI(srcRef string) error {
	src, err := c.shims.ParseReference(srcRef)
	if err != nil {
		return fmt.Errorf("parse source reference %s: %w", srcRef, err)
	}

	dstRef, err := c.destRef(src)
	if err != nil {
		return err
	}

	if c.destAlreadyHas(dstRef) {
		c.Skipped.Add(1)
		return nil
	}

	if err := c.shims.CraneCopy(srcRef, dstRef.Name()); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", srcRef, dstRef.Name(), err)
	}
	return nil
}

// CopyHelmHTTPS pulls chartName at version from an HTTPS Helm repository,
// wraps it as a canonical Helm OCI artifact, and writes it into the local
// registry under helm/<repoHost>/<chart>:<version>. Provenance (.prov) is
// included when the upstream publishes one. The push is skipped when the
// destination already holds a manifest at the same tag, so reruns avoid
// redundant chart downloads.
func (c *Copier) CopyHelmHTTPS(entry HelmHTTPSEntry) error {
	repoHost := helmRepoHost(entry.Repository)
	dstRefStr := fmt.Sprintf("%s/helm/%s/%s:%s", c.destHost, repoHost, entry.ChartName, entry.Version)
	dstRef, err := c.shims.ParseReference(dstRefStr)
	if err != nil {
		return fmt.Errorf("parse dest reference %s: %w", dstRefStr, err)
	}
	if _, err := c.shims.RemoteGet(dstRef); err == nil {
		c.Skipped.Add(1)
		return nil
	}

	tgzURL, indexEntry, err := resolveHelmChart(c.shims, entry.Repository, entry.ChartName, entry.Version)
	if err != nil {
		return err
	}
	chartBytes, err := fetchBytes(c.shims, tgzURL)
	if err != nil {
		return fmt.Errorf("download chart %s: %w", tgzURL, err)
	}
	provBytes, _ := fetchProvenance(c.shims, tgzURL)

	img, err := buildHelmOCIImage(indexEntry, chartBytes, provBytes)
	if err != nil {
		return err
	}
	return c.writeImage(dstRef, img)
}

// =============================================================================
// Private Methods
// =============================================================================

// destAlreadyHas reports whether the destination registry already has a
// manifest for dst. It deliberately does NOT contact the upstream source —
// doing so on every run would hammer rate-limited registries (Docker Hub
// in particular) for artifacts the user already mirrored. Once content is
// in the local store the mirror treats it as authoritative; users who need
// to refresh a moved tag can delete the repo from the cache directory and
// rerun, or use digest-pinned references from renovate annotations so the
// copy path is digest-driven and inherently reproducible.
func (c *Copier) destAlreadyHas(dst name.Reference) bool {
	_, err := c.shims.RemoteGet(dst)
	return err == nil
}

// destRef computes the local-registry reference corresponding to an upstream
// reference, prefixing the upstream registry host onto the repository path.
func (c *Copier) destRef(src name.Reference) (name.Reference, error) {
	ctx := src.Context()
	host := ctx.RegistryStr()
	if host == name.DefaultRegistry {
		host = "docker.io"
	}
	identifier := src.Identifier()
	dstStr := fmt.Sprintf("%s/%s/%s:%s", c.destHost, host, ctx.RepositoryStr(), identifier)
	if strings.HasPrefix(identifier, "sha256:") {
		dstStr = fmt.Sprintf("%s/%s/%s@%s", c.destHost, host, ctx.RepositoryStr(), identifier)
	}
	return c.shims.ParseReference(dstStr)
}

// writeImage uploads the image to the destination registry, explicitly pushing
// each layer and the config blob before the manifest to avoid MANIFEST_BLOB_
// UNKNOWN races that some registries exhibit under fast successive writes.
func (c *Copier) writeImage(dst name.Reference, img v1.Image) error {
	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}

	for _, layerDesc := range manifest.Layers {
		layer, err := img.LayerByDigest(layerDesc.Digest)
		if err != nil {
			return fmt.Errorf("get layer %s: %w", layerDesc.Digest, err)
		}
		blobRef := dst.Context().Digest(layerDesc.Digest.String())
		if _, err := c.shims.RemoteGet(blobRef); err == nil {
			continue
		}
		if err := c.shims.RemoteWriteLayer(dst.Context(), layer); err != nil {
			return fmt.Errorf("upload layer %s: %w", layerDesc.Digest, err)
		}
	}

	if err := c.shims.RemoteWrite(dst, img); err != nil {
		return fmt.Errorf("write manifest %s: %w", dst.Name(), err)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// helmRepoHost extracts the host portion of an HTTPS Helm repository URL for
// use as a namespace segment in the destination registry path.
func helmRepoHost(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}
