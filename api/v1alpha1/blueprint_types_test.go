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
