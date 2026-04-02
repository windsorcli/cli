package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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

		proj, err := prepareProject(cmd)
		if err != nil {
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
