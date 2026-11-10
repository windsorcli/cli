package artifact_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Test Setup
// =============================================================================

// imageTag is a file body with one artifact tag for the image ghcr.io/example/<name>.
func imageTag(name string) string {
	return "# artifact: type=image name=ghcr.io/example/" + name + "\nimage: ghcr.io/example/" + name + ":v1\n"
}

// terraformTag is a Terraform file body with one artifact tag for the image ghcr.io/example/<name>.
func terraformTag(name string) string {
	return "# artifact: type=image name=ghcr.io/example/" + name + "\nlocals {\n  image = \"ghcr.io/example/" + name + ":v1\"\n}\n"
}

// tagFiles returns the file of each tag, in order.
func tagFiles(tags []artifact.ArtifactTag) []string {
	files := make([]string, 0, len(tags))
	for _, tag := range tags {
		files = append(files, tag.File)
	}
	return files
}

// writeTree writes each file of files, keyed by its slash path relative to root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// scanFiles returns the sorted files of the tags of a scoped scan, and the joined notes.
func scanFiles(t *testing.T, files map[string]string, scope artifact.TagScope) ([]string, string) {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, files)
	tags, notes, err := artifact.ScanTagsScoped(root, scope)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	got := []string{}
	for _, tag := range tags {
		got = append(got, tag.File)
	}
	return got, strings.Join(notes, "\n")
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestScanTagsScoped(t *testing.T) {
	t.Run("ReadsSelectedComponentsAndSkipsTheOthers", func(t *testing.T) {
		// Given a kustomization with one selected component and one other component
		files := map[string]string{
			"kustomize/telemetry/install/kustomization.yaml":           "resources: []\n",
			"kustomize/telemetry/install/base.yaml":                    imageTag("base"),
			"kustomize/telemetry/install/fluentbit/kustomization.yaml": "kind: Component\n",
			"kustomize/telemetry/install/fluentbit/release.yaml":       imageTag("fluentbit"),
			"kustomize/telemetry/install/filebeat/kustomization.yaml":  "kind: Component\n",
			"kustomize/telemetry/install/filebeat/release.yaml":        imageTag("filebeat"),
		}
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "telemetry", Dir: "kustomize/telemetry/install", Components: []string{"fluentbit"}}}}

		// When the scope is scanned
		got, notes := scanFiles(t, files, scope)

		// Then only the base and the selected component are read
		want := []string{"kustomize/telemetry/install/base.yaml", "kustomize/telemetry/install/fluentbit/release.yaml"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if notes != "" {
			t.Errorf("expected no notes, got %q", notes)
		}
	})

	t.Run("FollowsLocalResourcesAndComponentsAlsoAboveTheDirectory", func(t *testing.T) {
		// Given a kustomization that names a sibling, a nested component, a single file and a remote base
		files := map[string]string{
			"kustomize/lb/layer2/kustomization.yaml":        "resources:\n  - ../arp\n  - ../shared/one.yaml\n  - https://example.com/remote.yaml\n  - github.com/org/repo?ref=v1\ncomponents:\n  - nested\n",
			"kustomize/lb/layer2/nested/kustomization.yaml": "resources:\n  - extra.yaml\n",
			"kustomize/lb/layer2/nested/extra.yaml":         imageTag("nested"),
			"kustomize/lb/arp/kustomization.yaml":           "resources:\n  - pool.yaml\n",
			"kustomize/lb/arp/pool.yaml":                    imageTag("arp"),
			"kustomize/lb/shared/one.yaml":                  imageTag("one"),
			"kustomize/lb/shared/two.yaml":                  imageTag("two"),
		}
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "lb", Dir: "kustomize/lb/layer2"}}}

		// When the scope is scanned
		got, notes := scanFiles(t, files, scope)

		// Then the reached files are read, and a remote entry gives no note
		want := []string{"kustomize/lb/arp/pool.yaml", "kustomize/lb/layer2/nested/extra.yaml", "kustomize/lb/shared/one.yaml"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if notes != "" {
			t.Errorf("expected no notes, got %q", notes)
		}
	})

	t.Run("ReadsPatchFilesInAReachedDirectory", func(t *testing.T) {
		// Given a component with a patches directory that has no kustomization file
		files := map[string]string{
			"kustomize/cni/hubble/kustomization.yaml":        "kind: Component\n",
			"kustomize/cni/hubble/patches/helm-release.yaml": imageTag("hubble"),
		}
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "cni", Dir: "kustomize/cni", Components: []string{"hubble"}}}}

		// When the scope is scanned
		got, _ := scanFiles(t, files, scope)

		// Then the patch file is read
		if want := []string{"kustomize/cni/hubble/patches/helm-release.yaml"}; !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("AlwaysReadsContextsTemplateAndTheCRDCatalog", func(t *testing.T) {
		// Given tags in the always-read directories and in a kustomize directory that no entry uses
		files := map[string]string{
			"contexts/_template/facets/f.yaml": imageTag("facet"),
			"_template/facets/g.yaml":          imageTag("bundled"),
			"kustomize/crds/sources.yaml":      imageTag("catalog"),
			"kustomize/unused/release.yaml":    imageTag("unused"),
			"terraform/unused/main.tf":         terraformTag("unused-tf"),
		}

		// When an empty scope is scanned
		got, _ := scanFiles(t, files, artifact.TagScope{})

		// Then only the always-read directories are read
		want := []string{"_template/facets/g.yaml", "contexts/_template/facets/f.yaml", "kustomize/crds/sources.yaml"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("WidensTheScanForAComponentNameWithAnExpression", func(t *testing.T) {
		// Given a kustomization whose component name is an expression, and a component with a sibling base
		files := map[string]string{
			"kustomize/app/kustomization.yaml":    "resources: []\n",
			"kustomize/app/a/kustomization.yaml":  "resources:\n  - ../../shared\n",
			"kustomize/app/a/release.yaml":        imageTag("a"),
			"kustomize/shared/kustomization.yaml": "resources:\n  - pool.yaml\n",
			"kustomize/shared/pool.yaml":          imageTag("shared"),
			"kustomize/other/kustomization.yaml":  "resources: []\n",
			"kustomize/other/release.yaml":        imageTag("other"),
		}
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app", Components: []string{"${terraform_output('x')}"}}}}

		// When the scope is scanned
		got, notes := scanFiles(t, files, scope)

		// Then each directory under the kustomization and its entries are read, and a note names the component
		want := []string{"kustomize/app/a/release.yaml", "kustomize/shared/pool.yaml"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if !strings.Contains(notes, `"${terraform_output('x')}"`) {
			t.Errorf("expected a note that names the component, got %q", notes)
		}
	})

	t.Run("WidensTheScanForAPathWithAnExpression", func(t *testing.T) {
		// Given entries whose paths are expressions
		files := map[string]string{
			"kustomize/a/release.yaml": imageTag("a"),
			"terraform/b/main.tf":      terraformTag("b"),
			"terraform/b/c/nested.tf":  terraformTag("c"),
		}
		scope := artifact.TagScope{
			Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/${x}"}},
			Terraform:      []artifact.TerraformScope{{Name: "net", Root: "terraform", Path: "${y}"}},
		}

		// When the scope is scanned
		got, notes := scanFiles(t, files, scope)

		// Then the whole directories are read, and a note names each path
		want := []string{"kustomize/a/release.yaml", "terraform/b/c/nested.tf", "terraform/b/main.tf"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if !strings.Contains(notes, `"kustomize/${x}"`) || !strings.Contains(notes, `"${y}"`) {
			t.Errorf("expected a note for each path, got %q", notes)
		}
	})

	t.Run("NotesANameThatTheSourceDoesNotHold", func(t *testing.T) {
		// Given entries that name missing paths, and an entry that leaves the source
		files := map[string]string{
			"kustomize/app/kustomization.yaml": "resources:\n  - ../../../outside\n  - missing.yaml\n",
			"terraform/net/main.tf":            "module \"m\" {\n  source = \"./absent\"\n}\n",
		}
		scope := artifact.TagScope{
			Kustomizations: []artifact.KustomizationScope{
				{Name: "app", Dir: "kustomize/app", Components: []string{"gone"}},
				{Name: "lost", Dir: "kustomize/lost"},
			},
			Terraform: []artifact.TerraformScope{{Name: "net", Root: "terraform", Path: "net"}, {Name: "void", Root: "terraform", Path: "void"}},
		}

		// When the scope is scanned
		_, notes := scanFiles(t, files, scope)

		// Then a note names each one
		for _, want := range []string{`"../../../outside"`, `"missing.yaml"`, `"gone"`, `"kustomize/lost"`, `"./absent"`, `"void"`} {
			if !strings.Contains(notes, want) {
				t.Errorf("expected a note that names %s, got %q", want, notes)
			}
		}
	})

	t.Run("NotesAKustomizationFileItCannotParse", func(t *testing.T) {
		// Given a kustomization file that is not valid YAML
		files := map[string]string{"kustomize/app/kustomization.yaml": "resources: [\n"}
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app"}}}

		// When the scope is scanned
		_, notes := scanFiles(t, files, scope)

		// Then a note names the file
		if !strings.Contains(notes, "kustomize/app/kustomization.yaml") {
			t.Errorf("expected a note that names the file, got %q", notes)
		}
	})

	t.Run("ReadsAllOfADirectoryWithNoKustomizationFile", func(t *testing.T) {
		// Given a directory with no kustomization file and a subdirectory that has one
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"kustomize/app/x.yaml":                 imageTag("x"),
			"kustomize/app/sub/kustomization.yaml": "resources:\n  - y.yaml\n",
			"kustomize/app/sub/y.yaml":             imageTag("y"),
		})
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app"}}}

		// When the scan runs
		tags, notes, err := artifact.ScanTagsScoped(root, scope)

		// Then the scan reads both files
		if err != nil || len(notes) != 0 {
			t.Fatalf("expected no error and no notes, got %v and %q", err, notes)
		}
		want := []string{"kustomize/app/sub/y.yaml", "kustomize/app/x.yaml"}
		if got := tagFiles(tags); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("FollowsEachFieldThatNamesAFile", func(t *testing.T) {
		// Given a kustomization file that names other files with fields other than resources
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"kustomize/app/kustomization.yaml":  "bases:\n  - ../base\npatches:\n  - path: ../shared/p.yaml\n  - patch: |\n      kind: X\npatchesStrategicMerge:\n  - ../shared/s.yaml\n  - |-\n    kind: Inline\npatchesJson6902:\n  - path: ../shared/j.yaml\n",
			"kustomize/base/kustomization.yaml": "resources: []\n",
			"kustomize/base/b.yaml":             imageTag("b"),
			"kustomize/shared/p.yaml":           imageTag("p"),
			"kustomize/shared/s.yaml":           imageTag("s"),
			"kustomize/shared/j.yaml":           imageTag("j"),
			"kustomize/shared/unused.yaml":      imageTag("unused"),
		})
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app"}}}

		// When the scan runs
		tags, notes, err := artifact.ScanTagsScoped(root, scope)

		// Then the scan reads each named file and no other file
		if err != nil || len(notes) != 0 {
			t.Fatalf("expected no error and no notes, got %v and %q", err, notes)
		}
		want := []string{"kustomize/base/b.yaml", "kustomize/shared/j.yaml", "kustomize/shared/p.yaml", "kustomize/shared/s.yaml"}
		if got := tagFiles(tags); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("LeavesTheFileContentOutOfAParseNote", func(t *testing.T) {
		// Given a kustomization file that does not parse and that holds a secret
		root := t.TempDir()
		writeTree(t, root, map[string]string{"kustomize/app/kustomization.yaml": "secretGenerator:\n  - literals:\n      - password=hunter2\nresources: foo\n"})
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app"}}}

		// When the scan runs
		_, notes, err := artifact.ScanTagsScoped(root, scope)

		// Then the note names the file and does not hold the secret
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if joined := strings.Join(notes, "\n"); !strings.Contains(joined, "kustomization.yaml") || strings.Contains(joined, "hunter2") {
			t.Errorf("expected a note without the secret, got %q", joined)
		}
	})

	t.Run("CleansTheTerraformRootWhenThePathIsAnExpression", func(t *testing.T) {
		// Given a Terraform root with a trailing slash and a path with an expression
		root := t.TempDir()
		writeTree(t, root, map[string]string{"terraform/a/main.tf": terraformTag("a")})
		scope := artifact.TagScope{Terraform: []artifact.TerraformScope{{Name: "a", Root: "terraform/", Path: "${x}"}}}

		// When the scan runs
		tags, notes, err := artifact.ScanTagsScoped(root, scope)

		// Then the scan widens to the root and gives a note
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 1 || len(notes) != 1 {
			t.Errorf("expected one tag and one note, got %v and %q", tags, notes)
		}
	})

	t.Run("ReturnsAnErrorForADirectoryThatItCannotRead", func(t *testing.T) {
		// Given a parent directory that the process cannot search
		if runtime.GOOS == "windows" || os.Getuid() == 0 {
			t.Skip("file modes do not block this process")
		}
		root := t.TempDir()
		writeTree(t, root, map[string]string{"terraform/a/main.tf": terraformTag("a")})
		parent := filepath.Join(root, "terraform")
		if err := os.Chmod(parent, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o750) })
		scope := artifact.TagScope{Terraform: []artifact.TerraformScope{{Name: "a", Root: "terraform", Path: "a"}}}

		// When the scan runs
		_, _, err := artifact.ScanTagsScoped(root, scope)

		// Then the scan returns the error
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("ReadsATerraformComponentAndItsLocalModules", func(t *testing.T) {
		// Given a component with a local module, a shared module, a remote module and a nested component
		files := map[string]string{
			"terraform/cluster/talos/main.tf":                 "module \"machine\" {\n  source = \"./modules/machine\"\n}\nmodule \"shared\" {\n  source = \"../../shared\"\n}\nmodule \"remote\" {\n  source = \"hashicorp/consul/aws\"\n}\n",
			"terraform/cluster/talos/variables.tf":            terraformTag("talos"),
			"terraform/cluster/talos/templates/values.yaml":   imageTag("template"),
			"terraform/cluster/talos/modules/machine/main.tf": terraformTag("machine"),
			"terraform/cluster/talos/modules/unused/main.tf":  terraformTag("unused"),
			"terraform/cluster/talos/extensions/main.tf":      terraformTag("extensions"),
			"terraform/shared/main.tf":                        terraformTag("shared"),
			"terraform/other/main.tf":                         terraformTag("other"),
		}
		scope := artifact.TagScope{Terraform: []artifact.TerraformScope{{Name: "talos", Root: "terraform", Path: "cluster/talos"}}}

		// When the scope is scanned
		got, notes := scanFiles(t, files, scope)

		// Then the component and its local modules are read
		want := []string{
			"terraform/cluster/talos/modules/machine/main.tf",
			"terraform/cluster/talos/templates/values.yaml",
			"terraform/cluster/talos/variables.tf",
			"terraform/shared/main.tf",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if notes != "" {
			t.Errorf("expected no notes, got %q", notes)
		}
	})

	t.Run("NotesATerraformFileItCannotParse", func(t *testing.T) {
		// Given a .tf file that is not valid HCL
		files := map[string]string{"terraform/net/main.tf": "module \"m\" {\n"}
		scope := artifact.TagScope{Terraform: []artifact.TerraformScope{{Name: "net", Root: "terraform", Path: "net"}}}

		// When the scope is scanned
		_, notes := scanFiles(t, files, scope)

		// Then a note names the file
		if !strings.Contains(notes, "terraform/net/main.tf") {
			t.Errorf("expected a note that names the file, got %q", notes)
		}
	})

	t.Run("ReturnsErrorForAMalformedTagInAScopedFile", func(t *testing.T) {
		// Given a used file with a tag that has no value line
		root := t.TempDir()
		writeTree(t, root, map[string]string{"kustomize/app/values.yaml": "# artifact: type=image name=ghcr.io/example/a\n"})
		scope := artifact.TagScope{Kustomizations: []artifact.KustomizationScope{{Name: "app", Dir: "kustomize/app"}}}

		// When the scope is scanned
		_, _, err := artifact.ScanTagsScoped(root, scope)

		// Then the error names the file
		if err == nil || !strings.Contains(err.Error(), "kustomize/app/values.yaml") {
			t.Fatalf("expected an error that names the file, got %v", err)
		}
	})
}
