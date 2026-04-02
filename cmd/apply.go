package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply infrastructure changes",
	Long:  "Apply infrastructure changes for Windsor environment components.",
}

var applyTerraformCmd = &cobra.Command{
	Use:          "terraform <project>",
	Short:        "Apply Terraform changes for a specific project",
	Long:         "Apply Terraform changes for a specific project layer.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentID := args[0]

		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj := project.NewProject("", opts...)

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
		if err := proj.Provisioner.Apply(blueprint, componentID); err != nil {
			return fmt.Errorf("error applying terraform for %s: %w", componentID, err)
		}

		return nil
	},
}

func init() {
	applyCmd.AddCommand(applyTerraformCmd)
	rootCmd.AddCommand(applyCmd)
}
