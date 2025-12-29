package v1alpha1

import (
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

func TestFeatureDeepCopy(t *testing.T) {
	t.Run("ReturnsNilForNilFeature", func(t *testing.T) {
		var f *Feature
		result := f.DeepCopy()
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("CreatesDeepCopyOfFeature", func(t *testing.T) {
		original := &Feature{
			Kind:       "Feature",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata: Metadata{
				Name:        "test-feature",
				Description: "Test feature",
			},
			When: "provider == 'aws'",
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

func TestFeatureYAMLTags(t *testing.T) {
	t.Run("FeatureMarshalsAndUnmarshalsYAML", func(t *testing.T) {
		feature := Feature{
			Kind:       "Feature",
			ApiVersion: "blueprints.windsorcli.dev/v1alpha1",
			Metadata: Metadata{
				Name:        "test-feature",
				Description: "Test feature description",
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

		data, err := yaml.Marshal(&feature)
		if err != nil {
			t.Fatalf("Failed to marshal Feature struct to YAML: %v", err)
		}

		var out Feature
		err = yaml.Unmarshal(data, &out)
		if err != nil {
			t.Fatalf("Failed to unmarshal YAML into Feature struct: %v", err)
		}

		if out.Kind != feature.Kind {
			t.Errorf("Expected Kind %q, got %q after YAML unmarshal", feature.Kind, out.Kind)
		}
		if out.ApiVersion != feature.ApiVersion {
			t.Errorf("Expected ApiVersion %q, got %q after YAML unmarshal", feature.ApiVersion, out.ApiVersion)
		}
		if out.Metadata.Name != feature.Metadata.Name {
			t.Errorf("Expected Metadata.Name %q, got %q after YAML unmarshal", feature.Metadata.Name, out.Metadata.Name)
		}
		if out.Metadata.Description != feature.Metadata.Description {
			t.Errorf("Expected Metadata.Description %q, got %q after YAML unmarshal", feature.Metadata.Description, out.Metadata.Description)
		}
		if out.When != feature.When {
			t.Errorf("Expected When %q, got %q after YAML unmarshal", feature.When, out.When)
		}
		if len(out.TerraformComponents) != len(feature.TerraformComponents) {
			t.Errorf("Expected %d TerraformComponents, got %d after YAML unmarshal", len(feature.TerraformComponents), len(out.TerraformComponents))
		}
		if len(out.Kustomizations) != len(feature.Kustomizations) {
			t.Errorf("Expected %d Kustomizations, got %d after YAML unmarshal", len(feature.Kustomizations), len(out.Kustomizations))
		}
	})

	t.Run("FeatureUnmarshalsDurationStrings", func(t *testing.T) {
		featureYAML := []byte(`kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-feature
  description: Test feature with durations
kustomize:
  - name: test-kustomization
    path: test/path
    interval: 5m
    retryInterval: 2m
    timeout: 10m
`)

		var feature Feature
		err := yaml.Unmarshal(featureYAML, &feature)
		if err != nil {
			t.Fatalf("Failed to unmarshal Feature with duration strings: %v", err)
		}

		if len(feature.Kustomizations) != 1 {
			t.Fatalf("Expected 1 kustomization, got %d", len(feature.Kustomizations))
		}

		k := feature.Kustomizations[0]
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
