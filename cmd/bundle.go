package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

// bundleCmd represents the bundle command
var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Bundle blueprints into distributable artifacts",
	Long: `Bundle blueprints into distributable artifacts for deployment.

This command packages your Windsor blueprints into compressed archives that can be
distributed and deployed to target environments. The bundling process includes:

- Template bundling: Packages Jsonnet templates from contexts/_template/
- Kustomize bundling: Packages Kustomize configurations
- Metadata generation: Creates deployment metadata with build information
- Archive creation: Compresses everything into .tar.gz format

The resulting artifacts are compatible with FluxCD OCI registries and other
deployment systems that support compressed archives.

Tag format is required as "name:version". If metadata.yaml
exists, tag values override metadata values. Tag is required if no metadata.yaml
exists or if metadata.yaml lacks name/version fields.`,
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
