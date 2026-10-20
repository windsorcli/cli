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
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// CliVersionAnnotationKey mirrors BlueprintMetadataInput.CliVersion into the OCI manifest, so
// GetCliVersionConstraint can check it without a full Pull.
const CliVersionAnnotationKey = "windsorcli.dev/cli-version"

const ociRefFormatHint = "Use an OCI reference in the format oci://registry/repository:tag or oci://registry/repository@sha256:digest."

const blueprintVersionHint = "Add a version to the blueprint reference, for example core:v1.0.0."

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
// It accepts "oci://registry/repository:tag" or "oci://registry/repository@sha256:digest".
// The registry can have a port, so the repository and tag split on the last colon.
// It rejects a repository with a colon, because the split would be wrong.
// For a digest reference, tag is the bare "sha256:<hex>" string. FormatOCIRef adds the correct separator again.
func (a *ArtifactBuilder) ParseOCIRef(ociRef string) (registry, repository, tag string, err error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return "", "", "", werror.New(fmt.Sprintf("invalid OCI reference format: %s", ociRef), ociRefFormatHint, nil)
	}

	ref := strings.TrimPrefix(ociRef, "oci://")

	firstSlash := strings.Index(ref, "/")
	if firstSlash <= 0 {
		return "", "", "", werror.New(fmt.Sprintf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef), ociRefFormatHint, nil)
	}
	registry = ref[:firstSlash]
	repoAndVersion := ref[firstSlash+1:]

	if atIdx := strings.Index(repoAndVersion, "@sha256:"); atIdx >= 0 {
		repository = repoAndVersion[:atIdx]
		tag = repoAndVersion[atIdx+1:]
		if repository == "" || strings.Contains(repository, ":") {
			return "", "", "", werror.New(fmt.Sprintf("invalid OCI reference format, expected registry/repository@sha256:digest: %s", ociRef), ociRefFormatHint, nil)
		}
		return registry, repository, tag, nil
	}

	lastColon := strings.LastIndex(repoAndVersion, ":")
	if lastColon <= 0 || lastColon == len(repoAndVersion)-1 {
		return "", "", "", werror.New(fmt.Sprintf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef), ociRefFormatHint, nil)
	}
	repository = repoAndVersion[:lastColon]
	tag = repoAndVersion[lastColon+1:]
	if strings.Contains(repository, ":") {
		return "", "", "", werror.New(fmt.Sprintf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef), ociRefFormatHint, nil)
	}

	return registry, repository, tag, nil
}

// ListTags returns the tags of the repository of an OCI reference. It ignores the tag in ociRef.
// The upgrade of a source uses it to find the latest version.
// It returns an error if the reference is not valid or the registry is not available.
func (a *ArtifactBuilder) ListTags(ociRef string) ([]string, error) {
	registry, repository, _, err := a.ParseOCIRef(ociRef)
	if err != nil {
		return nil, err
	}
	repo, err := a.shims.NewRepository(registry + "/" + repository)
	if err != nil {
		return nil, werror.Wrap(err, "invalid repository %q/%q", registry, repository)
	}
	tags, err := a.shims.RemoteList(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, werror.Wrap(err, "failed to list tags for %s/%s", registry, repository)
	}
	return tags, nil
}

