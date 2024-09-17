package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers" // Added import
)

func main() {
	// Create a new DI container
	container := di.NewContainer()

	// Register dependencies
	container.Register("configHandler", &config.ViperConfigHandler{})
	container.Register("baseHelper", func(c *di.Container) interface{} { // Pass by reference
		configHandler, err := c.Resolve("configHandler")
		if err != nil {
			panic(err) // Handle error appropriately
		}
		return helpers.NewBaseHelper(configHandler.(config.ConfigHandler)) // Type assertion
	})

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
