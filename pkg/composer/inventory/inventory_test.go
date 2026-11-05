package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/inventory"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

type mocks struct {
	runtime         *runtime.Runtime
	configHandler   *config.MockConfigHandler
	artifactBuilder *artifact.MockArtifact
}

func setupMocks(t *testing.T) *mocks {
	t.Helper()
	m := &mocks{}

	m.configHandler = config.NewMockConfigHandler()
	m.configHandler.GetFunc = func(key string) any { return nil }

	m.artifactBuilder = artifact.NewMockArtifact()
	m.artifactBuilder.ParseOCIRefFunc = func(ociRef string) (string, string, string, error) {
		ref := strings.TrimPrefix(ociRef, "oci://")
		repository, tag, _ := strings.Cut(ref, ":")
		registry, repository, _ := strings.Cut(repository, "/")
		return registry, repository, tag, nil
	}
	m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
		return filepath.Join(os.TempDir(), "windsor-inventory-missing", registry, repository, tag), nil
	}

	m.runtime = &runtime.Runtime{
		ContextName:   "local",
		ProjectRoot:   "/project",
		ConfigHandler: m.configHandler,
	}
	return m
}

// writeFile writes content to the path rel under dir, and creates the parent directories.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewLister(t *testing.T) {
	t.Run("PanicsWithoutRuntime", func(t *testing.T) {
		// Given no runtime
		defer func() {
			// Then NewLister panics
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()

		// When NewLister is called
		inventory.NewLister(nil, artifact.NewMockArtifact())
	})

	t.Run("PanicsWithoutArtifactBuilder", func(t *testing.T) {
		// Given no artifact builder
		m := setupMocks(t)
		defer func() {
			// Then NewLister panics
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()

		// When NewLister is called
		inventory.NewLister(m.runtime, nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseLister_List(t *testing.T) {
	t.Run("ListsEachOCIBlueprintSource", func(t *testing.T) {
		// Given a blueprint with two oci:// sources, a duplicate and a Git source
		m := setupMocks(t)
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "extra", Url: "oci://ghcr.io/windsorcli/extra:v1.0.0@sha256:abc"},
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
				{Name: "core-again", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
				{Name: "git", Url: "https://github.com/windsorcli/core"},
			},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{ChartsTo: "registry.example.com/windsor"})

		// Then the inventory holds one sorted entry for each oci:// source, with no destination
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Inventory.Version != inventory.InventoryVersion || result.Inventory.Context != "local" {
			t.Errorf("unexpected inventory header %+v", result.Inventory)
		}
		got := result.Inventory.Artifacts
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %+v", got)
		}
		want := []inventory.Entry{
			{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/core", Version: "v0.9.0", Source: inventory.Source{Object: "source/core"}},
			{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/extra", Version: "v1.0.0", Digest: "sha256:abc", Source: inventory.Source{Object: "source/extra"}},
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d: expected %+v, got %+v", i, want[i], got[i])
			}
		}
	})

	t.Run("ListsTheArtifactTagsOfTheProjectAndEachSource", func(t *testing.T) {
		// Given a project and a cached core source that carry artifact tags
		m := setupMocks(t)
		project, core := t.TempDir(), t.TempDir()
		writeFile(t, project, "kustomize/app/values.yaml", "# artifact: type=image name=ghcr.io/example/worker\nimage: ghcr.io/example/worker:1.2.3\n")
		writeFile(t, core, "kustomize/cert-manager/release.yaml", "# artifact: type=chart name=cert-manager repo=https://charts.jetstack.io\nversion: v1.17.0\n")
		writeFile(t, core, "terraform/cilium/main.tf", "# artifact: type=chart name=cilium repo=oci://quay.io/cilium/charts\ndefault = \"1.17.2\"\n")
		m.runtime.ProjectRoot = project
		m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) { return core, nil }
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "app"},
			Sources:  []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}},
		}

		// When List is called with a chart registry
		result, err := lister.List(blueprint, inventory.Options{ChartsTo: "registry.example.com/windsor"})

		// Then each tag is an entry, and each chart has a destination in the chart layout
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		got := make(map[string]string)
		for _, e := range result.Inventory.Artifacts {
			got[string(e.Kind)+" "+e.Reference+" "+e.Version] = e.Destination
		}
		want := map[string]string{
			"blueprint ghcr.io/windsorcli/core v0.9.0":  "",
			"image ghcr.io/example/worker 1.2.3":        "",
			"chart https://charts.jetstack.io v1.17.0":  "registry.example.com/windsor/cert-manager:v1.17.0",
			"chart quay.io/cilium/charts/cilium 1.17.2": "registry.example.com/windsor/cilium:1.17.2",
		}
		if len(got) != len(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
		for key, dest := range want {
			if d, ok := got[key]; !ok || d != dest {
				t.Errorf("expected %s with destination %q, got %v", key, dest, got)
			}
		}
	})

	t.Run("NotesASourceWhoseTagsItCannotRead", func(t *testing.T) {
		// Given a Git source and an oci:// source that is not in the cache
		m := setupMocks(t)
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "git", Url: "https://github.com/windsorcli/core"},
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
			},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then a note names each source
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		notes := strings.Join(result.Notes, "\n")
		if !strings.Contains(notes, `Source "git" is not an oci:// source`) || !strings.Contains(notes, `Source "core" is not in the cache`) {
			t.Errorf("expected a note for each source, got %q", notes)
		}
	})

	t.Run("ReturnsErrorForAMalformedTag", func(t *testing.T) {
		// Given a project file with a tag that has no version line
		m := setupMocks(t)
		project := t.TempDir()
		writeFile(t, project, "kustomize/app/values.yaml", "# artifact: type=image name=ghcr.io/example/worker\n")
		m.runtime.ProjectRoot = project
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)

		// When List is called
		_, err := lister.List(&blueprintv1alpha1.Blueprint{}, inventory.Options{})

		// Then the error names the file
		if err == nil || !strings.Contains(err.Error(), "kustomize/app/values.yaml") {
			t.Fatalf("expected an error that names the file, got %v", err)
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a lister
		m := setupMocks(t)
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)

		// When List is called with no blueprint
		_, err := lister.List(nil, inventory.Options{})

		// Then an error is returned
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
