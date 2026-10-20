package blueprint

import (
	"fmt"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessFacets_Requires(t *testing.T) {
	t.Run("SatisfiedByContextValues", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"cluster": map[string]any{"name": "mycluster"},
				"dns":     map[string]any{"domain": "example.com"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"cluster.name", "dns.domain"}},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when requirements are satisfied, got %v", err)
		}
	})

	t.Run("ConditionalBlockSkippedWhenFalse", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "docker"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "aws-only"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{When: "platform == 'aws'", Paths: []string{"aws.region"}},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error; aws block should be skipped, got %v", err)
		}
	})

	t.Run("UnsatisfiedRequirementProducesAggregatedError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"aws.region", "aws.account_id"}},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for unsatisfied requirements, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "the following required values are not set in values.yaml:") {
			t.Errorf("Expected lead instruction, got: %s", msg)
		}
		if !strings.Contains(msg, "- aws.account_id\n  - aws.region") {
			t.Errorf("Expected both paths listed alphabetically as bullets, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "platform == 'aws'") {
			t.Errorf("Expected error to not surface condition expression, got: %s", msg)
		}
		if strings.Contains(msg, "facet") {
			t.Errorf("Expected error to not mention 'facet', got: %s", msg)
		}
	})

	t.Run("EmptyValueIsTreatedAsMissing", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"dns":  map[string]any{"domain": ""},
				"tags": []any{},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"dns.domain", "tags"}},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for empty values, got nil")
		}
		if !strings.Contains(err.Error(), "dns.domain") || !strings.Contains(err.Error(), "tags") {
			t.Errorf("Expected both empty paths reported, got: %s", err.Error())
		}
	})

	t.Run("FalseAndZeroAreTreatedAsPresent", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"feature":  map[string]any{"enabled": false},
				"replicas": 0,
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"feature.enabled", "replicas"}},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected false/0 to satisfy requirement, got error: %v", err)
		}
	})

	t.Run("CrossFacetConfigBlockSatisfiesAfterDeferral", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "incus", "cluster": map[string]any{"name": "demo"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Ordinal:  intPtr(200),
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"talos.cluster_name"}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-talos"},
				Ordinal:  intPtr(100),
				When:     "platform == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", Body: map[string]any{"value": map[string]any{"cluster_name": "${cluster.name}"}}},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected requirement to be satisfied via config block in later round, got: %v", err)
		}
	})

	t.Run("FacetOwnConfigSatisfiesItsRequiresAcrossRounds", func(t *testing.T) {
		// Given an active facet whose requires (sconf.ready) is satisfied only by its own config
		// block, alongside another active facet that keeps the multi-pass loop running. The
		// self-referencing facet is pending in round one; its config must still merge so the value
		// lands in scope and satisfies the requires in round two. If the merge were gated behind
		// the requires check, the value never appears and composition fails. This mirrors ../core's
		// hetzner facet, which derives hetzner.token from env("HCLOUD_TOKEN") in its own config and
		// requires hetzner.token — the guardrail for keeping the config merge ahead of the check.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "incus"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "keeper"},
				Ordinal:  intPtr(100),
				When:     "platform == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "kconf", Body: map[string]any{"value": map[string]any{"ok": "yes"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "selfref"},
				Ordinal:  intPtr(200),
				When:     "platform == 'incus'",
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"sconf.ready"}},
				},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "sconf", Body: map[string]any{"value": map[string]any{"ready": "yes"}}},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected facet's own config to satisfy its requires across rounds, got: %v", err)
		}
	})

	t.Run("MessageRendersInNotesSection", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"observability": map[string]any{"enabled": true}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "obs"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{
						When:    "observability.enabled",
						Paths:   []string{"observability.endpoint", "observability.token"},
						Message: "See docs/observability for setup.",
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		msg := err.Error()
		if strings.Contains(msg, "Because") {
			t.Errorf("Expected no 'Because' heading in new format, got: %s", msg)
		}
		if !strings.Contains(msg, "\nNotes:\n") {
			t.Errorf("Expected 'Notes:' section, got: %s", msg)
		}
		if !strings.Contains(msg, "  observability.endpoint:\n    See docs/observability for setup.") {
			t.Errorf("Expected message in Notes under first path, got: %s", msg)
		}
		if !strings.Contains(msg, "  observability.token:\n    See docs/observability for setup.") {
			t.Errorf("Expected message in Notes under second path, got: %s", msg)
		}
	})

	t.Run("AggregatesAcrossMultipleFacets", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet-a"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"cluster.name"}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet-b"},
				When:     "platform == 'aws'",
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"aws.region"}},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected aggregated error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "- aws.region") || !strings.Contains(msg, "- cluster.name") {
			t.Errorf("Expected both paths as bullets, got: %s", msg)
		}
		if strings.Contains(msg, "platform == 'aws'") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("SelfReferenceSatisfiedAcrossRounds", func(t *testing.T) {
		// A lone facet requiring a path only its own config block produces is the same shape as
		// FacetOwnConfigSatisfiesItsRequiresAcrossRounds above, just without a second facet to keep
		// the round loop going past round one. The loop must keep running on its own until the
		// resolved scope itself stabilizes, not stop as soon as the (here, empty) included-facet set
		// repeats — otherwise this and the multi-facet case resolve inconsistently for no reason a
		// blueprint author controls.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "incus", "cluster": map[string]any{"name": "demo"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "self-referencing"},
				When:     "platform == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "self", Body: map[string]any{"value": map[string]any{"k": "${cluster.name}"}}},
				},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"self.k"}},
				},
			},
		}
		scope, err := processor.ProcessFacets(target, facets)
		if err != nil {
			t.Fatalf("Expected facet's own config to satisfy its requires across rounds, got: %v", err)
		}
		self, ok := scope["self"].(map[string]any)
		if !ok || self["k"] != "demo" {
			t.Errorf("Expected self.k to resolve to 'demo', got: %#v", scope["self"])
		}
	})
}

