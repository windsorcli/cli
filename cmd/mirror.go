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
	mirrorConcurrency int
	mirrorTarget      string
	mirrorList        bool
)

// mirrorCmd resolves the blueprint's transitive artifact graph and either
// writes a Talos-compatible image cache to disk (default) or pushes to an
// external OCI registry (--target).
var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Build a Talos image cache or push artifacts to an OCI registry",
	Long: `Build a Talos-compatible image cache from the current blueprint's artifact
graph. The default output is a flat cache directory at .windsor/cache/image-cache
containing blob/ and manifests/ trees that can be fed directly to the Talos
imager (--image-cache flag) for air-gapped boot media.

Alternatively, use --target to push artifacts to an existing OCI registry
for environments with a pre-provisioned mirror.

Use --list to output image refs (one per line) without downloading anything.

Examples:
  # Build Talos image cache (default)
  windsor mirror

  # Push to an external registry
  windsor mirror --target registry.internal:5000

  # Output image refs for external tooling
  windsor mirror --list`,
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

		m := mirror.NewMirror(rt, bp, localManifest, mirror.Options{
			Concurrency: mirrorConcurrency,
			Target:      mirrorTarget,
		})

		if mirrorList {
			plan, err := m.Resolve()
			if err != nil {
				return fmt.Errorf("failed to resolve artifacts: %w", err)
			}
			for _, ref := range plan.Blueprints {
				fmt.Println(ref)
			}
			for _, ref := range plan.DockerImages {
				fmt.Println(ref)
			}
			for _, ref := range plan.HelmOCI {
				fmt.Println(ref)
			}
			for _, h := range plan.HelmHTTPS {
				fmt.Printf("%s:%s\n", h.ChartName, h.Version)
			}
			return nil
		}

		report, err := m.Run()
		if err != nil {
			return fmt.Errorf("failed to mirror artifacts: %w", err)
		}

		if mirrorTarget != "" {
			fmt.Printf("Mirror ready at %s\n", report.Endpoint)
		} else {
			fmt.Printf("Image cache written to %s\n", report.Endpoint)
		}
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
	mirrorCmd.Flags().IntVar(&mirrorConcurrency, "concurrency", 8, "Maximum number of artifacts copied in parallel")
	mirrorCmd.Flags().StringVar(&mirrorTarget, "target", "", "Push to this OCI registry (host[:port][/path]) instead of writing a local image cache")
	mirrorCmd.Flags().BoolVar(&mirrorList, "list", false, "Output image refs (one per line) without downloading")
	rootCmd.AddCommand(mirrorCmd)
}
