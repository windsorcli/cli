package inventory_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
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

// fakeTrace proves the value on unusedLine unused and gives note for noteLine.
type fakeTrace struct {
	unusedLine int
	noteLine   int
	note       string
}

// UnusedFacetValue implements inventory.FacetTrace.
func (f fakeTrace) UnusedFacetValue(_ string, line int) (bool, string) {
	if line == f.noteLine {
		return false, f.note
	}
	return line == f.unusedLine, ""
}

// terraformTag is a Terraform file body with one artifact tag for the image ghcr.io/example/<name>.
func terraformTag(name string) string {
	return "# artifact: type=image name=ghcr.io/example/" + name + "\nlocals {\n  image = \"ghcr.io/example/" + name + ":v1\"\n}\n"
}

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
		result, err := lister.List(blueprint, inventory.Options{All: true, ChartsTo: "registry.example.com/windsor"})

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
		result, err := lister.List(blueprint, inventory.Options{All: true, ChartsTo: "registry.example.com/windsor"})

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

	t.Run("KeepsNoReferrersWhenTwoTagsNameTheSameImage", func(t *testing.T) {
		cases := map[string][2]string{
			"FalseFirst": {" referrers=false", ""},
			"FalseLast":  {"", " referrers=false"},
		}
		for name, attributes := range cases {
			t.Run(name, func(t *testing.T) {
				// Given two tags for the same image, and only one has referrers=false
				m := setupMocks(t)
				project := t.TempDir()
				writeFile(t, project, "kustomize/a/values.yaml", "# artifact: type=image name=docker.elastic.co/beats/filebeat"+attributes[0]+"\nimage: docker.elastic.co/beats/filebeat:9.0.0\n")
				writeFile(t, project, "kustomize/b/values.yaml", "# artifact: type=image name=docker.elastic.co/beats/filebeat"+attributes[1]+"\nimage: docker.elastic.co/beats/filebeat:9.0.0\n")
				m.runtime.ProjectRoot = project
				lister := inventory.NewLister(m.runtime, m.artifactBuilder)
				blueprint := &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "app"}}

				// When List is called
				result, err := lister.List(blueprint, inventory.Options{All: true})

				// Then the one entry has NoReferrers
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if len(result.Inventory.Artifacts) != 1 || !result.Inventory.Artifacts[0].NoReferrers {
					t.Fatalf("expected one entry with NoReferrers, got %+v", result.Inventory.Artifacts)
				}
			})
		}
	})

	t.Run("ListsTheBlueprintsAndTagsOfNestedSources", func(t *testing.T) {
		// Given a composed blueprint whose sources hold a parent blueprint and a blueprint it declares
		m := setupMocks(t)
		parent, child := t.TempDir(), t.TempDir()
		writeFile(t, parent, "kustomize/base/values.yaml", "# artifact: type=image name=ghcr.io/example/parent\nimage: ghcr.io/example/parent:1.0.0\n")
		writeFile(t, parent, "_template/blueprint.yaml", "sources:\n- name: child\n  url: oci://ghcr.io/windsorcli/child:v2.0.0\n")
		writeFile(t, child, "_template/facets/tools.yaml", "# artifact: type=image name=ghcr.io/example/child\nimage: ghcr.io/example/child:2.0.0\n")
		dirs := map[string]string{"parent": parent, "child": child}
		m.artifactBuilder.ParseOCIRefFunc = func(ref string) (string, string, string, error) {
			name := strings.TrimPrefix(ref, "oci://ghcr.io/windsorcli/")
			name, tag, _ := strings.Cut(name, ":")
			return "ghcr.io", "windsorcli/" + name, tag, nil
		}
		m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return dirs[strings.TrimPrefix(repository, "windsorcli/")], nil
		}
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "parent", Url: "oci://ghcr.io/windsorcli/parent:v1.0.0"},
				{Name: "child", Url: "oci://ghcr.io/windsorcli/child:v2.0.0"},
			},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{All: true})

		// Then both blueprints are entries, and the tags of each cache directory are entries
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		got := make(map[string]bool)
		for _, e := range result.Inventory.Artifacts {
			got[string(e.Kind)+" "+e.Reference+" "+e.Version] = true
		}
		for _, key := range []string{
			"blueprint ghcr.io/windsorcli/parent v1.0.0",
			"blueprint ghcr.io/windsorcli/child v2.0.0",
			"image ghcr.io/example/parent 1.0.0",
			"image ghcr.io/example/child 2.0.0",
		} {
			if !got[key] {
				t.Errorf("expected %q in %v", key, got)
			}
		}

		// And the tree nests the child under the parent that names it, each with its own image
		if len(result.Tree.Sources) != 1 || result.Tree.Sources[0].Name != "parent" {
			t.Fatalf("expected parent at the top, got %+v", result.Tree.Sources)
		}
		parentTree := result.Tree.Sources[0]
		if len(parentTree.Artifacts) != 1 || parentTree.Artifacts[0].Reference != "ghcr.io/example/parent" {
			t.Errorf("expected the parent image under parent, got %+v", parentTree.Artifacts)
		}
		if len(parentTree.Sources) != 1 || parentTree.Sources[0].Name != "child" {
			t.Fatalf("expected child under parent, got %+v", parentTree.Sources)
		}
		childTree := parentTree.Sources[0]
		if childTree.Entry.Reference != "ghcr.io/windsorcli/child" || len(childTree.Artifacts) != 1 || childTree.Artifacts[0].Reference != "ghcr.io/example/child" {
			t.Errorf("expected the child image under child, got %+v", childTree)
		}
	})

	t.Run("PutsASourceLoopAtTheTop", func(t *testing.T) {
		// Given two cached sources that each name the other
		m := setupMocks(t)
		first, second := t.TempDir(), t.TempDir()
		writeFile(t, first, "_template/blueprint.yaml", "sources:\n- name: second\n  url: oci://ghcr.io/windsorcli/second:v1\n")
		writeFile(t, second, "_template/blueprint.yaml", "sources:\n- name: first\n  url: oci://ghcr.io/windsorcli/first:v1\n")
		dirs := map[string]string{"first": first, "second": second}
		m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return dirs[strings.TrimPrefix(repository, "windsorcli/")], nil
		}
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "first", Url: "oci://ghcr.io/windsorcli/first:v1"},
				{Name: "second", Url: "oci://ghcr.io/windsorcli/second:v1"},
			},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{All: true})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the first source is at the top, and the walk stops where the loop closes
		if len(result.Tree.Sources) != 1 || result.Tree.Sources[0].Name != "first" {
			t.Fatalf("expected first at the top, got %+v", result.Tree.Sources)
		}
		loop := result.Tree.Sources[0].Sources[0].Sources[0]
		if loop.Name != "first" || len(loop.Sources) != 0 {
			t.Errorf("expected the walk to stop at first, got %+v", loop)
		}
	})

	t.Run("GivesASourceTheDigestOfItsCachedPull", func(t *testing.T) {
		// Given a cached source with the digest of its pull, and a tag in the URL only
		m := setupMocks(t)
		dir := t.TempDir()
		digest := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
		writeFile(t, dir, "digest", digest)
		m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
			return dir, nil
		}
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{All: true})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the source entry has the tag and the digest
		core := result.Tree.Sources[0].Entry
		if core.Version != "v0.9.0" || core.Digest != digest {
			t.Errorf("expected core with its tag and digest, got %+v", core)
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
		result, err := lister.List(blueprint, inventory.Options{All: true})

		// Then a note names each source
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		notes := strings.Join(result.Notes, "\n")
		if !strings.Contains(notes, `Source "git" is not an oci:// source`) || !strings.Contains(notes, `Source "core" is not in the cache`) {
			t.Errorf("expected a note for each source, got %q", notes)
		}
	})

	t.Run("LeavesOutAFacetTagThatTheTraceProvesUnused", func(t *testing.T) {
		// Given a facet with two tags and a trace that proves one unused and cannot resolve the other
		m := setupMocks(t)
		project := t.TempDir()
		writeFile(t, project, "contexts/_template/facets/f.yaml", "# artifact: type=image name=ghcr.io/example/idle\nidle: v1\n# artifact: type=image name=ghcr.io/example/open\nopen: v1\n")
		m.runtime.ProjectRoot = project
		trace := fakeTrace{unusedLine: 2, noteLine: 4, note: "cannot resolve"}
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)

		// When the list is made with the trace
		result, err := lister.List(&blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "project"}}, inventory.Options{Trace: trace})

		// Then the unused tag is absent, the other tag stays, and the note appears
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := len(result.Inventory.Artifacts); got != 1 || result.Inventory.Artifacts[0].Reference != "ghcr.io/example/open" {
			t.Errorf("expected only the open image, got %+v", result.Inventory.Artifacts)
		}
		if len(result.Notes) != 1 || result.Notes[0] != "cannot resolve" {
			t.Errorf("expected one note, got %q", result.Notes)
		}
	})

	t.Run("IgnoresTheTraceWithAll", func(t *testing.T) {
		// Given a facet tag that the trace proves unused
		m := setupMocks(t)
		project := t.TempDir()
		writeFile(t, project, "contexts/_template/facets/f.yaml", "# artifact: type=image name=ghcr.io/example/idle\nidle: v1\n")
		m.runtime.ProjectRoot = project
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)

		// When the list is made for whole sources
		result, err := lister.List(&blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "project"}}, inventory.Options{All: true, Trace: fakeTrace{unusedLine: 2}})

		// Then the tag stays
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result.Inventory.Artifacts) != 1 {
			t.Errorf("expected the image, got %+v", result.Inventory.Artifacts)
		}
	})

	t.Run("ListsOnlyTheTagsOfTheFilesThatTheBlueprintUses", func(t *testing.T) {
		// Given a project and a cached source with used and unused files
		m := setupMocks(t)
		project, core := t.TempDir(), t.TempDir()
		tag := func(name string) string {
			return "# artifact: type=image name=ghcr.io/example/" + name + "\nimage: ghcr.io/example/" + name + ":v1\n"
		}
		writeFile(t, project, "kustomize/app/values.yaml", tag("app"))
		writeFile(t, project, "kustomize/unused/values.yaml", tag("project-unused"))
		writeFile(t, project, "contexts/_template/facets/f.yaml", tag("facet"))
		writeFile(t, core, "kustomize/telemetry/install/kustomization.yaml", "resources: []\n")
		writeFile(t, core, "kustomize/telemetry/install/fluentbit/kustomization.yaml", "kind: Component\n")
		writeFile(t, core, "kustomize/telemetry/install/fluentbit/release.yaml", tag("fluentbit"))
		writeFile(t, core, "kustomize/telemetry/install/filebeat/kustomization.yaml", "kind: Component\n")
		writeFile(t, core, "kustomize/telemetry/install/filebeat/release.yaml", tag("filebeat"))
		writeFile(t, core, "kustomize/plain/values.yaml", tag("plain"))
		writeFile(t, core, "kustomize/disabled/values.yaml", tag("disabled"))
		writeFile(t, core, "terraform/cni/cilium/variables.tf", terraformTag("cilium"))
		writeFile(t, core, "modules/net/main.tf", terraformTag("prefixed"))
		m.runtime.ProjectRoot = project
		m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) { return core, nil }
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		off := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "app"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
				{Name: "prefixed", Url: "oci://ghcr.io/windsorcli/core:v0.9.0", PathPrefix: "modules"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app"},
				{Name: "plain", Source: "core"},
				{Name: "disabled", Source: "core", Enabled: &blueprintv1alpha1.BoolExpression{Value: &off}},
			},
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "telemetry",
				Source:  "core",
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"fluentbit"}},
			}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cni/cilium", Source: "core"},
				{Path: "net", Source: "prefixed"},
			},
		}

		// When List is called without All
		result, err := lister.List(blueprint, inventory.Options{})

		// Then only the used files and the always-read directories give entries
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		got := make(map[string]bool)
		for _, e := range result.Inventory.Artifacts {
			if e.Kind == inventory.KindImage {
				got[strings.TrimPrefix(e.Reference, "ghcr.io/example/")] = true
			}
		}
		want := map[string]bool{"app": true, "facet": true, "fluentbit": true, "plain": true, "cilium": true, "prefixed": true}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if len(result.Notes) != 0 {
			t.Errorf("expected no notes, got %v", result.Notes)
		}
	})

	t.Run("ListsWholeSourcesWithAll", func(t *testing.T) {
		// Given a project with a kustomize directory that the blueprint does not use
		m := setupMocks(t)
		project := t.TempDir()
		writeFile(t, project, "kustomize/unused/values.yaml", "# artifact: type=image name=ghcr.io/example/unused\nimage: ghcr.io/example/unused:v1\n")
		m.runtime.ProjectRoot = project
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)

		// When List is called with All
		result, err := lister.List(&blueprintv1alpha1.Blueprint{}, inventory.Options{All: true})

		// Then the tag of the unused directory is an entry
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result.Inventory.Artifacts) != 1 || result.Inventory.Artifacts[0].Reference != "ghcr.io/example/unused" {
			t.Errorf("expected the unused image, got %v", result.Inventory.Artifacts)
		}
	})

	t.Run("NotesAnEntryWhoseSourceTheBlueprintDoesNotDefine", func(t *testing.T) {
		// Given entries that name undefined sources, and an entry whose declared source has no cache
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "app"},
			Sources:             []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "a", Source: "ghost"}, {Name: "b", Source: "core"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "net", Source: "phantom"}},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then a note names each undefined source, and the declared source has one note
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		notes := strings.Join(result.Notes, "\n")
		if !strings.Contains(notes, `Kustomization "a" names the source "ghost"`) || !strings.Contains(notes, `Terraform component "net" names the source "phantom"`) {
			t.Errorf("expected a note for each undefined source, got %q", notes)
		}
		if strings.Count(notes, `"core"`) != 1 {
			t.Errorf("expected one note for the source with no cache, got %q", notes)
		}
	})

	t.Run("PrefixesAScanNoteWithItsSource", func(t *testing.T) {
		// Given a kustomization whose path is not in the project
		m := setupMocks(t)
		m.runtime.ProjectRoot = t.TempDir()
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "app"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "lost"}},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{})

		// Then the note names the source and the path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if notes := strings.Join(result.Notes, "\n"); !strings.Contains(notes, `Source "app"`) || !strings.Contains(notes, `"kustomize/lost"`) {
			t.Errorf("expected a note that names the source and the path, got %q", notes)
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
		_, err := lister.List(&blueprintv1alpha1.Blueprint{}, inventory.Options{All: true})

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
		_, err := lister.List(nil, inventory.Options{All: true})

		// Then an error is returned
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

