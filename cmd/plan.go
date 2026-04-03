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

var planKustomizeCmd = &cobra.Command{
	Use:          "kustomize <component|all>",
	Aliases:      []string{"k8s"},
	Short:        "Plan Flux kustomization changes",
	Long:         "Show a diff of pending Flux kustomization changes without applying them. Use 'all' to plan every kustomization in the blueprint.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentID := args[0]

		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if err := proj.Provisioner.PlanKustomization(blueprint, componentID); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	planCmd.AddCommand(planTerraformCmd)
	planCmd.AddCommand(planKustomizeCmd)
	rootCmd.AddCommand(planCmd)
}
