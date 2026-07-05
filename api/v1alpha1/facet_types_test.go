package v1alpha1

import (
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFacetDeepCopy(t *testing.T) {
	t.Run("ReturnsNilForNilFacet", func(t *testing.T) {
		var f *Facet
		result := f.DeepCopy()
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("CreatesDeepCopyOfFacet", func(t *testing.T) {
		original := &Facet{
			Kind:       "Facet",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata: Metadata{
				Name:        "test-facet",
				Description: "Test facet",
			},
			When: "provider == 'aws'",
			Config: []ConfigBlock{
				{
					Name: "talos",
					When: "provider == 'incus'",
					Body: map[string]any{
						"value": map[string]any{
							"controlplanes": "${cluster.controlplanes}",
							"patchVars":     map[string]any{"certSANs": "original", "poolPath": "default"},
						},
					},
				},
			},
			TerraformComponents: []ConditionalTerraformComponent{
				{
					TerraformComponent: TerraformComponent{
						Path:      "network/aws-vpc",
						DependsOn: []string{"policy-base"},
						Inputs: map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
					When: "vpc.enabled == true",
				},
			},
			Kustomizations: []ConditionalKustomization{
				{
					Kustomization: Kustomization{
						Name:       "ingress",
						Path:       "ingress",
						Components: []string{"nginx", "nginx/web"},
						DependsOn:  []string{"pki-base"},
						Substitutions: map[string]string{
							"host": "example.com",
						},
					},
					When: "ingress.enabled == true",
				},
			},
		}

		copy := original.DeepCopy()

		if copy.Kind != original.Kind {
			t.Errorf("Expected Kind %s, got %s", original.Kind, copy.Kind)
		}
		if copy.ApiVersion != original.ApiVersion {
			t.Errorf("Expected ApiVersion %s, got %s", original.ApiVersion, copy.ApiVersion)
		}
		if copy.Metadata.Name != original.Metadata.Name {
			t.Errorf("Expected Name %s, got %s", original.Metadata.Name, copy.Metadata.Name)
		}
		if copy.When != original.When {
			t.Errorf("Expected When %s, got %s", original.When, copy.When)
		}

		// Verify deep copy by modifying original
		original.Metadata.Description = "modified"
		if copy.Metadata.Description == "modified" {
			t.Error("Deep copy failed: metadata was not copied")
		}

		original.TerraformComponents[0].Inputs["cidr"] = "modified"
		if copy.TerraformComponents[0].Inputs["cidr"] == "modified" {
			t.Error("Deep copy failed: terraform inputs map was not copied")
		}

		original.Kustomizations[0].Components[0] = "modified"
		if copy.Kustomizations[0].Components[0] == "modified" {
			t.Error("Deep copy failed: kustomization components slice was not copied")
		}

		original.Kustomizations[0].Substitutions["host"] = "modified"
		if copy.Kustomizations[0].Substitutions["host"] == "modified" {
			t.Error("Deep copy failed: kustomization substitutions map was not copied")
		}

		// Verify top-level Substitutions independence (set after initial copy to test the map)
		original.Substitutions = map[string]string{"dns": "10.0.0.1"}
		copy2 := original.DeepCopy()
		original.Substitutions["dns"] = "mutated"
		if copy2.Substitutions["dns"] != "10.0.0.1" {
			t.Error("Deep copy failed: top-level Substitutions map was not copied independently")
		}

		if len(copy.Config) != len(original.Config) {
			t.Error("Deep copy failed: config slice length mismatch")
		}
		valOrig := original.Config[0].Body["value"].(map[string]any)
		valOrig["controlplanes"] = "modified"
		if copy.Config[0].Body["value"].(map[string]any)["controlplanes"] == "modified" {
			t.Error("Deep copy failed: config block body was not copied")
		}
		nested := valOrig["patchVars"].(map[string]any)
		nested["certSANs"] = "modified"
		if copy.Config[0].Body["value"].(map[string]any)["patchVars"].(map[string]any)["certSANs"] == "modified" {
			t.Error("Deep copy failed: config block body nested map was not copied")
		}
	})

	t.Run("PreservesBackend", func(t *testing.T) {
		// Given a facet that names the backend tier terminus
		original := &Facet{Backend: "cluster"}

		// When deep-copied
		copy := original.DeepCopy()

		// Then the Backend field round-trips
		if copy.Backend != "cluster" {
			t.Errorf("Expected copy.Backend=\"cluster\", got %q", copy.Backend)
		}
	})

	t.Run("PreservesRequiresIndependently", func(t *testing.T) {
		original := &Facet{
			Metadata: Metadata{Name: "test-facet"},
			Requires: []RequirementBlock{
				{Paths: []string{"cluster.name", "dns.domain"}},
				{
					When:    "provider == 'aws'",
					Paths:   []string{"aws.region"},
					Message: "Set aws.region",
				},
			},
		}

		copy := original.DeepCopy()

		if len(copy.Requires) != len(original.Requires) {
			t.Fatalf("Expected %d requirement blocks, got %d", len(original.Requires), len(copy.Requires))
		}

		original.Requires[0].Paths[0] = "modified"
		if copy.Requires[0].Paths[0] == "modified" {
			t.Error("Deep copy failed: requires paths slice was not copied")
		}

		original.Requires[1].Message = "modified"
		if copy.Requires[1].Message == "modified" {
			t.Error("Deep copy failed: requires message was not copied")
		}
	})
}

func TestRequirementBlockDeepCopy(t *testing.T) {
	t.Run("ReturnsNilForNilBlock", func(t *testing.T) {
		var r *RequirementBlock
		result := r.DeepCopy()
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("CreatesDeepCopyOfRequirementBlock", func(t *testing.T) {
		original := &RequirementBlock{
			When:    "provider == 'aws'",
			Paths:   []string{"aws.region", "aws.account_id"},
			Message: "Set these in values.yaml",
		}

		copy := original.DeepCopy()

		if copy.When != original.When {
			t.Errorf("Expected When %q, got %q", original.When, copy.When)
		}
		if copy.Message != original.Message {
			t.Errorf("Expected Message %q, got %q", original.Message, copy.Message)
		}
		if len(copy.Paths) != len(original.Paths) {
			t.Fatalf("Expected %d paths, got %d", len(original.Paths), len(copy.Paths))
		}

		original.Paths[0] = "modified"
		if copy.Paths[0] == "modified" {
			t.Error("Deep copy failed: paths slice was not copied")
		}
	})
}

func TestRequirementBlockUnmarshalYAML(t *testing.T) {
	t.Run("ErrorsWhenPathsKeyMissing", func(t *testing.T) {
		// Models a typo such as `pahts:` — the field is absent entirely, which would
		// otherwise produce a silently-disabled requirement.
		var f Facet
		err := yaml.Unmarshal([]byte(`requires:
  - when: provider == 'aws'
    message: missing the paths key entirely
`), &f)
		if err == nil {
			t.Fatal("Expected error when paths is missing, got nil")
		}
		if !strings.Contains(err.Error(), "paths is required") {
			t.Errorf("Expected error to mention 'paths is required', got: %v", err)
		}
	})

	t.Run("ErrorsWhenPathsListEmpty", func(t *testing.T) {
		var f Facet
		err := yaml.Unmarshal([]byte(`requires:
  - paths: []
`), &f)
		if err == nil {
			t.Fatal("Expected error when paths is empty, got nil")
		}
		if !strings.Contains(err.Error(), "paths is required") {
			t.Errorf("Expected error to mention 'paths is required', got: %v", err)
		}
	})

	t.Run("AcceptsValidBlock", func(t *testing.T) {
		var f Facet
		err := yaml.Unmarshal([]byte(`requires:
  - paths:
      - cluster.name
`), &f)
		if err != nil {
			t.Fatalf("Expected no error for valid block, got: %v", err)
		}
		if len(f.Requires) != 1 || len(f.Requires[0].Paths) != 1 || f.Requires[0].Paths[0] != "cluster.name" {
			t.Errorf("Expected one requires entry with paths=[cluster.name], got: %+v", f.Requires)
		}
	})
}

func TestConditionalTerraformComponentDeepCopy(t *testing.T) {
	t.Run("ReturnsNilForNilComponent", func(t *testing.T) {
		var c *ConditionalTerraformComponent
		result := c.DeepCopy()
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("CreatesDeepCopyOfConditionalTerraformComponent", func(t *testing.T) {
		original := &ConditionalTerraformComponent{
			TerraformComponent: TerraformComponent{
				Path:      "network/aws-vpc",
				DependsOn: []string{"policy-base", "pki-base"},
				Inputs: map[string]any{
					"cidr":    "10.0.0.0/16",
					"subnets": []string{"10.0.1.0/24", "10.0.2.0/24"},
				},
			},
			When: "provider == 'aws'",
		}

		copy := original.DeepCopy()

		if copy.Path != original.Path {
			t.Errorf("Expected Path %s, got %s", original.Path, copy.Path)
		}
		if copy.When != original.When {
			t.Errorf("Expected When %s, got %s", original.When, copy.When)
		}

		// Verify deep copy by modifying original
		original.DependsOn[0] = "modified"
		if copy.DependsOn[0] == "modified" {
			t.Error("Deep copy failed: dependsOn slice was not copied")
		}

		original.Inputs["cidr"] = "modified"
		if copy.Inputs["cidr"] == "modified" {
			t.Error("Deep copy failed: inputs map was not copied")
		}
	})

	t.Run("PreservesRequiresIndependently", func(t *testing.T) {
		original := &ConditionalTerraformComponent{
			TerraformComponent: TerraformComponent{Path: "network/aws-vpc"},
			When:               "platform == 'aws'",
			Requires: []RequirementBlock{
				{Paths: []string{"aws.region"}},
				{When: "cni_effective.driver == 'cilium'", Paths: []string{"aws.cilium.role_arn"}, Message: "Set the cilium role arn."},
			},
		}

		copy := original.DeepCopy()

		if len(copy.Requires) != len(original.Requires) {
			t.Fatalf("Expected %d requirement blocks, got %d", len(original.Requires), len(copy.Requires))
		}

		original.Requires[0].Paths[0] = "modified"
		if copy.Requires[0].Paths[0] == "modified" {
			t.Error("Deep copy failed: requires paths slice was not copied")
		}

		original.Requires[1].Message = "modified"
		if copy.Requires[1].Message == "modified" {
			t.Error("Deep copy failed: requires message was not copied")
		}
	})
}

func TestConditionalKustomizationDeepCopy(t *testing.T) {
	t.Run("ReturnsNilForNilKustomization", func(t *testing.T) {
		var k *ConditionalKustomization
		result := k.DeepCopy()
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("CreatesDeepCopyOfConditionalKustomization", func(t *testing.T) {
		interval := &DurationString{}
		original := &ConditionalKustomization{
			Kustomization: Kustomization{
				Name:       "ingress",
				Path:       "ingress",
				Components: []string{"nginx", "nginx/web"},
				DependsOn:  []string{"pki-base"},
				Interval:   interval,
				Substitutions: map[string]string{
					"host": "example.com",
				},
			},
			When: "ingress.enabled == true",
		}

		copy := original.DeepCopy()

		if copy.Name != original.Name {
			t.Errorf("Expected Name %s, got %s", original.Name, copy.Name)
		}
		if copy.When != original.When {
			t.Errorf("Expected When %s, got %s", original.When, copy.When)
		}

		// Verify deep copy by modifying original
		original.Components[0] = "modified"
		if copy.Components[0] == "modified" {
			t.Error("Deep copy failed: components slice was not copied")
		}

		original.DependsOn[0] = "modified"
		if copy.DependsOn[0] == "modified" {
			t.Error("Deep copy failed: dependsOn slice was not copied")
		}

		original.Substitutions["host"] = "modified"
		if copy.Substitutions["host"] == "modified" {
			t.Error("Deep copy failed: substitutions map was not copied")
		}
	})

	t.Run("PreservesRequiresIndependently", func(t *testing.T) {
		original := &ConditionalKustomization{
			Kustomization: Kustomization{Name: "ingress"},
			When:          "ingress.enabled == true",
			Requires: []RequirementBlock{
				{Paths: []string{"ingress.class"}},
				{When: "ingress.tls == true", Paths: []string{"ingress.tls_secret"}, Message: "Set the TLS secret name."},
			},
		}

		copy := original.DeepCopy()

		if len(copy.Requires) != len(original.Requires) {
			t.Fatalf("Expected %d requirement blocks, got %d", len(original.Requires), len(copy.Requires))
		}

		original.Requires[0].Paths[0] = "modified"
		if copy.Requires[0].Paths[0] == "modified" {
			t.Error("Deep copy failed: requires paths slice was not copied")
		}

		original.Requires[1].Message = "modified"
		if copy.Requires[1].Message == "modified" {
			t.Error("Deep copy failed: requires message was not copied")
		}
	})

	t.Run("CopiesSubstituteIndependently", func(t *testing.T) {
		original := &ConditionalKustomization{
			Kustomization: Kustomization{
				Name:       "cert-manager",
				Path:       "pki/cert-manager",
				Substitute: map[string]string{"region": "us-east-1"},
			},
		}

		copy := original.DeepCopy()

		if copy.Substitute["region"] != "us-east-1" {
			t.Fatalf("Expected substitute copied, got %v", copy.Substitute)
		}
		original.Substitute["region"] = "modified"
		if copy.Substitute["region"] == "modified" {
			t.Error("Deep copy failed: substitute map was not copied")
		}
	})
}

func TestConfigBlockDeepCopy(t *testing.T) {
	t.Run("PreservesRequiresIndependently", func(t *testing.T) {
		original := &ConfigBlock{
			Name: "talos",
			When: "platform == 'docker'",
			Body: map[string]any{"value": "x"},
			Requires: []RequirementBlock{
				{Paths: []string{"talos.endpoint"}},
				{When: "talos.tls == true", Paths: []string{"talos.ca_cert"}, Message: "Set the CA cert path."},
			},
		}

		copy := original.DeepCopy()

		if len(copy.Requires) != len(original.Requires) {
			t.Fatalf("Expected %d requirement blocks, got %d", len(original.Requires), len(copy.Requires))
		}

		original.Requires[0].Paths[0] = "modified"
		if copy.Requires[0].Paths[0] == "modified" {
			t.Error("Deep copy failed: requires paths slice was not copied")
		}

		original.Requires[1].Message = "modified"
		if copy.Requires[1].Message == "modified" {
			t.Error("Deep copy failed: requires message was not copied")
		}
	})
}

func TestFacetYAMLTags(t *testing.T) {
	t.Run("FacetMarshalsAndUnmarshalsYAML", func(t *testing.T) {
		facet := Facet{
			Kind:       "Facet",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata: Metadata{
				Name:        "test-facet",
				Description: "Test facet description",
			},
			When: "provider == 'aws'",
			TerraformComponents: []ConditionalTerraformComponent{
				{
					TerraformComponent: TerraformComponent{
						Path: "network/aws-vpc",
					},
					When: "vpc.enabled == true",
				},
			},
			Kustomizations: []ConditionalKustomization{
				{
					Kustomization: Kustomization{
						Name: "ingress",
						Path: "ingress",
					},
					When: "ingress.enabled == true",
				},
			},
		}

		data, err := yaml.Marshal(&facet)
		if err != nil {
			t.Fatalf("Failed to marshal Facet struct to YAML: %v", err)
		}

		var out Facet
		err = yaml.Unmarshal(data, &out)
		if err != nil {
			t.Fatalf("Failed to unmarshal YAML into Facet struct: %v", err)
		}

		if out.Kind != facet.Kind {
			t.Errorf("Expected Kind %q, got %q after YAML unmarshal", facet.Kind, out.Kind)
		}
		if out.ApiVersion != facet.ApiVersion {
			t.Errorf("Expected ApiVersion %q, got %q after YAML unmarshal", facet.ApiVersion, out.ApiVersion)
		}
		if out.Metadata.Name != facet.Metadata.Name {
			t.Errorf("Expected Metadata.Name %q, got %q after YAML unmarshal", facet.Metadata.Name, out.Metadata.Name)
		}
		if out.Metadata.Description != facet.Metadata.Description {
			t.Errorf("Expected Metadata.Description %q, got %q after YAML unmarshal", facet.Metadata.Description, out.Metadata.Description)
		}
		if out.When != facet.When {
			t.Errorf("Expected When %q, got %q after YAML unmarshal", facet.When, out.When)
		}
		if len(out.TerraformComponents) != len(facet.TerraformComponents) {
			t.Errorf("Expected %d TerraformComponents, got %d after YAML unmarshal", len(facet.TerraformComponents), len(out.TerraformComponents))
		}
		if len(out.Kustomizations) != len(facet.Kustomizations) {
			t.Errorf("Expected %d Kustomizations, got %d after YAML unmarshal", len(facet.Kustomizations), len(out.Kustomizations))
		}
	})

	t.Run("FacetMarshalsAndUnmarshalsBackend", func(t *testing.T) {
		// Given a facet that names a backend tier terminus
		facet := Facet{
			Kind:       "Facet",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata:   Metadata{Name: "cluster-facet"},
			Backend:    "cluster",
		}

		data, err := yaml.Marshal(&facet)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if !strings.Contains(string(data), "backend: cluster") {
			t.Errorf("Expected YAML to contain \"backend: cluster\", got:\n%s", string(data))
		}

		var out Facet
		if err := yaml.Unmarshal(data, &out); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if out.Backend != "cluster" {
			t.Errorf("Expected Backend=\"cluster\" after round-trip, got %q", out.Backend)
		}
	})

	t.Run("FacetOmitsBackendWhenEmpty", func(t *testing.T) {
		facet := Facet{Metadata: Metadata{Name: "no-backend"}}
		data, err := yaml.Marshal(&facet)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if strings.Contains(string(data), "backend:") {
			t.Errorf("Expected empty Backend to be omitted, got:\n%s", string(data))
		}
	})

	t.Run("FacetUnmarshalsConfig", func(t *testing.T) {
		facetYAML := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: config-facet
when: provider == 'incus'
config:
  - name: talos
    when: provider == 'incus' || provider == 'docker'
    value:
      controlplanes: ${cluster.controlplanes}
      workers: ${cluster.workers}
      patchVars:
        certSANs: ${cluster.apiServer.certSANs}
terraform:
  - name: cluster
    path: cluster
    inputs:
      controlplanes: ${talos.controlplanes}
      common_patch: ${talos.common_patch}
`)
		var facet Facet
		err := yaml.Unmarshal(facetYAML, &facet)
		if err != nil {
			t.Fatalf("Failed to unmarshal facet with config: %v", err)
		}
		if len(facet.Config) != 1 {
			t.Fatalf("Expected 1 config block, got %d", len(facet.Config))
		}
		block := facet.Config[0]
		if block.Name != "talos" {
			t.Errorf("Expected config block name talos, got %q", block.Name)
		}
		if block.When != "provider == 'incus' || provider == 'docker'" {
			t.Errorf("Expected when condition, got %q", block.When)
		}
		val, ok := block.Body["value"].(map[string]any)
		if !ok {
			t.Fatalf("Expected config block body to have value map, got %T", block.Body["value"])
		}
		if _, ok := val["controlplanes"]; !ok {
			t.Error("Expected controlplanes in config block value")
		}
		if _, ok := val["patchVars"]; !ok {
			t.Error("Expected patchVars in config block value")
		}
	})

	t.Run("FacetMarshalsConfig", func(t *testing.T) {
		block := ConfigBlock{
			Name: "talos",
			When: "provider == 'docker'",
			Body: map[string]any{"value": map[string]any{"controlplanes": "${cluster.controlplanes}", "workers": "${cluster.workers}"}},
		}
		out, err := yaml.Marshal(&block)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, "name: talos") {
			t.Error("Expected marshaled YAML to contain name: talos")
		}
		if !strings.Contains(outStr, "when: provider == 'docker'") {
			t.Error("Expected marshaled YAML to contain when condition")
		}
		if !strings.Contains(outStr, "value:") {
			t.Error("Expected marshaled YAML to contain value key")
		}
		if !strings.Contains(outStr, "controlplanes") || !strings.Contains(outStr, "workers") {
			t.Error("Expected marshaled YAML to contain value body keys controlplanes and workers")
		}
		if !strings.Contains(outStr, "cluster.controlplanes") {
			t.Error("Expected marshaled YAML to contain body value reference")
		}
	})

	t.Run("ConfigBlockUnmarshalErrorsWhenValueMissing", func(t *testing.T) {
		y := []byte(`name: talos
when: provider == 'incus'
controlplanes: ${cluster.controlplanes}
`)
		var block ConfigBlock
		err := yaml.Unmarshal(y, &block)
		if err == nil {
			t.Error("Expected error when config block value is missing")
		}
		if block.Body != nil {
			t.Error("Expected Body to be nil when unmarshal fails")
		}
	})

	t.Run("FacetUnmarshalsRequires", func(t *testing.T) {
		facetYAML := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: requires-facet
when: provider == 'aws'
requires:
  - paths:
      - cluster.name
      - dns.domain
  - when: cluster.workers.count > 0
    paths:
      - aws.subnets.private
  - when: observability.enabled
    paths:
      - observability.endpoint
      - observability.token
    message: |
      Observability is enabled but its endpoint/token are not set.
      See https://docs.windsorcli.dev/observability for setup details.
`)
		var facet Facet
		err := yaml.Unmarshal(facetYAML, &facet)
		if err != nil {
			t.Fatalf("Failed to unmarshal facet with requires: %v", err)
		}
		if len(facet.Requires) != 3 {
			t.Fatalf("Expected 3 requirement blocks, got %d", len(facet.Requires))
		}

		first := facet.Requires[0]
		if first.When != "" {
			t.Errorf("Expected first block When empty, got %q", first.When)
		}
		if len(first.Paths) != 2 || first.Paths[0] != "cluster.name" || first.Paths[1] != "dns.domain" {
			t.Errorf("Unexpected first block paths: %v", first.Paths)
		}
		if first.Message != "" {
			t.Errorf("Expected first block Message empty, got %q", first.Message)
		}

		second := facet.Requires[1]
		if second.When != "cluster.workers.count > 0" {
			t.Errorf("Unexpected second block When: %q", second.When)
		}
		if len(second.Paths) != 1 || second.Paths[0] != "aws.subnets.private" {
			t.Errorf("Unexpected second block paths: %v", second.Paths)
		}

		third := facet.Requires[2]
		if third.When != "observability.enabled" {
			t.Errorf("Unexpected third block When: %q", third.When)
		}
		if len(third.Paths) != 2 {
			t.Fatalf("Expected 2 paths in third block, got %d", len(third.Paths))
		}
		if !strings.Contains(third.Message, "endpoint/token are not set") {
			t.Errorf("Expected message to contain context, got %q", third.Message)
		}
	})

	t.Run("FacetMarshalsRequires", func(t *testing.T) {
		facet := Facet{
			Kind:       "Facet",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata:   Metadata{Name: "test"},
			Requires: []RequirementBlock{
				{Paths: []string{"cluster.name"}},
				{When: "provider == 'aws'", Paths: []string{"aws.region"}, Message: "Set aws.region"},
			},
		}
		out, err := yaml.Marshal(&facet)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "requires:") {
			t.Error("Expected marshaled YAML to contain requires:")
		}
		if !strings.Contains(s, "cluster.name") {
			t.Error("Expected marshaled YAML to contain cluster.name path")
		}
		if !strings.Contains(s, "when: provider == 'aws'") {
			t.Error("Expected marshaled YAML to contain when: provider == 'aws'")
		}
		if !strings.Contains(s, "Set aws.region") {
			t.Error("Expected marshaled YAML to contain message text")
		}
	})

	t.Run("FacetUnmarshalsDurationStrings", func(t *testing.T) {
		facetYAML := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-facet
  description: Test facet with durations
kustomize:
  - name: test-kustomization
    path: test/path
    interval: 5m
    retryInterval: 2m
    timeout: 10m
`)

		var facet Facet
		err := yaml.Unmarshal(facetYAML, &facet)
		if err != nil {
			t.Fatalf("Failed to unmarshal Facet with duration strings: %v", err)
		}

		if len(facet.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(facet.Kustomizations))
		}

		k := facet.Kustomizations[0]
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

	t.Run("ConfigBlockUnmarshalsAndMarshalsRequires", func(t *testing.T) {
		y := []byte(`name: talos
when: platform == 'docker'
requires:
  - paths:
      - talos.endpoint
  - when: talos.tls == true
    paths:
      - talos.ca_cert
    message: |
      Set the CA cert path.
value:
  controlplanes: ${cluster.controlplanes}
`)
		var block ConfigBlock
		if err := yaml.Unmarshal(y, &block); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(block.Requires) != 2 {
			t.Fatalf("Expected 2 requirement blocks, got %d", len(block.Requires))
		}
		if block.Requires[0].When != "" || block.Requires[0].Paths[0] != "talos.endpoint" {
			t.Errorf("Unexpected first block: %+v", block.Requires[0])
		}
		if block.Requires[1].When != "talos.tls == true" || block.Requires[1].Paths[0] != "talos.ca_cert" {
			t.Errorf("Unexpected second block: %+v", block.Requires[1])
		}
		if !strings.Contains(block.Requires[1].Message, "Set the CA cert path.") {
			t.Errorf("Unexpected second block message: %q", block.Requires[1].Message)
		}

		out, err := yaml.Marshal(&block)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		outStr := string(out)
		if !strings.Contains(outStr, "requires:") {
			t.Errorf("Expected marshaled YAML to contain requires: %s", outStr)
		}
		if !strings.Contains(outStr, "talos.endpoint") || !strings.Contains(outStr, "talos.ca_cert") {
			t.Errorf("Expected marshaled YAML to contain both required paths: %s", outStr)
		}
	})

	t.Run("ConditionalTerraformComponentUnmarshalsRequires", func(t *testing.T) {
		y := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws
when: platform == 'aws'
terraform:
  - path: cluster/aws-cilium
    when: cni_effective.driver == 'cilium'
    requires:
      - paths:
          - aws.cilium.role_arn
        message: |
          Set the cilium role arn.
`)
		var facet Facet
		if err := yaml.Unmarshal(y, &facet); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(facet.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 terraform component, got %d", len(facet.TerraformComponents))
		}
		comp := facet.TerraformComponents[0]
		if len(comp.Requires) != 1 {
			t.Fatalf("Expected 1 requirement block on terraform component, got %d", len(comp.Requires))
		}
		if comp.Requires[0].Paths[0] != "aws.cilium.role_arn" {
			t.Errorf("Unexpected required path: %v", comp.Requires[0].Paths)
		}
	})

	t.Run("ConditionalKustomizationUnmarshalsRequires", func(t *testing.T) {
		y := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: gateway
when: gateway.enabled == true
kustomize:
  - name: gateway
    path: gateway
    when: gateway.driver == 'envoy'
    requires:
      - paths:
          - gateway.cert_arn
`)
		var facet Facet
		if err := yaml.Unmarshal(y, &facet); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(facet.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(facet.Kustomizations))
		}
		k := facet.Kustomizations[0]
		if len(k.Requires) != 1 {
			t.Fatalf("Expected 1 requirement block on kustomization, got %d", len(k.Requires))
		}
		if k.Requires[0].Paths[0] != "gateway.cert_arn" {
			t.Errorf("Unexpected required path: %v", k.Requires[0].Paths)
		}
	})
}

// flux: is a distinct collection of system entries; kustomize: remains the plain-Kustomization
// passthrough. The two do not merge.
func TestFacet_FluxSystems(t *testing.T) {
	t.Run("FluxKeyParsesSystemEntriesSeparateFromKustomize", func(t *testing.T) {
		src := `kind: Facet
kustomize:
  - name: gateway-cilium
    path: gateway/cilium
    components: [base]
flux:
  - name: gateway
    dependsOn: [pki]
    install:
      components: [envoy]
      substitute: {cluster: prod}
    resources:
      - name: internal
        when: "${x}"
        components: [internal]
`
		var f Facet
		if err := yaml.Unmarshal([]byte(src), &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// kustomize: stays in Kustomizations; flux: lands in FluxSystems; they are disjoint
		if len(f.Kustomizations) != 1 || f.Kustomizations[0].Name != "gateway-cilium" {
			t.Fatalf("kustomize entry wrong: %+v", f.Kustomizations)
		}
		if len(f.FluxSystems) != 1 {
			t.Fatalf("expected one flux system, got %+v", f.FluxSystems)
		}
		sys := f.FluxSystems[0]
		if sys.Name != "gateway" || len(sys.DependsOn) != 1 || sys.DependsOn[0] != "pki" {
			t.Fatalf("system descriptor wrong: %+v", sys)
		}
		if sys.Install == nil || len(sys.Install.Components) != 1 || sys.Install.Components[0] != "envoy" {
			t.Fatalf("install tier wrong: %+v", sys.Install)
		}
		if sys.Install.Substitute["cluster"] != "prod" {
			t.Fatalf("install substitute wrong: %+v", sys.Install.Substitute)
		}
		if len(sys.Resources) != 1 || sys.Resources[0].Name != "internal" || sys.Resources[0].When != "${x}" {
			t.Fatalf("resources variant wrong: %+v", sys.Resources)
		}
		if len(sys.Resources[0].Components) != 1 || sys.Resources[0].Components[0] != "internal" {
			t.Fatalf("variant components wrong: %+v", sys.Resources[0])
		}
	})
}