// GetCliVersionConstraint returns the cliVersion constraint of ociRef from the manifest annotation.
// It reads only the manifest and does not pull the layer. An empty string means the artifact sets no constraint.
// If the registry rejects the keychain credentials, it tries again without credentials.
func (a *ArtifactBuilder) GetCliVersionConstraint(ociRef string) (string, error) {
	registry, repository, tag, err := a.ParseOCIRef(ociRef)
	if err != nil {
		return "", err
	}

	ref := FormatOCIRef(registry, repository, tag)
	parsedRef, err := a.shims.ParseReference(ref)
	if err != nil {
		return "", werror.Wrap(err, "failed to parse reference %s", ref)
	}

	img, err := a.shims.RemoteImage(parsedRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil && IsAuthenticationError(err) {
		if anonImg, anonErr := a.shims.RemoteImage(parsedRef, remote.WithAuth(authn.Anonymous)); anonErr == nil {
			img, err = anonImg, nil
		}
	}
	if err != nil {
		return "", werror.Wrap(err, "failed to get image manifest for %s", ref)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return "", werror.Wrap(err, "failed to read manifest for %s", ref)
	}

	return manifest.Annotations[CliVersionAnnotationKey], nil
}

// VerifyCliVersionCompatibility pulls ociRef and checks the cliVersion constraint in its metadata.yaml.
// ResolveCompatibleTag uses it when the manifest of a tag has no constraint annotation.
// It returns nil if the running CLI satisfies the constraint or the artifact has no metadata.yaml.
// Any other error means that the tag is not safe to use.
func (a *ArtifactBuilder) VerifyCliVersionCompatibility(ociRef string) error {
	artifacts, err := a.Pull([]string{ociRef})
	if err != nil {
		return werror.Wrap(err, "failed to pull %s", ociRef)
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
		return werror.Wrap(err, "failed to read %s", metadataPath)
	}
	var meta BlueprintMetadataInput
	if err := a.shims.YamlUnmarshal(data, &meta); err != nil {
		return werror.Wrap(err, "failed to parse %s", metadataPath)
	}
	return ValidateCliVersion(constants.Version, meta.CliVersion)
}

// Push bundles the project files and pushes the artifact to registryBase/repoName:tag.
// It uploads each layer and the config blob that the registry does not have, then writes the manifest.
// This order prevents MANIFEST_BLOB_UNKNOWN errors from the registry.
// It uses the Docker keychain for credentials, and writes the manifest with a retry backoff.
func (a *ArtifactBuilder) Push(registryBase string, repoName string, tag string) error {
	if err := a.Bundle(); err != nil {
		return werror.Wrap(err, "failed to bundle files")
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
		return werror.Wrap(err, "failed to create tarball in memory")
	}

	layer := static.NewLayer(tarballContent, types.DockerLayer)

	img, err := a.createOCIArtifactImage(layer, finalName, tagName, cliVersionConstraint)
	if err != nil {
		return werror.Wrap(err, "failed to create OCI image")
	}

	repoURL := fmt.Sprintf("%s/%s", registryBase, repoName)
	if tagName != "" {
		repoURL = fmt.Sprintf("%s:%s", repoURL, tagName)
	}

	ref, err := name.ParseReference(repoURL)
	if err != nil {
		return werror.Wrap(err, "failed to parse repository reference")
	}

	manifest, err := img.Manifest()
	if err != nil {
		return werror.Wrap(err, "failed to get image manifest")
	}

	for _, layerDesc := range manifest.Layers {
		layer, err := img.LayerByDigest(layerDesc.Digest)
		if err != nil {
			return werror.Wrap(err, "failed to get layer %s", layerDesc.Digest)
		}

		blobRef := ref.Context().Digest(layerDesc.Digest.String())
		_, err = a.shims.RemoteGet(blobRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			err = a.shims.RemoteWriteLayer(ref.Context(), layer, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				return werror.Wrap(err, "failed to upload layer %s", layerDesc.Digest)
			}
		}
	}

	configDigest, err := img.ConfigName()
	if err != nil {
		return werror.Wrap(err, "failed to get config digest")
	}

	configRef := ref.Context().Digest(configDigest.String())
	_, err = a.shims.RemoteGet(configRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		configBlob, err := img.RawConfigFile()
		if err != nil {
			return werror.Wrap(err, "failed to get config file")
		}

		configLayer := static.NewLayer(configBlob, types.DockerConfigJSON)
		err = a.shims.RemoteWriteLayer(ref.Context(), configLayer, remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			return werror.Wrap(err, "failed to upload config")
		}
	}

	err = a.shims.RemoteWrite(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithRetryBackoff(remote.Backoff{
		Duration: 1.0,
		Factor:   3.0,
		Jitter:   0.1,
		Steps:    5,
	}))
	if err != nil {
		return werror.Wrap(err, "failed to push artifact to registry")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// createOCIArtifactImage makes an OCI image from the layer, with an OCI manifest and a Windsor blueprint config media type.
// The config sets linux/amd64, and the manifest annotations set the time, source, revision, title, and version.
// The source and revision come from git, and are "unknown" if git cannot supply them.
// If cliVersionConstraint is set, it also adds CliVersionAnnotationKey to the manifest.
// GetCliVersionConstraint reads that annotation without a pull.
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
		return nil, werror.Wrap(err, "failed to append layer to image")
	}

	img, err = a.shims.ConfigFile(img, configFile)
	if err != nil {
		return nil, werror.Wrap(err, "failed to set config file")
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

// downloadOCIArtifact downloads the OCI artifact at registry/repository:tag and returns the data of its first layer.
// It uses the Docker keychain for private registries. If the registry rejects those credentials, it tries again without credentials.
// The retry lets a public artifact pull when the Docker config has old or limited tokens.
// In verbose mode, a successful retry writes a note to stderr that names the registry.
// If the retry also fails, it returns the keychain error.
func (a *ArtifactBuilder) downloadOCIArtifact(registry, repository, tag string) ([]byte, error) {
	ref := FormatOCIRef(registry, repository, tag)

	parsedRef, err := a.shims.ParseReference(ref)
	if err != nil {
		return nil, werror.Wrap(err, "failed to parse reference %s", ref)
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
		return nil, werror.Wrap(err, "failed to get image")
	}

	layers, err := a.shims.ImageLayers(img)
	if err != nil {
		return nil, werror.Wrap(err, "failed to get image layers")
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no layers found in image")
	}

	layer := layers[0]
	reader, err := a.shims.LayerUncompressed(layer)
	if err != nil {
		return nil, werror.Wrap(err, "failed to get layer reader")
	}
	defer reader.Close()

	data, err := a.shims.ReadAll(reader)
	if err != nil {
		return nil, werror.Wrap(err, "failed to read layer data")
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

// ResolveCompatibleTag returns the highest stable semver tag that the running CLI is compatible with.
// For each tag, it reads the manifest constraint annotation first. If there is no annotation, it pulls the tag to check.
// Older tags do not have the annotation, so it cannot think that they are compatible.
// A manifest read error skips that tag. It returns the last error only if no tag is compatible.
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
			constraintCheckErr = werror.Wrap(err, "failed to check compatibility for %s", candidateRef)
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

// ParseOCIReference parses a blueprint reference into an OCIArtifactInfo.
// It accepts "oci://registry/org/repo:v1.0.0" and "org/repo:v1.0.0". It adds ghcr.io to an org/repo reference.
// Both formats accept "@sha256:<hex>" in place of the tag. Tag is then the digest without "@".
// It returns nil and no error for an empty reference. It returns an error if the reference has no version.
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
			return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
		}
		if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
			name = pathPart[lastSlash+1:]
		} else {
			return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
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
			return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
		}
		if strings.Count(pathPart, "/") >= 1 {
			if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
				name = pathPart[lastSlash+1:]
			} else {
				return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
			}
			fullURL = "oci://ghcr.io/" + ociRef
		} else {
			return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
		}
	}

	if version == "" || name == "" {
		return nil, werror.New(fmt.Sprintf("blueprint reference '%s' is missing a version", ociRef), blueprintVersionHint, nil)
	}

	return &OCIArtifactInfo{
		Name: name,
		URL:  fullURL,
		Tag:  version,
	}, nil
}

