package blueprint_test

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Setup
// =============================================================================

// strategicMergePatch is a strategic merge patch that sets the replica count of a Deployment.
const strategicMergePatch = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  replicas: 3\n"

// jsonPatchFile is a JSON 6902 patch file. Its kind and metadata name the target.
const jsonPatchFile = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: apps\npatches:\n- op: replace\n  path: /spec/replicas\n  value: 3\n"

// renderedPatches returns the patches of the Flux Kustomization that the CLI renders for name.
func renderedPatches(t *testing.T, bp *blueprintv1alpha1.Blueprint, name string) []string {
	t.Helper()
	k := kustomization(bp, name)
	if k == nil {
		t.Fatalf("expected kustomization %q in the composed blueprint", name)
	}
	flux := k.ToFluxKustomization("system-gitops", bp.Metadata.Name, bp.Sources, constants.GitopsModePull)
	patches := make([]string, 0, len(flux.Spec.Patches))
	for _, p := range flux.Spec.Patches {
		patches = append(patches, p.Patch)
	}
	return patches
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestPatchPath(t *testing.T) {
	t.Run("InlinesAFacetPatchFileRelativeToTheFacet", func(t *testing.T) {
		// Given a facet kustomization with a patch path next to the facet file
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/facets/patches/web.yaml", strategicMergePatch)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/web.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the rendered Flux Kustomization holds the patch file content
		got := renderedPatches(t, bp, "web")
		if len(got) != 1 || got[0] != strategicMergePatch {
			t.Errorf("expected the patch file content, got %q", got)
		}
	})

	t.Run("InlinesABlueprintPatchFileRelativeToTheBlueprint", func(t *testing.T) {
		// Given a template blueprint kustomization with a patch path next to blueprint.yaml
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/patches/web.yaml", strategicMergePatch)
		f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\nkustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/web.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the rendered Flux Kustomization holds the patch file content
		got := renderedPatches(t, bp, "web")
		if len(got) != 1 || got[0] != strategicMergePatch {
			t.Errorf("expected the patch file content, got %q", got)
		}
	})

	t.Run("InlinesAUserBlueprintPatchFileRelativeToTheContext", func(t *testing.T) {
		// Given a template kustomization, and a user blueprint that patches it from the context directory
		f := newFixture(t)
		f.project(t)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n")
		f.write(t, "contexts/local/web-patch.yaml", strategicMergePatch)
		f.write(t, "contexts/local/blueprint.yaml", blueprintHeader+"metadata:\n  name: local\nsources:\n- name: template\n  install: true\nkustomize:\n- name: web\n  patches:\n  - path: web-patch.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the rendered Flux Kustomization holds the context patch file content
		got := renderedPatches(t, bp, "web")
		if len(got) != 1 || got[0] != strategicMergePatch {
			t.Errorf("expected the patch file content, got %q", got)
		}
	})

	t.Run("InlinesAFluxTierPatchFile", func(t *testing.T) {
		// Given a facet flux system whose install tier has a patch path
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/facets/patches/web.yaml", strategicMergePatch)
		f.facet(t, "web", "flux:\n- name: web\n  install:\n    patches:\n    - path: patches/web.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the install tier holds the patch file content
		if len(bp.FluxSystems) != 1 || bp.FluxSystems[0].Install == nil {
			t.Fatalf("expected one flux system with an install tier, got %+v", bp.FluxSystems)
		}
		patches := bp.FluxSystems[0].Install.Patches
		if len(patches) != 1 || patches[0].Patch != strategicMergePatch {
			t.Errorf("expected the patch file content, got %+v", patches)
		}
	})

	t.Run("ParsesAJSON6902PatchFile", func(t *testing.T) {
		// Given a facet kustomization with a JSON 6902 patch file
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/facets/patches/web.yaml", jsonPatchFile)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/web.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the patch holds the operations, and the target comes from the file
		k := kustomization(bp, "web")
		if k == nil || len(k.Patches) != 1 {
			t.Fatalf("expected one patch, got %+v", k)
		}
		p := k.Patches[0]
		if !strings.Contains(p.Patch, "op: replace") || strings.Contains(p.Patch, "kind:") {
			t.Errorf("expected only the patch operations, got %q", p.Patch)
		}
		if p.Target == nil || p.Target.Kind != "Deployment" || p.Target.Name != "web" || p.Target.Namespace != "apps" {
			t.Errorf("expected the target from the patch file, got %+v", p.Target)
		}
	})

	t.Run("KeepsADeclaredTarget", func(t *testing.T) {
		// Given a patch entry with a path and a declared target
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/facets/patches/web.yaml", jsonPatchFile)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/web.yaml\n    target:\n      kind: StatefulSet\n      name: db\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the declared target wins over the target in the file
		p := kustomization(bp, "web").Patches[0]
		if p.Target == nil || p.Target.Kind != "StatefulSet" || p.Target.Name != "db" {
			t.Errorf("expected the declared target, got %+v", p.Target)
		}
	})

	t.Run("ResolvesTheDeclaringSourceForACrossSourcePatch", func(t *testing.T) {
		// Given a core source that defines a kustomization, and a consumer facet that patches it
		f := newFixture(t)
		f.withCoreSource(t)
		f.publish(t, coreURL, map[string]string{
			"_template/blueprint.yaml":    blueprintHeader + "metadata:\n  name: core\n",
			"_template/facets/web.yaml":   facetHeader + "metadata:\n  name: web\nkustomize:\n- name: web\n  path: web\n",
			"_template/facets/patch.yaml": "kind: Wrong\n",
		})
		f.write(t, "contexts/_template/facets/patch.yaml", strategicMergePatch)
		f.facet(t, "consumer", "kustomize:\n- name: web\n  patches:\n  - path: patch.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the patch content comes from the consumer, not from the core source
		got := renderedPatches(t, bp, "web")
		if len(got) != 1 || got[0] != strategicMergePatch {
			t.Errorf("expected the consumer patch file content, got %q", got)
		}
	})

	t.Run("FailsWithThePathWhenTheFileIsMissing", func(t *testing.T) {
		// Given a facet patch path that names no file
		f := newFixture(t)
		f.project(t)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/missing.yaml\n")

		// When the blueprint loads
		_, err := f.load(t)

		// Then the error names the patch file
		if err == nil {
			t.Fatal("expected an error for the missing patch file")
		}
		if !strings.Contains(err.Error(), "patches/missing.yaml") {
			t.Errorf("expected the error to name the patch file, got %v", err)
		}
	})

	t.Run("RemovesAPatchByPathWithoutReadingTheRemoveEntry", func(t *testing.T) {
		// Given a blueprint patch path, and a facet remove entry with the same path and no file
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/_template/patches/web.yaml", strategicMergePatch)
		f.write(t, "contexts/_template/blueprint.yaml", blueprintHeader+"metadata:\n  name: template\nkustomize:\n- name: web\n  path: web\n  patches:\n  - path: patches/web.yaml\n")
		f.facet(t, "drop", "kustomize:\n- name: web\n  strategy: remove\n  patches:\n  - path: patches/web.yaml\n")

		// When the blueprint composes
		bp := f.mustLoad(t).Generate()

		// Then the remove entry deletes the patch, and its missing file causes no error
		if got := renderedPatches(t, bp, "web"); len(got) != 0 {
			t.Errorf("expected no patches, got %q", got)
		}
	})

	t.Run("RejectsAPathOutsideTheSource", func(t *testing.T) {
		// Given a facet patch path that leaves the blueprint source directory
		f := newFixture(t)
		f.project(t)
		f.write(t, "contexts/local/secret.yaml", strategicMergePatch)
		f.facet(t, "web", "kustomize:\n- name: web\n  path: web\n  patches:\n  - path: ../../local/secret.yaml\n")

		// When the blueprint loads
		_, err := f.load(t)

		// Then the load fails and names the path
		if err == nil || !strings.Contains(err.Error(), "../../local/secret.yaml") {
			t.Errorf("expected an error that names the path, got %v", err)
		}
	})
}
