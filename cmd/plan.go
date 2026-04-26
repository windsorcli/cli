package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/tui"
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
			var summary *provisioner.PlanSummary
			if err := tui.WithProgress("Generating plan...", func() error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanAll(blueprint)
				return planErr
			}); err != nil {
				return fmt.Errorf("error running plan: %w", err)
			}
			if planJSON {
				return printPlanSummaryJSON(os.Stdout, summary.Terraform, summary.Kustomize)
			}
			printPlanSummary(os.Stdout, summary.Terraform, summary.Kustomize, summary.Hints, planNoColor || os.Getenv("NO_COLOR") != "")
			return nil
		}

		componentID := args[0]
		inTerraform := blueprintHasTerraformComponent(blueprint, componentID)
		inKustomize := blueprintHasKustomization(blueprint, componentID)

		if !inTerraform && !inKustomize {
			return fmt.Errorf("component %q not found in blueprint", componentID)
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
				return printPlanSummaryJSON(os.Stdout, tfResults, k8sResults)
			}
			printPlanSummary(os.Stdout, tfResults, k8sResults, nil, planNoColor || os.Getenv("NO_COLOR") != "")
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
					return printPlanSummaryJSON(os.Stdout, summary.Terraform, nil)
				}
				printPlanSummary(os.Stdout, summary.Terraform, nil, nil, planNoColor || os.Getenv("NO_COLOR") != "")
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
				return printPlanSummaryJSON(os.Stdout, []terraforminfra.TerraformComponentPlan{result}, nil)
			}
			printPlanSummary(os.Stdout, []terraforminfra.TerraformComponentPlan{result}, nil, nil, planNoColor || os.Getenv("NO_COLOR") != "")
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
					return printPlanSummaryJSON(os.Stdout, nil, summary.Kustomize)
				}
				printPlanSummary(os.Stdout, nil, summary.Kustomize, summary.Hints, planNoColor || os.Getenv("NO_COLOR") != "")
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
				return printPlanSummaryJSON(os.Stdout, nil, []fluxinfra.KustomizePlan{result})
			}
			printPlanSummary(os.Stdout, nil, []fluxinfra.KustomizePlan{result}, nil, planNoColor || os.Getenv("NO_COLOR") != "")
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

