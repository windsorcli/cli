package blueprint

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessFacets_Config(t *testing.T) {
	t.Run("ResolvesTerraformInputsFromConfigBlock", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider": "incus",
				"cluster": map[string]any{
					"controlplanes": []any{map[string]any{"hostname": "cp0"}, map[string]any{"hostname": "cp1"}},
					"workers":       []any{map[string]any{"hostname": "w0"}},
				},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "talos"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos",
						Body: map[string]any{
							"value": map[string]any{
								"controlplanes": "${cluster.controlplanes}",
								"workers":       "${cluster.workers}",
							},
						},
					},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "cluster",
							Inputs: map[string]any{
								"controlplanes": "${talos.controlplanes}",
								"workers":       "${talos.workers}",
							},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		cp, ok := inputs["controlplanes"].([]any)
		if !ok {
			t.Fatalf("Expected controlplanes to be []any, got %T", inputs["controlplanes"])
		}
		if len(cp) != 2 {
			t.Errorf("Expected 2 controlplanes, got %d", len(cp))
		}
		w, ok := inputs["workers"].([]any)
		if !ok {
			t.Fatalf("Expected workers to be []any, got %T", inputs["workers"])
		}
		if len(w) != 1 {
			t.Errorf("Expected 1 worker, got %d", len(w))
		}
	})

	t.Run("SecondFacetSeesFirstFacetConfig", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus", "cluster": map[string]any{"name": "mycluster"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-only"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", Body: map[string]any{"value": map[string]any{"cluster_name": "${cluster.name}"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				When:     "provider == 'incus'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "cluster",
							Inputs: map[string]any{
								"name": "${talos.cluster_name}",
							},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["name"] != "mycluster" {
			t.Errorf("Expected name=mycluster, got %v", inputs["name"])
		}
	})

	t.Run("ScalarConfigBlockDoesNotCorruptLaterMapBlockMerge", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"provider":     "docker",
				"talos_common": map[string]any{"allowSchedulingOnControlPlanes": false},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "scalar-and-map-config"},
				When:     "provider == 'docker'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "primary_node_ip",
						Body: map[string]any{"value": "${'10.0.0.5'}"},
					},
					{
						Name: "talos_common",
						Body: map[string]any{
							"value": map[string]any{"allowSchedulingOnControlPlanes": true},
						},
					},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path: "cluster",
							Inputs: map[string]any{
								"primary_ip":  "${primary_node_ip}",
								"schedulable": "${talos_common.allowSchedulingOnControlPlanes}",
							},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if scope["primary_node_ip"] != "10.0.0.5" {
			t.Errorf("Expected scope primary_node_ip='10.0.0.5', got %v", scope["primary_node_ip"])
		}
		talosCommon, ok := scope["talos_common"].(map[string]any)
		if !ok {
			t.Fatalf("Expected scope talos_common to be map, got %T", scope["talos_common"])
		}
		if v, ok := talosCommon["allowSchedulingOnControlPlanes"].(bool); !ok || !v {
			t.Errorf("Expected talos_common.allowSchedulingOnControlPlanes=true (facet wins over context), got %v", talosCommon["allowSchedulingOnControlPlanes"])
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["primary_ip"] != "10.0.0.5" {
			t.Errorf("Expected inputs primary_ip='10.0.0.5', got %v", inputs["primary_ip"])
		}
		if inputs["schedulable"] != true {
			t.Errorf("Expected inputs schedulable=true, got %v", inputs["schedulable"])
		}
	})

	t.Run("SkipsConfigBlockWhenWhenFalse", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-config"},
				When:     "provider == 'docker'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", When: "provider == 'incus'", Body: map[string]any{"value": map[string]any{"x": "from-incus"}}},
					{Name: "talos", When: "provider == 'docker'", Body: map[string]any{"value": map[string]any{"x": "from-docker"}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"val": "${talos.x}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["val"] != "from-docker" {
			t.Errorf("Expected val=from-docker (second block when matched), got %v", inputs["val"])
		}
	})

	t.Run("ResolvesSameBlockReferenceWhenLaterKeyDependsOnEarlierKey", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "self-ref"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos_common_docker",
						Body: map[string]any{
							"value": map[string]any{
								"patchVars":    map[string]any{"key": "val"},
								"common_patch": "${string(talos_common_docker.patchVars)}",
								"patches":      "${string(talos_common_docker.common_patch)}",
							},
						},
					},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"out": "${talos_common_docker.patches}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		out, ok := inputs["out"].(string)
		if !ok {
			t.Fatalf("Expected out to be string, got %T", inputs["out"])
		}
		if out == "" || out == "null" {
			t.Errorf("Expected patches (and thus out) to resolve from same-block common_patch, got %q", out)
		}
	})

	t.Run("ResolvesSameNameConfigByWhenWhenMatchingBlockIsFirstInList", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "docker", "workstation": map[string]any{"enabled": true}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional-config"},
				When:     "provider == 'docker'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos_common", When: "provider == 'docker'", Body: map[string]any{"value": map[string]any{"patches": []any{"docker-patch.yaml"}}}},
					{Name: "talos_common", When: "!(provider == 'docker')", Body: map[string]any{"value": map[string]any{"patches": []any{"non-docker-patch.yaml"}}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"common_config_patches": "${talos_common.patches}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		patches, ok := inputs["common_config_patches"].([]any)
		if !ok {
			t.Fatalf("Expected common_config_patches to be []any, got %T", inputs["common_config_patches"])
		}
		if len(patches) != 1 || patches[0] != "docker-patch.yaml" {
			t.Errorf("Expected common_config_patches=[docker-patch.yaml] (matching block is first in list), got %v", patches)
		}
	})

	t.Run("ProcessesFacetsWithEmptyConfigList", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "no-config"},
				When:     "provider == 'incus'",
				Config:   nil,
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"x": "y"},
						},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Inputs["x"] != "y" {
			t.Errorf("Expected x=y, got %v", target.TerraformComponents[0].Inputs["x"])
		}
	})

	t.Run("ProcessesConfigBlockWithEmptyBody", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "empty-body"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "empty", Body: map[string]any{"value": map[string]any{}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"key": "${empty.foo ?? 'default'}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["key"] != "default" {
			t.Errorf("Expected key=default (undefined empty.foo with ?? fallback), got %v", inputs["key"])
		}
	})

	t.Run("PreservesScalarAndListConfigBlockValuesInScope", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "scalar-list-config"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "platform", Body: map[string]any{"value": "incus"}},
					{Name: "tags", Body: map[string]any{"value": []any{"a", "b"}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"platform": "${platform}", "tags": "${string(tags)}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["platform"] != "incus" {
			t.Errorf("Expected platform='incus' (scalar config block preserved), got %v", inputs["platform"])
		}
		if inputs["tags"] == nil {
			t.Error("Expected tags to be set (list config block preserved)")
		}
	})

	t.Run("EvaluatesExpressionsInScalarAndListConfigBlockValues", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"provider": "incus", "tag1": "x", "tag2": "y"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "expr-scalar-list-config"},
				When:     "provider == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "platform", Body: map[string]any{"value": "${provider}"}},
					{Name: "tags", Body: map[string]any{"value": []any{"${tag1}", "${tag2}"}}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{
							Path:   "cluster",
							Inputs: map[string]any{"platform": "${platform}", "tags": "${string(tags)}"},
						},
					},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		inputs := evalTerraformInputsWithScope(t, mocks.Runtime, target.TerraformComponents[0].Inputs, scope)
		if inputs["platform"] != "incus" {
			t.Errorf("Expected platform='incus' (scalar expression ${provider} evaluated), got %v", inputs["platform"])
		}
		tagsStr, _ := inputs["tags"].(string)
		if tagsStr == "" || !strings.Contains(tagsStr, "x") || !strings.Contains(tagsStr, "y") {
			t.Errorf("Expected tags to contain evaluated list (${tag1}/${tag2}), got %q", tagsStr)
		}
	})
}

