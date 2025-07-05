package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var (
	initBlueprint     string
	initTerraform     bool
	initK8s           bool
	initColima        bool
	initAws           bool
	initAzure         bool
	initDockerCompose bool
	initTalos         bool
	initSetFlags      []string
)

var initCmd = &cobra.Command{
	Use:          "init [context]",
	Short:        "Initialize the application",
	Long:         "Initialize the application by setting up necessary configurations and environment",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// First, run the env pipeline in quiet mode to set up environment variables
		var envPipeline pipelines.Pipeline
		if existing := injector.Resolve("envPipeline"); existing != nil {
			envPipeline = existing.(pipelines.Pipeline)
		} else {
			envPipeline = pipelines.NewEnvPipeline()
			if err := envPipeline.Initialize(injector); err != nil {
				return fmt.Errorf("failed to initialize env pipeline: %w", err)
			}
			injector.Register("envPipeline", envPipeline)
		}

		// Execute env pipeline in quiet mode (inject environment variables without printing)
		envCtx := context.WithValue(cmd.Context(), "quiet", true)
		envCtx = context.WithValue(envCtx, "decrypt", true)
		if err := envPipeline.Execute(envCtx); err != nil {
			return fmt.Errorf("failed to set up environment: %w", err)
		}

		// Then, create and run the init pipeline
		var initPipeline pipelines.Pipeline
		if existing := injector.Resolve("initPipeline"); existing != nil {
			initPipeline = existing.(pipelines.Pipeline)
		} else {
			initPipeline = pipelines.NewInitPipeline()
			if err := initPipeline.Initialize(injector); err != nil {
				return fmt.Errorf("Error initializing: %w", err)
			}
			injector.Register("initPipeline", initPipeline)
		}

		// Create execution context with arguments and flags
		ctx := cmd.Context()
		if len(args) > 0 {
			ctx = context.WithValue(ctx, "args", args)
		}
		if verbose {
			ctx = context.WithValue(ctx, "verbose", true)
		}

		// Pass flag values through context
		flagValues := map[string]interface{}{
			"blueprint":      initBlueprint,
			"terraform":      initTerraform,
			"k8s":            initK8s,
			"colima":         initColima,
			"aws":            initAws,
			"azure":          initAzure,
			"docker-compose": initDockerCompose,
			"talos":          initTalos,
		}
		ctx = context.WithValue(ctx, "flagValues", flagValues)

		// Pass changed flags information
		changedFlags := make(map[string]bool)
		for _, flagName := range []string{
			"blueprint", "terraform", "k8s", "colima",
			"aws", "azure", "docker-compose", "talos",
		} {
			changedFlags[flagName] = cmd.Flags().Changed(flagName)
		}
		ctx = context.WithValue(ctx, "changedFlags", changedFlags)

		// Pass set flags
		if len(initSetFlags) > 0 {
			ctx = context.WithValue(ctx, "setFlags", initSetFlags)
		}

		// Execute the init pipeline
		if err := initPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("Error executing init pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initBlueprint, "blueprint", "windsorcli/core", "Specify the blueprint to use")
	initCmd.Flags().BoolVar(&initTerraform, "terraform", true, "Enable Terraform")
	initCmd.Flags().BoolVar(&initK8s, "k8s", true, "Enable Kubernetes")
	initCmd.Flags().BoolVar(&initColima, "colima", false, "Use Colima as VM driver")
	initCmd.Flags().BoolVar(&initAws, "aws", false, "Enable AWS platform")
	initCmd.Flags().BoolVar(&initAzure, "azure", false, "Enable Azure platform")
	initCmd.Flags().BoolVar(&initDockerCompose, "docker-compose", true, "Enable Docker Compose")
	initCmd.Flags().BoolVar(&initTalos, "talos", false, "Enable Talos")
	initCmd.Flags().StringSliceVar(&initSetFlags, "set", []string{}, "Override configuration values. Example: --set dns.enabled=false --set cluster.endpoint=https://localhost:6443")
	rootCmd.AddCommand(initCmd)
}
