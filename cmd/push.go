package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse registry, repository name, and tag from positional argument
		if len(args) == 0 {
			return fmt.Errorf("registry is required: windsor push registry/repo[:tag]")
		}

		var registryBase, repoName, tag string
		arg := args[0]

		// First extract tag if present
		if lastColon := strings.LastIndex(arg, ":"); lastColon > 0 && lastColon < len(arg)-1 {
			// Has tag in URL format (registry/repo:tag)
			tag = arg[lastColon+1:]
			arg = arg[:lastColon] // Remove tag from argument
		}

		// Now extract repository name (last path component) and registry base
		if lastSlash := strings.LastIndex(arg, "/"); lastSlash >= 0 {
			registryBase = arg[:lastSlash]
			repoName = arg[lastSlash+1:]
		} else {
			return fmt.Errorf("invalid registry format: must include repository path (e.g., registry.com/namespace/repo)")
		}

		// Get injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create context with push mode and registry information
		ctx := context.WithValue(context.Background(), "artifactMode", "push")
		ctx = context.WithValue(ctx, "registryBase", registryBase)
		ctx = context.WithValue(ctx, "repoName", repoName)
		ctx = context.WithValue(ctx, "tag", tag)

		// Execute the artifact pipeline
		pipeline, err := pipelines.WithPipeline(injector, ctx, "artifactPipeline")
		if err != nil {
			return fmt.Errorf("failed to set up artifact pipeline: %w", err)
		}
		if err := pipeline.Execute(ctx); err != nil {
			return fmt.Errorf("failed to push artifacts: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