func TestProcessor_ProcessFacets_ConfigBlockSameBlockSiblingRefs(t *testing.T) {
	// A config value expression must be able to read a sibling key computed in the same block,
	// including through non-call operators (e.g. string concatenation), resolving at compose time
	// in dependency order rather than erroring on <nil> or being left unresolved.

	run := func(t *testing.T, body map[string]any) (map[string]any, error) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "identity"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "identity_effective", Body: map[string]any{"value": body}},
			},
		}}
		return processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
	}

	t.Run("SiblingReferenceViaConcatenationResolves", func(t *testing.T) {
		// Given a literal sibling and a key deriving from it via + (a non-call operator)
		scope, err := run(t, map[string]any{
			"issuer":   "https://id.example",
			"auth_url": "${identity_effective.issuer + '/protocol/openid-connect/auth'}",
		})

		// Then the derived key resolves against the sibling instead of erroring on <nil>
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block, ok := scope["identity_effective"].(map[string]any)
		if !ok {
			t.Fatalf("Expected identity_effective map, got %#v", scope["identity_effective"])
		}
		if block["auth_url"] != "https://id.example/protocol/openid-connect/auth" {
			t.Errorf("Expected auth_url derived from sibling issuer, got %v", block["auth_url"])
		}
	})

	t.Run("SiblingReferenceWhereSiblingIsItselfComputed", func(t *testing.T) {
		// Given a computed sibling and a key deriving from it, order must resolve issuer first
		scope, err := run(t, map[string]any{
			"issuer":   "${'https://' + 'id.example'}",
			"auth_url": "${identity_effective.issuer + '/auth'}",
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block, ok := scope["identity_effective"].(map[string]any)
		if !ok {
			t.Fatalf("Expected identity_effective map, got %#v", scope["identity_effective"])
		}
		if block["issuer"] != "https://id.example" {
			t.Errorf("Expected issuer resolved, got %v", block["issuer"])
		}
		if block["auth_url"] != "https://id.example/auth" {
			t.Errorf("Expected auth_url derived from computed issuer, got %v", block["auth_url"])
		}
	})

	t.Run("MultipleSiblingsDeriveFromOneIntermediate", func(t *testing.T) {
		// Given several keys deriving from one computed intermediate (the motivating OIDC case)
		scope, err := run(t, map[string]any{
			"issuer":       "${'https://id.example'}",
			"auth_url":     "${identity_effective.issuer + '/protocol/openid-connect/auth'}",
			"token_url":    "${identity_effective.issuer + '/protocol/openid-connect/token'}",
			"userinfo_url": "${identity_effective.issuer + '/protocol/openid-connect/userinfo'}",
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block := scope["identity_effective"].(map[string]any)
		if block["token_url"] != "https://id.example/protocol/openid-connect/token" {
			t.Errorf("Expected token_url derived, got %v", block["token_url"])
		}
		if block["userinfo_url"] != "https://id.example/protocol/openid-connect/userinfo" {
			t.Errorf("Expected userinfo_url derived, got %v", block["userinfo_url"])
		}
	})

	t.Run("LaterSameNameEntryReadsEarlierEntry", func(t *testing.T) {
		// Given two config entries with the same block name, the later reading the earlier
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "identity"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "identity_effective", Body: map[string]any{"value": map[string]any{"issuer": "https://id.example"}}},
				{Name: "identity_effective", Body: map[string]any{"value": map[string]any{"auth_url": "${identity_effective.issuer + '/auth'}"}}},
			},
		}}
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block := scope["identity_effective"].(map[string]any)
		if block["auth_url"] != "https://id.example/auth" {
			t.Errorf("Expected later entry to read earlier entry's issuer, got %v", block["auth_url"])
		}
	})

	t.Run("GenuineNonDeferredErrorStillSurfaces", func(t *testing.T) {
		// Given a value with a genuine error — arithmetic on a reference that never resolves —
		// the tolerant per-key pass keeps it raw, but composition must still fail rather than
		// emit an unexpanded ${...} into rendered config
		_, err := run(t, map[string]any{
			"issuer":   "https://id.example",
			"auth_url": "${identity_effective.nonexistent + '/auth'}",
		})
		if err == nil {
			t.Fatal("Expected a genuine unresolved-reference error to surface, got nil")
		}
		if !strings.Contains(err.Error(), "identity_effective") {
			t.Errorf("Expected error to name the config block, got %v", err)
		}
	})

	t.Run("NestedGenuineErrorSurfaces", func(t *testing.T) {
		// Given a genuine error nested inside a map value, it must surface too
		_, err := run(t, map[string]any{
			"issuer": "https://id.example",
			"urls": map[string]any{
				"auth": "${identity_effective.nonexistent + '/auth'}",
			},
		})
		if err == nil {
			t.Fatal("Expected a nested genuine error to surface, got nil")
		}
	})
	// Note: a value referencing a deferred helper (e.g. terraform_output) is kept raw for JIT rather
	// than surfaced as an error; that path is covered by TestProcessor_ProcessFacets_ConfigDeferredValues,
	// which registers a real deferred helper and exercises this post-convergence validation.
}

