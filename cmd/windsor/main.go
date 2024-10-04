package main

import (
	"log"

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
	cliConfigHandler, err := config.NewViperConfigHandler("")
	if err != nil {
		log.Fatalf("failed to create CLI config handler: %v", err)
	}
	container.Register("cliConfigHandler", cliConfigHandler)

	// Register Shell instance
	shellInstance := shell.NewDefaultShell()
	container.Register("shell", shellInstance)

	// Register the Project Config Handler (to be initialized later)
	projectConfigHandler, err := config.NewViperConfigHandler("")
	if err != nil {
		log.Fatalf("failed to create project config handler: %v", err)
	}
	container.Register("projectConfigHandler", projectConfigHandler)

	// Create and register the Context instance
	contextInstance := context.NewContext(cliConfigHandler, shellInstance)
	container.Register("context", contextInstance)

	// Create and register the BaseHelper instance
	baseHelper, err := helpers.NewBaseHelper(container)
	if err != nil {
		log.Fatalf("failed to create base helper: %v", err)
	}
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

	// Create and register the SopsHelper instance
	sopsHelper := helpers.NewSopsHelper(cliConfigHandler, shellInstance, contextInstance)
	container.Register("sopsHelper", sopsHelper)

	// Create and register the AwsHelper instance
	awsHelper, err := helpers.NewAwsHelper(container)
	if err != nil {
		log.Fatalf("failed to create aws helper: %v", err)
	}
	container.Register("awsHelper", awsHelper)

	// Create and register the DockerHelper instance
	dockerHelper, err := helpers.NewDockerHelper(container)
	if err != nil {
		log.Fatalf("failed to create docker helper: %v", err)
	}
	container.Register("dockerHelper", dockerHelper)

	// Create and register the ColimaHelper instance
	colimaHelper, err := helpers.NewColimaHelper(container)
	if err != nil {
		log.Fatalf("failed to create colima helper: %v", err)
	}
	container.Register("colimaHelper", colimaHelper)

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
