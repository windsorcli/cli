package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var (
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
	initBlueprint      string
	initToolsManager   string
	initPlatform       string
	initEndpoint       string
	initSetFlags       []string
	reset              bool
)

var initCmd = &cobra.Command{
	Use:   "init [context]",
	Short: "Initialize the application environment",
	Long:  "Initialize the application environment with the specified context configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// First, run the env pipeline in quiet mode to set up environment variables
		envPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "envPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up env pipeline: %w", err)
		}
		envCtx := context.WithValue(cmd.Context(), "quiet", true)
		envCtx = context.WithValue(envCtx, "decrypt", true)
		if err := envPipeline.Execute(envCtx); err != nil {
			return fmt.Errorf("failed to set up environment: %w", err)
		}

		// Set up the init pipeline
		initPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "initPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up init pipeline: %w", err)
		}

		ctx := cmd.Context()

		// Add context name and reset flag to context (these are needed during Initialize)
		if len(args) > 0 {
			ctx = context.WithValue(ctx, "contextName", args[0])
		}
		ctx = context.WithValue(ctx, "reset", reset)

		// Set flag values in config handler before execution
		configHandler := injector.Resolve("configHandler").(config.ConfigHandler)

		if initBackend != "" {
			if err := configHandler.SetContextValue("terraform.backend.type", initBackend); err != nil {
				return fmt.Errorf("failed to set terraform.backend.type: %w", err)
			}
		}
		if initAwsProfile != "" {
			if err := configHandler.SetContextValue("aws.profile", initAwsProfile); err != nil {
				return fmt.Errorf("failed to set aws.profile: %w", err)
			}
		}
		if initAwsEndpointURL != "" {
			if err := configHandler.SetContextValue("aws.endpoint_url", initAwsEndpointURL); err != nil {
				return fmt.Errorf("failed to set aws.endpoint_url: %w", err)
			}
		}
		if initVmDriver != "" {
			if err := configHandler.SetContextValue("vm.driver", initVmDriver); err != nil {
				return fmt.Errorf("failed to set vm.driver: %w", err)
			}
		}
		if initCpu > 0 {
			if err := configHandler.SetContextValue("vm.cpu", initCpu); err != nil {
				return fmt.Errorf("failed to set vm.cpu: %w", err)
			}
		}
		if initDisk > 0 {
			if err := configHandler.SetContextValue("vm.disk", initDisk); err != nil {
				return fmt.Errorf("failed to set vm.disk: %w", err)
			}
		}
		if initMemory > 0 {
			if err := configHandler.SetContextValue("vm.memory", initMemory); err != nil {
				return fmt.Errorf("failed to set vm.memory: %w", err)
			}
		}
		if initArch != "" {
			if err := configHandler.SetContextValue("vm.arch", initArch); err != nil {
				return fmt.Errorf("failed to set vm.arch: %w", err)
			}
		}
		if initDocker {
			if err := configHandler.SetContextValue("docker.enabled", true); err != nil {
				return fmt.Errorf("failed to set docker.enabled: %w", err)
			}
		}
		if initGitLivereload {
			if err := configHandler.SetContextValue("git.livereload.enabled", true); err != nil {
				return fmt.Errorf("failed to set git.livereload.enabled: %w", err)
			}
		}
		if initBlueprint != "" {
			if err := configHandler.SetContextValue("blueprint", initBlueprint); err != nil {
				return fmt.Errorf("failed to set blueprint: %w", err)
			}
		}
		if initToolsManager != "" {
			if err := configHandler.SetContextValue("tools.manager", initToolsManager); err != nil {
				return fmt.Errorf("failed to set tools.manager: %w", err)
			}
		}
		if initPlatform != "" {
			if err := configHandler.SetContextValue("platform", initPlatform); err != nil {
				return fmt.Errorf("failed to set platform: %w", err)
			}
		}

		// Handle set flags
		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) == 2 {
				if err := configHandler.SetContextValue(parts[0], parts[1]); err != nil {
					return fmt.Errorf("failed to set %s: %w", parts[0], err)
				}
			}
		}

		return initPipeline.Execute(ctx)
	},
}

func init() {
	initCmd.Flags().StringVar(&initBackend, "backend", "", "Specify the terraform backend to use")
	initCmd.Flags().StringVar(&initAwsProfile, "aws-profile", "", "Specify the AWS profile to use")
	initCmd.Flags().StringVar(&initAwsEndpointURL, "aws-endpoint-url", "", "Specify the AWS endpoint URL to use")
	initCmd.Flags().StringVar(&initVmDriver, "vm-driver", "", "Specify the VM driver. Only Colima is supported for now.")
	initCmd.Flags().IntVar(&initCpu, "vm-cpu", 0, "Specify the number of CPUs for Colima")
	initCmd.Flags().IntVar(&initDisk, "vm-disk", 0, "Specify the disk size for Colima")
	initCmd.Flags().IntVar(&initMemory, "vm-memory", 0, "Specify the memory size for Colima")
	initCmd.Flags().StringVar(&initArch, "vm-arch", "", "Specify the architecture for Colima")
	initCmd.Flags().BoolVar(&initDocker, "docker", false, "Enable Docker")
	initCmd.Flags().BoolVar(&initGitLivereload, "git-livereload", false, "Enable Git Livereload")
	initCmd.Flags().StringVar(&initPlatform, "platform", "", "Specify the platform to use [local|metal]")
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "", "Specify the blueprint to use")
	initCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Specify the kubernetes API endpoint")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false --set cluster.endpoint=https://localhost:6443")
	initCmd.Flags().BoolVar(&reset, "reset", false, "Reset/overwrite existing files and clean .terraform directory")
	rootCmd.AddCommand(initCmd)
}
