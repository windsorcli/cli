//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Test Setup
// =============================================================================

// renderedFluxPatch is one entry of spec.patches in a rendered Flux Kustomization.
type renderedFluxPatch struct {
	Patch  string `yaml:"patch"`
	Target *struct {
		Kind      string `yaml:"kind"`
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"target"`
}

// showKustomizationPatches runs show kustomization for name and returns its spec.patches.
func showKustomizationPatches(t *testing.T, dir string, env []string, name string) []renderedFluxPatch {
	t.Helper()
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", name}, env)
	if err != nil {
		t.Fatalf("show kustomization %s: %v\nstderr: %s", name, err, stderr)
	}
	var k struct {
		Spec struct {
			Patches []renderedFluxPatch `yaml:"patches"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	return k.Spec.Patches
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShowKustomization_InlinesPatchFilesFromPath(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-patch-path")
	env = append(env, "WINDSOR_CONTEXT=default")

	patches := showKustomizationPatches(t, dir, env, "web")

	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d: %+v", len(patches), patches)
	}
	if !strings.Contains(patches[0].Patch, "replicas: 3") {
		t.Errorf("expected the strategic merge file content, got %q", patches[0].Patch)
	}
	if !strings.Contains(patches[1].Patch, "/metadata/labels/tier") || strings.Contains(patches[1].Patch, "kind:") {
		t.Errorf("expected only the JSON 6902 operations, got %q", patches[1].Patch)
	}
	if patches[1].Target == nil || patches[1].Target.Kind != "Deployment" || patches[1].Target.Name != "web" || patches[1].Target.Namespace != "apps" {
		t.Errorf("expected the target from the JSON 6902 file, got %+v", patches[1].Target)
	}
}

func TestShowKustomization_InlinesFluxTierPatchFilesFromPath(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-patch-path")
	env = append(env, "WINDSOR_CONTEXT=default")

	patches := showKustomizationPatches(t, dir, env, "ingress-install")

	if len(patches) != 1 || !strings.Contains(patches[0].Patch, "replicas: 3") {
		t.Errorf("expected the patch file content on the install tier, got %+v", patches)
	}
}

func TestShowKustomization_FailsWhenPatchFileIsMissing(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-patch-path")
	env = append(env, "WINDSOR_CONTEXT=default")
	if err := os.Remove(filepath.Join(dir, "contexts", "_template", "facets", "patches", "web-json6902.yaml")); err != nil {
		t.Fatalf("remove patch file: %v", err)
	}

	_, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "web"}, env)

	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "patches/web-json6902.yaml") {
		t.Errorf("expected stderr to name the patch file, got: %s", stderr)
	}
}
