package shell

type Shell interface {
	DetermineShell() string
	PrintEnvVars(envVars map[string]string)
}