// printPlanSummary writes the combined Terraform and Kustomize plan summary to w.
// Component names are left-aligned in a column wide enough to fit the longest name.
// Each row shows add/change/destroy counts for Terraform or added/removed for Kustomize,
// with "(no changes)" when all counts are zero and "(error: ...)" when the plan failed.
// Any upgrade hints are printed in a footnote block at the bottom when present.
func printPlanSummary(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan, hints []string, noColor bool) {
	nameWidth := 20
	for _, p := range tfPlans {
		if len(p.ComponentID) > nameWidth {
			nameWidth = len(p.ComponentID)
		}
	}
	for _, p := range k8sPlans {
		if len(p.Name) > nameWidth {
			nameWidth = len(p.Name)
		}
	}
	nameWidth += 2

	sep := strings.Repeat("═", nameWidth+26)
	fmt.Fprintf(w, "\nWindsor Plan Summary\n%s\n", sep)

	if len(tfPlans) > 0 {
		fmt.Fprintln(w, "\nTerraform")
		for _, p := range tfPlans {
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, p.ComponentID, formatTerraformPlan(p, noColor))
			if p.Err != nil {
				lines := strings.Split(strings.TrimSpace(p.Err.Error()), "\n")
				for _, line := range lines[1:] {
					fmt.Fprintf(w, "  %s  %s\n", strings.Repeat(" ", nameWidth), line)
				}
			}
		}
	}

	if len(k8sPlans) > 0 {
		fmt.Fprintln(w, "\nKustomize")
		for _, p := range k8sPlans {
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, p.Name, formatKustomizePlan(p, noColor))
			if p.Err != nil {
				lines := strings.Split(strings.TrimSpace(p.Err.Error()), "\n")
				for _, line := range lines[1:] {
					fmt.Fprintf(w, "  %s  %s\n", strings.Repeat(" ", nameWidth), line)
				}
			}
		}
	}

	if len(tfPlans) == 0 && len(k8sPlans) == 0 {
		fmt.Fprintln(w, "\n  (no components in blueprint)")
	}

	if len(hints) > 0 {
		hintSep := strings.Repeat("─", nameWidth+26)
		fmt.Fprintf(w, "\n%s\n", hintSep)
		for _, h := range hints {
			for _, line := range strings.Split(h, "\n") {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
	}

	fmt.Fprintln(w)
}

// printPlanSummaryJSON encodes the plan results as JSON to w.
func printPlanSummaryJSON(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan) error {
	type tfRow struct {
		Component string `json:"component"`
		Add       int    `json:"add"`
		Change    int    `json:"change"`
		Destroy   int    `json:"destroy"`
		NoChanges bool   `json:"no_changes"`
		Error     string `json:"error,omitempty"`
	}
	type k8sRow struct {
		Name     string `json:"name"`
		Added    int    `json:"added"`
		Removed  int    `json:"removed"`
		IsNew    bool   `json:"is_new"`
		Degraded bool   `json:"degraded"`
		Error    string `json:"error,omitempty"`
	}
	type output struct {
		Terraform []tfRow  `json:"terraform,omitempty"`
		Kustomize []k8sRow `json:"kustomize,omitempty"`
	}

	out := output{}
	for _, p := range tfPlans {
		row := tfRow{Component: p.ComponentID, Add: p.Add, Change: p.Change, Destroy: p.Destroy, NoChanges: p.NoChanges}
		if p.Err != nil {
			row.Error = p.Err.Error()
		}
		out.Terraform = append(out.Terraform, row)
	}
	for _, p := range k8sPlans {
		row := k8sRow{Name: p.Name, Added: p.Added, Removed: p.Removed, IsNew: p.IsNew, Degraded: p.Degraded}
		if p.Err != nil {
			row.Error = p.Err.Error()
		}
		out.Kustomize = append(out.Kustomize, row)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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

// truncateFirstLine returns the first line of s, stripping any trailing content after \n or \r\n.
func truncateFirstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx != -1 {
		return strings.TrimRight(s[:idx], "\r")
	}
	return s
}

// formatTerraformPlan returns a concise human-readable status string for one Terraform component.
func formatTerraformPlan(p terraforminfra.TerraformComponentPlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.NoChanges || (p.Add == 0 && p.Change == 0 && p.Destroy == 0) {
		return "(no changes)"
	}
	if noColor {
		return fmt.Sprintf("+%d  ~%d  -%d", p.Add, p.Change, p.Destroy)
	}
	return fmt.Sprintf("\033[32m+%d\033[0m  \033[33m~%d\033[0m  \033[31m-%d\033[0m", p.Add, p.Change, p.Destroy)
}

// formatKustomizePlan returns a concise human-readable status string for one Kustomize component.
func formatKustomizePlan(p fluxinfra.KustomizePlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.Degraded {
		if p.IsNew {
			return "(new)"
		}
		return "(existing)"
	}
	if p.IsNew {
		if p.Added == 0 {
			return "(new — empty)"
		}
		if noColor {
			return fmt.Sprintf("+%d resources  (new)", p.Added)
		}
		return fmt.Sprintf("\033[32m+%d resources\033[0m  (new)", p.Added)
	}
	if p.Added == 0 && p.Removed == 0 {
		return "(no changes)"
	}
	if noColor {
		return fmt.Sprintf("+%d  -%d  lines", p.Added, p.Removed)
	}
	return fmt.Sprintf("\033[32m+%d\033[0m  \033[31m-%d\033[0m  lines", p.Added, p.Removed)
}
