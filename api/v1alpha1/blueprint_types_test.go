package v1alpha1

import (
	"strings"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Helpers
// =============================================================================

func boolExprPtr(b bool) *BoolExpression {
	return &BoolExpression{Value: &b, IsExpr: false}
}

func intExprPtr(i int) *IntExpression {
	return &IntExpression{Value: &i, IsExpr: false}
}

// Helper function to check if slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func findKustomization(all []Kustomization, name string) *Kustomization {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

func findKustomizationByName(all []Kustomization, name string) *Kustomization {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

func kustomizationNames(all []Kustomization) []string {
	names := make([]string, len(all))
	for i, k := range all {
		names[i] = k.Name
	}
	return names
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBlueprint_CrdsRenderAsScalarList(t *testing.T) {
	t.Run("MarshalsAsFlatScalarList", func(t *testing.T) {
		// Given a blueprint whose crds: is a flat list of local refs
		bp := &Blueprint{
			Kind: "Blueprint",
			Crds: []string{"gateway-api-experimental-1.5.1", "envoy-gateway-1.7.1"},
			Kustomizations: []Kustomization{
				{Name: "cert-manager", Path: "pki/cert-manager", DependsOn: []string{"crds"}},
			},
		}

		// When marshaled
		out, err := yaml.Marshal(bp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		// Then crds: renders as a bare scalar list — no source/refs objects, no kustomization fields
		text := string(out)
		if !strings.Contains(text, "- gateway-api-experimental-1.5.1") {
			t.Errorf("expected crds: as a flat scalar list, got:\n%s", text)
		}
		if strings.Contains(text, "source:") || strings.Contains(text, "refs:") || strings.Contains(text, "prune:") {
			t.Errorf("did not expect source/refs/prune in crds output, got:\n%s", text)
		}
	})

	t.Run("UnmarshalsFlatScalarList", func(t *testing.T) {
		// Given the natural flat form facets and test assertions author
		const doc = `
kind: Blueprint
crds:
  - gateway-api-experimental-1.5.1
  - envoy-gateway-1.7.1
`
		// When unmarshaled
		var bp Blueprint
		if err := yaml.Unmarshal([]byte(doc), &bp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then the bare refs land in Crds without forcing the verbose refs: nesting
		if len(bp.Crds) != 2 || bp.Crds[0] != "gateway-api-experimental-1.5.1" || bp.Crds[1] != "envoy-gateway-1.7.1" {
			t.Errorf("expected flat refs, got %+v", bp.Crds)
		}
	})
}

func TestSource_CrdsRoundTrip(t *testing.T) {
	t.Run("ParsesAndRendersScalarCrdsUnderSource", func(t *testing.T) {
		// Given a source that carries its own flat CRD list (installed in the background when install:true)
		const doc = `
name: core
url: oci://example.com/core
install: true
crds:
  - cert-manager-1.16.2
`
		// When unmarshaled then marshaled
		var src Source
		if err := yaml.Unmarshal([]byte(doc), &src); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(src.Crds) != 1 || src.Crds[0] != "cert-manager-1.16.2" {
			t.Fatalf("expected source to carry a scalar crds list, got %+v", src.Crds)
		}
		out, err := yaml.Marshal(&src)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(out), "- cert-manager-1.16.2") {
			t.Errorf("expected source crds rendered as a scalar list, got:\n%s", string(out))
		}
	})
}

func TestCrdKustomizationName(t *testing.T) {
	t.Run("DefaultSourceTakesBaseName", func(t *testing.T) {
		// Given the default/project source (empty name)
		if got := CrdKustomizationName(""); got != "crds" {
			t.Errorf("expected crds, got %q", got)
		}
	})

	t.Run("NamedSourceIsSuffixed", func(t *testing.T) {
		// Given a named source
		if got := CrdKustomizationName("core"); got != "crds-core" {
			t.Errorf("expected crds-core, got %q", got)
		}
	})
}

func TestIsCrdLayerName(t *testing.T) {
	t.Run("MatchesBaseAndNamespacedNames", func(t *testing.T) {
		// Given the synthesized CRD layer namespace
		for _, name := range []string{"crds", "crds-core", "crds-acme-oci"} {
			if !IsCrdLayerName(name) {
				t.Errorf("expected %q to be a CRD layer name", name)
			}
		}
		// And unrelated kustomization names are not in the namespace
		for _, name := range []string{"cert-manager", "crd", "crdsuffix"} {
			if IsCrdLayerName(name) {
				t.Errorf("did not expect %q to be a CRD layer name", name)
			}
		}
	})
}

func TestIsLocalTemplateSource(t *testing.T) {
	t.Run("ReturnsFalseForNonTemplateSource", func(t *testing.T) {
		source := Source{Name: "other", Url: ""}
		if IsLocalTemplateSource(source) {
			t.Error("Expected false for non-template source")
		}
	})

	t.Run("ReturnsFalseForTemplateWithURL", func(t *testing.T) {
		source := Source{Name: "template", Url: "https://github.com/example/repo.git"}
		if IsLocalTemplateSource(source) {
			t.Error("Expected false for template source with URL")
		}
	})

	t.Run("ReturnsTrueForTemplateWithEmptyURL", func(t *testing.T) {
		source := Source{Name: "template", Url: ""}
		if !IsLocalTemplateSource(source) {
			t.Error("Expected true for template source with empty URL")
		}
	})
}

func TestHasRemoteTemplateSource(t *testing.T) {
	t.Run("ReturnsFalseForEmptySources", func(t *testing.T) {
		if HasRemoteTemplateSource([]Source{}) {
			t.Error("Expected false for empty sources")
		}
	})

	t.Run("ReturnsFalseWhenOnlyLocalTemplate", func(t *testing.T) {
		sources := []Source{{Name: "template", Url: ""}}
		if HasRemoteTemplateSource(sources) {
			t.Error("Expected false when only local template")
		}
	})

	t.Run("ReturnsTrueWhenTemplateHasURL", func(t *testing.T) {
		sources := []Source{
			{Name: "template", Url: "https://github.com/example/repo.git"},
		}
		if !HasRemoteTemplateSource(sources) {
			t.Error("Expected true when template source has URL")
		}
	})

	t.Run("ReturnsTrueWhenMultipleSourcesIncludeRemoteTemplate", func(t *testing.T) {
		sources := []Source{
			{Name: "other", Url: "https://other.com/repo.git"},
			{Name: "template", Url: "https://github.com/example/repo.git"},
		}
		if !HasRemoteTemplateSource(sources) {
			t.Error("Expected true when sources include remote template")
		}
	})
}

func TestBlueprint_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		blueprint := &Blueprint{
			Metadata: Metadata{
				Name: "test-blueprint",
			},
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "https://example.com/repo1.git",
					PathPrefix: "terraform",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Inputs: map[string]any{
						"key1": "value1",
					},
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
				},
			},
		}
		copy := blueprint.DeepCopy()
		if copy.Metadata.Name != "test-blueprint" {
			t.Errorf("Expected copy to have name %v, but got %v", "test-blueprint", copy.Metadata.Name)
		}
		if len(copy.Sources) != 1 || copy.Sources[0].Name != "source1" {
			t.Errorf("Expected copy to have source %v, but got %v", "source1", copy.Sources)
		}
		if len(copy.TerraformComponents) != 1 || copy.TerraformComponents[0].Source != "source1" {
			t.Errorf("Expected copy to have terraform component source %v, but got %v", "source1", copy.TerraformComponents)
		}
		if copy.TerraformComponents[0].Path != "module/path1" {
			t.Errorf("Expected copy to have terraform component path %v, but got %v", "module/path1", copy.TerraformComponents[0].Path)
		}
		if len(copy.TerraformComponents[0].Inputs) != 1 || copy.TerraformComponents[0].Inputs["key1"] != "value1" {
			t.Errorf("Expected copy to have terraform component input 'key1' with value 'value1', but got %v", copy.TerraformComponents[0].Inputs)
		}
		if len(copy.Kustomizations) != 1 || copy.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected copy to have kustomization 'kustomization1', but got %v", copy.Kustomizations)
		}
		if len(copy.Kustomizations[0].Components) != 1 || copy.Kustomizations[0].Components[0] != "component1" {
			t.Errorf("Expected copy to have kustomization component 'component1', but got %v", copy.Kustomizations[0].Components)
		}
	})

	t.Run("EmptyBlueprint", func(t *testing.T) {
		var blueprint *Blueprint
		copy := blueprint.DeepCopy()
		if copy != nil {
			t.Errorf("Expected copy to be nil, but got non-nil")
		}
	})

	t.Run("PreservesBackend", func(t *testing.T) {
		// Given a blueprint with a declared backend terminus
		blueprint := &Blueprint{Backend: "cluster"}

		// When deep-copied
		copy := blueprint.DeepCopy()

		// Then the Backend field round-trips
		if copy.Backend != "cluster" {
			t.Errorf("Expected copy.Backend=\"cluster\", got %q", copy.Backend)
		}
	})

	t.Run("DeepCopiesConfigIndependently", func(t *testing.T) {
		// Given a blueprint with a nested config scope
		blueprint := &Blueprint{
			Config: map[string]any{
				"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
			},
		}

		// When deep-copied and the original is mutated
		copy := blueprint.DeepCopy()
		blueprint.Config["cluster"].(map[string]any)["controlplanes"].(map[string]any)["cpu"] = 99

		// Then the copy keeps its own value, unaffected by the mutation
		copiedCPU := copy.Config["cluster"].(map[string]any)["controlplanes"].(map[string]any)["cpu"]
		if copiedCPU != 7 {
			t.Errorf("Expected copy.Config cpu=7, got %v", copiedCPU)
		}
	})
}