// =============================================================================
// Test Mirrors
// =============================================================================

func TestBaseLister_ImageMirrors(t *testing.T) {
	setup := func(t *testing.T, registries map[string]any, images ...string) *inventory.Result {
		t.Helper()
		result, err := listImages(t, registries, images...)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return result
	}
	destination := func(t *testing.T, result *inventory.Result, reference string) string {
		t.Helper()
		for _, e := range result.Inventory.Artifacts {
			if e.Reference == reference {
				return e.Destination
			}
		}
		t.Fatalf("expected an entry for %s, got %+v", reference, result.Inventory.Artifacts)
		return ""
	}

	t.Run("UsesTheEndpointWithoutItsScheme", func(t *testing.T) {
		for _, endpoint := range []string{"harbor.example.com:8443/windsor", "http://harbor.example.com:8443/windsor", "https://harbor.example.com:8443/windsor/"} {
			// Given an entry with an endpoint
			registries := map[string]any{"ghcr.io": map[string]any{"endpoint": endpoint}}

			// When the images are listed
			result := setup(t, registries, "ghcr.io/example/worker")

			// Then the destination is under the endpoint without its scheme
			if got := destination(t, result, "ghcr.io/example/worker"); got != "harbor.example.com:8443/windsor/example/worker:1.0.0" {
				t.Errorf("endpoint %s: got destination %q", endpoint, got)
			}
		}
	})

	t.Run("UsesTheLocalHostAsTheUpstream", func(t *testing.T) {
		// Given an entry whose images name a different host than its key
		registries := map[string]any{"registry-1.docker.io": map[string]any{"local": "docker.io", "endpoint": "harbor.example.com/docker"}}

		// When the images are listed
		result := setup(t, registries, "docker.io/library/nginx")

		// Then the image of the local host goes to the endpoint
		if got := destination(t, result, "docker.io/library/nginx"); got != "harbor.example.com/docker/library/nginx:1.0.0" {
			t.Errorf("got destination %q", got)
		}
	})

	t.Run("UsesTheCatchAllForAHostWithNoEntry", func(t *testing.T) {
		// Given a catch-all entry and an entry for one host
		registries := map[string]any{
			"*":       map[string]any{"endpoint": "harbor.example.com/all"},
			"ghcr.io": map[string]any{"endpoint": "harbor.example.com/ghcr"},
		}

		// When the images are listed
		result := setup(t, registries, "ghcr.io/example/worker", "quay.io/example/agent")

		// Then the entry of the host comes first, and the catch-all applies to the other host
		if got := destination(t, result, "ghcr.io/example/worker"); got != "harbor.example.com/ghcr/example/worker:1.0.0" {
			t.Errorf("got destination %q", got)
		}
		if got := destination(t, result, "quay.io/example/agent"); got != "harbor.example.com/all/example/agent:1.0.0" {
			t.Errorf("got destination %q", got)
		}
	})

	t.Run("DoesNotUseTheKeyAsAMirror", func(t *testing.T) {
		// Given an entry with no endpoint and no hostname
		registries := map[string]any{"ghcr.io": map[string]any{"remote": "https://ghcr.io"}}

		// When the images are listed
		result := setup(t, registries, "ghcr.io/example/worker")

		// Then the image has no destination
		if got := destination(t, result, "ghcr.io/example/worker"); got != "" {
			t.Errorf("expected no destination, got %q", got)
		}
	})

	t.Run("GivesNoDestinationToAnEntryWithOnlyAHostname", func(t *testing.T) {
		// Given a pull-through cache with a hostname and no endpoint
		registries := map[string]any{"ghcr.io": map[string]any{"remote": "https://ghcr.io", "hostname": "ghcr.test", "hostport": 5002}}

		// When the images are listed
		result := setup(t, registries, "ghcr.io/example/worker")

		// Then the image has no destination
		if got := destination(t, result, "ghcr.io/example/worker"); got != "" {
			t.Errorf("expected no destination, got %q", got)
		}
	})

	t.Run("DoesNotUseTheCatchAllForAnEntryWithNoEndpoint", func(t *testing.T) {
		// Given a pull-through cache with no endpoint and a catch-all entry
		registries := map[string]any{
			"ghcr.io": map[string]any{"remote": "https://ghcr.io", "hostname": "ghcr.test"},
			"*":       map[string]any{"endpoint": "harbor.example.com/all"},
		}

		// When the images are listed
		result := setup(t, registries, "ghcr.io/example/worker")

		// Then the image has no destination
		if got := destination(t, result, "ghcr.io/example/worker"); got != "" {
			t.Errorf("expected no destination, got %q", got)
		}
	})

	t.Run("GivesNoDestinationToAnImageOnItsMirrorHost", func(t *testing.T) {
		// Given a catch-all entry and an image already on the mirror host
		registries := map[string]any{"*": map[string]any{"endpoint": "harbor.example.com/windsor"}}

		// When the images are listed
		result := setup(t, registries, "harbor.example.com/team/app")

		// Then the image has no destination
		if got := destination(t, result, "harbor.example.com/team/app"); got != "" {
			t.Errorf("expected no destination, got %q", got)
		}
	})

	t.Run("FailsWhenTwoEntriesGiveOneHostDifferentMirrors", func(t *testing.T) {
		// Given two entries that name one upstream host
		registries := map[string]any{
			"docker.io":            map[string]any{"endpoint": "a.example.com"},
			"registry-1.docker.io": map[string]any{"local": "docker.io", "endpoint": "b.example.com"},
		}

		// When the images are listed
		_, err := listImages(t, registries, "docker.io/library/nginx")

		// Then the error names the host
		if err == nil || !strings.Contains(err.Error(), "docker.io") {
			t.Fatalf("expected an error that names the host, got %v", err)
		}
	})

	t.Run("FailsWhenTwoImagesHaveTheSameDestination", func(t *testing.T) {
		// Given one catch-all entry and two images with the same path on different hosts
		registries := map[string]any{"*": map[string]any{"endpoint": "harbor.example.com"}}

		// When the images are listed
		_, err := listImages(t, registries, "ghcr.io/example/worker", "quay.io/example/worker")

		// Then the error names the destination
		if err == nil || !strings.Contains(err.Error(), "harbor.example.com/example/worker:1.0.0") {
			t.Fatalf("expected an error that names the destination, got %v", err)
		}
	})

	t.Run("GivesABlueprintTheMirrorOfItsHost", func(t *testing.T) {
		// Given a mirror for ghcr.io and an oci:// blueprint source on ghcr.io
		m := setupMocks(t)
		setRegistries(t, m.configHandler, map[string]any{"ghcr.io": map[string]any{"endpoint": "http://mirror.test:5000"}})
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}}}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{All: true})

		// Then the blueprint goes to the mirror and keeps its upstream reference
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := destination(t, result, "ghcr.io/windsorcli/core"); got != "mirror.test:5000/windsorcli/core:v0.9.0" {
			t.Errorf("got destination %q", got)
		}
	})

	t.Run("FailsWhenAnImageAndABlueprintHaveTheSameDestination", func(t *testing.T) {
		// Given a catch-all mirror, an image, and a blueprint with one path and tag on two hosts
		m := setupMocks(t)
		setRegistries(t, m.configHandler, map[string]any{"*": map[string]any{"endpoint": "harbor.example.com"}})
		project := t.TempDir()
		writeFile(t, project, "kustomize/app/values.yaml", "# artifact: type=image name=quay.io/windsorcli/core\nimage: quay.io/windsorcli/core:v0.9.0\n")
		m.runtime.ProjectRoot = project
		lister := inventory.NewLister(m.runtime, m.artifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{Sources: []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}}}

		// When List is called
		_, err := lister.List(blueprint, inventory.Options{All: true})

		// Then the error names the destination
		if err == nil || !strings.Contains(err.Error(), "harbor.example.com/windsorcli/core") {
			t.Errorf("expected an error that names the destination, got %v", err)
		}
	})
}

