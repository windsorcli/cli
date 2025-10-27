package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/runtime"
)

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
	initPlatform       string // Deprecated: use initProvider instead
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
		ctx := cmd.Context()
		if len(args) > 0 {
			ctx = context.WithValue(ctx, "contextName", args[0])
		}
		ctx = context.WithValue(ctx, "reset", initReset)
		ctx = context.WithValue(ctx, "trust", true)

		// Handle deprecated --platform flag (must come before automatic provider/blueprint setting)
		if initPlatform != "" {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: The --platform flag is deprecated and will be removed in a future version. Please use --provider instead.\033[0m\n")
			initProvider = initPlatform
		}

		ctx = context.WithValue(ctx, "initPipeline", true)

		// Set up environment variables using runtime
		deps := &runtime.Dependencies{
			Injector: injector,
		}
		if err := runtime.NewRuntime(deps).
			LoadShell().
			LoadConfig().
			LoadSecretsProviders().
			LoadEnvVars(runtime.EnvVarsOptions{
				Decrypt: true,
				Verbose: verbose,
			}).
			ExecutePostEnvHook(verbose).
			Do(); err != nil {
			return fmt.Errorf("failed to set up environment: %w", err)
		}

		// Set provider if context is "local" and no provider is specified
		if len(args) > 0 && strings.HasPrefix(args[0], "local") && initProvider == "" {
			initProvider = "generic"
		}

		// Pass blueprint and provider to pipeline for decision logic
		if initBlueprint != "" {
			ctx = context.WithValue(ctx, "blueprint", initBlueprint)
		}
		if initProvider != "" {
			ctx = context.WithValue(ctx, "provider", initProvider)
		}

		configHandler := injector.Resolve("configHandler").(config.ConfigHandler)

		// Initialize the config handler to ensure schema validator is available
		if err := configHandler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize config handler: %w", err)
		}

		// Set provider in context if it's been set (either via --provider or --platform)
		if initProvider != "" {
			if err := configHandler.Set("provider", initProvider); err != nil {
				return fmt.Errorf("failed to set provider: %w", err)
			}
		}

		// Set other configuration values
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

		hasSetFlags := len(initSetFlags) > 0
		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) == 2 {
				if err := configHandler.Set(parts[0], parts[1]); err != nil {
					return fmt.Errorf("failed to set %s: %w", parts[0], err)
				}
			}
		}

		ctx = context.WithValue(ctx, "hasSetFlags", hasSetFlags)
		ctx = context.WithValue(ctx, "quiet", false)
		ctx = context.WithValue(ctx, "decrypt", false)
		initPipeline, err := pipelines.WithPipeline(injector, ctx, "initPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up init pipeline: %w", err)
		}

		return initPipeline.Execute(ctx)
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
