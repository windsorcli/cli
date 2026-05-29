package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
)

var buildIdNewFlag bool

var buildIdCmd = &cobra.Command{
	Use:   "build-id",
	Short: "Print or generate a build ID.",
	Long: `Print the current build ID, or generate a new one with --new. Build IDs are stored in .windsor/.build-id and are available to your tools as:

  - the BUILD_ID environment variable
  - a postBuild substitution in Flux kustomizations

The ID starts with the current date (YYMMDD) and includes a same-day counter so artifacts tagged on the same day stay sortable.`,
	Example: `# Read the current build ID
windsor build-id

# Generate a new build ID and tag an image with it
BUILD_ID=$(windsor build-id --new)
docker build -t myapp:$BUILD_ID .`,
	Annotations: map[string]string{
		"docs.seealso": "[`env`](env.md)",
		"docs.source": "cmd/build_id.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		var buildID string
		var err error
		if buildIdNewFlag {
			buildID, err = rt.GenerateBuildID()
		} else {
			buildID, err = rt.GetBuildID()
		}
		if err != nil {
			return fmt.Errorf("failed to manage build ID: %w", err)
		}

		fmt.Printf("%s\n", buildID)
		return nil
	},
}

func init() {
	buildIdCmd.Flags().BoolVar(&buildIdNewFlag, "new", false, "Generate and print a new build ID.")
	rootCmd.AddCommand(buildIdCmd)
}
