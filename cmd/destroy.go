// The destroy command destroys live infrastructure after the user confirms.
// It provides the destroy terraform and destroy kustomize subcommands, and the plan checks that run first.

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	tuiplan "github.com/windsorcli/cli/pkg/tui/plan"
)

// =============================================================================
// Constants
// =============================================================================

// destroyPlanFailedHint tells the operator how to continue when Windsor cannot plan the destroy of a component.
const destroyPlanFailedHint = "Resolve the errors above, then run the command again. Windsor does not ask to confirm a destroy that it cannot complete."

// =============================================================================
// Destroy Commands
// =============================================================================

var (
	destroyConfirm  string
	destroyContinue bool
)

// destroyCmd destroys every component of the context, or one component in both layers.
// It requests all tool requirements, because the blueprint decides which layers run.
// It skips validation, so that a broken setup can still be destroyed.
// For a full destroy, it does these steps before the confirmation prompt:
//   - It validates the backend component names.
//   - It checks the cloud credentials, because the destroy plan needs them.
//   - For a kubernetes backend, it moves all state to a local backend, so no step uses the backend that the destroy removes.
//   - It plans the destroy, and stops when it cannot plan a component.
//
// For one terraform component, it checks the credentials before the plan.
// It uses the local state when the cluster of the kubernetes backend is gone. If not, it refuses a backend component before terraform init runs.
var destroyCmd = &cobra.Command{
	Use:   "destroy [component]",
	Short: "Destroy live infrastructure.",
	Long: `Destroy live infrastructure. With no argument, removes every Flux kustomization, then every Terraform component. With a component name, destroys that component across both layers (Terraform and/or Kustomize).

Every form requires confirmation. Either type the context or component name at the prompt, or pass --confirm=<expected> to satisfy the gate non-interactively (CI-safe). The --confirm value must match the prompt token exactly; mismatches abort the operation.

If terraform reports resources protected by 'lifecycle { prevent_destroy = true }', destroy warns up front so the operator knows the destroy may halt partway through. It also warns when deletion_policy, deletion_protection, force_destroy, or skip_final_snapshot has drifted from state. Terraform components whose state is empty are skipped with a warning naming any potentially orphaned cloud resources. Destroy skips kustomizations separately when no local kubeconfig exists for the context. This case prints no orphan warning: the cluster is already gone.

If any component fails destroy-plan generation, destroy halts before the confirmation prompt and names the failed components, rather than offering to destroy a plan it cannot fully execute. This is distinct from --continue, which governs failures during execution, after confirmation.

The default behavior is to abort on the first per-component destroy failure. Pass --continue to keep going past failures and print a one-line summary at the end (windsor destroy: N destroyed, N no-op (empty state), N no-op (cluster gone), N failed (...), terraform deferred). --continue applies to a layer-wide destroy only; it is refused with a component argument.

Terraform is skipped in two cases:
- A non-backend component is left un-destroyed.
- A Flux kustomization fails to delete. A live controller, such as Crossplane, may still be tearing down a cloud resource that terraform never tracked, and Windsor cannot tell whether that's the case — so it never guesses. Destroying the cluster now could orphan that resource permanently.

Rerun 'windsor destroy --continue' after you resolve the failures. The next pass picks up where the last one stopped.

When terraform.backend.type is 'kubernetes', a full-cycle destroy (no argument) migrates every component's state to local before destroying anything, then destroys entirely against that local copy — the kubernetes backend stores state on the cluster the destroy is about to tear down, so reads pivot away from it up front rather than stranding mid-teardown once the cluster is gone. A single component can't be destroyed in isolation while it's one of the backend's components, since destroying it directly would orphan every other component's state; run a full 'windsor destroy' instead.`,
	Example: `# Destroy everything in the current context (interactive)
windsor destroy
# → prompts: Type "local" to confirm:

# Same, scripted
windsor destroy --confirm=local

# Destroy just the dns component (across both layers)
windsor destroy dns --confirm=dns

# Continue past per-component failures and converge by rerunning
windsor destroy --confirm=local --continue`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`down`](down.md), [`plan`](plan.md)",
		"docs.source":  "cmd/destroy.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireContinueScope(args); err != nil {
			return err
		}
		proj, err := prepareProjectSkipValidation(cmd, tools.AllRequirements())
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if err := proj.Provisioner.ValidateBackendComponents(blueprint); err != nil {
				return err
			}
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			if _, err := proj.Provisioner.PrepareLocalTeardown(cmd.Context(), blueprint); err != nil {
				return err
			}
			var summary *provisioner.DestroyPlanSummary
			if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyAll(blueprint)
				return planErr
			}); err != nil {
				return werror.Wrap(err, "error generating destroy plan")
			}
			tuiplan.DestroySummary(os.Stdout, summary.Terraform, summary.Kustomize, os.Getenv("NO_COLOR") != "")

			warnPreventDestroy(cmd.ErrOrStderr(), summary.Terraform)
			warnDestroyAttributeDrift(cmd.ErrOrStderr(), summary.Terraform)

			if err := failOnDestroyPlanErrors(summary.Terraform); err != nil {
				return err
			}

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all infrastructure in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
				result, err := proj.Provisioner.Teardown(cmd.Context(), blueprint, false, destroyContinue)
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), result.Skipped)
				reportSkippedClusterGoneKustomizations(cmd.ErrOrStderr(), result.SkippedClusterGone)
				if err != nil {
					return werror.Wrap(err, "error destroying all components")
				}
				return finishContinueDestroy(cmd.ErrOrStderr(), result)
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
			if _, err := proj.Provisioner.PivotToLocalIfClusterGone(); err != nil {
				return err
			}
			if err := proj.Provisioner.CheckComponentDestroyable(blueprint, componentID); err != nil {
				return err
			}
		}

		var tfResults []terraforminfra.TerraformComponentPlan
		var k8sResults []fluxinfra.KustomizePlan
		if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
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
			return werror.Wrap(err, "error generating destroy plan")
		}
		tuiplan.DestroySummary(os.Stdout, tfResults, k8sResults, os.Getenv("NO_COLOR") != "")

		warnPreventDestroy(cmd.ErrOrStderr(), tfResults)
		warnDestroyAttributeDrift(cmd.ErrOrStderr(), tfResults)

		if err := failOnDestroyPlanErrors(tfResults); err != nil {
			return err
		}
		if err := failOnDestroyKustomizePlanErrors(k8sResults); err != nil {
			return err
		}

		desc := fmt.Sprintf("This will permanently destroy component %q across all layers.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
			if inKustomize {
				if err := proj.Provisioner.DestroyKustomize(cmd.Context(), blueprint, componentID); err != nil {
					return werror.Wrap(err, "error destroying kustomization %s", componentID)
				}
			}
			if inTerraform {
				skipped, err := proj.Provisioner.TeardownComponent(cmd.Context(), blueprint, componentID)
				if err != nil {
					return werror.Wrap(err, "error destroying terraform for %s", componentID)
				}
				if skipped {
					reportSkippedDestroyComponents(cmd.ErrOrStderr(), []string{componentID})
				}
			}
			return nil
		})
	},
}

