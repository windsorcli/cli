package inventory_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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
	Components   []testComponent `json:"components"`
	Dependencies []struct {
		Ref       string   `json:"ref"`
		DependsOn []string `json:"dependsOn"`
	} `json:"dependencies"`
}

// dependsOn returns the bom-refs that ref depends on in bom, and false when bom has no entry for ref.
func (b testBOM) dependsOn(ref string) ([]string, bool) {
	for _, d := range b.Dependencies {
		if d.Ref == ref {
			return d.DependsOn, true
		}
	}
	return nil, false
}

// refs returns the bom-ref of each component of b, in order.
func (b testBOM) refs() []string {
	out := make([]string, 0, len(b.Components))
	for _, c := range b.Components {
		out = append(out, c.BOMRef)
	}
	return out
}

// testComponent is the part of a CycloneDX component that the tests check.
type testComponent struct {
	Components []testComponent `json:"components"`
	BOMRef     string          `json:"bom-ref"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	Purl       string          `json:"purl"`
	Hashes     []struct {
		Alg     string `json:"alg"`
		Content string `json:"content"`
	} `json:"hashes"`
}

// Package URLs and digests of the tests.
const (
	coreDigestHex = "1111111111111111111111111111111111111111111111111111111111111111"
	baseDigestHex = "2222222222222222222222222222222222222222222222222222222222222222"
	corePurl      = "pkg:oci/core?repository_url=ghcr.io%2Fwindsorcli%2Fcore&tag=v0.9.0"
	basePurl      = "pkg:oci/base?repository_url=ghcr.io%2Fwindsorcli%2Fbase&tag=v1.0.0"
	busyboxPurl   = "pkg:oci/busybox?repository_url=docker.io%2Flibrary%2Fbusybox&tag=1.37"
	etcdPurl      = "pkg:oci/etcd?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"
)

// decodeBOM decodes data as a CycloneDX BOM and fails the test on an error.
func decodeBOM(t *testing.T, data []byte) testBOM {
	t.Helper()
	var bom testBOM
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatalf("failed to decode the BOM: %v", err)
	}
	return bom
}

// testTree returns the tree of testInventory: its blueprint entries as sources, and its other entries as artifacts.
func testTree() inventory.SourceTree {
	return flatTree(testInventory())
}

// flatTree returns a tree with each blueprint entry of inv as a source, and each other entry as an artifact.
func flatTree(inv inventory.Inventory) inventory.SourceTree {
	var tree inventory.SourceTree
	for _, e := range inv.Artifacts {
		if e.Kind == inventory.KindBlueprint {
			tree.Sources = append(tree.Sources, inventory.SourceTree{Name: "core", Entry: e})
			continue
		}
		tree.Artifacts = append(tree.Artifacts, e)
	}
	return tree
}

// writeArtifactTar writes an artifact.tar that holds files into the cache directory dir.
func writeArtifactTar(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "artifact.tar"))
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// writeDigest writes the manifest digest sha256:<hex> to the digest file of the cache directory dir.
func writeDigest(t *testing.T, dir, hex string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "digest"), []byte("sha256:"+hex), 0o600); err != nil {
		t.Fatal(err)
	}
}

// bundleBuilder returns a mock artifact builder that caches each source in dirs, by its repository
// name, and records each reference that it pulls.
func bundleBuilder(dirs map[string]string, pulled *[]string) *artifact.MockArtifact {
	builder := artifact.NewMockArtifact()
	builder.PullFunc = func(refs []string) (map[string]string, error) {
		*pulled = append(*pulled, refs...)
		return nil, nil
	}
	builder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
		repository, tag, _ := strings.Cut(strings.TrimPrefix(ref, "oci://ghcr.io/"), ":")
		return "ghcr.io", repository, tag, nil
	}
	builder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
		return dirs[repository[strings.LastIndex(repository, "/")+1:]], nil
	}
	return builder
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestCycloneDX(t *testing.T) {
	t.Run("WritesOneComponentForEachEntry", func(t *testing.T) {
		// Given a tree with charts, images and a source
		tree := testTree()

		// When CycloneDX is called
		data, err := inventory.CycloneDX(tree, "core", "v1.0.0")
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
		tree := flatTree(inventory.Inventory{Artifacts: []inventory.Entry{
			{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: "ghcr.io/fluent/helm-charts/fluent-operator", Name: "fluent-operator", Version: "4.3.0"},
			{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "ghcr.io/fluent/helm-charts/fluent-operator", Version: "4.3.0"},
		}})

		// When CycloneDX is called
		data, err := inventory.CycloneDX(tree, "", "")
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
		// Given an empty tree
		tree := inventory.SourceTree{}

		// When CycloneDX is called with no name
		data, err := inventory.CycloneDX(tree, "", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM has no metadata
		if bom := decodeBOM(t, data); bom.Metadata != nil {
			t.Errorf("expected no metadata, got %+v", bom.Metadata)
		}
	})

	t.Run("GivesTheSameBytesForTheSameTree", func(t *testing.T) {
		// Given a tree
		tree := testTree()

		// When CycloneDX is called twice
		first, err := inventory.CycloneDX(tree, "core", "v1.0.0")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		second, err := inventory.CycloneDX(tree, "core", "v1.0.0")
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

	t.Run("ListsEachArtifactOnceAndTheSourcesThatPullIt", func(t *testing.T) {
		// Given a tree in which the blueprint and core both pull busybox, and core names base
		busybox := inventory.Entry{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "docker.io/library/busybox", Version: "1.37"}
		etcd := inventory.Entry{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2"}
		tree := inventory.SourceTree{
			Artifacts: []inventory.Entry{busybox},
			Sources: []inventory.SourceTree{{
				Name:      "core",
				Entry:     inventory.Entry{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/core", Version: "v0.9.0"},
				Artifacts: []inventory.Entry{busybox},
				Sources: []inventory.SourceTree{{
					Name:      "base",
					Entry:     inventory.Entry{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/base", Version: "v1.0.0"},
					Artifacts: []inventory.Entry{etcd},
				}},
			}},
		}

		// When CycloneDX is called
		data, err := inventory.CycloneDX(tree, "app", "v1.0.0")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM lists each artifact once, sources first, in the order of the walk, with no nested components
		bom := decodeBOM(t, data)
		want := []string{corePurl, basePurl, etcdPurl, busyboxPurl}
		if got := bom.refs(); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("expected components %v, got %v", want, got)
		}
		for _, c := range bom.Components {
			if len(c.Components) != 0 || c.BOMRef != c.Purl {
				t.Errorf("expected a flat component with its package URL as bom-ref, got %+v", c)
			}
		}

		// And the graph tells that app and core both pull busybox, and that base pulls etcd
		for ref, wantDeps := range map[string][]string{
			"app":       {corePurl, busyboxPurl},
			corePurl:    {basePurl, busyboxPurl},
			basePurl:    {etcdPurl},
			busyboxPurl: nil,
			etcdPurl:    nil,
		} {
			got, ok := bom.dependsOn(ref)
			if !ok {
				t.Errorf("expected a dependencies entry for %s", ref)
				continue
			}
			sortedWant := append([]string(nil), wantDeps...)
			sort.Strings(sortedWant)
			if strings.Join(got, " ") != strings.Join(sortedWant, " ") {
				t.Errorf("%s: expected dependsOn %v, got %v", ref, sortedWant, got)
			}
		}
	})
}

func TestBundleBOMWriter(t *testing.T) {
	t.Run("NormalizesAndDedupesTheTags", func(t *testing.T) {
		// Given tags for an HTTP chart, an OCI chart and one Docker Hub image in two files, and no blueprint
		tags := []artifact.ArtifactTag{
			{Type: artifact.ArtifactTagImage, Name: "busybox", Version: "1.37", File: "terraform/main.tf", Line: 3},
			{Type: artifact.ArtifactTagChart, Name: "openebs", Repo: "oci://ghcr.io/openebs/charts/", Version: "4.6.2", File: "kustomize/b.yaml", Line: 1},
			{Type: artifact.ArtifactTagChart, Name: "ingress-nginx", Repo: "https://kubernetes.github.io/ingress-nginx", Version: "4.15.1", File: "kustomize/a.yaml", Line: 1},
			{Type: artifact.ArtifactTagImage, Name: "docker.io/library/busybox", Version: "1.37", File: "_template/values.yaml", Line: 9},
		}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled), nil)

		// When the writer is called
		data, err := writer("core", "v1.0.0", tags, nil)
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

		// And the metadata names the blueprint, and nothing is pulled
		if bom.Metadata == nil || bom.Metadata.Component.Name != "core" || bom.Metadata.Component.Version != "v1.0.0" {
			t.Errorf("expected metadata for core v1.0.0, got %+v", bom.Metadata)
		}
		if len(pulled) != 0 {
			t.Errorf("expected no pull, got %v", pulled)
		}
	})

	t.Run("AddsTheComponentsAndEdgesOfASourceBOM", func(t *testing.T) {
		// Given a bundled blueprint that names core, and a cached core artifact whose BOM names base
		coreTree := inventory.SourceTree{
			Artifacts: []inventory.Entry{{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2"}},
			Sources: []inventory.SourceTree{{
				Name:  "base",
				Entry: inventory.Entry{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/base", Version: "v1.0.0"},
				Artifacts: []inventory.Entry{
					{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "docker.io/library/busybox", Version: "1.37"},
				},
			}},
		}
		coreBOM, err := inventory.CycloneDX(coreTree, "core", "v0.9.0")
		if err != nil {
			t.Fatal(err)
		}
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": string(coreBOM)})
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), nil)
		blueprint := []byte("sources:\n- name: template\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n- name: git\n  url: https://github.com/example/git\n")

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, blueprint)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the writer pulls core only, and the BOM lists core, base and both images once
		if len(pulled) != 1 || pulled[0] != "oci://ghcr.io/windsorcli/core:v0.9.0" {
			t.Errorf("expected one pull of core, got %v", pulled)
		}
		bom := decodeBOM(t, data)
		want := []string{corePurl, basePurl, busyboxPurl, etcdPurl}
		if got := bom.refs(); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("expected components %v, got %v", want, got)
		}
		if bom.Components[0].Type != "data" || bom.Components[0].Version != "v0.9.0" {
			t.Errorf("expected the core data component, got %+v", bom.Components[0])
		}

		// And app pulls core, core pulls what the core BOM says it pulls, and base keeps its edge
		for ref, wantDeps := range map[string]string{
			"app":    corePurl,
			corePurl: basePurl + " " + etcdPurl,
			basePurl: busyboxPurl,
		} {
			if got, _ := bom.dependsOn(ref); strings.Join(got, " ") != wantDeps {
				t.Errorf("%s: expected dependsOn %q, got %v", ref, wantDeps, got)
			}
		}
		if _, ok := bom.dependsOn("core"); ok {
			t.Error("expected no entry for the metadata bom-ref of the core BOM")
		}
	})

	t.Run("ScansASourceArtifactWithNoBOM", func(t *testing.T) {
		// Given a cached core artifact with no BOM, a tagged file, and a blueprint that names base
		coreDir, baseDir := t.TempDir(), t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"metadata.yaml": "name: core\n"})
		writeFile(t, coreDir, "kustomize/app/values.yaml", "image: quay.io/coreos/etcd:v3.7.2 # artifact: type=image\n")
		writeFile(t, coreDir, "_template/blueprint.yaml", "sources:\n- name: base\n  url: oci://ghcr.io/windsorcli/base:v1.0.0\n")
		writeArtifactTar(t, baseDir, map[string]string{"metadata.yaml": "name: base\n"})
		writeFile(t, baseDir, "_template/blueprint.yaml", "sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n")
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir, "base": baseDir}, &pulled), nil)

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then core pulls base and its scanned image, and base pulls core back
		bom := decodeBOM(t, data)
		if got := bom.refs(); strings.Join(got, " ") != strings.Join([]string{corePurl, basePurl, etcdPurl}, " ") {
			t.Fatalf("expected core, base and etcd, got %v", got)
		}
		if got, _ := bom.dependsOn(corePurl); strings.Join(got, " ") != basePurl+" "+etcdPurl {
			t.Errorf("expected core to pull base and etcd, got %v", got)
		}
		if got, _ := bom.dependsOn(basePurl); strings.Join(got, " ") != corePurl {
			t.Errorf("expected base to pull core, got %v", got)
		}
		if len(pulled) != 2 {
			t.Errorf("expected pulls of core and base, got %v", pulled)
		}
	})

	t.Run("DescendsIntoASourceThatTheSourceBOMDoesNotHold", func(t *testing.T) {
		// Given a cached core artifact whose BOM lists its own image only, and whose blueprint names base
		coreBOM, err := inventory.CycloneDX(inventory.SourceTree{Artifacts: []inventory.Entry{
			{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2"},
		}}, "core", "v0.9.0")
		if err != nil {
			t.Fatal(err)
		}
		coreDir, baseDir := t.TempDir(), t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": string(coreBOM)})
		writeFile(t, coreDir, "_template/blueprint.yaml", "sources:\n- name: base\n  url: oci://ghcr.io/windsorcli/base:v1.0.0\n")
		writeArtifactTar(t, baseDir, map[string]string{"metadata.yaml": "name: base\n"})
		writeFile(t, baseDir, "kustomize/app/values.yaml", "image: docker.io/library/busybox:1.37 # artifact: type=image\n")
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir, "base": baseDir}, &pulled), nil)

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then core pulls base and the etcd image of its BOM, and base pulls its scanned image
		bom := decodeBOM(t, data)
		if got, _ := bom.dependsOn(corePurl); strings.Join(got, " ") != basePurl+" "+etcdPurl {
			t.Errorf("expected core to pull base and etcd, got %v", got)
		}
		if got, _ := bom.dependsOn(basePurl); strings.Join(got, " ") != busyboxPurl {
			t.Errorf("expected base to pull busybox, got %v", got)
		}
	})

	t.Run("RecordsTheDigestOfThePulledSource", func(t *testing.T) {
		// Given a cached core artifact with the digest of its pull, and a tag in the URL only
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"metadata.yaml": "name: core\n"})
		writeDigest(t, coreDir, coreDigestHex)
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), nil)

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the core component has the tag and the digest of the pull, and app pulls it
		bom := decodeBOM(t, data)
		core := bom.Components[0]
		if core.Type != "data" || core.Version != "v0.9.0" || len(core.Hashes) != 1 || core.Hashes[0].Content != coreDigestHex {
			t.Fatalf("expected core with its tag and digest, got %+v", core)
		}
		if !strings.Contains(core.Purl, coreDigestHex) {
			t.Errorf("expected the digest in the package URL, got %q", core.Purl)
		}
		if got, _ := bom.dependsOn("app"); strings.Join(got, " ") != core.BOMRef {
			t.Errorf("expected app to pull core, got %v", got)
		}
	})

	t.Run("MatchesASourceInTheSourceBOMByItsTagAndNotItsDigest", func(t *testing.T) {
		// Given a core BOM that lists base with the digest of its pull, and a blueprint in core that names base
		coreBOM, err := inventory.CycloneDX(inventory.SourceTree{Sources: []inventory.SourceTree{{
			Name:  "base",
			Entry: inventory.Entry{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/base", Version: "v1.0.0", Digest: "sha256:" + baseDigestHex},
		}}}, "core", "v0.9.0")
		if err != nil {
			t.Fatal(err)
		}
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": string(coreBOM)})
		writeFile(t, coreDir, "_template/blueprint.yaml", "sources:\n- name: base\n  url: oci://ghcr.io/windsorcli/base:v1.0.0\n")
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), nil)

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the writer pulls core only, and the BOM lists base once
		if len(pulled) != 1 {
			t.Errorf("expected one pull of core, got %v", pulled)
		}
		if got := decodeBOM(t, data).refs(); len(got) != 2 {
			t.Errorf("expected core and base, got %v", got)
		}
	})

	t.Run("KeepsOneComponentForASourceThatASourceBOMListsUnderAnotherDigest", func(t *testing.T) {
		// Given a blueprint that names core and base, and a core BOM that lists base with no digest
		coreBOM, err := inventory.CycloneDX(inventory.SourceTree{Sources: []inventory.SourceTree{{
			Name:      "base",
			Entry:     inventory.Entry{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/base", Version: "v1.0.0"},
			Artifacts: []inventory.Entry{{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2"}},
		}}}, "core", "v0.9.0")
		if err != nil {
			t.Fatal(err)
		}
		coreDir, baseDir := t.TempDir(), t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": string(coreBOM)})
		writeArtifactTar(t, baseDir, map[string]string{"metadata.yaml": "name: base\n"})
		writeDigest(t, baseDir, baseDigestHex)
		writeFile(t, baseDir, "kustomize/app/values.yaml", "image: quay.io/coreos/etcd:v3.7.2 # artifact: type=image\n")
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir, "base": baseDir}, &pulled), nil)
		blueprint := []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n- name: zbase\n  url: oci://ghcr.io/windsorcli/base:v1.0.0\n")

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, blueprint)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM lists base once, with the digest of its pull, and core pulls that component
		bom := decodeBOM(t, data)
		var bases []testComponent
		for _, c := range bom.Components {
			if c.Name == "ghcr.io/windsorcli/base" {
				bases = append(bases, c)
			}
		}
		if len(bases) != 1 || len(bases[0].Hashes) != 1 || bases[0].Hashes[0].Content != baseDigestHex {
			t.Fatalf("expected one base component with its digest, got %+v", bases)
		}
		if got, _ := bom.dependsOn(corePurl); !slices.Contains(got, bases[0].BOMRef) {
			t.Errorf("expected core to pull base, got %v", got)
		}
		if got, _ := bom.dependsOn(bases[0].BOMRef); strings.Join(got, " ") != etcdPurl {
			t.Errorf("expected base to pull etcd, got %v", got)
		}
	})

	t.Run("ListsTwoSourcesWithOneURLOnce", func(t *testing.T) {
		// Given a bundled blueprint that names one core artifact under two names
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"metadata.yaml": "name: core\n"})
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), nil)
		blueprint := []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n- name: core-b\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n")

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, blueprint)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the BOM has one core component, and the writer pulls core once
		if bom := decodeBOM(t, data); len(bom.Components) != 1 {
			t.Errorf("expected one core component, got %+v", bom.Components)
		}
		if len(pulled) != 1 {
			t.Errorf("expected one pull, got %v", pulled)
		}
	})

	t.Run("ReturnsErrorForSourcesNestedTooDeep", func(t *testing.T) {
		// Given a chain of cached sources in which each names the next
		dirs := make(map[string]string)
		for i := 0; i <= 20; i++ {
			dir := t.TempDir()
			writeArtifactTar(t, dir, map[string]string{"metadata.yaml": "name: s\n"})
			writeFile(t, dir, "_template/blueprint.yaml", fmt.Sprintf("sources:\n- name: s%d\n  url: oci://ghcr.io/windsorcli/s%d:v1\n", i+1, i+1))
			dirs[fmt.Sprintf("s%d", i)] = dir
		}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(dirs, &pulled), nil)

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: s0\n  url: oci://ghcr.io/windsorcli/s0:v1\n"))

		// Then it fails and names the depth
		if err == nil || !strings.Contains(err.Error(), "16 sources deep") {
			t.Errorf("expected a depth error, got %v", err)
		}
	})

	t.Run("ReadsOnlyTheNameAndURLOfEachSource", func(t *testing.T) {
		// Given a bundled blueprint whose source has an install expression
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled), nil)

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: git\n  url: https://github.com/example/git\n  install: ${a.b ?? true}\n"))

		// Then it does not fail
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("KeepsTheBundleAuthoritativeOverASourceBOM", func(t *testing.T) {
		// Given a core BOM that claims an edge from the bundle, an edge from the bundle image, an edge to a
		// missing ref, a different version of the bundle image, and a nested component with no bom-ref
		coreBOM := `{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,
			"metadata":{"component":{"bom-ref":"core","type":"data","name":"core"}},
			"components":[
				{"bom-ref":"` + busyboxPurl + `","type":"container","name":"evil","version":"6.6.6","purl":"` + busyboxPurl + `"},
				{"bom-ref":"` + basePurl + `","type":"data","name":"ghcr.io/windsorcli/base","purl":"` + basePurl + `",
					"components":[{"type":"container","name":"quay.io/coreos/etcd","purl":"` + etcdPurl + `"}]}],
			"dependencies":[
				{"ref":"core","dependsOn":["` + basePurl + `","pkg:oci/missing"]},
				{"ref":"app","dependsOn":["` + basePurl + `"]},
				{"ref":"` + busyboxPurl + `","dependsOn":["` + basePurl + `"]}]}`
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": coreBOM})
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), nil)
		tags := []artifact.ArtifactTag{{Type: artifact.ArtifactTagImage, Name: "docker.io/library/busybox", Version: "1.37", File: "kustomize/a.yaml", Line: 1}}

		// When the writer is called
		data, err := writer("app", "v1.0.0", tags, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the bundle image keeps its own name and version
		bom := decodeBOM(t, data)
		for _, c := range bom.Components {
			if c.BOMRef == busyboxPurl && (c.Name != "docker.io/library/busybox" || c.Version != "1.37") {
				t.Errorf("expected the bundle busybox component, got %+v", c)
			}
		}

		// And the source BOM adds no edge from the bundle or its image, and no edge to a missing ref
		if got, _ := bom.dependsOn("app"); strings.Join(got, " ") != busyboxPurl+" "+corePurl {
			t.Errorf("expected app to pull core and busybox only, got %v", got)
		}
		if got, _ := bom.dependsOn(busyboxPurl); len(got) != 0 {
			t.Errorf("expected busybox to pull nothing, got %v", got)
		}
		if got, _ := bom.dependsOn(corePurl); strings.Join(got, " ") != basePurl {
			t.Errorf("expected core to pull base only, got %v", got)
		}

		// And the nested component uses its package URL as bom-ref, with an edge from its parent
		if got, _ := bom.dependsOn(basePurl); strings.Join(got, " ") != etcdPurl {
			t.Errorf("expected base to pull etcd, got %v", got)
		}
	})

	t.Run("ReturnsErrorForTooManySources", func(t *testing.T) {
		// Given a bundled blueprint that names more sources than the limit
		dir := t.TempDir()
		writeArtifactTar(t, dir, map[string]string{"metadata.yaml": "name: s\n"})
		dirs := make(map[string]string)
		var blueprint strings.Builder
		blueprint.WriteString("sources:\n")
		for i := 0; i <= 256; i++ {
			dirs[fmt.Sprintf("s%d", i)] = dir
			fmt.Fprintf(&blueprint, "- name: s%d\n  url: oci://ghcr.io/windsorcli/s%d:v1\n", i, i)
		}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(dirs, &pulled), nil)

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte(blueprint.String()))

		// Then it fails and names the limit
		if err == nil || !strings.Contains(err.Error(), "limit of 256 sources") {
			t.Errorf("expected a source limit error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenAPullFails", func(t *testing.T) {
		// Given a builder whose pull fails
		var pulled []string
		builder := bundleBuilder(nil, &pulled)
		builder.PullFunc = func([]string) (map[string]string, error) {
			return nil, errors.New("unauthorized")
		}
		writer := inventory.BundleBOMWriter(builder, nil)

		// When the writer is called for a blueprint that names core
		_, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))

		// Then the error names the source and the cause
		if err == nil || !strings.Contains(err.Error(), `source "core"`) || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected a pull error that names core, got %v", err)
		}
	})

	t.Run("ReturnsErrorForAMalformedBlueprint", func(t *testing.T) {
		// Given a bundled blueprint that does not parse
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled), nil)

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte("sources: [\n"))

		// Then it returns an error
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("WritesTheRegistryDigestOfEachTagWithNoDigest", func(t *testing.T) {
		// Given an image tagged in two files, an HTTP chart, a pinned image and a chart with a version range
		tags := []artifact.ArtifactTag{
			{Type: artifact.ArtifactTagImage, Name: "busybox", Version: "1.37", File: "a.yaml", Line: 1},
			{Type: artifact.ArtifactTagImage, Name: "docker.io/library/busybox", Version: "1.37", File: "b.yaml", Line: 1},
			{Type: artifact.ArtifactTagChart, Name: "ingress-nginx", Repo: "https://kubernetes.github.io/ingress-nginx", Version: "4.15.1", File: "c.yaml", Line: 1},
			{Type: artifact.ArtifactTagImage, Name: "quay.io/coreos/etcd", Version: "v3.7.2", Digest: "sha256:" + baseDigestHex, File: "d.yaml", Line: 1},
			{Type: artifact.ArtifactTagChart, Name: "openebs", Repo: "oci://ghcr.io/openebs/charts", Version: "4.x", File: "e.yaml", Line: 1},
		}
		lookup := &fakeDigests{digests: map[string]string{
			"docker.io/library/busybox:1.37":                    "sha256:" + coreDigestHex,
			"https://kubernetes.github.io/ingress-nginx:4.15.1": "sha256:" + coreDigestHex,
		}}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled), lookup)

		// When the writer is called
		data, err := writer("app", "v1.0.0", tags, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the writer looks up busybox and the HTTP chart once each, and nothing else
		if got := strings.Join(lookup.sortedCalls(), " "); got != "docker.io/library/busybox:1.37 https://kubernetes.github.io/ingress-nginx:4.15.1" {
			t.Errorf("expected one lookup of busybox and of ingress-nginx, got %s", got)
		}

		// And busybox has the digest in its package URL and its hashes, and the HTTP chart has it in its hashes
		bom := decodeBOM(t, data)
		hashes := make(map[string]string)
		for _, c := range bom.Components {
			if len(c.Hashes) == 1 {
				hashes[c.Purl] = c.Hashes[0].Content
			}
		}
		busybox := "pkg:oci/busybox@sha256:" + coreDigestHex + "?repository_url=docker.io%2Flibrary%2Fbusybox&tag=1.37"
		ingress := "pkg:generic/ingress-nginx@4.15.1?download_url=https:%2F%2Fkubernetes.github.io%2Fingress-nginx"
		if hashes[busybox] != coreDigestHex || hashes[ingress] != coreDigestHex {
			t.Errorf("expected the looked-up digest for busybox and ingress-nginx, got %v", hashes)
		}
		if len(bom.Components) != 4 {
			t.Errorf("expected one busybox component and four in all, got %v", bom.refs())
		}
	})

	t.Run("LooksUpTheTagsOfASourceWithNoBOM", func(t *testing.T) {
		// Given a cached core artifact with no BOM, whose files tag one image
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"metadata.yaml": "name: core\n"})
		writeFile(t, coreDir, "kustomize/etcd/values.yaml", "image: quay.io/coreos/etcd:v3.7.2 # artifact: type=image\n")
		lookup := &fakeDigests{digests: map[string]string{"quay.io/coreos/etcd:v3.7.2": "sha256:" + baseDigestHex}}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), lookup)

		// When the writer is called for a blueprint that names core
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the etcd image of core has the looked-up digest
		want := "pkg:oci/etcd@sha256:" + baseDigestHex + "?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"
		if refs := decodeBOM(t, data).refs(); !slices.Contains(refs, want) {
			t.Errorf("expected %s, got %v", want, refs)
		}
	})

	t.Run("ReadsNoDigestForTheComponentsOfASourceBOM", func(t *testing.T) {
		// Given a cached core artifact whose BOM lists an image with no digest
		coreBOM, err := inventory.CycloneDX(inventory.SourceTree{Artifacts: []inventory.Entry{
			{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: "quay.io/coreos/etcd", Version: "v3.7.2"},
		}}, "core", "v0.9.0")
		if err != nil {
			t.Fatal(err)
		}
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"artifact-manifest.cdx.json": string(coreBOM)})
		lookup := &fakeDigests{}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled), lookup)

		// When the writer is called for a blueprint that names core
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the writer looks up nothing, and the BOM keeps the component of core as it is
		if len(lookup.calls) != 0 {
			t.Errorf("expected no lookup, got %v", lookup.calls)
		}
		if refs := decodeBOM(t, data).refs(); !slices.Contains(refs, etcdPurl) {
			t.Errorf("expected %s, got %v", etcdPurl, refs)
		}
	})

	t.Run("ReturnsErrorWhenADigestLookupFails", func(t *testing.T) {
		// Given a tag whose registry does not answer
		tags := []artifact.ArtifactTag{{Type: artifact.ArtifactTagImage, Name: "ghcr.io/example/app", Version: "1.0.0", File: "a.yaml", Line: 1}}
		lookup := &fakeDigests{err: errors.New("connection refused")}
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled), lookup)

		// When the writer is called
		_, err := writer("app", "v1.0.0", tags, nil)

		// Then the error names the artifact and the cause
		if err == nil || !strings.Contains(err.Error(), "image ghcr.io/example/app:1.0.0") || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("expected a lookup error that names the image, got %v", err)
		}
	})
}
