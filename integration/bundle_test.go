//go:build integration
// +build integration

package integration

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Test Helpers
// =============================================================================

// extractedManifest is a lightweight mirror of the real ArtifactManifest shape
// used only for integration-side assertions. It avoids importing the internal
// pkg/composer/artifact package, keeping integration tests decoupled.
type extractedManifest struct {
	Version   string `yaml:"version"`
	Artifacts []struct {
		Type       string `yaml:"type"`
		Transport  string `yaml:"transport"`
		Reference  string `yaml:"reference"`
		Version    string `yaml:"version,omitempty"`
		Digest     string `yaml:"digest,omitempty"`
		Repository string `yaml:"repository,omitempty"`
		Source     struct {
			File       string `yaml:"file"`
			Datasource string `yaml:"datasource"`
		} `yaml:"source"`
	} `yaml:"artifacts"`
}

// readManifestFromBundle opens a gzipped tar bundle and returns the parsed
// artifact-manifest.yaml contents. It fails the test when the manifest entry
// is missing or malformed, which are both correctness regressions.
func readManifestFromBundle(t *testing.T, bundlePath string) extractedManifest {
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

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if hdr.Name != "artifact-manifest.yaml" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read manifest entry: %v", err)
		}
		var m extractedManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal manifest: %v\ncontent: %s", err, data)
		}
		return m
	}
	t.Fatal("artifact-manifest.yaml not present in bundle")
	return extractedManifest{}
}

// =============================================================================
// Integration Tests
// =============================================================================

// Success path — bundle a fixture whose kustomize and terraform sources carry
// Renovate annotations and confirm that artifact-manifest.yaml is embedded in
// the tar.gz with the expected entries, digests, and transports.
func TestBundle_EmitsArtifactManifestWithRenovatePins(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "bundle")

	outPath := filepath.Join(dir, "out.tar.gz")
	_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "bundle-it:v1.0.0", "-o", outPath}, env)
	if err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}

	manifest := readManifestFromBundle(t, outPath)

	if manifest.Version == "" {
		t.Errorf("expected non-empty manifest version")
	}
	if len(manifest.Artifacts) < 3 {
		t.Fatalf("expected at least 3 entries (helm + docker + github-release), got %d", len(manifest.Artifacts))
	}

	var sawHelm, sawDocker, sawGitHub bool
	for _, a := range manifest.Artifacts {
		switch a.Type {
		case "helm":
			if a.Reference == "cert-manager" && a.Version == "1.20.1" && a.Transport == "oci" {
				sawHelm = true
			}
		case "docker":
			if strings.Contains(a.Reference, "cert-manager-controller") &&
				a.Digest == "sha256:9f9556b4b131554694c67c8229d231b1f7d69b882b5f061a56bafa465f3b22fc" &&
				a.Transport == "oci" {
				sawDocker = true
			}
		case "github-release":
			if a.Reference == "fluxcd/flux2" && a.Version == "v2.3.0" && a.Transport == "tarball" {
				sawGitHub = true
			}
		}
	}
	if !sawHelm {
		t.Errorf("missing expected helm entry for cert-manager@1.20.1")
	}
	if !sawDocker {
		t.Errorf("missing expected docker entry for cert-manager-controller with digest")
	}
	if !sawGitHub {
		t.Errorf("missing expected github-release entry for fluxcd/flux2")
	}
}

// Edge path — bundle runs successfully when there are no annotations to scan;
// the manifest is still emitted, but with an empty artifacts list.
func TestBundle_EmitsEmptyManifestWhenNoAnnotationsPresent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	outPath := filepath.Join(dir, "out.tar.gz")
	_, stderr, err := helpers.RunCLI(dir, []string{"bundle", "-t", "plain:v0.0.1", "-o", outPath}, env)
	if err != nil {
		t.Fatalf("bundle: %v\nstderr: %s", err, stderr)
	}

	manifest := readManifestFromBundle(t, outPath)
	if len(manifest.Artifacts) != 0 {
		t.Errorf("expected empty artifacts list, got %d entries", len(manifest.Artifacts))
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
