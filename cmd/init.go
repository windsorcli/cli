package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Shared Init Logic
// =============================================================================

// runInit performs the common initialization logic for init, up, and down commands.
// It creates execution contexts, sets up infrastructure dependencies, applies default configs,
// generates configurations, and persists the config state.
func runInit(injector di.Injector, contextName string, overwrite bool) error {
	baseCtx := &context.ExecutionContext{
		Injector: injector,
	}

	baseCtx, err := context.NewContext(baseCtx)
	if err != nil {
		return fmt.Errorf("failed to initialize context: %w", err)
	}

	if err := baseCtx.Shell.AddCurrentDirToTrustedFile(); err != nil {
		return fmt.Errorf("failed to add current directory to trusted file: %w", err)
	}

	configHandler := baseCtx.ConfigHandler

	if err := configHandler.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config handler: %w", err)
	}

	if err := configHandler.SetContext(contextName); err != nil {
		return fmt.Errorf("failed to set context: %w", err)
	}

	if !configHandler.IsLoaded() {
		existingProvider := configHandler.GetString("provider")
		isDevMode := configHandler.IsDevMode(contextName)

		if isDevMode {
			if err := configHandler.Set("dev", true); err != nil {
				return fmt.Errorf("failed to set dev mode: %w", err)
			}
		}

		vmDriver := configHandler.GetString("vm.driver")
		if isDevMode && vmDriver == "" {
			switch runtime.GOOS {
			case "darwin", "windows":
				vmDriver = "docker-desktop"
			default:
				vmDriver = "docker"
			}
		}

		if vmDriver == "docker-desktop" {
			if err := configHandler.SetDefault(config.DefaultConfig_Localhost); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else if isDevMode {
			if err := configHandler.SetDefault(config.DefaultConfig_Full); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		} else {
			if err := configHandler.SetDefault(config.DefaultConfig); err != nil {
				return fmt.Errorf("failed to set default config: %w", err)
			}
		}

		if isDevMode && configHandler.GetString("vm.driver") == "" && vmDriver != "" {
			if err := configHandler.Set("vm.driver", vmDriver); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
			}
		}

		if existingProvider == "" && isDevMode {
			if err := configHandler.Set("provider", "generic"); err != nil {
				return fmt.Errorf("failed to set provider from context name: %w", err)
			}
		}
	}

	provider := configHandler.GetString("provider")
	if provider != "" {
		switch provider {
		case "aws":
			if err := configHandler.Set("aws.enabled", true); err != nil {
				return fmt.Errorf("failed to set aws.enabled: %w", err)
			}
			if err := configHandler.Set("cluster.driver", "eks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "azure":
			if err := configHandler.Set("azure.enabled", true); err != nil {
				return fmt.Errorf("failed to set azure.enabled: %w", err)
			}
			if err := configHandler.Set("cluster.driver", "aks"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		case "generic":
			if err := configHandler.Set("cluster.driver", "talos"); err != nil {
				return fmt.Errorf("failed to set cluster.driver: %w", err)
			}
		}
	}

	provCtx := &provisioner.ProvisionerExecutionContext{
		ExecutionContext: *baseCtx,
	}
	_ = provisioner.NewProvisioner(provCtx)

	if configHandler.IsDevMode(contextName) {
		workstationCtx := &workstation.WorkstationExecutionContext{
			ExecutionContext: *baseCtx,
		}
		_, err = workstation.NewWorkstation(workstationCtx, injector)
		if err != nil {
			return fmt.Errorf("failed to initialize workstation: %w", err)
		}
	}

	if err := configHandler.GenerateContextID(); err != nil {
		return fmt.Errorf("failed to generate context ID: %w", err)
	}

	if err := configHandler.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := configHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to reload context config: %w", err)
	}

	composerCtx := &composer.ComposerExecutionContext{
		ExecutionContext: *baseCtx,
	}

	comp := composer.NewComposer(composerCtx)

	if err := comp.Generate(overwrite); err != nil {
		return fmt.Errorf("failed to generate infrastructure: %w", err)
	}

	return nil
}

// =============================================================================
// Init Command
// =============================================================================