func TestBlueprint_DeepCopy_WithNewFields(t *testing.T) {
	t.Run("CopiesSourceInstallField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			Sources: []Source{
				{
					Name:    "test-source",
					Url:     "oci://example.com/repo:tag",
					Install: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.Sources) != 1 {
			t.Fatalf("Expected 1 source, got %d", len(copy.Sources))
		}
		if copy.Sources[0].Install == nil {
			t.Fatal("Expected Install to be copied")
		}
		if copy.Sources[0].Install.Value == nil || *copy.Sources[0].Install.Value {
			t.Error("Expected Install to be false in copy")
		}
	})

	t.Run("CopiesCrdsDeeply", func(t *testing.T) {
		blueprint := &Blueprint{
			Crds:    []string{"cert-manager-1.16.2"},
			Sources: []Source{{Name: "core", Crds: []string{"gateway-api-1.5.1"}}},
		}

		copy := blueprint.DeepCopy()
		if len(copy.Crds) != 1 || copy.Crds[0] != "cert-manager-1.16.2" {
			t.Fatalf("Expected local crds copied, got %+v", copy.Crds)
		}
		if len(copy.Sources) != 1 || len(copy.Sources[0].Crds) != 1 || copy.Sources[0].Crds[0] != "gateway-api-1.5.1" {
			t.Fatalf("Expected source crds copied, got %+v", copy.Sources)
		}
		// Mutating the copy must not touch the original (deep copy, not aliased)
		copy.Crds[0] = "mutated"
		copy.Sources[0].Crds[0] = "mutated"
		if blueprint.Crds[0] != "cert-manager-1.16.2" {
			t.Error("Expected original local crds untouched after mutating copy")
		}
		if blueprint.Sources[0].Crds[0] != "gateway-api-1.5.1" {
			t.Error("Expected original source crds untouched after mutating copy")
		}
	})

	t.Run("CopiesFluxSystemsDeeply", func(t *testing.T) {
		blueprint := &Blueprint{
			FluxSystems: []FluxSystem{
				{
					Name:      "gateway",
					DependsOn: []string{"pki-install"},
					Install:   &Kustomization{Components: []string{"envoy"}},
					Resources: []FluxVariant{
						{Kustomization: Kustomization{Name: "internal", Components: []string{"internal"}}},
					},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.FluxSystems) != 1 {
			t.Fatalf("Expected 1 flux system, got %d", len(copy.FluxSystems))
		}
		sys := copy.FluxSystems[0]
		if sys.Install == nil || len(sys.Install.Components) != 1 || sys.Install.Components[0] != "envoy" {
			t.Fatalf("Expected install copied, got %+v", sys.Install)
		}
		if len(sys.Resources) != 1 || sys.Resources[0].Name != "internal" {
			t.Fatalf("Expected resources copied, got %+v", sys.Resources)
		}

		// Mutating the copy must not touch the original (deep copy, not aliased)
		copy.FluxSystems[0].DependsOn[0] = "mutated"
		copy.FluxSystems[0].Install.Components[0] = "mutated"
		copy.FluxSystems[0].Resources[0].Components[0] = "mutated"
		if blueprint.FluxSystems[0].DependsOn[0] != "pki-install" {
			t.Error("Expected original dependsOn untouched after mutating copy")
		}
		if blueprint.FluxSystems[0].Install.Components[0] != "envoy" {
			t.Error("Expected original install untouched after mutating copy")
		}
		if blueprint.FluxSystems[0].Resources[0].Components[0] != "internal" {
			t.Error("Expected original resources untouched after mutating copy")
		}
	})

	t.Run("CopiesTerraformComponentEnabledField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:    "test/path",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(copy.TerraformComponents))
		}
		if copy.TerraformComponents[0].Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.TerraformComponents[0].Enabled.Value == nil || *copy.TerraformComponents[0].Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}
	})

	t.Run("CopiesKustomizationEnabledField", func(t *testing.T) {
		falseVal := false
		blueprint := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test-kustomization",
					Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		copy := blueprint.DeepCopy()
		if len(copy.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(copy.Kustomizations))
		}
		if copy.Kustomizations[0].Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Kustomizations[0].Enabled.Value == nil || *copy.Kustomizations[0].Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}
	})

	t.Run("CopiesTopLevelSubstitutions", func(t *testing.T) {
		// Given a blueprint with top-level Substitutions
		blueprint := &Blueprint{
			Substitutions: map[string]string{
				"private_dns": "10.0.0.1",
				"public_dns":  "8.8.8.8",
			},
		}

		// When deep copying
		copy := blueprint.DeepCopy()

		// Then the copy should have the same values
		if copy.Substitutions == nil {
			t.Fatal("Expected Substitutions to be copied")
		}
		if copy.Substitutions["private_dns"] != "10.0.0.1" {
			t.Errorf("Expected private_dns='10.0.0.1', got '%s'", copy.Substitutions["private_dns"])
		}

		// And mutating the copy must not affect the original
		copy.Substitutions["private_dns"] = "mutated"
		if blueprint.Substitutions["private_dns"] != "10.0.0.1" {
			t.Error("Deep copy failed: mutating copy affected original Substitutions map")
		}
	})
}