// destroyTerraformCmd destroys one terraform component, or all of them.
// It requests only terraform and the secrets tools. It skips validation, so that a broken setup can still be destroyed.
// For all components, it validates the backend names and moves the state to a local backend before the plan.
// For one component, it uses the local state when the cluster of the kubernetes backend is gone.
// If not, it refuses a backend component, because that destroy would orphan the state of the other components.
var destroyTerraformCmd = &cobra.Command{
	Use:     "terraform [component]",
	Aliases: []string{"tf"},
	Short:   "Destroy Terraform component(s).",
	Long:    `Destroy a specific Terraform component, or all components when no argument is given. Inherits --confirm from the parent 'destroy' command.`,
	Example: `# Destroy a single terraform component
windsor destroy terraform cluster --confirm=cluster

# Destroy every terraform component in the current context
windsor destroy terraform --confirm=local`,
	Annotations: map[string]string{
		"docs.seealso": "[`destroy`](destroy.md), [`apply terraform`](apply-terraform.md), [`plan terraform`](plan-terraform.md)",
		"docs.source":  "cmd/destroy.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireContinueScope(args); err != nil {
			return err
		}
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Terraform: true, Secrets: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			if err := proj.Provisioner.ValidateBackendComponents(blueprint); err != nil {
				return err
			}
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			if _, err := proj.Provisioner.PrepareLocalTeardown(cmd.Context(), blueprint); err != nil {
				return err
			}
			var summary *provisioner.DestroyPlanSummary
			if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyTerraformSummary(blueprint)
				return planErr
			}); err != nil {
				return werror.Wrap(err, "error generating destroy plan")
			}
			tuiplan.DestroySummary(os.Stdout, summary.Terraform, nil, os.Getenv("NO_COLOR") != "")

			warnPreventDestroy(cmd.ErrOrStderr(), summary.Terraform)
			warnDestroyAttributeDrift(cmd.ErrOrStderr(), summary.Terraform)

			if err := failOnDestroyPlanErrors(summary.Terraform); err != nil {
				return err
			}

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Terraform components in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
				result, err := proj.Provisioner.Teardown(cmd.Context(), blueprint, true, destroyContinue)
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), result.Skipped)
				if err != nil {
					return werror.Wrap(err, "error destroying all terraform")
				}
				return finishContinueDestroy(cmd.ErrOrStderr(), result)
			})
		}

		componentID := args[0]
		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}
		if _, err := proj.Provisioner.PivotToLocalIfClusterGone(); err != nil {
			return err
		}
		if err := proj.Provisioner.CheckComponentDestroyable(blueprint, componentID); err != nil {
			return err
		}
		var tfResult terraforminfra.TerraformComponentPlan
		if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
			var planErr error
			tfResult, planErr = proj.Provisioner.PlanDestroyTerraformComponentSummary(blueprint, componentID)
			return planErr
		}); err != nil {
			return werror.Wrap(err, "error generating destroy plan")
		}
		tuiplan.DestroySummary(os.Stdout, []terraforminfra.TerraformComponentPlan{tfResult}, nil, os.Getenv("NO_COLOR") != "")

		warnPreventDestroy(cmd.ErrOrStderr(), []terraforminfra.TerraformComponentPlan{tfResult})
		warnDestroyAttributeDrift(cmd.ErrOrStderr(), []terraforminfra.TerraformComponentPlan{tfResult})

		if err := failOnDestroyPlanErrors([]terraforminfra.TerraformComponentPlan{tfResult}); err != nil {
			return err
		}

		desc := fmt.Sprintf("This will permanently destroy Terraform component %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
			skipped, err := proj.Provisioner.TeardownComponent(cmd.Context(), blueprint, componentID)
			if err != nil {
				return werror.Wrap(err, "error destroying terraform for %s", componentID)
			}
			if skipped {
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), []string{componentID})
			}
			return nil
		})
	},
}

