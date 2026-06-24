package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	composerblueprint "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/tui"
	tuiplan "github.com/windsorcli/cli/pkg/tui/plan"
)

var planNoColor bool
var planSummary bool
var planJSON bool

var planCmd = &cobra.Command{
	Use:   "plan [component]",
	Short: "Preview terraform and Flux changes.",
	Long: `Preview pending changes across Terraform components and Flux kustomizations without applying them.

With no argument, prints a compact summary across all components. Components that have never been applied show as '(new)' so you can distinguish first-time creates from updates.

With a component name, runs a full streaming plan for every layer (Terraform and/or Kustomize) that contains that component. Use a subcommand to restrict to a single layer.

The --summary, --json, and --no-color flags are persistent and apply to all subcommands.`,
	Example: `# Compact summary across both layers
windsor plan

# Full streaming plan for one component (both layers)
windsor plan cluster

# JSON-formatted summary, suitable for CI parsing
windsor plan --summary --json

# Just terraform, just one component
windsor plan terraform cluster`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`show`](show.md), [`explain`](explain.md)",
		"docs.source": "cmd/plan.go",
	},
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
			return stacklock.With(cmd.Context(), proj.Runtime, "plan", func() error {
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
				describePlanMode(cmd, proj, blueprint)
				describePendingPrunes(cmd, proj, blueprint)
				tuiplan.Summary(os.Stdout, summary.Terraform, summary.Kustomize, summary.Hints, planNoColor || os.Getenv("NO_COLOR") != "")
				return nil
			})
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

		// Lock only when terraform is involved; pure kustomize plans are read-only flux diffs
		// against the cluster and must not block infra-mutating windsor operations.
		runPlan := func(fn func() error) error {
			if inTerraform {
				return stacklock.With(cmd.Context(), proj.Runtime, "plan", fn)
			}
			return fn()
		}

		if planSummary || planJSON {
			return runPlan(func() error {
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
			})
		}

		return runPlan(func() error {
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
		})
	},
}

var planTerraformCmd = &cobra.Command{
	Use:     "terraform [component]",
	Aliases: []string{"tf"},
	Short:   "Plan Terraform changes.",
	Long: `Stream 'terraform init' and 'terraform plan' for a specific component, or all components when no argument is given. Inherits --summary, --json, and --no-color from the parent 'plan' command.`,
	Example: `# Stream the plan for one component
windsor plan terraform cluster

# Compact summary across all components
windsor plan terraform --summary

# Machine-readable JSON of all component plans
windsor plan terraform --json`,
	Annotations: map[string]string{
		"docs.seealso": "[`plan`](plan.md), [`apply terraform`](apply-terraform.md), [`destroy terraform`](destroy-terraform.md)",
		"docs.source": "cmd/plan.go",
	},
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

		return stacklock.With(cmd.Context(), proj.Runtime, "plan", func() error {
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
		})
	},
}

var planKustomizeCmd = &cobra.Command{
	Use:     "kustomize [component]",
	Aliases: []string{"k8s"},
	Short:   "Plan Flux kustomization changes.",
	Long: `Stream 'flux diff' for a specific kustomization, or all kustomizations when no argument is given. Inherits --summary, --json, and --no-color from the parent 'plan' command.`,
	Example: `# Stream the diff for one kustomization
windsor plan kustomize dns

# Compact summary across all kustomizations
windsor plan kustomize --summary`,
	Annotations: map[string]string{
		"docs.seealso": "[`plan`](plan.md), [`apply kustomize`](apply-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)",
		"docs.source": "cmd/plan.go",
	},
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
	planCmd.PersistentFlags().BoolVar(&planNoColor, "no-color", false, "Disable color output.")
	planCmd.PersistentFlags().BoolVar(&planSummary, "summary", false, "Show a compact summary table instead of streaming output.")
	planCmd.PersistentFlags().BoolVar(&planJSON, "json", false, "Output as JSON. Streams full plan JSON on subcommands; emits the summary as JSON on root 'plan'.")
	planCmd.AddCommand(planTerraformCmd)
	planCmd.AddCommand(planKustomizeCmd)
	rootCmd.AddCommand(planCmd)
}

// describePlanMode prints to stderr whether `plan` is previewing in-version content drift or a
// pending version transition, so the operator knows the path forward is apply or upgrade. It is
// best-effort: a legacy or unreadable marker prints nothing and lets the content plan speak for
// itself. The transition's content delta is the plan shown below; this only labels which mode the
// operator is in.
func describePlanMode(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint) {
	gate, err := proj.Provisioner.CheckVersionGate(blueprint)
	if err != nil || !gate.MarkerFound {
		return
	}
	switch {
	case gate.InFlight:
		fmt.Fprintln(cmd.ErrOrStderr(), "An upgrade is in progress for this context. Run `windsor upgrade` to resume it; the plan below shows the current content delta.")
	case !gate.VersionMatch:
		fmt.Fprintln(cmd.ErrOrStderr(), "The blueprint targets a different version than what is applied. The plan below shows the content delta; run `windsor upgrade` to transition versions.")
	}
}

// describePendingPrunes prints to stderr the kustomizations a `windsor upgrade` would prune — those
// this context still has live but the blueprint no longer declares — so a pending removal is visible
// before it happens. It is best-effort: a legacy/unreachable cluster or no orphans prints nothing.
func describePendingPrunes(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint) {
	prunable, err := proj.Provisioner.PrunableKustomizations(blueprint)
	if err != nil || len(prunable) == 0 {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "These kustomizations are no longer declared and would be pruned by `windsor apply --yes` or `windsor upgrade --yes`:\n  %s\n", strings.Join(prunable, "\n  "))
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

// blueprintHasKustomization reports whether the blueprint contains a Kustomization with the given
// name, including the synthesized CRD layers (crds / crds-<source>) the provisioner materializes
// from crds: at plan time — so those layers are valid `windsor plan <name>` targets.
func blueprintHasKustomization(blueprint *blueprintv1alpha1.Blueprint, componentID string) bool {
	if blueprint == nil {
		return false
	}
	for _, k := range blueprint.Kustomizations {
		if k.Name == componentID {
			return true
		}
	}
	for _, layer := range composerblueprint.CrdLayers(blueprint) {
		if blueprintv1alpha1.CrdKustomizationName(layer.Source) == componentID {
			return true
		}
	}
	return false
}