func TestProcessor_ProcessFacets_ComponentRequires(t *testing.T) {
	t.Run("TerraformComponentRequiresSatisfied", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"platform": "aws",
				"aws":      map[string]any{"cilium_role_arn": "arn:aws:iam::123:role/cilium"},
				"cni":      map[string]any{"driver": "cilium"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"aws.cilium_role_arn"}},
						},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when component requires are satisfied, got %v", err)
		}
	})

	t.Run("TerraformComponentRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"aws.cilium_role_arn"}},
						},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for unsatisfied component requirement")
		}
		msg := err.Error()
		if !strings.Contains(msg, "aws.cilium_role_arn") {
			t.Errorf("Expected missing path listed, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "platform == 'aws'") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("TerraformComponentRequiresSkippedWhenComponentExcluded", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "calico"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"aws.cilium_role_arn"}},
						},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when component is excluded by When, got %v", err)
		}
	})

	t.Run("KustomizationRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"gateway": map[string]any{"enabled": true, "driver": "envoy"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway"},
				When:     "gateway.enabled == true",
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{Name: "gateway", Path: "gateway"},
						When:          "gateway.driver == 'envoy'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"gateway.cert_arn"}},
						},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for unsatisfied kustomization requirement")
		}
		msg := err.Error()
		if !strings.Contains(msg, "gateway.cert_arn") {
			t.Errorf("Expected missing path listed, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "gateway.enabled") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("KustomizationRequiresSkippedWhenExcluded", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"gateway": map[string]any{"enabled": true, "driver": "cilium"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway"},
				When:     "gateway.enabled == true",
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{
						Kustomization: blueprintv1alpha1.Kustomization{Name: "gateway", Path: "gateway"},
						When:          "gateway.driver == 'envoy'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"gateway.cert_arn"}},
						},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when kustomization is excluded by When, got %v", err)
		}
	})

	t.Run("ConfigBlockRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cluster": map[string]any{"driver": "talos"}, "talos": map[string]any{"tls": true}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-talos"},
				When:     "cluster.driver == 'talos'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos",
						When: "talos.tls == true",
						Body: map[string]any{"value": "x"},
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"talos.ca_cert"}},
						},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error for unsatisfied config-block requirement")
		}
		msg := err.Error()
		if !strings.Contains(msg, "talos.ca_cert") {
			t.Errorf("Expected missing path listed, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "cluster.driver") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("ConfigBlockRequiresSkippedWhenExcluded", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cluster": map[string]any{"driver": "talos"}, "talos": map[string]any{"tls": false}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-talos"},
				When:     "cluster.driver == 'talos'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "talos",
						When: "talos.tls == true",
						Body: map[string]any{"value": "x"},
						Requires: []blueprintv1alpha1.RequirementBlock{
							{Paths: []string{"talos.ca_cert"}},
						},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when config block is excluded by When, got %v", err)
		}
	})

	t.Run("MultipleComponentsProduceDistinctBuckets", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"platform": "aws",
				"cni":      map[string]any{"driver": "cilium"},
				"dns":      map[string]any{"public_domain": "example.com"},
			}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires:           []blueprintv1alpha1.RequirementBlock{{Paths: []string{"aws.cilium_role_arn"}}},
					},
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "dns/aws-zone"},
						When:               "dns.public_domain != ''",
						Requires:           []blueprintv1alpha1.RequirementBlock{{Paths: []string{"aws.public_zone_id"}}},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected aggregated error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "- aws.cilium_role_arn") || !strings.Contains(msg, "- aws.public_zone_id") {
			t.Errorf("Expected both paths as bullets, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "cni.driver") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("FacetAndComponentRequiresAggregateTogether", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"aws.region"}},
				},
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires:           []blueprintv1alpha1.RequirementBlock{{Paths: []string{"aws.cilium_role_arn"}}},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected aggregated error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "- aws.cilium_role_arn") || !strings.Contains(msg, "- aws.region") {
			t.Errorf("Expected both paths as bullets, got: %s", msg)
		}
		if strings.Contains(msg, "Because") || strings.Contains(msg, "platform == 'aws'") {
			t.Errorf("Expected no condition expression in output, got: %s", msg)
		}
	})

	t.Run("CrossFacetConfigBlockSatisfiesComponentRequire", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		producerOrdinal := 100
		consumerOrdinal := 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "aws-defaults"},
				Ordinal:  &producerOrdinal,
				When:     "platform == 'aws'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "aws", Body: map[string]any{"value": map[string]any{"cilium_role_arn": "arn:aws:iam::123:role/cilium"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				Ordinal:  &consumerOrdinal,
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires:           []blueprintv1alpha1.RequirementBlock{{Paths: []string{"aws.cilium_role_arn"}}},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected convergence to satisfy the requirement, got %v", err)
		}
	})

	t.Run("ComponentRequiresWhenSkipsBlockButComponentStillIncluded", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "cluster/aws-cilium"},
						When:               "cni.driver == 'cilium'",
						Requires: []blueprintv1alpha1.RequirementBlock{
							{When: "aws.advanced == true", Paths: []string{"aws.advanced_role_arn"}},
						},
					},
				},
			},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("Expected no error when the block's own When is false, got %v", err)
		}
	})

	t.Run("ComponentWhenErrorIsSurfaced", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "platform-aws"},
				When:     "platform == 'aws'",
				TerraformComponents: []blueprintv1alpha1.ConditionalTerraformComponent{
					{
						TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "broken"},
						When:               "this is not a valid expression!!!",
						Requires:           []blueprintv1alpha1.RequirementBlock{{Paths: []string{"x"}}},
					},
				},
			},
		}
		_, err := processor.ProcessFacets(target, facets)
		if err == nil {
			t.Fatal("Expected error from invalid component When")
		}
		if !strings.Contains(err.Error(), "terraform component when") {
			t.Errorf("Expected error to mention terraform component when, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestBaseBlueprintProcessor_evaluateSubstitutions(t *testing.T) {
	t.Run("PreservesUnresolvedExpressions", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			expr := expression
			if strings.HasPrefix(expression, "${") && strings.HasSuffix(expression, "}") {
				expr = expression[2 : len(expression)-1]
			}
			if expr == "terraform_output('cluster', 'key')" {
				return fmt.Sprintf("${%s}", expr), nil
			}
			if strings.Contains(expr, "terraform_output") {
				return fmt.Sprintf("${%s}", expr), nil
			}
			if !strings.Contains(expression, "${") {
				return expression, nil
			}
			return "interpolated-value", nil
		}

		subs := map[string]string{
			"deferred": "${terraform_output('cluster', 'key')}",
			"normal":   "value",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, _, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["deferred"] != "${terraform_output('cluster', 'key')}" {
			t.Errorf("Expected deferred substitution to preserve original expression, got %v", result["deferred"])
		}

		if result["normal"] != "value" {
			t.Errorf("Expected normal substitution to be preserved, got %v", result["normal"])
		}
	})

	t.Run("MarksUntrackedUnresolvedExpressionAsDeferred", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		subs := map[string]string{
			"stillUnresolved": "${database_effective.cloud_driver == 'cloudsql' ? terraform_output('crossplane-identity', 'x') : ''}",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, deferredKeys, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !deferredKeys["stillUnresolved"] {
			t.Error("Expected key to be marked deferred when the evaluated result still contains an expression, even without a DeferredValue wrapper")
		}

		if result["stillUnresolved"] != subs["stillUnresolved"] {
			t.Errorf("Expected the unresolved expression text to be preserved, got %q", result["stillUnresolved"])
		}
	})

	t.Run("IncludesEmptyStringForNilResults", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${undefined.path}" {
				return nil, nil
			}
			return "resolved", nil
		}

		subs := map[string]string{
			"undefined": "${undefined.path}",
			"defined":   "${defined.path}",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, _, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["undefined"] != "" {
			t.Errorf("Expected undefined substitution to be empty string, got %q", result["undefined"])
		}

		if result["defined"] != "resolved" {
			t.Errorf("Expected defined substitution to be 'resolved', got %v", result["defined"])
		}
	})

	t.Run("CoalesceOperatorReturnsDefaultForUndefinedPath", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == `${addons.observability.admin_password ?? "grafana"}` {
				return "grafana", nil
			}
			return "resolved", nil
		}

		subs := map[string]string{
			"password": `${addons.observability.admin_password ?? "grafana"}`,
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, _, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result["password"] != "grafana" {
			t.Errorf("Expected 'grafana' from coalesce operator, got %q", result["password"])
		}
	})

	t.Run("AllKeysIncludedEvenWhenUndefined", func(t *testing.T) {
		mocks := setupProcessorMocks(t)

		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			switch expression {
			case "${dns.domain}":
				return nil, nil
			case "${timezone}":
				return nil, nil
			case "${defined.value}":
				return "exists", nil
			default:
				return nil, nil
			}
		}

		subs := map[string]string{
			"domain":   "${dns.domain}",
			"timezone": "${timezone}",
			"defined":  "${defined.value}",
		}

		baseProcessor := &BaseBlueprintProcessor{
			runtime:   mocks.Runtime,
			evaluator: mocks.Evaluator,
		}
		result, _, err := baseProcessor.evaluateSubstitutions(subs, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := result["domain"]; !exists {
			t.Error("Expected 'domain' key to exist in result")
		}
		if _, exists := result["timezone"]; !exists {
			t.Error("Expected 'timezone' key to exist in result")
		}
		if result["defined"] != "exists" {
			t.Errorf("Expected 'exists', got %q", result["defined"])
		}
	})
}

