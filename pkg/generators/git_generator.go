package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// GitGenerator is a generator that writes Git configuration files
type GitGenerator struct {
	BaseGenerator
}

// NewGitGenerator creates a new GitGenerator
func NewGitGenerator(injector di.Injector) *GitGenerator {
	return &GitGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// Write generates the Git configuration files
func (g *GitGenerator) Write() error {
	// Get the project root
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	// Define the path to the .gitignore file
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	// Read the existing .gitignore file, or create it if it doesn't exist
	content, err := osReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// If the file does not exist, initialize content as an empty byte slice
	if os.IsNotExist(err) {
		content = []byte{}
	}

	// Convert the content to a set for idempotency
	existingLines := make(map[string]struct{})
	var unmanagedLines []string
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		existingLines[line] = struct{}{}
		if i == len(lines)-1 && line == "" {
			continue // Skip appending the last line if it's empty
		}
		unmanagedLines = append(unmanagedLines, line)
	}

	// Define the lines to add
	linesToAdd := []string{
		"# managed by windsor cli",
		".volumes/",
		".tf_modules/",
		".docker-cache/",
		"terraform/**/backend_override.tf",
		"contexts/**/.terraform/",
		"contexts/**/.tfstate/",
		"contexts/**/.kube/",
		"contexts/**/.talos/",
		"contexts/**/.aws/",
	}

	// Add only the lines that are not already present
	for _, line := range linesToAdd {
		if _, exists := existingLines[line]; !exists {
			if line == "# managed by windsor cli" {
				unmanagedLines = append(unmanagedLines, "")
			}
			unmanagedLines = append(unmanagedLines, line)
		}
	}

	// Join all lines into the final content
	finalContent := strings.Join(unmanagedLines, "\n")

	// Ensure the final content ends with a single newline
	if !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	// Write the final content to the .gitignore file
	if err := osWriteFile(gitignorePath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write to .gitignore: %w", err)
	}

	return nil
}

// Ensure GitGenerator implements Generator
var _ Generator = (*GitGenerator)(nil)