var (
	initReset          bool
	initBackend        string
	initAwsProfile     string
	initAwsEndpointURL string
	initVmDriver       string
	initCpu            int
	initDisk           int
	initMemory         int
	initArch           string
	initDocker         bool
	initGitLivereload  bool
	initProvider       string
	initPlatform       string
	initBlueprint      string
	initEndpoint       string
	initSetFlags       []string
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application environment",
	Long:         "Initialize the application environment with the specified context configuration",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		baseCtx := &context.ExecutionContext{
			Injector: injector,
		}

		baseCtx, err := context.NewContext(baseCtx)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		contextName := "local"
		if len(args) > 0 {
			contextName = args[0]
		} else {
			currentContext := baseCtx.ConfigHandler.GetContext()
			if currentContext != "" && currentContext != "local" {
				contextName = currentContext
			}
		}

		if initPlatform != "" {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: The --platform flag is deprecated and will be removed in a future version. Please use --provider instead.\033[0m\n")
			initProvider = initPlatform
		}

		if baseCtx.ConfigHandler.IsDevMode(contextName) && initProvider == "" {
			initProvider = "generic"
		}

		configHandler := baseCtx.ConfigHandler

		if initProvider != "" {
			if err := configHandler.Set("provider", initProvider); err != nil {
				return fmt.Errorf("failed to set provider: %w", err)
			}
		}
		if initBackend != "" {
			if err := configHandler.Set("terraform.backend.type", initBackend); err != nil {
				return fmt.Errorf("failed to set terraform.backend.type: %w", err)
			}
		}
		if initAwsProfile != "" {
			if err := configHandler.Set("aws.profile", initAwsProfile); err != nil {
				return fmt.Errorf("failed to set aws.profile: %w", err)
			}
		}
		if initAwsEndpointURL != "" {
			if err := configHandler.Set("aws.endpoint_url", initAwsEndpointURL); err != nil {
				return fmt.Errorf("failed to set aws.endpoint_url: %w", err)
			}
		}
		if initVmDriver != "" {
			if err := configHandler.Set("vm.driver", initVmDriver); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
			}
		}
		if initCpu > 0 {
			if err := configHandler.Set("vm.cpu", initCpu); err != nil {
				return fmt.Errorf("failed to set vm.cpu: %w", err)
			}
		}
		if initDisk > 0 {
			if err := configHandler.Set("vm.disk", initDisk); err != nil {
				return fmt.Errorf("failed to set vm.disk: %w", err)
			}
		}
		if initMemory > 0 {
			if err := configHandler.Set("vm.memory", initMemory); err != nil {
				return fmt.Errorf("failed to set vm.memory: %w", err)
			}
		}
		if initArch != "" {
			if err := configHandler.Set("vm.arch", initArch); err != nil {
				return fmt.Errorf("failed to set vm.arch: %w", err)
			}
		}
		if initDocker {
			if err := configHandler.Set("docker.enabled", true); err != nil {
				return fmt.Errorf("failed to set docker.enabled: %w", err)
			}
		}
		if initGitLivereload {
			if err := configHandler.Set("git.livereload.enabled", true); err != nil {
				return fmt.Errorf("failed to set git.livereload.enabled: %w", err)
			}
		}

		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) == 2 {
				if err := configHandler.Set(parts[0], parts[1]); err != nil {
					return fmt.Errorf("failed to set %s: %w", parts[0], err)
				}
			}
		}

		if err := runInit(injector, contextName, initReset); err != nil {
			return err
		}

		hasSetFlags := len(initSetFlags) > 0
		if err := configHandler.SaveConfig(hasSetFlags); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Initialization successful")

		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initReset, "reset", false, "Reset/overwrite existing files and clean .terraform directory")
	initCmd.Flags().StringVar(&initBackend, "backend", "", "Specify the backend to use")
	initCmd.Flags().StringVar(&initAwsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&initAwsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&initVmDriver, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&initCpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&initDisk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&initMemory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&initArch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&initDocker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&initGitLivereload, "git-livereload", false, "Enable Git Livereload")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "Specify the provider to use [local|metal|aws|azure]")
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Deprecated: use --provider instead")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Specify the blueprint to use")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Specify the kubernetes API endpoint")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false --set cluster.endpoint=https://localhost:6443")

	rootCmd.AddCommand(initCmd)
}
