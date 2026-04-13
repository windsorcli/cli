package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

var applyWaitFlag bool // Wait for kustomization resources to be ready after applying

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
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
		}
		waitBlueprint := blueprint

		if len(args) == 0 {
			if err := proj.Provisioner.ApplyKustomizeAll(blueprint); err != nil {
				return fmt.Errorf("error applying kustomize: %w", err)
			}
		} else {
			componentID := args[0]
			if err := proj.Provisioner.ApplyKustomize(blueprint, componentID); err != nil {
				return fmt.Errorf("error applying kustomize for %s: %w", componentID, err)
			}
			// Narrow the wait scope to only the kustomization that was applied.
			for _, k := range blueprint.Kustomizations {
				if k.Name == componentID {
					kCopy := k
					filtered := *blueprint
					filtered.Kustomizations = []blueprintv1alpha1.Kustomization{kCopy}
					waitBlueprint = &filtered
					break
				}
			}
		}

		if applyWaitFlag {
			if err := proj.Provisioner.Wait(waitBlueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}
		}

		return nil
	},
}

func init() {
	applyKustomizeCmd.Flags().BoolVar(&applyWaitFlag, "wait", false, "Wait for kustomization resources to be ready")
	applyCmd.AddCommand(applyTerraformCmd)
	applyCmd.AddCommand(applyKustomizeCmd)
	rootCmd.AddCommand(applyCmd)
}
