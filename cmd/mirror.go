package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/mirror"
	"github.com/windsorcli/cli/pkg/runtime"
)

var (
	mirrorPort        int
	mirrorConcurrency int
	mirrorTarget      string
)

// mirrorCmd hydrates a local distribution/distribution registry with every
// OCI artifact referenced by the current project's blueprint graph.
var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Hydrate a local OCI mirror registry with the blueprint's artifacts",
	Long: `Hydrate a local OCI mirror registry with every artifact the current blueprint
transitively depends on. This command starts a distribution/distribution
container on localhost:5000, walks the blueprint's oci:// sources recursively,
and copies each reachable blueprint artifact, docker image, and helm chart
into the local registry. The registry's storage is bind-mounted from
.windsor/cache/docker so mirrored content persists across invocations.

Cluster-side image references (e.g. docker.io/library/alpine:3.21) do not
need to change — configure Talos registry mirrors with overridePath: true to
redirect pulls through the local mirror.

Examples:
  # Mirror every artifact for the current blueprint
  windsor mirror`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rtOpts []*runtime.Runtime
		if overridesVal := cmd.Context().Value(runtimeOverridesKey); overridesVal != nil {
			if rt, ok := overridesVal.(*runtime.Runtime); ok {
				rtOpts = []*runtime.Runtime{rt}
			}
		}

		rt := runtime.NewRuntime(rtOpts...)

		if rt.TemplateRoot == "" {
			return fmt.Errorf("no template root configured for current context")
		}
		bp, err := blueprint.LoadBlueprintFile(filepath.Join(rt.TemplateRoot, "blueprint.yaml"))
		if err != nil {
			return fmt.Errorf("failed to load blueprint: %w", err)
		}

		localManifest, err := artifact.ScanProject(rt.ProjectRoot)
		if err != nil {
			return fmt.Errorf("failed to scan project for local artifacts: %w", err)
		}

		target := mirrorTarget
		if target == "" {
			discovered, err := mirror.DiscoverTarget(rt.Shell, rt.ConfigHandler.GetContext())
			if err != nil {
				return fmt.Errorf("failed to discover workstation mirror: %w", err)
			}
			target = discovered
		}

		report, err := mirror.NewMirror(rt, bp, localManifest, mirror.Options{
			HostPort:    mirrorPort,
			Concurrency: mirrorConcurrency,
			Target:      target,
		}).Run()
		if err != nil {
			return fmt.Errorf("failed to mirror artifacts: %w", err)
		}

		fmt.Printf("Mirror ready at %s\n", report.Endpoint)
		fmt.Printf("  Blueprints:    %d\n", report.MirroredBlueprints)
		fmt.Printf("  Docker images: %d\n", report.MirroredDockerImages)
		fmt.Printf("  Helm charts:   %d\n", report.MirroredHelmCharts)
		fmt.Printf("  Already cached: %d (skipped re-download)\n", report.SkippedExisting)
		if len(report.Skipped) > 0 {
			fmt.Fprintf(os.Stderr, "\nSkipped %d non-OCI entries:\n", len(report.Skipped))
			for _, s := range report.Skipped {
				fmt.Fprintf(os.Stderr, "  - %s (%s): %s\n", s.Reference, s.Type, s.Reason)
			}
		}
		return nil
	},
}

func init() {
	mirrorCmd.Flags().IntVar(&mirrorPort, "port", 5000, "Host port to expose the local registry on (ignored when --target is set)")
	mirrorCmd.Flags().IntVar(&mirrorConcurrency, "concurrency", 8, "Maximum number of artifacts copied in parallel")
	mirrorCmd.Flags().StringVar(&mirrorTarget, "target", "", "Push to this OCI registry (host[:port][/path]) instead of starting a local mirror container")
	rootCmd.AddCommand(mirrorCmd)
}
