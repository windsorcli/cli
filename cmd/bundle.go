package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/runtime"
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
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt, err := runtime.NewRuntime(rtOpts...)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		var opts []*composer.Composer
		if overridesVal := cmd.Context().Value(composerOverridesKey); overridesVal != nil {
			opts = []*composer.Composer{overridesVal.(*composer.Composer)}
		}

		comp := composer.NewComposer(rt, opts...)

		tag, _ := cmd.Flags().GetString("tag")
		outputPath, _ := cmd.Flags().GetString("output")

		actualOutputPath, err := comp.Bundle(outputPath, tag)
		if err != nil {
			return fmt.Errorf("failed to bundle artifacts: %w", err)
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
