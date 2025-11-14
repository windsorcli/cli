package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/di"
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
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		execCtx := &runtime.Runtime{
			Injector: injector,
		}

		execCtx, err := runtime.NewRuntime(execCtx)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		var buildID string
		if buildIdNewFlag {
			buildID, err = execCtx.GenerateBuildID()
		} else {
			buildID, err = execCtx.GetBuildID()
		}
		if err != nil {
			return fmt.Errorf("failed to manage build ID: %w", err)
		}

		fmt.Printf("%s\n", buildID)
		return nil
	},
}

func init() {
	buildIdCmd.Flags().BoolVar(&buildIdNewFlag, "new", false, "Generate a new build ID")
	rootCmd.AddCommand(buildIdCmd)
}
