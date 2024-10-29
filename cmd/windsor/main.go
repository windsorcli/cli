package main

import (
	"log"

	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/shell"
)

func main() {
	// Create a new DI container
	container := di.NewContainer()

	// Register CLI Config Handler (to be initialized later)
	cliConfigHandler, err := config.NewYamlConfigHandler("")
	if err != nil {
		log.Fatalf("failed to create CLI config handler: %v", err)
	}
	container.Register("cliConfigHandler", cliConfigHandler)

	// Register Shell instance
	shellInstance := shell.NewDefaultShell()
	container.Register("shell", shellInstance)

	// Create and register the Context instance
	contextInstance := context.NewContext(cliConfigHandler, shellInstance)
	container.Register("contextInstance", contextInstance)

	// Create and register the BaseHelper instance
	baseHelper, err := helpers.NewBaseHelper(container)
	if err != nil {
		log.Fatalf("failed to create base helper: %v", err)
	}
	container.Register("baseHelper", baseHelper)

	// Create and register the KubeHelper instance
	kubeHelper, err := helpers.NewKubeHelper(container)
	if err != nil {
		log.Fatalf("failed to create kube helper: %v", err)
	}
	container.Register("kubeHelper", kubeHelper)

	// Create and register the TerraformHelper instance
	terraformHelper, err := helpers.NewTerraformHelper(container)
	if err != nil {
		log.Fatalf("failed to create terraform helper: %v", err)
	}
	container.Register("terraformHelper", terraformHelper)

	// Create and register the TalosHelper instance
	talosHelper, err := helpers.NewTalosHelper(container)
	if err != nil {
		log.Fatalf("failed to create talos helper: %v", err)
	}
	container.Register("talosHelper", talosHelper)

	// Create and register the OmniHelper instance
	omniHelper, err := helpers.NewOmniHelper(container)
	if err != nil {
		log.Fatalf("failed to create omni helper: %v", err)
	}
	container.Register("omniHelper", omniHelper)

	// Create and register the SopsHelper instance
	sopsHelper, err := helpers.NewSopsHelper(container)
	if err != nil {
		log.Fatalf("failed to create sops helper: %v", err)
	}
	container.Register("sopsHelper", sopsHelper)

	// Create and register the AwsHelper instance
	awsHelper, err := helpers.NewAwsHelper(container)
	if err != nil {
		log.Fatalf("failed to create aws helper: %v", err)
	}
	container.Register("awsHelper", awsHelper)

	// Create and register the GitHelper instance
	gitHelper, err := helpers.NewGitHelper(container)
	if err != nil {
		log.Fatalf("failed to create git helper: %v", err)
	}
	container.Register("gitHelper", gitHelper)

	// Create and register the ColimaHelper instance
	colimaHelper, err := helpers.NewColimaHelper(container)
	if err != nil {
		log.Fatalf("failed to create colima helper: %v", err)
	}
	container.Register("colimaHelper", colimaHelper)

	// Create and register the DNSHelper instance
	dnsHelper, err := helpers.NewDNSHelper(container)
	if err != nil {
		log.Fatalf("failed to create dns helper: %v", err)
	}
	container.Register("dnsHelper", dnsHelper)

	// Create and register the DockerHelper instance
	// This should go last!
	dockerHelper, err := helpers.NewDockerHelper(container)
	if err != nil {
		log.Fatalf("failed to create docker helper: %v", err)
	}
	container.Register("dockerHelper", dockerHelper)

	// Create and register the NetworkManager instance
	networkManager, err := network.NewNetworkManager(container)
	if err != nil {
		log.Fatalf("failed to create network manager: %v", err)
	}
	container.Register("networkManager", networkManager)

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
