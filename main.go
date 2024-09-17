package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/di"
)

func main() {
	// Initialize the DI container
	container := di.NewContainer()

	// Inject the dependencies into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	if err := cmd.Execute(); err != nil {
		// Handle the error appropriately
	}
}
