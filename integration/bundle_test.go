//go:build integration
// +build integration

package integration

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Test Helpers
// =============================================================================

// extractedBOM is the part of a CycloneDX BOM that the integration tests check.
type extractedBOM struct {
	SpecVersion  string               `json:"specVersion"`
	Components   []extractedComponent `json:"components"`
	Dependencies []struct {
		Ref       string   `json:"ref"`
		DependsOn []string `json:"dependsOn"`
	} `json:"dependencies"`
}

// Package URLs of the source tests.
const (
	coreSourcePurl = "pkg:oci/core?repository_url=ghcr.io%2Fwindsorcli%2Fcore&tag=v0.1.0"
	etcdImagePurl  = "pkg:oci/etcd?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"
)

// dependsOn returns the bom-refs that ref depends on in bom.
func (b extractedBOM) dependsOn(ref string) []string {
	for _, d := range b.Dependencies {
		if d.Ref == ref {
			return d.DependsOn
		}
	}
	return nil
}

// extractedComponent is the part of a CycloneDX component that the integration tests check.
type extractedComponent struct {
	BOMRef     string               `json:"bom-ref"`
	Type       string               `json:"type"`
	Name       string               `json:"name"`
	Version    string               `json:"version"`
	Purl       string               `json:"purl"`
	Components []extractedComponent `json:"components"`
	Hashes     []struct {
		Alg     string `json:"alg"`
		Content string `json:"content"`
	} `json:"hashes"`
}

// seedSourceArtifact writes a cached ghcr.io/windsorcli/<name>:<tag> artifact into the OCI cache of
// the project in dir. The artifact.tar holds files, and the cache directory holds them extracted, as
// after a pull.
func seedSourceArtifact(t *testing.T, dir, name, tag string, files map[string]string) {
	t.Helper()
	cacheDir := filepath.Join(dir, ".windsor", "cache", "oci", "ghcr.io_windsorcli_"+name+"_"+tag)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	f, err := os.Create(filepath.Join(cacheDir, "artifact.tar"))
	if err != nil {
		t.Fatalf("create artifact.tar: %v", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for entry, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: entry, Mode: 0644, Size: int64(len(content))}); err != nil {
			t.Fatalf("write tar header %s: %v", entry, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar entry %s: %v", entry, err)
		}
		path := filepath.Join(cacheDir, filepath.FromSlash(entry))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create dir for %s: %v", entry, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", entry, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close artifact.tar: %v", err)
	}
}

// writeProjectFile writes content to the path rel under dir, and creates the parent directories.
func writeProjectFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create dir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// readBundleEntries opens a gzipped tar bundle and returns the content of each entry by name.
func readBundleEntries(t *testing.T, bundlePath string) map[string][]byte {
	t.Helper()
	f, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	entries := make(map[string][]byte)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read entry %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = data
	}
}

// decodeBOM decodes data as a CycloneDX BOM and fails the test on an error.
func decodeBOM(t *testing.T, data []byte) extractedBOM {
	t.Helper()
	var bom extractedBOM
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatalf("unmarshal BOM: %v\ncontent: %s", err, data)
	}
	return bom
}

// readBOMFromBundle returns the artifact-manifest.cdx.json of a bundle. It fails the test when the
// bundle has no BOM.
func readBOMFromBundle(t *testing.T, bundlePath string) extractedBOM {
	t.Helper()
	data, ok := readBundleEntries(t, bundlePath)["artifact-manifest.cdx.json"]
	if !ok {
		t.Fatal("artifact-manifest.cdx.json not present in bundle")
	}
	return decodeBOM(t, data)
}

// purls returns the sorted package URLs of the components of bom.
func purls(bom extractedBOM) []string {
	out := make([]string, 0, len(bom.Components))
	for _, c := range bom.Components {
		out = append(out, c.Purl)
	}
	sort.Strings(out)
	return out
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestBundle_WritesTheCycloneDXBOMOfTheArtifactTags(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "bundle")

	outPath := filepath.Join(dir, "out.tar.gz")
	_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "bundle-it:v1.0.0", "-o", outPath}, env)
	if err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}

	if _, ok := readBundleEntries(t, outPath)["artifact-manifest.yaml"]; ok {
		t.Error("expected no artifact-manifest.yaml in the bundle")
	}
	bom := readBOMFromBundle(t, outPath)
	if bom.SpecVersion != "1.6" {
		t.Errorf("expected CycloneDX 1.6, got %q", bom.SpecVersion)
	}
	want := []string{
		"pkg:generic/cert-manager@1.20.1?download_url=https:%2F%2Fcharts.jetstack.io",
		"pkg:oci/cert-manager-controller@sha256:9f9556b4b131554694c67c8229d231b1f7d69b882b5f061a56bafa465f3b22fc?repository_url=quay.io%2Fjetstack%2Fcert-manager-controller&tag=v1.20.1",
		"pkg:oci/installer?repository_url=ghcr.io%2Fsiderolabs%2Finstaller&tag=v1.11.5",
	}
	if got := purls(bom); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("expected package URLs:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
	for _, c := range bom.Components {
		if c.Name == "quay.io/jetstack/cert-manager-controller" && (len(c.Hashes) != 1 || c.Hashes[0].Alg != "SHA-256") {
			t.Errorf("expected a SHA-256 hash for the digest, got %+v", c.Hashes)
		}
	}
}

