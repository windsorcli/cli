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
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
	"github.com/windsorcli/cli/pkg/tui"
	tuiplan "github.com/windsorcli/cli/pkg/tui/plan"
)

// =============================================================================
// Destroy Commands
// =============================================================================

var (
	destroyConfirm  string
	destroyContinue bool
)

var destroyCmd = &cobra.Command{
	Use:   "destroy [component]",
	Short: "Destroy live infrastructure.",
	Long: `Destroy live infrastructure. With no argument, removes every Flux kustomization, then every Terraform component. With a component name, destroys that component across both layers (Terraform and/or Kustomize).

Every form requires confirmation. Either type the context or component name at the prompt, or pass --confirm=<expected> to satisfy the gate non-interactively (CI-safe). The --confirm value must match the prompt token exactly; mismatches abort the operation.

If terraform reports resources protected by 'lifecycle { prevent_destroy = true }', destroy warns up front so the operator knows the destroy may halt partway through. Resources whose state is empty are skipped with a warning naming any potentially orphaned cloud resources.

The default behavior is to abort on the first per-component destroy failure. Pass --continue to keep going past individual failures, collect them, and print a one-line summary at the end (windsor destroy: N destroyed, N no-op (empty state), N failed (...), backend tier deferred). --continue is layer-wide only and is refused when combined with a component argument — on a single component there is nothing to continue past. When --continue leaves any non-tier component un-destroyed, the backend tier is NOT attempted — this prevents destroying the state store while other components still depend on it. Rerun 'windsor destroy --continue' after resolving the underlying failures; the second pass picks up where the first left off and converges on a clean slate.`,
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
			// For a kubernetes backend, pull all terraform state to local and pivot the whole teardown to a
			// local backend up front — before planning — so no step (plan or destroy) dials the kubernetes
			// backend that this teardown is about to destroy.
			pivoted, err := proj.Provisioner.PrepareLocalTeardown(blueprint)
			if err != nil {
				return err
			}
			if pivoted {
				fmt.Fprintln(cmd.ErrOrStderr(), "Terraform state migrated to local; running teardown against local state.")
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

			warnPreventDestroy(cmd.ErrOrStderr(), summary.Terraform)

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all infrastructure in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
				result, err := proj.Provisioner.Teardown(blueprint, false, destroyContinue)
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), result.Skipped)
				if err != nil {
					return fmt.Errorf("error destroying all components: %w", err)
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

		// Gate auth before plan (for any terraform leg) so credential failures
		// surface before the prompt rather than between plan and confirm.
		if inTerraform {
			if err := requireCloudAuth(cmd, proj); err != nil {
				return err
			}
			// If the kubernetes backend's cluster is gone, operate on the local state a prior teardown
			// migrated; otherwise refuse a backend-tier component up front, before the plan runs terraform
			// init against the kubernetes backend — the operator gets the clean "run windsor destroy"
			// guidance instead of a raw init connection error.
			if _, err := proj.Provisioner.PivotToLocalIfClusterGone(blueprint); err != nil {
				return err
			}
			if err := proj.Provisioner.CheckComponentDestroyable(blueprint, componentID); err != nil {
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

		warnPreventDestroy(cmd.ErrOrStderr(), tfResults)

		desc := fmt.Sprintf("This will permanently destroy component %q across all layers.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
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
		})
	},
}

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
			// Destroying every terraform component: pull state to local and pivot up front, like the full
			// teardown, so the plan and destroy never dial the kubernetes backend being torn down.
			pivoted, err := proj.Provisioner.PrepareLocalTeardown(blueprint)
			if err != nil {
				return err
			}
			if pivoted {
				fmt.Fprintln(cmd.ErrOrStderr(), "Terraform state migrated to local; running teardown against local state.")
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

			warnPreventDestroy(cmd.ErrOrStderr(), summary.Terraform)

			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Terraform components in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
				result, err := proj.Provisioner.Teardown(blueprint, true, destroyContinue)
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), result.Skipped)
				if err != nil {
					return fmt.Errorf("error destroying all terraform: %w", err)
				}
				return finishContinueDestroy(cmd.ErrOrStderr(), result)
			})
		}

		componentID := args[0]
		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}
		// Targeted destroy: if the kubernetes backend's cluster is gone, operate on the local state a prior
		// teardown migrated; otherwise refuse a backend-tier member (destroying it while its backend is live
		// would orphan every other component's state) before the plan surfaces a raw init error.
		if _, err := proj.Provisioner.PivotToLocalIfClusterGone(blueprint); err != nil {
			return err
		}
		if err := proj.Provisioner.CheckComponentDestroyable(blueprint, componentID); err != nil {
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

		warnPreventDestroy(cmd.ErrOrStderr(), []terraforminfra.TerraformComponentPlan{tfResult})

		desc := fmt.Sprintf("This will permanently destroy Terraform component %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
			skipped, err := proj.Provisioner.TeardownComponent(blueprint, componentID)
			if err != nil {
				return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
			}
			if skipped {
				reportSkippedDestroyComponents(cmd.ErrOrStderr(), []string{componentID})
			}
			return nil
		})
	},
}

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
			return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
				if err := proj.Provisioner.Uninstall(blueprint); err != nil {
					return fmt.Errorf("error destroying all kustomizations: %w", err)
				}
				return nil
			})
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
		return stacklock.With(cmd.Context(), proj.Runtime, "destroy", func() error {
			if err := proj.Provisioner.DestroyKustomize(blueprint, componentID); err != nil {
				return fmt.Errorf("error destroying kustomization %s: %w", componentID, err)
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

// requireContinueScope refuses --continue when a component argument is also
// present. The flag's contract (best-effort across multiple components plus a
// summary line) only makes sense for a layer-wide destroy; on a single
// component there is nothing to continue past. Without this guard the flag
// is silently ignored, which gives the operator the default abort-on-error
// behaviour while believing they opted into best-effort.
func requireContinueScope(args []string) error {
	if destroyContinue && len(args) > 0 {
		return fmt.Errorf("--continue applies to layer-wide destroy only; omit the component argument or drop --continue")
	}
	return nil
}

// finishContinueDestroy emits the one-line --continue summary on stderr and
// returns a non-zero exit error when any component failed. Without --continue
// the existing per-error abort already surfaces failures, so no extra output
// is needed; this only fires when the operator opted into best-effort mode.
// After the summary, each failure's underlying error is printed so the operator
// can diagnose the cause (e.g. a stuck kustomization finalizer) without re-running
// under a debugger. Returns nil when destroyContinue is false or when no failures
// were recorded.
func finishContinueDestroy(w io.Writer, result provisioner.DestroyResult) error {
	if !destroyContinue {
		return nil
	}
	parts := []string{
		fmt.Sprintf("%d destroyed", len(result.Destroyed)),
		fmt.Sprintf("%d no-op (empty state)", len(result.Skipped)),
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
	if result.TierDeferred {
		parts = append(parts, "backend tier deferred")
	}
	fmt.Fprintf(w, "windsor destroy: %s\n", strings.Join(parts, ", "))
	if len(result.Failed) > 0 {
		for _, f := range result.Failed {
			if f.Err != nil {
				fmt.Fprintf(w, "  %s: %v\n", f.ID, f.Err)
			}
		}
		return fmt.Errorf("destroy completed with %d failure(s); rerun `windsor destroy --continue` after resolving them", len(result.Failed))
	}
	return nil
}

// init registers destroy subcommands and persistent flags. --confirm must exactly match the
// context name (for layer-wide destroy) or component name (for targeted destroy); this is the
// CI-safe equivalent of the interactive prompt. --continue switches the bulk destroy passes
// to best-effort: per-component failures are collected rather than aborting, and the backend
// tier is deferred when any non-tier component is left un-destroyed (rerun to converge).
func init() {
	destroyCmd.PersistentFlags().StringVar(&destroyConfirm, "confirm", "", "Context or component name to confirm destruction. Must match the prompt token exactly; mismatches abort.")
	destroyCmd.PersistentFlags().BoolVar(&destroyContinue, "continue", false, "Continue past per-component destroy failures and report a summary at the end. Layer-wide destroy only — refuses when combined with a component argument. Backend tier is deferred when any non-tier component fails.")
	destroyCmd.AddCommand(destroyTerraformCmd)
	destroyCmd.AddCommand(destroyKustomizeCmd)
	rootCmd.AddCommand(destroyCmd)
}
