package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements including bundler functionality
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			CommandName: cmd.Name(),
			Bundler:     true,
		}); err != nil {
			return fmt.Errorf("failed to initialize controller: %w", err)
		}

		// Resolve artifact builder from controller
		artifact := controller.ResolveArtifactBuilder()
		if artifact == nil {
			return fmt.Errorf("artifact builder not available")
		}

		// Resolve all bundlers and run them
		bundlers := controller.ResolveAllBundlers()
		for _, bundler := range bundlers {
			if err := bundler.Bundle(artifact); err != nil {
				return fmt.Errorf("bundling failed: %w", err)
			}
		}

		// Get tag and output path from flags
		tag, _ := cmd.Flags().GetString("tag")
		outputPath, _ := cmd.Flags().GetString("output")

		// Create the final artifact
		actualOutputPath, err := artifact.Create(outputPath, tag)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		fmt.Printf("Blueprint bundled successfully: %s\n", actualOutputPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("output", "o", ".", "Output path for bundle archive (file or directory)")
	bundleCmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' format (required if no metadata.yaml or missing name/version)")
}