func TestTerraformComponent_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		component := &TerraformComponent{
			Source:      "test-source",
			Path:        "test/path",
			FullPath:    "/full/test/path",
			DependsOn:   []string{"dep1", "dep2"},
			Inputs:      map[string]any{"key1": "value1", "key2": 42},
			Destroy:     boolExprPtr(true),
			Parallelism: intExprPtr(3),
		}

		copy := component.DeepCopy()

		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.FullPath != "/full/test/path" {
			t.Errorf("Expected full path '/full/test/path', got '%s'", copy.FullPath)
		}
		if len(copy.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(copy.DependsOn))
		}
		if copy.DependsOn[0] != "dep1" || copy.DependsOn[1] != "dep2" {
			t.Errorf("Expected dependencies ['dep1', 'dep2'], got %v", copy.DependsOn)
		}
		if len(copy.Inputs) != 2 {
			t.Errorf("Expected 2 inputs, got %d", len(copy.Inputs))
		}
		if copy.Inputs["key1"] != "value1" {
			t.Errorf("Expected input key1='value1', got '%v'", copy.Inputs["key1"])
		}
		if copy.Inputs["key2"] != 42 {
			t.Errorf("Expected input key2=42, got '%v'", copy.Inputs["key2"])
		}
		destroy := copy.Destroy.ToBool()
		if destroy == nil || *destroy != true {
			t.Errorf("Expected destroy=true, got %v", destroy)
		}
		parallelism := copy.Parallelism.ToInt()
		if parallelism == nil || *parallelism != 3 {
			t.Errorf("Expected parallelism=3, got %v", parallelism)
		}

		// Verify it's a deep copy (modifying copy shouldn't affect original)
		copy.Inputs["key3"] = "new-value"
		if _, exists := component.Inputs["key3"]; exists {
			t.Error("Expected modifying copy inputs not to affect original")
		}
		copy.DependsOn = append(copy.DependsOn, "dep3")
		if len(component.DependsOn) != 2 {
			t.Error("Expected modifying copy dependencies not to affect original")
		}
	})

	t.Run("NilComponent", func(t *testing.T) {
		var component *TerraformComponent
		copy := component.DeepCopy()
		if copy != nil {
			t.Errorf("Expected nil copy, got non-nil")
		}
	})

	t.Run("EmptyFields", func(t *testing.T) {
		component := &TerraformComponent{
			Source: "test-source",
			Path:   "test/path",
		}

		copy := component.DeepCopy()

		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if len(copy.DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", copy.DependsOn)
		}
		if len(copy.Inputs) != 0 {
			t.Errorf("Expected empty inputs, got %v", copy.Inputs)
		}
		if copy.Destroy != nil {
			t.Errorf("Expected nil destroy, got %v", copy.Destroy)
		}
		if copy.Parallelism != nil {
			t.Errorf("Expected nil parallelism, got %v", copy.Parallelism)
		}
	})

	t.Run("IncludesNameInDeepCopy", func(t *testing.T) {
		component := &TerraformComponent{
			Name:   "test-component",
			Source: "test-source",
			Path:   "test/path",
		}

		copy := component.DeepCopy()

		if copy.Name != "test-component" {
			t.Errorf("Expected name 'test-component', got '%s'", copy.Name)
		}
	})
}

func TestTerraformComponent_GetID(t *testing.T) {
	t.Run("ReturnsNameWhenPresent", func(t *testing.T) {
		component := &TerraformComponent{
			Name: "my-component",
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "my-component" {
			t.Errorf("Expected ID 'my-component', got '%s'", id)
		}
	})

	t.Run("ReturnsPathWhenNameNotPresent", func(t *testing.T) {
		component := &TerraformComponent{
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "terraform/path" {
			t.Errorf("Expected ID 'terraform/path', got '%s'", id)
		}
	})

	t.Run("ReturnsPathWhenNameIsEmpty", func(t *testing.T) {
		component := &TerraformComponent{
			Name: "",
			Path: "terraform/path",
		}

		id := component.GetID()

		if id != "terraform/path" {
			t.Errorf("Expected ID 'terraform/path', got '%s'", id)
		}
	})
}

func TestBlueprint_BackendComponentID(t *testing.T) {
	t.Run("ReturnsBackendFieldWhenSet", func(t *testing.T) {
		// Given Blueprint.Backend names a component in TerraformComponents
		bp := &Blueprint{
			Backend: "cluster",
			TerraformComponents: []TerraformComponent{
				{Path: "vpc"},
				{Name: "cluster", Path: "cluster/eks"},
			},
		}
		if got := bp.BackendComponentID(); got != "cluster" {
			t.Errorf("Expected \"cluster\", got %q", got)
		}
	})

	t.Run("ReturnsEmptyWhenBackendUnset", func(t *testing.T) {
		// Given Blueprint.Backend is not set, even when a component happens to be named "backend"
		bp := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "backend"},
				{Path: "vpc"},
			},
		}
		if got := bp.BackendComponentID(); got != "" {
			t.Errorf("Expected empty (naming convention is no longer recognised), got %q", got)
		}
	})

	t.Run("ReturnsEmptyWhenNilBlueprint", func(t *testing.T) {
		var bp *Blueprint
		if got := bp.BackendComponentID(); got != "" {
			t.Errorf("Expected empty string, got %q", got)
		}
	})

	t.Run("DoesNotValidateExistence", func(t *testing.T) {
		// BackendComponentID is a pure accessor and returns whatever Blueprint.Backend
		// holds even when it does not resolve to a declared component. The composer
		// validation layer is responsible for rejecting unresolved references at
		// blueprint load.
		bp := &Blueprint{Backend: "ghost"}
		if got := bp.BackendComponentID(); got != "ghost" {
			t.Errorf("Expected \"ghost\" (unvalidated passthrough), got %q", got)
		}
	})
}