// ParseRegistryURL parses a registry URL into the registry base, the repository name, and the tag.
// It accepts "registry.com/repo:tag", "registry.com/repo", and "oci://registry.com/repo:tag".
// It returns an error if the URL has no repository path.
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
		return "", "", "", werror.New("invalid registry format: must include repository path", "Use a registry URL with a repository path, for example ghcr.io/namespace/repo.", nil)
	}

	return registryBase, repoName, tag, nil
}

// IsAuthenticationError reports whether err comes from a registry authentication or authorization failure.
// It looks for a go-containerregistry *transport.Error with HTTP 401 or 403, or an UNAUTHORIZED or DENIED code.
// It uses errors.As, so a werror.Wrap frame or a fmt.Errorf %w wrap does not hide the error.
// The push command uses it to tell the user to run `docker login`.
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

// ValidateCliVersion checks that cliVersion satisfies the cliVersion constraint of the blueprint metadata.
// It does not check when the constraint or cliVersion is empty, for the "dev", "main", or "latest" builds, or for a "nightly" build.
// A nightly build has a snapshot string that is not a semver version.
// It checks a pre-release version, such as "0.9.0-rc.1", as its release version.
// Masterminds/semver excludes pre-releases from a range that does not include one, so an rc build would always fail.
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
		return werror.Wrap(err, "invalid CLI version format '%s'", cliVersion)
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return werror.Wrap(err, "invalid cliVersion constraint '%s'", constraint)
	}

	releaseVersion, err := version.SetPrerelease("")
	if err != nil {
		return werror.Wrap(err, "invalid CLI version format '%s'", cliVersion)
	}

	if !c.Check(&releaseVersion) {
		return werror.New(fmt.Sprintf("CLI version %s does not satisfy required constraint '%s'", cliVersion, constraint),
			"Install a Windsor CLI version that satisfies the constraint, or use a blueprint version that supports this CLI version.", nil)
	}

	return nil
}
