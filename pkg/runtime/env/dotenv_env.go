// The DotEnvEnvPrinter is a specialized component that manages per-context .env environment configuration.
// It provides a lightweight, portable mechanism for operators to supply provider environment variables.
// The DotEnvEnvPrinter reads contexts/<context>/.env, resolves secret(...) and other evaluator
// expressions, and registers loaded values for output scrubbing since the file may hold credentials.

package env

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// dotEnvFileName is the name of the per-context environment file.
const dotEnvFileName = ".env"

// =============================================================================
// Types
// =============================================================================

// DotEnvEnvPrinter is a struct that implements per-context .env environment configuration
type DotEnvEnvPrinter struct {
	BaseEnvPrinter
	evaluator     evaluator.ExpressionEvaluator
	warningWriter io.Writer
}

// =============================================================================
// Constructor
// =============================================================================

// NewDotEnvEnvPrinter creates a new DotEnvEnvPrinter instance
func NewDotEnvEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler, eval evaluator.ExpressionEvaluator) *DotEnvEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &DotEnvEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
		evaluator:      eval,
		warningWriter:  os.Stderr,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars loads contexts/<context>/.env, evaluating secret(...) expressions and
// registering every value with the shell for output scrubbing. A key already cached in
// the shell is omitted rather than re-evaluated. Returns an empty map when the file is
// absent, and warns (without failing) when the file has group- or world-readable permissions.
func (e *DotEnvEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	dotEnvPath := filepath.Join(configRoot, dotEnvFileName)

	info, err := e.shims.Stat(dotEnvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return envVars, nil
		}
		return nil, fmt.Errorf("error checking %s: %w", dotEnvPath, err)
	}

	e.warnOnLoosePermissions(dotEnvPath, info.Mode())

	data, err := e.shims.ReadFile(dotEnvPath)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", dotEnvPath, err)
	}

	for k, v := range parseDotEnv(string(data)) {
		e.SetManagedEnv(k)

		normalizedValue := secrets.NormalizeLegacyBraces(v)

		if e.evaluator != nil && evaluator.ContainsExpression(normalizedValue) {
			if existingValue, exists := e.shims.LookupEnv(k); exists &&
				e.shouldUseCache() && !strings.Contains(existingValue, "<ERROR") {
				continue
			}
			envVars[k] = evaluateExpressionValue(e.evaluator, normalizedValue)
		} else {
			envVars[k] = normalizedValue
		}
		e.shell.RegisterSecret(envVars[k])
	}

	return envVars, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// warnOnLoosePermissions writes a non-fatal warning to warningWriter when path is
// group- or world-accessible rather than restricted to the owner (0600-equivalent).
func (e *DotEnvEnvPrinter) warnOnLoosePermissions(path string, mode os.FileMode) {
	if mode.Perm()&0077 != 0 {
		fmt.Fprintf(e.warningWriter, "\033[33mWarning: %s is readable by group/other; it may contain credentials. Consider running: chmod 600 %s\033[0m\n", path, path)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// parseDotEnv parses standard dotenv content (KEY=VALUE lines, # comments, blank
// lines) into a map. Lines without a top-level "=" are skipped silently.
func parseDotEnv(content string) map[string]string {
	result := make(map[string]string)

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		result[key] = strings.TrimSpace(value)
	}

	return result
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure DotEnvEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*DotEnvEnvPrinter)(nil)
