package env

import (
	"github.com/windsor-hotel/cli/internal/di"
)

// EnvInterface defines the methods for environment-specific utility functions
type EnvInterface interface {
	Print(envVars map[string]string)
	PostEnvHook() error
}

// Env is a struct that implements the EnvInterface
type Env struct {
	diContainer di.ContainerInterface
}
