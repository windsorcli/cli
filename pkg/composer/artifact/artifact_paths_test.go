package artifact

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/paths"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// writeTree creates each file of tree, a map of slash path to content, under root.
func writeTree(t *testing.T, root string, tree map[string]string) {
	t.Helper()
	for rel, content := range tree {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// bundledNames bundles the project at root with block and returns the sorted archive paths.
func bundledNames(t *testing.T, root string, block *v1alpha2.Paths) []string {
	t.Helper()
	resolver, err := paths.New(root, block, nil)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewArtifactBuilder(&runtime.Runtime{Shell: shell.NewMockShell(), ProjectRoot: root, Paths: resolver})
	if err := builder.Bundle(); err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	var names []string
	for name := range builder.files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestArtifactBuilder_Bundle_ProjectPaths(t *testing.T) {
	want := []string{"_template/blueprint.yaml", "kustomize/app/kustomization.yaml", "terraform/vpc/main.tf"}

	t.Run("ReadsFromBaseAndWritesTheArchiveLayout", func(t *testing.T) {
		// Given a project whose sources sit under infra/
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"infra/contexts/_template/blueprint.yaml": "kind: Blueprint\n",
			"infra/kustomize/app/kustomization.yaml":  "resources: []\n",
			"infra/terraform/vpc/main.tf":             "terraform {}\n",
			"terraform/stale/main.tf":                 "terraform {}\n",
		})

		// When the project bundles with paths.base set
		got := bundledNames(t, root, &v1alpha2.Paths{Base: "infra"})

		// Then the archive holds the standard layout and nothing from the default directories
		assertNames(t, got, want)
	})

	t.Run("ReadsTheDefaultLayout", func(t *testing.T) {
		// Given a project with the default directories
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"contexts/_template/blueprint.yaml": "kind: Blueprint\n",
			"kustomize/app/kustomization.yaml":  "resources: []\n",
			"terraform/vpc/main.tf":             "terraform {}\n",
		})

		// When the project bundles with no paths block
		got := bundledNames(t, root, nil)

		// Then the archive holds the standard layout
		assertNames(t, got, want)
	})
}

func TestScanTagsDirIn(t *testing.T) {
	tag := "# artifact: type=image name=ghcr.io/example/app\nimage: ghcr.io/example/app:1.0.0\n"

	t.Run("ScansTheProjectLayout", func(t *testing.T) {
		// Given a project with moved directories and a tag in the moved kustomize directory
		root := t.TempDir()
		writeTree(t, root, map[string]string{"infra/kustomize/app/values.yaml": tag})
		resolver, err := paths.New(root, &v1alpha2.Paths{Base: "infra"}, nil)
		if err != nil {
			t.Fatal(err)
		}

		// When the project layout scans the root
		tags, err := ScanTagsDirIn(root, ProjectLayout(resolver))

		// Then the tag is found with a root-relative file path
		if err != nil || len(tags) != 1 || tags[0].File != "infra/kustomize/app/values.yaml" {
			t.Fatalf("got %v, %v", tags, err)
		}
	})

	t.Run("ArchiveLayoutIgnoresTheProjectDirectories", func(t *testing.T) {
		// Given the same tree
		root := t.TempDir()
		writeTree(t, root, map[string]string{"infra/kustomize/app/values.yaml": tag})

		// When the archive layout scans the root
		tags, err := ScanTagsDir(root)

		// Then it finds nothing, because a consumed source has the standard layout
		if err != nil || len(tags) != 0 {
			t.Fatalf("got %v, %v", tags, err)
		}
	})
}

func TestScanTagsScoped_ProjectLayout(t *testing.T) {
	t.Run("ReadsTheMovedKustomizationDirectory", func(t *testing.T) {
		// Given a project with a tagged kustomization in the moved kustomize directory
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"infra/kustomize/app/kustomization.yaml": "resources:\n  - values.yaml\n",
			"infra/kustomize/app/values.yaml":        "# artifact: type=image name=ghcr.io/example/app\nimage: ghcr.io/example/app:1.0.0\n",
		})
		resolver, err := paths.New(root, &v1alpha2.Paths{Base: "infra"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		scope := TagScope{
			Layout:         ProjectLayout(resolver),
			Kustomizations: []KustomizationScope{{Name: "app", Dir: resolver.RebaseKustomize("kustomize/app")}},
		}

		// When the scoped scan runs
		tags, _, err := ScanTagsScoped(root, scope)

		// Then it finds the tag
		if err != nil || len(tags) != 1 {
			t.Fatalf("got %v, %v", tags, err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// assertNames fails when got differs from want.
func assertNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
