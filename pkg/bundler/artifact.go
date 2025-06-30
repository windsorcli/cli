package bundler

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ArtifactBuilder creates tar.gz artifacts from prepared build directories.
// It provides a unified interface for packaging prepared files into distributable artifacts
// without requiring git synchronization or validation. The ArtifactBuilder serves as the final
// step in the bundling pipeline, creating self-contained artifacts that include all bundled
// dependencies and metadata for distribution.

const artifactVersion = "v1"

// =============================================================================
// Types
// =============================================================================

// BlueprintMetadata represents the metadata structure embedded in the artifact
// This metadata is used by Windsor CLI during unbundling to rewrite blueprint references
type BlueprintMetadata struct {
	Version   string        `json:"version"`
	Name      string        `json:"name"`
	Timestamp string        `json:"timestamp"`
	Git       GitProvenance `json:"git"`
	Builder   BuilderInfo   `json:"builder"`
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

// =============================================================================
// Interfaces
// =============================================================================

// Artifact defines the interface for artifact creation operations
type Artifact interface {
	Initialize(injector di.Injector) error
	Create(buildDir, outputPath string) error
}

// =============================================================================
// ArtifactBuilder Implementation
// =============================================================================

// ArtifactBuilder implements the Artifact interface for blueprint artifacts
type ArtifactBuilder struct {
	shims *Shims
	shell shell.Shell
}

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactBuilder creates a new ArtifactBuilder instance
func NewArtifactBuilder() *ArtifactBuilder {
	return &ArtifactBuilder{
		shims: NewShims(),
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

// Create generates a tar.gz artifact from the prepared build directory.
// It extracts blueprint metadata, creates artifact metadata, and packages everything
// into a self-contained artifact for distribution.
func (a *ArtifactBuilder) Create(buildDir, outputPath string) error {
	metadata, err := a.generateMetadata(buildDir)
	if err != nil {
		return fmt.Errorf("failed to generate metadata: %w", err)
	}

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
		Name: ".blueprint-metadata.json",
		Mode: 0644,
		Size: int64(len(metadata)),
	}

	if err := tarWriter.WriteHeader(metadataHeader); err != nil {
		return fmt.Errorf("failed to write metadata header: %w", err)
	}

	if _, err := tarWriter.Write(metadata); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	err = a.shims.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := a.shims.FilepathRel(buildDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", relPath, err)
		}

		if info.IsDir() {
			return nil
		}

		file, err := a.shims.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer file.Close()

		if _, err := a.shims.Copy(tarWriter, file); err != nil {
			return fmt.Errorf("failed to copy file %s: %w", path, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk build directory: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateMetadata creates metadata for bundled artifacts.
// It reads the blueprint.yaml file, extracts git provenance and builder information,
// then marshals the complete metadata structure to JSON for embedding in the artifact.
func (a *ArtifactBuilder) generateMetadata(buildDir string) ([]byte, error) {
	blueprintPath := filepath.Join(buildDir, "blueprint.yaml")
	blueprintData, err := a.shims.ReadFile(blueprintPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read blueprint.yaml: %w", err)
	}

	var blueprint blueprintv1alpha1.Blueprint
	if err := a.shims.YamlUnmarshal(blueprintData, &blueprint); err != nil {
		return nil, fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	gitInfo, err := a.getGitProvenance()
	if err != nil {
		return nil, fmt.Errorf("failed to get git information: %w", err)
	}

	builderInfo, err := a.getBuilderInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get builder information: %w", err)
	}

	metadata := BlueprintMetadata{
		Version:   artifactVersion,
		Name:      blueprint.Metadata.Name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Git:       gitInfo,
		Builder:   builderInfo,
	}

	return a.shims.JsonMarshal(metadata)
}

// getGitProvenance extracts git repository information for provenance tracking.
// It retrieves the current commit SHA, tag (if present), and remote origin URL.
// Tag retrieval failures are ignored as tags are optional for provenance.
func (a *ArtifactBuilder) getGitProvenance() (GitProvenance, error) {
	var gitInfo GitProvenance

	if commitSHA, err := a.shell.ExecSilent("git", "rev-parse", "HEAD"); err != nil {
		return gitInfo, fmt.Errorf("failed to get commit SHA: %w", err)
	} else {
		gitInfo.CommitSHA = strings.TrimSpace(commitSHA)
	}

	if tag, err := a.shell.ExecSilent("git", "tag", "--points-at", "HEAD"); err == nil {
		gitInfo.Tag = strings.TrimSpace(tag)
	}

	if remoteURL, err := a.shell.ExecSilent("git", "config", "--get", "remote.origin.url"); err != nil {
		return gitInfo, fmt.Errorf("failed to get remote URL: %w", err)
	} else {
		gitInfo.RemoteURL = strings.TrimSpace(remoteURL)
	}

	return gitInfo, nil
}

// getBuilderInfo extracts information about who/what built the artifact.
// It retrieves git user name and email configuration. Configuration failures
// are ignored and result in empty strings for the respective fields.
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
