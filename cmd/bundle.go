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
	Short: "Bundle the blueprint into a .tar.gz archive.",
	Long: `Bundle the current blueprint into a .tar.gz archive for sharing or offline deployment.

Uses metadata.yaml ('name', 'version') to derive the output filename when --tag is not given. If neither is set, --tag is required.

When --output is a directory, the filename is derived from the tag.`,
	Example: `# Bundle using metadata.yaml for name/version, into the current directory
windsor bundle

# Explicit tag, auto-generated filename
windsor bundle -t myapp:v1.0.0

# Explicit tag, explicit output path
windsor bundle -t myapp:v1.0.0 -o ./dist/myapp-v1.0.0.tar.gz

# Tag set, output is a directory (filename auto-generated)
windsor bundle -t myapp:v1.0.0 -o ./dist/`,
	Annotations: map[string]string{
		"docs.seealso": "[Metadata reference](../metadata.md)\n" +
			"[`push`](push.md)",
		"docs.source": "cmd/bundle.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			rtOpts = []*runtime.Runtime{overridesVal.(*runtime.Runtime)}
		}

		rt := runtime.NewRuntime(rtOpts...)

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
	bundleCmd.Flags().StringP("output", "o", ".", "Output path for the archive. May be a file or a directory.")
	bundleCmd.Flags().StringP("tag", "t", "", "Tag in 'name:version' form. Required when metadata.yaml lacks name or version.")
}
