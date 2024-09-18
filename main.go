package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

func main() {

	// Load configuration
	var path = os.Getenv("WINDSORCONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error finding home directory, %s\n", err)
			os.Exit(1)
		}
		path = filepath.Join(home, ".config", "windsor", "config.yaml")
	}

	// Create a new DI container
	container := di.NewContainer()

	// Register dependencies
	configHandler := config.NewViperConfigHandler(path)
	shellInstance := shell.NewDefaultShell()
	container.Register("configHandler", configHandler)
	container.Register("shell", shellInstance)

	// Create and register the BaseHelper instance
	container.Register("baseHelper", helpers.NewBaseHelper(configHandler, shellInstance))

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
