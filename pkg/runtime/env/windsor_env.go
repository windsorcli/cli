// The WindsorEnvPrinter is a specialized component that manages Windsor environment configuration.
// It provides Windsor-specific environment variable management and configuration,
// The WindsorEnvPrinter handles context, project root, and secrets management,
// ensuring proper Windsor CLI integration and environment setup for application operations.

package env

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/debug"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// WindsorPrefixedVars are the environment variables that are managed by Windsor.
var WindsorPrefixedVars = []string{
	"WINDSOR_CONTEXT",
	"WINDSOR_CONTEXT_ID",
	"BUILD_ID",
	"WINDSOR_PROJECT_ROOT",
	"WINDSOR_SESSION_TOKEN",
	"WINDSOR_MANAGED_ENV",
	"WINDSOR_MANAGED_ALIAS",
}

// =============================================================================
// Types
// =============================================================================

// WindsorEnvPrinter is a struct that implements Windsor environment configuration
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	evaluator      evaluator.ExpressionEvaluator
	allEnvPrinters []EnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewWindsorEnvPrinter creates a new WindsorEnvPrinter instance
func NewWindsorEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler, eval evaluator.ExpressionEvaluator, allEnvPrinters []EnvPrinter) *WindsorEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &WindsorEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
		evaluator:      eval,
		allEnvPrinters: allEnvPrinters,
	}
}

// GetEnvVars sets WINDSOR_CONTEXT, WINDSOR_CONTEXT_ID, WINDSOR_PROJECT_ROOT,
// WINDSOR_SESSION_TOKEN, and BUILD_ID. It evaluates secret expressions in the context's
// custom environment variables and caches the resolved values. It merges GetManagedEnv
// and GetManagedAlias from every printer into WINDSOR_MANAGED_ENV and WINDSOR_MANAGED_ALIAS.
// This printer runs last among all env printers. It then unsets any key the previous
// WINDSOR_MANAGED_ENV listed that no printer manages this round.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	contextID := e.configHandler.GetString("id", "")
	envVars["WINDSOR_CONTEXT_ID"] = contextID

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	sessionToken, err := e.shell.GetSessionToken()
	if err != nil {
		return nil, fmt.Errorf("error retrieving session token: %w", err)
	}
	envVars["WINDSOR_SESSION_TOKEN"] = sessionToken

	buildID, err := e.getBuildID()
	if err == nil && buildID != "" {
		envVars["BUILD_ID"] = buildID
	}

	originalEnvVars := e.configHandler.GetStringMap("environment")

	_, managedEnvExists := e.shims.LookupEnv("WINDSOR_MANAGED_ENV")

	for k, v := range originalEnvVars {
		if !managedEnvExists {
			e.SetManagedEnv(k)
		}

		normalizedValue := secrets.NormalizeLegacyBraces(v)

		if evaluator.ContainsExpression(normalizedValue) {
			if existingValue, exists := e.shims.LookupEnv(k); exists {
				if managedEnvExists {
					e.SetManagedEnv(k)
				}
				if e.shouldUseCache() && !strings.Contains(existingValue, "<ERROR") {
					e.shell.RegisterSecret(existingValue)
					continue
				}
			}
			envVars[k] = e.evaluateEnvValue(normalizedValue)
		} else {
			envVars[k] = normalizedValue
		}
	}

	var allManagedEnv []string
	var allManagedAlias []string

	allManagedEnv = append(allManagedEnv, e.GetManagedEnv()...)
	allManagedAlias = append(allManagedAlias, e.GetManagedAlias()...)

	for _, printer := range e.allEnvPrinters {
		if printer != nil && printer != e {
			allManagedEnv = append(allManagedEnv, printer.GetManagedEnv()...)
			allManagedAlias = append(allManagedAlias, printer.GetManagedAlias()...)
		}
	}

	windsorVars := make([]string, 0, len(WindsorPrefixedVars))
	for _, varName := range WindsorPrefixedVars {
		if varName == "BUILD_ID" {
			if _, exists := envVars["BUILD_ID"]; exists {
				windsorVars = append(windsorVars, varName)
			}
		} else {
			windsorVars = append(windsorVars, varName)
		}
	}
	allManagedEnv = append(allManagedEnv, windsorVars...)

	if prevManagedEnv, exists := e.shims.LookupEnv("WINDSOR_MANAGED_ENV"); exists && prevManagedEnv != "" {
		stillManaged := make(map[string]bool, len(allManagedEnv))
		for _, k := range allManagedEnv {
			stillManaged[k] = true
		}
		for k := range strings.SplitSeq(prevManagedEnv, ",") {
			k = strings.TrimSpace(k)
			if k == "" || stillManaged[k] {
				continue
			}
			if _, alreadySet := envVars[k]; alreadySet {
				continue
			}
			debug.Log("windsor env: unsetting stale managed var %s (no printer reports it this round)", k)
			envVars[k] = ""
		}
	}

	envVars["WINDSOR_MANAGED_ENV"] = strings.Join(allManagedEnv, ",")
	envVars["WINDSOR_MANAGED_ALIAS"] = strings.Join(allManagedAlias, ",")

	return envVars, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// evaluateEnvValue evaluates an environment value through the expression evaluator.
