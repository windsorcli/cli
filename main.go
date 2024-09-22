package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

func main() {
	// Create a new DI container
	container := di.NewContainer()

	// Register CLI Config Handler (to be initialized later)
	cliConfigHandler := config.NewViperConfigHandler("")
	container.Register("cliConfigHandler", cliConfigHandler)

	// Register Shell instance
	shellInstance := shell.NewDefaultShell()
	container.Register("shell", shellInstance)

	// Register the Project Config Handler (to be initialized later)
	projectConfigHandler := config.NewViperConfigHandler("")
	container.Register("projectConfigHandler", projectConfigHandler)

	// Create and register the BaseHelper instance
	baseHelper := helpers.NewBaseHelper(cliConfigHandler, shellInstance)
	container.Register("baseHelper", baseHelper)

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
