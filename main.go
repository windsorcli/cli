package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func main() {
	// Create a new DI container
	container := di.NewContainer()

	// Register dependencies
	container.Register("configHandler", &config.ViperConfigHandler{})
	container.Register("shell", shell.NewDefaultShell())

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
