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
						"controlplanes": "${cluster.controlplanes}",
						"patchVars":     map[string]any{"certSANs": "original", "poolPath": "default"},
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

		if len(copy.Config) != len(original.Config) {
			t.Error("Deep copy failed: config slice length mismatch")
		}
		original.Config[0].Body["controlplanes"] = "modified"
		if copy.Config[0].Body["controlplanes"] == "modified" {
			t.Error("Deep copy failed: config block body was not copied")
		}
		nested := original.Config[0].Body["patchVars"].(map[string]any)
		nested["certSANs"] = "modified"
		if copy.Config[0].Body["patchVars"].(map[string]any)["certSANs"] == "modified" {
			t.Error("Deep copy failed: config block body nested map was not copied")
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

	t.Run("FacetUnmarshalsConfig", func(t *testing.T) {
		facetYAML := []byte(`kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: config-facet
when: provider == 'incus'
config:
  - name: talos
    when: provider == 'incus' || provider == 'docker'
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
		if _, ok := block.Body["controlplanes"]; !ok {
			t.Error("Expected controlplanes in config block body")
		}
		if _, ok := block.Body["patchVars"]; !ok {
			t.Error("Expected patchVars in config block body")
		}
	})

	t.Run("FacetMarshalsConfig", func(t *testing.T) {
		block := ConfigBlock{
			Name: "talos",
			When: "provider == 'docker'",
			Body: map[string]any{"controlplanes": "${cluster.controlplanes}", "workers": "${cluster.workers}"},
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
		if !strings.Contains(outStr, "controlplanes") || !strings.Contains(outStr, "workers") {
			t.Error("Expected marshaled YAML to contain body keys controlplanes and workers")
		}
		if !strings.Contains(outStr, "cluster.controlplanes") {
			t.Error("Expected marshaled YAML to contain body value reference")
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
}
