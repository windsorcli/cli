package inventory_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Components []testComponent `json:"components"`
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

	t.Run("NestsTheArtifactsOfEachSourceWithPrefixedRefs", func(t *testing.T) {
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

		// Then core is at the top, before busybox, and holds base and its own busybox
		bom := decodeBOM(t, data)
		busyboxPurl := "pkg:oci/busybox?repository_url=docker.io%2Flibrary%2Fbusybox&tag=1.37"
		basePurl := "pkg:oci/base?repository_url=ghcr.io%2Fwindsorcli%2Fbase&tag=v1.0.0"
		etcdPurl := "pkg:oci/etcd?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"
		if len(bom.Components) != 2 || bom.Components[0].Name != "ghcr.io/windsorcli/core" || bom.Components[1].BOMRef != busyboxPurl {
			t.Fatalf("expected core and busybox at the top, got %+v", bom.Components)
		}
		core := bom.Components[0]
		if len(core.Components) != 2 {
			t.Fatalf("expected two components under core, got %+v", core.Components)
		}

		// And each nested bom-ref has the source names in front, and the package URL does not change
		base, nestedBusybox := core.Components[0], core.Components[1]
		if base.BOMRef != "core/"+basePurl || base.Purl != basePurl {
			t.Errorf("expected base with a core prefix, got %+v", base)
		}
		if nestedBusybox.BOMRef != "core/"+busyboxPurl || nestedBusybox.Purl != busyboxPurl {
			t.Errorf("expected busybox with a core prefix, got %+v", nestedBusybox)
		}
		if len(base.Components) != 1 || base.Components[0].BOMRef != "core/base/"+etcdPurl || base.Components[0].Purl != etcdPurl {
			t.Errorf("expected etcd with a core/base prefix, got %+v", base.Components)
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
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled))

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

	t.Run("NestsTheBOMOfASourceArtifact", func(t *testing.T) {
		// Given a bundled blueprint that names core, and a cached core artifact that holds a BOM with a nested source
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
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled))
		blueprint := []byte("sources:\n- name: template\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n- name: git\n  url: https://github.com/example/git\n")

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, blueprint)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the writer pulls core only, and core is a data component with its tag
		if len(pulled) != 1 || pulled[0] != "oci://ghcr.io/windsorcli/core:v0.9.0" {
			t.Errorf("expected one pull of core, got %v", pulled)
		}
		bom := decodeBOM(t, data)
		if len(bom.Components) != 1 {
			t.Fatalf("expected core only at the top, got %+v", bom.Components)
		}
		core := bom.Components[0]
		if core.Type != "data" || core.Name != "ghcr.io/windsorcli/core" || core.Version != "v0.9.0" {
			t.Errorf("expected the core data component, got %+v", core)
		}

		// And the components of the core BOM nest under core with a core prefix, at every depth
		if len(core.Components) != 2 {
			t.Fatalf("expected base and etcd under core, got %+v", core.Components)
		}
		base := core.Components[0]
		if !strings.HasPrefix(base.BOMRef, "core/pkg:oci/base") || len(base.Components) != 1 || !strings.HasPrefix(base.Components[0].BOMRef, "core/base/pkg:oci/busybox") {
			t.Errorf("expected base and busybox with prefixes, got %+v", base)
		}
		if !strings.HasPrefix(core.Components[1].BOMRef, "core/pkg:oci/etcd") {
			t.Errorf("expected etcd with a core prefix, got %+v", core.Components[1])
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
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir, "base": baseDir}, &pulled))

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then core holds base and its scanned image
		bom := decodeBOM(t, data)
		if len(bom.Components) != 1 || len(bom.Components[0].Components) != 2 {
			t.Fatalf("expected core with two components, got %+v", bom.Components)
		}
		base, etcd := bom.Components[0].Components[0], bom.Components[0].Components[1]
		if !strings.HasPrefix(etcd.BOMRef, "core/pkg:oci/etcd") {
			t.Errorf("expected the scanned etcd image under core, got %+v", etcd)
		}

		// And the walk stops at core under base, because core is already above it
		if len(base.Components) != 1 || base.Components[0].Name != "ghcr.io/windsorcli/core" || len(base.Components[0].Components) != 0 {
			t.Errorf("expected core with no children under base, got %+v", base.Components)
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
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir, "base": baseDir}, &pulled))

		// When the writer is called
		data, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: core\n  url: oci://ghcr.io/windsorcli/core:v0.9.0\n"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then core holds base, with its scanned image, before the components of the core BOM
		bom := decodeBOM(t, data)
		core := bom.Components[0]
		if len(core.Components) != 2 || core.Components[0].Name != "ghcr.io/windsorcli/base" || !strings.HasPrefix(core.Components[1].BOMRef, "core/pkg:oci/etcd") {
			t.Fatalf("expected base and etcd under core, got %+v", core.Components)
		}
		if base := core.Components[0]; len(base.Components) != 1 || !strings.HasPrefix(base.Components[0].BOMRef, "core/base/pkg:oci/busybox") {
			t.Errorf("expected busybox under base, got %+v", base.Components)
		}
	})

	t.Run("ListsTwoSourcesWithOneURLOnce", func(t *testing.T) {
		// Given a bundled blueprint that names one core artifact under two names
		coreDir := t.TempDir()
		writeArtifactTar(t, coreDir, map[string]string{"metadata.yaml": "name: core\n"})
		var pulled []string
		writer := inventory.BundleBOMWriter(bundleBuilder(map[string]string{"core": coreDir}, &pulled))
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
		writer := inventory.BundleBOMWriter(bundleBuilder(dirs, &pulled))

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
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled))

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte("sources:\n- name: git\n  url: https://github.com/example/git\n  install: ${a.b ?? true}\n"))

		// Then it does not fail
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenAPullFails", func(t *testing.T) {
		// Given a builder whose pull fails
		var pulled []string
		builder := bundleBuilder(nil, &pulled)
		builder.PullFunc = func([]string) (map[string]string, error) {
			return nil, errors.New("unauthorized")
		}
		writer := inventory.BundleBOMWriter(builder)

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
		writer := inventory.BundleBOMWriter(bundleBuilder(nil, &pulled))

		// When the writer is called
		_, err := writer("app", "v1.0.0", nil, []byte("sources: [\n"))

		// Then it returns an error
		if err == nil {
			t.Error("expected an error")
		}
	})
}
