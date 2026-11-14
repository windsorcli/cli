package kubernetes_test

import (
	"testing"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFluxKustomization(t *testing.T) {
	resolver, err := paths.New("/project", &v1alpha2.Paths{Base: "infra"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sources := []blueprintv1alpha1.Source{
		{Name: "app", Url: "https://github.com/example/app.git"},
		{Name: "bundle", Url: "oci://registry.example.com/app:v1"},
	}
	build := func(source string, r *paths.Resolver) string {
		k := &blueprintv1alpha1.Kustomization{Name: "ingress", Source: source}
		return kubernetes.FluxKustomization(k, r, "system-gitops", "app", sources, constants.GitopsModePull).Spec.Path
	}

	t.Run("MapsThePathOfTheProjectGitSource", func(t *testing.T) {
		// Given a kustomization on the default git source and a project with moved directories
		// When the Flux Kustomization is built
		got := build("", resolver)

		// Then the path points at the configured kustomize directory
		if got != "infra/kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsTheStandardPathForAnOCISource", func(t *testing.T) {
		// Given a kustomization on an OCI source, which holds the bundle layout
		// When the Flux Kustomization is built
		got := build("bundle", resolver)

		// Then the path keeps the kustomize/ prefix
		if got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsTheStandardPathForAnotherGitSource", func(t *testing.T) {
		// Given a kustomization on a git source other than the project
		// When the Flux Kustomization is built
		got := build("app-remote", resolver)

		// Then the path keeps the kustomize/ prefix
		if got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsThePathWithoutAResolver", func(t *testing.T) {
		if got := build("", nil); got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsThePathForTheDefaultLayout", func(t *testing.T) {
		if got := build("", paths.Default("/project")); got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})
}
