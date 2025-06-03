package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

var (
	installFlag bool // Declare the install flag
	waitFlag    bool // Declare the wait flag
)

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Set up the Windsor environment",
	Long:         "Set up the Windsor environment by executing necessary shell commands.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Don't do any caching of application state (secrets, etc.) when performing "up"
		if err := shims.Setenv("NO_CACHE", "true"); err != nil {
			return fmt.Errorf("Error setting NO_CACHE environment variable: %w", err)
		}

		// Initialize with requirements
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			ConfigLoaded: true,
			Env:          true,
			Secrets:      true,
			VM:           true,
			Containers:   true,
			Services:     true,
			Network:      true,
			Blueprint:    true,
			Kubernetes:   true,
			Generators:   true,
			Stack:        true,
			CommandName:  cmd.Name(),
			Flags: map[string]bool{
				"verbose": verbose,
			},
		}); err != nil {
			return fmt.Errorf("Error initializing: %w", err)
		}

		// Set the environment variables internally in the process
		if err := controller.SetEnvironmentVariables(); err != nil {
			return fmt.Errorf("Error setting environment variables: %w", err)
		}

		if err := controller.WriteConfigurationFiles(); err != nil {
			return fmt.Errorf("Error writing configuration files: %w", err)
		}

		// Resolve the secrets provider to unlock secrets.
		secretsProviders := controller.ResolveAllSecretsProviders()
		for _, secretsProvider := range secretsProviders {
			if err := secretsProvider.LoadSecrets(); err != nil {
				return fmt.Errorf("Error loading secrets: %w", err)
			}
		}

		// Resolve the tools manager, check the tools, and install them
		toolsManager := controller.ResolveToolsManager()
		if toolsManager != nil {
			if err := toolsManager.Check(); err != nil {
				return fmt.Errorf("Error checking tools: %w", err)
			}
			if err := toolsManager.Install(); err != nil {
				return fmt.Errorf("Error installing tools: %w", err)
			}
		}

		// Resolve the config handler
		configHandler := controller.ResolveConfigHandler()

		// If the virtualization driver is 'colima', start the virtual machine and configure networking.
		vmDriverConfig := configHandler.GetString("vm.driver")
		if vmDriverConfig == "colima" {
			virtualMachine := controller.ResolveVirtualMachine()
			if virtualMachine == nil {
				return fmt.Errorf("No virtual machine found")
			}
			if err := virtualMachine.Up(); err != nil {
				return fmt.Errorf("Error running virtual machine Up command: %w", err)
			}
		}

		// If the container runtime is enabled in the configuration, start it.
		containerRuntimeEnabled := configHandler.GetBool("docker.enabled")
		if containerRuntimeEnabled {
			containerRuntime := controller.ResolveContainerRuntime()
			if containerRuntime == nil {
				return fmt.Errorf("No container runtime found")
			}
			if err := containerRuntime.Up(); err != nil {
				return fmt.Errorf("Error running container runtime Up command: %w", err)
			}
		}

		// Resolve the network manager
		networkManager := controller.ResolveNetworkManager()
		if networkManager == nil {
			return fmt.Errorf("No network manager found")
		}

		if networkManager != nil {
			// Configure networking for the virtual machine
			if vmDriverConfig == "colima" {
				if err := networkManager.ConfigureGuest(); err != nil {
					return fmt.Errorf("Error configuring guest network: %w", err)
				}
				if err := networkManager.ConfigureHostRoute(); err != nil {
					return fmt.Errorf("Error configuring host network: %w", err)
				}
			}

			// Configure DNS settings
			if dnsEnabled := configHandler.GetBool("dns.enabled"); dnsEnabled {
				if err := networkManager.ConfigureDNS(); err != nil {
					return fmt.Errorf("Error configuring DNS: %w", err)
				}
			}
		}

		// Start the stack components
		stack := controller.ResolveStack()
		if stack == nil {
			return fmt.Errorf("No stack found")
		}
		if err := stack.Up(); err != nil {
			return fmt.Errorf("Error running stack Up command: %w", err)
		}

		// Initialize Kubernetes client after stack is up
		kubernetesManager := controller.ResolveKubernetesManager()
		if kubernetesManager != nil {
			if err := kubernetesManager.InitializeClient(); err != nil {
				return fmt.Errorf("Error initializing kubernetes client: %w", err)
			}
		}

		// If the install flag is set, install the blueprint to finalize the setup process.
		if installFlag {
			blueprintHandler := controller.ResolveBlueprintHandler()
			if blueprintHandler == nil {
				return fmt.Errorf("No blueprint handler found")
			}
			if err := blueprintHandler.Install(); err != nil {
				return fmt.Errorf("Error installing blueprint: %w", err)
			}
			// If wait flag is set, poll for kustomization readiness
			if waitFlag {
				if err := blueprintHandler.WaitForKustomizations("⏳ Waiting for kustomizations to be ready"); err != nil {
					return fmt.Errorf("Error waiting for kustomizations: %w", err)
				}
			}
		}

		// Indicate successful setup of the Windsor environment.
		fmt.Fprintln(os.Stderr, "Windsor environment set up successfully.")

		return nil
	},
}

func init() {
	upCmd.Flags().BoolVar(&installFlag, "install", false, "Install the blueprint after setting up the environment")
	upCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for kustomization resources to be ready")
	rootCmd.AddCommand(upCmd)
}
