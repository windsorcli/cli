package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
)

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push <registry/repo[:tag]>",
	Short: "Push the blueprint to an OCI registry.",
	Long: `Bundle the current blueprint and push it to an OCI-compatible registry (Docker Hub, GHCR, ECR, etc.). The pushed artifact is consumable by FluxCD's OCIRepository.

The registry argument is required. When the tag is omitted, metadata.yaml ('name', 'version') is used to derive it.

Authentication uses your existing Docker credential helper. If push fails with an auth error, the CLI suggests 'docker login <registry>'.`,
	Example: `# Docker Hub
windsor push docker.io/myorg/myblueprint:v1.0.0

# GitHub Container Registry
windsor push ghcr.io/myorg/myblueprint:v1.0.0

# Tag derived from metadata.yaml
windsor push registry.example.com/myorg/myblueprint`,
	Annotations: map[string]string{
		"docs.seealso": "[Metadata reference](../metadata.md)\n" +
			"[`bundle`](bundle.md)",
		"docs.source": "cmd/push.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return werror.New("registry is required", "Specify the registry in the format registry/repo[:tag].", nil)
		}
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		var compOpts []*composer.Composer
		if overridesVal := cmd.Context().Value(composerOverridesKey); overridesVal != nil {
			if c, ok := overridesVal.(*composer.Composer); ok {
				compOpts = []*composer.Composer{c}
			}
		}

		comp := composer.NewComposer(rt, compOpts...)

		registryURL, err := comp.Push(args[0])
		if err != nil {
			if artifact.IsAuthenticationError(err) {
				hint := "Run docker login for the registry, then run the command again."
				if registryBase, _, _, parseErr := artifact.ParseRegistryURL(args[0]); parseErr == nil {
					hint = fmt.Sprintf("Run docker login %s, then run the command again.", registryBase)
				}
				return werror.New("Authentication failed", hint, err)
			}
			return werror.Wrap(err, "failed to push artifacts")
		}

		fmt.Printf("Blueprint pushed successfully: %s\n", registryURL)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
