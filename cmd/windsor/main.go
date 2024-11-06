package main

import (
	"log"

	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/vm"
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

	// Register SecureShell instance
	secureShellInstance, err := shell.NewSecureShell(container)
	if err != nil {
		log.Fatalf("failed to create secure shell: %v", err)
	}
	container.Register("secureShell", secureShellInstance)

	// Create and register the Context instance
	contextHandler := context.NewContext(cliConfigHandler, shellInstance)
	container.Register("contextHandler", contextHandler)

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

	// Register SSH Client instance
	sshClient := ssh.NewSSHClient()
	container.Register("sshClient", sshClient)

	// Create and register the ColimaVM instance using the mock as reference
	colimaVM := vm.NewColimaVM(container)
	container.Register("colimaVM", colimaVM)

	// Create and register the AwsEnv instance
	awsEnv := env.NewAwsEnv(container)
	container.Register("awsEnv", awsEnv)

	// Create and register the DockerEnv instance
	dockerEnv := env.NewDockerEnv(container)
	container.Register("dockerEnv", dockerEnv)

	// Create and register the KubeEnv instance
	kubeEnv := env.NewKubeEnv(container)
	container.Register("kubeEnv", kubeEnv)

	// Create and register the OmniEnv instance
	omniEnv := env.NewOmniEnv(container)
	container.Register("omniEnv", omniEnv)

	// Create and register the SopsEnv instance
	sopsEnv := env.NewSopsEnv(container)
	container.Register("sopsEnv", sopsEnv)

	// Create and register the TalosEnv instance
	talosEnv := env.NewTalosEnv(container)
	container.Register("talosEnv", talosEnv)

	// Create and register the TerraformEnv instance
	terraformEnv := env.NewTerraformEnv(container)
	container.Register("terraformEnv", terraformEnv)

	// Create and register the WindsorEnv instance
	windsorEnv := env.NewWindsorEnv(container)
	container.Register("windsorEnv", windsorEnv)

	// Inject the DI container into the cmd package
	cmd.Initialize(container)

	// Execute the root command
	cmd.Execute()
}
