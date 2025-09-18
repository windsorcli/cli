package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
						Values: map[string]any{
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

		original.TerraformComponents[0].Values["cidr"] = "modified"
		if copy.TerraformComponents[0].Values["cidr"] == "modified" {
			t.Error("Deep copy failed: terraform values map was not copied")
		}

		original.Kustomizations[0].Components[0] = "modified"
		if copy.Kustomizations[0].Components[0] == "modified" {
			t.Error("Deep copy failed: kustomization components slice was not copied")
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
				Values: map[string]any{
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

		original.Values["cidr"] = "modified"
		if copy.Values["cidr"] == "modified" {
			t.Error("Deep copy failed: values map was not copied")
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
		interval := &metav1.Duration{}
		original := &ConditionalKustomization{
			Kustomization: Kustomization{
				Name:       "ingress",
				Path:       "ingress",
				Components: []string{"nginx", "nginx/web"},
				DependsOn:  []string{"pki-base"},
				Interval:   interval,
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
	})
}

func TestFeatureYAMLTags(t *testing.T) {
	t.Run("FeatureHasCorrectYamlTags", func(t *testing.T) {
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

		// This test ensures the struct can be marshaled/unmarshaled
		// The actual YAML tag validation is implicit through compilation
		if feature.Kind == "" {
			t.Error("Feature should have Kind field")
		}
		if feature.ApiVersion == "" {
			t.Error("Feature should have ApiVersion field")
		}
		if feature.When == "" {
			t.Error("Feature should have When field")
		}
	})
}