// destroyKustomizeCmd deletes one Flux kustomization, or all of them.
// It requests only the secrets tools and kubelogin, because it uses only the cluster API.
// It skips validation, so that a broken setup can still be destroyed.
var destroyKustomizeCmd = &cobra.Command{
	Use:     "kustomize [name]",
	Aliases: []string{"k8s"},
	Short:   "Destroy Flux kustomization(s).",
	Long:    `Delete a specific Flux kustomization from the cluster, or all kustomizations when no argument is given. Inherits --confirm from the parent 'destroy' command.`,
	Example: `# Delete a single kustomization
windsor destroy kustomize dns --confirm=dns

# Delete every kustomization in the current context
windsor destroy kustomize --confirm=local`,
	Annotations: map[string]string{
		"docs.seealso": "[`destroy`](destroy.md), [`apply kustomize`](apply-kustomize.md), [`plan kustomize`](plan-kustomize.md)",
		"docs.source":  "cmd/destroy.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireContinueScope(args); err != nil {
			return err
		}
		proj, err := prepareProjectSkipValidation(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			var summary *provisioner.DestroyPlanSummary
			if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
				var planErr error
				summary, planErr = proj.Provisioner.PlanDestroyKustomizeSummary(blueprint)
				return planErr
			}); err != nil {
				return werror.Wrap(err, "error generating destroy plan")
			}
			tuiplan.DestroySummary(os.Stdout, nil, summary.Kustomize, os.Getenv("NO_COLOR") != "")

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Flux kustomizations in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
				if err := proj.Provisioner.Uninstall(cmd.Context(), blueprint); err != nil {
					return werror.Wrap(err, "error destroying all kustomizations")
				}
				return nil
			})
		}

		componentID := args[0]
		var k8sResult fluxinfra.KustomizePlan
		if err := presenter.Track(cmd.Context(), "plan:destroy", "Generating destroy plan", func(context.Context) error {
			var planErr error
			k8sResult, planErr = proj.Provisioner.PlanDestroyKustomizeComponentSummary(blueprint, componentID)
			return planErr
		}); err != nil {
			return werror.Wrap(err, "error generating destroy plan")
		}
		tuiplan.DestroySummary(os.Stdout, nil, []fluxinfra.KustomizePlan{k8sResult}, os.Getenv("NO_COLOR") != "")

		if err := failOnDestroyKustomizePlanErrors([]fluxinfra.KustomizePlan{k8sResult}); err != nil {
			return err
		}

		desc := fmt.Sprintf("This will permanently destroy Flux kustomization %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", lockTimeout, func() error {
			if err := proj.Provisioner.DestroyKustomize(cmd.Context(), blueprint, componentID); err != nil {
				return werror.Wrap(err, "error destroying kustomization %s", componentID)
			}
			return nil
		})
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

