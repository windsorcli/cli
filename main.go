package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
)

func main() {
	// Create a new DI container
	container := di.NewContainer()

	// Register dependencies
	container.Register("configHandler", &config.ViperConfigHandler{})
	container.Register("baseHelper", func(c di.ContainerInterface) interface{} {
		instance, err := c.Resolve("configHandler")
		if err != nil {
			panic(err)
		}
		configHandler, ok := instance.(config.ConfigHandler)
		if !ok {
			panic("resolved instance is not of type config.ConfigHandler")
		}
		return helpers.NewBaseHelper(configHandler)
	})

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
