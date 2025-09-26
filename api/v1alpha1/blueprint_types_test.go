package v1alpha1

import (
	"reflect"
	"sort"
	"testing"
)

func TestBlueprint_Merge(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
			},
			Repository: Repository{
				Url: "http://example.com/repo1",
				Ref: Reference{
					Branch: "main",
				},
			},
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "http://example.com/source1",
					PathPrefix: "prefix1",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source:    "source1",
					Path:      "module/path1",
					Values:    map[string]any{"key1": "value1"},
					FullPath:  "original/full/path",
					DependsOn: []string{},
					Destroy:   ptrBool(true),
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					PostBuild: &PostBuild{
						Substitute: map[string]string{"key1": "value1"},
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
						},
					},
				},
			},
		}

		overlay := &Blueprint{
			Metadata: Metadata{
				Name:        "updated",
				Description: "updated description",
			},
			Repository: Repository{
				Url: "http://example.com/repo2",
				Ref: Reference{
					Branch: "develop",
				},
			},
			Sources: []Source{
				{
					Name:       "source2",
					Url:        "http://example.com/source2",
					PathPrefix: "prefix2",
					Ref: Reference{
						Branch: "feature",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source:    "source1",
					Path:      "module/path1",
					Values:    map[string]any{"key2": "value2"},
					FullPath:  "updated/full/path",
					DependsOn: []string{"module/path2"},
					Destroy:   ptrBool(false),
				},
				{
					Source:    "source2",
					Path:      "module/path2",
					Values:    map[string]any{"key3": "value3"},
					FullPath:  "new/full/path",
					DependsOn: []string{},
					Destroy:   ptrBool(true),
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization2",
					Path:       "kustomize/path2",
					Components: []string{"component2"},
					PostBuild: &PostBuild{
						Substitute: map[string]string{"key2": "value2"},
						SubstituteFrom: []SubstituteReference{
							{Kind: "Secret", Name: "secret1"},
						},
					},
				},
			},
		}

		dst.Merge(overlay)

		if dst.Metadata.Name != "updated" {
			t.Errorf("Expected Metadata.Name to be 'updated', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "updated description" {
			t.Errorf("Expected Metadata.Description to be 'updated description', but got '%s'", dst.Metadata.Description)
		}

		expectedSources := map[string]Source{
			"source1": {
				Name:       "source1",
				Url:        "http://example.com/source1",
				PathPrefix: "prefix1",
				Ref: Reference{
					Branch: "main",
				},
			},
			"source2": {
				Name:       "source2",
				Url:        "http://example.com/source2",
				PathPrefix: "prefix2",
				Ref: Reference{
					Branch: "feature",
				},
			},
		}
		if len(dst.Sources) != len(expectedSources) {
			t.Fatalf("Expected %d Sources, but got %d", len(expectedSources), len(dst.Sources))
		}
		for _, source := range dst.Sources {
			if expectedSource, exists := expectedSources[source.Name]; !exists || expectedSource != source {
				t.Errorf("Unexpected source found: %v", source)
			}
		}

		if dst.Repository.Url != "http://example.com/repo2" {
			t.Errorf("Expected Repository.Url to be 'http://example.com/repo2', but got '%s'", dst.Repository.Url)
		}
		if dst.Repository.Ref.Branch != "develop" {
			t.Errorf("Expected Repository.Ref.Branch to be 'develop', but got '%s'", dst.Repository.Ref.Branch)
		}

		if len(dst.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 TerraformComponents, but got %d", len(dst.TerraformComponents))
		}

		sort.Slice(dst.TerraformComponents, func(i, j int) bool {
			return dst.TerraformComponents[i].Source < dst.TerraformComponents[j].Source
		})

		component1 := dst.TerraformComponents[0]
		if component1.Values == nil || len(component1.Values) != 2 || component1.Values["key1"] != "value1" || component1.Values["key2"] != "value2" {
			t.Errorf("Expected Values to contain both 'key1' and 'key2', but got %v", component1.Values)
		}
		if component1.FullPath != "updated/full/path" {
			t.Errorf("Expected FullPath to be 'updated/full/path', but got '%s'", component1.FullPath)
		}
		if len(component1.DependsOn) != 1 || component1.DependsOn[0] != "module/path2" {
			t.Errorf("Expected DependsOn to contain ['module/path2'], but got %v", component1.DependsOn)
		}
		if component1.Destroy == nil || *component1.Destroy != false {
			t.Errorf("Expected Destroy to be false, but got %v", component1.Destroy)
		}

		component2 := dst.TerraformComponents[1]
		if component2.Values == nil || len(component2.Values) != 1 || component2.Values["key3"] != "value3" {
			t.Errorf("Expected Values to contain 'key3', but got %v", component2.Values)
		}
		if component2.FullPath != "new/full/path" {
			t.Errorf("Expected FullPath to be 'new/full/path', but got '%s'", component2.FullPath)
		}
		if len(component2.DependsOn) != 0 {
			t.Errorf("Expected DependsOn to be empty, but got %v", component2.DependsOn)
		}
		if component2.Destroy == nil || *component2.Destroy != true {
			t.Errorf("Expected Destroy to be true, but got %v", component2.Destroy)
		}

		if len(dst.Kustomizations) != 1 {
			t.Fatalf("Expected 1 Kustomization, but got %d", len(dst.Kustomizations))
		}

		if dst.Kustomizations[0].Name != "kustomization2" {
			t.Errorf("Expected Kustomization to be 'kustomization2', but got '%s'", dst.Kustomizations[0].Name)
		}

		if dst.Kustomizations[0].Path != "kustomize/path2" {
			t.Errorf("Expected Kustomization Path to be 'kustomize/path2', but got '%s'", dst.Kustomizations[0].Path)
		}

		if !reflect.DeepEqual(dst.Kustomizations[0].Components, []string{"component2"}) {
			t.Errorf("Expected Kustomization Components to be ['component2'], but got %v", dst.Kustomizations[0].Components)
		}

		if dst.Kustomizations[0].PostBuild.Substitute["key2"] != "value2" {
			t.Errorf("Expected Kustomization PostBuild.Substitute to have 'key2:value2', but got %v", dst.Kustomizations[0].PostBuild.Substitute)
		}

		if dst.Kustomizations[0].PostBuild.SubstituteFrom[0].Kind != "Secret" || dst.Kustomizations[0].PostBuild.SubstituteFrom[0].Name != "secret1" {
			t.Errorf("Expected Kustomization PostBuild.SubstituteFrom to have 'Secret:secret1', but got %v", dst.Kustomizations[0].PostBuild.SubstituteFrom)
		}
	})

	t.Run("NilValues", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			TerraformComponents: []TerraformComponent{
				{
					Source:    "source1",
					Path:      "module/path1",
					Values:    nil, // Initialize with nil
					FullPath:  "original/full/path",
					DependsOn: []string{},
					Destroy:   ptrBool(true),
				},
			},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Values: map[string]any{
						"key1": "value1",
					},
					FullPath:  "overlay/full/path",
					DependsOn: []string{"dependency1"},
					Destroy:   ptrBool(false),
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
		}

		component := dst.TerraformComponents[0]
		if component.Values == nil || len(component.Values) != 1 || component.Values["key1"] != "value1" {
			t.Errorf("Expected Values to contain 'key1', but got %v", component.Values)
		}
		if component.FullPath != "overlay/full/path" {
			t.Errorf("Expected FullPath to be 'overlay/full/path', but got '%s'", component.FullPath)
		}
		if component.Destroy == nil || *component.Destroy != false {
			t.Errorf("Expected Destroy to be false, but got %v", component.Destroy)
		}
	})

	t.Run("NoMergeWhenSrcIsNil", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key1": "value1"},
					FullPath: "original/full/path",
					Destroy:  ptrBool(true),
				},
			},
		}

		dst.Merge(nil)

		if len(dst.TerraformComponents) != 1 {
			t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
		}

		component := dst.TerraformComponents[0]
		if component.Values == nil || len(component.Values) != 1 || component.Values["key1"] != "value1" {
			t.Errorf("Expected Values to contain 'key1', but got %v", component.Values)
		}
		if component.FullPath != "original/full/path" {
			t.Errorf("Expected FullPath to be 'original/full/path', but got '%s'", component.FullPath)
		}
		if component.Destroy == nil || *component.Destroy != true {
			t.Errorf("Expected Destroy to be true, but got %v", component.Destroy)
		}
	})

	t.Run("DestroyFieldMerge", func(t *testing.T) {
		tests := []struct {
			name     string
			dst      *bool
			overlay  *bool
			expected *bool
		}{
			{
				name:     "BothNil",
				dst:      nil,
				overlay:  nil,
				expected: nil,
			},
			{
				name:     "DstNilOverlayTrue",
				dst:      nil,
				overlay:  ptrBool(true),
				expected: ptrBool(true),
			},
			{
				name:     "DstNilOverlayFalse",
				dst:      nil,
				overlay:  ptrBool(false),
				expected: ptrBool(false),
			},
			{
				name:     "DstTrueOverlayNil",
				dst:      ptrBool(true),
				overlay:  nil,
				expected: ptrBool(true),
			},
			{
				name:     "DstFalseOverlayNil",
				dst:      ptrBool(false),
				overlay:  nil,
				expected: ptrBool(false),
			},
			{
				name:     "DstTrueOverlayTrue",
				dst:      ptrBool(true),
				overlay:  ptrBool(true),
				expected: ptrBool(true),
			},
			{
				name:     "DstTrueOverlayFalse",
				dst:      ptrBool(true),
				overlay:  ptrBool(false),
				expected: ptrBool(false),
			},
			{
				name:     "DstFalseOverlayTrue",
				dst:      ptrBool(false),
				overlay:  ptrBool(true),
				expected: ptrBool(true),
			},
			{
				name:     "DstFalseOverlayFalse",
				dst:      ptrBool(false),
				overlay:  ptrBool(false),
				expected: ptrBool(false),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				dst := &Blueprint{
					TerraformComponents: []TerraformComponent{
						{
							Source:  "source1",
							Path:    "module/path1",
							Destroy: tt.dst,
						},
					},
				}

				overlay := &Blueprint{
					TerraformComponents: []TerraformComponent{
						{
							Source:  "source1",
							Path:    "module/path1",
							Destroy: tt.overlay,
						},
					},
				}

				dst.Merge(overlay)

				if len(dst.TerraformComponents) != 1 {
					t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
				}

				component := dst.TerraformComponents[0]
				if tt.expected == nil {
					if component.Destroy != nil {
						t.Errorf("Expected Destroy to be nil, but got %v", component.Destroy)
					}
				} else {
					if component.Destroy == nil {
						t.Errorf("Expected Destroy to be %v, but got nil", *tt.expected)
					} else if *component.Destroy != *tt.expected {
						t.Errorf("Expected Destroy to be %v, but got %v", *tt.expected, *component.Destroy)
					}
				}
			})
		}
	})

	t.Run("ParallelismFieldMerge", func(t *testing.T) {
		tests := []struct {
			name     string
			dst      *int
			overlay  *int
			expected *int
		}{
			{
				name:     "BothNil",
				dst:      nil,
				overlay:  nil,
				expected: nil,
			},
			{
				name:     "DstNilOverlaySet",
				dst:      nil,
				overlay:  ptrInt(5),
				expected: ptrInt(5),
			},
			{
				name:     "DstSetOverlayNil",
				dst:      ptrInt(10),
				overlay:  nil,
				expected: ptrInt(10),
			},
			{
				name:     "BothSet",
				dst:      ptrInt(10),
				overlay:  ptrInt(5),
				expected: ptrInt(5),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				dst := &Blueprint{
					TerraformComponents: []TerraformComponent{
						{
							Source:      "source1",
							Path:        "module/path1",
							Parallelism: tt.dst,
						},
					},
				}

				overlay := &Blueprint{
					TerraformComponents: []TerraformComponent{
						{
							Source:      "source1",
							Path:        "module/path1",
							Parallelism: tt.overlay,
						},
					},
				}

				dst.Merge(overlay)

				if len(dst.TerraformComponents) != 1 {
					t.Fatalf("Expected 1 TerraformComponent, but got %d", len(dst.TerraformComponents))
				}

				component := dst.TerraformComponents[0]
				if tt.expected == nil {
					if component.Parallelism != nil {
						t.Errorf("Expected Parallelism to be nil, but got %v", component.Parallelism)
					}
				} else {
					if component.Parallelism == nil {
						t.Errorf("Expected Parallelism to be %v, but got nil", *tt.expected)
					} else if *component.Parallelism != *tt.expected {
						t.Errorf("Expected Parallelism to be %v, but got %v", *tt.expected, *component.Parallelism)
					}
				}
			})
		}
	})

	t.Run("OverlayComponentWithDifferentSource", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "mod", Source: "A"}},
		}
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "mod", Source: "B"}},
		}
		base.Merge(overlay)
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Source != "B" {
			t.Errorf("expected overlay component with Source B, got %v", base.TerraformComponents)
		}
	})

	t.Run("OverlayComponentWithNewPath", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "mod1", Source: "A"}},
		}
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{{Path: "mod2", Source: "B"}},
		}
		base.Merge(overlay)
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Path != "mod2" {
			t.Errorf("expected overlay component with Path mod2, got %v", base.TerraformComponents)
		}
	})

	t.Run("OverlaySourceWithNewName", func(t *testing.T) {
		base := &Blueprint{
			Sources: []Source{{Name: "A", Url: "urlA"}},
		}
		overlay := &Blueprint{
			Sources: []Source{{Name: "B", Url: "urlB"}},
		}
		base.Merge(overlay)
		foundB := false
		for _, s := range base.Sources {
			if s.Name == "B" && s.Url == "urlB" {
				foundB = true
			}
		}
		if !foundB {
			t.Errorf("expected overlay source with Name B, got %v", base.Sources)
		}
	})

	t.Run("OverlayRepositoryRefFirstNonEmptyField", func(t *testing.T) {
		cases := []struct {
			name  string
			ref   Reference
			check func(t *testing.T, ref Reference)
		}{
			{
				name: "Commit",
				ref:  Reference{Commit: "abc123", Name: "v1", SemVer: "1.0.0", Tag: "v1.0.0", Branch: "develop"},
				check: func(t *testing.T, ref Reference) {
					if ref.Commit != "abc123" {
						t.Errorf("Commit not set")
					}
					if ref.Name != "" {
						t.Errorf("Name should be empty")
					}
					if ref.SemVer != "" {
						t.Errorf("SemVer should be empty")
					}
					if ref.Tag != "" {
						t.Errorf("Tag should be empty")
					}
					if ref.Branch != "main" {
						t.Errorf("Branch should remain 'main'")
					}
				},
			},
			{
				name: "Name",
				ref:  Reference{Name: "v1", SemVer: "1.0.0", Tag: "v1.0.0", Branch: "develop"},
				check: func(t *testing.T, ref Reference) {
					if ref.Commit != "" {
						t.Errorf("Commit should be empty")
					}
					if ref.Name != "v1" {
						t.Errorf("Name not set")
					}
					if ref.SemVer != "" {
						t.Errorf("SemVer should be empty")
					}
					if ref.Tag != "" {
						t.Errorf("Tag should be empty")
					}
					if ref.Branch != "main" {
						t.Errorf("Branch should remain 'main'")
					}
				},
			},
			{
				name: "SemVer",
				ref:  Reference{SemVer: "1.0.0", Tag: "v1.0.0", Branch: "develop"},
				check: func(t *testing.T, ref Reference) {
					if ref.Commit != "" {
						t.Errorf("Commit should be empty")
					}
					if ref.Name != "" {
						t.Errorf("Name should be empty")
					}
					if ref.SemVer != "1.0.0" {
						t.Errorf("SemVer not set")
					}
					if ref.Tag != "" {
						t.Errorf("Tag should be empty")
					}
					if ref.Branch != "main" {
						t.Errorf("Branch should remain 'main'")
					}
				},
			},
			{
				name: "Tag",
				ref:  Reference{Tag: "v1.0.0", Branch: "develop"},
				check: func(t *testing.T, ref Reference) {
					if ref.Commit != "" {
						t.Errorf("Commit should be empty")
					}
					if ref.Name != "" {
						t.Errorf("Name should be empty")
					}
					if ref.SemVer != "" {
						t.Errorf("SemVer should be empty")
					}
					if ref.Tag != "v1.0.0" {
						t.Errorf("Tag not set")
					}
					if ref.Branch != "main" {
						t.Errorf("Branch should remain 'main'")
					}
				},
			},
			{
				name: "Branch",
				ref:  Reference{Branch: "develop"},
				check: func(t *testing.T, ref Reference) {
					if ref.Commit != "" {
						t.Errorf("Commit should be empty")
					}
					if ref.Name != "" {
						t.Errorf("Name should be empty")
					}
					if ref.SemVer != "" {
						t.Errorf("SemVer should be empty")
					}
					if ref.Tag != "" {
						t.Errorf("Tag should be empty")
					}
					if ref.Branch != "develop" {
						t.Errorf("Branch not set")
					}
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				base := &Blueprint{
					Repository: Repository{
						Ref: Reference{Branch: "main"},
					},
				}
				overlay := &Blueprint{
					Repository: Repository{Ref: tc.ref},
				}
				base.Merge(overlay)
				tc.check(t, base.Repository.Ref)
			})
		}
	})

	t.Run("OverlayWithRepositoryRefFields", func(t *testing.T) {
		base := &Blueprint{
			Repository: Repository{
				Ref: Reference{
					Branch: "main",
				},
			},
		}
		overlay := &Blueprint{
			Repository: Repository{
				Ref: Reference{
					Name:   "v1",
					SemVer: "1.0.0",
					Tag:    "v1.0.0",
					Branch: "develop",
				},
			},
		}
		base.Merge(overlay)
		if base.Repository.Ref.Commit != "" {
			t.Errorf("Expected Repository.Ref.Commit to be '', but got '%s'", base.Repository.Ref.Commit)
		}
		if base.Repository.Ref.Name != "v1" {
			t.Errorf("Expected Repository.Ref.Name to be 'v1', but got '%s'", base.Repository.Ref.Name)
		}
		if base.Repository.Ref.SemVer != "" {
			t.Errorf("Expected Repository.Ref.SemVer to be '', but got '%s'", base.Repository.Ref.SemVer)
		}
		if base.Repository.Ref.Tag != "" {
			t.Errorf("Expected Repository.Ref.Tag to be '', but got '%s'", base.Repository.Ref.Tag)
		}
		if base.Repository.Ref.Branch != "main" {
			t.Errorf("Expected Repository.Ref.Branch to remain 'main', but got '%s'", base.Repository.Ref.Branch)
		}
	})

	t.Run("OverlayWithEmptyKustomizations", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{{Name: "A"}},
		}
		overlay := &Blueprint{
			Kustomizations: []Kustomization{{Name: "B"}},
		}
		base.Merge(overlay)
		if len(base.Kustomizations) != 1 || base.Kustomizations[0].Name != "B" {
			t.Errorf("expected overlay kustomization with Name B, got %v", base.Kustomizations)
		}
	})

	t.Run("OverlayWithEmptyFields", func(t *testing.T) {
		base := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
			},
			Repository: Repository{
				Url: "http://example.com/repo1",
				Ref: Reference{
					Branch: "main",
				},
			},
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "http://example.com/source1",
					PathPrefix: "prefix1",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key1": "value1"},
					FullPath: "original/full/path",
					Destroy:  ptrBool(true),
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					PostBuild: &PostBuild{
						Substitute: map[string]string{"key1": "value1"},
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
						},
					},
				},
			},
		}
		overlay := &Blueprint{}
		base.Merge(overlay)
		if base.Kind != "Blueprint" {
			t.Errorf("Expected Kind to be 'Blueprint', but got '%s'", base.Kind)
		}
		if base.ApiVersion != "v1alpha1" {
			t.Errorf("Expected ApiVersion to be 'v1alpha1', but got '%s'", base.ApiVersion)
		}
		if base.Metadata.Name != "original" {
			t.Errorf("Expected Metadata.Name to be 'original', but got '%s'", base.Metadata.Name)
		}
		if base.Repository.Url != "http://example.com/repo1" {
			t.Errorf("Expected Repository.Url to be 'http://example.com/repo1', but got '%s'", base.Repository.Url)
		}
		if base.Repository.Ref.Branch != "main" {
			t.Errorf("Expected Repository.Ref.Branch to be 'main', but got '%s'", base.Repository.Ref.Branch)
		}
		if len(base.Sources) != 1 || base.Sources[0].Name != "source1" {
			t.Errorf("Expected Sources to contain 'source1', but got %v", base.Sources)
		}
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Path != "module/path1" {
			t.Errorf("Expected TerraformComponents to contain 'module/path1', but got %v", base.TerraformComponents)
		}
		if len(base.Kustomizations) != 1 || base.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected Kustomizations to contain 'kustomization1', but got %v", base.Kustomizations)
		}
	})

	t.Run("OverlayWithEmptySlices", func(t *testing.T) {
		base := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
			},
			Repository: Repository{
				Url: "http://example.com/repo1",
				Ref: Reference{
					Branch: "main",
				},
			},
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "http://example.com/source1",
					PathPrefix: "prefix1",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key1": "value1"},
					FullPath: "original/full/path",
					Destroy:  ptrBool(true),
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					PostBuild: &PostBuild{
						Substitute: map[string]string{"key1": "value1"},
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
						},
					},
				},
			},
		}
		overlay := &Blueprint{
			Sources:             []Source{},
			TerraformComponents: []TerraformComponent{},
			Kustomizations:      []Kustomization{},
		}
		base.Merge(overlay)
		if len(base.Sources) != 1 || base.Sources[0].Name != "source1" {
			t.Errorf("Expected Sources to contain 'source1', but got %v", base.Sources)
		}
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Path != "module/path1" {
			t.Errorf("Expected TerraformComponents to contain 'module/path1', but got %v", base.TerraformComponents)
		}
		if len(base.Kustomizations) != 1 || base.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected Kustomizations to contain 'kustomization1', but got %v", base.Kustomizations)
		}
	})

	t.Run("OverlayWithRepositoryRefFields", func(t *testing.T) {
		base := &Blueprint{
			Repository: Repository{
				Ref: Reference{
					Branch: "main",
				},
			},
		}
		overlay := &Blueprint{
			Repository: Repository{
				Ref: Reference{
					Commit: "abc123",
					Name:   "v1",
					SemVer: "1.0.0",
					Tag:    "v1.0.0",
					Branch: "develop",
				},
			},
		}
		base.Merge(overlay)
		if base.Repository.Ref.Commit != "abc123" {
			t.Errorf("Expected Repository.Ref.Commit to be 'abc123', but got '%s'", base.Repository.Ref.Commit)
		}
		if base.Repository.Ref.Name != "" {
			t.Errorf("Expected Repository.Ref.Name to be '', but got '%s'", base.Repository.Ref.Name)
		}
		if base.Repository.Ref.SemVer != "" {
			t.Errorf("Expected Repository.Ref.SemVer to be '', but got '%s'", base.Repository.Ref.SemVer)
		}
		if base.Repository.Ref.Tag != "" {
			t.Errorf("Expected Repository.Ref.Tag to be '', but got '%s'", base.Repository.Ref.Tag)
		}
		if base.Repository.Ref.Branch != "main" {
			t.Errorf("Expected Repository.Ref.Branch to remain 'main', but got '%s'", base.Repository.Ref.Branch)
		}
	})

	t.Run("OverlayWithEmptyRepositorySecretName", func(t *testing.T) {
		base := &Blueprint{
			Repository: Repository{
				SecretName: "base-secret",
			},
		}
		overlay := &Blueprint{
			Repository: Repository{
				SecretName: "",
			},
		}
		base.Merge(overlay)
		if base.Repository.SecretName != "base-secret" {
			t.Errorf("Expected Repository.SecretName to be 'base-secret', but got '%s'", base.Repository.SecretName)
		}
	})

	t.Run("OverlayWithEmptySourceName", func(t *testing.T) {
		base := &Blueprint{
			Sources: []Source{
				{
					Name:       "source1",
					Url:        "http://example.com/source1",
					PathPrefix: "prefix1",
					Ref: Reference{
						Branch: "main",
					},
				},
			},
		}
		overlay := &Blueprint{
			Sources: []Source{
				{
					Name:       "",
					Url:        "http://example.com/source2",
					PathPrefix: "prefix2",
					Ref: Reference{
						Branch: "feature",
					},
				},
			},
		}
		base.Merge(overlay)
		if len(base.Sources) != 1 || base.Sources[0].Name != "source1" {
			t.Errorf("Expected Sources to contain 'source1', but got %v", base.Sources)
		}
	})

	t.Run("OverlayWithNoMatchingTerraformComponent", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key1": "value1"},
					FullPath: "original/full/path",
					Destroy:  ptrBool(true),
				},
			},
		}
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source2",
					Path:     "module/path2",
					Values:   map[string]any{"key2": "value2"},
					FullPath: "overlay/full/path",
					Destroy:  ptrBool(false),
				},
			},
		}
		base.Merge(overlay)
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Path != "module/path2" {
			t.Errorf("Expected TerraformComponents to contain 'module/path2', but got %v", base.TerraformComponents)
		}
	})

	t.Run("OverlayWithSamePathDifferentSource", func(t *testing.T) {
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key1": "value1"},
					FullPath: "original/full/path",
					Destroy:  ptrBool(true),
				},
			},
		}
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source:   "source2",
					Path:     "module/path1",
					Values:   map[string]any{"key2": "value2"},
					FullPath: "overlay/full/path",
					Destroy:  ptrBool(false),
				},
			},
		}
		base.Merge(overlay)
		if len(base.TerraformComponents) != 1 || base.TerraformComponents[0].Source != "source2" {
			t.Errorf("Expected TerraformComponents to contain 'source2', but got %v", base.TerraformComponents)
		}
	})

	t.Run("OverlayWithEmptyKustomizations", func(t *testing.T) {
		base := &Blueprint{
			Kustomizations: []Kustomization{{Path: "kustomize"}},
		}
		overlay := &Blueprint{
			Kustomizations: []Kustomization{},
		}
		base.Merge(overlay)
		if len(base.Kustomizations) != 1 || base.Kustomizations[0].Path != "kustomize" {
			t.Errorf("expected base kustomizations to be retained, got %v", base.Kustomizations)
		}
	})

	t.Run("OverlayWithNonEmptyKind", func(t *testing.T) {
		base := &Blueprint{Kind: "base-kind"}
		overlay := &Blueprint{Kind: "new-kind"}
		base.Merge(overlay)
		if base.Kind != "new-kind" {
			t.Errorf("expected Kind to be overwritten to 'new-kind', got %v", base.Kind)
		}
	})

	t.Run("OverlayWithNonEmptyApiVersion", func(t *testing.T) {
		base := &Blueprint{ApiVersion: "v1"}
		overlay := &Blueprint{ApiVersion: "v2"}
		base.Merge(overlay)
		if base.ApiVersion != "v2" {
			t.Errorf("expected ApiVersion to be overwritten to 'v2', got %v", base.ApiVersion)
		}
	})

	t.Run("OverlayWithNonEmptyRepositorySecretName", func(t *testing.T) {
		base := &Blueprint{Repository: Repository{SecretName: "base-secret"}}
		overlay := &Blueprint{Repository: Repository{SecretName: "new-secret"}}
		base.Merge(overlay)
		if base.Repository.SecretName != "new-secret" {
			t.Errorf("expected Repository.SecretName to be overwritten to 'new-secret', got %v", base.Repository.SecretName)
		}
	})
}

