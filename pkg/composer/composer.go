package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	context "github.com/windsorcli/cli/pkg/runtime"
)

// The Composer package provides high-level resource management functionality
// for artifact, blueprint, and terraform operations. It consolidates the creation
// and management of these core resources, providing a unified interface for
// resource lifecycle operations across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// ComposerRuntime holds the execution context for resource operations.
// It embeds the base Runtime and includes all resource-specific dependencies.
type ComposerRuntime struct {
	context.Runtime

	// Resource-specific dependencies
	ArtifactBuilder   artifact.Artifact
	BlueprintHandler  blueprint.BlueprintHandler
	TerraformResolver terraform.ModuleResolver
}

// Composer manages the lifecycle of all resource types (artifact, blueprint, terraform).
// It provides a unified interface for creating, initializing, and managing these resources
// with proper dependency injection and error handling.
type Composer struct {
	*ComposerRuntime
}

// =============================================================================
// Constructor
// =============================================================================

// NewComposer creates and initializes a new Composer instance with the provided execution context.
// It sets up all required resource handlers—artifact builder, blueprint handler, and terraform resolver—
// and registers each handler with the dependency injector for use throughout the resource lifecycle.
// Returns a pointer to the fully initialized Composer struct.
func NewComposer(ctx *ComposerRuntime) *Composer {
	composer := &Composer{
		ComposerRuntime: ctx,
	}

	if composer.ArtifactBuilder == nil {
		composer.ArtifactBuilder = artifact.NewArtifactBuilder()
	}
	composer.Injector.Register("artifactBuilder", composer.ArtifactBuilder)

	composer.BlueprintHandler = blueprint.NewBlueprintHandler(composer.Injector)
	composer.Injector.Register("blueprintHandler", composer.BlueprintHandler)

	composer.TerraformResolver = terraform.NewStandardModuleResolver(composer.Injector)
	composer.Injector.Register("terraformResolver", composer.TerraformResolver)

	return composer
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle creates a complete artifact bundle from the project's templates, kustomize, and terraform files.
// It initializes the artifact builder and creates a distributable artifact.
// The outputPath specifies where to save the bundle file. Returns the actual output path or an error.
func (r *Composer) Bundle(outputPath, tag string) (string, error) {
	if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
		return "", fmt.Errorf("failed to initialize artifact builder: %w", err)
	}

	actualOutputPath, err := r.ArtifactBuilder.Write(outputPath, tag)
	if err != nil {
		return "", fmt.Errorf("failed to create artifact bundle: %w", err)
	}

	return actualOutputPath, nil
}

// Push creates and pushes an artifact to a container registry.
// It bundles all project files and pushes them to the specified registry with the given tag.
// The registryURL can be in formats like "registry.com/repo:tag", "registry.com/repo", or "oci://registry.com/repo:tag".
// Returns the registry URL or an error.
func (r *Composer) Push(registryURL string) (string, error) {
	registryBase, repoName, tag, err := artifact.ParseRegistryURL(registryURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse registry URL: %w", err)
	}

	if err := r.ArtifactBuilder.Initialize(r.Injector); err != nil {
		return "", fmt.Errorf("failed to initialize artifact builder: %w", err)
	}

	if err := r.ArtifactBuilder.Bundle(); err != nil {
		return "", fmt.Errorf("failed to bundle artifacts: %w", err)
	}

	if err := r.ArtifactBuilder.Push(registryBase, repoName, tag); err != nil {
		return "", fmt.Errorf("failed to push artifact: %w", err)
	}

	resultURL := fmt.Sprintf("%s/%s", registryBase, repoName)
	if tag != "" {
		resultURL = fmt.Sprintf("%s:%s", resultURL, tag)
	}

	return resultURL, nil
}

// Generate processes and deploys the complete project infrastructure.
// It initializes all core resources, processes blueprints, and handles terraform modules
// for the project. The optional overwrite parameter determines whether existing files
// should be overwritten during blueprint processing. This is the main deployment method.
// Returns an error if any initialization or processing step fails.
func (r *Composer) Generate(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	if err := r.BlueprintHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize blueprint handler: %w", err)
	}
	if err := r.BlueprintHandler.LoadBlueprint(); err != nil {
		return fmt.Errorf("failed to load blueprint data: %w", err)
	}

	if err := r.TerraformResolver.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize terraform resolver: %w", err)
	}

	if err := r.BlueprintHandler.Write(shouldOverwrite); err != nil {
		return fmt.Errorf("failed to write blueprint files: %w", err)
	}

	if err := r.TerraformResolver.ProcessModules(); err != nil {
		return fmt.Errorf("failed to process terraform modules: %w", err)
	}

	if err := r.generateGitignore(); err != nil {
		return fmt.Errorf("failed to generate .gitignore: %w", err)
	}

	if r.ConfigHandler.GetBool("terraform.enabled", false) {
		if err := r.TerraformResolver.GenerateTfvars(shouldOverwrite); err != nil {
			return fmt.Errorf("failed to generate terraform files: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateGitignore creates or updates the .gitignore file with Windsor-specific entries.
// It ensures Windsor-specific paths are excluded from version control while preserving user-defined entries.
func (r *Composer) generateGitignore() error {
	gitIgnoreLines := []string{
		"# managed by windsor cli",
		".windsor/",
		".volumes/",
		"terraform/**/backend_override.tf",
		"contexts/**/.terraform/",
		"contexts/**/.tfstate/",
		"contexts/**/.kube/",
		"contexts/**/.talos/",
		"contexts/**/.omni/",
		"contexts/**/.aws/",
		"contexts/**/.azure/",
	}

	projectRoot, err := r.Shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	// #nosec G304 - gitignorePath is constructed from trusted project root
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	if os.IsNotExist(err) {
		content = []byte{}
	}

	existingLines := make(map[string]struct{})
	commentedNormalized := make(map[string]struct{})
	var unmanagedLines []string
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		existingLines[line] = struct{}{}

		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			norm := normalizeGitignoreComment(trimmed)
			if norm != "" {
				commentedNormalized[norm] = struct{}{}
			}
		}

		if i == len(lines)-1 && line == "" {
			continue
		}
		unmanagedLines = append(unmanagedLines, line)
	}

	for _, line := range gitIgnoreLines {
		if line == "# managed by windsor cli" {
			if _, exists := existingLines[line]; !exists {
				unmanagedLines = append(unmanagedLines, "")
				unmanagedLines = append(unmanagedLines, line)
			}
			continue
		}

		if _, exists := existingLines[line]; !exists {
			if _, commentedExists := commentedNormalized[line]; !commentedExists {
				unmanagedLines = append(unmanagedLines, line)
			}
		}
	}

	finalContent := strings.Join(unmanagedLines, "\n")

	if !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	// #nosec G306 - .gitignore files are standard 0644 permissions (world-readable, owner-writable)
	if err := os.WriteFile(gitignorePath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write to .gitignore: %w", err)
	}

	return nil
}

// normalizeGitignoreComment normalizes a commented .gitignore line to its uncommented form.
// It removes all leading #, whitespace, and trailing whitespace.
func normalizeGitignoreComment(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return ""
	}
	noHash := strings.TrimLeft(trimmed, "#")
	noHash = strings.TrimLeft(noHash, " \t")
	return strings.TrimSpace(noHash)
}
