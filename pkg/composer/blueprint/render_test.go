package blueprint_test

import (
	"reflect"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRenderForDisplay(t *testing.T) {
	t.Run("ReturnsResourceUnchangedWhenRawMode", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := blueprint.RenderForDisplay(bp, true, map[string]bool{"substitutions.key": true})
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected raw value preserved, got '%s'", got.Substitutions["key"])
		}
	})

	t.Run("ReturnsResourceUnchangedWhenNoDeferredPaths", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := blueprint.RenderForDisplay(bp, false, nil)
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected value unchanged with empty deferred paths, got '%s'", got.Substitutions["key"])
		}
	})

	t.Run("ClearsMessagesRegardlessOfRawOrDeferredPaths", func(t *testing.T) {
		newBlueprint := func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{
				Messages: []blueprintv1alpha1.Message{
					{When: "dev == true", Text: "Grafana is ready"},
				},
			}
		}

		cases := map[string]struct {
			raw           bool
			deferredPaths map[string]bool
		}{
			"raw":              {raw: true, deferredPaths: map[string]bool{"substitutions.key": true}},
			"noDeferredPaths":  {raw: false, deferredPaths: nil},
			"withDeferredPath": {raw: false, deferredPaths: map[string]bool{"substitutions.key": true}},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				result := blueprint.RenderForDisplay(newBlueprint(), tc.raw, tc.deferredPaths)
				got := result.(*blueprintv1alpha1.Blueprint)
				if got.Messages != nil {
					t.Errorf("Expected Messages cleared, got %v", got.Messages)
				}
			})
		}
	})

	t.Run("RewritesDeferredTopLevelSubstitutionToPlaceholder", func(t *testing.T) {
		// Given a blueprint with a deferred top-level substitution
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{
				"private_dns": "${dns.private}",
				"public_dns":  "8.8.8.8",
			},
		}
		deferredPaths := map[string]bool{"substitutions.private_dns": true}

		// When rendering deferred placeholders
		result := blueprint.RenderForDisplay(bp, false, deferredPaths)

		// Then the deferred key becomes <deferred> and the resolved key is unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["private_dns"] != "<deferred>" {
			t.Errorf("Expected deferred substitution to be '<deferred>', got '%s'", got.Substitutions["private_dns"])
		}
		if got.Substitutions["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected resolved substitution to be unchanged, got '%s'", got.Substitutions["public_dns"])
		}
	})

	t.Run("RewritesDeferredConfigMapEntryToPlaceholder", func(t *testing.T) {
		// Given a blueprint with a deferred ConfigMap entry
		bp := &blueprintv1alpha1.Blueprint{
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"DEFERRED_KEY": "${terraform_output(\"x\")}",
					"RESOLVED_KEY": "resolved-value",
				},
			},
		}
		deferredPaths := map[string]bool{"configmaps.values-common.DEFERRED_KEY": true}

		// When rendering deferred placeholders
		result := blueprint.RenderForDisplay(bp, false, deferredPaths)

		// Then the deferred key becomes <deferred> and the resolved key is unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.ConfigMaps["values-common"]["DEFERRED_KEY"] != "<deferred>" {
			t.Errorf("Expected deferred ConfigMap entry to be '%s', got '%s'", "<deferred>", got.ConfigMaps["values-common"]["DEFERRED_KEY"])
		}
		if got.ConfigMaps["values-common"]["RESOLVED_KEY"] != "resolved-value" {
			t.Errorf("Expected resolved ConfigMap entry unchanged, got '%s'", got.ConfigMaps["values-common"]["RESOLVED_KEY"])
		}
	})

	t.Run("RewritesDeferredFluxSystemInstallAndResourcesSubstitutionsToPlaceholder", func(t *testing.T) {
		// Two resources variants share a substitution key name; only "internal"'s is deferred.
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "gateway",
					Install: &blueprintv1alpha1.Kustomization{
						Substitutions: map[string]string{
							"cluster_name": "${terraform_output('cluster', 'cluster_name')}",
							"resolved_key": "already-resolved",
						},
					},
					Resources: []blueprintv1alpha1.FluxVariant{
						{
							Kustomization: blueprintv1alpha1.Kustomization{
								Name: "internal",
								Substitutions: map[string]string{
									"gateway_lb_ip": "${terraform_output('network', 'internal_lb_ip')}",
								},
							},
						},
						{
							Kustomization: blueprintv1alpha1.Kustomization{
								Name: "external",
								Substitutions: map[string]string{
									"gateway_lb_ip": "already-resolved",
								},
							},
						},
					},
				},
			},
		}
		deferredPaths := map[string]bool{
			"flux.gateway.install.substitutions.cluster_name":             true,
			"flux.gateway.resources-internal.substitutions.gateway_lb_ip": true,
		}

		// When rendering deferred placeholders
		result := blueprint.RenderForDisplay(bp, false, deferredPaths)

		// Then the deferred install/resources keys become <deferred>, unrelated keys are unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		sys := got.FluxSystems[0]
		if sys.Install.Substitutions["cluster_name"] != "<deferred>" {
			t.Errorf("Expected deferred install substitution to be '%s', got '%s'", "<deferred>", sys.Install.Substitutions["cluster_name"])
		}
		if sys.Install.Substitutions["resolved_key"] != "already-resolved" {
			t.Errorf("Expected resolved install substitution unchanged, got '%s'", sys.Install.Substitutions["resolved_key"])
		}
		if sys.Resources[0].Substitutions["gateway_lb_ip"] != "<deferred>" {
			t.Errorf("Expected deferred 'internal' resources substitution to be '%s', got '%s'", "<deferred>", sys.Resources[0].Substitutions["gateway_lb_ip"])
		}
		if sys.Resources[1].Substitutions["gateway_lb_ip"] != "already-resolved" {
			t.Errorf("Expected 'external' resources substitution unchanged, got '%s'", sys.Resources[1].Substitutions["gateway_lb_ip"])
		}
	})

	t.Run("DoesNotMutateOriginalBlueprint", func(t *testing.T) {
		// Given a blueprint with a deferred substitution
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"private_dns": "${dns.private}"},
		}
		deferredPaths := map[string]bool{"substitutions.private_dns": true}

		// When rendering
		blueprint.RenderForDisplay(bp, false, deferredPaths)

		// Then the original blueprint is not mutated
		if bp.Substitutions["private_dns"] != "${dns.private}" {
			t.Errorf("Original blueprint was mutated: got '%s'", bp.Substitutions["private_dns"])
		}
	})

	t.Run("ReplacesDeferredValuesAndDropsMessages", func(t *testing.T) {
		// Given a blueprint with a deferred input, a deferred substitution, and a message
		bp := &blueprintv1alpha1.Blueprint{
			Messages:            []blueprintv1alpha1.Message{{Text: "note"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "c", Path: "c", Inputs: map[string]any{"vpc": "${x}", "plain": "y"}}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "k", Substitutions: map[string]string{"vpc": "${x}"}}},
		}
		deferred := map[string]bool{"terraform.c.inputs.vpc": true, "kustomize.k.substitutions.vpc": true}

		// When it is rendered for display
		got := blueprint.RenderForDisplay(bp, false, deferred).(*blueprintv1alpha1.Blueprint)

		// Then deferred values show a placeholder, messages are cleared, and the input is unchanged
		if got.TerraformComponents[0].Inputs["vpc"] != "<deferred>" || got.TerraformComponents[0].Inputs["plain"] != "y" {
			t.Errorf("unexpected inputs %v", got.TerraformComponents[0].Inputs)
		}
		if got.Kustomizations[0].Substitutions["vpc"] != "<deferred>" || got.Messages != nil {
			t.Errorf("unexpected display blueprint %+v", got)
		}
		if bp.TerraformComponents[0].Inputs["vpc"] != "${x}" || bp.Messages == nil {
			t.Error("expected the input blueprint unchanged")
		}
	})

	t.Run("KeepsDeferredExpressionsWhenRaw", func(t *testing.T) {
		// Given a blueprint with a deferred input
		bp := &blueprintv1alpha1.Blueprint{TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "c", Path: "c", Inputs: map[string]any{"vpc": "${x}"}}}}

		// When it is rendered raw
		got := blueprint.RenderForDisplay(bp, true, map[string]bool{"terraform.c.inputs.vpc": true}).(*blueprintv1alpha1.Blueprint)

		// Then the expression is kept
		if got.TerraformComponents[0].Inputs["vpc"] != "${x}" {
			t.Errorf("expected the expression, got %v", got.TerraformComponents[0].Inputs["vpc"])
		}
	})

	t.Run("PassesOtherTypesThrough", func(t *testing.T) {
		// Given a value that is not a blueprint or kustomization
		value := map[string]string{"a": "b"}

		// When it is rendered
		got := blueprint.RenderForDisplay(value, false, nil)

		// Then it is returned unchanged
		if !reflect.DeepEqual(got, value) {
			t.Errorf("expected %v, got %v", value, got)
		}
	})
}