// TestProcessor_ProcessFacets_ConfigBlockDeepSameBlockChain reconstructs the shape of the
// resource-tapering config in windsorcli/core's config-talos.yaml (#3206): sibling fields chained
// five-plus levels deep within one config block (ratio -> factor -> a boolean derived from the
// factor -> a boolean OR'd from two such booleans -> a when:-gated block reading the factor again).
// A same-block convergence budget too small for the chain's depth causes a field partway through
// the chain to freeze at a stale value instead of erroring, so a structurally-identical sibling
// chain (the worker axis) silently resolves using a value read from a different chain (the
// control-plane axis) instead of its own.
func TestProcessor_ProcessFacets_ConfigBlockDeepSameBlockChain(t *testing.T) {
	t.Run("StructurallyIdenticalSiblingChainsResolveIndependently", func(t *testing.T) {
		// Given two independent tiers computing a taper factor via a chained ratio -> factor ->
		// constrained-flag -> reserve pipeline, five-plus same-block references deep, gated
		// behind when: clauses that themselves depend on earlier links in the same chain
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"cluster_resources_effective": map[string]any{
					"controlplanes": map[string]any{"cpu": 100, "memory": 6},
					"workers":       map[string]any{"cpu": 100, "memory": 5},
				},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "talos"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos_common", Body: map[string]any{"value": map[string]any{
						"cp_tier":          map[string]any{"threshold": map[string]any{"cpu": 2, "memory": 4}, "max": map[string]any{"kubelet_cpu": 500}},
						"worker_threshold": map[string]any{"cpu": 4, "memory": 4},
						"worker_max":       map[string]any{"kubelet_cpu": 80},
					}}},
					{Name: "talos_common", Body: map[string]any{"value": map[string]any{
						"cp_cpu_ratio":          "${cluster_resources_effective.controlplanes.cpu / talos_common.cp_tier.threshold.cpu}",
						"cp_memory_ratio":       "${cluster_resources_effective.controlplanes.memory / talos_common.cp_tier.threshold.memory}",
						"cp_ratio":              "${talos_common.cp_cpu_ratio < talos_common.cp_memory_ratio ? talos_common.cp_cpu_ratio : talos_common.cp_memory_ratio}",
						"cp_factor":             "${talos_common.cp_ratio > 2 ? 0 : (talos_common.cp_ratio < 1 ? 1 : (2 - talos_common.cp_ratio))}",
						"resource_constrained":  "${talos_common.cp_factor > 0}",
						"worker_cpu_ratio":      "${cluster_resources_effective.workers.cpu / talos_common.worker_threshold.cpu}",
						"worker_memory_ratio":   "${cluster_resources_effective.workers.memory / talos_common.worker_threshold.memory}",
						"worker_ratio":          "${talos_common.worker_cpu_ratio < talos_common.worker_memory_ratio ? talos_common.worker_cpu_ratio : talos_common.worker_memory_ratio}",
						"worker_factor":         "${talos_common.worker_ratio > 2 ? 0 : (talos_common.worker_ratio < 1 ? 1 : (2 - talos_common.worker_ratio))}",
						"worker_constrained":    "${talos_common.worker_factor > 0}",
						"worker_reserve_needed": "${talos_common.resource_constrained || talos_common.worker_constrained}",
					}}},
					{Name: "talos_common", When: "talos_common.resource_constrained", Body: map[string]any{"value": map[string]any{
						"cp_reserve": map[string]any{
							"kubelet_cpu": "${string(int(talos_common.cp_tier.max.kubelet_cpu * talos_common.cp_factor + 0.5)) + 'm'}",
						},
					}}},
					{Name: "talos_common", When: "talos_common.worker_reserve_needed", Body: map[string]any{"value": map[string]any{
						"worker_reserve": map[string]any{
							"kubelet_cpu": "${string(int(talos_common.worker_max.kubelet_cpu * talos_common.worker_factor + 0.5)) + 'm'}",
						},
					}}},
				},
			},
		}

		// When processing
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		// Then both chains converge, and the worker chain resolves using its own factor
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block := scope["talos_common"].(map[string]any)
		if block["cp_factor"] != 0.5 {
			t.Errorf("Expected cp_factor 0.5, got %v", block["cp_factor"])
		}
		if block["worker_factor"] != 0.75 {
			t.Errorf("Expected worker_factor 0.75, got %v", block["worker_factor"])
		}
		cpReserve, ok := block["cp_reserve"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cp_reserve to resolve, got %#v", block["cp_reserve"])
		}
		if cpReserve["kubelet_cpu"] != "250m" {
			t.Errorf("Expected cp_reserve.kubelet_cpu 250m, got %v", cpReserve["kubelet_cpu"])
		}
		workerReserve, ok := block["worker_reserve"].(map[string]any)
		if !ok {
			t.Fatalf("Expected worker_reserve to resolve, got %#v", block["worker_reserve"])
		}
		if workerReserve["kubelet_cpu"] != "60m" {
			t.Errorf("Expected worker_reserve.kubelet_cpu 60m (worker_factor 0.75), not a value collided from cp_factor, got %v", workerReserve["kubelet_cpu"])
		}
	})
}

