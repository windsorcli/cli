package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/di"
)

// BuildIDPipeline manages build ID operations for Windsor CLI build workflows.
// It provides methods for initializing, generating, retrieving, and persisting build IDs
// used to uniquely identify build executions within a Windsor project.
type BuildIDPipeline struct {
	BasePipeline
}

// NewBuildIDPipeline constructs a new BuildIDPipeline instance with default base pipeline initialization.
func NewBuildIDPipeline() *BuildIDPipeline {
	return &BuildIDPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// Initialize sets up the BuildIDPipeline by initializing its base pipeline with the provided dependency injector and context.
// Returns an error if base initialization fails.
func (p *BuildIDPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}
	return nil
}

// Execute runs the build ID pipeline logic. If the context contains a "new" flag set to true,
// a new build ID is generated and persisted. Otherwise, the current build ID is retrieved and output.
// Returns an error if any operation fails.
func (p *BuildIDPipeline) Execute(ctx context.Context) error {
	new, _ := ctx.Value("new").(bool)

	if new {
		return p.generateNewBuildID()
	}

	return p.getBuildID()
}

// getBuildID retrieves the current build ID from the .windsor/.build-id file and outputs it.
// If no build ID exists, a new one is generated, persisted, and output.
// Returns an error if retrieval or persistence fails.
func (p *BuildIDPipeline) getBuildID() error {
	buildID, err := p.getBuildIDFromFile()
	if err != nil {
		return fmt.Errorf("failed to get build ID: %w", err)
	}

	if buildID == "" {
		newBuildID, err := p.generateBuildID()
		if err != nil {
			return fmt.Errorf("failed to generate build ID: %w", err)
		}
		if err := p.setBuildIDToFile(newBuildID); err != nil {
			return fmt.Errorf("failed to set build ID: %w", err)
		}
		fmt.Printf("%s\n", newBuildID)
	} else {
		fmt.Printf("%s\n", buildID)
	}

	return nil
}

// generateNewBuildID generates a new build ID and persists it to the .windsor/.build-id file,
// overwriting any existing value. Outputs the new build ID. Returns an error if generation or persistence fails.
func (p *BuildIDPipeline) generateNewBuildID() error {
	newBuildID, err := p.generateBuildID()
	if err != nil {
		return fmt.Errorf("failed to generate build ID: %w", err)
	}

	if err := p.setBuildIDToFile(newBuildID); err != nil {
		return fmt.Errorf("failed to set build ID: %w", err)
	}

	fmt.Printf("%s\n", newBuildID)
	return nil
}

// getBuildIDFromFile reads and returns the build ID string from the .windsor/.build-id file in the project root.
// If the file does not exist, returns an empty string and no error. Returns an error if file access fails.
func (p *BuildIDPipeline) getBuildIDFromFile() (string, error) {
	buildIDPath, err := p.getBuildIDPath()
	if err != nil {
		return "", fmt.Errorf("failed to get build ID path: %w", err)
	}

	if _, err := p.shims.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := p.shims.ReadFile(buildIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read build ID file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// setBuildIDToFile writes the provided build ID string to the .windsor/.build-id file in the project root.
// Ensures the .windsor directory exists before writing. Returns an error if directory creation or file write fails.
func (p *BuildIDPipeline) setBuildIDToFile(buildID string) error {
	buildIDPath, err := p.getBuildIDPath()
	if err != nil {
		return fmt.Errorf("failed to get build ID path: %w", err)
	}

	buildIDDir := filepath.Dir(buildIDPath)
	if err := os.MkdirAll(buildIDDir, 0755); err != nil {
		return fmt.Errorf("failed to create build ID directory: %w", err)
	}

	return os.WriteFile(buildIDPath, []byte(buildID), 0644)
}

// generateBuildID returns a new build ID string based on the current Unix timestamp.
// Returns the generated build ID and no error.
func (p *BuildIDPipeline) generateBuildID() (string, error) {
	buildID := fmt.Sprintf("%d", time.Now().Unix())
	return buildID, nil
}

// getBuildIDPath computes and returns the absolute path to the .windsor/.build-id file in the project root directory.
// Returns an error if the project root cannot be determined.
func (p *BuildIDPipeline) getBuildIDPath() (string, error) {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	return filepath.Join(projectRoot, ".windsor", ".build-id"), nil
}
