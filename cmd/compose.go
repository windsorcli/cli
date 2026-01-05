package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var composeBlueprintJSON bool

var composeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Compose and output resources to stdout",
	Long:  "Compose and output resources to stdout",
}

var composeBlueprintCmd = &cobra.Command{
	Use:          "blueprint",
	Short:        "Output the fully compiled blueprint",
	Long:         "Output the fully compiled blueprint to stdout before cleaning/removing transient fields. Defaults to YAML output, use --json for JSON output.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj, err := project.NewProject("", opts...)
		if err != nil {
			return err
		}

		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := proj.Configure(nil); err != nil {
			return err
		}

		if err := proj.Initialize(false); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("failed to generate blueprint")
		}

		var output []byte
		if composeBlueprintJSON {
			output, err = json.MarshalIndent(blueprint, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal blueprint to JSON: %w", err)
			}
		} else {
			output, err = yaml.Marshal(blueprint)
			if err != nil {
				return fmt.Errorf("failed to marshal blueprint to YAML: %w", err)
			}
		}

		fmt.Print(string(output))
		return nil
	},
}

func init() {
	composeBlueprintCmd.Flags().BoolVar(&composeBlueprintJSON, "json", false, "Output blueprint as JSON instead of YAML")
	composeCmd.AddCommand(composeBlueprintCmd)
	rootCmd.AddCommand(composeCmd)
}