// TestProcessor_ProcessFacets_ConfigBlockDerivedFromOwnBlock covers a field whose default calls a
// function with a sibling field of the same block as an argument (e.g. cidrhost(network.cidr_block,
// 1) inside the network block): it must resolve once the block settles, and it must still defer
// when its own dependency (e.g. terraform_output) is not yet available.
func TestProcessor_ProcessFacets_ConfigBlockDerivedFromOwnBlock(t *testing.T) {
	t.Run("SelfReferencingFunctionCallResolvesAgainstSettledSibling", func(t *testing.T) {
		// Given a block whose own field derives its default from a sibling field of the same
		// block through a function call
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "platform-vsphere"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "network", Body: map[string]any{"value": map[string]any{
					"cidr_block": "10.10.0.0/24",
					"gateway":    "${cidrhost(network.cidr_block ?? '10.5.0.0/16', 1)}",
				}}},
			},
		}}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		// Then gateway resolves from the block's own cidr_block, not left as literal text
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %T", scope["network"])
		}
		if network["gateway"] != "10.10.0.1" {
			t.Errorf("Expected gateway derived from cidr_block 10.10.0.0/24, got %v", network["gateway"])
		}
	})

	t.Run("DerivedFieldBlockedOnDeferredDependencyStaysRawForJIT", func(t *testing.T) {
		// Regression guard for 63dbef0cd: a derived field (string(block.field)) whose target
		// field still holds a genuinely deferred helper (terraform_output not yet available)
		// must stay at a raw, still-deferred expression rather than being committed with a
		// value that has baked the unresolved reference into it — a consumer resolves it later,
		// once the dependency is final.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }

		realEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)
		realEval.Register("terraform_output", func(params []any, deferred bool) (any, error) {
			if !deferred {
				return nil, &evaluator.DeferredError{
					Expression: fmt.Sprintf("terraform_output(%q, %q)", params[0], params[1]),
					Message:    "deferred",
				}
			}
			return map[string]any{"gcr.io": "mirror.test"}, nil
		}, new(func(string, string) any))
		mocks.Evaluator.EvaluateFunc = realEval.Evaluate
		mocks.Evaluator.EvaluateMapFunc = realEval.EvaluateMap

		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "config-base"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "block_a", Body: map[string]any{"value": map[string]any{
					"common_patch": map[string]any{
						"machine": map[string]any{
							"registries": "${terraform_output('workstation', 'registries')}",
						},
					},
					"common_config_patches": "${string(block_a.common_patch)}",
				}}},
			},
		}}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		// Then no error surfaces, and the derived field stays deferred rather than committing
		// a value that has lost the ability to resolve once terraform_output is available
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		block, ok := scope["block_a"].(map[string]any)
		if !ok {
			t.Fatalf("Expected block_a map, got %T", scope["block_a"])
		}
		patches, ok := block["common_config_patches"].(string)
		if !ok || patches != "${string(block_a.common_patch)}" {
			t.Errorf("Expected common_config_patches to stay at its raw expression for later JIT resolution, got %#v", block["common_config_patches"])
		}
	})
}

