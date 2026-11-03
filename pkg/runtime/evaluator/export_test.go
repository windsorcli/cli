// The export_test file gives the black-box tests of package evaluator_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package evaluator

import (
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Seams
// =============================================================================

// ConfigHandler returns the configHandler field of the DefaultExpressionEvaluator.
func (e *DefaultExpressionEvaluator) ConfigHandler() config.ConfigHandler {
	return e.configHandler
}

// ProjectRoot returns the projectRoot field of the DefaultExpressionEvaluator.
func (e *DefaultExpressionEvaluator) ProjectRoot() string {
	return e.projectRoot
}

// TemplateData returns the templateData field of the DefaultExpressionEvaluator.
func (e *DefaultExpressionEvaluator) TemplateData() map[string][]byte {
	return e.templateData
}

// TemplateRoot returns the templateRoot field of the DefaultExpressionEvaluator.
func (e *DefaultExpressionEvaluator) TemplateRoot() string {
	return e.templateRoot
}
