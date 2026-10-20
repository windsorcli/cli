// The ArtifactBuilder is a builder that packages a prepared build directory into a blueprint artifact.
// It provides the bundle step, which collects the files and the artifact manifest, and the tar.gz write.
// It adds a metadata.yaml with the name, version, git provenance, and builder information.
// The registry and cache functions of the ArtifactBuilder are in registry.go and cache.go.

package artifact

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

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
}

// =============================================================================
// Interfaces
// =============================================================================

// Artifact defines the interface for artifact creation operations
type Artifact interface {
	Bundle() error
	Write(outputPath string, tag string) (string, error)
	Push(registryBase string, repoName string, tag string) error
	Pull(ociRefs []string) (map[string]string, error)
	ExtractModulePath(registry, repository, tag, modulePath string) (string, error)
	ParseOCIRef(ociRef string) (registry, repository, tag string, err error)
	GetCacheDir(registry, repository, tag string) (string, error)
	ListTags(ociRef string) ([]string, error)
	GetCliVersionConstraint(ociRef string) (string, error)
	VerifyCliVersionCompatibility(ociRef string) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactBuilder creates a new ArtifactBuilder instance with the provided shell dependency.
// If overrides are provided, any non-nil component in the override ArtifactBuilder will be used instead of creating a default.
// The shell is used for retrieving git provenance and builder information during metadata generation.
func NewArtifactBuilder(rt *runtime.Runtime) *ArtifactBuilder {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}

	builder := &ArtifactBuilder{
		shims:   NewShims(),
		files:   make(map[string]FileInfo),
		shell:   rt.Shell,
		runtime: rt,
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

	finalName, finalVersion, _, metadata, err := a.parseTagAndResolveMetadata("", tag)
	if err != nil {
		return "", err
	}

	a.metadata.Name = finalName
	a.metadata.Version = finalVersion
	if err := a.refreshArtifactManifest(); err != nil {
		return "", err
	}

	finalOutputPath := a.resolveOutputPath(outputPath, finalName, finalVersion)

	err = a.createTarballToDisk(finalOutputPath, metadata)
	if err != nil {
		return "", err
	}

	a.tarballPath = finalOutputPath

	return finalOutputPath, nil
}

// Bundle traverses the project directories and collects all relevant files to be
// included in the artifact. It applies configurable path-based processors that determine
// how files from each directory (such as "_template", "kustomize", or "terraform") are
// incorporated into the artifact. The method supports extensibility by allowing custom
// handling of different directory structures and types, and skips files in the "terraform"
// directory based on predefined logic.
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

	if err := a.walkAndProcessFiles(processors); err != nil {
		return err
	}

	manifestYAML, err := a.generateArtifactManifest()
	if err != nil {
		return fmt.Errorf("failed to generate artifact manifest: %w", err)
	}
	return a.addFile(ManifestFileName, manifestYAML, 0644)
}

// =============================================================================
// Private Methods
// =============================================================================

// refreshArtifactManifest regenerates the artifact manifest using current metadata
// and replaces the manifest entry in the bundled files. Called after metadata name
// and version are resolved so Blueprint provenance reflects the final values rather
// than the zero values present when Bundle() first wrote the manifest.
func (a *ArtifactBuilder) refreshArtifactManifest() error {
	manifestYAML, err := a.generateArtifactManifest()
	if err != nil {
		return fmt.Errorf("failed to generate artifact manifest: %w", err)
	}
	return a.addFile(ManifestFileName, manifestYAML, 0644)
}

// generateArtifactManifest scans every bundled file for Renovate annotations
// and returns a YAML-encoded ArtifactManifest capturing the pinned artifacts.
// Only kustomize and terraform sources are scanned, since those are the paths
// where blueprint authors declare external dependencies.
func (a *ArtifactBuilder) generateArtifactManifest() ([]byte, error) {
	entries := make([]ManifestEntry, 0)
	for path, info := range a.files {
		if !isScannablePath(path) {
			continue
		}
		entries = append(entries, scanRenovateAnnotations(path, info.Content)...)
	}
	sortManifestEntries(entries)
	entries = dedupeManifestEntries(entries)

	manifest := ArtifactManifest{
		Version:   ManifestVersion,
		Blueprint: Provenance{Name: a.metadata.Name, Version: a.metadata.Version},
		Artifacts: entries,
	}
	return a.shims.YamlMarshal(&manifest)
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
// Returns final name, version, the cliVersion constraint (empty if none declared), and complete
// marshaled metadata ready for embedding in artifacts.
func (a *ArtifactBuilder) parseTagAndResolveMetadata(repoName, tag string) (string, string, string, []byte, error) {
	var tagName, tagVersion string

	if repoName == "" {
		if tag != "" {
			if strings.Contains(tag, ":") {
				parts := strings.Split(tag, ":")
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					return "", "", "", nil, fmt.Errorf("tag must be in format 'name:version', got: %s", tag)
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
			return "", "", "", nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
		}
		if err := ValidateCliVersion(constants.Version, input.CliVersion); err != nil {
			return "", "", "", nil, err
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
			return "", "", "", nil, fmt.Errorf("name is required: provide via tag parameter or metadata.yaml")
		} else {
			return "", "", "", nil, fmt.Errorf("name is required: provide via metadata.yaml")
		}
	}
	if finalVersion == "" {
		return "", "", "", nil, fmt.Errorf("version is required: provide via tag parameter or metadata.yaml")
	}

	metadata, err := a.generateMetadata(input, finalName, finalVersion)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("failed to generate metadata: %w", err)
	}

	return finalName, finalVersion, input.CliVersion, metadata, nil
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

	now := time.Now().UTC()

	metadataHeader := &tar.Header{
		Name:    "metadata.yaml",
		Mode:    0644,
		Size:    int64(len(metadata)),
		ModTime: now,
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
			Name:    path,
			Mode:    int64(fileInfo.Mode),
			Size:    int64(len(fileInfo.Content)),
			ModTime: now,
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure ArtifactBuilder implements Artifact interface
var _ Artifact = (*ArtifactBuilder)(nil)
