package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/tui"
	tuiplan "github.com/windsorcli/cli/pkg/tui/plan"
)

var planNoColor bool
var planSummary bool
var planJSON bool

var planCmd = &cobra.Command{
	Use:          "plan [component]",
	Short:        "Plan infrastructure changes",
	Long:         "Plan infrastructure changes for Windsor environment components.\n\nWith no argument, shows a summary plan across all Terraform components and Flux\nkustomizations. With a component name, runs a full streaming plan for every\nlayer (Terraform and/or Kustomize) that contains that component.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `plan` (no args, or with a component name) can dispatch to terraform plan and/or
		// flux diff depending on what the blueprint contains. Both are read-only but exercise
		// terraform + cluster API + secrets, so request the full read-side surface.
		proj, err := prepareProject(cmd, tools.Requirements{Terraform: true, Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			var summary *provisioner.PlanSummary
			if err := tui.WithProgress("Generating plan...", func() error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanAll(blueprint)
				return planErr
			}); err != nil {
				return fmt.Errorf("error running plan: %w", err)
			}
			if planJSON {
				return tuiplan.SummaryJSON(os.Stdout, summary.Terraform, summary.Kustomize)
			}
			tuiplan.Summary(os.Stdout, summary.Terraform, summary.Kustomize, summary.Hints, planNoColor || os.Getenv("NO_COLOR") != "")
			return nil
		}

		componentID := args[0]
		inTerraform := blueprintHasTerraformComponent(blueprint, componentID)
		inKustomize := blueprintHasKustomization(blueprint, componentID)

		if !inTerraform && !inKustomize {
			return fmt.Errorf("component %q not found in blueprint", componentID)
		}

		if inTerraform {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
		}

		if planSummary || planJSON {
			var tfResults []terraforminfra.TerraformComponentPlan
			var k8sResults []fluxinfra.KustomizePlan
			if inTerraform {
				result, err := proj.Provisioner.PlanTerraformComponentSummary(blueprint, componentID)
				if err != nil {
					return fmt.Errorf("error running plan: %w", err)
				}
				tfResults = []terraforminfra.TerraformComponentPlan{result}
			}
			if inKustomize {
				result, err := proj.Provisioner.PlanKustomizeComponentSummary(blueprint, componentID)
				if err != nil {
					return fmt.Errorf("error running plan: %w", err)
				}
				k8sResults = []fluxinfra.KustomizePlan{result}
			}
			if planJSON {
				return tuiplan.SummaryJSON(os.Stdout, tfResults, k8sResults)
			}
			tuiplan.Summary(os.Stdout, tfResults, k8sResults, nil, planNoColor || os.Getenv("NO_COLOR") != "")
			return nil
		}

		if inTerraform {
			fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Terraform: "+componentID))
			if err := proj.Provisioner.Plan(blueprint, componentID); err != nil {
				return fmt.Errorf("error planning terraform for %s: %w", componentID, err)
			}
		}
		if inKustomize {
			if err := proj.Provisioner.PlanKustomization(blueprint, componentID); err != nil {
				return err
			}
		}
		return nil
	},
}

var planTerraformCmd = &cobra.Command{
	Use:          "terraform [project]",
	Aliases:      []string{"tf"},
	Short:        "Plan Terraform changes",
	Long:         "Stream terraform init and plan for a specific component, or all components when no argument is given. Use --summary for a compact table or --json for machine-readable counts.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `plan terraform` only invokes terraform; cluster/k8s tools are not used.
		proj, err := prepareProject(cmd, tools.Requirements{Terraform: true, Secrets: true})
		if err != nil {
			return err
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if planJSON && !planSummary {
				return proj.Provisioner.PlanTerraformAllJSON(blueprint)
			}
			if planSummary {
				summary, err := proj.Provisioner.PlanTerraformSummary(blueprint)
				if err != nil {
					return fmt.Errorf("error running plan: %w", err)
				}
				if planJSON {
					return tuiplan.SummaryJSON(os.Stdout, summary.Terraform, nil)
				}
				tuiplan.Summary(os.Stdout, summary.Terraform, nil, nil, planNoColor || os.Getenv("NO_COLOR") != "")
				return nil
			}
			return proj.Provisioner.PlanTerraformAll(blueprint)
		}

		componentID := args[0]

		if planJSON && !planSummary {
			if err := proj.Provisioner.PlanTerraformJSON(blueprint, componentID); err != nil {
				return fmt.Errorf("error planning terraform for %s: %w", componentID, err)
			}
			return nil
		}
		if planSummary {
			result, err := proj.Provisioner.PlanTerraformComponentSummary(blueprint, componentID)
			if err != nil {
				return fmt.Errorf("error running plan: %w", err)
			}
			if planJSON {
				return tuiplan.SummaryJSON(os.Stdout, []terraforminfra.TerraformComponentPlan{result}, nil)
			}
			tuiplan.Summary(os.Stdout, []terraforminfra.TerraformComponentPlan{result}, nil, nil, planNoColor || os.Getenv("NO_COLOR") != "")
			return nil
		}

		fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Terraform: "+componentID))
		if err := proj.Provisioner.Plan(blueprint, componentID); err != nil {
			return fmt.Errorf("error planning terraform for %s: %w", componentID, err)
		}
		return nil
	},
}

