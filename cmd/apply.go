package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var applyWaitFlag bool  // Wait for kustomization resources to be ready after applying
var applyForceFlag bool // Apply even when the blueprint version differs from what is applied

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply terraform and install the blueprint.",
	Long: `Run Terraform components, then install the Flux blueprint. Use the 'terraform' or 'kustomize' subcommand to scope to a single layer.

For workstation contexts, prefer 'windsor up' — it does the same work plus VM management.

Pass --wait to block until kustomizations report ready.`,
	Example: `# Apply everything and block until ready
windsor apply --wait

# Apply only the cluster terraform component
windsor apply terraform cluster

# Apply just the dns kustomization
windsor apply kustomize dns`,
	Annotations: map[string]string{
		"docs.seealso": "[`plan`](plan.md), [`destroy`](destroy.md), [`up`](up.md), [`bootstrap`](bootstrap.md)",
		"docs.source": "cmd/apply.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `windsor apply` runs whatever is in the blueprint — terraform components and Flux
		// kustomizations both — so the tool surface is not statically narrowable. Request
		// AllRequirements() and let the per-tool config gates inside CheckRequirements decide
		// what actually runs (e.g. platform=="azure" or an azure: block triggers kubelogin).
		proj, err := prepareProject(cmd, tools.AllRequirements())
		if err != nil {
			return err
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
		}

		if err := enforceApplyVersionGate(cmd, proj, blueprint, applyForceFlag); err != nil {
			return err
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "apply", func() error {
			// 'apply' doesn't run the workstation prep that registers MakeApplyHook, so no
			// onApply hooks fire and the halted return is always false. Ignore it.
			if _, err := proj.Provisioner.Up(blueprint); err != nil {
				return fmt.Errorf("error applying terraform: %w", err)
			}

			// Re-generate with deferred substitutions resolved now that terraform
			// outputs are available from the Up step above.
			var resolveErr error
			blueprint, resolveErr = proj.Composer.BlueprintHandler.GenerateResolved()
			if resolveErr != nil {
				return fmt.Errorf("error resolving blueprint substitutions: %w", resolveErr)
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error applying kustomize: %w", err)
			}

			if applyWaitFlag {
				if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
					return fmt.Errorf("error waiting for kustomizations: %w", err)
				}
			}

			return nil
		})
	},
}

var applyTerraformCmd = &cobra.Command{
	Use:     "terraform <component>",
	Aliases: []string{"tf"},
	Short:   "Apply Terraform changes for a single component.",
	Long:    `Run terraform apply for a single component. The <component> argument is required and must match a terraform component declared in the blueprint.`,
	Example: `# Apply the cluster component
windsor apply terraform cluster

# Same, using the 'tf' alias
windsor apply tf cluster`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`plan terraform`](plan-terraform.md), [`destroy terraform`](destroy-terraform.md)",
		"docs.source": "cmd/apply.go",
	},
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentID := args[0]

		// `apply terraform <project>` only invokes terraform. Secrets backends are required
		// because terraform can dereference 1Password / SOPS-encrypted values during plan/apply;
		// docker, colima, and kubelogin are not exercised by this codepath.
		proj, err := prepareProject(cmd, tools.Requirements{Terraform: true, Secrets: true})
		if err != nil {
			return err
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		return stacklock.With(cmd.Context(), proj.Runtime, "apply", func() error {
			if err := proj.Provisioner.Apply(blueprint, componentID); err != nil {
				return fmt.Errorf("error applying terraform for %s: %w", componentID, err)
			}
			return nil
		})
	},
}

var applyKustomizeCmd = &cobra.Command{
	Use:   "kustomize [name]",
	Short: "Apply Flux kustomization(s) to the cluster.",
	Long: `Apply a single Flux kustomization to the cluster by name, or all kustomizations when no argument is given.

When a name is supplied with --wait, the wait scope is narrowed to only that kustomization.`,
	Example: `# Apply all kustomizations
windsor apply kustomize

# Apply just the dns kustomization
windsor apply kustomize dns

# Apply and wait for one kustomization to be ready
windsor apply kustomize dns --wait`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`plan kustomize`](plan-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)",
		"docs.source": "cmd/apply.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `apply kustomize` only talks to the cluster API and dereferences secrets; it does not
		// invoke terraform or the local container runtime.
		proj, err := prepareProject(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
		}
		waitBlueprint := blueprint

		return stacklock.With(cmd.Context(), proj.Runtime, "apply", func() error {
			if len(args) == 0 {
				if err := proj.Provisioner.ApplyKustomizeAll(cmd.Context(), blueprint); err != nil {
					return fmt.Errorf("error applying kustomize: %w", err)
				}
			} else {
				componentID := args[0]
				if err := proj.Provisioner.ApplyKustomize(cmd.Context(), blueprint, componentID); err != nil {
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
				if err := proj.Provisioner.Wait(cmd.Context(), waitBlueprint); err != nil {
					return fmt.Errorf("error waiting for kustomizations: %w", err)
				}
			}

			return nil
		})
	},
}

// enforceApplyVersionGate enforces the version-equality seam for `apply`: apply reconciles the
// currently-applied blueprint version and refuses to cross a version boundary, which belongs to
// `upgrade`. A context with no readable marker (pre-bootstrap, no cluster, or legacy) is allowed —
// there is nothing applied to cross. An in-flight upgrade is refused so a half-finished transition
// is resumed with `upgrade`, never reconciled by `apply`; --force does not override this, since
// applying over a live transition risks corrupting it. A version mismatch is refused and redirected
// to `upgrade`, unless --force is set, in which case it warns and proceeds. A marker read failure
// (cluster unreachable) is best-effort: it warns and proceeds rather than blocking terraform-based
// recovery, since apply's later steps would surface a genuinely-unreachable cluster anyway.
func enforceApplyVersionGate(cmd *cobra.Command, proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, force bool) error {
	gate, err := proj.Provisioner.CheckVersionGate(blueprint)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not verify the applied blueprint version (%v); proceeding without the version gate.\n", err)
		return nil
	}
	if !gate.MarkerFound {
		return nil
	}
	if gate.InFlight {
		silenceErrorsOnAncestors(cmd)
		return fmt.Errorf("an upgrade is in progress for this context; run `windsor upgrade` to resume it. apply cannot reconcile a half-finished version transition")
	}
	if !gate.VersionMatch {
		if force {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: the blueprint version differs from what is applied; applying anyway because --force was set.")
			return nil
		}
		silenceErrorsOnAncestors(cmd)
		return fmt.Errorf("the blueprint version differs from what is applied; run `windsor upgrade` to transition versions, or re-run with --force to apply against the current version anyway")
	}
	return nil
}

func init() {
	applyCmd.Flags().BoolVar(&applyWaitFlag, "wait", false, "Wait for kustomization resources to be ready.")
	applyCmd.Flags().BoolVar(&applyForceFlag, "force", false, "Apply even if the blueprint version differs from what is applied (skips the upgrade version gate).")
	applyKustomizeCmd.Flags().BoolVar(&applyWaitFlag, "wait", false, "Wait for kustomization resources to be ready.")
	applyCmd.AddCommand(applyTerraformCmd)
	applyCmd.AddCommand(applyKustomizeCmd)
	rootCmd.AddCommand(applyCmd)
}
