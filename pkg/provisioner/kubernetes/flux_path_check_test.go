package kubernetes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckKustomizePaths(t *testing.T) {
	newBlueprint := func(ks ...blueprintv1alpha1.Kustomization) *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "app"},
			Sources:        []blueprintv1alpha1.Source{{Name: "remote", Url: "oci://registry.example.com/core:v1"}},
			Kustomizations: ks,
		}
	}

	t.Run("PassesWhenPathAndComponentsExist", func(t *testing.T) {
		// Given a project with kustomize/copia/install/chart
		root := t.TempDir()
		mkdirs(t, root, "kustomize/copia/install/chart")
		bp := newBlueprint(blueprintv1alpha1.Kustomization{Name: "copia-install", Path: "copia/install", Components: []string{"chart", ""}})

		// When the paths are checked
		problems, skipped := kubernetes.CheckKustomizePaths(bp, root, "")

		// Then nothing is reported
		if len(problems) != 0 || len(skipped) != 0 {
			t.Errorf("expected no output, got %v %v", problems, skipped)
		}
	})

	t.Run("ReportsMissingPath", func(t *testing.T) {
		// Given a project with no kustomize directory
		root := t.TempDir()
		bp := newBlueprint(blueprintv1alpha1.Kustomization{Name: "copia-install", Path: "copia/install"})

		// When the paths are checked
		problems, _ := kubernetes.CheckKustomizePaths(bp, root, "")

		// Then the message names the path, the source, the root and the expected location
		if len(problems) != 1 {
			t.Fatalf("expected 1 problem, got %v", problems)
		}
		for _, want := range []string{"kustomize/copia/install", `source "app"`, root, "<project>/kustomize"} {
			if !strings.Contains(problems[0], want) {
				t.Errorf("expected %q in %q", want, problems[0])
			}
		}
	})

	t.Run("ReportsMissingComponent", func(t *testing.T) {
		// Given a kustomization directory without its component directory
		root := t.TempDir()
		mkdirs(t, root, "kustomize/copia/install")
		bp := newBlueprint(blueprintv1alpha1.Kustomization{Name: "copia-install", Path: "copia/install", Components: []string{"chart"}})

		// When the paths are checked
		problems, _ := kubernetes.CheckKustomizePaths(bp, root, "")

		// Then the missing component is reported
		if len(problems) != 1 || !strings.Contains(problems[0], `component "chart"`) {
			t.Errorf("expected a component problem, got %v", problems)
		}
	})

	t.Run("SkipsRemoteSources", func(t *testing.T) {
		// Given a kustomization that reads a remote OCI source
		root := t.TempDir()
		bp := newBlueprint(blueprintv1alpha1.Kustomization{Name: "dns", Path: "dns", Source: "remote"})

		// When the paths are checked
		problems, skipped := kubernetes.CheckKustomizePaths(bp, root, "")

		// Then it reports no problem and names the skip
		if len(problems) != 0 || len(skipped) != 1 {
			t.Errorf("expected one skip and no problem, got %v %v", problems, skipped)
		}
	})

	t.Run("ChecksFluxSystemTiers", func(t *testing.T) {
		// Given a flux system whose install tier has a component
		root := t.TempDir()
		bp := newBlueprint()
		bp.FluxSystems = []blueprintv1alpha1.FluxSystem{{Name: "copia", Path: "copia", Install: &blueprintv1alpha1.Kustomization{Components: []string{"chart"}}}}

		// When the paths are checked
		problems, _ := kubernetes.CheckKustomizePaths(bp, root, "")

		// Then the install tier path is reported
		if len(problems) != 1 || !strings.Contains(problems[0], "kustomize/copia/install") {
			t.Errorf("expected the install tier problem, got %v", problems)
		}
	})

	t.Run("ResolvesAgainstTheBaseDirectory", func(t *testing.T) {
		// Given a project whose base is infra
		root := t.TempDir()
		mkdirs(t, root, "infra/kustomize/dns")
		bp := newBlueprint(blueprintv1alpha1.Kustomization{Name: "dns", Path: "dns"})

		// When the paths are checked with the base prefix
		problems, _ := kubernetes.CheckKustomizePaths(bp, root, "infra")

		// Then the path under the base resolves
		if len(problems) != 0 {
			t.Errorf("expected no problems, got %v", problems)
		}
	})
}

func TestFindMisplacedKustomizeDirs(t *testing.T) {
	t.Run("FindsKustomizeUnderTemplate", func(t *testing.T) {
		// Given manifests under contexts/_template/kustomize
		root := t.TempDir()
		mkdirs(t, root, "contexts/_template/kustomize/copia/install", "kustomize/ok")

		// When the misplaced directories are listed
		got := kubernetes.FindMisplacedKustomizeDirs(root, "")

		// Then only the directory under contexts is returned
		if len(got) != 1 || got[0] != "contexts/_template/kustomize" {
			t.Errorf("unexpected result %v", got)
		}
		if msg := kubernetes.MisplacedKustomizeMessage(got[0], ""); !strings.Contains(msg, "<project>/kustomize") {
			t.Errorf("message lacks expected location: %s", msg)
		}
	})

	t.Run("ReturnsNilWithoutContexts", func(t *testing.T) {
		if got := kubernetes.FindMisplacedKustomizeDirs(t.TempDir(), ""); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}
