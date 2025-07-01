package bundler

import (
	"archive/tar"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	AddFile(path string, content []byte) error
	Create(outputPath string, tag string) (string, error)
}

// =============================================================================
// ArtifactBuilder Implementation
// =============================================================================

// ArtifactBuilder implements the Artifact interface for blueprint artifacts
type ArtifactBuilder struct {
	shims *Shims
	shell shell.Shell
	files map[string][]byte
}

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactBuilder creates a new ArtifactBuilder instance
func NewArtifactBuilder() *ArtifactBuilder {
	return &ArtifactBuilder{
		shims: NewShims(),
		files: make(map[string][]byte),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize initializes the ArtifactBuilder with dependency injection
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

// AddFile adds a file with the given path and content to the artifact
func (a *ArtifactBuilder) AddFile(path string, content []byte) error {
	a.files[path] = content
	return nil
}

// Create generates a tar.gz artifact from the stored files and metadata.
// It accepts an optional tag in "name:version" format to override or provide metadata values.
// Tag values take precedence over metadata.yaml when both are present. If no metadata.yaml
// exists, the tag is required to provide name and version. The output is a compressed tar.gz
// file containing all bundled files plus a generated metadata.yaml at the root.
// The outputPath can be a file or directory - if directory, filename is generated from metadata.
func (a *ArtifactBuilder) Create(outputPath string, tag string) (string, error) {
	// Parse tag if provided
	var tagName, tagVersion string
	if tag != "" {
		parts := strings.Split(tag, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("tag must be in format 'name:version', got: %s", tag)
		}
		tagName = parts[0]
		tagVersion = parts[1]
	}

	// Parse existing metadata if available
	metadataData, hasMetadata := a.files["_templates/metadata.yaml"]
	var input BlueprintMetadataInput

	if hasMetadata {
		if err := a.shims.YamlUnmarshal(metadataData, &input); err != nil {
			return "", fmt.Errorf("failed to parse metadata.yaml: %w", err)
		}
	}

	// Determine final name and version (tag takes precedence)
	finalName := input.Name
	finalVersion := input.Version

	if tagName != "" {
		finalName = tagName
	}
	if tagVersion != "" {
		finalVersion = tagVersion
	}

	if finalName == "" {
		return "", fmt.Errorf("name is required: provide via tag parameter or metadata.yaml")
	}
	if finalVersion == "" {
		return "", fmt.Errorf("version is required: provide via tag parameter or metadata.yaml")
	}

	// Handle output path - generate filename if needed
	finalOutputPath := a.resolveOutputPath(outputPath, finalName, finalVersion)

	metadata, err := a.generateMetadataWithNameVersion(input, finalName, finalVersion)
	if err != nil {
		return "", fmt.Errorf("failed to generate metadata: %w", err)
	}

	outputFile, err := a.shims.Create(finalOutputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
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
		return "", fmt.Errorf("failed to write metadata header: %w", err)
	}

	if _, err := tarWriter.Write(metadata); err != nil {
		return "", fmt.Errorf("failed to write metadata: %w", err)
	}

	for path, content := range a.files {
		if path == "_templates/metadata.yaml" {
			continue
		}

		header := &tar.Header{
			Name: path,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return "", fmt.Errorf("failed to write header for %s: %w", path, err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			return "", fmt.Errorf("failed to write content for %s: %w", path, err)
		}
	}

	return finalOutputPath, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// resolveOutputPath determines the final output path for the artifact.
// If outputPath is a directory or ends with slash, generates filename from name and version in that directory.
// If outputPath appears to be a directory path (no extension), generates filename from name and version.
// Otherwise uses outputPath as-is for the filename (user provided explicit filename).
func (a *ArtifactBuilder) resolveOutputPath(outputPath, name, version string) string {
	filename := fmt.Sprintf("%s-%s.tar.gz", name, version)

	// Check if output path is an existing directory
	if stat, err := a.shims.Stat(outputPath); err == nil && stat.IsDir() {
		return filepath.Join(outputPath, filename)
	}

	// Check if path ends with slash (intended as directory)
	if strings.HasSuffix(outputPath, "/") {
		return filepath.Join(outputPath, filename)
	}

	// Check if this looks like a directory path (no file extension)
	if filepath.Ext(outputPath) == "" {
		return filepath.Join(outputPath, filename)
	}

	// Use outputPath as-is (user provided explicit filename)
	return outputPath
}

// generateMetadataWithNameVersion creates metadata for bundled artifacts with final name and version.
// It combines input metadata with git provenance and builder information, then marshals
// the complete metadata structure to YAML for embedding in the artifact.
func (a *ArtifactBuilder) generateMetadataWithNameVersion(input BlueprintMetadataInput, name, version string) ([]byte, error) {
	gitInfo, _ := a.getGitProvenance()
	builderInfo, _ := a.getBuilderInfo()

	metadata := BlueprintMetadata{
		Name:        name,
		Version:     version,
		Description: input.Description,
		Author:      input.Author,
		Tags:        input.Tags,
		Homepage:    input.Homepage,
		License:     input.License,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Git:         gitInfo,
		Builder:     builderInfo,
	}

	return a.shims.YamlMarshal(metadata)
}

// getGitProvenance extracts git repository information for provenance tracking.
// All git operations are best-effort and failures are ignored since git provenance is optional.
// Returns empty values for any git operations that fail.
func (a *ArtifactBuilder) getGitProvenance() (GitProvenance, error) {
	var gitInfo GitProvenance

	if commitSHA, err := a.shell.ExecSilent("git", "rev-parse", "HEAD"); err == nil {
		gitInfo.CommitSHA = strings.TrimSpace(commitSHA)
	}

	if tag, err := a.shell.ExecSilent("git", "tag", "--points-at", "HEAD"); err == nil {
		gitInfo.Tag = strings.TrimSpace(tag)
	}

	if remoteURL, err := a.shell.ExecSilent("git", "config", "--get", "remote.origin.url"); err == nil {
		gitInfo.RemoteURL = strings.TrimSpace(remoteURL)
	}

	return gitInfo, nil
}

// getBuilderInfo extracts information about who/what built the artifact.
// It retrieves git user name and email configuration. All git operations are
// best-effort and failures are ignored since builder info is optional.
func (a *ArtifactBuilder) getBuilderInfo() (BuilderInfo, error) {
	var builderInfo BuilderInfo

	if user, err := a.shell.ExecSilent("git", "config", "user.name"); err == nil {
		builderInfo.User = strings.TrimSpace(user)
	}

	if email, err := a.shell.ExecSilent("git", "config", "user.email"); err == nil {
		builderInfo.Email = strings.TrimSpace(email)
	}

	return builderInfo, nil
}

// Ensure ArtifactBuilder implements Artifact interface
var _ Artifact = (*ArtifactBuilder)(nil)