// TestProcessor_ProcessFacets_ConfigBlockWhenChainAcrossRounds covers #3209: a config block's
// `when:` only sees the previous round's resolved scope, so a block gated on another block that
// is itself newly-included this round needs a further outer round to see that block's value. The
// outer round loop used to stop as soon as the included-facet set stopped changing, which for the
// common case of facets with no facet-level `when:` is already true from round zero — so a `when:`
// chain two or more blocks deep across different config blocks never got the extra round it needed
// and stayed permanently (and silently) excluded.
func TestProcessor_ProcessFacets_ConfigBlockWhenChainAcrossRounds(t *testing.T) {
	t.Run("CrossBlockWhenChainResolvesRegardlessOfFileOrder", func(t *testing.T) {
		// Given a two-hop when: chain (tier_b depends on tier_a, tier_a depends on sizing) split
		// across three facets declared in the most adversarial order — the deepest consumer first,
		// the root producer last — dependency order must still win over declaration order.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cluster_resources": map[string]any{"controlplanes": map[string]any{"cpu": 100}}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "a-tier-b"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "tier_b", When: "(tier_a.big ?? false) == true", Body: map[string]any{"value": map[string]any{"bigger": true}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "b-tier-a"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "tier_a", When: "(sizing.cpu ?? 0) > 50", Body: map[string]any{"value": map[string]any{"big": true}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "c-sizing"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "sizing", Body: map[string]any{"value": map[string]any{"cpu": "${cluster_resources.controlplanes.cpu}"}}},
				},
			},
		}

		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		if err != nil {
			t.Fatalf("Expected the when: chain to resolve, got: %v", err)
		}
		tierB, ok := scope["tier_b"].(map[string]any)
		if !ok || tierB["bigger"] != true {
			t.Errorf("Expected tier_b.bigger to resolve to true, got: %#v", scope["tier_b"])
		}
	})

	t.Run("OscillatingWhenDependencyErrorsInsteadOfLoopingForever", func(t *testing.T) {
		// Given two blocks whose when: clauses each gate on the other's absence, inclusion flips
		// every round and never settles. Composition must fail clearly rather than silently return
		// whichever round happened to run last, or loop until an unrelated caller-side timeout.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "oscillator"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "a", When: "(b.flag ?? false) == false", Body: map[string]any{"value": map[string]any{"flag": true}}},
					{Name: "b", When: "(a.flag ?? false) == true", Body: map[string]any{"value": map[string]any{"flag": true}}},
				},
			},
		}

		_, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		if err == nil {
			t.Fatal("Expected an error for a when: dependency that never converges, got nil")
		}
		if !strings.Contains(err.Error(), "did not converge") {
			t.Errorf("Expected a convergence error, got: %v", err)
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "when: or value: cycle") {
			t.Errorf("Expected remediation containing %q, got %v", "when: or value: cycle", werr)
		}
	})
}

func TestProcessor_ProcessFacets_ConfigBlockEvaluationOrder(t *testing.T) {
	// These tests pin the dependency-driven evaluation order for config blocks. The
	// processor must evaluate a block AFTER every block it references via ${...}, no
	// matter which facet wrote each block or in what source order the blocks appear.

	t.Run("ConsumerWrittenBeforeProducerStillEvaluatesAfter", func(t *testing.T) {
		// Mirrors the production core blueprint failure: a low-ordinal facet writes a
		// consumer block (talos_common.k8s_service_host) that calls a function on
		// network_effective.cidr_block. A higher-ordinal facet writes network_effective,
		// whose cidr_block references network_cidr_effective from a third facet. With
		// authoring-order evaluation the consumer would evaluate first and call cidrhost
		// on an unevaluated template string. With dependency-order evaluation each
		// block evaluates only after its prerequisites.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		ord100, ord200, ord300 := 100, 200, 300
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer-first"},
				Ordinal:  &ord100,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos_common", Body: map[string]any{"value": map[string]any{
						"k8s_service_host": "${cidrhost(network_effective.cidr_block, 10)}",
					}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "network-mid"},
				Ordinal:  &ord200,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network_effective", Body: map[string]any{"value": map[string]any{
						"cidr_block": "${network_cidr_effective.value}",
					}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "network-root"},
				Ordinal:  &ord300,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network_cidr_effective", Body: map[string]any{"value": map[string]any{
						"value": "10.5.0.0/16",
					}}},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		talos, ok := scope["talos_common"].(map[string]any)
		if !ok {
			t.Fatalf("Expected talos_common map, got %T", scope["talos_common"])
		}
		if talos["k8s_service_host"] != "10.5.0.10" {
			t.Errorf("Expected talos_common.k8s_service_host='10.5.0.10' (cidrhost of evaluated network), got %v", talos["k8s_service_host"])
		}
	})

	t.Run("ExtendedBlockKeepsDependencyOrderingNotSourceOrder", func(t *testing.T) {
		// Mirrors the platform-base / platform-hyperv scenario: one facet writes the
		// producer block (network with a self-defaulting cidr_block), a higher-ordinal
		// facet writes both an extension of network (gateway) and a consumer block
		// (hyperv_effective) whose value calls cidrhost on the producer. The
		// dependency-driven order must put network ahead of hyperv_effective even though
		// authoring order in the high-ordinal facet lists hyperv_effective first.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		low, high := 100, 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Ordinal:  &low,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"cidr_block": "${network.cidr_block ?? '10.5.0.0/16'}",
					}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-hyperv"},
				Ordinal:  &high,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "hyperv_effective", Body: map[string]any{"value": map[string]any{
						"nat_host_address": "${cidrhost(network.cidr_block ?? '10.5.0.0/16', 1)}",
					}}},
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"gateway": "10.5.0.1",
					}}},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}
		hyperv, ok := scope["hyperv_effective"].(map[string]any)
		if !ok {
			t.Fatalf("Expected hyperv_effective map, got %T", scope["hyperv_effective"])
		}
		if hyperv["nat_host_address"] != "10.5.0.1" {
			t.Errorf("Expected hyperv_effective.nat_host_address='10.5.0.1', got %v", hyperv["nat_host_address"])
		}
	})

	t.Run("CircularReferenceProducesError", func(t *testing.T) {
		// Two blocks reference each other through different keys; no valid evaluation
		// order exists. The topo sort must surface this as an explicit error rather
		// than silently looping until the cross-block retry budget is exhausted.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "cycle"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "alpha", Body: map[string]any{"value": map[string]any{"x": "${beta.y}"}}},
					{Name: "beta", Body: map[string]any{"value": map[string]any{"y": "${alpha.x}"}}},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for circular references between alpha and beta")
		}
		if !strings.Contains(err.Error(), "circular reference") {
			t.Errorf("Expected error to mention 'circular reference', got: %v", err)
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "reference cycle") {
			t.Errorf("Expected remediation containing %q, got %v", "reference cycle", werr)
		}
		if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
			t.Errorf("Expected error to name both blocks in cycle, got: %v", err)
		}
	})
}

