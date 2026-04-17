package mirror

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// The CacheWriter writes OCI artifacts to a Talos-compatible image cache
// directory. The flat layout (blob/ + manifests/) matches the format produced
// by `talosctl images cache-create` and consumed by the Talos imager and
// node-local registryd service at boot time.

// =============================================================================
// Types
// =============================================================================

// CacheWriter writes OCI artifacts into a flat Talos image cache directory.
// Blobs are stored as blob/sha256-<hex>, manifests under
// manifests/<registry>/<repo>/reference/<tag> and digest/sha256-<hex>.
type CacheWriter struct {
	shims    *Shims
	cacheDir string
	Skipped  atomic.Int64
}

// =============================================================================
// Constructor
// =============================================================================

// NewCacheWriter creates a CacheWriter targeting cacheDir (e.g.
// .windsor/cache/image-cache). The directory is created if needed.
func NewCacheWriter(shims *Shims, cacheDir string) *CacheWriter {
	return &CacheWriter{shims: shims, cacheDir: cacheDir}
}

// =============================================================================
// Public Methods
// =============================================================================

// WriteOCI fetches an OCI image (or image index) from srcRef and writes its
// manifest, config, and layers to the cache in the Talos flat format. Multi-arch
// image indexes are handled by writing the index manifest and each child image.
func (w *CacheWriter) WriteOCI(srcRef string) error {
	ref, err := w.shims.ParseReference(srcRef)
	if err != nil {
		return fmt.Errorf("parse reference %s: %w", srcRef, err)
	}

	manifestBytes, err := w.shims.CraneManifest(srcRef)
	if err != nil {
		return fmt.Errorf("fetch manifest %s: %w", srcRef, err)
	}

	registry := rewriteRegistry(ref.Context().RegistryStr())
	repo := ref.Context().RepositoryStr()
	tag := ref.Identifier()

	if err := w.writeManifest(registry, repo, tag, manifestBytes); err != nil {
		return err
	}

	desc, err := w.shims.RemoteGet(ref)
	if err != nil {
		return fmt.Errorf("remote get %s: %w", srcRef, err)
	}

	switch desc.MediaType {
	case "application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json":
		return w.writeImageIndex(srcRef, ref, desc)
	default:
		return w.writeImage(ref)
	}
}

// WriteHelmOCI writes a Helm OCI chart to the cache under its original registry
// path (not the flat charts/ path used by the Copier — the cache preserves
// upstream paths so registryd can serve them as-is).
func (w *CacheWriter) WriteHelmOCI(srcRef string) error {
	return w.WriteOCI(srcRef)
}

// WriteHelmHTTPS fetches a Helm chart from an HTTPS repository, wraps it as an
// OCI artifact, and writes it to the cache under charts/<name>:<version>.
func (w *CacheWriter) WriteHelmHTTPS(entry HelmHTTPSEntry) error {
	tgzURL, indexEntry, err := resolveHelmChart(w.shims, entry.Repository, entry.ChartName, entry.Version)
	if err != nil {
		return err
	}
	chartBytes, err := fetchBytes(w.shims, tgzURL)
	if err != nil {
		return fmt.Errorf("download chart %s: %w", tgzURL, err)
	}
	provBytes, _ := fetchProvenance(w.shims, tgzURL)

	img, err := buildHelmOCIImage(indexEntry, chartBytes, provBytes)
	if err != nil {
		return err
	}

	registry := "helm"
	repoHost := helmHTTPSRepoHost(entry.Repository)
	repo := repoHost + "/" + entry.ChartName

	return w.writeSingleImage(registry, repo, entry.Version, img)
}

// =============================================================================
// Private Methods
// =============================================================================

// writeImage fetches a single-platform image and writes its config and layers.
func (w *CacheWriter) writeImage(ref name.Reference) error {
	img, err := w.shims.RemoteImage(ref)
	if err != nil {
		return fmt.Errorf("fetch image %s: %w", ref.Name(), err)
	}

	configName, err := img.ConfigName()
	if err != nil {
		return fmt.Errorf("config name %s: %w", ref.Name(), err)
	}
	rawConfig, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("raw config %s: %w", ref.Name(), err)
	}
	if err := w.writeBlob(configName, rawConfig); err != nil {
		return err
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("layers %s: %w", ref.Name(), err)
	}
	for _, layer := range layers {
		if err := w.writeLayer(layer); err != nil {
			return err
		}
	}
	return nil
}