func TestBundle_WritesAnEmptyBOMWhenNoArtifactTagsArePresent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	outPath := filepath.Join(dir, "out.tar.gz")
	_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "plain:v0.0.1", "-o", outPath}, env)
	if err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}

	if bom := readBOMFromBundle(t, outPath); len(bom.Components) != 0 {
		t.Errorf("expected no components, got %d", len(bom.Components))
	}
}

func TestBundle_WritesTheSameArtifactsAsShowArtifactsAll(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	if err := os.Remove(filepath.Join(dir, "contexts", "local", "terraform", "comp.tfvars")); err != nil {
		t.Fatalf("remove the context values file, which a bundle does not hold: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "--all", "-o", "cyclonedx"}, env)
	if err != nil {
		t.Fatalf("show artifacts --all -o cyclonedx: %v\nstderr: %s", err, stderr)
	}
	shown := decodeBOM(t, stdout)

	outPath := filepath.Join(dir, "out.tar.gz")
	if _, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "show-artifacts:v1.0.0", "-o", outPath}, env); err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}
	bundled := readBOMFromBundle(t, outPath)

	if len(shown.Components) == 0 {
		t.Fatal("expected components from show artifacts")
	}
	if got, want := purls(bundled), purls(shown); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("expected the bundle BOM to match show artifacts --all.\nbundle:\n%s\nshow:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBundle_ListsTheArtifactsOfEachOCISource(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "bundle")
	writeProjectFile(t, dir, "contexts/_template/blueprint.yaml", "kind: Blueprint\napiVersion: blueprints.windsorcli.dev/v1alpha1\nmetadata:\n  name: bundle-it\nsources:\n  - name: core\n    url: oci://ghcr.io/windsorcli/core:v0.1.0\n")
	coreBOM := `{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":[` +
		`{"bom-ref":"pkg:oci/etcd?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2","type":"container","name":"quay.io/coreos/etcd","version":"v3.7.2","purl":"pkg:oci/etcd?repository_url=quay.io%2Fcoreos%2Fetcd&tag=v3.7.2"}]}`
	seedSourceArtifact(t, dir, "core", "v0.1.0", map[string]string{
		"metadata.yaml":              "name: core\nversion: v0.1.0\n",
		"artifact-manifest.cdx.json": coreBOM,
	})

	outPath := filepath.Join(dir, "out.tar.gz")
	if _, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "bundle-it:v1.0.0", "-o", outPath}, env); err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}

	bom := readBOMFromBundle(t, outPath)
	got := make(map[string]extractedComponent)
	for _, c := range bom.Components {
		if len(c.Components) != 0 {
			t.Errorf("expected no nested components, got %+v", c)
		}
		got[c.BOMRef] = c
	}
	if core, ok := got[coreSourcePurl]; !ok || core.Type != "data" || core.Version != "v0.1.0" {
		t.Errorf("expected a data component for core v0.1.0, got %+v", bom.Components)
	}
	if _, ok := got[etcdImagePurl]; !ok {
		t.Errorf("expected the etcd image of the core BOM, got %+v", bom.Components)
	}
	if deps := bom.dependsOn("bundle-it"); !slices.Contains(deps, coreSourcePurl) {
		t.Errorf("expected the bundle to pull core, got %v", deps)
	}
	if deps := bom.dependsOn(coreSourcePurl); strings.Join(deps, " ") != etcdImagePurl {
		t.Errorf("expected core to pull etcd, got %v", deps)
	}
}

func TestBundle_FailsWhenASourceCannotBePulled(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "bundle")
	writeProjectFile(t, dir, "contexts/_template/blueprint.yaml", "kind: Blueprint\napiVersion: blueprints.windsorcli.dev/v1alpha1\nmetadata:\n  name: bundle-it\nsources:\n  - name: core\n    url: oci://127.0.0.1:1/windsorcli/core:v0.1.0\n")

	outPath := filepath.Join(dir, "out.tar.gz")
	_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "bundle-it:v1.0.0", "-o", outPath}, env)
	if err == nil {
		t.Fatal("expected bundle to fail when a source cannot be pulled")
	}
	if !strings.Contains(string(stderr), `source "core"`) {
		t.Errorf("expected stderr to name the source, got: %s", stderr)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("expected no bundle file")
	}
}

// Error path — bundling without a tag or metadata.yaml declaring name/version
// fails with a clear message rather than producing an untagged archive.
func TestBundle_FailsWithoutTagOrMetadata(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "bundle")

	_, stderr, err := helpers.RunCLI(dir, []string{"bundle"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "name is required") {
		t.Errorf("expected stderr to contain 'name is required', got: %s", stderr)
	}
}