func TestBlueprint_DeepCopy(t *testing.T) {
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
					Ref: Reference{
						Branch: "main",
					},
				},
			},
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Values: map[string]any{
						"key1": "value1",
					},
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					PostBuild: &PostBuild{
						Substitute: map[string]string{
							"key1": "value1",
						},
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
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
		if len(copy.TerraformComponents[0].Values) != 1 || copy.TerraformComponents[0].Values["key1"] != "value1" {
			t.Errorf("Expected copy to have terraform component value 'key1' with value 'value1', but got %v", copy.TerraformComponents[0].Values)
		}
		if len(copy.Kustomizations) != 1 || copy.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected copy to have kustomization 'kustomization1', but got %v", copy.Kustomizations)
		}
		if len(copy.Kustomizations[0].Components) != 1 || copy.Kustomizations[0].Components[0] != "component1" {
			t.Errorf("Expected copy to have kustomization component 'component1', but got %v", copy.Kustomizations[0].Components)
		}
		if len(copy.Kustomizations[0].PostBuild.Substitute) != 1 || copy.Kustomizations[0].PostBuild.Substitute["key1"] != "value1" {
			t.Errorf("Expected copy to have Substitute 'key1:value1', but got %v", copy.Kustomizations[0].PostBuild.Substitute)
		}
		if len(copy.Kustomizations[0].PostBuild.SubstituteFrom) != 1 || copy.Kustomizations[0].PostBuild.SubstituteFrom[0].Kind != "ConfigMap" || copy.Kustomizations[0].PostBuild.SubstituteFrom[0].Name != "config1" {
			t.Errorf("Expected copy to have SubstituteFrom 'ConfigMap:config1', but got %v", copy.Kustomizations[0].PostBuild.SubstituteFrom)
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

// TestPostBuildOmitEmpty verifies that empty PostBuild objects are omitted from YAML serialization
func TestPostBuildOmitEmpty(t *testing.T) {
	t.Run("EmptyPostBuildOmitted", func(t *testing.T) {
		kustomization := Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			PostBuild: &PostBuild{
				Substitute:     map[string]string{},
				SubstituteFrom: []SubstituteReference{},
			},
		}

		// Create a copy using DeepCopy which should omit empty PostBuild
		copied := kustomization.DeepCopy()

		// Verify that PostBuild is nil for empty content
		if copied.PostBuild != nil {
			t.Errorf("Expected PostBuild to be nil for empty content, but got %v", copied.PostBuild)
		}
	})

	t.Run("NonEmptyPostBuildPreserved", func(t *testing.T) {
		kustomization := Kustomization{
			Name: "test-kustomization",
			Path: "test/path",
			PostBuild: &PostBuild{
				Substitute: map[string]string{
					"key": "value",
				},
				SubstituteFrom: []SubstituteReference{
					{Kind: "ConfigMap", Name: "test"},
				},
			},
		}

		// Create a copy using DeepCopy which should preserve non-empty PostBuild
		copied := kustomization.DeepCopy()

		// Verify that PostBuild is preserved for non-empty content
		if copied.PostBuild == nil {
			t.Error("Expected PostBuild to be preserved for non-empty content, but got nil")
		}
		if copied.PostBuild.Substitute["key"] != "value" {
			t.Errorf("Expected substitute key to be 'value', but got %s", copied.PostBuild.Substitute["key"])
		}
		if len(copied.PostBuild.SubstituteFrom) != 1 {
			t.Errorf("Expected 1 substitute reference, but got %d", len(copied.PostBuild.SubstituteFrom))
		}
	})
}