func TestBlueprint_BackendComponents(t *testing.T) {
	t.Run("ReturnsTerminusPlusAllPriorComponents", func(t *testing.T) {
		// Given a blueprint with backend components (vpc, iam, cluster) and a non-backend one (workloads)
		bp := &Blueprint{
			Backend: "cluster",
			TerraformComponents: []TerraformComponent{
				{Path: "vpc"},
				{Path: "iam"},
				{Name: "cluster", Path: "cluster/eks"},
				{Path: "workloads"},
			},
		}

		components := bp.BackendComponents()

		if len(components) != 3 {
			t.Fatalf("Expected 3 backend components, got %d", len(components))
		}
		for i, expected := range []string{"vpc", "iam", "cluster"} {
			if components[i].GetID() != expected {
				t.Errorf("Expected components[%d]=%q, got %q", i, expected, components[i].GetID())
			}
		}
	})

	t.Run("ReturnsSingleComponentWhenBackendIsFirst", func(t *testing.T) {
		bp := &Blueprint{
			Backend: "backend",
			TerraformComponents: []TerraformComponent{
				{Path: "backend"},
				{Path: "cluster"},
			},
		}

		components := bp.BackendComponents()

		if len(components) != 1 || components[0].GetID() != "backend" {
			t.Errorf("Expected single-element result [backend], got %+v", components)
		}
	})

	t.Run("ReturnsNilWhenBackendUnset", func(t *testing.T) {
		bp := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "vpc"}, {Path: "cluster"}},
		}

		if components := bp.BackendComponents(); components != nil {
			t.Errorf("Expected nil when Backend is unset, got %+v", components)
		}
	})

	t.Run("ReturnsNilWhenBackendDoesNotResolve", func(t *testing.T) {
		// Validation should reject this at load time, but BackendComponents returns nil
		// when the named component is not found so a callsite that runs without
		// validation does not panic.
		bp := &Blueprint{
			Backend:             "ghost",
			TerraformComponents: []TerraformComponent{{Path: "vpc"}, {Path: "cluster"}},
		}

		if components := bp.BackendComponents(); components != nil {
			t.Errorf("Expected nil when Backend does not resolve, got %+v", components)
		}
	})

	t.Run("ReturnsNilWhenNilBlueprint", func(t *testing.T) {
		var bp *Blueprint
		if components := bp.BackendComponents(); components != nil {
			t.Errorf("Expected nil for nil blueprint, got %+v", components)
		}
	})

	t.Run("MembersPointIntoBlueprintSlice", func(t *testing.T) {
		// The returned slice holds pointers into Blueprint.TerraformComponents so callers
		// can mutate component state in place if needed (e.g., for in-flight orchestration
		// bookkeeping). Verify the addresses match.
		bp := &Blueprint{
			Backend: "cluster",
			TerraformComponents: []TerraformComponent{
				{Path: "vpc"},
				{Name: "cluster", Path: "cluster/eks"},
			},
		}

		components := bp.BackendComponents()

		if &bp.TerraformComponents[0] != components[0] || &bp.TerraformComponents[1] != components[1] {
			t.Error("Expected entries to be pointers into Blueprint.TerraformComponents")
		}
	})
}

func TestBlueprint_IsBackendComponent(t *testing.T) {
	bp := &Blueprint{
		Backend: "cluster",
		TerraformComponents: []TerraformComponent{
			{Path: "vpc"},
			{Path: "iam"},
			{Name: "cluster", Path: "cluster/eks"},
			{Path: "workloads"},
		},
	}

	t.Run("ReportsTrueForTerminusComponent", func(t *testing.T) {
		if !bp.IsBackendComponent("cluster") {
			t.Error("Expected cluster (terminus) to be a backend component")
		}
	})

	t.Run("ReportsTrueForComponentBeforeTerminus", func(t *testing.T) {
		for _, id := range []string{"vpc", "iam"} {
			if !bp.IsBackendComponent(id) {
				t.Errorf("Expected %q (declared before terminus) to be a backend component", id)
			}
		}
	})

	t.Run("ReportsFalseForComponentAfterTerminus", func(t *testing.T) {
		if bp.IsBackendComponent("workloads") {
			t.Error("Expected workloads (declared after terminus) to be non-backend")
		}
	})

	t.Run("ReportsFalseForUnknownComponent", func(t *testing.T) {
		if bp.IsBackendComponent("ghost") {
			t.Error("Expected unknown component to be non-backend")
		}
	})

	t.Run("ReportsFalseForEmptyID", func(t *testing.T) {
		if bp.IsBackendComponent("") {
			t.Error("Expected empty ID to be non-backend")
		}
	})

	t.Run("ReportsFalseWhenBackendUnset", func(t *testing.T) {
		external := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "vpc"}, {Path: "cluster"}},
		}
		if external.IsBackendComponent("vpc") {
			t.Error("Expected non-backend membership when Backend is unset (external backend case)")
		}
	})
}

