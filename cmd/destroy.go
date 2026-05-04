package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/tui"
	tuiplan "github.com/windsorcli/cli/pkg/tui/plan"
)

// =============================================================================
// Destroy Commands
// =============================================================================

var destroyConfirm string

var destroyCmd = &cobra.Command{
	Use:   "destroy [component]",
	Short: "Destroy infrastructure components",
	Long: `Destroy infrastructure components for Windsor environment.

With no argument, destroys all Flux kustomizations then all Terraform components.
With a component name, destroys every layer (Terraform and/or Kustomize) that contains that component.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `destroy` (no args, or with a component name that may live in either layer) can
		// dispatch to terraform or kustomize destroy paths depending on blueprint contents.
		// Layer choice is data-driven so requirements can't be statically narrowed; request
		// AllRequirements() and lean on the per-tool config gates. Skip-validation tolerates a
		// deployed-but-misordered blueprint so the operator can still tear down a broken setup.
		proj, err := prepareProjectSkipValidation(cmd, tools.AllRequirements())
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			// Auth must precede the plan: terraform plan -destroy and the live
			// inventory query both need credentials, and a credential failure
			// should surface before the operator is asked to confirm.
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			var summary *provisioner.DestroyPlanSummary
			if err := tui.WithProgress("Generating destroy plan...", func() error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyAll(blueprint)
				return planErr
			}); err != nil {
				return fmt.Errorf("error generating destroy plan: %w", err)
			}
			tuiplan.DestroySummary(os.Stdout, summary.Terraform, summary.Kustomize, os.Getenv("NO_COLOR") != "")

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all infrastructure in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			skipped, err := proj.Provisioner.Teardown(blueprint, false)
			reportSkippedDestroyComponents(cmd.ErrOrStderr(), skipped)
			if err != nil {
				return fmt.Errorf("error destroying all components: %w", err)
			}
			return nil
		}

		componentID := args[0]
		inTerraform := blueprintHasTerraformComponent(blueprint, componentID)
		inKustomize := blueprintHasKustomization(blueprint, componentID)

		if !inTerraform && !inKustomize {
			return fmt.Errorf("component %q not found in blueprint", componentID)
		}

		// Gate auth before plan (for any terraform leg) so credential failures
		// surface before the prompt rather than between plan and confirm.
		if inTerraform {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
		}

		var tfResults []terraforminfra.TerraformComponentPlan
		var k8sResults []fluxinfra.KustomizePlan
		if err := tui.WithProgress("Generating destroy plan...", func() error {
			if inTerraform {
				result, err := proj.Provisioner.PlanDestroyTerraformComponentSummary(blueprint, componentID)
				if err != nil {
					return err
				}
				tfResults = []terraforminfra.TerraformComponentPlan{result}
			}
			if inKustomize {
				result, err := proj.Provisioner.PlanDestroyKustomizeComponentSummary(blueprint, componentID)
				if err != nil {
					return err
				}
				k8sResults = []fluxinfra.KustomizePlan{result}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("error generating destroy plan: %w", err)
		}
		tuiplan.DestroySummary(os.Stdout, tfResults, k8sResults, os.Getenv("NO_COLOR") != "")

		desc := fmt.Sprintf("This will permanently destroy component %q across all layers.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}

		if inKustomize {
			if err := proj.Provisioner.DestroyKustomize(blueprint, componentID); err != nil {
				return fmt.Errorf("error destroying kustomization %s: %w", componentID, err)
			}
		}
		if inTerraform {
			skipped, err := proj.Provisioner.TeardownComponent(blueprint, componentID)
			if err != nil {
				return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
			}
			if skipped {
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), []string{componentID})
			}
		}

		return nil
	},
}

var destroyTerraformCmd = &cobra.Command{
	Use:          "terraform [project]",
	Aliases:      []string{"tf"},
	Short:        "Destroy Terraform component(s)",
	Long:         "Destroy a specific Terraform component, or all components when no argument is given.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `destroy terraform` only invokes terraform; kustomize/k8s/docker tools are not used.
		// Skip-validation tolerates a deployed-but-misordered blueprint so teardown can run
		// against a setup the validator would otherwise reject.
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Terraform: true, Secrets: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			var summary *provisioner.DestroyPlanSummary
			if err := tui.WithProgress("Generating destroy plan...", func() error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyTerraformSummary(blueprint)
				return planErr
			}); err != nil {
				return fmt.Errorf("error generating destroy plan: %w", err)
			}
			tuiplan.DestroySummary(os.Stdout, summary.Terraform, nil, os.Getenv("NO_COLOR") != "")

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Terraform components in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			skipped, err := proj.Provisioner.Teardown(blueprint, true)
			reportSkippedDestroyComponents(cmd.ErrOrStderr(), skipped)
			if err != nil {
				return fmt.Errorf("error destroying all terraform: %w", err)
			}
			return nil
		}

		componentID := args[0]
		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}
		var tfResult terraforminfra.TerraformComponentPlan
		if err := tui.WithProgress("Generating destroy plan...", func() error {
			var planErr error
			tfResult, planErr = proj.Provisioner.PlanDestroyTerraformComponentSummary(blueprint, componentID)
			return planErr
		}); err != nil {
			return fmt.Errorf("error generating destroy plan: %w", err)
		}
		tuiplan.DestroySummary(os.Stdout, []terraforminfra.TerraformComponentPlan{tfResult}, nil, os.Getenv("NO_COLOR") != "")

		desc := fmt.Sprintf("This will permanently destroy Terraform component %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		skipped, err := proj.Provisioner.TeardownComponent(blueprint, componentID)
		if err != nil {
			return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
		}
		if skipped {
			reportSkippedDestroyComponents(cmd.ErrOrStderr(), []string{componentID})
		}
		return nil
	},
}

var destroyKustomizeCmd = &cobra.Command{
	Use:          "kustomize [name]",
	Aliases:      []string{"k8s"},
	Short:        "Destroy Flux kustomization(s)",
	Long:         "Delete a specific Flux kustomization from the cluster, or all kustomizations when no argument is given.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `destroy kustomize` only talks to the cluster API; terraform/docker tools are not used.
		// Skip-validation tolerates a deployed-but-misordered blueprint.
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			var summary *provisioner.DestroyPlanSummary
			if err := tui.WithProgress("Generating destroy plan...", func() error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyKustomizeSummary(blueprint)
				return planErr
			}); err != nil {
				return fmt.Errorf("error generating destroy plan: %w", err)
			}
			tuiplan.DestroySummary(os.Stdout, nil, summary.Kustomize, os.Getenv("NO_COLOR") != "")

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Flux kustomizations in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			if err := proj.Provisioner.Uninstall(blueprint); err != nil {
				return fmt.Errorf("error destroying all kustomizations: %w", err)
			}
			return nil
		}

		componentID := args[0]
		var k8sResult fluxinfra.KustomizePlan
		if err := tui.WithProgress("Generating destroy plan...", func() error {
			var planErr error
			k8sResult, planErr = proj.Provisioner.PlanDestroyKustomizeComponentSummary(blueprint, componentID)
			return planErr
		}); err != nil {
			return fmt.Errorf("error generating destroy plan: %w", err)
		}
		tuiplan.DestroySummary(os.Stdout, nil, []fluxinfra.KustomizePlan{k8sResult}, os.Getenv("NO_COLOR") != "")

		desc := fmt.Sprintf("This will permanently destroy Flux kustomization %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		if err := proj.Provisioner.DestroyKustomize(blueprint, componentID); err != nil {
			return fmt.Errorf("error destroying kustomization %s: %w", componentID, err)
		}
		return nil
	},
}

// =============================================================================
// Private Methods
// =============================================================================

// confirmDestroy prompts the user to type confirmValue to proceed with a destructive operation.
// It prints a description of what will be destroyed and the expected confirmation token.
// Returns nil if the user types the correct value, or an error if input does not match or cannot be read.
func confirmDestroy(r io.Reader, w io.Writer, description, confirmValue string) error {
	fmt.Fprintf(w, "%s\n", description)
	fmt.Fprintf(w, "Type %q to confirm: ", confirmValue)
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return fmt.Errorf("confirmation aborted")
	}
	if strings.TrimSpace(scanner.Text()) != confirmValue {
		return fmt.Errorf("confirmation failed: input did not match %q", confirmValue)
	}
	return nil
}

// reportSkippedDestroyComponents prints a stderr warning naming any terraform
// components whose destroy was skipped because their state was empty. Empty
// state has two indistinguishable causes: the component was never applied
// (legitimate skip) or the remote state was lost while cloud resources still
// exist (silent orphan hazard — common with multi-machine config drift, manual
// state deletion, or remote storage being wiped). The destroy can't tell the
// two apart without comparing against the cloud, so the message names the
// risk and points the operator at the cloud console. No-op when skipped is
// empty.
func reportSkippedDestroyComponents(w io.Writer, skipped []string) {
	if len(skipped) == 0 {
		return
	}
	if len(skipped) == 1 {
		fmt.Fprintf(w, "warning: component %q had empty state during destroy and was skipped\n", skipped[0])
	} else {
		fmt.Fprintf(w, "warning: %d components had empty state during destroy and were skipped: %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	fmt.Fprintln(w, "   if previously bootstrapped, cloud resources may now be orphaned —")
	fmt.Fprintln(w, "   verify in your cloud console; remediate with `terraform import` if needed.")
}

// resolveDestroyConfirmation gates a destructive operation. If --confirm was supplied it must
// match expected exactly; otherwise the user is prompted interactively. This mirrors the prompt
// in both directions so scripted callers cannot accidentally destroy the wrong target.
func resolveDestroyConfirmation(r io.Reader, w io.Writer, description, expected string) error {
	if destroyConfirm != "" {
		if destroyConfirm != expected {
			return fmt.Errorf("confirmation failed: --confirm did not match %q", expected)
		}
		return nil
	}
	return confirmDestroy(r, w, description, expected)
}

// init registers destroy subcommands and the --confirm flag. --confirm must exactly match the
// context name (for layer-wide destroy) or component name (for targeted destroy); this is the
// CI-safe equivalent of the interactive prompt. There is no flag that skips confirmation entirely.
func init() {
	destroyCmd.PersistentFlags().StringVar(&destroyConfirm, "confirm", "", "Context or component name to confirm destruction (bypasses interactive prompt)")
	destroyCmd.AddCommand(destroyTerraformCmd)
	destroyCmd.AddCommand(destroyKustomizeCmd)
	rootCmd.AddCommand(destroyCmd)
}