func TestBlueprint_StrategicMerge(t *testing.T) {
	t.Run("MergesTerraformComponentsStrategically", func(t *testing.T) {
		// Given a base blueprint with terraform components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc",
					Source:    "core",
					Values:    map[string]any{"cidr": "10.0.0.0/16"},
					DependsOn: []string{"backend"},
				},
			},
		}

		// And an overlay with same component (should merge) and new component (should append)
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "network/vpc", // Same path+source - should merge
					Source:    "core",
					Values:    map[string]any{"enable_dns": true},
					DependsOn: []string{"security"},
				},
				{
					Path:   "cluster/eks", // New component - should append
					Source: "core",
					Values: map[string]any{"version": "1.28"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 components
		if len(base.TerraformComponents) != 2 {
			t.Errorf("Expected 2 terraform components, got %d", len(base.TerraformComponents))
		}

		// And first component should have merged values and dependencies
		vpc := base.TerraformComponents[0]
		if vpc.Path != "network/vpc" {
			t.Errorf("Expected path 'network/vpc', got '%s'", vpc.Path)
		}
		if len(vpc.Values) != 2 {
			t.Errorf("Expected 2 values, got %d", len(vpc.Values))
		}
		if vpc.Values["cidr"] != "10.0.0.0/16" {
			t.Errorf("Expected original cidr value preserved")
		}
		if vpc.Values["enable_dns"] != true {
			t.Errorf("Expected new enable_dns value added")
		}
		if len(vpc.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(vpc.DependsOn))
		}
		if !contains(vpc.DependsOn, "backend") || !contains(vpc.DependsOn, "security") {
			t.Errorf("Expected both backend and security dependencies, got %v", vpc.DependsOn)
		}

		// And second component should be the new one
		eks := base.TerraformComponents[1]
		if eks.Path != "cluster/eks" {
			t.Errorf("Expected path 'cluster/eks', got '%s'", eks.Path)
		}
	})

	t.Run("MergesKustomizationsStrategically", func(t *testing.T) {
		// Given a base blueprint with kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress",
					Components: []string{"nginx"},
					DependsOn:  []string{"pki"},
				},
			},
		}

		// And an overlay with same kustomization (should merge) and new kustomization (should append)
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "ingress", // Same name - should merge
					Components: []string{"nginx/tls"},
					DependsOn:  []string{"cert-manager"},
				},
				{
					Name:       "monitoring", // New kustomization - should append
					Components: []string{"prometheus"},
				},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have 2 kustomizations
		if len(base.Kustomizations) != 2 {
			t.Errorf("Expected 2 kustomizations, got %d", len(base.Kustomizations))
		}

		// Components should be ordered by their original order since both have unresolved dependencies
		ingress := base.Kustomizations[0]
		if ingress.Name != "ingress" {
			t.Errorf("Expected name 'ingress' at index 0, got '%s'", ingress.Name)
		}

		// And second kustomization should be monitoring
		monitoring := base.Kustomizations[1]
		if monitoring.Name != "monitoring" {
			t.Errorf("Expected name 'monitoring' at index 1, got '%s'", monitoring.Name)
		}
		if len(ingress.Components) != 2 {
			t.Errorf("Expected 2 components, got %d", len(ingress.Components))
		}
		if !contains(ingress.Components, "nginx") || !contains(ingress.Components, "nginx/tls") {
			t.Errorf("Expected both nginx and nginx/tls components, got %v", ingress.Components)
		}
		if len(ingress.DependsOn) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(ingress.DependsOn))
		}
		if !contains(ingress.DependsOn, "pki") || !contains(ingress.DependsOn, "cert-manager") {
			t.Errorf("Expected both pki and cert-manager dependencies, got %v", ingress.DependsOn)
		}

		// Check monitoring component (should have no dependencies)
		if len(monitoring.Components) != 1 {
			t.Errorf("Expected 1 component, got %d", len(monitoring.Components))
		}
		if !contains(monitoring.Components, "prometheus") {
			t.Errorf("Expected prometheus component, got %v", monitoring.Components)
		}
		if len(monitoring.DependsOn) != 0 {
			t.Errorf("Expected no dependencies for monitoring, got %v", monitoring.DependsOn)
		}
	})

	t.Run("HandlesDependencyAwareInsertion", func(t *testing.T) {
		// Given a base blueprint with ordered components
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "backend", Source: "core"},
				{Path: "network", Source: "core"},
			},
		}

		// When adding a component that depends on existing component
		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Path:      "cluster",
					Source:    "core",
					DependsOn: []string{"network"}, // Should be inserted after network
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then component should be inserted in correct order
		if len(base.TerraformComponents) != 3 {
			t.Errorf("Expected 3 components, got %d", len(base.TerraformComponents))
		}

		// Should be: backend, network, cluster (cluster after its dependency)
		if base.TerraformComponents[2].Path != "cluster" {
			t.Errorf("Expected cluster component at index 2, got '%s'", base.TerraformComponents[2].Path)
		}
	})

	t.Run("HandlesNilOverlay", func(t *testing.T) {
		// Given a base blueprint
		base := &Blueprint{
			Metadata: Metadata{Name: "test"},
		}

		// When strategic merging with nil overlay
		base.StrategicMerge(nil)

		// Then base should be unchanged
		if base.Metadata.Name != "test" {
			t.Errorf("Expected metadata name preserved")
		}
	})

	t.Run("MergesMetadataAndRepository", func(t *testing.T) {
		// Given a base blueprint
		base := &Blueprint{
			Metadata: Metadata{
				Name:        "base",
				Description: "base description",
			},
			Repository: Repository{
				Url: "base-url",
				Ref: Reference{Branch: "main"},
			},
		}

		// And an overlay with updated metadata
		overlay := &Blueprint{
			Metadata: Metadata{
				Name:        "updated",
				Description: "updated description",
			},
			Repository: Repository{
				Url: "updated-url",
				Ref: Reference{Tag: "v1.0.0"},
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then metadata should be updated
		if base.Metadata.Name != "updated" {
			t.Errorf("Expected name 'updated', got '%s'", base.Metadata.Name)
		}
		if base.Metadata.Description != "updated description" {
			t.Errorf("Expected description 'updated description', got '%s'", base.Metadata.Description)
		}

		// And repository should be updated
		if base.Repository.Url != "updated-url" {
			t.Errorf("Expected url 'updated-url', got '%s'", base.Repository.Url)
		}
		if base.Repository.Ref.Tag != "v1.0.0" {
			t.Errorf("Expected tag 'v1.0.0', got '%s'", base.Repository.Ref.Tag)
		}
	})

	t.Run("MergesSourcesUniquely", func(t *testing.T) {
		// Given a base blueprint with sources
		base := &Blueprint{
			Sources: []Source{
				{Name: "source1", Url: "url1"},
			},
		}

		// And an overlay with overlapping and new sources
		overlay := &Blueprint{
			Sources: []Source{
				{Name: "source1", Url: "updated-url1"}, // Should update
				{Name: "source2", Url: "url2"},         // Should add
			},
		}

		// When strategic merging
		base.StrategicMerge(overlay)

		// Then should have both sources with updated values
		if len(base.Sources) != 2 {
			t.Errorf("Expected 2 sources, got %d", len(base.Sources))
		}

		// Check that source1 was updated and source2 was added
		sourceMap := make(map[string]string)
		for _, source := range base.Sources {
			sourceMap[source.Name] = source.Url
		}

		if sourceMap["source1"] != "updated-url1" {
			t.Errorf("Expected source1 url to be updated")
		}
		if sourceMap["source2"] != "url2" {
			t.Errorf("Expected source2 to be added")
		}
	})

	t.Run("EmptyOverlayDoesNothing", func(t *testing.T) {
		// Given a base blueprint with content
		base := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{Path: "test", Source: "core"},
			},
			Kustomizations: []Kustomization{
				{Name: "test"},
			},
		}

		// When strategic merging with empty overlay
		overlay := &Blueprint{}
		base.StrategicMerge(overlay)

		// Then base should be unchanged
		if len(base.TerraformComponents) != 1 {
			t.Errorf("Expected terraform components unchanged")
		}
		if len(base.Kustomizations) != 1 {
			t.Errorf("Expected kustomizations unchanged")
		}
	})

	t.Run("KustomizationDependencyAwareInsertion", func(t *testing.T) {
		// Given a base blueprint with ordered kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "policy", Path: "policy"},
				{Name: "pki", Path: "pki"},
			},
		}

		// When adding a kustomization that depends on existing one
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:      "ingress",
					Path:      "ingress",
					DependsOn: []string{"pki"}, // Should be inserted after pki
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then kustomization should be inserted in correct order
		if len(base.Kustomizations) != 3 {
			t.Errorf("Expected 3 kustomizations, got %d", len(base.Kustomizations))
		}

		// Should have ingress after pki (its dependency)
		pkiIndex := -1
		ingressIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "pki" {
				pkiIndex = i
			}
			if k.Name == "ingress" {
				ingressIndex = i
			}
		}

		if pkiIndex == -1 {
			t.Errorf("Expected pki kustomization to be present")
		}
		if ingressIndex == -1 {
			t.Errorf("Expected ingress kustomization to be present")
		}
		if pkiIndex >= ingressIndex {
			t.Errorf("Expected ingress (index %d) to come after pki (index %d)", ingressIndex, pkiIndex)
		}
	})

	t.Run("KustomizationUpdatesFieldsSelectively", func(t *testing.T) {
		// Given a base blueprint with a kustomization
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test",
					Path:    "original-path",
					Source:  "original-source",
					Destroy: ptrBool(false),
				},
			},
		}

		// When merging with partial updates
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:    "test", // Same name - should merge
					Path:    "updated-path",
					Source:  "updated-source",
					Destroy: ptrBool(true),
					// Note: not setting Components or DependsOn - should preserve existing
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should have updated fields
		kustomization := base.Kustomizations[0]
		if kustomization.Path != "updated-path" {
			t.Errorf("Expected path to be updated to 'updated-path', got '%s'", kustomization.Path)
		}
		if kustomization.Source != "updated-source" {
			t.Errorf("Expected source to be updated to 'updated-source', got '%s'", kustomization.Source)
		}
		if kustomization.Destroy == nil || *kustomization.Destroy != true {
			t.Errorf("Expected destroy to be updated to true, got %v", kustomization.Destroy)
		}
	})

	t.Run("KustomizationPreservesExistingComponents", func(t *testing.T) {
		// Given a base blueprint with kustomization that has components
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "test",
					Components: []string{"existing1", "existing2"},
					DependsOn:  []string{"dep1"},
				},
			},
		}

		// When merging with additional components and dependencies
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "test",
					Components: []string{"existing2", "new1"}, // existing2 is duplicate, new1 is new
					DependsOn:  []string{"dep1", "dep2"},      // dep1 is duplicate, dep2 is new
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should have all unique components and dependencies
		kustomization := base.Kustomizations[0]
		if len(kustomization.Components) != 3 {
			t.Errorf("Expected 3 unique components, got %d: %v", len(kustomization.Components), kustomization.Components)
		}

		expectedComponents := []string{"existing1", "existing2", "new1"}
		for _, expected := range expectedComponents {
			if !contains(kustomization.Components, expected) {
				t.Errorf("Expected component '%s' to be present, got %v", expected, kustomization.Components)
			}
		}

		if len(kustomization.DependsOn) != 2 {
			t.Errorf("Expected 2 unique dependencies, got %d: %v", len(kustomization.DependsOn), kustomization.DependsOn)
		}

		expectedDeps := []string{"dep1", "dep2"}
		for _, expected := range expectedDeps {
			if !contains(kustomization.DependsOn, expected) {
				t.Errorf("Expected dependency '%s' to be present, got %v", expected, kustomization.DependsOn)
			}
		}
	})

	t.Run("KustomizationMultipleDependencyInsertion", func(t *testing.T) {
		// Given a base blueprint with multiple kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "base", Path: "base"},
				{Name: "pki", Path: "pki"},
				{Name: "storage", Path: "storage"},
			},
		}

		// When adding a kustomization that depends on multiple existing ones
		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:      "app",
					Path:      "app",
					DependsOn: []string{"pki", "storage"}, // Depends on multiple
				},
			},
		}

		base.StrategicMerge(overlay)

		// Then should be inserted after the latest dependency
		if len(base.Kustomizations) != 4 {
			t.Errorf("Expected 4 kustomizations, got %d", len(base.Kustomizations))
		}

		// App should come after its dependencies (pki and storage)
		appIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "app" {
				appIndex = i
				break
			}
		}
		if appIndex == -1 {
			t.Errorf("Expected app kustomization to be present")
		}

		// Find indices of dependencies
		pkiIndex := -1
		storageIndex := -1
		for i, k := range base.Kustomizations {
			if k.Name == "pki" {
				pkiIndex = i
			}
			if k.Name == "storage" {
				storageIndex = i
			}
		}

		// App should come after both dependencies
		if appIndex <= pkiIndex || appIndex <= storageIndex {
			t.Errorf("Expected app (index %d) to come after pki (index %d) and storage (index %d)", appIndex, pkiIndex, storageIndex)
		}
	})

	t.Run("ComplexDependencyOrdering", func(t *testing.T) {
		// Test the complex dependency scenario described by the user
		// where pki-* components are separated by dns, but dns depends on both pki-base and ingress

		// Start with a base blueprint that has some kustomizations
		base := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "policy-base", Path: "policy/base"},
				{Name: "policy-resources", Path: "policy/resources", DependsOn: []string{"policy-base"}},
			},
		}

		// Add kustomizations one by one to trigger strategic merge and sorting
		overlay1 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "pki-base", Path: "pki/base", DependsOn: []string{"policy-resources"}},
			},
		}
		base.StrategicMerge(overlay1)

		overlay2 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "pki-resources", Path: "pki/resources", DependsOn: []string{"pki-base"}},
			},
		}
		base.StrategicMerge(overlay2)

		overlay3 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "ingress", Path: "ingress", DependsOn: []string{"pki-resources"}},
			},
		}
		base.StrategicMerge(overlay3)

		overlay4 := &Blueprint{
			Kustomizations: []Kustomization{
				{Name: "dns", Path: "dns", DependsOn: []string{"pki-base", "ingress"}},
			},
		}
		base.StrategicMerge(overlay4)

		// Expected order: policy-base, policy-resources, pki-base, pki-resources, ingress, dns
		expectedOrder := []string{"policy-base", "policy-resources", "pki-base", "pki-resources", "ingress", "dns"}

		if len(base.Kustomizations) != len(expectedOrder) {
			t.Errorf("Expected %d kustomizations, got %d", len(expectedOrder), len(base.Kustomizations))
		}

		for i, expected := range expectedOrder {
			if i >= len(base.Kustomizations) || base.Kustomizations[i].Name != expected {
				actual := "none"
				if i < len(base.Kustomizations) {
					actual = base.Kustomizations[i].Name
				}
				t.Errorf("Expected '%s' at position %d, got '%s'", expected, i, actual)
			}
		}

		// Verify that dependencies are satisfied
		nameToIndex := make(map[string]int)
		for i, k := range base.Kustomizations {
			nameToIndex[k.Name] = i
		}

		for _, k := range base.Kustomizations {
			for _, dep := range k.DependsOn {
				if depIndex, exists := nameToIndex[dep]; exists {
					if depIndex >= nameToIndex[k.Name] {
						t.Errorf("Dependency violation: '%s' (index %d) depends on '%s' (index %d), but dependency should come first",
							k.Name, nameToIndex[k.Name], dep, depIndex)
					}
				}
			}
		}
	})
}

// Helper function to check if slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}
