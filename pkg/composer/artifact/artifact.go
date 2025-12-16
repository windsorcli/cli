package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/briandowns/spinner"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The ArtifactBuilder creates tar.gz artifacts from prepared build directories.
// It provides a unified interface for packaging prepared files into distributable artifacts
// without requiring git synchronization or validation. The ArtifactBuilder serves as the final
// step in the bundling pipeline, creating self-contained artifacts that include all bundled
// dependencies and metadata for distribution.

// =============================================================================
// Interfaces
// =============================================================================

// Artifact defines the interface for artifact creation operations
type Artifact interface {
	Bundle() error
	Write(outputPath string, tag string) (string, error)
	Push(registryBase string, repoName string, tag string) error
	Pull(ociRefs []string) (map[string][]byte, error)
	GetTemplateData(ociRef string) (map[string][]byte, error)
	ParseOCIRef(ociRef string) (registry, repository, tag string, err error)
	GetCacheDir(registry, repository, tag string) (string, error)
}

// =============================================================================
// Types
// =============================================================================

// BlueprintMetadata represents the complete metadata embedded in artifacts
type BlueprintMetadata struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Version     string        `json:"version,omitempty"`
	Author      string        `json:"author,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Homepage    string        `json:"homepage,omitempty"`
	License     string        `json:"license,omitempty"`
	CliVersion  string        `json:"cliVersion,omitempty"`
	Timestamp   string        `json:"timestamp"`
	Git         GitProvenance `json:"git"`
	Builder     BuilderInfo   `json:"builder"`
}

// GitProvenance contains git repository information for traceability
type GitProvenance struct {
	CommitSHA string `json:"commitSHA"`
	Tag       string `json:"tag,omitempty"`
	RemoteURL string `json:"remoteURL"`
}

// BuilderInfo contains information about who/what built the artifact
type BuilderInfo struct {
	User  string `json:"user"`
	Email string `json:"email"`
}

// OCIArtifactInfo contains information about the OCI artifact source for blueprint data
type OCIArtifactInfo struct {
	Name string
	URL  string
	Tag  string
}

// BlueprintMetadataInput represents the input metadata from contexts/_template/metadata.yaml
type BlueprintMetadataInput struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Version     string   `yaml:"version,omitempty"`
	Author      string   `yaml:"author,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty"`
	License     string   `yaml:"license,omitempty"`
	CliVersion  string   `yaml:"cliVersion,omitempty"`
}

// FileInfo holds file content and permission information
type FileInfo struct {
	Content []byte
	Mode    os.FileMode
}

// PathProcessor defines a processor for files matching a specific path pattern
type PathProcessor struct {
	Pattern string
	Handler func(relPath string, data []byte, mode os.FileMode) error
}

// ArtifactBuilder implements the Artifact interface
type ArtifactBuilder struct {
	files       map[string]FileInfo
	shims       *Shims
	shell       shell.Shell
	runtime     *runtime.Runtime
	tarballPath string
	metadata    BlueprintMetadataInput
	ociCache    map[string][]byte
}

const ociExtractionCompleteMarker = ".windsor_extraction_complete"

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactBuilder creates a new ArtifactBuilder instance with the provided shell dependency.
// If overrides are provided, any non-nil component in the override ArtifactBuilder will be used instead of creating a default.
// The shell is used for retrieving git provenance and builder information during metadata generation.
func NewArtifactBuilder(rt *runtime.Runtime) *ArtifactBuilder {
	builder := &ArtifactBuilder{
		shims:    NewShims(),
		files:    make(map[string]FileInfo),
		ociCache: make(map[string][]byte),
		shell:    rt.Shell,
		runtime:  rt,
	}

	return builder
}

// =============================================================================
// Public Methods
// =============================================================================

// Write bundles all files and creates a compressed tar.gz artifact file with optional tag override.
// Accepts optional tag in "name:version" format to override metadata.yaml values.
// Tag takes precedence over existing metadata. If no metadata.yaml exists, tag is required.
// OutputPath can be file or directory - generates filename from metadata if directory.
// Creates compressed tar.gz with all files including enriched metadata.yaml.
// Returns the final output path of the created artifact file.
func (a *ArtifactBuilder) Write(outputPath string, tag string) (string, error) {
	if err := a.Bundle(); err != nil {
		return "", fmt.Errorf("failed to bundle files: %w", err)
	}

	finalName, finalVersion, metadata, err := a.parseTagAndResolveMetadata("", tag)
	if err != nil {
		return "", err
	}

	finalOutputPath := a.resolveOutputPath(outputPath, finalName, finalVersion)

	err = a.createTarballToDisk(finalOutputPath, metadata)
	if err != nil {
		return "", err
	}

	a.tarballPath = finalOutputPath
	a.metadata.Name = finalName
	a.metadata.Version = finalVersion

	return finalOutputPath, nil
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

	finalName, tagName, metadata, err := a.parseTagAndResolveMetadata(repoName, tag)
	if err != nil {
		return err
	}

	tarballContent, err := a.createTarballInMemory(metadata)
	if err != nil {
		return fmt.Errorf("failed to create tarball in memory: %w", err)
	}

	layer := static.NewLayer(tarballContent, types.DockerLayer)

	img, err := a.createOCIArtifactImage(layer, finalName, tagName)
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

// Pull downloads and extracts OCI artifacts in memory for use by other components.
// It takes a slice of OCI references and downloads unique artifacts, returning a map
// of cached artifacts keyed by their registry/repository:tag identifier.
// The method provides efficient caching to avoid duplicate downloads of the same artifact.
// It first checks the in-memory cache, then the disk cache, and only downloads if neither contains the artifact.
func (a *ArtifactBuilder) Pull(ociRefs []string) (map[string][]byte, error) {
	ociArtifacts := make(map[string][]byte)

	uniqueOCIRefs := make(map[string]bool)
	for _, ref := range ociRefs {
		if strings.HasPrefix(ref, "oci://") {
			uniqueOCIRefs[ref] = true
		}
	}

	if len(uniqueOCIRefs) == 0 {
		return ociArtifacts, nil
	}

	var artifactsToDownload []string
	for ref := range uniqueOCIRefs {
		registry, repository, tag, err := a.ParseOCIRef(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
		}

		cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

		if cachedData, exists := a.ociCache[cacheKey]; exists {
			ociArtifacts[cacheKey] = cachedData
			continue
		}

		cacheDir, err := a.GetCacheDir(registry, repository, tag)
		if err == nil {
			if err := a.validateOCIDiskCache(cacheDir); err == nil {
				ociArtifacts[cacheKey] = nil
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				if removeErr := a.shims.RemoveAll(cacheDir); removeErr != nil {
					return nil, fmt.Errorf("failed to remove corrupted OCI cache directory %s: %w", cacheDir, removeErr)
				}
			}
		}

		artifactsToDownload = append(artifactsToDownload, ref)
	}

	if len(artifactsToDownload) > 0 {
		message := "📦 Loading OCI sources"
		spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
		spin.Suffix = " " + message
		spin.Start()

		defer func() {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
		}()

		for _, ref := range artifactsToDownload {
			registry, repository, tag, err := a.ParseOCIRef(ref)
			if err != nil {
				return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
			}

			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

			artifactData, err := a.downloadOCIArtifact(registry, repository, tag)
			if err != nil {
				return nil, fmt.Errorf("failed to download OCI artifact %s: %w", ref, err)
			}

			if err := a.extractArtifactToCache(artifactData, registry, repository, tag); err != nil {
				return nil, fmt.Errorf("failed to extract artifact to cache: %w", err)
			}

			a.ociCache[cacheKey] = artifactData
			ociArtifacts[cacheKey] = artifactData
		}
	}

	return ociArtifacts, nil
}

// validateOCIDiskCache validates that an extracted OCI artifact cache directory is complete and safe to use.
// It requires the extraction-complete marker file to exist (written atomically during extraction) and does not
// treat metadata.yaml alone as sufficient, which prevents partial extraction states from being mistaken as valid cache hits.
func (a *ArtifactBuilder) validateOCIDiskCache(cacheDir string) error {
	if _, err := a.shims.Stat(cacheDir); err != nil {
		return err
	}

	markerPath := filepath.Join(cacheDir, ociExtractionCompleteMarker)
	if _, err := a.shims.Stat(markerPath); err != nil {
		return err
	}

	return nil
}

// GetTemplateData extracts and returns template data from an OCI artifact reference or local .tar.gz file.
// For OCI references (oci://...), first checks the local .oci_extracted cache on disk, then downloads if needed.
// For local .tar.gz files, reads from disk.
// Returns a map with all files from the _template directory using their relative paths as keys.
// Files are stored with their relative paths from _template (e.g., "_template/schema.yaml", "_template/blueprint.yaml", "_template/features/base.yaml").
// All files from _template/ are included - no filtering is performed.
// The metadata name is extracted and stored in the returned map with the key "_metadata_name".
// Returns an error on invalid reference, download failure, file read failure, or extraction error.
func (a *ArtifactBuilder) GetTemplateData(blueprintRef string) (map[string][]byte, error) {
	if strings.HasPrefix(blueprintRef, "oci://") {
		registry, repository, tag, err := a.ParseOCIRef(blueprintRef)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OCI reference %s: %w", blueprintRef, err)
		}

		templateData, err := a.getTemplateDataFromCache(registry, repository, tag)
		if err != nil {
			return nil, fmt.Errorf("cache validation failed: %w", err)
		}
		if templateData != nil {
			return templateData, nil
		}

		artifacts, err := a.Pull([]string{blueprintRef})
		if err != nil {
			return nil, fmt.Errorf("failed to pull OCI artifact %s: %w", blueprintRef, err)
		}

		cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
		tarData, exists := artifacts[cacheKey]
		if !exists {
			return nil, fmt.Errorf("failed to retrieve artifact data for %s", blueprintRef)
		}

		if tarData == nil {
			templateData, cacheErr := a.getTemplateDataFromCache(registry, repository, tag)
			if cacheErr != nil {
				return nil, fmt.Errorf("cache validation failed: %w", cacheErr)
			}
			if templateData != nil {
				return templateData, nil
			}
			return nil, fmt.Errorf("OCI artifact marked as cached but no valid template cache found for %s", blueprintRef)
		}

		if err := a.extractArtifactToCache(tarData, registry, repository, tag); err != nil {
			return nil, fmt.Errorf("failed to extract artifact to cache: %w", err)
		}

		return a.extractTemplateDataFromTar(tarData)
	}

	if strings.HasSuffix(blueprintRef, ".tar.gz") {
		compressedData, err := a.shims.ReadFile(blueprintRef)
		if err != nil {
			return nil, fmt.Errorf("failed to read local artifact file %s: %w", blueprintRef, err)
		}

		gzipReader, err := gzip.NewReader(bytes.NewReader(compressedData))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader for %s: %w", blueprintRef, err)
		}
		defer gzipReader.Close()

		tarData, err := a.shims.ReadAll(gzipReader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress artifact file %s: %w", blueprintRef, err)
		}

		return a.extractTemplateDataFromTar(tarData)
	}

	return nil, fmt.Errorf("invalid blueprint reference: %s (must be oci://... or path to .tar.gz file)", blueprintRef)
}

// extractArtifactToCache extracts the full OCI artifact tar archive to the disk cache keyed by registry, repository, and tag.
// It unpacks the contents into a dedicated extraction directory under the project root, preserving permissions and handling executables.
// Extraction is atomic: all files are first unpacked to a temporary directory, then renamed into place on success, which avoids leaving partial state.
// This approach ensures all artifact files are available for subsequent module and feature resolutions and enables cache reuse.
// Returns an error if any extraction phase fails or if path validation checks do not pass.
func (a *ArtifactBuilder) extractArtifactToCache(artifactData []byte, registry, repository, tag string) (err error) {
	projectRoot := a.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("failed to get project root: project root is empty")
	}

	reader := a.shims.NewBytesReader(artifactData)
	tarReader := a.shims.NewTarReader(reader)

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
	extractionDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)
	tmpExtractionDir := extractionDir + ".tmp"

	defer func() {
		if err != nil {
			if _, statErr := a.shims.Stat(tmpExtractionDir); statErr == nil {
				if removeErr := a.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to clean up temporary extraction directory: %v\n", removeErr)
				}
			}
		}
	}()

	if _, statErr := a.shims.Stat(tmpExtractionDir); statErr == nil {
		if removeErr := a.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
			return fmt.Errorf("failed to clean up existing temporary extraction directory: %w", removeErr)
		}
	}

	if mkdirErr := a.shims.MkdirAll(tmpExtractionDir, 0755); mkdirErr != nil {
		return fmt.Errorf("failed to create temporary extraction directory: %w", mkdirErr)
	}

	if err := a.extractTarEntries(tarReader, tmpExtractionDir); err != nil {
		return err
	}

	markerPath := filepath.Join(tmpExtractionDir, ociExtractionCompleteMarker)
	markerFile, markerErr := a.shims.Create(markerPath)
	if markerErr != nil {
		return fmt.Errorf("failed to create extraction marker file: %w", markerErr)
	}
	if _, writeErr := markerFile.Write([]byte("ok\n")); writeErr != nil {
		_ = markerFile.Close()
		return fmt.Errorf("failed to write extraction marker file: %w", writeErr)
	}
	if closeErr := markerFile.Close(); closeErr != nil {
		return fmt.Errorf("failed to close extraction marker file: %w", closeErr)
	}

	parentDir := filepath.Dir(extractionDir)
	if mkdirErr := a.shims.MkdirAll(parentDir, 0755); mkdirErr != nil {
		return fmt.Errorf("failed to create parent directory: %w", mkdirErr)
	}

	if _, statErr := a.shims.Stat(extractionDir); statErr == nil {
		if removeErr := a.shims.RemoveAll(extractionDir); removeErr != nil {
			return fmt.Errorf("failed to remove existing extraction directory: %w", removeErr)
		}
	}

	if renameErr := a.shims.Rename(tmpExtractionDir, extractionDir); renameErr != nil {
		return fmt.Errorf("failed to rename temporary extraction directory to final location: %w", renameErr)
	}

	return nil
}

// GetCacheDir returns the cache directory path for an OCI artifact identified by registry, repository, and tag.
// The cache directory is located at <projectRoot>/.windsor/.oci_extracted/<extractionKey> where extractionKey
// is the cacheKey with / and : replaced with _ for filesystem safety.
// Returns an error if project root is empty.
func (a *ArtifactBuilder) GetCacheDir(registry, repository, tag string) (string, error) {
	projectRoot := a.runtime.ProjectRoot
	if projectRoot == "" {
		return "", fmt.Errorf("failed to get project root: project root is empty")
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
	cacheDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)

	return cacheDir, nil
}

// ParseOCIRef parses an OCI reference into registry, repository, and tag components.
// Validates OCI reference format and extracts registry, repository, and tag parts.
// Requires OCI reference to follow the format "oci://registry/repository:tag".
// Returns individual components for separate handling in OCI operations.
func (a *ArtifactBuilder) ParseOCIRef(ociRef string) (registry, repository, tag string, err error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
	}

	ref := strings.TrimPrefix(ociRef, "oci://")

	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	repoWithRegistry := parts[0]
	tag = parts[1]

	repoParts := strings.SplitN(repoWithRegistry, "/", 2)
	if len(repoParts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	registry = repoParts[0]
	repository = repoParts[1]

	return registry, repository, tag, nil
}

// Bundle traverses the project directories and collects all relevant files to be
// included in the artifact. It applies configurable path-based processors that determine
// how files from each directory (such as "_template", "kustomize", or "terraform") are
// incorporated into the artifact. The method supports extensibility by allowing custom
// handling of different directory structures and types, and skips files in the "terraform"
// directory based on predefined logic.
//
// Returns an error if any file processing or traversal fails.
func (a *ArtifactBuilder) Bundle() error {
	processors := []PathProcessor{
		{
			Pattern: "contexts/_template",
			Handler: func(relPath string, data []byte, mode os.FileMode) error {
				artifactPath := "_template/" + filepath.ToSlash(relPath)
				return a.addFile(artifactPath, data, mode)
			},
		},
		{
			Pattern: "kustomize",
			Handler: func(relPath string, data []byte, mode os.FileMode) error {
				artifactPath := "kustomize/" + filepath.ToSlash(relPath)
				return a.addFile(artifactPath, data, mode)
			},
		},
		{
			Pattern: "terraform",
			Handler: func(relPath string, data []byte, mode os.FileMode) error {
				if a.shouldSkipTerraformFile(filepath.Base(relPath)) {
					return nil
				}
				artifactPath := "terraform/" + filepath.ToSlash(relPath)
				return a.addFile(artifactPath, data, mode)
			},
		},
	}

	return a.walkAndProcessFiles(processors)
}

// =============================================================================
// Package Functions
// =============================================================================

// ParseOCIReference parses a blueprint reference string in OCI URL or org/repo:tag format and returns an OCIArtifactInfo struct.
// Accepts full OCI URLs (e.g., oci://ghcr.io/org/repo:v1.0.0) and org/repo:v1.0.0 formats only.
// Returns nil if the reference is empty, missing a version, or not in a supported format.
func ParseOCIReference(ociRef string) (*OCIArtifactInfo, error) {
	if ociRef == "" {
		return nil, nil
	}

	var name, version, fullURL string

	if strings.HasPrefix(ociRef, "oci://") {
		fullURL = ociRef
		remaining := strings.TrimPrefix(ociRef, "oci://")
		if lastColon := strings.LastIndex(remaining, ":"); lastColon > 0 {
			version = remaining[lastColon+1:]
			pathPart := remaining[:lastColon]
			if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
				name = pathPart[lastSlash+1:]
			} else {
				return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
			}
		} else {
			return nil, fmt.Errorf("blueprint reference '%s' is missing a version (e.g., core:v1.0.0)", ociRef)
		}
	} else {
		if colonIdx := strings.LastIndex(ociRef, ":"); colonIdx > 0 {
			pathPart := ociRef[:colonIdx]
			version = ociRef[colonIdx+1:]
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

// IsAuthenticationError checks if the error is related to authentication failure.
// It examines common authentication error patterns in error messages to determine
// if the failure is due to authentication issues rather than other problems.
func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	authErrorPatterns := []string{
		"UNAUTHORIZED",
		"unauthorized",
		"authentication required",
		"authentication failed",
		"not authorized",
		"access denied",
		"login required",
		"credentials required",
		"401",
		"403",
		"unauthenticated",
		"User cannot be authenticated",
		"failed to push artifact",
		"POST https://",
		"blobs/uploads",
	}

	for _, pattern := range authErrorPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// ValidateCliVersion validates that the provided CLI version satisfies the cliVersion constraint
// specified in the template metadata. If constraint is empty, validation is skipped.
// If cliVersion is empty, validation is skipped (caller cannot determine version).
// If the CLI version is "dev" or "main" or "latest", validation is skipped as these are development builds.
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

	versionStr := strings.TrimPrefix(cliVersion, "v")
	version, err := semver.NewVersion(versionStr)
	if err != nil {
		return fmt.Errorf("invalid CLI version format '%s': %w", cliVersion, err)
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return fmt.Errorf("invalid cliVersion constraint '%s': %w", constraint, err)
	}

	if !c.Check(version) {
		return fmt.Errorf("CLI version %s does not satisfy required constraint '%s'", cliVersion, constraint)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// addFile stores a file with the specified path and content in the artifact for later packaging.
// Files are held in memory until create() or Push() is called. The path becomes the relative
// path within the generated tar.gz archive. Multiple calls with the same path will overwrite
// the previous content. Special handling exists for "_template/metadata.yaml" during packaging.
func (a *ArtifactBuilder) addFile(path string, content []byte, mode os.FileMode) error {
	a.files[path] = FileInfo{
		Content: content,
		Mode:    mode,
	}
	return nil
}

// walkAndProcessFiles traverses the "contexts", "kustomize", and "terraform" directories and processes
// all files found using the provided list of PathProcessors. For each file, the method identifies the
// corresponding processor (if any) by matching patterns and invokes its Handler with the file's relative
// path, data, and permissions. Non-existent directories are skipped. Any ".terraform" directories are
// skipped during traversal. If any error occurs while reading files, obtaining relative paths, or while
// invoking a processor, the error is returned and processing halts.
func (a *ArtifactBuilder) walkAndProcessFiles(processors []PathProcessor) error {
	dirSet := make(map[string]bool)
	for _, processor := range processors {
		dir := strings.Split(processor.Pattern, "/")[0]
		dirSet[dir] = true
	}

	for dir := range dirSet {
		if _, err := a.shims.Stat(dir); os.IsNotExist(err) {
			continue
		}

		if err := a.shims.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if info.Name() == ".terraform" {
					return filepath.SkipDir
				}
				return nil
			}

			processor := a.findMatchingProcessor(path, processors)
			if processor == nil {
				return nil
			}

			data, err := a.shims.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			relPath, err := a.shims.FilepathRel(processor.Pattern, path)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			return processor.Handler(relPath, data, info.Mode())
		}); err != nil {
			return fmt.Errorf("failed to walk directory %s: %w", dir, err)
		}
	}

	return nil
}

// findMatchingProcessor finds the first processor whose pattern matches the given path
func (a *ArtifactBuilder) findMatchingProcessor(path string, processors []PathProcessor) *PathProcessor {
	for _, processor := range processors {
		if strings.HasPrefix(path, processor.Pattern) {
			return &processor
		}
	}
	return nil
}

// shouldSkipTerraformFile determines if a terraform file should be excluded from bundling
func (a *ArtifactBuilder) shouldSkipTerraformFile(filename string) bool {
	if strings.HasSuffix(filename, "_override.tf") ||
		strings.HasSuffix(filename, "_override.tf.json") ||
		filename == "override.tf" ||
		filename == "override.tf.json" {
		return true
	}

	if strings.HasSuffix(filename, ".tfstate") ||
		strings.Contains(filename, ".tfstate.") {
		return true
	}

	if strings.HasSuffix(filename, ".tfvars") ||
		strings.HasSuffix(filename, ".tfvars.json") {
		return true
	}

	if strings.HasSuffix(filename, ".tfplan") {
		return true
	}

	if filename == ".terraformrc" || filename == "terraform.rc" {
		return true
	}

	if filename == "crash.log" || (strings.HasPrefix(filename, "crash.") && strings.HasSuffix(filename, ".log")) {
		return true
	}

	return false
}

// parseTagAndResolveMetadata extracts name and version from tag parameter or metadata file and generates final metadata.
// For Create method (repoName is empty): tag can be "name:version" format or empty to use metadata.yaml
// For Push method (repoName provided): tag is version only, repoName is used as fallback name
// Loads existing metadata.yaml from files if present and parses it as BlueprintMetadataInput.
// Tag parameter takes precedence over metadata file values for version and/or name.
// Returns final name, version, and complete marshaled metadata ready for embedding in artifacts.
func (a *ArtifactBuilder) parseTagAndResolveMetadata(repoName, tag string) (string, string, []byte, error) {
	var tagName, tagVersion string

	if repoName == "" {
		if tag != "" {
			if strings.Contains(tag, ":") {
				parts := strings.Split(tag, ":")
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					return "", "", nil, fmt.Errorf("tag must be in format 'name:version', got: %s", tag)
				}
				tagName = parts[0]
				tagVersion = parts[1]
			} else {
				tagVersion = tag
			}
		}
	} else {
		if tag != "" {
			tagVersion = tag
		}
	}

	metadataFileInfo, hasMetadata := a.files["_template/metadata.yaml"]
	var input BlueprintMetadataInput

	if hasMetadata {
		if err := a.shims.YamlUnmarshal(metadataFileInfo.Content, &input); err != nil {
			return "", "", nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
		}
		if err := ValidateCliVersion(constants.Version, input.CliVersion); err != nil {
			return "", "", nil, err
		}
	}

	finalName := input.Name
	finalVersion := input.Version

	if tagName != "" {
		finalName = tagName
	}
	if tagVersion != "" {
		finalVersion = tagVersion
	}

	if finalName == "" && repoName != "" {
		finalName = repoName
	}

	if finalName == "" {
		if repoName == "" {
			return "", "", nil, fmt.Errorf("name is required: provide via tag parameter or metadata.yaml")
		} else {
			return "", "", nil, fmt.Errorf("name is required: provide via metadata.yaml")
		}
	}
	if finalVersion == "" {
		return "", "", nil, fmt.Errorf("version is required: provide via tag parameter or metadata.yaml")
	}

	metadata, err := a.generateMetadata(input, finalName, finalVersion)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate metadata: %w", err)
	}

	return finalName, finalVersion, metadata, nil
}

// generateMetadata creates enriched metadata by merging input metadata with generated values.
// Input metadata from _template/metadata.yaml provides base values, which are then enriched with git provenance,
// builder info, and timestamp. Generated values (name, version, timestamp, git, builder) overwrite input values.
// Returns marshaled YAML bytes ready for inclusion in tar archives as metadata.yaml at the root.
func (a *ArtifactBuilder) generateMetadata(input BlueprintMetadataInput, name, version string) ([]byte, error) {
	gitProvenance, err := a.getGitProvenance()
	if err != nil {
		gitProvenance = GitProvenance{}
	}

	builderInfo, err := a.getBuilderInfo()
	if err != nil {
		builderInfo = BuilderInfo{}
	}

	metadata := BlueprintMetadata{
		Name:        name,
		Description: input.Description,
		Version:     version,
		Author:      input.Author,
		Tags:        input.Tags,
		Homepage:    input.Homepage,
		License:     input.License,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Git:         gitProvenance,
		Builder:     builderInfo,
	}

	if input.CliVersion != "" {
		metadata.CliVersion = input.CliVersion
	}

	return a.shims.YamlMarshal(metadata)
}

// createTarball writes a compressed tar.gz archive to the provided writer.
// Creates a gzip-compressed tar archive containing all stored files from a.files.
// Writes metadata.yaml at the root of the archive (generated from _template/metadata.yaml if present).
// Skips any existing "_template/metadata.yaml" file from the input files to avoid duplication.
// All files are written with their stored permissions in the archive.
// Properly closes writers to ensure all data is flushed before returning.
func (a *ArtifactBuilder) createTarball(w io.Writer, metadata []byte) error {
	gzipWriter := a.shims.NewGzipWriter(w)
	defer gzipWriter.Close()

	tarWriter := a.shims.NewTarWriter(gzipWriter)
	defer tarWriter.Close()

	metadataHeader := &tar.Header{
		Name: "metadata.yaml",
		Mode: 0644,
		Size: int64(len(metadata)),
	}

	if err := tarWriter.WriteHeader(metadataHeader); err != nil {
		return fmt.Errorf("failed to write metadata header: %w", err)
	}

	if _, err := tarWriter.Write(metadata); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	for path, fileInfo := range a.files {
		if path == "_template/metadata.yaml" {
			continue
		}

		header := &tar.Header{
			Name: path,
			Mode: int64(fileInfo.Mode),
			Size: int64(len(fileInfo.Content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", path, err)
		}

		if _, err := tarWriter.Write(fileInfo.Content); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", path, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return nil
}

// createTarballInMemory builds a compressed tar.gz archive in memory and returns the complete content as bytes.
// Creates a gzip-compressed tar archive containing all stored files from a.files.
// Writes metadata.yaml at the root of the archive (generated from _template/metadata.yaml if present).
// All files are written with their stored permissions in the archive.
// Returns the complete archive as a byte slice for in-memory operations like OCI push.
func (a *ArtifactBuilder) createTarballInMemory(metadata []byte) ([]byte, error) {
	var buf bytes.Buffer

	if err := a.createTarball(&buf, metadata); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// createTarballToDisk builds a compressed tar.gz archive and writes it directly to the specified file path.
// Creates the output file at the specified path and writes a gzip-compressed tar archive.
// Writes metadata.yaml at the root of the archive (generated from _template/metadata.yaml if present).
// Skips any existing "_template/metadata.yaml" file from the input files to avoid duplication.
// All files are written with their stored permissions in the archive.
// Properly closes writers to ensure all data is flushed to disk before returning.
func (a *ArtifactBuilder) createTarballToDisk(outputPath string, metadata []byte) error {
	outputFile, err := a.shims.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	return a.createTarball(outputFile, metadata)
}

// createOCIArtifactImage constructs an OCI image from a layer with generic OCI artifact media types and annotations.
// Creates a generic OCI artifact config file compatible with both FluxCD and blueprint consumers.
// Sets architecture to amd64 and OS to linux for compatibility with container runtimes.
// Uses standard OCI media types with optional artifactType for tool-specific identification.
// Adds comprehensive OCI annotations including creation time, source, revision, title, and version.
// Returns a complete OCI image ready for pushing to any OCI 1.1 compatible registry.
func (a *ArtifactBuilder) createOCIArtifactImage(layer v1.Layer, repoName, tagName string) (v1.Image, error) {
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

	img = a.shims.Annotations(img, annotations)

	return img, nil
}

// resolveOutputPath determines the final output file path based on the provided path and artifact metadata.
// If outputPath is a directory or ends with slash, generates filename from name and version in that directory.
// If outputPath appears to be a directory path (no extension), generates filename from name and version.
// Otherwise uses outputPath as-is for the filename (user provided explicit filename).
// Generated filenames follow the pattern: {name}-{version}.tar.gz.
func (a *ArtifactBuilder) resolveOutputPath(outputPath, name, version string) string {
	filename := fmt.Sprintf("%s-%s.tar.gz", name, version)

	if stat, err := a.shims.Stat(outputPath); err == nil && stat.IsDir() {
		return filepath.Join(outputPath, filename)
	}

	if strings.HasSuffix(outputPath, "/") {
		return filepath.Join(outputPath, filename)
	}

	if filepath.Ext(outputPath) == "" {
		return filepath.Join(outputPath, filename)
	}

	return outputPath
}

// getGitProvenance retrieves git repository information including commit SHA, tag, and remote URL.
// Extracts git repository information for provenance tracking using shell commands.
// Requires shell dependency to be available for git command execution.
// Gets current commit SHA, attempts to find exact tag match for HEAD, and retrieves origin URL.
// Tag lookup uses exact match only - returns empty string if HEAD is not tagged.
// All git operations return errors if commands fail, unlike the best-effort approach in some functions.
func (a *ArtifactBuilder) getGitProvenance() (GitProvenance, error) {
	commitSHA, err := a.shell.ExecSilent("git", "rev-parse", "HEAD")
	if err != nil {
		return GitProvenance{}, fmt.Errorf("failed to get commit SHA: %w", err)
	}

	tag, _ := a.shell.ExecSilent("git", "describe", "--tags", "--exact-match", "HEAD")

	remoteURL, err := a.shell.ExecSilent("git", "config", "--get", "remote.origin.url")
	if err != nil {
		remoteURL = ""
	}

	return GitProvenance{
		CommitSHA: strings.TrimSpace(commitSHA),
		Tag:       strings.TrimSpace(tag),
		RemoteURL: strings.TrimSpace(remoteURL),
	}, nil
}

// getBuilderInfo retrieves information about the current user building the artifact.
// Extracts information about who/what built the artifact using git configuration or environment variables.
// First attempts to get user.name and user.email from git config (checks local config first, then global).
// If git config is not available (common in CI/CD environments), falls back to common environment variables:
// - USER, USERNAME, or GIT_AUTHOR_NAME for user name
// - EMAIL, GIT_AUTHOR_EMAIL, or GIT_COMMITTER_EMAIL for email
// Returns empty strings for missing configuration rather than errors for optional builder info.
// Used for audit trails and artifact attribution in generated metadata.
func (a *ArtifactBuilder) getBuilderInfo() (BuilderInfo, error) {
	var user, email string

	getEnvVar := func(vars ...string) string {
		for _, v := range vars {
			if val := os.Getenv(v); val != "" {
				return strings.TrimSpace(val)
			}
		}
		return ""
	}

	gitUser, err := a.shell.ExecSilent("git", "config", "--get", "user.name")
	if err == nil && strings.TrimSpace(gitUser) != "" {
		user = strings.TrimSpace(gitUser)
	} else {
		user = getEnvVar("USER", "USERNAME", "GIT_AUTHOR_NAME")
	}

	gitEmail, err := a.shell.ExecSilent("git", "config", "--get", "user.email")
	if err == nil && strings.TrimSpace(gitEmail) != "" {
		email = strings.TrimSpace(gitEmail)
	} else {
		email = getEnvVar("EMAIL", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL")
	}

	return BuilderInfo{
		User:  user,
		Email: email,
	}, nil
}

// getTemplateDataFromCache attempts to read template data from the local .oci_extracted cache.
// Returns the template data if found in cache, or nil if not cached (without error).
// Returns an error only if there's a problem reading the cache (not if cache doesn't exist).
func (a *ArtifactBuilder) getTemplateDataFromCache(registry, repository, tag string) (map[string][]byte, error) {
	projectRoot := a.runtime.ProjectRoot

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	extractionKey := strings.ReplaceAll(strings.ReplaceAll(cacheKey, "/", "_"), ":", "_")
	cacheDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil, nil
	}

	if err := a.validateOCIDiskCache(cacheDir); err != nil {
		if removeErr := a.shims.RemoveAll(cacheDir); removeErr != nil {
			return nil, fmt.Errorf("failed to remove corrupted OCI cache directory %s: %w", cacheDir, removeErr)
		}
		return nil, nil
	}

	templateData := make(map[string][]byte)
	templateDir := filepath.Join(cacheDir, "_template")

	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		if removeErr := a.shims.RemoveAll(cacheDir); removeErr != nil {
			return nil, fmt.Errorf("failed to remove corrupted OCI cache directory %s: %w", cacheDir, removeErr)
		}
		return nil, nil
	}

	root, err := os.OpenRoot(templateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open template root: %w", err)
	}
	defer root.Close()

	err = filepath.Walk(templateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(cacheDir, path)
		if err != nil {
			return err
		}

		templateRelPath, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}

		content, err := root.ReadFile(filepath.ToSlash(templateRelPath))
		if err != nil {
			return err
		}

		templateData[filepath.ToSlash(relPath)] = content
		return nil
	})

	if err != nil {
		return nil, err
	}

	cacheRoot, err := os.OpenRoot(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache root: %w", err)
	}
	defer cacheRoot.Close()

	metadataContent, err := cacheRoot.ReadFile("metadata.yaml")
	if err != nil {
		return nil, fmt.Errorf("OCI artifact missing required metadata.yaml file: %w", err)
	}

	var metadata BlueprintMetadata
	if err := a.shims.YamlUnmarshal(metadataContent, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
	}

	if err := ValidateCliVersion(constants.Version, metadata.CliVersion); err != nil {
		return nil, err
	}

	a.addMetadataToTemplateData(templateData, metadata)

	if len(templateData) == 0 {
		return nil, nil
	}

	return templateData, nil
}

// addMetadataToTemplateData adds metadata fields to the template data map.
// Extracts common metadata fields (name, version, description, author) from BlueprintMetadata
// and stores them in the template data map with standardized keys for use by blueprint handlers.
func (a *ArtifactBuilder) addMetadataToTemplateData(templateData map[string][]byte, metadata BlueprintMetadata) {
	if metadata.Name != "" {
		templateData["_metadata_name"] = []byte(metadata.Name)
	}
	if metadata.Version != "" {
		templateData["_metadata_version"] = []byte(metadata.Version)
	}
	if metadata.Description != "" {
		templateData["_metadata_description"] = []byte(metadata.Description)
	}
	if metadata.Author != "" {
		templateData["_metadata_author"] = []byte(metadata.Author)
	}
}

// extractTarEntries extracts all entries from a tar archive to the specified destination directory.
// It handles directories, files, path validation, permission setting, and executable bit handling for shell scripts.
// The destination directory must already exist. Returns an error if any entry extraction fails.
func (a *ArtifactBuilder) extractTarEntries(tarReader TarReader, destDir string) error {
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		sanitizedPath, err := a.validateAndSanitizePath(header.Name)
		if err != nil {
			return fmt.Errorf("invalid path in tar archive: %w", err)
		}

		destPath := filepath.Join(destDir, sanitizedPath)

		if !strings.HasPrefix(destPath, destDir) {
			return fmt.Errorf("path traversal attempt detected: %s", header.Name)
		}

		if header.Typeflag == tar.TypeDir {
			if err := a.shims.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		if err := a.shims.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		file, err := a.shims.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		_, err = a.shims.Copy(file, tarReader)
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("failed to close file %s: %w", destPath, closeErr)
		}
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		modeValue := header.Mode & 0777
		if modeValue < 0 || modeValue > 0777 {
			return fmt.Errorf("invalid file mode %o for %s", header.Mode, destPath)
		}
		fileMode := os.FileMode(uint32(modeValue))

		if strings.HasSuffix(destPath, ".sh") {
			fileMode |= 0111
		}

		if err := a.shims.Chmod(destPath, fileMode); err != nil {
			return fmt.Errorf("failed to set file permissions for %s: %w", destPath, err)
		}
	}

	return nil
}

// validateAndSanitizePath sanitizes a file path for safe extraction by removing path traversal sequences
// and rejecting absolute paths. Returns the cleaned path if valid, or an error if the path is unsafe.
// This function checks for absolute paths in a platform-agnostic way since tar archives use Unix-style paths
// regardless of the host OS.
func (a *ArtifactBuilder) validateAndSanitizePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	if strings.HasPrefix(cleanPath, string(filepath.Separator)) || (len(cleanPath) >= 2 && cleanPath[1] == ':' && (cleanPath[0] >= 'A' && cleanPath[0] <= 'Z' || cleanPath[0] >= 'a' && cleanPath[0] <= 'z')) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	return cleanPath, nil
}

// downloadOCIArtifact downloads an OCI artifact and returns the tar.gz data.
// Constructs an OCI reference from registry, repository, and tag components.
// Downloads the first layer of the OCI image which contains the artifact data.
// Returns the uncompressed layer data as bytes for further processing.
func (a *ArtifactBuilder) downloadOCIArtifact(registry, repository, tag string) ([]byte, error) {
	ref := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

	parsedRef, err := a.shims.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %s: %w", ref, err)
	}

	img, err := a.shims.RemoteImage(parsedRef)
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

// extractTemplateDataFromTar extracts template data and metadata from tar archive bytes.
// Reads the tar archive, extracts _template/ files and metadata.yaml, validates CLI version,
// and returns a map with all template files and metadata fields.
// Returns an error if tar reading fails, metadata is missing, or CLI version is incompatible.
func (a *ArtifactBuilder) extractTemplateDataFromTar(tarData []byte) (map[string][]byte, error) {
	artifactData := make(map[string][]byte)
	tarReader := tar.NewReader(bytes.NewReader(tarData))

	var hasMetadata bool
	var metadata BlueprintMetadata

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.ToSlash(header.Name)

		if name == "metadata.yaml" {
			hasMetadata = true
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read metadata.yaml: %w", err)
			}
			if err := a.shims.YamlUnmarshal(content, &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
			}
			if err := ValidateCliVersion(constants.Version, metadata.CliVersion); err != nil {
				return nil, err
			}
			continue
		}

		if !strings.HasPrefix(name, "_template/") {
			continue
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", name, err)
		}

		artifactData[name] = content
	}

	if !hasMetadata {
		return nil, fmt.Errorf("OCI artifact missing required metadata.yaml file")
	}

	a.addMetadataToTemplateData(artifactData, metadata)

	return artifactData, nil
}

// Ensure ArtifactBuilder implements Artifact interface
var _ Artifact = (*ArtifactBuilder)(nil)