func TestProcessor_ProcessFacets_ConfigBlockReadsContextUnderOwnName(t *testing.T) {
	// A config block's own name must not be a blind spot: keys the block does not define
	// itself — schema defaults and operator-set values — stay readable from inside it.

	t.Run("ReadsSchemaDefaultUnderOwnName", func(t *testing.T) {
		// Given a context carrying the network.cidr_block schema default and a network block
		// that does not set cidr_block itself
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"network": map[string]any{"cidr_block": "10.5.0.0/16"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"loadbalancer_driver": "kube-vip",
						"effective_cidr":      "${network.cidr_block}",
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the same-name read resolves against context rather than the block's partial value
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %T", scope["network"])
		}
		if network["effective_cidr"] != "10.5.0.0/16" {
			t.Errorf("Expected network.effective_cidr='10.5.0.0/16' from schema default, got %v", network["effective_cidr"])
		}
	})

	t.Run("ReadsContextUnderOwnNameFromNestedValue", func(t *testing.T) {
		// Given a network block whose nested loadbalancer_ips map reads network.cidr_block
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"network": map[string]any{"cidr_block": "10.5.0.0/16"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"loadbalancer_ips": map[string]any{
							"start": "${cidrhost(network.cidr_block, 266)}",
							"end":   "${cidrhost(network.cidr_block, 356)}",
						},
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then both nested reads resolve
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %T", scope["network"])
		}
		ips, ok := network["loadbalancer_ips"].(map[string]any)
		if !ok {
			t.Fatalf("Expected loadbalancer_ips map, got %T", network["loadbalancer_ips"])
		}
		if ips["start"] != "10.5.1.10" {
			t.Errorf("Expected loadbalancer_ips.start='10.5.1.10', got %v", ips["start"])
		}
		if ips["end"] != "10.5.1.100" {
			t.Errorf("Expected loadbalancer_ips.end='10.5.1.100', got %v", ips["end"])
		}
	})

	t.Run("BlockValueWinsOverContextForSameKey", func(t *testing.T) {
		// Given a block that sets the same key the context carries
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"network": map[string]any{"cidr_block": "10.5.0.0/16"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"cidr_block": "10.9.0.0/16",
						"loadbalancer_ips": map[string]any{
							"start": "${cidrhost(network.cidr_block, 266)}",
						},
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the block's own value wins over context for that key
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %T", scope["network"])
		}
		if network["cidr_block"] != "10.9.0.0/16" {
			t.Errorf("Expected block value to win, got %v", network["cidr_block"])
		}
		ips, ok := network["loadbalancer_ips"].(map[string]any)
		if !ok {
			t.Fatalf("Expected loadbalancer_ips map, got %T", network["loadbalancer_ips"])
		}
		if ips["start"] != "10.9.1.10" {
			t.Errorf("Expected loadbalancer_ips.start derived from block value, got %v", ips["start"])
		}
	})

	t.Run("OperatorExplicitValueWinsOverUnconditionalBlockOnSingleSourcePath", func(t *testing.T) {
		// Given an operator's own explicit value, and a block that unconditionally computes a
		// different value for that same key, via ProcessFacets (the single-source path)
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		operatorValues := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		mocks.ConfigHandler.GetSetValuesFunc = func() map[string]any { return operatorValues }
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "cluster-sizing"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Body: map[string]any{"value": map[string]any{
						"controlplanes": map[string]any{"cpu": 7},
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the operator's explicit value wins, matching ProcessGlobally's guarantee
		controlplanes := scope["cluster"].(map[string]any)["controlplanes"].(map[string]any)
		if controlplanes["cpu"] != 3 {
			t.Errorf("Expected operator cpu=3 to win over facet-computed cpu=7, got %v", controlplanes["cpu"])
		}
	})

	t.Run("OperatorOverrideVisibleToASiblingBlockDerivingFromItInTheSameRound", func(t *testing.T) {
		// Given one block the operator overrides, and a sibling block that derives its own
		// value from the first block's key
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		operatorValues := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		mocks.ConfigHandler.GetSetValuesFunc = func() map[string]any { return operatorValues }
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "cluster-sizing"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "cluster", Body: map[string]any{"value": map[string]any{
						"controlplanes": map[string]any{"cpu": 7},
					}}},
					{Name: "vm_sizing", Body: map[string]any{"value": map[string]any{
						"memory": "${cluster.controlplanes.cpu * 4}",
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the sibling block's derivation used the operator's value (3), not the facet's
		// unconditional computation (7): 3*4=12, not 7*4=28
		vmSizing := scope["vm_sizing"].(map[string]any)
		if vmSizing["memory"] != 12 {
			t.Errorf("Expected memory derived from the operator's cpu=3 (12), got %v", vmSizing["memory"])
		}
	})

	t.Run("ScalarBlockValueReplacesContextValue", func(t *testing.T) {
		// Given a block whose value is a scalar rather than a map, there is nothing to merge
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"network": map[string]any{"cidr_block": "10.5.0.0/16"},
				"ha":      false,
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": "10.5.0.0/16"}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the scalar stands on its own
		if scope["network"] != "10.5.0.0/16" {
			t.Errorf("Expected scalar block value preserved, got %#v", scope["network"])
		}
	})
}

func TestProcessor_ProcessFacets_ConfigBlockGuardBlindToOwnWrite(t *testing.T) {
	// A config block's `when:` guard must be blind to the keys that same block writes,
	// so a "write this default only when the field is unset" guard does not cancel itself:
	// the block's own write in one round must not flip its guard false in the next.

	t.Run("WriteWhenUnsetDefaultAppliesInsteadOfSelfCancelling", func(t *testing.T) {
		// Given an empty context and a network block that defaults cidr_block only when unset
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", When: "(network.cidr_block ?? '') == ''", Body: map[string]any{"value": map[string]any{
						"cidr_block": "10.5.0.0/16",
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the default applies and survives — the block's own write does not drop it
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %#v", scope["network"])
		}
		if network["cidr_block"] != "10.5.0.0/16" {
			t.Errorf("Expected network.cidr_block='10.5.0.0/16', got %v", network["cidr_block"])
		}
	})

	t.Run("WriteWhenUnsetSkippedWhenOperatorAlreadySet", func(t *testing.T) {
		// Given a context where the operator set cidr_block, and the same defaulting block
		// carrying a sentinel that would appear only if the block wrongly fired
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"network": map[string]any{"cidr_block": "10.9.0.0/16"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-base"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", When: "(network.cidr_block ?? '') == ''", Body: map[string]any{"value": map[string]any{
						"cidr_block": "10.5.0.0/16",
						"defaulted":  "yes",
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the guard sees the operator value under its own name and the block does not fire,
		// so it contributes neither the default cidr_block nor its sentinel
		if network, ok := scope["network"].(map[string]any); ok {
			if network["cidr_block"] == "10.5.0.0/16" {
				t.Errorf("Expected operator value not overwritten by default, got %v", network["cidr_block"])
			}
			if network["defaulted"] != nil {
				t.Errorf("Expected guarded block to be skipped, but its sentinel appeared: %v", network["defaulted"])
			}
		}
	})

	t.Run("GuardSeesSiblingKeyWrittenByAnotherFacet", func(t *testing.T) {
		// Given one facet that unconditionally sets network.region and a second whose network
		// block is guarded on that sibling key (not on the key it writes)
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		low, high := 100, 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "region-writer"},
				Ordinal:  &low,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", Body: map[string]any{"value": map[string]any{
						"region": "us",
					}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "cidr-defaulter"},
				Ordinal:  &high,
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "network", When: "network.region == 'us'", Body: map[string]any{"value": map[string]any{
						"cidr_block": "10.5.0.0/16",
					}}},
				},
			},
		}

		// When the facets are processed
		scope, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the guard reads the sibling key from the other facet, so the block contributes
		network, ok := scope["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %#v", scope["network"])
		}
		if network["region"] != "us" {
			t.Errorf("Expected network.region='us', got %v", network["region"])
		}
		if network["cidr_block"] != "10.5.0.0/16" {
			t.Errorf("Expected guard on sibling key to include block, got cidr_block=%v", network["cidr_block"])
		}
	})
}

