package shell

type Shell interface {
	PrintEnvVars(envVars map[string]string)
}

type DefaultShell struct{}

func NewDefaultShell() *DefaultShell {
	return &DefaultShell{}
}
