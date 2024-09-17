package helpers

type Helper interface {
	GetEnvVars() (map[string]string, error)
}
