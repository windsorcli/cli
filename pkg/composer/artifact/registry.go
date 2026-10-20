// The registry functions of the ArtifactBuilder push artifacts to an OCI registry and read data from it.
// It provides the OCI reference parsers, the tag list, and the CLI version check of an artifact.
// Push writes the artifact layer and the image manifest. If the metadata sets a CLI version, the manifest also gets it.
// ResolveCompatibleTag selects the highest tag that the running CLI version supports.

package artifact

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// CliVersionAnnotationKey mirrors BlueprintMetadataInput.CliVersion into the OCI manifest, so
// GetCliVersionConstraint can check it without a full Pull.
const CliVersionAnnotationKey = "windsorcli.dev/cli-version"

// =============================================================================
// Types
// =============================================================================

// OCIArtifactInfo contains information about the OCI artifact source for blueprint data
type OCIArtifactInfo struct {
	Name string
	URL  string
	Tag  string
}

// =============================================================================
// Public Methods
// =============================================================================

// ParseOCIRef parses an OCI reference into registry, repository, and tag.
// Accepts "oci://registry/repository:tag" or "oci://registry/repository@sha256:digest".
// The registry may contain a port, so repository and tag split on the last colon.
// A repository containing a colon is rejected to avoid a wrong split.
// A digest reference returns tag as the bare "sha256:<hex>" string.
// Pass tag to FormatOCIRef to rebuild the correct separator.
func (a *ArtifactBuilder) ParseOCIRef(ociRef string) (registry, repository, tag string, err error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
	}

	ref := strings.TrimPrefix(ociRef, "oci://")

	firstSlash := strings.Index(ref, "/")
	if firstSlash <= 0 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}
	registry = ref[:firstSlash]
	repoAndVersion := ref[firstSlash+1:]

	if atIdx := strings.Index(repoAndVersion, "@sha256:"); atIdx >= 0 {
		repository = repoAndVersion[:atIdx]
		tag = repoAndVersion[atIdx+1:]
		if repository == "" || strings.Contains(repository, ":") {
			return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository@sha256:digest: %s", ociRef)
		}
		return registry, repository, tag, nil
	}

	lastColon := strings.LastIndex(repoAndVersion, ":")
	if lastColon <= 0 || lastColon == len(repoAndVersion)-1 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}
	repository = repoAndVersion[:lastColon]
	tag = repoAndVersion[lastColon+1:]
	if strings.Contains(repository, ":") {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	return registry, repository, tag, nil
}

// ListTags returns the tags published for the repository of an OCI reference (the tag in ociRef is
// ignored). It is used to resolve the latest available version when upgrading a source. Returns an
// error if the reference is malformed or the registry cannot be reached.
func (a *ArtifactBuilder) ListTags(ociRef string) ([]string, error) {
	registry, repository, _, err := a.ParseOCIRef(ociRef)
	if err != nil {
		return nil, err
	}
	repo, err := a.shims.NewRepository(registry + "/" + repository)
	if err != nil {
		return nil, fmt.Errorf("invalid repository %q/%q: %w", registry, repository, err)
	}
	tags, err := a.shims.RemoteList(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for %s/%s: %w", registry, repository, err)
	}
	return tags, nil
}