// setRegistries makes the header of handler hold registries as its registries map.
func setRegistries(t *testing.T, handler *config.MockConfigHandler, registries map[string]any) {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{"registries": registries})
	if err != nil {
		t.Fatal(err)
	}
	var header cliconfig.Context
	if err := yaml.Unmarshal(data, &header); err != nil {
		t.Fatal(err)
	}
	handler.GetHeaderFunc = func() *cliconfig.Context { return &header }
}

// listImages lists a project that tags each image at version 1.0.0, with registries as the registries map.
func listImages(t *testing.T, registries map[string]any, images ...string) (*inventory.Result, error) {
	t.Helper()
	tags := make([]string, 0, len(images))
	for _, image := range images {
		tags = append(tags, "# artifact: type=image name="+image+"\nimage: "+image+":1.0.0\n")
	}
	return listTagged(t, registries, tags...)
}

// listTagged lists a project that holds tags, with registries as the registries map.
func listTagged(t *testing.T, registries map[string]any, tags ...string) (*inventory.Result, error) {
	t.Helper()
	m := setupMocks(t)
	setRegistries(t, m.configHandler, registries)
	project := t.TempDir()
	var content strings.Builder
	for _, tag := range tags {
		content.WriteString(tag)
	}
	writeFile(t, project, "kustomize/app/values.yaml", content.String())
	m.runtime.ProjectRoot = project
	lister := inventory.NewLister(m.runtime, m.artifactBuilder)
	return lister.List(&blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "app"}}, inventory.Options{All: true})
}

func TestCheckDestinationDigests(t *testing.T) {
	list := func(t *testing.T, tags ...string) error {
		t.Helper()
		_, err := listTagged(t, map[string]any{"*": map[string]any{"endpoint": "harbor.example.com"}}, tags...)
		return err
	}
	worker := func(host, digest string) string {
		return "# artifact: type=image name=" + host + "/example/worker\nimage: " + host + "/example/worker:1.0.0@" + digest + "\n"
	}

	t.Run("FailsWhenImagesDifferOnlyByDigest", func(t *testing.T) {
		// Given two images with one tag and different digests on different hosts
		// When the images are listed
		err := list(t, worker("ghcr.io", "sha256:"+strings.Repeat("a", 64)), worker("quay.io", "sha256:"+strings.Repeat("b", 64)))

		// Then the images collide at the tag
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("AllowsOneImageOnTwoHosts", func(t *testing.T) {
		// Given one image on two hosts, with one digest
		// When the images are listed
		err := list(t, worker("ghcr.io", "sha256:"+strings.Repeat("a", 64)), worker("quay.io", "sha256:"+strings.Repeat("a", 64)))

		// Then the images do not collide
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