func TestBlueprint_BackendYAMLRoundTrip(t *testing.T) {
	t.Run("UnmarshalsBackendField", func(t *testing.T) {
		yamlData := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-eks-prod
backend: cluster
terraform:
  - path: networking/vpc
  - name: cluster
    path: cluster/eks
  - path: workloads/argocd
`)

		var bp Blueprint
		if err := yaml.Unmarshal(yamlData, &bp); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if bp.Backend != "cluster" {
			t.Errorf("Expected Backend=\"cluster\", got %q", bp.Backend)
		}
		if id := bp.BackendComponentID(); id != "cluster" {
			t.Errorf("Expected BackendComponentID=\"cluster\", got %q", id)
		}
		components := bp.BackendComponents()
		if len(components) != 2 {
			t.Fatalf("Expected 2 backend components (vpc + cluster), got %d", len(components))
		}
		if components[0].Path != "networking/vpc" || components[1].GetID() != "cluster" {
			t.Errorf("Expected components=[networking/vpc, cluster], got [%s, %s]", components[0].Path, components[1].GetID())
		}
	})

	t.Run("MarshalsBackendField", func(t *testing.T) {
		bp := Blueprint{Backend: "cluster"}
		out, err := yaml.Marshal(&bp)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if !strings.Contains(string(out), "backend: cluster") {
			t.Errorf("Expected YAML to contain \"backend: cluster\", got:\n%s", string(out))
		}
	})

	t.Run("OmitsBackendFieldWhenEmpty", func(t *testing.T) {
		bp := Blueprint{}
		out, err := yaml.Marshal(&bp)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if strings.Contains(string(out), "backend:") {
			t.Errorf("Expected empty Backend to be omitted, got:\n%s", string(out))
		}
	})
}

// The `flux:` key is a preferred alias of `kustomize:` on the blueprint, mirroring Facet.
// flux: on a blueprint is a distinct FluxSystems collection, separate from kustomize:.
func TestBlueprint_FluxSystems(t *testing.T) {
	t.Run("FluxKeyParsesSystemEntriesSeparateFromKustomize", func(t *testing.T) {
		src := `kind: Blueprint
kustomize:
  - name: plain
    path: plain
flux:
  - name: dns
    install:
      components: [coredns-controller]
    resources:
      - components: [coredns, external-dns]
`
		var bp Blueprint
		if err := yaml.Unmarshal([]byte(src), &bp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(bp.Kustomizations) != 1 || bp.Kustomizations[0].Name != "plain" {
			t.Fatalf("kustomize entry wrong: %+v", bp.Kustomizations)
		}
		if len(bp.FluxSystems) != 1 || bp.FluxSystems[0].Name != "dns" {
			t.Fatalf("expected one flux system dns, got %+v", bp.FluxSystems)
		}
		if bp.FluxSystems[0].Install == nil || bp.FluxSystems[0].Install.Components[0] != "coredns-controller" {
			t.Fatalf("install wrong: %+v", bp.FluxSystems[0].Install)
		}
		if len(bp.FluxSystems[0].Resources) != 1 {
			t.Fatalf("expected one resources variant, got %+v", bp.FluxSystems[0].Resources)
		}
	})
}

func TestKustomization_DeepCopy(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		interval := DurationString{Duration: 5 * time.Minute}
		retryInterval := DurationString{Duration: 2 * time.Minute}
		timeout := DurationString{Duration: 10 * time.Minute}
		deleteTimeout := DurationString{Duration: 45 * time.Minute}
		wait := true
		force := false
		prune := true
		destroy := false
		destroyOnly := true

		kustomization := &Kustomization{
			Name:            "test-kustomization",
			Path:            "test/path",
			Source:          "test-source",
			Namespace:       "custom-ns",
			TargetNamespace: "workload-ns",
			DependsOn:       []string{"dep1", "dep2"},
			Interval:        &interval,
			RetryInterval:   &retryInterval,
			Timeout:         &timeout,
			DeleteTimeout:   &deleteTimeout,
			Patches: []BlueprintPatch{
				{
					Patch: "test-patch",
					Target: &kustomize.Selector{
						Kind: "Service",
						Name: "test",
					},
				},
			},
			Wait:          &wait,
			Force:         &force,
			Prune:         &prune,
			Components:    []string{"comp1", "comp2"},
			Destroy:       boolExprPtr(destroy),
			DestroyOnly:   &destroyOnly,
			Substitutions: map[string]string{"key1": "value1", "key2": "value2"},
		}

		copy := kustomization.DeepCopy()

		if copy.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", copy.Name)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if copy.Namespace != "custom-ns" {
			t.Errorf("Expected namespace 'custom-ns', got '%s'", copy.Namespace)
		}
		if copy.TargetNamespace != "workload-ns" {
			t.Errorf("Expected targetNamespace 'workload-ns', got '%s'", copy.TargetNamespace)
		}
		if len(copy.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(copy.DependsOn))
		}
		if copy.DependsOn[0] != "dep1" || copy.DependsOn[1] != "dep2" {
			t.Errorf("Expected dependencies ['dep1', 'dep2'], got %v", copy.DependsOn)
		}
		if copy.Interval == nil || copy.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected interval 5m, got %v", copy.Interval)
		}
		if copy.RetryInterval == nil || copy.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected retry interval 2m, got %v", copy.RetryInterval)
		}
		if copy.Timeout == nil || copy.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected timeout 10m, got %v", copy.Timeout)
		}
		if copy.DeleteTimeout == nil || copy.DeleteTimeout.Duration != 45*time.Minute {
			t.Errorf("Expected delete timeout 45m, got %v", copy.DeleteTimeout)
		}
		if len(copy.Patches) != 1 {
			t.Errorf("Expected 1 patch, got %d", len(copy.Patches))
		}
		if copy.Patches[0].Patch != "test-patch" {
			t.Errorf("Expected patch 'test-patch', got '%s'", copy.Patches[0].Patch)
		}
		if copy.Wait == nil || *copy.Wait != true {
			t.Errorf("Expected wait=true, got %v", copy.Wait)
		}
		if copy.Force == nil || *copy.Force != false {
			t.Errorf("Expected force=false, got %v", copy.Force)
		}
		if copy.Prune == nil || *copy.Prune != true {
			t.Errorf("Expected prune=true, got %v", copy.Prune)
		}
		if len(copy.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(copy.Components))
		}
		if copy.Components[0] != "comp1" || copy.Components[1] != "comp2" {
			t.Errorf("Expected components ['comp1', 'comp2'], got %v", copy.Components)
		}
		destroyVal := copy.Destroy.ToBool()
		if destroyVal == nil || *destroyVal != false {
			t.Errorf("Expected destroy=false, got %v", destroyVal)
		}
		if copy.DestroyOnly == nil || *copy.DestroyOnly != true {
			t.Errorf("Expected destroyOnly=true, got %v", copy.DestroyOnly)
		}
		if len(copy.Substitutions) != 2 {
			t.Errorf("Expected 2 substitutions, got %d", len(copy.Substitutions))
		}
		if copy.Substitutions["key1"] != "value1" || copy.Substitutions["key2"] != "value2" {
			t.Errorf("Expected substitutions map[key1:value1 key2:value2], got %v", copy.Substitutions)
		}

		// Verify it's a deep copy (modifying copy shouldn't affect original)
		copy.DependsOn = append(copy.DependsOn, "dep3")
		if len(kustomization.DependsOn) != 2 {
			t.Error("Expected modifying copy dependencies not to affect original")
		}
		copy.Components = append(copy.Components, "comp3")
		if len(kustomization.Components) != 2 {
			t.Error("Expected modifying copy components not to affect original")
		}
		copy.Substitutions["key3"] = "value3"
		if _, exists := kustomization.Substitutions["key3"]; exists {
			t.Error("Expected modifying copy substitutions not to affect original")
		}
	})

	t.Run("NilKustomization", func(t *testing.T) {
		var kustomization *Kustomization
		copy := kustomization.DeepCopy()
		if copy != nil {
			t.Errorf("Expected nil copy, got non-nil")
		}
	})

	t.Run("EmptyFields", func(t *testing.T) {
		kustomization := &Kustomization{
			Name:   "test-kustomization",
			Path:   "test/path",
			Source: "test-source",
		}

		copy := kustomization.DeepCopy()

		if copy.Name != "test-kustomization" {
			t.Errorf("Expected name 'test-kustomization', got '%s'", copy.Name)
		}
		if copy.Path != "test/path" {
			t.Errorf("Expected path 'test/path', got '%s'", copy.Path)
		}
		if copy.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", copy.Source)
		}
		if len(copy.DependsOn) != 0 {
			t.Errorf("Expected empty dependsOn, got %v", copy.DependsOn)
		}
		if len(copy.Patches) != 0 {
			t.Errorf("Expected empty patches, got %v", copy.Patches)
		}
		if len(copy.Components) != 0 {
			t.Errorf("Expected empty components, got %v", copy.Components)
		}
		if len(copy.Substitutions) != 0 {
			t.Errorf("Expected empty substitutions, got %v", copy.Substitutions)
		}
	})
}

func TestKustomization_UnmarshalYAML(t *testing.T) {
	t.Run("UnmarshalsDurationStrings", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
path: test/path
interval: 5m
retryInterval: 2m
timeout: 10m
deleteTimeout: 45m
`)

		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)
		if err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with duration strings: %v", err)
		}

		if k.Name != "test-kustomization" {
			t.Errorf("Expected Name 'test-kustomization', got %q", k.Name)
		}
		if k.Path != "test/path" {
			t.Errorf("Expected Path 'test/path', got %q", k.Path)
		}

		if k.Interval == nil {
			t.Error("Expected Interval to be set, got nil")
		} else if k.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected Interval duration 5m, got %v", k.Interval.Duration)
		}

		if k.RetryInterval == nil {
			t.Error("Expected RetryInterval to be set, got nil")
		} else if k.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected RetryInterval duration 2m, got %v", k.RetryInterval.Duration)
		}

		if k.Timeout == nil {
			t.Error("Expected Timeout to be set, got nil")
		} else if k.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected Timeout duration 10m, got %v", k.Timeout.Duration)
		}

		if k.DeleteTimeout == nil {
			t.Error("Expected DeleteTimeout to be set, got nil")
		} else if k.DeleteTimeout.Duration != 45*time.Minute {
			t.Errorf("Expected DeleteTimeout duration 45m, got %v", k.DeleteTimeout.Duration)
		}
	})

	t.Run("UnmarshalsDurationObjects", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
path: test/path
interval:
  duration: 5m0s
retryInterval:
  duration: 2m0s
timeout:
  duration: 10m0s
`)

		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)
		if err != nil {
			t.Fatalf("Failed to unmarshal Kustomization with duration objects: %v", err)
		}

		if k.Interval == nil {
			t.Error("Expected Interval to be set, got nil")
		} else if k.Interval.Duration != 5*time.Minute {
			t.Errorf("Expected Interval duration 5m, got %v", k.Interval.Duration)
		}

		if k.RetryInterval == nil {
			t.Error("Expected RetryInterval to be set, got nil")
		} else if k.RetryInterval.Duration != 2*time.Minute {
			t.Errorf("Expected RetryInterval duration 2m, got %v", k.RetryInterval.Duration)
		}

		if k.Timeout == nil {
			t.Error("Expected Timeout to be set, got nil")
		} else if k.Timeout.Duration != 10*time.Minute {
			t.Errorf("Expected Timeout duration 10m, got %v", k.Timeout.Duration)
		}
	})

	t.Run("RejectsInvalidDurationString", func(t *testing.T) {
		// Given a kustomization with an interval that is not a duration
		yamlData := []byte("name: test\ninterval: soon\n")

		// When the kustomization is unmarshalled
		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)

		// Then the error names the invalid duration
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), `invalid duration "soon"`) {
			t.Errorf("Expected error to name the invalid duration, got %q", err.Error())
		}
	})

	t.Run("RejectsInvalidDurationObject", func(t *testing.T) {
		// Given a kustomization with an interval object that is not a duration
		yamlData := []byte("name: test\ninterval:\n  duration: soon\n")

		// When the kustomization is unmarshalled
		var k Kustomization
		err := yaml.Unmarshal(yamlData, &k)

		// Then the error names the invalid duration
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), `invalid duration "soon"`) {
			t.Errorf("Expected error to name the invalid duration, got %q", err.Error())
		}
	})
}

func TestKustomization_MarshalYAML_OmitsEmptyName(t *testing.T) {
	t.Run("UnnamedInlineFluxVariantEmitsNoName", func(t *testing.T) {
		// Given a flux system whose install and resources variant have no explicit name
		system := FluxSystem{
			Name:      "cni",
			Install:   &Kustomization{Components: []string{"helm"}},
			Resources: []FluxVariant{{Kustomization: Kustomization{Components: []string{"cilium"}}}},
		}

		// When marshaled to YAML
		out, err := yaml.Marshal(system)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		// Then the unnamed install and resources variant emit no blank name:, while the named
		// system keeps its name
		if strings.Contains(string(out), `name: ""`) {
			t.Errorf("expected no empty name: in output, got:\n%s", out)
		}
		if !strings.Contains(string(out), "name: cni") {
			t.Errorf("expected named system to keep its name, got:\n%s", out)
		}
	})
}

func TestBoolExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: "${shouldDestroy}"`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if !component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be '${shouldDestroy}', got %q", component.Destroy.Expr)
		}
		if component.Destroy.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if !roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Destroy.Expr != "${shouldDestroy}" {
			t.Errorf("Expected Expr to be preserved as '${shouldDestroy}', got %q", roundTrip.Destroy.Expr)
		}
	})

	t.Run("PreservesBooleanValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`destroy: true`)

		var component struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Destroy == nil {
			t.Fatal("Expected Destroy to be set")
		}
		if component.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Destroy.Value == nil || !*component.Destroy.Value {
			t.Error("Expected Value to be true")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Destroy *BoolExpression `yaml:"destroy,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Destroy == nil {
			t.Fatal("Expected Destroy to be preserved after round-trip")
		}
		if roundTrip.Destroy.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Destroy.Value == nil || !*roundTrip.Destroy.Value {
			t.Error("Expected Value to be true after round-trip")
		}
	})
}

func TestIntExpression_MarshalYAML(t *testing.T) {
	t.Run("PreservesExpressionInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: "${cluster.parallelism ?? 10}"`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if !component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be '${cluster.parallelism ?? 10}', got %q", component.Parallelism.Expr)
		}
		if component.Parallelism.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if !roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be true after round-trip")
		}
		if roundTrip.Parallelism.Expr != "${cluster.parallelism ?? 10}" {
			t.Errorf("Expected Expr to be preserved as '${cluster.parallelism ?? 10}', got %q", roundTrip.Parallelism.Expr)
		}
	})

	t.Run("PreservesIntegerValueInRoundTrip", func(t *testing.T) {
		yamlData := []byte(`parallelism: 5`)

		var component struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Parallelism == nil {
			t.Fatal("Expected Parallelism to be set")
		}
		if component.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false")
		}
		if component.Parallelism.Value == nil || *component.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5")
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip struct {
			Parallelism *IntExpression `yaml:"parallelism,omitempty"`
		}
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Parallelism == nil {
			t.Fatal("Expected Parallelism to be preserved after round-trip")
		}
		if roundTrip.Parallelism.IsExpr {
			t.Error("Expected IsExpr to be false after round-trip")
		}
		if roundTrip.Parallelism.Value == nil || *roundTrip.Parallelism.Value != 5 {
			t.Error("Expected Value to be 5 after round-trip")
		}
	})
}

