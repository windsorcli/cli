// The install command installs the Flux kustomizations of the blueprint.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// installCmd installs the Flux kustomizations of the blueprint on the cluster.
// It requests only the secrets tools and kubelogin, because it does not run terraform or the container runtime.
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

// init registers the install command with the root command.
func init() {
	addDeprecatedWaitFlag(installCmd)
	rootCmd.AddCommand(installCmd)
}
