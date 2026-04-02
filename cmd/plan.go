package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan infrastructure changes",
	Long:  "Plan infrastructure changes for Windsor environment components.",
}

var planTerraformCmd = &cobra.Command{
	Use:          "terraform <project>",
	Short:        "Plan Terraform changes for a specific project",
	Long:         "Plan Terraform changes for a specific project layer without applying them.",
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
		if err := proj.Provisioner.Plan(blueprint, componentID); err != nil {
			return fmt.Errorf("error planning terraform for %s: %w", componentID, err)
		}

		return nil
	},
}

func init() {
	planCmd.AddCommand(planTerraformCmd)
	rootCmd.AddCommand(planCmd)
}