// Deferred evaluation is enabled (evaluateDeferred=true) so secrets resolve immediately.
func (e *WindsorEnvPrinter) evaluateEnvValue(value string) string {
	if e.evaluator == nil {
		return value
	}
	return evaluateExpressionValue(e.evaluator, value)
}

// getBuildID retrieves the current build ID from the .windsor/.build-id file.
// This is a read-only operation - it never creates a build ID.
// Returns the build ID string if it exists, or empty string if it doesn't exist.
// Returns an error only if file read fails (not if file doesn't exist).
func (e *WindsorEnvPrinter) getBuildID() (string, error) {
	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	if _, err := e.shims.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := e.shims.ReadFile(buildIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read build ID file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// writeBuildIDToFile writes the provided build ID string to the .windsor/.build-id file in the project root.
// Ensures the .windsor directory exists before writing. Returns an error if directory creation or file write fails.
func (e *WindsorEnvPrinter) writeBuildIDToFile(buildID string) error {
	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")
	buildIDDir := filepath.Dir(buildIDPath)

	if err := e.shims.MkdirAll(buildIDDir, 0755); err != nil {
		return fmt.Errorf("failed to create build ID directory: %w", err)
	}

	return e.shims.WriteFile(buildIDPath, []byte(buildID), 0644)
}

// generateBuildID generates and returns a build ID string in the format YYMMDD.RANDOM.#.
// YYMMDD is the current date (year, month, day), RANDOM is a random three-digit number for collision prevention,
// and # is a sequential counter incremented for each build on the same day. If a build ID already exists for the current day,
// the counter is incremented; otherwise, a new build ID is generated with counter set to 1. Ensures global ordering and uniqueness.
// Returns the build ID string or an error if generation or retrieval fails.
func (e *WindsorEnvPrinter) generateBuildID() (string, error) {
	now := time.Now()
	yy := now.Year() % 100
	mm := int(now.Month())
	dd := now.Day()
	datePart := fmt.Sprintf("%02d%02d%02d", yy, mm, dd)

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")
	var existingBuildID string

	if _, err := e.shims.Stat(buildIDPath); os.IsNotExist(err) {
		existingBuildID = ""
	} else {
		data, err := e.shims.ReadFile(buildIDPath)
		if err != nil {
			return "", fmt.Errorf("failed to read build ID file: %w", err)
		}
		existingBuildID = strings.TrimSpace(string(data))
	}

	if existingBuildID != "" {
		return e.incrementBuildID(existingBuildID, datePart)
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
func (e *WindsorEnvPrinter) incrementBuildID(existingBuildID, currentDate string) (string, error) {
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