// GetCliVersionConstraint returns ociRef's declared cliVersion constraint, via a manifest-only
// fetch (no layer pull). Empty string, not an error, means no constraint declared.
func (a *ArtifactBuilder) GetCliVersionConstraint(ociRef string) (string, error) {
	registry, repository, tag, err := a.ParseOCIRef(ociRef)
	if err != nil {
		return "", err
	}

	ref := FormatOCIRef(registry, repository, tag)
	parsedRef, err := a.shims.ParseReference(ref)
	if err != nil {
		return "", fmt.Errorf("failed to parse reference %s: %w", ref, err)
	}

	img, err := a.shims.RemoteImage(parsedRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil && IsAuthenticationError(err) {
		if anonImg, anonErr := a.shims.RemoteImage(parsedRef, remote.WithAuth(authn.Anonymous)); anonErr == nil {
			img, err = anonImg, nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("failed to get image manifest for %s: %w", ref, err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return "", fmt.Errorf("failed to read manifest for %s: %w", ref, err)
	}

	return manifest.Annotations[CliVersionAnnotationKey], nil
}

// VerifyCliVersionCompatibility pulls ociRef and validates its metadata.yaml cliVersion constraint
// against the running CLI. Used as ResolveCompatibleTag's fallback when a candidate's manifest
// annotation is absent. Returns nil if compatible or undeclared; any other error means "don't
// trust this candidate."
func (a *ArtifactBuilder) VerifyCliVersionCompatibility(ociRef string) error {
	artifacts, err := a.Pull([]string{ociRef})
	if err != nil {
		return fmt.Errorf("failed to pull %s: %w", ociRef, err)
	}
	registry, repository, tag, err := a.ParseOCIRef(ociRef)
	if err != nil {
		return err
	}
	cacheKey := FormatOCIRef(registry, repository, tag)
	cacheDir, ok := artifacts[cacheKey]
	if !ok {
		return fmt.Errorf("failed to retrieve cache directory for %s", ociRef)
	}

	metadataPath := filepath.Join(cacheDir, "metadata.yaml")
	if _, err := a.shims.Stat(metadataPath); os.IsNotExist(err) {
		return nil
	}
	data, err := a.shims.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", metadataPath, err)
	}
	var meta BlueprintMetadataInput
	if err := a.shims.YamlUnmarshal(data, &meta); err != nil {
		return fmt.Errorf("failed to parse %s: %w", metadataPath, err)
	}
	return ValidateCliVersion(constants.Version, meta.CliVersion)
}

// Push uploads the artifact to an OCI registry with explicit blob handling to prevent MANIFEST_BLOB_UNKNOWN errors.
// Implements robust blob upload strategy recommended by Red Hat for resolving registry upload issues.
// Bundles files from the project, creates tarball in memory, constructs OCI image, uploads blobs explicitly, then uploads manifest.
// Uses authenticated keychain for registry access and retry backoff for resilience.
// Registry base should be the base URL (e.g., "ghcr.io/namespace"), repoName the repository name, tag the version.
func (a *ArtifactBuilder) Push(registryBase string, repoName string, tag string) error {
	if err := a.Bundle(); err != nil {
		return fmt.Errorf("failed to bundle files: %w", err)
	}

	finalName, tagName, cliVersionConstraint, metadata, err := a.parseTagAndResolveMetadata(repoName, tag)
	if err != nil {
		return err
	}

	a.metadata.Name = finalName
	a.metadata.Version = tagName
	a.metadata.CliVersion = cliVersionConstraint
	if err := a.refreshArtifactManifest(); err != nil {
		return err
	}

	tarballContent, err := a.createTarballInMemory(metadata)
	if err != nil {
		return fmt.Errorf("failed to create tarball in memory: %w", err)
	}

	layer := static.NewLayer(tarballContent, types.DockerLayer)

	img, err := a.createOCIArtifactImage(layer, finalName, tagName, cliVersionConstraint)
	if err != nil {
		return fmt.Errorf("failed to create OCI image: %w", err)
	}

	repoURL := fmt.Sprintf("%s/%s", registryBase, repoName)
	if tagName != "" {
		repoURL = fmt.Sprintf("%s:%s", repoURL, tagName)
	}

	ref, err := name.ParseReference(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repository reference: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("failed to get image manifest: %w", err)
	}

	for _, layerDesc := range manifest.Layers {
		layer, err := img.LayerByDigest(layerDesc.Digest)
		if err != nil {
			return fmt.Errorf("failed to get layer %s: %w", layerDesc.Digest, err)
		}

		blobRef := ref.Context().Digest(layerDesc.Digest.String())
		_, err = a.shims.RemoteGet(blobRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			err = a.shims.RemoteWriteLayer(ref.Context(), layer, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				return fmt.Errorf("failed to upload layer %s: %w", layerDesc.Digest, err)
			}
		}
	}

	configDigest, err := img.ConfigName()
	if err != nil {
		return fmt.Errorf("failed to get config digest: %w", err)
	}

	configRef := ref.Context().Digest(configDigest.String())
	_, err = a.shims.RemoteGet(configRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		configBlob, err := img.RawConfigFile()
		if err != nil {
			return fmt.Errorf("failed to get config file: %w", err)
		}

		configLayer := static.NewLayer(configBlob, types.DockerConfigJSON)
		err = a.shims.RemoteWriteLayer(ref.Context(), configLayer, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			return fmt.Errorf("failed to upload config: %w", err)
		}
	}

	err = a.shims.RemoteWrite(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithRetryBackoff(remote.Backoff{
		Duration: 1.0,
		Factor:   3.0,
		Jitter:   0.1,
		Steps:    5,
	}))
	if err != nil {
		return fmt.Errorf("failed to push artifact to registry: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createOCIArtifactImage constructs an OCI image from a layer with generic OCI artifact media types and annotations.
// Creates a generic OCI artifact config file compatible with both FluxCD and blueprint consumers.
// Sets architecture to amd64 and OS to linux for compatibility with container runtimes.
// Uses standard OCI media types with optional artifactType for tool-specific identification.
// Adds comprehensive OCI annotations including creation time, source, revision, title, and version.
// When cliVersionConstraint is non-empty, also sets CliVersionAnnotationKey so a consumer can check
// CLI compatibility for this tag via GetCliVersionConstraint's manifest-only fetch, without a full Pull.
// Returns a complete OCI image ready for pushing to any OCI 1.1 compatible registry.
func (a *ArtifactBuilder) createOCIArtifactImage(layer v1.Layer, repoName, tagName, cliVersionConstraint string) (v1.Image, error) {
	gitProvenance, err := a.getGitProvenance()
	if err != nil {
		gitProvenance = GitProvenance{}
	}

	revision := gitProvenance.CommitSHA
	if revision == "" {
		revision = "unknown"
	}

	source := gitProvenance.RemoteURL
	if source == "" {
		source = "unknown"
	}

	configFile := &v1.ConfigFile{
		Architecture: "amd64",
		OS:           "linux",
		Config: v1.Config{
			Labels: map[string]string{
				"org.opencontainers.image.title":       repoName,
				"org.opencontainers.image.description": fmt.Sprintf("Windsor blueprint artifact for %s", repoName),
			},
		},
		RootFS: v1.RootFS{
			Type: "layers",
		},
		History: []v1.History{
			{
				Created: v1.Time{Time: time.Now()},
				Comment: "Windsor blueprint artifact layer",
			},
		},
	}

	img, err := a.shims.AppendLayers(a.shims.EmptyImage(), layer)
	if err != nil {
		return nil, fmt.Errorf("failed to append layer to image: %w", err)
	}

	img, err = a.shims.ConfigFile(img, configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to set config file: %w", err)
	}

	img = a.shims.MediaType(img, "application/vnd.oci.image.manifest.v1+json")
	img = a.shims.ConfigMediaType(img, "application/vnd.windsorcli.blueprint.v1+json")

	annotations := map[string]string{
		"org.opencontainers.image.created":  time.Now().UTC().Format(time.RFC3339),
		"org.opencontainers.image.source":   source,
		"org.opencontainers.image.revision": revision,
		"org.opencontainers.image.title":    repoName,
		"org.opencontainers.image.version":  tagName,
	}
	if cliVersionConstraint != "" {
		annotations[CliVersionAnnotationKey] = cliVersionConstraint
	}

	img = a.shims.Annotations(img, annotations)

	return img, nil
}

// downloadOCIArtifact downloads an OCI artifact and returns the tar.gz data.
// Constructs an OCI reference from registry, repository, and tag components.
// Downloads the first layer of the OCI image which contains the artifact data.
// Returns the uncompressed layer data as bytes for further processing.
// Authenticates via the Docker keychain so private registries work; if the registry rejects
// those credentials (per IsAuthenticationError), retries anonymously so public artifacts still
// pull when the local Docker config holds stale or scope-limited tokens. In verbose mode, on
// a successful anonymous retry, emits a stderr note naming the registry so an operator who
// opts into the detail can see that their configured credentials were bypassed; silent in
// normal mode because the case is common for any user who has ever run `docker login`.
// If the anonymous retry also fails, the original keychain error is returned so the
// underlying auth failure remains visible.
func (a *ArtifactBuilder) downloadOCIArtifact(registry, repository, tag string) ([]byte, error) {
	ref := FormatOCIRef(registry, repository, tag)

	parsedRef, err := a.shims.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %s: %w", ref, err)
	}

	img, err := a.shims.RemoteImage(parsedRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil && IsAuthenticationError(err) {
		if anonImg, anonErr := a.shims.RemoteImage(parsedRef, remote.WithAuth(authn.Anonymous)); anonErr == nil {
			if a.shell.IsVerbose() {
				fmt.Fprintf(os.Stderr, "keychain credentials rejected by %s; pulled anonymously\n", registry)
			}
			img, err = anonImg, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	layers, err := a.shims.ImageLayers(img)
	if err != nil {
		return nil, fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no layers found in image")
	}

	layer := layers[0]
	reader, err := a.shims.LayerUncompressed(layer)
	if err != nil {
		return nil, fmt.Errorf("failed to get layer reader: %w", err)
	}
	defer reader.Close()

	data, err := a.shims.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer data: %w", err)
	}

	return data, nil
}

// =============================================================================
// Helpers
// =============================================================================

// FormatOCIRef builds "registry/repository:tag", or "registry/repository@tag" when tag is a
// digest ("sha256:<hex>"). Every ParseOCIRef caller uses this to rebuild a reference or cache
// key, so a digest round-trips correctly.
func FormatOCIRef(registry, repository, tag string) string {
	if strings.HasPrefix(tag, "sha256:") {
		return fmt.Sprintf("%s/%s@%s", registry, repository, tag)
	}
	return fmt.Sprintf("%s/%s:%s", registry, repository, tag)
}

// ResolveCompatibleTag walks tags newest-first (stable semver only) and returns the highest one
// this CLI is compatible with. Checks the cheap manifest annotation first; falls back to
// VerifyCliVersionCompatibility (a real pull) when the annotation is absent, since older tags
// predate that annotation and can't be assumed compatible. A manifest-fetch error skips that
// candidate instead of aborting the walk; it's only returned if no candidate ends up qualifying.
func ResolveCompatibleTag(artifactBuilder Artifact, urlPrefix string, tags []string) (tag string, version *semver.Version, ok bool, err error) {
	type candidate struct {
		tag string
		ver *semver.Version
	}
	var candidates []candidate
	for _, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil || v.Prerelease() != "" {
			continue
		}
		candidates = append(candidates, candidate{tag: t, ver: v})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ver.GreaterThan(candidates[j].ver)
	})

	var constraintCheckErr error
	for _, c := range candidates {
		candidateRef := urlPrefix + ":" + c.tag
		constraint, err := artifactBuilder.GetCliVersionConstraint(candidateRef)
		if err != nil {
			constraintCheckErr = fmt.Errorf("failed to check compatibility for %s: %w", candidateRef, err)
			continue
		}
		if constraint != "" {
			if ValidateCliVersion(constants.Version, constraint) == nil {
				return c.tag, c.ver, true, nil
			}
			continue
		}
		if artifactBuilder.VerifyCliVersionCompatibility(candidateRef) == nil {
			return c.tag, c.ver, true, nil
		}
	}
	return "", nil, false, constraintCheckErr
}

// ParseOCIReference parses a blueprint reference string in OCI URL or org/repo:tag format and returns an OCIArtifactInfo struct.
// Accepts full OCI URLs (e.g., oci://ghcr.io/org/repo:v1.0.0) and org/repo:v1.0.0 formats.
// Both formats also accept a "@sha256:<hex>" digest in place of the tag, checked before the
// version colon since the digest itself contains one. Tag holds the bare digest string, without "@".
// Returns nil if the reference is empty, missing a version, or not in a supported format.
func ParseOCIReference(ociRef string) (*OCIArtifactInfo, error) {
	if ociRef == "" {
		return nil, nil
	}

	var name, version, fullURL string

	if strings.HasPrefix(ociRef, "oci://") {
		fullURL = ociRef
		remaining := strings.TrimPrefix(ociRef, "oci://")
		var pathPart string
		if atIdx := strings.Index(remaining, "@sha256:"); atIdx >= 0 {
			version = remaining[atIdx+1:]
			pathPart = remaining[:atIdx]
		} else if lastColon := strings.LastIndex(remaining, ":"); lastColon > 0 {
			version = remaining[lastColon+1:]
			pathPart = remaining[:lastColon]
		} else {
			return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
		}
		if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
			name = pathPart[lastSlash+1:]
		} else {
			return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
		}
	} else {
		var pathPart string
		if atIdx := strings.Index(ociRef, "@sha256:"); atIdx >= 0 {
			version = ociRef[atIdx+1:]
			pathPart = ociRef[:atIdx]
		} else if colonIdx := strings.LastIndex(ociRef, ":"); colonIdx > 0 {
			version = ociRef[colonIdx+1:]
			pathPart = ociRef[:colonIdx]
		} else {
			return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
		}
		if strings.Count(pathPart, "/") >= 1 {
			if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
				name = pathPart[lastSlash+1:]
			} else {
				return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
			}
			fullURL = "oci://ghcr.io/" + ociRef
		} else {
			return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
		}
	}

	if version == "" || name == "" {
		return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
	}

	return &OCIArtifactInfo{
		Name: name,
		URL:  fullURL,
		Tag:  version,
	}, nil
}

// ParseRegistryURL parses a registry URL string into its components.
// It handles formats like "registry.com/repo:tag", "registry.com/repo", or "oci://registry.com/repo:tag".
// Returns registryBase, repoName, tag, and an error if parsing fails.
func ParseRegistryURL(registryURL string) (registryBase, repoName, tag string, err error) {
	arg := strings.TrimPrefix(registryURL, "oci://")

	if lastColon := strings.LastIndex(arg, ":"); lastColon > 0 && lastColon < len(arg)-1 {
		tag = arg[lastColon+1:]
		arg = arg[:lastColon]
	}

	if firstSlash := strings.Index(arg, "/"); firstSlash >= 0 {
		registryBase = arg[:firstSlash]
		repoName = arg[firstSlash+1:]
	} else {
		return "", "", "", fmt.Errorf("invalid registry format: must include repository path (e.g., registry.com/namespace/repo)")
	}

	return registryBase, repoName, tag, nil
}

// IsAuthenticationError reports whether err originates from a registry authentication or
// authorization failure, detected via the *transport.Error type from go-containerregistry
// (HTTP 401/403 or UNAUTHORIZED/DENIED diagnostic codes). Walks the wrap chain via errors.As,
// so wrapping with fmt.Errorf("...: %w", err) is transparent. Used by the push command to
// decide whether to hint the user to run `docker login`.
func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	var tErr *transport.Error
	if !errors.As(err, &tErr) {
		return false
	}
	if tErr.StatusCode == http.StatusUnauthorized || tErr.StatusCode == http.StatusForbidden {
		return true
	}
	for _, d := range tErr.Errors {
		if d.Code == transport.UnauthorizedErrorCode || d.Code == transport.DeniedErrorCode {
			return true
		}
	}
	return false
}

// ValidateCliVersion validates that the provided CLI version satisfies the cliVersion constraint
// specified in the template metadata. If constraint is empty, validation is skipped.
// If cliVersion is empty, validation is skipped (caller cannot determine version).
// If the CLI version is "dev", "main", or "latest", validation is skipped as these are development builds.
// If the CLI version starts with "nightly", validation is skipped: the nightly release builds a
// non-SemVer snapshot string ("nightly-SNAPSHOT-<sha>") that has no real version to check.
// A pre-release CLI version (e.g. "0.9.0-rc.1") is checked against its release version
// (Masterminds/semver excludes pre-releases from a range unless the constraint itself
// declares one, which would otherwise fail every release-candidate build against an
// ordinary ">=" constraint).
// Returns an error if the constraint is specified and the version does not satisfy it.
func ValidateCliVersion(cliVersion, constraint string) error {
	if constraint == "" {
		return nil
	}

	if cliVersion == "" {
		return nil
	}

	if cliVersion == "dev" || cliVersion == "main" || cliVersion == "latest" {
		return nil
	}

	if strings.HasPrefix(cliVersion, "nightly") {
		return nil
	}

	versionStr := strings.TrimPrefix(cliVersion, "v")
	version, err := semver.NewVersion(versionStr)
	if err != nil {
		return fmt.Errorf("invalid CLI version format '%s': %w", cliVersion, err)
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return fmt.Errorf("invalid cliVersion constraint '%s': %w", constraint, err)
	}

	releaseVersion, err := version.SetPrerelease("")
	if err != nil {
		return fmt.Errorf("invalid CLI version format '%s': %w", cliVersion, err)
	}

	if !c.Check(&releaseVersion) {
		return fmt.Errorf("CLI version %s does not satisfy required constraint '%s'", cliVersion, constraint)
	}

	return nil
}
