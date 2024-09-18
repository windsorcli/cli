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
	// Load CLI configuration
	cliConfigPath := getConfigPath()

	// Create a new DI container
	container := di.NewContainer()

	// Register CLI Config Handler
	cliConfigHandler := config.NewViperConfigHandler(cliConfigPath)
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

func getConfigPath() string {
	cliConfigPath := os.Getenv("WINDSORCONFIG")
	if cliConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error finding home directory, %s\n", err)
			os.Exit(1)
		}
		cliConfigPath = filepath.Join(home, ".config", "windsor", "config.yaml")
	}
	return cliConfigPath
}
