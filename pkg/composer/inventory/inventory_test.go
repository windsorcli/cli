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

	t.Run("ListsTheBlueprintsAndTagsOfNestedSources", func(t *testing.T) {
		// Given a composed blueprint whose sources hold a parent blueprint and a blueprint it declares
		m := setupMocks(t)
		parent, child := t.TempDir(), t.TempDir()
		writeFile(t, parent, "kustomize/base/values.yaml", "# artifact: type=image name=ghcr.io/example/parent\nimage: ghcr.io/example/parent:1.0.0\n")
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
		result, err := lister.List(blueprint, inventory.Options{})

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
}

// listImages lists a project that tags each image at version 1.0.0, with registries as docker.registries.
func listImages(t *testing.T, registries map[string]any, images ...string) (*inventory.Result, error) {
	t.Helper()
	tags := make([]string, 0, len(images))
	for _, image := range images {
		tags = append(tags, "# artifact: type=image name="+image+"\nimage: "+image+":1.0.0\n")
	}
	return listTagged(t, registries, tags...)
}

// listTagged lists a project that holds tags, with registries as docker.registries.
func listTagged(t *testing.T, registries map[string]any, tags ...string) (*inventory.Result, error) {
	t.Helper()
	m := setupMocks(t)
	m.configHandler.GetFunc = func(key string) any {
		if key == "docker.registries" {
			return registries
		}
		return nil
	}
	project := t.TempDir()
	var content strings.Builder
	for _, tag := range tags {
		content.WriteString(tag)
	}
	writeFile(t, project, "kustomize/app/values.yaml", content.String())
	m.runtime.ProjectRoot = project
	lister := inventory.NewLister(m.runtime, m.artifactBuilder)
	return lister.List(&blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "app"}}, inventory.Options{})
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