var planKustomizeCmd = &cobra.Command{
	Use:          "kustomize [component]",
	Aliases:      []string{"k8s"},
	Short:        "Plan Flux kustomization changes",
	Long:         "Stream flux diff for a specific kustomization, or all kustomizations when no argument is given. Use --summary for a compact table or --json for machine-readable counts.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `plan kustomize` runs flux diff against the cluster; terraform/docker are not used.
		proj, err := prepareProject(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if planJSON && !planSummary {
				return proj.Provisioner.PlanKustomizeAllJSON(blueprint)
			}
			if planSummary {
				summary, err := proj.Provisioner.PlanKustomizeSummary(blueprint)
				if err != nil {
					return fmt.Errorf("error running plan: %w", err)
				}
				if planJSON {
					return tuiplan.SummaryJSON(os.Stdout, nil, summary.Kustomize)
				}
				tuiplan.Summary(os.Stdout, nil, summary.Kustomize, summary.Hints, planNoColor || os.Getenv("NO_COLOR") != "")
				return nil
			}
			return proj.Provisioner.PlanKustomizeAll(blueprint)
		}

		componentID := args[0]

		if planJSON && !planSummary {
			return proj.Provisioner.PlanKustomizeJSON(blueprint, componentID)
		}
		if planSummary {
			result, err := proj.Provisioner.PlanKustomizeComponentSummary(blueprint, componentID)
			if err != nil {
				return fmt.Errorf("error running plan: %w", err)
			}
			if planJSON {
				return tuiplan.SummaryJSON(os.Stdout, nil, []fluxinfra.KustomizePlan{result})
			}
			tuiplan.Summary(os.Stdout, nil, []fluxinfra.KustomizePlan{result}, nil, planNoColor || os.Getenv("NO_COLOR") != "")
			return nil
		}

		if err := proj.Provisioner.PlanKustomization(blueprint, componentID); err != nil {
			return err
		}
		return nil
	},
}

// init registers plan subcommands and persistent flags on the plan command group.
func init() {
	planCmd.PersistentFlags().BoolVar(&planNoColor, "no-color", false, "Disable color output")
	planCmd.PersistentFlags().BoolVar(&planSummary, "summary", false, "Show a compact summary table instead of streaming output")
	planCmd.PersistentFlags().BoolVar(&planJSON, "json", false, "Output as JSON; on subcommands streams full plan JSON, on root 'plan' outputs summary as JSON")
	planCmd.AddCommand(planTerraformCmd)
	planCmd.AddCommand(planKustomizeCmd)
	rootCmd.AddCommand(planCmd)
}

// blueprintHasTerraformComponent reports whether the blueprint contains an enabled Terraform component with the given ID.
// Components with Enabled explicitly set to false are excluded, matching the filtering applied by TerraformStack.
func blueprintHasTerraformComponent(blueprint *blueprintv1alpha1.Blueprint, componentID string) bool {
	if blueprint == nil {
		return false
	}
	for i := range blueprint.TerraformComponents {
		c := blueprint.TerraformComponents[i]
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		if c.GetID() == componentID {
			return true
		}
	}
	return false
}

// blueprintHasKustomization reports whether the blueprint contains a Kustomization with the given name.
func blueprintHasKustomization(blueprint *blueprintv1alpha1.Blueprint, componentID string) bool {
	if blueprint == nil {
		return false
	}
	for _, k := range blueprint.Kustomizations {
		if k.Name == componentID {
			return true
		}
	}
	return false
}
