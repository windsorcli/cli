package pipelines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"math/rand"
	"strconv"

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
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")
	var buildID string

	if _, err := p.shims.Stat(buildIDPath); os.IsNotExist(err) {
		buildID = ""
	} else {
		data, err := p.shims.ReadFile(buildIDPath)
		if err != nil {
			return fmt.Errorf("failed to read build ID file: %w", err)
		}
		buildID = strings.TrimSpace(string(data))
	}

	if buildID == "" {
		newBuildID, err := p.generateBuildID()
		if err != nil {
			return fmt.Errorf("failed to generate build ID: %w", err)
		}
		if err := p.writeBuildIDToFile(newBuildID); err != nil {
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

	if err := p.writeBuildIDToFile(newBuildID); err != nil {
		return fmt.Errorf("failed to set build ID: %w", err)
	}

	fmt.Printf("%s\n", newBuildID)
	return nil
}

// writeBuildIDToFile writes the provided build ID string to the .windsor/.build-id file in the project root.
// Ensures the .windsor directory exists before writing. Returns an error if directory creation or file write fails.
func (p *BuildIDPipeline) writeBuildIDToFile(buildID string) error {
	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")
	buildIDDir := filepath.Dir(buildIDPath)

	if err := p.shims.MkdirAll(buildIDDir, 0755); err != nil {
		return fmt.Errorf("failed to create build ID directory: %w", err)
	}

	return p.shims.WriteFile(buildIDPath, []byte(buildID), 0644)
}

// generateBuildID generates and returns a build ID string in the format YYMMDD.RANDOM.#.
// YYMMDD is the current date (year, month, day), RANDOM is a random three-digit number for collision prevention,
// and # is a sequential counter incremented for each build on the same day. If a build ID already exists for the current day,
// the counter is incremented; otherwise, a new build ID is generated with counter set to 1. Ensures global ordering and uniqueness.
// Returns the build ID string or an error if generation or retrieval fails.
func (p *BuildIDPipeline) generateBuildID() (string, error) {
	now := time.Now()
	yy := now.Year() % 100
	mm := int(now.Month())
	dd := now.Day()
	datePart := fmt.Sprintf("%02d%02d%02d", yy, mm, dd)

	projectRoot, err := p.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")
	var existingBuildID string

	if _, err := p.shims.Stat(buildIDPath); os.IsNotExist(err) {
		existingBuildID = ""
	} else {
		data, err := p.shims.ReadFile(buildIDPath)
		if err != nil {
			return "", fmt.Errorf("failed to read build ID file: %w", err)
		}
		existingBuildID = strings.TrimSpace(string(data))
	}

	if existingBuildID != "" {
		return p.incrementBuildID(existingBuildID, datePart)
	}

	random := rand.Intn(1000)
	counter := 1
	randomPart := fmt.Sprintf("%03d", random)
	counterPart := fmt.Sprintf("%d", counter)

	return fmt.Sprintf("%s.%s.%s", datePart, randomPart, counterPart), nil
}

// incrementBuildID parses an existing build ID and increments its counter component.
// If the date component differs from the current date, generates a new random number and resets the counter to 1.
// Returns the incremented or reset build ID string, or an error if the input format is invalid.
func (p *BuildIDPipeline) incrementBuildID(existingBuildID, currentDate string) (string, error) {
	parts := strings.Split(existingBuildID, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid build ID format: %s", existingBuildID)
	}

	existingDate := parts[0]
	existingRandom := parts[1]
	existingCounter, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid counter component: %s", parts[2])
	}

	if existingDate != currentDate {
		random := rand.Intn(1000)
		return fmt.Sprintf("%s.%03d.1", currentDate, random), nil
	}

	existingCounter++
	return fmt.Sprintf("%s.%s.%d", existingDate, existingRandom, existingCounter), nil
}
