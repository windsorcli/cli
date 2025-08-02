package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

var buildIdNewFlag bool

var buildIdCmd = &cobra.Command{
	Use:   "build-id",
	Short: "Manage build IDs for artifact tagging",
	Long: `Manage build IDs for artifact tagging in local development environments.

The build-id command provides functionality to retrieve and generate new build IDs
that are used for tagging Docker images and other artifacts during development.
Build IDs are stored persistently in the .windsor/.build-id file and are available
as the BUILD_ID environment variable and postBuild variable in Kustomizations.

Examples:
  windsor build-id                    # Output current build ID
  windsor build-id --new              # Generate and output new build ID
  BUILD_ID=$(windsor build-id --new) && docker build -t myapp:$BUILD_ID .`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Set up the build ID pipeline
		buildIDPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "buildIDPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up build ID pipeline: %w", err)
		}

		// Create execution context with flags
		ctx := cmd.Context()
		if buildIdNewFlag {
			ctx = context.WithValue(ctx, "new", true)
		}

		// Execute the build ID pipeline
		if err := buildIDPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("failed to execute build ID pipeline: %w", err)
		}

		return nil
	},
}

func init() {
	buildIdCmd.Flags().BoolVar(&buildIdNewFlag, "new", false, "Generate a new build ID")
	rootCmd.AddCommand(buildIdCmd)
}
