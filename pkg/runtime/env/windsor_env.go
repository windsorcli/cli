// The WindsorEnvPrinter is a specialized component that manages Windsor environment configuration.
// It provides Windsor-specific environment variable management and configuration,
// The WindsorEnvPrinter handles context, project root, and secrets management,
// ensuring proper Windsor CLI integration and environment setup for application operations.

package env

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
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
		return nil, werror.Wrap(err, "error retrieving project root")
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	sessionToken, err := e.shell.GetSessionToken()
	if err != nil {
		return nil, werror.Wrap(err, "error retrieving session token")
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
		return "", werror.Wrap(err, "failed to get project root")
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	if _, err := e.shims.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := e.shims.ReadFile(buildIDPath)
	if err != nil {
		return "", werror.Wrap(err, "failed to read build ID file")
	}

	return strings.TrimSpace(string(data)), nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
