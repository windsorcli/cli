package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner/stacklock"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var applyPruneFlag bool // Remove kustomizations the blueprint no longer declares

// applyCmd applies all terraform components and installs all Flux kustomizations of the blueprint.
// It requests all tool requirements, and the config gates in CheckRequirements decide which tools to check.
// It reconciles at each blueprint version. When an upgrade did not finish, or a gate check fails, it warns and continues.
// It ignores the halted result of Up, because apply does not register the apply hook.
// After the Up step, it generates the blueprint again with the terraform outputs.
// With --prune, it deletes the kustomizations that the blueprint no longer declares, after the declared set is Ready.
// Without --prune, it only reports them.
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply terraform and install the blueprint.",
	Long: `Run Terraform components, then install the Flux blueprint. Use the 'terraform' or 'kustomize' subcommand to scope to a single layer.

For workstation contexts, prefer 'windsor up' — it does the same work plus VM management.

Waits until every kustomization is Ready, and fails when one does not become Ready. Pass --prune to also remove kustomizations the blueprint no longer declares, once the new set is Ready.`,
	Example: `# Apply everything
windsor apply

# Apply and remove kustomizations no longer declared
windsor apply --prune

# Apply only the cluster terraform component
windsor apply terraform cluster

# Apply just the dns kustomization
windsor apply kustomize dns`,
	Annotations: map[string]string{
		"docs.seealso": "[`plan`](plan.md), [`destroy`](destroy.md), [`up`](up.md), [`bootstrap`](bootstrap.md)",
		"docs.source":  "cmd/apply.go",
	},
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		if gate, gateErr := proj.Provisioner.CheckVersionGate(blueprint); gateErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: pre-apply version check: %v\n", gateErr)
		} else if gate.InFlight {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: an upgrade is in progress or was interrupted for this context. apply will reconcile to the declared blueprint; run `windsor upgrade` to complete the version transition.")
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "apply", lockTimeout, func() error {
			if _, err := proj.Provisioner.Up(cmd.Context(), blueprint); err != nil {
				return werror.Wrap(err, "error applying terraform")
			}

			var resolveErr error
			blueprint, resolveErr = proj.Composer.BlueprintHandler.GenerateResolved()
			if resolveErr != nil {
				return werror.Wrap(resolveErr, "error resolving blueprint substitutions")
			}
			if blueprint == nil {
				return fmt.Errorf("resolved blueprint is not available")
			}

			if err := proj.Provisioner.Install(cmd.Context(), blueprint, applyPruneFlag); err != nil {
				if applyPruneFlag || warnNotReady(cmd.ErrOrStderr(), err, blueprint.Messages) != nil {
					return werror.Wrap(err, "error applying kustomize")
				}
				printPostRunMessages(cmd.ErrOrStderr(), blueprint.Messages)
			}

			prunable, err := proj.Provisioner.PrunableKustomizations(blueprint)
			if err != nil {
				return werror.Wrap(err, "error listing kustomizations to prune")
			}
			if len(prunable) > 0 && !applyPruneFlag {
				fmt.Fprintf(cmd.OutOrStdout(), "The following kustomizations are no longer declared; re-run with --prune to remove them:\n  %s\n", strings.Join(prunable, "\n  "))
			}
			if len(prunable) == 0 || !applyPruneFlag {
				return nil
			}
			return pruneOrphaned(cmd, proj, blueprint, prunable)
		})
	},
}

// applyTerraformCmd applies one terraform component.
// It requests terraform and the secrets tools, because terraform can resolve 1Password and SOPS values.
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
		"docs.source":  "cmd/apply.go",
	},
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		componentID := args[0]

		proj, err := prepareProject(cmd, tools.Requirements{Terraform: true, Secrets: true})
		if err != nil {
			return err
		}

		if err := requireCloudAuth(cmd, proj); err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		return stacklock.With(cmd.Context(), proj.Runtime, "apply", lockTimeout, func() error {
			if err := proj.Provisioner.Apply(cmd.Context(), blueprint, componentID); err != nil {
				return werror.Wrap(err, "error applying terraform for %s", componentID)
			}
			return nil
		})
	},
}

// applyKustomizeCmd applies one Flux kustomization, or all of them.
// It requests only the secrets tools and kubelogin, because it does not run terraform or the container runtime.
var applyKustomizeCmd = &cobra.Command{
	Use:     "kustomize [name]",
	Aliases: []string{"k8s"},
	Short:   "Apply Flux kustomization(s) to the cluster.",
	Long: `Apply a single Flux kustomization to the cluster by name, or all kustomizations when no argument is given.

Waits until the applied kustomizations are Ready, and fails when one does not become Ready.`,
	Example: `# Apply all kustomizations
windsor apply kustomize

# Apply just the dns kustomization
windsor apply kustomize dns

# Same, using the 'k8s' alias
windsor apply k8s dns`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`plan kustomize`](plan-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)",
		"docs.source":  "cmd/apply.go",
	},
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := prepareProject(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()
		if blueprint == nil {
			return fmt.Errorf("blueprint is not available")
		}

		scope := make(map[string]bool)
		if len(args) == 0 {
			for _, k := range blueprint.Kustomizations {
				if k.DestroyOnly != nil && *k.DestroyOnly {
					continue
				}
				scope[k.Name] = true
			}
		} else {
			scope[args[0]] = true
		}
		if proj.Composer.BlueprintHandler.DeferredSubstitutionsInScope(scope) {
			return werror.New("unresolved terraform_output() substitutions in scope", "Run windsor apply or windsor upgrade first. If you do not, this command writes raw expression text over resolved ConfigMap values.", nil)
		}

		return stacklock.With(cmd.Context(), proj.Runtime, "apply", lockTimeout, func() error {
			if len(args) == 0 {
				if err := proj.Provisioner.ApplyKustomizeAll(cmd.Context(), blueprint); err != nil {
					return werror.Wrap(err, "error applying kustomize")
				}
			} else {
				componentID := args[0]
				if err := proj.Provisioner.ApplyKustomize(cmd.Context(), blueprint, componentID); err != nil {
					return werror.Wrap(err, "error applying kustomize for %s", componentID)
				}
			}

			return nil
		})
	},
}

func init() {
	addDeprecatedWaitFlag(applyCmd)
	applyCmd.Flags().BoolVar(&applyPruneFlag, "prune", false, "Remove kustomizations the blueprint no longer declares.")
	addDeprecatedWaitFlag(applyKustomizeCmd)
	applyCmd.AddCommand(applyTerraformCmd)
	applyCmd.AddCommand(applyKustomizeCmd)
	rootCmd.AddCommand(applyCmd)
}
