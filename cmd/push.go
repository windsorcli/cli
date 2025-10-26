package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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
		// Parse registry, repository name, and tag from positional argument
		if len(args) == 0 {
			return fmt.Errorf("registry is required: windsor push registry/repo[:tag]")
		}

		var registryBase, repoName, tag string
		arg := args[0]

		// Strip oci:// prefix if present
		arg = strings.TrimPrefix(arg, "oci://")

		// First extract tag if present
		if lastColon := strings.LastIndex(arg, ":"); lastColon > 0 && lastColon < len(arg)-1 {
			// Has tag in URL format (registry/repo:tag)
			tag = arg[lastColon+1:]
			arg = arg[:lastColon] // Remove tag from argument
		}

		// Now extract repository name and registry base
		// For URLs like "ghcr.io/windsorcli/core", we want:
		// registryBase = "ghcr.io"
		// repoName = "windsorcli/core"
		if firstSlash := strings.Index(arg, "/"); firstSlash >= 0 {
			registryBase = arg[:firstSlash]
			repoName = arg[firstSlash+1:]
		} else {
			return fmt.Errorf("invalid registry format: must include repository path (e.g., registry.com/namespace/repo)")
		}

		// Get injector from context
		injector := cmd.Context().Value(injectorKey).(di.Injector)

		// Create runtime instance and push artifacts
		if err := runtime.NewRuntime(&runtime.Dependencies{
			Injector: injector,
		}).
			LoadShell().
			ProcessArtifacts(runtime.ArtifactOptions{
				RegistryBase: registryBase,
				RepoName:     repoName,
				Tag:          tag,
				OutputFunc: func(registryURL string) {
					fmt.Printf("Blueprint pushed successfully: %s\n", registryURL)
				},
			}).
			Do(); err != nil {
			if isAuthenticationError(err) {
				fmt.Fprintf(os.Stderr, "Have you run 'docker login %s'?\nSee https://docs.docker.com/engine/reference/commandline/login/ for details.\n", registryBase)
				return fmt.Errorf("Authentication failed")
			}
			return fmt.Errorf("failed to push artifacts: %w", err)
		}

		return nil
	},
}

// isAuthenticationError checks if the error is related to authentication failure
func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Common authentication error patterns
	authErrorPatterns := []string{
		"UNAUTHORIZED",
		"unauthorized",
		"authentication required",
		"authentication failed",
		"not authorized",
		"access denied",
		"login required",
		"credentials required",
		"401",
		"403",
		"unauthenticated",
		"User cannot be authenticated",
		"failed to push artifact",
		"POST https://",
		"blobs/uploads",
	}

	for _, pattern := range authErrorPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