// writeImageIndex handles multi-arch image indexes by writing the index
// manifest and then each platform-specific child image.
func (w *CacheWriter) writeImageIndex(srcRef string, ref name.Reference, desc *remote.Descriptor) error {
	idx, err := desc.ImageIndex()
	if err != nil {
		return fmt.Errorf("image index %s: %w", srcRef, err)
	}
	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return fmt.Errorf("index manifest %s: %w", srcRef, err)
	}

	for _, child := range idxManifest.Manifests {
		childRef := ref.Context().Digest(child.Digest.String())
		childManifestBytes, err := w.shims.CraneManifest(childRef.Name())
		if err != nil {
			continue
		}
		registry := rewriteRegistry(ref.Context().RegistryStr())
		repo := ref.Context().RepositoryStr()
		digestPath := blobName(child.Digest)
		manifestDir := filepath.Join(w.cacheDir, "manifests", registry, repo, "digest")
		if err := w.shims.MkdirAll(manifestDir, 0755); err != nil {
			return fmt.Errorf("create manifest digest dir: %w", err)
		}
		if err := w.shims.WriteFile(filepath.Join(manifestDir, digestPath), childManifestBytes, 0644); err != nil {
			return fmt.Errorf("write child manifest: %w", err)
		}

		childImg, err := w.shims.RemoteImage(childRef)
		if err != nil {
			continue
		}
		configName, err := childImg.ConfigName()
		if err != nil {
			continue
		}
		rawConfig, err := childImg.RawConfigFile()
		if err != nil {
			continue
		}
		if err := w.writeBlob(configName, rawConfig); err != nil {
			return err
		}
		layers, err := childImg.Layers()
		if err != nil {
			continue
		}
		for _, layer := range layers {
			if err := w.writeLayer(layer); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeSingleImage writes a locally-constructed image (e.g. wrapped Helm chart)
// to the cache with the given registry, repo, and tag.
func (w *CacheWriter) writeSingleImage(registry, repo, tag string, img v1.Image) error {
	rawManifest, err := img.RawManifest()
	if err != nil {
		return fmt.Errorf("raw manifest: %w", err)
	}
	if err := w.writeManifest(registry, repo, tag, rawManifest); err != nil {
		return err
	}

	configName, err := img.ConfigName()
	if err != nil {
		return fmt.Errorf("config name: %w", err)
	}
	rawConfig, err := img.RawConfigFile()
	if err != nil {
		return fmt.Errorf("raw config: %w", err)
	}
	if err := w.writeBlob(configName, rawConfig); err != nil {
		return err
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("layers: %w", err)
	}
	for _, layer := range layers {
		if err := w.writeLayer(layer); err != nil {
			return err
		}
	}
	return nil
}

// writeManifest writes manifest bytes to both reference/<tag> and
// digest/sha256-<hex> paths under the manifests directory.
func (w *CacheWriter) writeManifest(registry, repo, tag string, data []byte) error {
	base := filepath.Join(w.cacheDir, "manifests", registry, repo)

	refDir := filepath.Join(base, "reference")
	if err := w.shims.MkdirAll(refDir, 0755); err != nil {
		return fmt.Errorf("create reference dir: %w", err)
	}
	if err := w.shims.WriteFile(filepath.Join(refDir, tag), data, 0644); err != nil {
		return fmt.Errorf("write manifest reference: %w", err)
	}

	digest := sha256.Sum256(data)
	digestHex := fmt.Sprintf("sha256-%x", digest)
	digestDir := filepath.Join(base, "digest")
	if err := w.shims.MkdirAll(digestDir, 0755); err != nil {
		return fmt.Errorf("create digest dir: %w", err)
	}
	if err := w.shims.WriteFile(filepath.Join(digestDir, digestHex), data, 0644); err != nil {
		return fmt.Errorf("write manifest digest: %w", err)
	}
	return nil
}

// writeBlob writes data to blob/sha256-<hex> if it doesn't already exist.
func (w *CacheWriter) writeBlob(hash v1.Hash, data []byte) error {
	blobPath := filepath.Join(w.cacheDir, "blob", blobName(hash))
	if _, err := w.shims.Stat(blobPath); err == nil {
		w.Skipped.Add(1)
		return nil
	}
	if err := w.shims.MkdirAll(filepath.Join(w.cacheDir, "blob"), 0755); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	return w.shims.WriteFile(blobPath, data, 0644)
}

// writeLayer writes a compressed layer to blob/sha256-<hex>.
func (w *CacheWriter) writeLayer(layer v1.Layer) error {
	digest, err := layer.Digest()
	if err != nil {
		return fmt.Errorf("layer digest: %w", err)
	}

	blobPath := filepath.Join(w.cacheDir, "blob", blobName(digest))
	if _, err := w.shims.Stat(blobPath); err == nil {
		w.Skipped.Add(1)
		return nil
	}

	compressed, err := layer.Compressed()
	if err != nil {
		return fmt.Errorf("layer compressed: %w", err)
	}
	defer compressed.Close()

	data, err := io.ReadAll(compressed)
	if err != nil {
		return fmt.Errorf("read layer: %w", err)
	}

	if err := w.shims.MkdirAll(filepath.Join(w.cacheDir, "blob"), 0755); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	return w.shims.WriteFile(blobPath, data, 0644)
}

// =============================================================================
// Helpers
// =============================================================================

// blobName converts a v1.Hash to the Talos blob filename format: sha256-<hex>.
func blobName(h v1.Hash) string {
	return strings.ReplaceAll(h.String(), "sha256:", "sha256-")
}

// rewriteRegistry normalizes registry names for the Talos cache path format.
// Docker Hub's canonical name becomes docker.io, and ports are rewritten
// with underscores for VFAT compatibility (e.g. registry:5000 → registry_5000_).
func rewriteRegistry(registry string) string {
	if registry == name.DefaultRegistry || registry == "index.docker.io" {
		return "docker.io"
	}
	if strings.Contains(registry, ":") {
		return strings.ReplaceAll(registry, ":", "_") + "_"
	}
	return registry
}

// helmHTTPSRepoHost extracts the host from an HTTPS Helm repository URL.
func helmHTTPSRepoHost(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}
