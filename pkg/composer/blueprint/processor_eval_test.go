package blueprint_test

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessGlobally_Requires(t *testing.T) {
	t.Run("SatisfiedByContextValues", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"cluster": map[string]any{"name": "mycluster"},
				"dns":     map[string]any{"domain": "example.com"},
			}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"cluster.name", "dns.domain"}},
				},
			},
		}
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error when requirements are satisfied, got %v", err)
		}
	})

	t.Run("ConditionalBlockSkippedWhenFalse", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "docker"}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "aws-only"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{When: "platform == 'aws'", Paths: []string{"aws.region"}},
				},
			},
		}
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error; aws block should be skipped, got %v", err)
		}
	})

	t.Run("UnsatisfiedRequirementProducesAggregatedError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"dns.domain", "tags"}},
				},
			},
		}
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"feature.enabled", "replicas"}},
				},
			},
		}
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected false/0 to satisfy requirement, got error: %v", err)
		}
	})

	t.Run("CrossFacetConfigBlockSatisfiesAfterDeferral", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "incus", "cluster": map[string]any{"name": "demo"}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "consumer"},
				Ordinal:  ptr(200),
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"talos.cluster_name"}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "config-talos"},
				Ordinal:  ptr(100),
				When:     "platform == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "talos", Body: map[string]any{"value": map[string]any{"cluster_name": "${cluster.name}"}}},
				},
			},
		}
		if _, err := processFacets(processor, target, facets); err != nil {
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "keeper"},
				Ordinal:  ptr(100),
				When:     "platform == 'incus'",
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "kconf", Body: map[string]any{"value": map[string]any{"ok": "yes"}}},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "selfref"},
				Ordinal:  ptr(200),
				When:     "platform == 'incus'",
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"sconf.ready"}},
				},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "sconf", Body: map[string]any{"value": map[string]any{"ready": "yes"}}},
				},
			},
		}
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected facet's own config to satisfy its requires across rounds, got: %v", err)
		}
	})

	t.Run("MessageRendersInNotesSection", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"observability": map[string]any{"enabled": true}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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

	t.Run("ConflictBlockFailsWhenTrue", func(t *testing.T) {
		// Given a block without paths whose when is true
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"gateway": map[string]any{
				"driver":   "cilium",
				"internal": map[string]any{"node_ports": map[string]any{"3010": 30310}},
			}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "option-gateway"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{
						When:    "gateway.driver == 'cilium' && len(gateway.internal.node_ports ?? {}) > 0",
						Message: "gateway.<name>.node_ports needs gateway.driver envoy.",
					},
				},
			},
		}

		// When the facets are processed
		_, err := processFacets(processor, target, facets)

		// Then the error shows the message under the conflict heading only
		if err == nil {
			t.Fatal("Expected conflict error, got nil")
		}
		want := "these values conflict:\n\n  - gateway.<name>.node_ports needs gateway.driver envoy."
		if err.Error() != want {
			t.Errorf("Expected %q, got %q", want, err.Error())
		}
	})

	t.Run("ConflictBlockSkippedWhenFalse", func(t *testing.T) {
		// Given a block without paths whose when is false
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"gateway": map[string]any{"driver": "envoy"}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "option-gateway"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{When: "gateway.driver == 'cilium'", Message: "cilium is not supported here."},
				},
			},
		}

		// When the facets are processed
		_, err := processFacets(processor, target, facets)

		// Then composition succeeds
		if err != nil {
			t.Fatalf("Expected no error when the conflict when is false, got %v", err)
		}
	})

	t.Run("MissingAndConflictSectionsBothRender", func(t *testing.T) {
		// Given one facet with a missing path and another with a true conflict block
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet-a"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{Paths: []string{"aws.region"}, Message: "Set the AWS region."},
				},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "facet-b"},
				Requires: []blueprintv1alpha1.RequirementBlock{
					{When: "platform == 'aws'", Message: "This option does not run on aws."},
				},
			},
		}

		// When the facets are processed
		_, err := processFacets(processor, target, facets)

		// Then the error lists the missing section and then the conflict section
		if err == nil {
			t.Fatal("Expected aggregated error, got nil")
		}
		want := "the following required values are not set in values.yaml:\n\n" +
			"  - aws.region\n\n" +
			"Notes:\n\n" +
			"  aws.region:\n" +
			"    Set the AWS region.\n\n" +
			"these values conflict:\n\n" +
			"  - This option does not run on aws."
		if err.Error() != want {
			t.Errorf("Expected:\n%s\ngot:\n%s", want, err.Error())
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		scope, err := processFacets(processor, target, facets)
		if err != nil {
			t.Fatalf("Expected facet's own config to satisfy its requires across rounds, got: %v", err)
		}
		self, ok := scope["self"].(map[string]any)
		if !ok || self["k"] != "demo" {
			t.Errorf("Expected self.k to resolve to 'demo', got: %#v", scope["self"])
		}
	})
}

func TestProcessor_ProcessGlobally_ComponentRequires(t *testing.T) {
	t.Run("TerraformComponentRequiresSatisfied", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"platform": "aws",
				"aws":      map[string]any{"cilium_role_arn": "arn:aws:iam::123:role/cilium"},
				"cni":      map[string]any{"driver": "cilium"},
			}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error when component requires are satisfied, got %v", err)
		}
	})

	t.Run("TerraformComponentRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error when component is excluded by When, got %v", err)
		}
	})

	t.Run("KustomizationRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"gateway": map[string]any{"enabled": true, "driver": "envoy"}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error when kustomization is excluded by When, got %v", err)
		}
	})

	t.Run("ConfigBlockRequiresUnsatisfiedProducesError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cluster": map[string]any{"driver": "talos"}, "talos": map[string]any{"tls": true}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
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
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected convergence to satisfy the requirement, got %v", err)
		}
	})

	t.Run("ComponentRequiresWhenSkipsBlockButComponentStillIncluded", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws", "cni": map[string]any{"driver": "cilium"}}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		if _, err := processFacets(processor, target, facets); err != nil {
			t.Fatalf("Expected no error when the block's own When is false, got %v", err)
		}
	})

	t.Run("ComponentWhenErrorIsSurfaced", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "aws"}, nil
		}
		processor := blueprint.NewBlueprintProcessor(mocks.Runtime)
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
		_, err := processFacets(processor, target, facets)
		if err == nil {
			t.Fatal("Expected error from invalid component When")
		}
		if !strings.Contains(err.Error(), "terraform component when") {
			t.Errorf("Expected error to mention terraform component when, got: %v", err)
		}
	})
}
