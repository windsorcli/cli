// The RequirementsError is the error for facet inputs that are required but not set.
// It provides the operator-facing message that formatRequirementsError builds.
// Callers detect it with errors.As and return it without more wrap frames.
// The operator then sees only the message.

package blueprint

// =============================================================================
// Types
// =============================================================================

// RequirementsError is the error for required facet inputs that are not set. Callers that wrap
// blueprint errors detect it with errors.As and return it unwrapped, so the operator sees only
// the message that formatRequirementsError builds.
type RequirementsError struct {
	Message string
}

// =============================================================================
// Public Methods
// =============================================================================

// Error returns the operator-facing message.
func (e *RequirementsError) Error() string {
	return e.Message
}