func TestBoolExpression_IsEnabled(t *testing.T) {
	t.Run("ReturnsTrueWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsEnabled() {
			t.Error("Expected IsEnabled to return false when value is false")
		}
	})

	t.Run("ReturnsTrueWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when IsExpr is true")
		}
	})

	t.Run("ReturnsTrueWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if !expr.IsEnabled() {
			t.Error("Expected IsEnabled to return true when Value is nil and not expression")
		}
	})
}

func TestBoolExpression_IsInstalled(t *testing.T) {
	t.Run("ReturnsFalseWhenNil", func(t *testing.T) {
		var expr *BoolExpression
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when nil")
		}
	})

	t.Run("ReturnsTrueWhenValueIsTrue", func(t *testing.T) {
		trueVal := true
		expr := &BoolExpression{Value: &trueVal, IsExpr: false}
		if !expr.IsInstalled() {
			t.Error("Expected IsInstalled to return true when value is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsFalse", func(t *testing.T) {
		falseVal := false
		expr := &BoolExpression{Value: &falseVal, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when value is false")
		}
	})

	t.Run("ReturnsFalseWhenIsExpression", func(t *testing.T) {
		expr := &BoolExpression{IsExpr: true, Expr: "${some.condition ?? true}"}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when IsExpr is true")
		}
	})

	t.Run("ReturnsFalseWhenValueIsNilButNotExpression", func(t *testing.T) {
		expr := &BoolExpression{Value: nil, IsExpr: false}
		if expr.IsInstalled() {
			t.Error("Expected IsInstalled to return false when Value is nil and not expression")
		}
	})
}

func TestSource_Install(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		source := Source{
			Name: "test-source",
			Url:  "oci://example.com/repo:tag",
		}
		if source.Name != "test-source" || source.Url != "oci://example.com/repo:tag" {
			t.Error("Expected Name and Url to be set")
		}
		if source.Install != nil {
			t.Error("Expected Install to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		source := Source{
			Name:    "test-source",
			Url:     "oci://example.com/repo:tag",
			Install: &BoolExpression{Value: &falseVal, IsExpr: false},
		}
		if source.Name != "test-source" || source.Url != "oci://example.com/repo:tag" {
			t.Error("Expected Name and Url to be set")
		}
		if source.Install == nil {
			t.Fatal("Expected Install to be set")
		}
		if source.Install.Value == nil || *source.Install.Value {
			t.Error("Expected Install to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`name: test-source
url: oci://example.com/repo:tag
install: "${some.condition ?? true}"`)

		var source Source
		err := yaml.Unmarshal(yamlData, &source)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if source.Install == nil {
			t.Fatal("Expected Install to be set")
		}
		if !source.Install.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if source.Install.Expr != "${some.condition ?? true}" {
			t.Errorf("Expected Expr to be '${some.condition ?? true}', got %q", source.Install.Expr)
		}
		if source.Install.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		source := Source{
			Name:    "test-source",
			Url:     "oci://example.com/repo:tag",
			Install: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&source)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip Source
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Install == nil {
			t.Fatal("Expected Install to be preserved after round-trip")
		}
		if roundTrip.Install.Value == nil || *roundTrip.Install.Value {
			t.Error("Expected Install to be false after round-trip")
		}
	})
}

func TestTerraformComponent_Enabled(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		component := TerraformComponent{
			Path: "test/path",
		}
		if component.Path != "test/path" {
			t.Error("Expected Path to be set")
		}
		if component.Enabled != nil {
			t.Error("Expected Enabled to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		component := TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}
		if component.Path != "test/path" {
			t.Error("Expected Path to be set")
		}
		if component.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if component.Enabled.Value == nil || *component.Enabled.Value {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`path: test/path
enabled: "${cluster.driver != ''}"`)

		var component TerraformComponent
		err := yaml.Unmarshal(yamlData, &component)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if component.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if !component.Enabled.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if component.Enabled.Expr != "${cluster.driver != ''}" {
			t.Errorf("Expected Expr to be '${cluster.driver != ''}', got %q", component.Enabled.Expr)
		}
		if component.Enabled.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		component := TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&component)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip TerraformComponent
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Enabled == nil {
			t.Fatal("Expected Enabled to be preserved after round-trip")
		}
		if roundTrip.Enabled.Value == nil || *roundTrip.Enabled.Value {
			t.Error("Expected Enabled to be false after round-trip")
		}
	})

	t.Run("IsCopiedInDeepCopy", func(t *testing.T) {
		falseVal := false
		original := &TerraformComponent{
			Path:    "test/path",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		copy := original.DeepCopy()
		if copy == nil {
			t.Fatal("Expected DeepCopy to return non-nil")
		}
		if copy.Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}

		trueVal := true
		original.Enabled.Value = &trueVal
		if copy.Enabled == nil || copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to be independent - modifying original should not affect copy")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to still have false value after modifying original")
		}
	})
}

func TestKustomization_Enabled(t *testing.T) {
	t.Run("IsNilWhenNotSet", func(t *testing.T) {
		kustomization := Kustomization{
			Name: "test-kustomization",
		}
		if kustomization.Name != "test-kustomization" {
			t.Error("Expected Name to be set")
		}
		if kustomization.Enabled != nil {
			t.Error("Expected Enabled to be nil when not set")
		}
	})

	t.Run("CanBeSetToFalse", func(t *testing.T) {
		falseVal := false
		kustomization := Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}
		if kustomization.Name != "test-kustomization" {
			t.Error("Expected Name to be set")
		}
		if kustomization.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if kustomization.Enabled.Value == nil || *kustomization.Enabled.Value {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("SupportsExpression", func(t *testing.T) {
		yamlData := []byte(`name: test-kustomization
enabled: "${observability.enabled ?? true}"`)

		var kustomization Kustomization
		err := yaml.Unmarshal(yamlData, &kustomization)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if kustomization.Enabled == nil {
			t.Fatal("Expected Enabled to be set")
		}
		if !kustomization.Enabled.IsExpr {
			t.Error("Expected IsExpr to be true")
		}
		if kustomization.Enabled.Expr != "${observability.enabled ?? true}" {
			t.Errorf("Expected Expr to be '${observability.enabled ?? true}', got %q", kustomization.Enabled.Expr)
		}
		if kustomization.Enabled.Value != nil {
			t.Error("Expected Value to be nil for expression")
		}
	})

	t.Run("PreservesValueInRoundTrip", func(t *testing.T) {
		falseVal := false
		kustomization := Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		marshaled, err := yaml.Marshal(&kustomization)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var roundTrip Kustomization
		err = yaml.Unmarshal(marshaled, &roundTrip)
		if err != nil {
			t.Fatalf("Failed to unmarshal round-trip: %v", err)
		}

		if roundTrip.Enabled == nil {
			t.Fatal("Expected Enabled to be preserved after round-trip")
		}
		if roundTrip.Enabled.Value == nil || *roundTrip.Enabled.Value {
			t.Error("Expected Enabled to be false after round-trip")
		}
	})

	t.Run("IsCopiedInDeepCopy", func(t *testing.T) {
		falseVal := false
		original := &Kustomization{
			Name:    "test-kustomization",
			Enabled: &BoolExpression{Value: &falseVal, IsExpr: false},
		}

		copy := original.DeepCopy()
		if copy == nil {
			t.Fatal("Expected DeepCopy to return non-nil")
		}
		if copy.Enabled == nil {
			t.Fatal("Expected Enabled to be copied")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected Enabled to be false in copy")
		}

		trueVal := true
		original.Enabled.Value = &trueVal
		if copy.Enabled == nil || copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to be independent - modifying original should not affect copy")
		}
		if copy.Enabled.Value == nil || *copy.Enabled.Value {
			t.Error("Expected copy to still have false value after modifying original")
		}
	})
}

func TestKustomization_Secrets(t *testing.T) {
	t.Run("DeepCopyClonesSecretsIndependently", func(t *testing.T) {
		// Given a kustomization carrying a named secret with a data key
		original := &Kustomization{
			Name:    "cdn-install",
			Secrets: map[string]SecretEntry{"cloudflare-creds": {Data: map[string]string{"api_token": "${cdn.cloudflare_api_key}"}}},
		}

		// When deep-copying and mutating the copy's inner data map
		clone := original.DeepCopy()
		entry := clone.Secrets["cloudflare-creds"]
		entry.Data["api_token"] = "changed"
		entry.Data["extra"] = "value"
		clone.Secrets["cloudflare-creds"] = entry

		// Then the original inner map is untouched
		if original.Secrets["cloudflare-creds"].Data["api_token"] != "${cdn.cloudflare_api_key}" {
			t.Errorf("Expected original unchanged, got %q", original.Secrets["cloudflare-creds"].Data["api_token"])
		}
		if _, exists := original.Secrets["cloudflare-creds"].Data["extra"]; exists {
			t.Error("Expected original to not gain the clone's key")
		}
	})

	t.Run("SecretsNeverMarshalToYAML", func(t *testing.T) {
		// Given a kustomization with a literal secret in its compiled carrier
		k := Kustomization{
			Name:    "cdn-install",
			Secrets: map[string]SecretEntry{"cloudflare-creds": {Data: map[string]string{"api_token": "PLAINTEXT-SECRET"}}},
		}

		// When marshaling to YAML
		out, err := yaml.Marshal(k)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		// Then neither the value, the keys, nor a "secrets" field appears
		s := string(out)
		if strings.Contains(s, "PLAINTEXT-SECRET") || strings.Contains(s, "api_token") || strings.Contains(s, "cloudflare-creds") || strings.Contains(s, "secrets") {
			t.Errorf("Expected Secrets to be omitted from marshaled output, got:\n%s", s)
		}
	})
}

func TestFluxSystem_Secrets(t *testing.T) {
	t.Run("DeepCopyClonesSecretsIndependently", func(t *testing.T) {
		// Given a flux system with a named secret carrying namespaces and a data key
		original := &FluxSystem{
			Name:    "cdn",
			Secrets: map[string]SecretEntry{"cloudflare-creds": {Namespaces: []string{"system-dns"}, Data: map[string]string{"api_token": "${cdn.cloudflare_api_key}"}}},
		}

		// When deep-copying and mutating the copy's inner data map and namespaces
		clone := original.DeepCopy()
		entry := clone.Secrets["cloudflare-creds"]
		entry.Data["api_token"] = "changed"
		entry.Data["extra"] = "value"
		entry.Namespaces[0] = "system-other"
		clone.Secrets["cloudflare-creds"] = entry

		// Then the original inner map and namespaces are untouched
		if original.Secrets["cloudflare-creds"].Data["api_token"] != "${cdn.cloudflare_api_key}" {
			t.Errorf("Expected original secret unchanged, got %q", original.Secrets["cloudflare-creds"].Data["api_token"])
		}
		if _, exists := original.Secrets["cloudflare-creds"].Data["extra"]; exists {
			t.Error("Expected original to not gain the clone's key")
		}
		if original.Secrets["cloudflare-creds"].Namespaces[0] != "system-dns" {
			t.Errorf("Expected original namespaces unchanged, got %v", original.Secrets["cloudflare-creds"].Namespaces)
		}
	})

	t.Run("UnmarshalsNestedSecretsFromYAML", func(t *testing.T) {
		// Given a flux system authored with a nested secrets: map using the data: form
		data := []byte(`
name: cdn
secrets:
  cloudflare-creds:
    data:
      api_token: ${cdn.cloudflare_api_key}
      zone_id: ${cdn.cloudflare_zone_id}
install:
  components: [cloudflared]
`)

		// When unmarshaling
		var system FluxSystem
		if err := yaml.Unmarshal(data, &system); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		// Then each data key is populated under the secret name
		if got := system.Secrets["cloudflare-creds"].Data["api_token"]; got != "${cdn.cloudflare_api_key}" {
			t.Errorf("Expected api_token reference, got %q", got)
		}
		if got := system.Secrets["cloudflare-creds"].Data["zone_id"]; got != "${cdn.cloudflare_zone_id}" {
			t.Errorf("Expected zone_id reference, got %q", got)
		}
	})

	t.Run("UnmarshalsExplicitNamespacesFromYAML", func(t *testing.T) {
		// Given a flux system whose secret names the namespaces to place it into
		data := []byte(`
name: pki
secrets:
  hetzner-dns:
    namespaces: [system-pki, system-dns]
    data:
      token: ${hetzner.dns_token}
install:
  components: [cert-manager]
`)

		// When unmarshaling
		var system FluxSystem
		if err := yaml.Unmarshal(data, &system); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		// Then the target namespaces and data both decode without a custom parser
		entry := system.Secrets["hetzner-dns"]
		if len(entry.Namespaces) != 2 || entry.Namespaces[0] != "system-pki" || entry.Namespaces[1] != "system-dns" {
			t.Errorf("Expected namespaces [system-pki system-dns], got %v", entry.Namespaces)
		}
		if entry.Data["token"] != "${hetzner.dns_token}" {
			t.Errorf("Expected token reference, got %q", entry.Data["token"])
		}
	})
}
