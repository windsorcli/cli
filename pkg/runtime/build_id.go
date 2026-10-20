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

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// buildIDPath is the path of the build ID file, relative to the project root.
const buildIDPath = ".windsor/.build-id"

// corruptBuildIDHint tells the operator how to replace a build ID file that does not parse.
const corruptBuildIDHint = "Delete .windsor/.build-id, then run 'windsor build-id --new'."

// =============================================================================
// Public Methods
// =============================================================================

// GetBuildID returns the build ID in .windsor/.build-id, or an empty string if the file does not exist.
// It does not create a build ID. It creates the project root if the project root does not exist.
// A file that it cannot read returns an error with a hint.
func (rt *Runtime) GetBuildID() (string, error) {
	projectRoot := rt.ProjectRoot

	if err := rt.Shims.MkdirAll(projectRoot, 0750); err != nil {
		return "", werror.Wrap(err, "failed to create project root directory")
	}

	root, err := rt.Shims.OpenRoot(projectRoot)
	if err != nil {
		return "", werror.Wrap(err, "failed to open project root")
	}
	if root == nil {
		return "", fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	if _, err := root.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := root.ReadFile(buildIDPath)
	if err != nil {
		return "", werror.New("failed to read build ID file",
			"Check that you have read permission on .windsor/.build-id in the project root.", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// GenerateBuildID makes a new build ID and writes it to .windsor/.build-id, over any existing value.
// It returns the new build ID.
func (rt *Runtime) GenerateBuildID() (string, error) {
	newBuildID, err := rt.generateBuildID()
	if err != nil {
		return "", werror.Wrap(err, "failed to generate build ID")
	}

	if err := rt.writeBuildIDToFile(newBuildID); err != nil {
		return "", werror.Wrap(err, "failed to set build ID")
	}

	return newBuildID, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// generateBuildID returns the next build ID in the YYMMDD.RANDOM.COUNTER format.
// RANDOM is a random three-digit number, and COUNTER counts the builds of one day.
// If a build ID exists, incrementBuildID makes the next one. If not, the counter starts at 1.
func (rt *Runtime) generateBuildID() (string, error) {
	now := time.Now()
	datePart := fmt.Sprintf("%02d%02d%02d", now.Year()%100, int(now.Month()), now.Day())

	existingBuildID, err := rt.GetBuildID()
	if err != nil {
		return "", werror.Wrap(err, "failed to get existing build ID")
	}
	if existingBuildID != "" {
		return rt.incrementBuildID(existingBuildID, datePart)
	}

	random, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return "", werror.Wrap(err, "failed to generate random number")
	}

	return fmt.Sprintf("%s.%03d.1", datePart, random.Int64()), nil
}

// incrementBuildID returns the build ID that follows existingBuildID.
// On the same date, it adds 1 to the counter. On a new date, it uses a new random part and a counter of 1.
// A build ID that does not parse returns an error with a hint.
func (rt *Runtime) incrementBuildID(existingBuildID, currentDate string) (string, error) {
	parts := strings.Split(existingBuildID, ".")
	if len(parts) != 3 {
		return "", werror.New(fmt.Sprintf("invalid build ID format: %s", existingBuildID), corruptBuildIDHint, nil)
	}

	existingDate := parts[0]
	existingRandom := parts[1]
	existingCounter, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", werror.New(fmt.Sprintf("invalid counter component: %s", parts[2]), corruptBuildIDHint, nil)
	}

	if existingDate != currentDate {
		random, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			return "", werror.Wrap(err, "failed to generate random number")
		}
		return fmt.Sprintf("%s.%03d.1", currentDate, random.Int64()), nil
	}

	existingCounter++
	return fmt.Sprintf("%s.%s.%d", existingDate, existingRandom, existingCounter), nil
}

// writeBuildIDToFile writes buildID to .windsor/.build-id in the project root.
// It creates the project root and the .windsor directory if they do not exist.
// A file that it cannot write returns an error with a hint.
func (rt *Runtime) writeBuildIDToFile(buildID string) error {
	projectRoot := rt.ProjectRoot

	if err := rt.Shims.MkdirAll(projectRoot, 0750); err != nil {
		return werror.Wrap(err, "failed to create project root directory")
	}

	root, err := rt.Shims.OpenRoot(projectRoot)
	if err != nil {
		return werror.Wrap(err, "failed to open project root")
	}
	if root == nil {
		return fmt.Errorf("failed to open project root: root is nil")
	}
	defer root.Close()

	if err := root.MkdirAll(".windsor", 0750); err != nil {
		return werror.Wrap(err, "failed to create build ID directory")
	}

	if err := root.WriteFile(buildIDPath, []byte(buildID), 0600); err != nil {
		return werror.New("failed to write build ID file",
			"Check that you have write permission on .windsor in the project root.", err)
	}
	return nil
}
