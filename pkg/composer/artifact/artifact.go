package artifact

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"maps"
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
	// Name is the name of the OCI artifact
	Name string
	// URL is the full OCI URL of the artifact
	URL string
	// Tag is the tag/version of the OCI artifact
	Tag string
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
}

// =============================================================================
// ArtifactBuilder Implementation
// =============================================================================

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
	tarballPath string
	metadata    BlueprintMetadataInput
	ociCache    map[string][]byte
}

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
// Creates compressed tar.gz with all files plus generated metadata.yaml at root.
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

// addFile stores a file with the specified path and content in the artifact for later packaging.
// Files are held in memory until create() or Push() is called. The path becomes the relative
// path within the generated tar.gz archive. Multiple calls with the same path will overwrite
// the previous content. Special handling exists for "_templates/metadata.yaml" during packaging.
func (a *ArtifactBuilder) addFile(path string, content []byte, mode os.FileMode) error {
	a.files[path] = FileInfo{
		Content: content,
		Mode:    mode,
	}
	return nil
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
// "ociUrl" (the original OCI reference), "name" (from metadata.yaml if present), and "values" (from values.yaml if present).
// Only .jsonnet files are included as template data. Returns an error on invalid reference, download failure, or extraction error.
func (a *ArtifactBuilder) GetTemplateData(ociRef string) (map[string][]byte, error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return nil, fmt.Errorf("invalid OCI reference: %s", ociRef)
	}

	registry, repository, tag, err := a.parseOCIRef(ociRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ociRef, err)
	}

	artifacts, err := a.Pull([]string{ociRef})
	if err != nil {
		return nil, fmt.Errorf("failed to pull OCI artifact %s: %w", ociRef, err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	artifactData, exists := artifacts[cacheKey]
	if !exists {
		return nil, fmt.Errorf("failed to retrieve artifact data for %s", ociRef)
	}

	templateData := make(map[string][]byte)
	templateData["ociUrl"] = []byte(ociRef)

	tarReader := tar.NewReader(bytes.NewReader(artifactData))

	var metadataName string
	jsonnetFiles := make(map[string][]byte)
	var hasMetadata, hasBlueprintJsonnet bool
	var schemaContent []byte

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
			if err := ValidateCliVersion(constants.Version, metadata.CliVersion); err != nil {
				return nil, err
			}
			metadataName = metadata.Name
		case name == "_template/schema.yaml":
			schemaContent, err = io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read _template/schema.yaml: %w", err)
			}
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

	if schemaContent != nil {
		templateData["schema"] = schemaContent
	}

	maps.Copy(templateData, jsonnetFiles)

	return templateData, nil
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

// =============================================================================
// Private Methods
// =============================================================================

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

	metadataFileInfo, hasMetadata := a.files["_templates/metadata.yaml"]
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

	if input.CliVersion != "" {
		metadata.CliVersion = input.CliVersion
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

// =============================================================================
// Helper Functions
// =============================================================================

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

	version, err := semver.NewVersion(cliVersion)
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

// Ensure ArtifactBuilder implements Artifact interface
var _ Artifact = (*ArtifactBuilder)(nil)
