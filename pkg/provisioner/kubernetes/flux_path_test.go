package kubernetes_test

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFluxKustomization(t *testing.T) {
	sources := []blueprintv1alpha1.Source{
		{Name: "app", Url: "https://github.com/example/app.git"},
		{Name: "bundle", Url: "oci://registry.example.com/app:v1"},
	}
	build := func(source, basePrefix string) string {
		k := &blueprintv1alpha1.Kustomization{Name: "ingress", Source: source}
		return kubernetes.FluxKustomization(k, basePrefix, "system-gitops", "app", sources, constants.GitopsModePull).Spec.Path
	}

	t.Run("PrefixesThePathOfTheProjectGitSource", func(t *testing.T) {
		// Given a kustomization on the default git source and a project base of infra
		// When the Flux Kustomization is built
		got := build("", "infra")

		// Then the path starts at the base directory
		if got != "infra/kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsThePathForAnOCISource", func(t *testing.T) {
		// Given a kustomization on an OCI source, which holds the bundle layout
		// When the Flux Kustomization is built
		got := build("bundle", "infra")

		// Then the path keeps the kustomize/ prefix
		if got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsThePathForAnotherGitSource", func(t *testing.T) {
		if got := build("app-remote", "infra"); got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("KeepsThePathWithoutABase", func(t *testing.T) {
		if got := build("", ""); got != "kustomize/ingress" {
			t.Fatalf("got %q", got)
		}
	})
}