func TestProcessor_ProcessFacets_ConfigDeferredValues(t *testing.T) {
	t.Run("DeferredConfigBlockValuePreservesExpressionInSubstitution", func(t *testing.T) {
		// Given a config block with a deferred helper and a substitution that
		// references the config block value. During composition the helper is
		// deferred, so the config block stores the raw expression string in scope
		// and the substitution is also marked as deferred for JIT resolution.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		realEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)
		realEval.Register("deferred_helper", func(params []any, deferred bool) (any, error) {
			if !deferred {
				return nil, &evaluator.DeferredError{
					Expression: fmt.Sprintf("deferred_helper(%q)", params[0]),
					Message:    "deferred",
				}
			}
			return "mirror.test", nil
		}, new(func(string) any))

		mocks.Evaluator.EvaluateFunc = realEval.Evaluate
		mocks.Evaluator.EvaluateMapFunc = realEval.EvaluateMap

		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "test-facet"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "mirror_effective",
						Body: map[string]any{
							"value": map[string]any{
								"address": "${(deferred_helper('workstation') ?? 'mirror') + ':5000'}",
							},
						},
					},
				},
				Substitutions: map[string]string{
					"helm_registry": "${mirror_effective.address != '' ? 'oci://' + mirror_effective.address + '/charts' : ''}",
				},
			},
		}

		_, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("ProcessFacets failed: %v", err)
		}

		// Then helm_registry should be marked as deferred for JIT resolution
		helmReg := target.Substitutions["helm_registry"]
		deferred := processor.GetDeferredPaths()
		if !deferred["substitutions.helm_registry"] {
			t.Errorf("Expected helm_registry to be marked as deferred, got deferred paths: %v (value: %q)", deferred, helmReg)
		}
	})

	t.Run("PermanentlyUnresolvableKeyStopsPassesEarlyRegardlessOfBlockSize", func(t *testing.T) {
		// Given a large config block (so the same-block pass cap, sized to the block's own key
		// count, would be large too) where one key can never resolve at composition time
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}

		realEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)
		calls := 0
		realEval.Register("deferred_helper", func(params []any, deferred bool) (any, error) {
			calls++
			if !deferred {
				return nil, &evaluator.DeferredError{Expression: "deferred_helper()", Message: "deferred"}
			}
			return "resolved", nil
		}, new(func() any))

		mocks.Evaluator.EvaluateFunc = realEval.Evaluate
		mocks.Evaluator.EvaluateMapFunc = realEval.EvaluateMap

		processor := NewBlueprintProcessor(mocks.Runtime)
		body := map[string]any{"deferred_field": "${deferred_helper()}"}
		for i := 0; i < 30; i++ {
			body[fmt.Sprintf("padding_%d", i)] = fmt.Sprintf("literal_%d", i)
		}
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "test-facet"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "wide_block", Body: map[string]any{"value": body}},
			},
		}}

		// When processing
		_, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)

		// Then the pass loop stops as soon as re-evaluation stops changing, not after burning
		// through all 30+ keys' worth of passes on a value that can never resolve here
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if calls > 20 {
			t.Errorf("Expected the pass loop to stop quickly once stable rather than scaling with the block's 31 keys, got %d calls to the never-resolving helper", calls)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestProcessor_mergeHelpers(t *testing.T) {
	t.Run("deepMergeMapMergesNestedMaps", func(t *testing.T) {
		base := map[string]any{"a": 1, "b": map[string]any{"x": 10}}
		overlay := map[string]any{"b": map[string]any{"y": 20}, "c": 3}
		out := deepMergeMap(base, overlay)
		if out["a"].(int) != 1 {
			t.Errorf("Expected a=1, got %v", out["a"])
		}
		if out["c"].(int) != 3 {
			t.Errorf("Expected c=3, got %v", out["c"])
		}
		b := out["b"].(map[string]any)
		if b["x"].(int) != 10 {
			t.Errorf("Expected b.x=10, got %v", b["x"])
		}
		if b["y"].(int) != 20 {
			t.Errorf("Expected b.y=20, got %v", b["y"])
		}
	})

	t.Run("MergeScopeMapsDeepMergesSameBlockName", func(t *testing.T) {
		globalScope := map[string]any{
			"talos": map[string]any{"controlplanes": []any{"cp0"}, "workers": []any{"w0"}},
		}
		facetConfig := map[string]any{
			"talos": map[string]any{"workers": []any{"w1", "w2"}},
		}
		out := MergeScopeMaps(globalScope, facetConfig)
		talos := out["talos"].(map[string]any)
		if _, ok := talos["controlplanes"]; !ok {
			t.Error("Expected controlplanes from global to remain")
		}
		workers := talos["workers"].([]any)
		if len(workers) != 2 {
			t.Errorf("Expected overlay workers to overwrite, got %d", len(workers))
		}
	})

	t.Run("OperatorOverlayConfigBlocksBuildsAnEntryForACollidingName", func(t *testing.T) {
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
		}
		operatorOverlay := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)
		cluster, ok := incoming["cluster"]
		if !ok {
			t.Fatal("Expected a synthetic entry for 'cluster'")
		}
		if cluster.Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got %q", cluster.Strategy)
		}
		if cluster.Ordinal == nil || *cluster.Ordinal != math.MaxInt {
			t.Errorf("Expected ordinal math.MaxInt, got %v", cluster.Ordinal)
		}
		if !reflect.DeepEqual(cluster.Body["value"], operatorOverlay["cluster"]) {
			t.Errorf("Expected body value %v, got %v", operatorOverlay["cluster"], cluster.Body["value"])
		}
	})

	t.Run("OperatorOverlayConfigBlocksSkipsNamesAbsentFromGlobalScope", func(t *testing.T) {
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
		}
		operatorOverlay := map[string]any{
			"cluster":   map[string]any{"controlplanes": map[string]any{"cpu": 3}},
			"terraform": map[string]any{"enabled": true},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)
		if _, exists := incoming["terraform"]; exists {
			t.Error("Expected an operator key with no matching config block to be skipped")
		}
	})

	t.Run("OperatorOverlayConfigBlocksHandlesNilGlobalScope", func(t *testing.T) {
		if incoming := operatorOverlayConfigBlocks(nil, map[string]any{"cluster": map[string]any{"cpu": 3}}); incoming != nil {
			t.Errorf("Expected nil for nil globalScope, got %v", incoming)
		}
	})

	t.Run("OperatorOverlayConfigBlocksHandlesEmptyOverlay", func(t *testing.T) {
		globalScope := map[string]any{"cluster": map[string]any{"cpu": 7}}
		if incoming := operatorOverlayConfigBlocks(globalScope, nil); incoming != nil {
			t.Errorf("Expected nil for an empty overlay, got %v", incoming)
		}
	})

	t.Run("OperatorOverlayConfigBlocksMergedIntoScopeOverridesCollidingKeyAndKeepsSiblings", func(t *testing.T) {
		// Given a merge helper's synthetic entry fed through the same mergeConfigBlocks
		// pipeline every facet-vs-facet conflict already uses
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7, "memory": 16}},
		}
		operatorOverlay := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)

		// When merged via mergeConfigBlocks
		out, _, err := processor.mergeConfigBlocks(globalScope, nil, incoming)
		if err != nil {
			t.Fatalf("mergeConfigBlocks failed: %v", err)
		}

		// Then the operator's value wins at the colliding key, and the untouched sibling remains
		controlplanes := out["cluster"].(map[string]any)["controlplanes"].(map[string]any)
		if controlplanes["cpu"] != 3 {
			t.Errorf("Expected operator cpu=3 to win, got %v", controlplanes["cpu"])
		}
		if controlplanes["memory"] != 16 {
			t.Errorf("Expected untouched key memory=16 to remain, got %v", controlplanes["memory"])
		}
	})
}
