package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The Composer package provides high-level resource management functionality
// for artifact, blueprint, and terraform operations. It consolidates the creation
// and management of these core resources, providing a unified interface for
// resource lifecycle operations across the Windsor CLI.

// =============================================================================
// Types
// =============================================================================

// Composer holds the execution context for resource operations.
// It includes all resource-specific dependencies and core runtime dependencies.
type Composer struct {
	configHandler config.ConfigHandler
	shell         shell.Shell
	evaluator     evaluator.ExpressionEvaluator
	projectRoot   string
	configRoot    string
	templateRoot  string

	// Resource-specific dependencies
	ArtifactBuilder   artifact.Artifact
	BlueprintHandler  blueprint.BlueprintHandler
	TerraformResolver terraform.ModuleResolver
}

// =============================================================================
// Constructor
// =============================================================================

// NewComposer creates and initializes a new Composer instance with the provided execution context.
// It sets up all required resource handlers—artifact builder, blueprint handler, and terraform resolver.
// If overrides are provided, any non-nil component in the override Composer will be used instead of creating a default.
// Panics if runtime or any required dependencies are nil.
func NewComposer(rt *runtime.Runtime, opts ...*Composer) *Composer {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.ConfigHandler == nil {
		panic("config handler is required on runtime")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}

	composer := &Composer{
		configHandler: rt.ConfigHandler,
		shell:         rt.Shell,
		evaluator:     rt.Evaluator,
		projectRoot:   rt.ProjectRoot,
		configRoot:    rt.ConfigRoot,
		templateRoot:  rt.TemplateRoot,
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.ArtifactBuilder != nil {
			composer.ArtifactBuilder = overrides.ArtifactBuilder
		}
		if overrides.BlueprintHandler != nil {
			composer.BlueprintHandler = overrides.BlueprintHandler
		}
		if overrides.TerraformResolver != nil {
			composer.TerraformResolver = overrides.TerraformResolver
		}
	}

	if composer.ArtifactBuilder == nil {
		composer.ArtifactBuilder = artifact.NewArtifactBuilder(rt)
	}

	if composer.BlueprintHandler == nil {
		composer.BlueprintHandler = blueprint.NewBlueprintHandler(rt, composer.ArtifactBuilder)
	}

	if composer.TerraformResolver == nil {
		composer.TerraformResolver = terraform.NewCompositeModuleResolver(rt, composer.BlueprintHandler, composer.ArtifactBuilder)
	}

	return composer
}

// =============================================================================
// Public Methods
// =============================================================================

// Bundle creates a complete artifact bundle from the project's templates, kustomize, and terraform files.
// It creates a distributable artifact.
// The outputPath specifies where to save the bundle file. Returns the actual output path or an error.
func (r *Composer) Bundle(outputPath, tag string) (string, error) {
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
// should be overwritten during blueprint processing. The optional blueprintURL parameter
// specifies the blueprint artifact to load (OCI URL or local .tar.gz path). If not provided,
// LoadBlueprint will use the default blueprint URL. This is the main deployment method.
// Returns an error if any initialization or processing step fails.
func (r *Composer) Generate(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	if err := r.BlueprintHandler.Write(shouldOverwrite); err != nil {
		return fmt.Errorf("failed to write blueprint files: %w", err)
	}

	if err := r.TerraformResolver.ProcessModules(); err != nil {
		return fmt.Errorf("failed to process terraform modules: %w", err)
	}

	if err := r.writeLocalGitignores(); err != nil {
		return fmt.Errorf("failed to write local gitignores: %w", err)
	}

	if r.configHandler.GetBool("terraform.enabled", true) {
		if err := r.TerraformResolver.GenerateTfvars(shouldOverwrite); err != nil {
			return fmt.Errorf("failed to generate terraform files: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// windsorMarkerContent is written to .gitignore inside Windsor-owned folders
// (.windsor/, .volumes/). The pattern ignores every file in the folder including
// the .gitignore itself, which keeps the folder invisible to `git status` so
// users never have to commit Windsor-managed scaffolding.
const windsorMarkerContent = "*\n"

// contextIgnoreContent is written to contexts/<ctx>/.gitignore to keep
// per-context credential and state directories out of version control.
const contextIgnoreContent = ".kube/\n.talos/\n.omni/\n.aws/\n.azure/\n.gcp/\n.vsphere/\n.env\n"

// writeLocalGitignores writes self-contained .gitignore files into Windsor-owned
// folders so re-running Windsor never touches the project-root .gitignore.
// Each file is written once: if a target .gitignore already exists, it is left
// alone. .volumes/ and contexts/ are skipped silently when the directory itself
// is absent; .windsor/ is created if missing.
func (r *Composer) writeLocalGitignores() error {
	windsorDir := filepath.Join(r.projectRoot, ".windsor")
	if err := os.MkdirAll(windsorDir, 0750); err != nil {
		return fmt.Errorf("failed to ensure .windsor directory: %w", err)
	}
	if err := writeIfMissing(filepath.Join(windsorDir, ".gitignore"), windsorMarkerContent); err != nil {
		return fmt.Errorf("failed to write .windsor/.gitignore: %w", err)
	}

	volumesDir := filepath.Join(r.projectRoot, ".volumes")
	if _, err := os.Stat(volumesDir); err == nil {
		if err := writeIfMissing(filepath.Join(volumesDir, ".gitignore"), windsorMarkerContent); err != nil {
			return fmt.Errorf("failed to write .volumes/.gitignore: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat .volumes: %w", err)
	}

	contextsDir := filepath.Join(r.projectRoot, "contexts")
	entries, err := os.ReadDir(contextsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read contexts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "_template" {
			continue
		}
		target := filepath.Join(contextsDir, entry.Name(), ".gitignore")
		if err := writeOrMergeContextIgnore(target, contextIgnoreContent); err != nil {
			return fmt.Errorf("failed to write contexts/%s/.gitignore: %w", entry.Name(), err)
		}
	}
	return nil
}

// writeOrMergeContextIgnore writes desired's content fresh when target is
// missing, or appends any of desired's lines that a pre-existing target lacks
// (preserving the file's other content) so a contextIgnoreContent addition
// still reaches contexts created by an earlier CLI version.
func writeOrMergeContextIgnore(target, desired string) error {
	if err := writeIfMissing(target, desired); err != nil {
		return err
	}

	// #nosec G304 - target is composed from the project's own contexts/ directory listing, not user-supplied
	existing, err := os.ReadFile(target)
	if err != nil {
		return err
	}

	present := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}

	var missing strings.Builder
	for _, line := range strings.Split(strings.TrimRight(desired, "\n"), "\n") {
		if line != "" && !present[line] {
			missing.WriteString(line)
			missing.WriteString("\n")
		}
	}
	if missing.Len() == 0 {
		return nil
	}

	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += missing.String()

	// #nosec G306 G703 - .gitignore files use standard 0644 permissions; target is
	// composed from the project's own contexts/ directory listing, not user-supplied
	return os.WriteFile(target, []byte(content), 0644)
}

// writeIfMissing writes content to path with 0644 perms only when the file
// does not yet exist. Existing files are left untouched.
func writeIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	// #nosec G306 - .gitignore files use standard 0644 permissions
	return os.WriteFile(path, []byte(content), 0644)
}
