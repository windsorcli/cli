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
			defer restore()
			if err := proj.Provisioner.DestroyAll(blueprint); err != nil {
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
			defer restore()
			if err := proj.Provisioner.Destroy(blueprint, componentID); err != nil {
				return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
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
			defer restore()
			if err := proj.Provisioner.DestroyAllTerraform(blueprint); err != nil {
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
		defer restore()
		if err := proj.Provisioner.Destroy(blueprint, componentID); err != nil {
			return fmt.Errorf("error destroying terraform for %s: %w", componentID, err)
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

// migrateStateToLocal overrides the configured terraform backend to "local" in-memory and runs
// MigrateState so each component's state is pulled from its currently configured remote backend
// into local files. The override must remain active through the subsequent destroy pass —
// otherwise destroy would try to read state from the remote backend again, which may be about
// to be torn down (for example, the S3 bucket that stores its own state). Returns a restore
// function the caller must defer; it re-applies the originally configured backend type once
// destruction completes. Components whose state is already local (or whose directories are not
// materialized) are no-ops, so this is safe to call on every terraform-touching destroy path.
func migrateStateToLocal(proj *project.Project, blueprint *blueprintv1alpha1.Blueprint) (func(), error) {
	ch := proj.Runtime.ConfigHandler
	originalBackend := ch.GetString("terraform.backend.type", "local")
	if err := ch.Set("terraform.backend.type", "local"); err != nil {
		return nil, fmt.Errorf("failed to override backend for destroy: %w", err)
	}
	restore := func() {
		if err := ch.Set("terraform.backend.type", originalBackend); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore terraform.backend.type to %q after destroy: %v\n", originalBackend, err)
		}
	}
	if _, err := proj.Provisioner.MigrateState(blueprint); err != nil {
		restore()
		return nil, fmt.Errorf("failed to migrate state to local before destroy: %w", err)
	}
	return restore, nil
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
