package main

import (
	"log"

	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/network"
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
	contextHandler := context.NewContext(configHandler, shellInstance)
	injector.Register("contextHandler", contextHandler)

	// Create and register the AwsHelper instance
	awsHelper, err := helpers.NewAwsHelper(injector)
	if err != nil {
		log.Fatalf("failed to create aws helper: %v", err)
	}
	injector.Register("awsHelper", awsHelper)

	// Create and register the GitHelper instance
	gitHelper, err := helpers.NewGitHelper(injector)
	if err != nil {
		log.Fatalf("failed to create git helper: %v", err)
	}
	injector.Register("gitHelper", gitHelper)

	// Create and register the DNSHelper instance
	dnsHelper, err := helpers.NewDNSHelper(injector)
	if err != nil {
		log.Fatalf("failed to create dns helper: %v", err)
	}
	injector.Register("dnsHelper", dnsHelper)

	// Create and register the DockerHelper instance
	// This should go last!
	dockerHelper, err := helpers.NewDockerHelper(injector)
	if err != nil {
		log.Fatalf("failed to create docker helper: %v", err)
	}
	injector.Register("dockerHelper", dockerHelper)

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

	// Execute the root command
	if err := cmd.Execute(injector); err != nil {
		log.Fatalf("failed to execute command: %v", err)
	}
}
