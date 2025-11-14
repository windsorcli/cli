package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
)

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push [registry/repo:tag]",
	Short: "Push blueprints to an OCI registry",
	Long: `Push your Windsor blueprints to an OCI registry for sharing and deployment.

This command packages your blueprint and pushes it to any OCI-compatible registry
like Docker Hub, GitHub Container Registry, or AWS ECR. The artifacts are compatible
with FluxCD's OCIRepository.

Examples:
  # Push to Docker Hub
  windsor push docker.io/myuser/myblueprint:v1.0.0

  # Push to GitHub Container Registry  
  windsor push ghcr.io/myorg/myblueprint:v1.0.0

  # Push using metadata.yaml for name/version
  windsor push registry.example.com/blueprints`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("registry is required: windsor push registry/repo[:tag]")
		}

		injector := cmd.Context().Value(injectorKey).(di.Injector)

		rt := &runtime.Runtime{
			Injector: injector,
		}

		rt, err := runtime.NewRuntime(rt)
		if err != nil {
			return fmt.Errorf("failed to initialize context: %w", err)
		}

		comp := composer.NewComposer(rt)

		registryURL, err := comp.Push(args[0])
		if err != nil {
			if artifact.IsAuthenticationError(err) {
				registryBase, _, _, parseErr := artifact.ParseRegistryURL(args[0])
				if parseErr == nil {
					fmt.Fprintf(os.Stderr, "Have you run 'docker login %s'?\nSee https://docs.docker.com/engine/reference/commandline/login/ for details.\n", registryBase)
				}
				return fmt.Errorf("Authentication failed")
			}
			return fmt.Errorf("failed to push artifacts: %w", err)
		}

		fmt.Printf("Blueprint pushed successfully: %s\n", registryURL)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
