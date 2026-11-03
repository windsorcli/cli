// The export_test file gives the black-box tests of package terraform_test their test seams.
// It exposes private fields for fault injection and state checks. It does not expose private methods.

package terraform

import (
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Seams
// =============================================================================

// SetEvaluator sets the evaluator field of the DefaultTerraformProvider.
func (p *DefaultTerraformProvider) SetEvaluator(v evaluator.ExpressionEvaluator) {
	p.evaluator = v
}
