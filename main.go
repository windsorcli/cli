package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
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

	// Create and register the Context instance
	contextInstance := context.NewContext(cliConfigHandler, shellInstance)
	container.Register("context", contextInstance)

	// Create and register the BaseHelper instance
	baseHelper := helpers.NewBaseHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("baseHelper", baseHelper)

	// Create and register the KubeHelper instance
	kubeHelper := helpers.NewKubeHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("kubeHelper", kubeHelper)

	// Create and register the TerraformHelper instance
	terraformHelper := helpers.NewTerraformHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("terraformHelper", terraformHelper)

	// Create and register the TalosHelper instance
	talosHelper := helpers.NewTalosHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("talosHelper", talosHelper)

	// Create and register the OmniHelper instance
	omniHelper := helpers.NewOmniHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("omniHelper", omniHelper)

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