// reportSkippedClusterGoneKustomizations prints a stderr note naming
// kustomizations skipped for a missing kubeconfig. This is the expected
// result of a destroy re-run after the cluster is already gone, not an
// orphan hazard. No-op when skipped is empty.
func reportSkippedClusterGoneKustomizations(w io.Writer, skipped []string) {
	if len(skipped) == 0 {
		return
	}
	if len(skipped) == 1 {
		fmt.Fprintf(w, "note: kustomization %q was already gone (no local kubeconfig) and was skipped\n", skipped[0])
	} else {
		fmt.Fprintf(w, "note: %d kustomizations were already gone (no local kubeconfig) and were skipped: %s\n", len(skipped), strings.Join(skipped, ", "))
	}
}

// warnPreventDestroy emits a stderr warning naming any resource addresses
// that terraform's plan -destroy flagged with `lifecycle { prevent_destroy =
// true }`. The wrapper deliberately does not refuse — operators who run
// `windsor destroy` typically want convergence on zero, and a partial
// destroy that halts mid-graph leaves more mess than a destroy that surfaces
// the protected addresses up front. Module authors who want hard protection
// should gate their resources on TF_VAR_operation / TF_VAR_ephemeral and
// design the destroy contract in HCL; see docs/spikes/terraform-lifecycle-hardening.md §6.2.
// No-op when protected is empty.
func warnPreventDestroy(w io.Writer, plans []terraforminfra.TerraformComponentPlan) {
	var protected []string
	for _, p := range plans {
		protected = append(protected, p.Protected...)
	}
	if len(protected) == 0 {
		return
	}
	fmt.Fprintf(w, "warning: terraform will refuse to destroy %d resource(s) protected by lifecycle { prevent_destroy = true }:\n", len(protected))
	for _, addr := range protected {
		fmt.Fprintf(w, "  %s\n", addr)
	}
	fmt.Fprintln(w, "   the destroy may halt partway; remove the lifecycle block in HCL to enable tear-down.")
}

// warnDestroyAttributeDrift emits a stderr warning naming any destroy-behavior attribute
// drift. plan -destroy never re-diffs these attributes against config, since the resource is
// being removed, not updated. A stale state value can persist silently through destroy until
// an intervening plan or apply reconciles it; see #3328. This mirrors warnPreventDestroy: it
// warns but does not refuse, since a blocked destroy leaves more mess than a completed one.
// No-op when no plan reports drift.
func warnDestroyAttributeDrift(w io.Writer, plans []terraforminfra.TerraformComponentPlan) {
	var drifted []terraforminfra.DestroyAttributeDrift
	for _, p := range plans {
		drifted = append(drifted, p.DriftedAttributes...)
	}
	if len(drifted) == 0 {
		return
	}
	fmt.Fprintf(w, "warning: %d resource(s) have destroy-behavior attributes that drifted from state:\n", len(drifted))
	for _, d := range drifted {
		fmt.Fprintf(w, "  %s: %s state=%s config=%s\n", d.Address, d.Attribute, d.State, d.Config)
	}
	fmt.Fprintln(w, "   destroy will use the value already in state, not your current config.")
	fmt.Fprintln(w, "   run `terraform apply` first to reconcile state, or expect the destroy to repeat this failure.")
}

