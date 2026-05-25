package blueprint

// RequirementsError signals that one or more required facet inputs are not set. It carries the
// formatted operator-facing message produced by formatRequirementsError. Pipeline callers that
// would otherwise wrap blueprint errors with internal context (project's "failed to load
// blueprint data", handler's "failed to compose blueprint" and "failed to process facets for
// 'X'") detect this type via errors.As and pass it through unwrapped, so the operator sees the
// prose alone instead of a chain of internal frame names.
type RequirementsError struct {
	Message string
}

func (e *RequirementsError) Error() string {
	return e.Message
}
