// The export_test file gives the black-box tests of package env_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package env

import (
	"io"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Seams
// =============================================================================

// SetWarningWriter sets the warningWriter field of the AwsEnvPrinter.
func (e *AwsEnvPrinter) SetWarningWriter(v io.Writer) {
	e.warningWriter = v
}

// ConfigHandler returns the configHandler field of the BaseEnvPrinter.
func (e *BaseEnvPrinter) ConfigHandler() config.ConfigHandler {
	return e.configHandler
}

// SetShims sets the shims field of the BaseEnvPrinter.
func (e *BaseEnvPrinter) SetShims(v *Shims) {
	e.shims = v
}

// SetWarningWriter sets the warningWriter field of the DotEnvEnvPrinter.
func (e *DotEnvEnvPrinter) SetWarningWriter(v io.Writer) {
	e.warningWriter = v
}

// Evaluator returns the evaluator field of the WindsorEnvPrinter.
func (e *WindsorEnvPrinter) Evaluator() evaluator.ExpressionEvaluator {
	return e.evaluator
}
