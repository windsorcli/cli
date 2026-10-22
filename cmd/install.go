package cmd

import (
	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the blueprint's Flux kustomizations.",
	Long: `Apply only the Flux kustomizations to the cluster, skipping Terraform. Use this when Terraform has already been applied separately (e.g. by another tool or pipeline) and you only want to hand the cluster off to Flux.

For most workflows, prefer 'windsor apply', which runs Terraform and Flux in the right order.

Waits until every kustomization is Ready, and fails when one does not become Ready.`,
	Example: `# Install kustomizations
windsor install`,
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
			return werror.Wrap(err, "error resolving blueprint substitutions")
		}

		if err := proj.Provisioner.Install(cmd.Context(), blueprint, false); err != nil {
			return werror.Wrap(err, "error installing blueprint")
		}

		return nil
	},
}

func init() {
	addDeprecatedWaitFlag(installCmd)
	rootCmd.AddCommand(installCmd)
}