// failOnDestroyPlanErrors stops the destroy before the confirmation prompt when a component failed destroy-plan generation.
// Each plan keeps its error in the Err field, and the summary shows it, but the summary methods return no error.
// Without this check, the operator can confirm a destroy that skips the components that Windsor could not plan.
// These are often the components that own the cloud infrastructure, so a skip leaves billed resources and partial state.
// It does nothing when every component planned without error.
// It is different from --continue, which controls failures during the destroy, after the confirmation.
func failOnDestroyPlanErrors(plans []terraforminfra.TerraformComponentPlan) error {
	var failed []string
	for _, p := range plans {
		if p.Err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", p.ComponentID, p.Err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return werror.New(fmt.Sprintf("destroy-plan generation failed for %d component(s): %s", len(failed), strings.Join(failed, ", ")), destroyPlanFailedHint, nil)
}

// failOnDestroyKustomizePlanErrors does the check of failOnDestroyPlanErrors for the Flux layer.
// PlanDestroyComponentSummary keeps a query failure in the Err field of the plan and does not return it.
// Without this check, the operator can confirm the destroy of a kustomization that Windsor could not plan.
// A stale kubeconfig from an earlier partial destroy is one cause of such a failure.
// It does nothing when every kustomization planned without error.
func failOnDestroyKustomizePlanErrors(plans []fluxinfra.KustomizePlan) error {
	var failed []string
	for _, p := range plans {
		if p.Err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", p.Name, p.Err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return werror.New(fmt.Sprintf("destroy-plan generation failed for %d kustomization(s): %s", len(failed), strings.Join(failed, ", ")), destroyPlanFailedHint, nil)
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

// requireContinueScope refuses --continue when the operator also gives a component argument.
// --continue collects failures across many components and prints a summary, so it applies only to a layer-wide destroy.
// Without this check, Windsor ignores the flag and stops at the first error, which the operator does not expect.
func requireContinueScope(args []string) error {
	if destroyContinue && len(args) > 0 {
		return werror.New("--continue applies to layer-wide destroy only", "Remove the component argument or remove --continue.", nil)
	}
	return nil
}

// finishContinueDestroy prints the --continue summary line to stderr, then the error of each failure.
// The errors let the operator find the cause, for example a stuck kustomization finalizer, without a second run.
// It returns an error when a component failed, so the command exits with a non-zero code.
// It returns nil when --continue is not set or when no component failed.
func finishContinueDestroy(w io.Writer, result provisioner.DestroyResult) error {
	if !destroyContinue {
		return nil
	}
	parts := []string{
		fmt.Sprintf("%d destroyed", len(result.Destroyed)),
		fmt.Sprintf("%d no-op (empty state)", len(result.Skipped)),
		fmt.Sprintf("%d no-op (cluster gone)", len(result.SkippedClusterGone)),
	}
	if len(result.Failed) > 0 {
		failedIDs := make([]string, 0, len(result.Failed))
		for _, f := range result.Failed {
			failedIDs = append(failedIDs, f.ID)
		}
		parts = append(parts, fmt.Sprintf("%d failed (%s)", len(result.Failed), strings.Join(failedIDs, ", ")))
	} else {
		parts = append(parts, "0 failed")
	}
	if result.TerraformDeferred {
		parts = append(parts, "terraform deferred")
	}
	fmt.Fprintf(w, "windsor destroy: %s\n", strings.Join(parts, ", "))
	if len(result.Failed) > 0 {
		for _, f := range result.Failed {
			if f.Err != nil {
				fmt.Fprintf(w, "  %s: %v\n", f.ID, f.Err)
			}
		}
		return werror.New(fmt.Sprintf("destroy completed with %d failure(s)", len(result.Failed)), "Resolve the failures, then run windsor destroy --continue.", nil)
	}
	return nil
}

// init registers destroy subcommands and persistent flags. --confirm must match the context
// name (layer-wide destroy) or component name (targeted destroy) exactly; this is the CI-safe
// equivalent of the interactive prompt. --continue makes the bulk destroy passes best-effort:
// it collects per-component failures instead of aborting, and defers terraform whenever a
// kustomize failure occurs or a non-backend component is left un-destroyed.
func init() {
	destroyCmd.PersistentFlags().StringVar(&destroyConfirm, "confirm", "", "Context or component name to confirm destruction. Must match the prompt token exactly; mismatches abort.")
	destroyCmd.PersistentFlags().BoolVar(&destroyContinue, "continue", false, "Continue past per-component destroy failures and report a summary at the end. Layer-wide destroy only. Defers terraform on any kustomize failure, or when a non-backend component fails.")
	destroyCmd.AddCommand(destroyTerraformCmd)
	destroyCmd.AddCommand(destroyKustomizeCmd)
	rootCmd.AddCommand(destroyCmd)
}
