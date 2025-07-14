package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ArtifactBuilder creates tar.gz artifacts from prepared build directories.
// It provides a unified interface for packaging prepared files into distributable artifacts
// without requiring git synchronization or validation. The ArtifactBuilder serves as the final
// step in the bundling pipeline, creating self-contained artifacts that include all bundled
// dependencies and metadata for distribution.

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

// BlueprintMetadataInput represents the input metadata from contexts/_template/metadata.yaml
type BlueprintMetadataInput struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Version     string   `yaml:"version,omitempty"`
	Author      string   `yaml:"author,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty"`
	License     string   `yaml:"license,omitempty"`
}

// =============================================================================
// Interfaces
// =============================================================================

// Artifact defines the interface for artifact creation operations
type Artifact interface {
	Initialize(injector di.Injector) error
	AddFile(path string, content []byte, mode os.FileMode) error
	Create(outputPath string, tag string) (string, error)
	Push(registryBase string, repoName string, tag string) error
	Pull(ociRefs []string) (map[string][]byte, error)
	GetTemplateData(ociRef string) (map[string][]byte, error)
}

// =============================================================================
// ArtifactBuilder Implementation
// =============================================================================

// FileInfo holds file content and permission information
type FileInfo struct {
	Content []byte
	Mode    os.FileMode
}

