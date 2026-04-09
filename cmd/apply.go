package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply infrastructure changes",
	Long:  "Apply infrastructure changes for Windsor environment components.",
}

var applyTerraformCmd = &cobra.Command{
	Use:          "terraform <project>",
	Aliases:      []string{"tf"},
	Short:        "Apply Terraform changes for a specific project",
	Long:         "Apply Terraform changes for a specific project layer.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentID := args[0]

		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if err := proj.Provisioner.Apply(blueprint, componentID); err != nil {
			return fmt.Errorf("error applying terraform for %s: %w", componentID, err)
		}

		return nil
	},
}

var applyKustomizeCmd = &cobra.Command{
	Use:          "kustomize [name]",
	Short:        "Apply Flux kustomization(s) to the cluster",
	Long:         "Apply a single Flux kustomization to the cluster by name, or all kustomizations when no argument is given.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if err := proj.Provisioner.ApplyKustomizeAll(blueprint); err != nil {
				return fmt.Errorf("error applying kustomize: %w", err)
			}
			return nil
		}

		componentID := args[0]
		if err := proj.Provisioner.ApplyKustomize(blueprint, componentID); err != nil {
			return fmt.Errorf("error applying kustomize for %s: %w", componentID, err)
		}

		return nil
	},
}

func init() {
	applyCmd.AddCommand(applyTerraformCmd)
	applyCmd.AddCommand(applyKustomizeCmd)
	rootCmd.AddCommand(applyCmd)
}
