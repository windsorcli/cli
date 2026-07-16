package blueprint

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRenderDeferredPlaceholders(t *testing.T) {
	t.Run("ReturnsResourceUnchangedWhenRawMode", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := RenderDeferredPlaceholders(bp, true, map[string]bool{"substitutions.key": true})
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected raw value preserved, got '%s'", got.Substitutions["key"])
		}
	})

	t.Run("ReturnsResourceUnchangedWhenNoDeferredPaths", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"key": "${unresolved}"},
		}
		result := RenderDeferredPlaceholders(bp, false, nil)
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["key"] != "${unresolved}" {
			t.Errorf("Expected value unchanged with empty deferred paths, got '%s'", got.Substitutions["key"])
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
		result := RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the deferred key becomes <deferred> and the resolved key is unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.Substitutions["private_dns"] != deferredPlaceholder {
			t.Errorf("Expected deferred substitution to be '%s', got '%s'", deferredPlaceholder, got.Substitutions["private_dns"])
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
		result := RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the deferred key becomes <deferred> and the resolved key is unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		if got.ConfigMaps["values-common"]["DEFERRED_KEY"] != deferredPlaceholder {
			t.Errorf("Expected deferred ConfigMap entry to be '%s', got '%s'", deferredPlaceholder, got.ConfigMaps["values-common"]["DEFERRED_KEY"])
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
		result := RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the deferred install/resources keys become <deferred>, unrelated keys are unchanged
		got := result.(*blueprintv1alpha1.Blueprint)
		sys := got.FluxSystems[0]
		if sys.Install.Substitutions["cluster_name"] != deferredPlaceholder {
			t.Errorf("Expected deferred install substitution to be '%s', got '%s'", deferredPlaceholder, sys.Install.Substitutions["cluster_name"])
		}
		if sys.Install.Substitutions["resolved_key"] != "already-resolved" {
			t.Errorf("Expected resolved install substitution unchanged, got '%s'", sys.Install.Substitutions["resolved_key"])
		}
		if sys.Resources[0].Substitutions["gateway_lb_ip"] != deferredPlaceholder {
			t.Errorf("Expected deferred 'internal' resources substitution to be '%s', got '%s'", deferredPlaceholder, sys.Resources[0].Substitutions["gateway_lb_ip"])
		}
		if sys.Resources[1].Substitutions["gateway_lb_ip"] != "already-resolved" {
			t.Errorf("Expected 'external' resources substitution unchanged, got '%s'", sys.Resources[1].Substitutions["gateway_lb_ip"])
		}
	})

	t.Run("RewritesDeferredFlatFluxSystemSubstitutionsToPlaceholder", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{
				{
					Name: "gateway-cilium",
					Flat: &blueprintv1alpha1.Kustomization{
						Substitutions: map[string]string{
							"gateway_ip":   "${terraform_output('network', 'gateway_ip')}",
							"resolved_key": "already-resolved",
						},
					},
				},
			},
		}
		deferredPaths := map[string]bool{
			"flux.gateway-cilium.substitutions.gateway_ip": true,
		}

		result := RenderDeferredPlaceholders(bp, false, deferredPaths)

		got := result.(*blueprintv1alpha1.Blueprint)
		sys := got.FluxSystems[0]
		if sys.Flat.Substitutions["gateway_ip"] != deferredPlaceholder {
			t.Errorf("Expected deferred flat substitution to be '%s', got '%s'", deferredPlaceholder, sys.Flat.Substitutions["gateway_ip"])
		}
		if sys.Flat.Substitutions["resolved_key"] != "already-resolved" {
			t.Errorf("Expected resolved flat substitution unchanged, got '%s'", sys.Flat.Substitutions["resolved_key"])
		}
	})

	t.Run("DoesNotMutateOriginalBlueprint", func(t *testing.T) {
		// Given a blueprint with a deferred substitution
		bp := &blueprintv1alpha1.Blueprint{
			Substitutions: map[string]string{"private_dns": "${dns.private}"},
		}
		deferredPaths := map[string]bool{"substitutions.private_dns": true}

		// When rendering
		RenderDeferredPlaceholders(bp, false, deferredPaths)

		// Then the original blueprint is not mutated
		if bp.Substitutions["private_dns"] != "${dns.private}" {
			t.Errorf("Original blueprint was mutated: got '%s'", bp.Substitutions["private_dns"])
		}
	})
}
