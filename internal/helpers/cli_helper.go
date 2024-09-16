package helpers

type CLIHelperInterface interface {
	GetEnvVars() (map[string]string, error)
	PrintEnvVars() error
}
