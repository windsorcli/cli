package inventory_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/inventory"
)

// =============================================================================
// Test Setup
// =============================================================================

// testBOM is the part of a CycloneDX BOM that the tests check.
type testBOM struct {
	BOMFormat   string `json:"bomFormat"`
	SpecVersion string `json:"specVersion"`
	Metadata    *struct {
		Component struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"component"`
	} `json:"metadata"`
	Components []testComponent `json:"components"`
}

// testComponent is the part of a CycloneDX component that the tests check.
type testComponent struct {
	BOMRef  string `json:"bom-ref"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Purl    string `json:"purl"`
	Hashes  []struct {
		Alg     string `json:"alg"`
		Content string `json:"content"`
	} `json:"hashes"`
}

// decodeBOM decodes data as a CycloneDX BOM and fails the test on an error.
func decodeBOM(t *testing.T, data []byte) testBOM {
	t.Helper()
	var bom testBOM
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatalf("failed to decode the BOM: %v", err)
	}
	return bom
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCycloneDX(t *testing.T) {
	t.Run("WritesOneComponentForEachEntry", func(t *testing.T) {
		// Given an inventory with charts, images and a blueprint
		inv := testInventory()

		// When CycloneDX is called
		data, err := inventory.CycloneDX(inv, "core", "v1.0.0")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM is CycloneDX 1.6 with the blueprint in its metadata
		bom := decodeBOM(t, data)
		if bom.BOMFormat != "CycloneDX" || bom.SpecVersion != "1.6" {
			t.Errorf("expected CycloneDX 1.6, got %s %s", bom.BOMFormat, bom.SpecVersion)
		}
		if bom.Metadata == nil || bom.Metadata.Component.Name != "core" || bom.Metadata.Component.Version != "v1.0.0" {
			t.Errorf("expected metadata for core v1.0.0, got %+v", bom.Metadata)
		}

		// And each entry has its type, name, version and package URL
		want := []testComponent{
			{Type: "data", Name: "ghcr.io/windsorcli/core", Version: "v0.9.0", Purl: "pkg:oci/core?repository_url=ghcr.io%2Fwindsorcli%2Fcore&tag=v0.9.0"},
			{Type: "application", Name: "ingress-nginx", Version: "4.15.1", Purl: "pkg:generic/ingress-nginx@4.15.1?download_url=https:%2F%2Fkubernetes.github.io%2Fingress-nginx"},
			{Type: "application", Name: "openebs", Version: "4.6.2", Purl: "pkg:oci/openebs?repository_url=ghcr.io%2Fopenebs%2Fcharts%2Fopenebs&tag=4.6.2"},
			{Type: "application", Name: "kyverno", Version: "3.9.2", Purl: "pkg:oci/kyverno?repository_url=registry.example.com%2Fwindsor%2Fkyverno&tag=3.9.2"},
			{Type: "container", Name: "quay.io/coreos/etcd", Version: "v3.7.2", Purl: "pkg:oci/etcd@sha256:abc?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"},
			{Type: "container", Name: "docker.io/library/busybox", Version: "1.37", Purl: "pkg:oci/busybox?repository_url=docker.io%2Flibrary%2Fbusybox&tag=1.37"},
			{Type: "container", Name: "mirror.test:5000/app", Version: "sha256:def", Purl: "pkg:oci/app@sha256:def?repository_url=mirror.test:5000%2Fapp"},
		}
		if len(bom.Components) != len(want) {
			t.Fatalf("expected %d components, got %d", len(want), len(bom.Components))
		}
		for i, w := range want {
			got := bom.Components[i]
			if got.Type != w.Type || got.Name != w.Name || got.Version != w.Version || got.Purl != w.Purl {
				t.Errorf("component %d: expected %+v, got %+v", i, w, got)
			}
			if got.BOMRef != got.Purl {
				t.Errorf("component %d: expected the bom-ref to be the package URL, got %q", i, got.BOMRef)
			}
		}

		// And a digest is a SHA-256 hash
		etcd := bom.Components[4]
		if len(etcd.Hashes) != 1 || etcd.Hashes[0].Alg != "SHA-256" || etcd.Hashes[0].Content != "abc" {
			t.Errorf("expected one SHA-256 hash, got %+v", etcd.Hashes)
		}
		if len(bom.Components[5].Hashes) != 0 {
			t.Errorf("expected no hash without a digest, got %+v", bom.Components[5].Hashes)
		}
	})

	t.Run("KeepsOneComponentForEachPackageURL", func(t *testing.T) {
		// Given a chart and an image tag that name the same OCI repository and tag
		inv := inventory.Inventory{Artifacts: []inventory.Entry{
			{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: "ghcr.io/fluent/helm-charts/fluent-operator", Name: "fluent-operator", Version: "4.3.0"},
			{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "ghcr.io/fluent/helm-charts/fluent-operator", Version: "4.3.0"},
		}}

		// When CycloneDX is called
		data, err := inventory.CycloneDX(inv, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM has one component, the chart, so each bom-ref is unique
		bom := decodeBOM(t, data)
		if len(bom.Components) != 1 {
			t.Fatalf("expected 1 component, got %+v", bom.Components)
		}
		if bom.Components[0].Type != "application" {
			t.Errorf("expected the chart component, got %+v", bom.Components[0])
		}
	})

	t.Run("OmitsMetadataWithNoName", func(t *testing.T) {
		// Given an empty inventory
		inv := inventory.Inventory{}

		// When CycloneDX is called with no name
		data, err := inventory.CycloneDX(inv, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM has no metadata
		if bom := decodeBOM(t, data); bom.Metadata != nil {
			t.Errorf("expected no metadata, got %+v", bom.Metadata)
		}
	})

	t.Run("GivesTheSameBytesForTheSameInventory", func(t *testing.T) {
		// Given an inventory
		inv := testInventory()

		// When CycloneDX is called twice
		first, err := inventory.CycloneDX(inv, "core", "v1.0.0")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		second, err := inventory.CycloneDX(inv, "core", "v1.0.0")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOMs are the same, with no serial number or timestamp
		if !bytes.Equal(first, second) {
			t.Error("expected the same BOM bytes")
		}
		if bytes.Contains(first, []byte("serialNumber")) || bytes.Contains(first, []byte("timestamp")) {
			t.Error("expected no serial number and no timestamp")
		}
	})
}

func TestBundleBOM(t *testing.T) {
	t.Run("NormalizesAndDedupesTheTags", func(t *testing.T) {
		// Given tags for an HTTP chart, an OCI chart and one Docker Hub image in two files
		tags := []artifact.ArtifactTag{
			{Type: artifact.ArtifactTagImage, Name: "busybox", Version: "1.37", File: "terraform/main.tf", Line: 3},
			{Type: artifact.ArtifactTagChart, Name: "openebs", Repo: "oci://ghcr.io/openebs/charts/", Version: "4.6.2", File: "kustomize/b.yaml", Line: 1},
			{Type: artifact.ArtifactTagChart, Name: "ingress-nginx", Repo: "https://kubernetes.github.io/ingress-nginx", Version: "4.15.1", File: "kustomize/a.yaml", Line: 1},
			{Type: artifact.ArtifactTagImage, Name: "docker.io/library/busybox", Version: "1.37", File: "_template/values.yaml", Line: 9},
		}

		// When BundleBOM is called
		data, err := inventory.BundleBOM("core", "v1.0.0", tags)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM has one component for each artifact, sorted like the inventory
		bom := decodeBOM(t, data)
		want := []string{
			"pkg:oci/openebs?repository_url=ghcr.io%2Fopenebs%2Fcharts%2Fopenebs&tag=4.6.2",
			"pkg:generic/ingress-nginx@4.15.1?download_url=https:%2F%2Fkubernetes.github.io%2Fingress-nginx",
			"pkg:oci/busybox?repository_url=docker.io%2Flibrary%2Fbusybox&tag=1.37",
		}
		if len(bom.Components) != len(want) {
			t.Fatalf("expected %d components, got %+v", len(want), bom.Components)
		}
		for i, w := range want {
			if bom.Components[i].Purl != w {
				t.Errorf("component %d: expected %q, got %q", i, w, bom.Components[i].Purl)
			}
		}

		// And the metadata names the blueprint
		if bom.Metadata == nil || bom.Metadata.Component.Name != "core" || bom.Metadata.Component.Version != "v1.0.0" {
			t.Errorf("expected metadata for core v1.0.0, got %+v", bom.Metadata)
		}
	})
}
