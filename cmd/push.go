package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	ctrl "github.com/windsorcli/cli/pkg/controller"
)

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push [registry/repo:tag]",
	Short: "Push blueprints to an OCI registry",
	Long: `Push your Windsor blueprints to an OCI registry for sharing and deployment.

This command packages your blueprint and pushes it to any OCI-compatible registry
like Docker Hub, GitHub Container Registry, or AWS ECR. The artifacts are compatible
with FluxCD and other GitOps tools.

Examples:
  # Push to Docker Hub
  windsor push docker.io/myuser/myblueprint:v1.0.0

  # Push to GitHub Container Registry  
  windsor push ghcr.io/myorg/myblueprint:v1.0.0

  # Push using metadata.yaml for name/version
  windsor push registry.example.com/blueprints`,
	RunE: func(cmd *cobra.Command, args []string) error {
		controller := cmd.Context().Value(controllerKey).(ctrl.Controller)

		// Initialize with requirements including bundler functionality
		if err := controller.InitializeWithRequirements(ctrl.Requirements{
			CommandName: cmd.Name(),
			Bundler:     true,
		}); err != nil {
			return fmt.Errorf("failed to initialize controller: %w", err)
		}

		// Resolve artifact builder from controller
		artifact := controller.ResolveArtifactBuilder()
		if artifact == nil {
			return fmt.Errorf("artifact builder not available")
		}

		// Resolve all bundlers and run them
		bundlers := controller.ResolveAllBundlers()
		for _, bundler := range bundlers {
			if err := bundler.Bundle(artifact); err != nil {
				return fmt.Errorf("bundling failed: %w", err)
			}
		}

		// Parse registry and tag from positional argument
		if len(args) == 0 {
			return fmt.Errorf("registry is required: windsor push registry/repo[:tag]")
		}

		var registry, tag string
		arg := args[0]

		if lastColon := strings.LastIndex(arg, ":"); lastColon > 0 && lastColon < len(arg)-1 {
			// Has tag in URL format (registry/repo:tag)
			registry = arg[:lastColon]
			tag = arg[lastColon+1:]
		} else {
			// No tag in URL, registry only
			registry = arg
		}

		// Push the artifact to the registry
		if err := artifact.Push(registry, tag); err != nil {
			return fmt.Errorf("failed to push artifact: %w", err)
		}

		if tag != "" {
			fmt.Printf("Blueprint pushed successfully to %s:%s\n", registry, tag)
		} else {
			fmt.Printf("Blueprint pushed successfully to %s\n", registry)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