func TestEvaluateSubstitutions_RejectsSensitiveReference(t *testing.T) {
	t.Run("RejectsSensitiveReferenceWholeAndEmbedded", func(t *testing.T) {
		// Given substitutions that reference a sensitive property whole-string and embedded
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.IsSensitivePathFunc = func(path string) bool { return path == "cdn.cloudflare_api_key" }
		processor := NewBlueprintProcessor(mocks.Runtime)

		// Then each form fails closed before the value is resolved
		for name, value := range map[string]string{
			"Whole":       "${cdn.cloudflare_api_key}",
			"Prefixed":    "https://api.example.com/${cdn.cloudflare_api_key}",
			"Suffixed":    "${cdn.cloudflare_api_key}-suffix",
			"MidSentence": "token=${cdn.cloudflare_api_key};v=1",
		} {
			_, _, err := processor.evaluateSubstitutions(map[string]string{"FOO": value}, "", nil)
			if err == nil || !strings.Contains(err.Error(), "references sensitive property") {
				t.Errorf("%s: expected sensitive-substitution error for %q, got %v", name, value, err)
			}
		}
	})

	t.Run("AllowsNonSensitiveReference", func(t *testing.T) {
		// Given a substitution referencing a non-sensitive property
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.IsSensitivePathFunc = func(path string) bool { return false }
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cdn": map[string]any{"zone": "example.com"}}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating, it resolves normally
		result, _, err := processor.evaluateSubstitutions(map[string]string{"ZONE": "${cdn.zone}"}, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["ZONE"] != "example.com" {
			t.Errorf("Expected resolved value, got %v", result)
		}
	})

	t.Run("AllowsPlainLiteral", func(t *testing.T) {
		// Given a plaintext literal substitution (not a bare reference)
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.IsSensitivePathFunc = func(path string) bool { return true }
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating, the literal is preserved and the guard does not fire
		result, _, err := processor.evaluateSubstitutions(map[string]string{"REGION": "us-east-1"}, "", nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result["REGION"] != "us-east-1" {
			t.Errorf("Expected literal preserved, got %v", result)
		}
	})
}

func TestEvaluateCondition_EvaluatesAgainstScope(t *testing.T) {
	t.Run("PlatformEqualsIncusWhenScopeHasPlainValue", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		scope := map[string]any{"platform": "incus", "provider": "incus"}
		got, err := processor.evaluateCondition("platform == 'incus'", "", scope)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !got {
			t.Error("Expected condition to be true when platform is 'incus'")
		}
	})

	t.Run("PlatformEqualsFailsWhenValueDiffers", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		scope := map[string]any{"platform": "docker"}
		got, err := processor.evaluateCondition("platform == 'incus'", "", scope)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got {
			t.Error("Expected condition to be false when platform is 'docker'")
		}
	})

	t.Run("ReturnsNamedErrorWhenExpressionEvaluatesToDeferredValue", func(t *testing.T) {
		// Given a when: expression that resolves to a deferred value
		mocks := setupProcessorMocks(t)
		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return evaluator.DeferredValue{Expression: "terraform_output('compute', 'host_cpu')"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating the condition
		got, err := processor.evaluateCondition("terraform_output('compute', 'host_cpu') > 4", "", nil)

		// Then it returns a named error identifying the unresolved expression, not a type-name leak
		if got {
			t.Error("Expected false result on error")
		}
		if err == nil {
			t.Fatal("Expected an error")
		}
		if !strings.Contains(err.Error(), "not yet resolved") {
			t.Errorf("Expected error to name the unresolved-value cause, got: %v", err)
		}
		if strings.Contains(err.Error(), "evaluator.DeferredValue") {
			t.Errorf("Expected error not to leak the internal Go type name, got: %v", err)
		}
		if !strings.Contains(err.Error(), "terraform_output('compute', 'host_cpu')") {
			t.Errorf("Expected error to include the deferred expression text, got: %v", err)
		}
	})

	t.Run("ReturnsNamedErrorWhenExpressionEvaluatesToDeferredValuePointer", func(t *testing.T) {
		// Given a when: expression that resolves to a deferred value in its pointer form
		mocks := setupProcessorMocks(t)
		mocks.Evaluator.EvaluateFunc = func(expression string, featurePath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return &evaluator.DeferredValue{Expression: "terraform_output('pki', 'cert')"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating the condition
		got, err := processor.evaluateCondition("terraform_output('pki', 'cert') != ''", "", nil)

		// Then it returns the same named not-yet-resolved error as the value form
		if got {
			t.Error("Expected false result on error")
		}
		if err == nil || !strings.Contains(err.Error(), "not yet resolved") {
			t.Errorf("Expected a named not-yet-resolved error, got: %v", err)
		}
	})
}

func TestProcessor_evaluateStringSlice(t *testing.T) {
	t.Run("ReturnsNilForEmptySlice", func(t *testing.T) {
		// Given an empty slice
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty slice
		result, err := processor.evaluateStringSlice([]string{}, "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty slice, got %v", result)
		}
	})

	t.Run("PreservesEmptyStrings", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return expression, nil
		}

		result, err := processor.evaluateStringSlice([]string{"value1", "", "value2", ""}, "", nil)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 4 {
			t.Errorf("Expected 4 values (empty preserved), got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "" || result[2] != "value2" || result[3] != "" {
			t.Errorf("Expected ['value1', '', 'value2', ''], got %v", result)
		}
	})

	t.Run("HandlesNilEvaluationResult", func(t *testing.T) {
		// Given a slice with expressions that evaluate to nil
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "nil" {
				return nil, nil
			}
			return expression, nil
		}

		// When evaluating slice with nil results
		result, err := processor.evaluateStringSlice([]string{"value1", "nil", "value2"}, "", nil)

		// Then nil results should be filtered
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 values, got %d: %v", len(result), result)
		}
	})

	t.Run("FlattensArrayResults", func(t *testing.T) {
		// Given a slice with expression that evaluates to array
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${array}" {
				return []any{"item1", "item2", "item3"}, nil
			}
			return expression, nil
		}

		// When evaluating slice with array expression
		result, err := processor.evaluateStringSlice([]string{"value1", "${array}", "value2"}, "", nil)

		// Then array should be flattened
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 5 {
			t.Errorf("Expected 5 values (flattened array), got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "item1" || result[2] != "item2" || result[3] != "item3" || result[4] != "value2" {
			t.Errorf("Expected ['value1', 'item1', 'item2', 'item3', 'value2'], got %v", result)
		}
	})

	t.Run("HandlesNonStringTypes", func(t *testing.T) {
		// Given a slice with expression that evaluates to non-string
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${number}" {
				return 42, nil
			}
			if expression == "${bool}" {
				return true, nil
			}
			return expression, nil
		}

		// When evaluating slice with non-string expressions
		result, err := processor.evaluateStringSlice([]string{"value1", "${number}", "${bool}"}, "", nil)

		// Then non-string values should be converted to strings
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 values, got %d: %v", len(result), result)
		}
		if result[0] != "value1" || result[1] != "42" || result[2] != "true" {
			t.Errorf("Expected ['value1', '42', 'true'], got %v", result)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given a slice with expression that fails
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if expression == "${error}" {
				return nil, fmt.Errorf("evaluation failed")
			}
			return expression, nil
		}

		// When evaluating slice with failing expression
		result, err := processor.evaluateStringSlice([]string{"value1", "${error}"}, "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

func TestProcessor_evaluateBooleanExpression(t *testing.T) {
	t.Run("ReturnsNilForEmptyExpression", func(t *testing.T) {
		// Given an empty expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty expression
		result, err := processor.evaluateBooleanExpression("", "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty expression, got %v", result)
		}
	})

	t.Run("EvaluatesBooleanTrue", func(t *testing.T) {
		// Given a boolean true expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return true, nil
		}

		// When evaluating boolean expression
		result, err := processor.evaluateBooleanExpression("enabled == true", "", nil)

		// Then should return true
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("EvaluatesBooleanFalse", func(t *testing.T) {
		// Given a boolean false expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return false, nil
		}

		// When evaluating boolean expression
		result, err := processor.evaluateBooleanExpression("enabled == false", "", nil)

		// Then should return false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("EvaluatesStringInteger", func(t *testing.T) {
		// Given a string integer expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "42", nil
		}

		// When evaluating string integer expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidStringInteger", func(t *testing.T) {
		// Given a string that's not a valid integer
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "not-a-number", nil
		}

		// When evaluating invalid string integer expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid string integer")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected integer") {
			t.Errorf("Expected error about expected integer, got: %v", err)
		}
	})

	t.Run("EvaluatesStringTrue", func(t *testing.T) {
		// Given a string "true" expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "true", nil
		}

		// When evaluating string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return true
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("EvaluatesStringFalse", func(t *testing.T) {
		// Given a string "false" expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "false", nil
		}

		// When evaluating string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return false
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidString", func(t *testing.T) {
		// Given a string that's not "true" or "false"
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "maybe", nil
		}

		// When evaluating invalid string expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid string")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected boolean") {
			t.Errorf("Expected error about expected boolean, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidType", func(t *testing.T) {
		// Given an expression that evaluates to invalid type
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42, nil
		}

		// When evaluating invalid type expression
		result, err := processor.evaluateBooleanExpression("enabled", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid type")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected boolean") {
			t.Errorf("Expected error about expected boolean, got: %v", err)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given an expression that fails to evaluate
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, fmt.Errorf("evaluation failed")
		}

		// When evaluating failing expression
		result, err := processor.evaluateBooleanExpression("invalid", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

func TestProcessor_evaluateIntegerExpression(t *testing.T) {
	t.Run("ReturnsNilForEmptyExpression", func(t *testing.T) {
		// Given an empty expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		// When evaluating empty expression
		result, err := processor.evaluateIntegerExpression("", "", nil)

		// Then should return nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil for empty expression, got %v", result)
		}
	})

	t.Run("EvaluatesInt", func(t *testing.T) {
		// Given an int expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42, nil
		}

		// When evaluating int expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("EvaluatesInt64", func(t *testing.T) {
		// Given an int64 expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return int64(42), nil
		}

		// When evaluating int64 expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("EvaluatesFloat64", func(t *testing.T) {
		// Given a float64 expression
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return 42.5, nil
		}

		// When evaluating float64 expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return int (truncated)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if result == nil || *result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidType", func(t *testing.T) {
		// Given an expression that evaluates to invalid type
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return "not a number", nil
		}

		// When evaluating invalid type expression
		result, err := processor.evaluateIntegerExpression("workers", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid type")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
		if !strings.Contains(err.Error(), "expected integer") {
			t.Errorf("Expected error about expected integer, got: %v", err)
		}
	})

	t.Run("ReturnsErrorOnEvaluationFailure", func(t *testing.T) {
		// Given an expression that fails to evaluate
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		mocks.Evaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			return nil, fmt.Errorf("evaluation failed")
		}

		// When evaluating failing expression
		result, err := processor.evaluateIntegerExpression("invalid", "", nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for evaluation failure")
		}
		if result != nil {
			t.Error("Expected nil result on error")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestFormatRequirementsError(t *testing.T) {
	t.Run("PathsListedAlphabetically", func(t *testing.T) {
		pending := map[string]facetRequirementMisses{
			"a": {Misses: []requirementBlockMiss{
				{Paths: []string{"cluster.name"}},
			}},
			"b": {Misses: []requirementBlockMiss{
				{Paths: []string{"aws.region"}},
			}},
		}
		msg := formatRequirementsError(pending).Error()
		idxAws := strings.Index(msg, "- aws.region")
		idxCluster := strings.Index(msg, "- cluster.name")
		if idxAws < 0 || idxCluster < 0 {
			t.Fatalf("Missing bullets in: %s", msg)
		}
		if idxAws > idxCluster {
			t.Error("Expected aws.region to appear before cluster.name (alphabetical)")
		}
	})

	t.Run("DuplicatePathAcrossFacetsRendersOnce", func(t *testing.T) {
		pending := map[string]facetRequirementMisses{
			"a": {Misses: []requirementBlockMiss{
				{Paths: []string{"aws.region"}},
			}},
			"b": {Misses: []requirementBlockMiss{
				{Paths: []string{"aws.region"}},
			}},
		}
		msg := formatRequirementsError(pending).Error()
		if c := strings.Count(msg, "- aws.region"); c != 1 {
			t.Errorf("Expected one bullet for aws.region (deduplicated), got %d. Msg: %s", c, msg)
		}
	})

	t.Run("NotesSectionRendersAuthorMessage", func(t *testing.T) {
		pending := map[string]facetRequirementMisses{
			"obs": {Misses: []requirementBlockMiss{
				{Message: "See docs/obs.", Paths: []string{"observability.token"}},
			}},
		}
		msg := formatRequirementsError(pending).Error()
		if !strings.Contains(msg, "\nNotes:\n") {
			t.Errorf("Expected 'Notes:' section, got: %s", msg)
		}
		if !strings.Contains(msg, "  observability.token:\n    See docs/obs.") {
			t.Errorf("Expected '<path>:' header with indented message, got: %s", msg)
		}
	})

	t.Run("NotesSectionOmittedWhenNoMessages", func(t *testing.T) {
		pending := map[string]facetRequirementMisses{
			"a": {Misses: []requirementBlockMiss{
				{Paths: []string{"cluster.name"}},
			}},
		}
		msg := formatRequirementsError(pending).Error()
		if strings.Contains(msg, "Notes:") {
			t.Errorf("Expected no 'Notes:' section when no messages, got: %s", msg)
		}
	})

	t.Run("ErrorDoesNotMentionFacet", func(t *testing.T) {
		pending := map[string]facetRequirementMisses{
			"facet-name-here": {Misses: []requirementBlockMiss{
				{Paths: []string{"cluster.name"}},
			}},
		}
		msg := formatRequirementsError(pending).Error()
		if strings.Contains(msg, "facet") {
			t.Errorf("Expected error not to mention 'facet', got: %s", msg)
		}
	})
}

func TestCombineConditions(t *testing.T) {
	t.Run("NoPartsYieldsEmpty", func(t *testing.T) {
		if got := combineConditions(); got != "" {
			t.Errorf("Expected empty, got %q", got)
		}
	})

	t.Run("AllEmptyYieldsEmpty", func(t *testing.T) {
		if got := combineConditions("", "  ", ""); got != "" {
			t.Errorf("Expected empty, got %q", got)
		}
	})

	t.Run("SingleNonEmptyReturnsItTrimmed", func(t *testing.T) {
		if got := combineConditions("", "platform == 'aws'", ""); got != "platform == 'aws'" {
			t.Errorf("Unexpected: %q", got)
		}
	})

	t.Run("MultipleNonEmptyJoinedWithAnd", func(t *testing.T) {
		got := combineConditions("platform == 'aws'", "cluster.driver == 'eks'", "dns.public_domain != ''")
		want := "platform == 'aws' && cluster.driver == 'eks' && dns.public_domain != ''"
		if got != want {
			t.Errorf("Got %q, want %q", got, want)
		}
	})

	t.Run("MixedEmptyAndNonEmptyDropsEmpties", func(t *testing.T) {
		got := combineConditions("platform == 'aws'", "", "cni.driver == 'cilium'")
		want := "platform == 'aws' && cni.driver == 'cilium'"
		if got != want {
			t.Errorf("Got %q, want %q", got, want)
		}
	})
}
