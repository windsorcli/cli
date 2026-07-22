// The HetznerEnvPrinter is a specialized component that manages Hetzner Cloud environment configuration.
// It provides Hetzner-specific environment variable management and configuration.
// The HetznerEnvPrinter emits the Hetzner Cloud API token as HCLOUD_TOKEN, the credential the
// hcloud Terraform provider and CLI read, resolving secret(...) expressions and registering the
// value for output scrubbing since the token is a credential.

package env

import (
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// HetznerEnvPrinter is a struct that implements Hetzner Cloud environment configuration
type HetznerEnvPrinter struct {
	BaseEnvPrinter
	evaluator evaluator.ExpressionEvaluator
}

// =============================================================================
// Constructor
// =============================================================================

// NewHetznerEnvPrinter creates a new HetznerEnvPrinter instance
func NewHetznerEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler, eval evaluator.ExpressionEvaluator) *HetznerEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &HetznerEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
		evaluator:      eval,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the Hetzner Cloud environment.
// HCLOUD_TOKEN is emitted whenever hetzner.token is set, in both project and global modes,
// since the token authenticates the hcloud Terraform provider and CLI which run from global
// shells too. A secret(...) expression is resolved through the evaluator and the resolved
// value registered for output scrubbing; an already-exported token is reused rather than
// re-resolved so the secret provider is not hit on every shell prompt. When the token is unset
// the var is omitted so the operator's ambient HCLOUD_TOKEN or credential chain applies.
func (e *HetznerEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	config := e.configHandler.GetConfig()
	if config == nil || config.Hetzner == nil || config.Hetzner.Token == nil {
		return envVars, nil
	}

	e.SetManagedEnv("HCLOUD_TOKEN")
	normalizedValue := secrets.NormalizeLegacyBraces(*config.Hetzner.Token)

	if e.evaluator != nil && evaluator.ContainsExpression(normalizedValue) {
		if existingValue, exists := e.shims.LookupEnv("HCLOUD_TOKEN"); exists &&
			e.shouldUseCache() && !strings.Contains(existingValue, "<ERROR") {
			e.shell.RegisterSecret(existingValue)
			return envVars, nil
		}
		envVars["HCLOUD_TOKEN"] = evaluateExpressionValue(e.evaluator, normalizedValue)
	} else {
		envVars["HCLOUD_TOKEN"] = normalizedValue
	}
	e.shell.RegisterSecret(envVars["HCLOUD_TOKEN"])

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure HetznerEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*HetznerEnvPrinter)(nil)
