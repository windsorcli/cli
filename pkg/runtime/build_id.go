// The build ID functions of the Runtime read and write the build ID of a project.
// It provides build IDs in the YYMMDD.RANDOM.COUNTER format, stored in .windsor/.build-id.
// A new build on the same day increments the counter.
// A build on a new day gets a new random part and a counter of 1.

package runtime

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// Public Methods
// =============================================================================

// GetBuildID retrieves the current build ID from the .windsor/.build-id file.
// This is a read-only operation - it never creates a build ID.
// Returns the build ID string if it exists, or empty string if it doesn't exist.
// Returns an error only if file read fails (not if file doesn't exist).
func (rt *Runtime) GetBuildID() (string, error) {
	projectRoot := rt.ProjectRoot

	if err := rt.Shims.MkdirAll(projectRoot, 0750); err != nil {
		return "", fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := rt.Shims.OpenRoot(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to open project root: %w", err)
	}
	if root == nil {
		return "", fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := root.ReadFile(buildIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read build ID file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// GenerateBuildID generates a new build ID and persists it to the .windsor/.build-id file,
// overwriting any existing value. Returns the new build ID or an error if generation or persistence fails.
func (rt *Runtime) GenerateBuildID() (string, error) {
	newBuildID, err := rt.generateBuildID()
	if err != nil {
		return "", fmt.Errorf("failed to generate build ID: %w", err)
	}

	if err := rt.writeBuildIDToFile(newBuildID); err != nil {
		return "", fmt.Errorf("failed to set build ID: %w", err)
	}

	return newBuildID, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateBuildID generates and returns a build ID string in the format YYMMDD.RANDOM.#.
// YYMMDD is the current date (year, month, day), RANDOM is a random three-digit number for collision prevention,
// and # is a sequential counter incremented for each build on the same day. If a build ID already exists for the current day,
// the counter is incremented; otherwise, a new build ID is generated with counter set to 1. Ensures global ordering and uniqueness.
// Returns the build ID string or an error if generation or retrieval fails.
func (rt *Runtime) generateBuildID() (string, error) {
	now := time.Now()
	yy := now.Year() % 100
	mm := int(now.Month())
	dd := now.Day()
	datePart := fmt.Sprintf("%02d%02d%02d", yy, mm, dd)

	projectRoot := rt.ProjectRoot

	if err := rt.Shims.MkdirAll(projectRoot, 0750); err != nil {
		return "", fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := rt.Shims.OpenRoot(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to open project root: %w", err)
	}
	if root == nil {
		return "", fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"

	var existingBuildID string

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		existingBuildID = ""
	} else {
		data, err := root.ReadFile(buildIDPath)
		if err != nil {
			return "", fmt.Errorf("failed to read build ID file: %w", err)
		}
		existingBuildID = strings.TrimSpace(string(data))
	}

	if existingBuildID != "" {
		return rt.incrementBuildID(existingBuildID, datePart)
	}

	random, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return "", fmt.Errorf("failed to generate random number: %w", err)
	}
	counter := 1
	randomPart := fmt.Sprintf("%03d", random.Int64())
	counterPart := fmt.Sprintf("%d", counter)

	return fmt.Sprintf("%s.%s.%s", datePart, randomPart, counterPart), nil
}

// incrementBuildID parses an existing build ID and increments its counter component.
// If the date component differs from the current date, generates a new random number and resets the counter to 1.
// Returns the incremented or reset build ID string, or an error if the input format is invalid.
func (rt *Runtime) incrementBuildID(existingBuildID, currentDate string) (string, error) {
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
		random, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		return fmt.Sprintf("%s.%03d.1", currentDate, random.Int64()), nil
	}

	existingCounter++
	return fmt.Sprintf("%s.%s.%d", existingDate, existingRandom, existingCounter), nil
}

// writeBuildIDToFile writes the provided build ID string to the .windsor/.build-id file in the project root.
// Ensures the .windsor directory exists before writing. Returns an error if directory creation or file write fails.
func (rt *Runtime) writeBuildIDToFile(buildID string) error {
	projectRoot := rt.ProjectRoot

	if err := rt.Shims.MkdirAll(projectRoot, 0750); err != nil {
		return fmt.Errorf("failed to create project root directory: %w", err)
	}

	root, err := rt.Shims.OpenRoot(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to open project root: %w", err)
	}
	if root == nil {
		return fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	buildIDPath := ".windsor/.build-id"
	buildIDDir := ".windsor"

	if err := root.MkdirAll(buildIDDir, 0750); err != nil {
		return fmt.Errorf("failed to create build ID directory: %w", err)
	}

	return root.WriteFile(buildIDPath, []byte(buildID), 0600)
}
