package artifact

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArtifactManifest_YAMLRoundTrip(t *testing.T) {
	t.Run("FullManifest_PreservesAllFields", func(t *testing.T) {
		// Given a manifest populated in every field, including optional ones
		original := ArtifactManifest{
			Version: ManifestVersion,
			Blueprint: Provenance{
				Name:    "core",
				Version: "v1.2.0",
				Commit:  "abc123",
			},
			Artifacts: []ManifestEntry{
				{
					Type:       ArtifactTypeDocker,
					Reference:  "ghcr.io/fluxcd/kustomize-controller:v1.8.3",
					Version:    "v1.8.3",
					Digest:     "sha256:c59e81059330a55203bf60806229a052617134d8b557c1bd83cdc69a8ece7ea2",
					Repository: "ghcr.io/fluxcd/kustomize-controller",
					Source: ArtifactSource{
						File:       "terraform/gitops/flux/main.tf",
						Line:       72,
						Datasource: "docker",
						DepName:    "ghcr.io/fluxcd/kustomize-controller",
						Package:    "ghcr.io/fluxcd/kustomize-controller",
					},
				},
				{
					Type:       ArtifactTypeHelm,
					Reference:  "cilium",
					Version:    "1.16.3",
					Repository: "https://helm.cilium.io",
					Source: ArtifactSource{
						File:       "terraform/cni/cilium/variables.tf",
						Line:       5,
						Datasource: "helm",
						DepName:    "cilium",
						Package:    "cilium",
						HelmRepo:   "https://helm.cilium.io",
					},
				},
			},
		}

		// When marshaled to YAML and unmarshaled back
		data, err := yaml.Marshal(&original)
		if err != nil {
			t.Fatalf("expected no marshal error, got %v", err)
		}

		var roundTripped ArtifactManifest
		if err := yaml.Unmarshal(data, &roundTripped); err != nil {
			t.Fatalf("expected no unmarshal error, got %v", err)
		}

		// Then the result equals the original across every field
		if roundTripped.Version != original.Version {
			t.Errorf("Version: got %q, want %q", roundTripped.Version, original.Version)
		}
		if roundTripped.Blueprint != original.Blueprint {
			t.Errorf("Blueprint: got %+v, want %+v", roundTripped.Blueprint, original.Blueprint)
		}
		if len(roundTripped.Artifacts) != len(original.Artifacts) {
			t.Fatalf("Artifacts length: got %d, want %d", len(roundTripped.Artifacts), len(original.Artifacts))
		}
		for i, got := range roundTripped.Artifacts {
			want := original.Artifacts[i]
			if got != want {
				t.Errorf("Artifacts[%d]: got %+v, want %+v", i, got, want)
			}
		}
	})

	t.Run("MinimalManifest_OmitsEmptyFields", func(t *testing.T) {
		// Given a manifest with only required fields populated
		minimal := ArtifactManifest{
			Version: ManifestVersion,
			Artifacts: []ManifestEntry{
				{
					Type:      ArtifactTypeHelm,
					Reference: "cilium",
					Source: ArtifactSource{
						File:       "terraform/cni/cilium/variables.tf",
						Datasource: "helm",
					},
				},
			},
		}

		// When marshaled to YAML
		data, err := yaml.Marshal(&minimal)
		if err != nil {
			t.Fatalf("expected no marshal error, got %v", err)
		}
		out := string(data)

		// Then empty optional fields do not appear in the YAML output
		unwanted := []string{
			"name:", "version: \"\"", "commit:",
			"digest:", "repository:",
			"line:", "depName:", "package:", "helmRepo:",
		}
		for _, token := range unwanted {
			if containsExact(out, token) {
				t.Errorf("expected %q to be omitted, got:\n%s", token, out)
			}
		}

		// And required fields are present
		required := []string{"version:", "artifacts:", "type:", "reference:", "source:", "file:", "datasource:"}
		for _, token := range required {
			if !containsExact(out, token) {
				t.Errorf("expected %q to be present, got:\n%s", token, out)
			}
		}
	})

	t.Run("AllArtifactTypes_SerializeAsExpectedStrings", func(t *testing.T) {
		// Given every ArtifactType constant represented in a manifest
		types := []ArtifactType{
			ArtifactTypeDocker,
			ArtifactTypeHelm,
			ArtifactTypeGitHubRelease,
			ArtifactTypeGitTag,
		}
		expected := []string{
			"docker", "helm", "github-release", "git-tag",
		}

		entries := make([]ManifestEntry, len(types))
		for i, typ := range types {
			entries[i] = ManifestEntry{
				Type:      typ,
				Reference: "ref",
				Source:    ArtifactSource{File: "f", Datasource: "ds"},
			}
		}
		m := ArtifactManifest{Version: ManifestVersion, Artifacts: entries}

		// When marshaled
		data, err := yaml.Marshal(&m)
		if err != nil {
			t.Fatalf("expected no marshal error, got %v", err)
		}
		out := string(data)

		// Then each type appears as its documented lowercase string
		for _, want := range expected {
			if !containsExact(out, "type: "+want) {
				t.Errorf("expected %q in output, got:\n%s", "type: "+want, out)
			}
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// containsExact reports whether s contains substr. It exists so the test file
// can avoid importing strings just for a single call.
func containsExact(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
