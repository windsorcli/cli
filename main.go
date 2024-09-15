package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
)

func main() {
	// Initialize the config handler
	configHandler := &config.ViperConfigHandler{}

	// Inject the config handler into the cmd package
	cmd.Initialize(configHandler)

	// Execute the root command
	cmd.Execute()
}
