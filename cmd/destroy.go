package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/project"
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
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all infrastructure in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			restore, err := migrateStateToLocal(proj, blueprint)
			if err != nil {
				return err
			}
			skipped, err := destroyAllWithBackendLifecycle(proj, blueprint, false)
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
			restore, err := migrateStateToLocal(proj, blueprint)
			if err != nil {
				return err
			}
			skipped, err := destroyTerraformComponentWithBackendLifecycle(proj, blueprint, componentID)
			if err != nil {
				return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
			}
			if skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "Component %q has empty state — nothing to destroy.\n", componentID)
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
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
			contextName := proj.Runtime.ContextName
			desc := fmt.Sprintf("This will permanently destroy all Terraform components in context %q.", contextName)
			if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, contextName); err != nil {
				return err
			}
			restore, err := migrateStateToLocal(proj, blueprint)
			if err != nil {
				return err
			}
			skipped, err := destroyAllWithBackendLifecycle(proj, blueprint, true)
			reportSkippedDestroyComponents(cmd.ErrOrStderr(), skipped)
			if err != nil {
				return fmt.Errorf("error destroying all terraform: %w", err)
			}
			return nil
		}

		componentID := args[0]
		desc := fmt.Sprintf("This will permanently destroy Terraform component %q.", componentID)
		if err := resolveDestroyConfirmation(cmd.InOrStdin(), cmd.ErrOrStderr(), desc, componentID); err != nil {
			return err
		}
		restore, err := migrateStateToLocal(proj, blueprint)
		if err != nil {
			return err
		}
		skipped, err := destroyTerraformComponentWithBackendLifecycle(proj, blueprint, componentID)
		if err != nil {
			return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
		}
		if skipped {
			fmt.Fprintf(cmd.ErrOrStderr(), "Component %q has empty state — nothing to destroy.\n", componentID)
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
		proj, err := prepareProject(cmd)
		if err != nil {
			return err
		}

		blueprint := proj.Composer.BlueprintHandler.Generate()

		if len(args) == 0 {
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

// reportSkippedDestroyComponents prints a one-line summary to w naming any terraform
// components whose destroy was skipped because their state was empty (never applied, fully
// torn down already, or upstream destroy collapsed their cloud objects out from under them).
// Called after DestroyAll/DestroyAllTerraform regardless of whether the operation overall
// succeeded — the skip list is paired with the returned error so an operator sees both
// "these were no-ops" and "this one failed" in the same output. No-op when skipped is empty.
func reportSkippedDestroyComponents(w io.Writer, skipped []string) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(w, "Skipped (empty state, nothing to destroy): %s\n", strings.Join(skipped, ", "))
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

// destroyAllWithBackendLifecycle dispatches bulk destroy on terraform.backend.type:
//
//   - kubernetes  → full-cycle: pin backend.type=local, MigrateState pulls every
//     component's state from the cluster's Secrets to local files, then DestroyAll
//     tears everything down in reverse dependency order against the local state
//     (which is critical because the cluster itself is going away). Mirror image
//     of runFullCycleBootstrap on the destroy side.
//
//   - s3 / azurerm → per-component dance: destroy non-backend components FIRST
//     against the live remote backend (init reads remote, destroy writes remote),
//     then pin backend.type=local, migrate the backend component's state from
//     remote to local, destroy the backend component last. Avoids the wasteful
//     "migrate everything to local first" pattern for backends where every
//     non-backend component's state is independent of the backend bucket.
//
//   - local / unset → direct DestroyAll, no migration.
//
// terraformOnly=true skips the kustomize uninstall that DestroyAll normally runs
// first. Returns the IDs of components skipped because their state was empty
// alongside any error, mirroring Provisioner.DestroyAll's contract.
func destroyAllWithBackendLifecycle(proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool) ([]string, error) {
	originalBackend := proj.Runtime.ConfigHandler.GetString("terraform.backend.type", "local")

	switch originalBackend {
	case "kubernetes":
		return runFullCycleDestroyAll(proj, blueprint, terraformOnly, originalBackend)
	case "s3", "azurerm":
		return runPerComponentDestroyAll(proj, blueprint, terraformOnly, originalBackend)
	default:
		if terraformOnly {
			return proj.Provisioner.DestroyAllTerraform(blueprint)
		}
		return proj.Provisioner.DestroyAll(blueprint)
	}
}

// runFullCycleDestroyAll is the destroy mirror of runFullCycleBootstrap. The
// backend (kubernetes Secrets in the cluster) is going away as part of this
// teardown, so every component's state must move to local before any destroy
// runs — otherwise destroy-in-progress would race with the cluster's Secret
// store evaporating. After migration, DestroyAll iterates components in reverse
// against the local state. The deferred restore guards exit paths between the
// override and any panic; restoring twice is idempotent.
func runFullCycleDestroyAll(proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, originalBackend string) ([]string, error) {
	ch := proj.Runtime.ConfigHandler
	if err := ch.Set("terraform.backend.type", "local"); err != nil {
		return nil, fmt.Errorf("failed to override backend for destroy: %w", err)
	}
	defer func() {
		if err := ch.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if _, err := proj.Provisioner.MigrateState(blueprint); err != nil {
		return nil, fmt.Errorf("failed to migrate state to local before destroy: %w", err)
	}

	if terraformOnly {
		return proj.Provisioner.DestroyAllTerraform(blueprint)
	}
	return proj.Provisioner.DestroyAll(blueprint)
}

// runPerComponentDestroyAll is the destroy mirror of runPerComponentBootstrap.
// When the blueprint declares a "backend" terraform component and the configured
// backend is s3/azurerm, non-backend components are destroyed first against the
// live remote backend (their state is independent of any one component, the
// bucket still exists), then backend.type flips to local, the backend
// component's state is migrated from remote to local, and the backend component
// is destroyed last. Without a backend component the call collapses to a direct
// DestroyAll — out-of-band bucket setups stay on remote state through teardown.
func runPerComponentDestroyAll(proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, terraformOnly bool, originalBackend string) ([]string, error) {
	ch := proj.Runtime.ConfigHandler
	backendID := findBackendComponentID(blueprint)

	if backendID == "" {
		if terraformOnly {
			return proj.Provisioner.DestroyAllTerraform(blueprint)
		}
		return proj.Provisioner.DestroyAll(blueprint)
	}

	var skipped []string
	var bulkErr error
	if terraformOnly {
		skipped, bulkErr = proj.Provisioner.DestroyAllTerraform(blueprint, backendID)
	} else {
		skipped, bulkErr = proj.Provisioner.DestroyAll(blueprint, backendID)
	}
	if bulkErr != nil {
		return skipped, bulkErr
	}

	if err := ch.Set("terraform.backend.type", "local"); err != nil {
		return skipped, fmt.Errorf("failed to override backend for backend-component destroy: %w", err)
	}
	defer func() {
		if err := ch.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if err := proj.Provisioner.MigrateComponentState(blueprint, backendID); err != nil {
		return skipped, err
	}
	backendSkipped, err := proj.Provisioner.Destroy(blueprint, backendID)
	if err != nil {
		return skipped, err
	}
	if backendSkipped {
		skipped = append(skipped, backendID)
	}
	return skipped, nil
}

// destroyTerraformComponentWithBackendLifecycle destroys a single terraform
// component. The kubernetes path is intentionally simple: direct destroy with
// kubeconfig already in env from a prior `windsor up` — operators destroying
// individual components don't need the full-cycle dance because the cluster is
// still alive and serving Secrets. For s3/azurerm, the backend component
// special-case applies: migrate its state to local first, then destroy. All
// other backends/components fall through to a direct destroy. Returns the same
// (skipped, err) tuple as Provisioner.Destroy.
func destroyTerraformComponentWithBackendLifecycle(proj *project.Project, blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	originalBackend := proj.Runtime.ConfigHandler.GetString("terraform.backend.type", "local")
	if originalBackend != "s3" && originalBackend != "azurerm" {
		return proj.Provisioner.Destroy(blueprint, componentID)
	}

	backendID := findBackendComponentID(blueprint)
	if componentID != backendID {
		return proj.Provisioner.Destroy(blueprint, componentID)
	}

	ch := proj.Runtime.ConfigHandler
	if err := ch.Set("terraform.backend.type", "local"); err != nil {
		return false, fmt.Errorf("failed to override backend for backend-component destroy: %w", err)
	}
	defer func() {
		if err := ch.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}()

	if err := proj.Provisioner.MigrateComponentState(blueprint, componentID); err != nil {
		return false, err
	}
	return proj.Provisioner.Destroy(blueprint, componentID)
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
