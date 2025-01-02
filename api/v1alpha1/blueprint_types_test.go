package v1alpha1

import (
	"sort"
	"testing"
)

func TestBlueprintV1Alpha1_Merge(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
				Authors:     []string{"author1"},
			},
			Sources: []Source{
				{
					Name: "source1",
					Url:  "http://example.com/source1",
					Ref:  "v1.0.0",
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "path1",
					Variables: map[string]TerraformVariable{
						"var1": {Default: "default1"},
					},
					Values:   nil, // Set Values to nil to test initialization
					FullPath: "original/full/path",
				},
			},
		}

		src := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "updated",
				Description: "updated description",
				Authors:     []string{"author2"},
			},
			Sources: []Source{
				{
					Name: "source2",
					Url:  "http://example.com/source2",
					Ref:  "v2.0.0",
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "path1",
					Variables: map[string]TerraformVariable{
						"var2": {Default: "default2"},
					},
					Values: map[string]interface{}{
						"key2": "value2",
					},
					FullPath: "updated/full/path",
				},
				{
					Source: "source3",
					Path:   "path3",
					Variables: map[string]TerraformVariable{
						"var3": {Default: "default3"},
					},
					Values: map[string]interface{}{
						"key3": "value3",
					},
					FullPath: "new/full/path",
				},
			},
		}

		dst.Merge(src)

		if dst.Metadata.Name != "updated" {
			t.Errorf("Expected Metadata.Name to be 'updated', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "updated description" {
			t.Errorf("Expected Metadata.Description to be 'updated description', but got '%s'", dst.Metadata.Description)
		}
		if len(dst.Metadata.Authors) != 1 || dst.Metadata.Authors[0] != "author2" {
			t.Errorf("Expected Metadata.Authors to be ['author2'], but got %v", dst.Metadata.Authors)
		}

		expectedSources := map[string]Source{
			"source1": {Name: "source1", Url: "http://example.com/source1", Ref: "v1.0.0"},
			"source2": {Name: "source2", Url: "http://example.com/source2", Ref: "v2.0.0"},
		}
		if len(dst.Sources) != len(expectedSources) {
			t.Fatalf("Expected %d Sources, but got %d", len(expectedSources), len(dst.Sources))
		}
		for _, source := range dst.Sources {
			if expectedSource, exists := expectedSources[source.Name]; !exists || expectedSource != source {
				t.Errorf("Unexpected source found: %v", source)
			}
		}

		if len(dst.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 TerraformComponents, but got %d", len(dst.TerraformComponents))
		}

		// Sort TerraformComponents by Source to ensure consistent order
		sort.Slice(dst.TerraformComponents, func(i, j int) bool {
			return dst.TerraformComponents[i].Source < dst.TerraformComponents[j].Source
		})

		component1 := dst.TerraformComponents[0]
		if len(component1.Variables) != 2 || component1.Variables["var1"].Default != "default1" || component1.Variables["var2"].Default != "default2" {
			t.Errorf("Expected Variables to contain ['var1', 'var2'], but got %v", component1.Variables)
		}
		if component1.Values == nil || len(component1.Values) != 1 || component1.Values["key2"] != "value2" {
			t.Errorf("Expected Values to be overwritten and contain 'key2', but got %v", component1.Values)
		}
		if component1.FullPath != "updated/full/path" {
			t.Errorf("Expected FullPath to be 'updated/full/path', but got '%s'", component1.FullPath)
		}
		component2 := dst.TerraformComponents[1]
		if len(component2.Variables) != 1 || component2.Variables["var3"].Default != "default3" {
			t.Errorf("Expected Variables to be ['var3'], but got %v", component2.Variables)
		}
		if component2.Values == nil || len(component2.Values) != 1 || component2.Values["key3"] != "value3" {
			t.Errorf("Expected Values to contain 'key3', but got %v", component2.Values)
		}
		if component2.FullPath != "new/full/path" {
			t.Errorf("Expected FullPath to be 'new/full/path', but got '%s'", component2.FullPath)
		}
	})

	t.Run("NoMergeWhenSrcIsNil", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
				Authors:     []string{"author1"},
			},
		}

		dst.Merge(nil)

		if dst.Metadata.Name != "original" {
			t.Errorf("Expected Metadata.Name to remain 'original', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "original description" {
			t.Errorf("Expected Metadata.Description to remain 'original description', but got '%s'", dst.Metadata.Description)
		}
		if dst.Sources != nil {
			t.Errorf("Expected Sources to remain nil, but got %v", dst.Sources)
		}
		if dst.TerraformComponents != nil {
			t.Errorf("Expected TerraformComponents to remain nil, but got %v", dst.TerraformComponents)
		}
	})

	t.Run("MatchingPathNotSource", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Variables: map[string]TerraformVariable{
						"var1": {Default: "default1"},
					},
					Values: map[string]interface{}{
						"key1": "value1",
					},
					FullPath: "original/full/path",
				},
			},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source: "source2",      // Different source
					Path:   "module/path1", // Same path
					Variables: map[string]TerraformVariable{
						"var2": {Default: "default2"},
					},
					Values: map[string]interface{}{
						"key2": "value2",
					},
					FullPath: "updated/full/path",
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
		}

		component := dst.TerraformComponents[0]
		if component.Source != "source2" {
			t.Errorf("Expected Source to be 'source2', but got '%s'", component.Source)
		}
		if len(component.Variables) != 1 || component.Variables["var2"].Default != "default2" {
			t.Errorf("Expected Variables to be ['var2'], but got %v", component.Variables)
		}
		if component.Values == nil || len(component.Values) != 1 || component.Values["key2"] != "value2" {
			t.Errorf("Expected Values to contain 'key2', but got %v", component.Values)
		}
		if component.FullPath != "updated/full/path" {
			t.Errorf("Expected FullPath to be 'updated/full/path', but got '%s'", component.FullPath)
		}
	})

	t.Run("OverlayWithoutComponents", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Variables: map[string]TerraformVariable{
						"var1": {Default: "default1"},
					},
					Values: map[string]interface{}{
						"key1": "value1",
					},
					FullPath: "original/full/path",
				},
			},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{},
		}

		dst.Merge(overlay)

		if len(dst.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
		}

		component := dst.TerraformComponents[0]
		if component.Source != "source1" {
			t.Errorf("Expected Source to be 'source1', but got '%s'", component.Source)
		}
		if len(component.Variables) != 1 || component.Variables["var1"].Default != "default1" {
			t.Errorf("Expected Variables to be ['var1'], but got %v", component.Variables)
		}
		if component.Values == nil || len(component.Values) != 1 || component.Values["key1"] != "value1" {
			t.Errorf("Expected Values to contain 'key1', but got %v", component.Values)
		}
		if component.FullPath != "original/full/path" {
			t.Errorf("Expected FullPath to be 'original/full/path', but got '%s'", component.FullPath)
		}
	})

	t.Run("EmptyDstWithOverlayComponents", func(t *testing.T) {
		dst := &Blueprint{
			Kind:                "Blueprint",
			ApiVersion:          "v1alpha1",
			TerraformComponents: []TerraformComponent{},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Variables: map[string]TerraformVariable{
						"var1": {Default: "default1"},
					},
					Values: map[string]interface{}{
						"key1": "value1",
					},
					FullPath: "overlay/full/path",
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
		}

		component := dst.TerraformComponents[0]
		if component.Source != "source1" {
			t.Errorf("Expected Source to be 'source1', but got '%s'", component.Source)
		}
		if len(component.Variables) != 1 || component.Variables["var1"].Default != "default1" {
			t.Errorf("Expected Variables to be ['var1'], but got %v", component.Variables)
		}
		if component.Values == nil || len(component.Values) != 1 || component.Values["key1"] != "value1" {
			t.Errorf("Expected Values to contain 'key1', but got %v", component.Values)
		}
		if component.FullPath != "overlay/full/path" {
			t.Errorf("Expected FullPath to be 'overlay/full/path', but got '%s'", component.FullPath)
		}
	})
}

func TestBlueprintV1Alpha1_Copy(t *testing.T) {
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
					Ref:        "main",
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Variables: map[string]TerraformVariable{
						"var1": {
							Type:        "string",
							Default:     "default1",
							Description: "A test variable",
						},
					},
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
		if len(copy.TerraformComponents[0].Variables) != 1 || copy.TerraformComponents[0].Variables["var1"].Default != "default1" {
			t.Errorf("Expected copy to have terraform component variable 'var1' with default 'default1', but got %v", copy.TerraformComponents[0].Variables)
		}
	})

	t.Run("EmptyBlueprint", func(t *testing.T) {
		var blueprint *Blueprint
		copy := blueprint.DeepCopy()
		if copy != nil {
			t.Errorf("Expected copy to be nil, but got non-nil")
		}
	})
}
