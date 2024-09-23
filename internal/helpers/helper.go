package helpers

// Helper is an interface that defines methods for retrieving environment variables
// and can be implemented for individual providers.
type Helper interface {
	// GetEnvVars retrieves environment variables for the current context.
	GetEnvVars() (map[string]string, error)
}
