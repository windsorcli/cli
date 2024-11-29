package main

import (
	"log"

	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

func main() {
	// Create a new DI injector
	injector := di.NewInjector()

	// Register CLI Config Handler (to be initialized later)
	configHandler, err := config.NewYamlConfigHandler("")
	if err != nil {
		log.Fatalf("failed to create CLI config handler: %v", err)
	}
	injector.Register("configHandler", configHandler)

	// Register Shell instance
	shellInstance := shell.NewDefaultShell(injector)
	injector.Register("shell", shellInstance)

	// Register SecureShell instance
	secureShellInstance, err := shell.NewSecureShell(injector)
	if err != nil {
		log.Fatalf("failed to create secure shell: %v", err)
	}
	injector.Register("secureShell", secureShellInstance)

	// Create and register the Context instance
	contextHandler := context.NewBaseContextHandler(configHandler, shellInstance)
	injector.Register("contextHandler", contextHandler)

	// Create and register the AwsService instance
	awsService := services.NewAwsService(injector)
	injector.Register("awsService", awsService)

	// Create and register the GitService instance
	gitService := services.NewGitService(injector)
	injector.Register("gitService", gitService)

	// Create and register the DNSService instance
	dnsService := services.NewDNSService(injector)
	injector.Register("dnsService", dnsService)

	// Create and register the DockerService instance
	dockerService := services.NewDockerService(injector)
	injector.Register("dockerService", dockerService)

	// Create and register the TalosControlPlaneService instance
	talosControlPlaneService := services.NewTalosControlPlaneService(injector)
	injector.Register("talosControlPlaneService", talosControlPlaneService)

	// Register SSH Client instance
	sshClient := ssh.NewSSHClient()
	injector.Register("sshClient", sshClient)

	// Create and register the ColimaVirt instance
	colimaVM := virt.NewColimaVirt(injector)
	injector.Register("colimaVirt", colimaVM)

	// Create and register the DockerVirt instance
	dockerVM := virt.NewDockerVirt(injector)
	injector.Register("dockerVirt", dockerVM)

	// Create and register the ColimaNetworkManager instance
	colimaNetworkManager := network.NewColimaNetworkManager(injector)
	injector.Register("colimaNetworkManager", colimaNetworkManager)

	// Create and register the AwsEnv instance
	awsEnv := env.NewAwsEnvPrinter(injector)
	injector.Register("awsEnv", awsEnv)

	// Create and register the DockerEnv instance
	dockerEnv := env.NewDockerEnvPrinter(injector)
	injector.Register("dockerEnv", dockerEnv)

	// Create and register the KubeEnv instance
	kubeEnv := env.NewKubeEnvPrinter(injector)
	injector.Register("kubeEnv", kubeEnv)

	// Create and register the OmniEnv instance
	omniEnv := env.NewOmniEnvPrinter(injector)
	injector.Register("omniEnv", omniEnv)

	// Create and register the SopsEnv instance
	sopsEnv := env.NewSopsEnvPrinter(injector)
	injector.Register("sopsEnv", sopsEnv)

	// Create and register the TalosEnv instance
	talosEnv := env.NewTalosEnvPrinter(injector)
	injector.Register("talosEnv", talosEnv)

	// Create and register the TerraformEnv instance
	terraformEnv := env.NewTerraformEnvPrinter(injector)
	injector.Register("terraformEnv", terraformEnv)

	// Create and register the WindsorEnv instance
	windsorEnv := env.NewWindsorEnvPrinter(injector)
	injector.Register("windsorEnv", windsorEnv)

	// Create and register the RealNetworkInterfaceProvider instance
	networkInterfaceProvider := &network.RealNetworkInterfaceProvider{}
	injector.Register("networkInterfaceProvider", networkInterfaceProvider)

	// Execute the root command and handle the error silently,
	// allowing the CLI framework to report the error
	_ = cmd.Execute(injector)
}
