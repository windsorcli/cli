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
		injector := di.NewInjector()
		pipeline := pipelines.NewInitPipeline()

		ctx := context.Background()

		// Add context name and reset flag to context (these are needed during Initialize)
		if len(args) > 0 {
			ctx = context.WithValue(ctx, "contextName", args[0])
		}
		ctx = context.WithValue(ctx, "reset", reset)

		if err := pipeline.Initialize(injector, ctx); err != nil {
			return fmt.Errorf("failed to initialize pipeline: %w", err)
		}

		// Set flag values in config handler before execution
		configHandler := injector.Resolve("configHandler").(config.ConfigHandler)

		if initBackend != "" {
			configHandler.Set("backend", initBackend)
		}
		if initAwsProfile != "" {
			configHandler.Set("aws.profile", initAwsProfile)
		}
		if initAwsEndpointURL != "" {
			configHandler.Set("aws.endpoint_url", initAwsEndpointURL)
		}
		if initVmDriver != "" {
			configHandler.SetContextValue("vm.driver", initVmDriver)
		}
		if initCpu > 0 {
			configHandler.Set("vm.cpu", initCpu)
		}
		if initDisk > 0 {
			configHandler.Set("vm.disk", initDisk)
		}
		if initMemory > 0 {
			configHandler.Set("vm.memory", initMemory)
		}
		if initArch != "" {
			configHandler.Set("vm.arch", initArch)
		}
		if initDocker {
			configHandler.Set("docker.enabled", true)
		}
		if initGitLivereload {
			configHandler.Set("git.livereload.enabled", true)
		}
		if initBlueprint != "" {
			configHandler.Set("blueprint", initBlueprint)
		}
		if initToolsManager != "" {
			configHandler.Set("tools.manager", initToolsManager)
		}
		if initPlatform != "" {
			configHandler.Set("platform", initPlatform)
		}

		// Handle set flags
		for _, setFlag := range initSetFlags {
			parts := strings.SplitN(setFlag, "=", 2)
			if len(parts) == 2 {
				configHandler.Set(parts[0], parts[1])
			}
		}

		return pipeline.Execute(ctx)
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
