package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// The GitGenerator is a specialized component that manages Git configuration files.
// It provides functionality to create and update .gitignore files with Windsor-specific entries.
// The GitGenerator ensures proper Git configuration for Windsor projects,
// maintaining consistent version control settings across all contexts.

// =============================================================================
// Constants
// =============================================================================

// Define the item to add to the .gitignore
var gitIgnoreLines = []string{
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

// =============================================================================
// Types
// =============================================================================

// GitGenerator is a generator that writes Git configuration files
type GitGenerator struct {
	BaseGenerator
}

// =============================================================================
// Constructor
// =============================================================================

// NewGitGenerator creates a new GitGenerator
func NewGitGenerator(injector di.Injector) *GitGenerator {
	return &GitGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write generates the Git configuration files by delegating to the Generate method.
// It maintains backward compatibility while Generate handles the actual file generation.
// The overwrite parameter is currently not used but preserved for interface compatibility.
func (g *GitGenerator) Write(overwrite ...bool) error {
	return g.Generate(nil)
}

// Generate creates the Git configuration files by creating or updating the .gitignore file.
// It ensures that Windsor-specific entries are added while preserving any existing user-defined entries.
// For GitGenerator, the data parameter is not used since it always generates the .gitignore file
// in the project root based on predefined rules.
func (g *GitGenerator) Generate(data map[string]any) error {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	content, err := g.shims.ReadFile(gitignorePath)
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

		// Track normalized commented versions of Windsor entries
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

	if err := g.shims.WriteFile(gitignorePath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write to .gitignore: %w", err)
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// normalizeGitignoreComment normalizes a commented .gitignore line to its uncommented form.
// It removes all leading #, whitespace, and trailing whitespace.
func normalizeGitignoreComment(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return ""
	}
	// Remove all leading # and whitespace after #
	noHash := strings.TrimLeft(trimmed, "#")
	noHash = strings.TrimLeft(noHash, " \t")
	return strings.TrimSpace(noHash)
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure GitGenerator implements Generator
var _ Generator = (*GitGenerator)(nil)
