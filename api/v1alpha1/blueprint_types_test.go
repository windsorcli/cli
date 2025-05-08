package v1alpha1

import (
	"reflect"
	"sort"
	"testing"

	"github.com/fluxcd/pkg/apis/kustomize"
)

func TestBlueprint_Merge(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: Metadata{
				Name:        "original",
				Description: "original description",
				Authors:     []string{"author1"},
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
				Authors:     []string{"author2"},
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
					Source:   "source1",
					Path:     "module/path1",
					Values:   map[string]any{"key2": "value2"},
					FullPath: "updated/full/path",
				},
				{
					Source:   "source2",
					Path:     "module/path2",
					Values:   map[string]any{"key3": "value3"},
					FullPath: "new/full/path",
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
		if !reflect.DeepEqual(dst.Metadata.Authors, []string{"author2"}) {
			t.Errorf("Expected Metadata.Authors to be ['author2'], but got %v", dst.Metadata.Authors)
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
		component2 := dst.TerraformComponents[1]
		if component2.Values == nil || len(component2.Values) != 1 || component2.Values["key3"] != "value3" {
			t.Errorf("Expected Values to contain 'key3', but got %v", component2.Values)
		}
		if component2.FullPath != "new/full/path" {
			t.Errorf("Expected FullPath to be 'new/full/path', but got '%s'", component2.FullPath)
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
					Source:   "source1",
					Path:     "module/path1",
					Values:   nil, // Initialize with nil
					FullPath: "original/full/path",
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
					FullPath: "overlay/full/path",
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
					Path:     "path1",
					Values:   nil,
					FullPath: "original/full/path",
				},
			},
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					PostBuild: &PostBuild{
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
						},
					},
				},
			},
		}

		dst.Merge(nil)

		if dst.Metadata.Name != "original" {
			t.Errorf("Expected Metadata.Name to remain 'original', but got '%s'", dst.Metadata.Name)
		}
		if dst.Metadata.Description != "original description" {
			t.Errorf("Expected Metadata.Description to remain 'original description', but got '%s'", dst.Metadata.Description)
		}
		if len(dst.Sources) != 1 || dst.Sources[0].Name != "source1" {
			t.Errorf("Expected Sources to remain unchanged, but got %v", dst.Sources)
		}
		if len(dst.TerraformComponents) != 1 || dst.TerraformComponents[0].Source != "source1" {
			t.Errorf("Expected TerraformComponents to remain unchanged, but got %v", dst.TerraformComponents)
		}
		if len(dst.Kustomizations) != 1 || dst.Kustomizations[0].Name != "kustomization1" {
			t.Errorf("Expected Kustomizations to remain unchanged, but got %v", dst.Kustomizations)
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
					Values: map[string]any{
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
					Values: map[string]any{
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
					Values: map[string]any{
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
					Values: map[string]any{
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
		if component.Values == nil || len(component.Values) != 1 || component.Values["key1"] != "value1" {
			t.Errorf("Expected Values to contain 'key1', but got %v", component.Values)
		}
		if component.FullPath != "overlay/full/path" {
			t.Errorf("Expected FullPath to be 'overlay/full/path', but got '%s'", component.FullPath)
		}
	})

	t.Run("MergeUniqueKustomizePatches", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component1"},
					Patches: []kustomize.Patch{
						{Patch: "patch1", Target: &kustomize.Selector{Group: "group1", Version: "v1", Kind: "Kind1", Namespace: "namespace1", Name: "name1"}},
					},
					PostBuild: &PostBuild{
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config1"},
						},
					},
				},
			},
		}

		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name:       "kustomization1",
					Path:       "kustomize/path1",
					Components: []string{"component2"}, // New component
					Patches: []kustomize.Patch{
						{Patch: "patch2", Target: &kustomize.Selector{Group: "group2", Version: "v2", Kind: "Kind2", Namespace: "namespace2", Name: "name2"}},
					},
					PostBuild: &PostBuild{
						SubstituteFrom: []SubstituteReference{
							{Kind: "Secret", Name: "secret1"},
						},
					},
				},
				{
					Name:       "kustomization2",
					Path:       "kustomize/path2",
					Components: []string{"component3"},
					Patches: []kustomize.Patch{
						{Patch: "patch3", Target: &kustomize.Selector{Group: "group3", Version: "v3", Kind: "Kind3", Namespace: "namespace3", Name: "name3"}},
					},
					PostBuild: &PostBuild{
						SubstituteFrom: []SubstituteReference{
							{Kind: "ConfigMap", Name: "config2"},
						},
					},
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.Kustomizations) != 2 {
			t.Fatalf("Expected 2 Kustomizations, but got %d", len(dst.Kustomizations))
		}

		kustomization1 := dst.Kustomizations[0]
		if len(kustomization1.Components) != 1 || kustomization1.Components[0] != "component2" {
			t.Errorf("Expected Kustomization1 Components to be ['component2'], but got %v", kustomization1.Components)
		}
		if len(kustomization1.Patches) != 1 || kustomization1.Patches[0].Patch != "patch2" {
			t.Errorf("Expected Kustomization1 Patches to be ['patch2'], but got %v", kustomization1.Patches)
		}
		if len(kustomization1.PostBuild.SubstituteFrom) != 1 || kustomization1.PostBuild.SubstituteFrom[0].Kind != "Secret" || kustomization1.PostBuild.SubstituteFrom[0].Name != "secret1" {
			t.Errorf("Expected Kustomization1 SubstituteFrom to be ['Secret:secret1'], but got %v", kustomization1.PostBuild.SubstituteFrom)
		}

		kustomization2 := dst.Kustomizations[1]
		if len(kustomization2.Components) != 1 || kustomization2.Components[0] != "component3" {
			t.Errorf("Expected Kustomization2 Components to be ['component3'], but got %v", kustomization2.Components)
		}
		if len(kustomization2.Patches) != 1 || kustomization2.Patches[0].Patch != "patch3" {
			t.Errorf("Expected Kustomization2 Patches to be ['patch3'], but got %v", kustomization2.Patches)
		}
		if len(kustomization2.PostBuild.SubstituteFrom) != 1 || kustomization2.PostBuild.SubstituteFrom[0].Kind != "ConfigMap" || kustomization2.PostBuild.SubstituteFrom[0].Name != "config2" {
			t.Errorf("Expected Kustomization2 SubstituteFrom to be ['ConfigMap:config2'], but got %v", kustomization2.PostBuild.SubstituteFrom)
		}
	})

	t.Run("MergeUniqueComponents", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Values: map[string]any{
						"key1": "value1",
					},
					FullPath: "original/full/path",
				},
			},
		}

		overlay := &Blueprint{
			TerraformComponents: []TerraformComponent{
				{
					Source: "source1",
					Path:   "module/path1",
					Values: map[string]any{
						"key2": "value2",
					},
					FullPath: "updated/full/path",
				},
				{
					Source: "source2",
					Path:   "module/path2",
					Values: map[string]any{
						"key3": "value3",
					},
					FullPath: "new/full/path",
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.TerraformComponents) != 2 {
			t.Fatalf("Expected 2 TerraformComponents, but got %d", len(dst.TerraformComponents))
		}

		component1 := dst.TerraformComponents[0]
		if component1.Values == nil || len(component1.Values) != 2 || component1.Values["key1"] != "value1" || component1.Values["key2"] != "value2" {
			t.Errorf("Expected Values to contain both 'key1' and 'key2', but got %v", component1.Values)
		}
		if component1.FullPath != "updated/full/path" {
			t.Errorf("Expected FullPath to be 'updated/full/path', but got '%s'", component1.FullPath)
		}

		component2 := dst.TerraformComponents[1]
		if component2.Values == nil || len(component2.Values) != 1 || component2.Values["key3"] != "value3" {
			t.Errorf("Expected Values to contain 'key3', but got %v", component2.Values)
		}
		if component2.FullPath != "new/full/path" {
			t.Errorf("Expected FullPath to be 'new/full/path', but got '%s'", component2.FullPath)
		}
	})

	t.Run("RepositoryMerge", func(t *testing.T) {
		tests := []struct {
			name           string
			dst            *Blueprint
			overlay        *Blueprint
			expectedCommit string
			expectedName   string
			expectedSemVer string
			expectedTag    string
			expectedBranch string
			expectedSecret string
		}{
			{
				name: "OverlayWithCommit",
				dst: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Commit: "originalCommit",
							Name:   "originalName",
							SemVer: "originalSemVer",
							Tag:    "originalTag",
							Branch: "originalBranch",
						},
						SecretName: "originalSecret",
					},
				},
				overlay: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Commit: "newCommit",
						},
						SecretName: "newSecret",
					},
				},
				expectedCommit: "newCommit",
				expectedName:   "originalName",
				expectedSemVer: "originalSemVer",
				expectedTag:    "originalTag",
				expectedBranch: "originalBranch",
				expectedSecret: "newSecret",
			},
			{
				name: "OverlayWithName",
				dst: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Name:   "originalName",
							SemVer: "originalSemVer",
							Tag:    "originalTag",
							Branch: "originalBranch",
						},
						SecretName: "originalSecret",
					},
				},
				overlay: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Name: "newName",
						},
						SecretName: "newSecret",
					},
				},
				expectedCommit: "",
				expectedName:   "newName",
				expectedSemVer: "originalSemVer",
				expectedTag:    "originalTag",
				expectedBranch: "originalBranch",
				expectedSecret: "newSecret",
			},
			{
				name: "OverlayWithSemVer",
				dst: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							SemVer: "originalSemVer",
							Tag:    "originalTag",
							Branch: "originalBranch",
						},
						SecretName: "originalSecret",
					},
				},
				overlay: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							SemVer: "newSemVer",
						},
						SecretName: "newSecret",
					},
				},
				expectedCommit: "",
				expectedName:   "",
				expectedSemVer: "newSemVer",
				expectedTag:    "originalTag",
				expectedBranch: "originalBranch",
				expectedSecret: "newSecret",
			},
			{
				name: "OverlayWithTag",
				dst: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Tag:    "originalTag",
							Branch: "originalBranch",
						},
						SecretName: "originalSecret",
					},
				},
				overlay: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Tag: "newTag",
						},
						SecretName: "newSecret",
					},
				},
				expectedCommit: "",
				expectedName:   "",
				expectedSemVer: "",
				expectedTag:    "newTag",
				expectedBranch: "originalBranch",
				expectedSecret: "newSecret",
			},
			{
				name: "OverlayWithBranch",
				dst: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Branch: "originalBranch",
						},
						SecretName: "originalSecret",
					},
				},
				overlay: &Blueprint{
					Repository: Repository{
						Ref: Reference{
							Branch: "newBranch",
						},
						SecretName: "newSecret",
					},
				},
				expectedCommit: "",
				expectedName:   "",
				expectedSemVer: "",
				expectedTag:    "",
				expectedBranch: "newBranch",
				expectedSecret: "newSecret",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tt.dst.Merge(tt.overlay)

				if tt.dst.Repository.Ref.Commit != tt.expectedCommit {
					t.Errorf("Expected Commit to be '%s', but got '%s'", tt.expectedCommit, tt.dst.Repository.Ref.Commit)
				}
				if tt.dst.Repository.Ref.Name != tt.expectedName {
					t.Errorf("Expected Name to be '%s', but got '%s'", tt.expectedName, tt.dst.Repository.Ref.Name)
				}
				if tt.dst.Repository.Ref.SemVer != tt.expectedSemVer {
					t.Errorf("Expected SemVer to be '%s', but got '%s'", tt.expectedSemVer, tt.dst.Repository.Ref.SemVer)
				}
				if tt.dst.Repository.Ref.Tag != tt.expectedTag {
					t.Errorf("Expected Tag to be '%s', but got '%s'", tt.expectedTag, tt.dst.Repository.Ref.Tag)
				}
				if tt.dst.Repository.Ref.Branch != tt.expectedBranch {
					t.Errorf("Expected Branch to be '%s', but got '%s'", tt.expectedBranch, tt.dst.Repository.Ref.Branch)
				}
				if tt.dst.Repository.SecretName != tt.expectedSecret {
					t.Errorf("Expected SecretName to be '%s', but got '%s'", tt.expectedSecret, tt.dst.Repository.SecretName)
				}
			})
		}
	})

	t.Run("PostBuildMerge", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Kustomizations: []Kustomization{
				{
					Name: "kustomization1",
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

		overlay := &Blueprint{
			Kustomizations: []Kustomization{
				{
					Name: "kustomization1",
					PostBuild: &PostBuild{
						Substitute: map[string]string{
							"key2": "value2",
						},
						SubstituteFrom: []SubstituteReference{
							{Kind: "Secret", Name: "secret1"},
						},
					},
				},
			},
		}

		dst.Merge(overlay)

		if len(dst.Kustomizations) != 1 {
			t.Fatalf("Expected 1 Kustomization, but got %d", len(dst.Kustomizations))
		}

		postBuild := dst.Kustomizations[0].PostBuild
		if postBuild == nil {
			t.Fatalf("Expected PostBuild to be non-nil")
		}

		if len(postBuild.Substitute) != 1 || postBuild.Substitute["key2"] != "value2" {
			t.Errorf("Expected Substitute to contain ['key2'], but got %v", postBuild.Substitute)
		}

		if len(postBuild.SubstituteFrom) != 1 {
			t.Errorf("Expected SubstituteFrom to contain 1 item, but got %d", len(postBuild.SubstituteFrom))
		}
	})

	t.Run("KindAndApiVersionMerge", func(t *testing.T) {
		dst := &Blueprint{
			Kind:       "OldKind",
			ApiVersion: "old/v1",
		}

		overlay := &Blueprint{
			Kind:       "NewKind",
			ApiVersion: "new/v2",
		}

		dst.Merge(overlay)

		if dst.Kind != "NewKind" {
			t.Errorf("Expected Kind to be 'NewKind', but got '%s'", dst.Kind)
		}

		if dst.ApiVersion != "new/v2" {
			t.Errorf("Expected ApiVersion to be 'new/v2', but got '%s'", dst.ApiVersion)
		}

		// Test with empty values which shouldn't override
		emptyOverlay := &Blueprint{
			Kind:       "",
			ApiVersion: "",
		}

		dst.Merge(emptyOverlay)

		if dst.Kind != "NewKind" {
			t.Errorf("Expected Kind to remain 'NewKind', but got '%s'", dst.Kind)
		}

		if dst.ApiVersion != "new/v2" {
			t.Errorf("Expected ApiVersion to remain 'new/v2', but got '%s'", dst.ApiVersion)
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
