package v1alpha1

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// The `flux:` key is a preferred alias of `kustomize:`; both deserialize into Kustomizations and a
// file may use either or, transitionally, both.

func TestFacet_FluxAlias(t *testing.T) {
	t.Run("FluxKeyPopulatesKustomizations", func(t *testing.T) {
		// Given a facet authored with the flux: key
		src := `kind: Facet
flux:
  - name: pki-install
    path: pki/install
`
		// When it is unmarshaled
		var f Facet
		if err := yaml.Unmarshal([]byte(src), &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then the entry lands in Kustomizations, indistinguishable from a kustomize: entry
		if len(f.Kustomizations) != 1 || f.Kustomizations[0].Name != "pki-install" || f.Kustomizations[0].Path != "pki/install" {
			t.Fatalf("flux entry not merged into Kustomizations: %+v", f.Kustomizations)
		}
	})

	t.Run("KustomizeKeyStillWorks", func(t *testing.T) {
		// Given the legacy kustomize: key
		src := `kind: Facet
kustomize:
  - name: legacy
    path: legacy
`
		// When it is unmarshaled
		var f Facet
		if err := yaml.Unmarshal([]byte(src), &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then it parses exactly as before
		if len(f.Kustomizations) != 1 || f.Kustomizations[0].Name != "legacy" {
			t.Fatalf("kustomize entry regressed: %+v", f.Kustomizations)
		}
	})

	t.Run("BothKeysMerge", func(t *testing.T) {
		// Given a file using both keys during a transition
		src := `kind: Facet
kustomize:
  - name: a
    path: a
flux:
  - name: b
    path: b
`
		// When it is unmarshaled
		var f Facet
		if err := yaml.Unmarshal([]byte(src), &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then both contribute to Kustomizations
		names := []string{}
		for _, k := range f.Kustomizations {
			names = append(names, k.Name)
		}
		if len(f.Kustomizations) != 2 {
			t.Fatalf("expected both kustomize: and flux: entries, got %v", names)
		}
	})

	t.Run("ConditionalFieldsSurviveTheAlias", func(t *testing.T) {
		// Given a flux entry carrying conditional fields
		src := `kind: Facet
flux:
  - name: dns
    path: dns
    when: "dns.enabled == true"
`
		// When it is unmarshaled
		var f Facet
		if err := yaml.Unmarshal([]byte(src), &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then the conditional fields are preserved (it decodes as a ConditionalKustomization)
		if len(f.Kustomizations) != 1 || f.Kustomizations[0].When != "dns.enabled == true" {
			t.Fatalf("conditional fields lost through the alias: %+v", f.Kustomizations)
		}
	})
}

func TestBlueprint_FluxAlias(t *testing.T) {
	t.Run("FluxKeyPopulatesKustomizations", func(t *testing.T) {
		// Given a blueprint authored with the flux: key
		src := `kind: Blueprint
flux:
  - name: x
    path: x
`
		// When it is unmarshaled
		var b Blueprint
		if err := yaml.Unmarshal([]byte(src), &b); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then the entry lands in Kustomizations
		if len(b.Kustomizations) != 1 || b.Kustomizations[0].Name != "x" {
			t.Fatalf("flux entry not merged into Kustomizations: %+v", b.Kustomizations)
		}
	})

	t.Run("KustomizeKeyStillWorksAndBothMerge", func(t *testing.T) {
		// Given both keys
		src := `kind: Blueprint
kustomize:
  - name: a
    path: a
flux:
  - name: b
    path: b
`
		// When it is unmarshaled
		var b Blueprint
		if err := yaml.Unmarshal([]byte(src), &b); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Then both are present
		if len(b.Kustomizations) != 2 {
			t.Fatalf("expected both entries, got %+v", b.Kustomizations)
		}
	})
}