// ArtifactBuilder implements the Artifact interface
type ArtifactBuilder struct {
	files       map[string]FileInfo
	shims       *Shims
	shell       shell.Shell
	tarballPath string
	metadata    BlueprintMetadataInput
	ociCache    map[string][]byte // Cache for downloaded OCI artifacts
}

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactBuilder creates a new ArtifactBuilder instance with default configuration.
// Initializes an empty file map for storing artifact contents and sets up default shims
// for system operations. The returned builder is ready for dependency injection and file operations.
func NewArtifactBuilder() *ArtifactBuilder {
	return &ArtifactBuilder{
		shims:    NewShims(),
		files:    make(map[string]FileInfo),
		ociCache: make(map[string][]byte),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the ArtifactBuilder with dependency injection and resolves required dependencies.
// Extracts the shell dependency from the injector for git operations and command execution.
// The shell is used for retrieving git provenance and builder information during metadata generation.
// Returns an error if the shell cannot be resolved from the injector.
func (a *ArtifactBuilder) Initialize(injector di.Injector) error {
	if injector != nil {
		shell, ok := injector.Resolve("shell").(shell.Shell)
		if !ok {
			return fmt.Errorf("failed to resolve shell from injector")
		}
		a.shell = shell
	}
	return nil
}

// AddFile stores a file with the specified path and content in the artifact for later packaging.
// Files are held in memory until Create() or Push() is called. The path becomes the relative
// path within the generated tar.gz archive. Multiple calls with the same path will overwrite
// the previous content. Special handling exists for "_templates/metadata.yaml" during packaging.
func (a *ArtifactBuilder) AddFile(path string, content []byte, mode os.FileMode) error {
	a.files[path] = FileInfo{
		Content: content,
		Mode:    mode,
	}
	return nil
}

// Create generates a compressed tar.gz artifact file from stored files and metadata with optional tag override.
// Accepts optional tag in "name:version" format to override metadata.yaml values.
// Tag takes precedence over existing metadata. If no metadata.yaml exists, tag is required.
// OutputPath can be file or directory - generates filename from metadata if directory.
// Creates compressed tar.gz with all files plus generated metadata.yaml at root.
// Returns the final output path of the created artifact file.
func (a *ArtifactBuilder) Create(outputPath string, tag string) (string, error) {
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
// Creates tarball in memory, constructs OCI image, uploads blobs explicitly, then uploads manifest.
// Uses authenticated keychain for registry access and retry backoff for resilience.
// Registry base should be the base URL (e.g., "ghcr.io/namespace"), repoName the repository name, tag the version.
func (a *ArtifactBuilder) Push(registryBase string, repoName string, tag string) error {
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
		registry, repository, tag, err := a.parseOCIRef(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
		}

		cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

		if cachedData, exists := a.ociCache[cacheKey]; exists {
			ociArtifacts[cacheKey] = cachedData
		} else {
			artifactsToDownload = append(artifactsToDownload, ref)
		}
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
			registry, repository, tag, err := a.parseOCIRef(ref)
			if err != nil {
				return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
			}

			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

			artifactData, err := a.downloadOCIArtifact(registry, repository, tag)
			if err != nil {
				return nil, fmt.Errorf("failed to download OCI artifact %s: %w", ref, err)
			}

			a.ociCache[cacheKey] = artifactData
			ociArtifacts[cacheKey] = artifactData
		}
	}

	return ociArtifacts, nil
}

// GetTemplateData extracts and returns template data from an OCI artifact reference.
// Downloads and caches the OCI artifact, decompresses the tar.gz payload, and returns a map
// with forward-slash file paths as keys and file contents as values. The returned map always includes
// "ociUrl" (the original OCI reference) and "name" (from metadata.yaml if present). Only .jsonnet files
// are included as template data. Returns an error on invalid reference, download failure, or extraction error.
func (a *ArtifactBuilder) GetTemplateData(ociRef string) (map[string][]byte, error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return nil, fmt.Errorf("invalid OCI reference: %s", ociRef)
	}

	registry, repository, tag, err := a.parseOCIRef(ociRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ociRef, err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	var artifactData []byte
	if cached, ok := a.ociCache[cacheKey]; ok {
		artifactData = cached
	} else {
		artifactData, err = a.downloadOCIArtifact(registry, repository, tag)
		if err != nil {
			return nil, fmt.Errorf("failed to download OCI artifact %s: %w", ociRef, err)
		}
		a.ociCache[cacheKey] = artifactData
	}

	templateData := make(map[string][]byte)
	templateData["ociUrl"] = []byte(ociRef)

	gzipReader, err := gzip.NewReader(bytes.NewReader(artifactData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	var metadataName string
	jsonnetFiles := make(map[string][]byte)
	var hasMetadata, hasBlueprintJsonnet bool

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
		switch {
		case name == "metadata.yaml":
			hasMetadata = true
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read metadata.yaml: %w", err)
			}
			var metadata BlueprintMetadata
			if err := a.shims.YamlUnmarshal(content, &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
			}
			metadataName = metadata.Name
		case strings.HasSuffix(name, ".jsonnet"):
			normalized := strings.TrimPrefix(name, "_template/")
			if normalized == "blueprint.jsonnet" {
				hasBlueprintJsonnet = true
			}
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", name, err)
			}
			jsonnetFiles[filepath.ToSlash(normalized)] = content
		}
	}

	if !hasMetadata {
		return nil, fmt.Errorf("OCI artifact missing required metadata.yaml file")
	}
	if !hasBlueprintJsonnet {
		return nil, fmt.Errorf("OCI artifact missing required _template/blueprint.jsonnet file")
	}

	templateData["name"] = []byte(metadataName)
	maps.Copy(templateData, jsonnetFiles)

	return templateData, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

	metadataFileInfo, hasMetadata := a.files["_templates/metadata.yaml"]
	var input BlueprintMetadataInput

	if hasMetadata {
		if err := a.shims.YamlUnmarshal(metadataFileInfo.Content, &input); err != nil {
			return "", "", nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
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

	metadata, err := a.generateMetadataWithNameVersion(input, finalName, finalVersion)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate metadata: %w", err)
	}

	return finalName, finalVersion, metadata, nil
}

// createTarballInMemory builds a compressed tar.gz archive in memory and returns the complete content as bytes.
// Creates a gzip-compressed tar archive containing all stored files plus generated metadata.yaml.
// The metadata.yaml file is always written first at the root of the archive.
// Skips any existing "_templates/metadata.yaml" file to avoid duplication.
// All files are written with 0644 permissions in the archive.
// Returns the complete archive as a byte slice for in-memory operations like OCI push.
func (a *ArtifactBuilder) createTarballInMemory(metadata []byte) ([]byte, error) {
	var buf bytes.Buffer

	gzipWriter := a.shims.NewGzipWriter(&buf)
	defer gzipWriter.Close()

	tarWriter := a.shims.NewTarWriter(gzipWriter)
	defer tarWriter.Close()

	metadataHeader := &tar.Header{
		Name: "metadata.yaml",
		Mode: 0644,
		Size: int64(len(metadata)),
	}

	if err := tarWriter.WriteHeader(metadataHeader); err != nil {
		return nil, fmt.Errorf("failed to write metadata header: %w", err)
	}

	if _, err := tarWriter.Write(metadata); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	for path, fileInfo := range a.files {
		if path == "_templates/metadata.yaml" {
			continue
		}

		header := &tar.Header{
			Name: path,
			Mode: int64(fileInfo.Mode),
			Size: int64(len(fileInfo.Content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write header for %s: %w", path, err)
		}

		if _, err := tarWriter.Write(fileInfo.Content); err != nil {
			return nil, fmt.Errorf("failed to write content for %s: %w", path, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// createTarballToDisk builds a compressed tar.gz archive and writes it directly to the specified file path.
// Creates the output file at the specified path and writes a gzip-compressed tar archive.
// The metadata.yaml file is always written first at the root of the archive.
// Skips any existing "_templates/metadata.yaml" file to avoid duplication.
// All files are written with 0644 permissions in the archive.
// Properly closes writers to ensure all data is flushed to disk before returning.
func (a *ArtifactBuilder) createTarballToDisk(outputPath string, metadata []byte) error {
	outputFile, err := a.shims.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	gzipWriter := a.shims.NewGzipWriter(outputFile)
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
		if path == "_templates/metadata.yaml" {
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

	return nil
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

// generateMetadataWithNameVersion creates complete blueprint metadata by merging input metadata with name and version.
// Combines input metadata with git provenance and builder information, then marshals
// the complete metadata structure to YAML for embedding in the artifact.
// Git provenance and builder info are best-effort - failures result in empty values rather than errors.
// Includes timestamp in RFC3339 format for artifact creation tracking.
// Returns marshaled YAML bytes ready for inclusion in tar archives.
func (a *ArtifactBuilder) generateMetadataWithNameVersion(input BlueprintMetadataInput, name, version string) ([]byte, error) {
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

	return a.shims.YamlMarshal(metadata)
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
// Extracts information about who/what built the artifact using git configuration.
// Retrieves git user name and email configuration from git global or repository config.
// Returns empty strings for missing configuration rather than errors for optional builder info.
// Used for audit trails and artifact attribution in generated metadata.
func (a *ArtifactBuilder) getBuilderInfo() (BuilderInfo, error) {
	user, err := a.shell.ExecSilent("git", "config", "--get", "user.name")
	if err != nil {
		user = ""
	}

	email, err := a.shell.ExecSilent("git", "config", "--get", "user.email")
	if err != nil {
		email = ""
	}

	return BuilderInfo{
		User:  strings.TrimSpace(user),
		Email: strings.TrimSpace(email),
	}, nil
}

// parseOCIRef parses an OCI reference into registry, repository, and tag components.
// Validates OCI reference format and extracts registry, repository, and tag parts.
// Requires OCI reference to follow the format "oci://registry/repository:tag".
// Returns individual components for separate handling in OCI operations.
func (a *ArtifactBuilder) parseOCIRef(ociRef string) (registry, repository, tag string, err error) {
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

// Ensure ArtifactBuilder implements Artifact interface
var _ Artifact = (*ArtifactBuilder)(nil)
