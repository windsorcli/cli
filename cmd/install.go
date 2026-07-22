package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var installWaitFlag bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the blueprint's Flux kustomizations.",
	Long: `Apply only the Flux kustomizations to the cluster, skipping Terraform. Use this when Terraform has already been applied separately (e.g. by another tool or pipeline) and you only want to hand the cluster off to Flux.

For most workflows, prefer 'windsor apply', which runs Terraform and Flux in the right order.

Pass --wait to block until kustomizations report ready.`,
	Example: `# Install kustomizations and wait for them to settle
windsor install --wait`,
	Annotations: map[string]string{
		"docs.seealso": "[`apply`](apply.md), [`apply kustomize`](apply-kustomize.md)",
		"docs.source":  "cmd/install.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// `install` applies the blueprint to the cluster (Flux + kustomizations); it does not
		// invoke terraform or the local container runtime.
		proj, err := prepareProject(cmd, tools.Requirements{Secrets: true, Kubelogin: true})
		if err != nil {
			return err
		}

		blueprint, err := proj.Composer.BlueprintHandler.GenerateResolved()
		if err != nil {
			return fmt.Errorf("error resolving blueprint substitutions: %w", err)
		}

		resolvedSecrets, err := proj.Provisioner.ResolveSecrets(blueprint)
		if err != nil {
			return fmt.Errorf("error resolving secrets: %w", err)
		}

		if err := proj.Provisioner.Install(cmd.Context(), blueprint); err != nil {
			return fmt.Errorf("error installing blueprint: %w", err)
		}

		if err := proj.Provisioner.PlaceSecrets(cmd.Context(), resolvedSecrets, false); err != nil {
			return fmt.Errorf("error placing secrets: %w", err)
		}

		if installWaitFlag {
			if err := proj.Provisioner.Wait(cmd.Context(), blueprint); err != nil {
				return fmt.Errorf("error waiting for kustomizations: %w", err)
			}
		}

		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&installWaitFlag, "wait", false, "Wait for kustomization resources to be ready.")
	rootCmd.AddCommand(installCmd)
}
