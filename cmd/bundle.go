package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// bundleCmd represents the bundle command
var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Bundle blueprints into a .tar.gz archive",
	Long: `Bundle your Windsor blueprints into a compressed archive for distribution.

This command packages your blueprints into a .tar.gz file that can be shared,
stored, or deployed to target environments.

Examples:
  # Bundle with automatic naming
  windsor bundle -t myapp:v1.0.0

  # Bundle to specific file
  windsor bundle -t myapp:v1.0.0 -o myapp-v1.0.0.tar.gz

  # Bundle to directory (filename auto-generated)
  windsor bundle -t myapp:v1.0.0 -o ./dist/

  # Bundle using metadata.yaml for name/version
  windsor bundle`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get shared dependency injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Get tag and output path from flags
		tag, _ := cmd.Flags().GetString("tag")
		outputPath, _ := cmd.Flags().GetString("output")

		// Set up the artifact pipeline
		artifactPipeline, err := pipelines.WithPipeline(injector, cmd.Context(), "artifactPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up artifact pipeline: %w", err)
		}

		// Create execution context with bundle mode and parameters
		ctx := context.WithValue(cmd.Context(), "artifactMode", "bundle")
		ctx = context.WithValue(ctx, "outputPath", outputPath)
		ctx = context.WithValue(ctx, "tag", tag)

		// Execute the artifact pipeline in bundle mode
		if err := artifactPipeline.Execute(ctx); err != nil {
			return fmt.Errorf("failed to bundle artifacts: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("output", "o", ".", "Output path for bundle archive (file or directory)")
	bundleCmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format (required if no metadata.yaml or missing name/version)")
}
