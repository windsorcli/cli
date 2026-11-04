package inventory_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	blueprintcomposer "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/composer/inventory"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	sigsyaml "sigs.k8s.io/yaml"
)

// =============================================================================
// Test Setup
// =============================================================================

const renderedInstall = `apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: openebs
  namespace: system-csi
spec:
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
  ref:
    tag: 4.6.2
  url: oci://ghcr.io/openebs/charts/openebs
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: ingress-nginx
  namespace: system-ingress
spec:
  url: https://kubernetes.github.io/ingress-nginx
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: ingress-nginx
  namespace: system-ingress
spec:
  chart:
    spec:
      chart: ingress-nginx
      version: 4.15.1
      sourceRef:
        kind: HelmRepository
        name: ingress-nginx
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: etcd
  namespace: system-dns
spec:
  template:
    spec:
      initContainers:
      - image: busybox:1.37
      containers:
      - image: quay.io/coreos/etcd:v3.7.2@sha256:abc
`

type mocks struct {
	runtime         *runtime.Runtime
	shell           *shell.MockShell
	configHandler   *config.MockConfigHandler
	artifactBuilder *artifact.MockArtifact
	shims           *inventory.Shims
	fluxArgs        [][]string
	files           map[string]string
}

func setupMocks(t *testing.T) *mocks {
	t.Helper()
	m := &mocks{files: make(map[string]string)}

	m.shell = shell.NewMockShell()
	m.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "flux: v2.9.3\n", nil
	}
	m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
		m.fluxArgs = append(m.fluxArgs, append([]string{command}, args...))
		return renderedInstall, nil
	}

	m.configHandler = config.NewMockConfigHandler()
	m.configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	m.configHandler.GetFunc = func(key string) any { return nil }

	m.artifactBuilder = artifact.NewMockArtifact()
	m.artifactBuilder.ParseOCIRefFunc = func(ociRef string) (string, string, string, error) {
		return "ghcr.io", "windsorcli/core", "v0.9.0", nil
	}
	m.artifactBuilder.GetCacheDirFunc = func(registry, repository, tag string) (string, error) {
		return "/project/.windsor/cache/oci/core", nil
	}

	m.shims = inventory.NewShims()
	m.shims.LookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	m.shims.MkdirTemp = func(dir, pattern string) (string, error) { return "/tmp/inventory", nil }
	m.shims.RemoveAll = func(path string) error { return nil }
	m.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		m.files[filepath.ToSlash(name)] = string(data)
		return nil
	}

	m.runtime = &runtime.Runtime{
		ContextName:   "local",
		ProjectRoot:   "/project",
		ConfigHandler: m.configHandler,
		Shell:         m.shell,
	}
	return m
}

func newLister(m *mocks) *inventory.BaseLister {
	lister := inventory.NewLister(m.runtime, m.artifactBuilder)
	lister.SetShims(m.shims)
	lister.SetExtractors([]inventory.Extractor{})
	lister.SetChartRenderer(&fakeRenderer{})
	return lister
}

type fakeRenderer struct {
	requests []inventory.ChartRequest
	out      map[string]string
	err      error
}

func (f *fakeRenderer) Render(req inventory.ChartRequest) ([]map[string]any, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return decode(f.out[req.Chart.Name]), nil
}

func decode(data string) []map[string]any {
	var objects []map[string]any
	for _, doc := range strings.Split(data, "\n---\n") {
		var obj map[string]any
		if err := sigsyaml.Unmarshal([]byte(doc), &obj); err == nil && len(obj) > 0 {
			objects = append(objects, obj)
		}
	}
	return objects
}

func testBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "local"},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "csi-install", Path: "csi/install", Components: []string{"openebs"}},
		},
	}
}

type fakeExtractor struct {
	objects []map[string]any
	err     error
}

func (f *fakeExtractor) Matches(obj map[string]any) bool { return obj["kind"] == "StatefulSet" }

func (f *fakeExtractor) Extract(obj map[string]any) ([]map[string]any, error) {
	return f.objects, f.err
}

func findEntry(entries []inventory.Entry, kind inventory.Kind, reference string) *inventory.Entry {
	for i := range entries {
		if entries[i].Kind == kind && entries[i].Reference == reference {
			return &entries[i]
		}
	}
	return nil
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
	t.Run("ListsChartsAndImagesFromTheRender", func(t *testing.T) {
		// Given a Kustomization that renders chart sources and a pod template
		m := setupMocks(t)
		lister := newLister(m)

		// When List is called with a chart registry
		result, err := lister.List(testBlueprint(), inventory.Options{ChartsTo: "oci://registry.example.com/windsor/"})

		// Then the charts and images are listed with destinations
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		entries := result.Inventory.Artifacts
		if len(entries) != 4 {
			t.Fatalf("expected 4 entries, got %d: %+v", len(entries), entries)
		}
		openebs := findEntry(entries, inventory.KindChart, "ghcr.io/openebs/charts/openebs")
		if openebs == nil || openebs.Destination != "registry.example.com/windsor/openebs:4.6.2" || openebs.Source.Object != "OCIRepository/system-csi/openebs" {
			t.Errorf("unexpected openebs entry: %+v", openebs)
		}
		nginx := findEntry(entries, inventory.KindChart, "https://kubernetes.github.io/ingress-nginx")
		if nginx == nil || nginx.Transport != inventory.TransportHTTP || nginx.Destination != "registry.example.com/windsor/ingress-nginx:4.15.1" {
			t.Errorf("unexpected ingress-nginx entry: %+v", nginx)
		}
		etcd := findEntry(entries, inventory.KindImage, "quay.io/coreos/etcd")
		if etcd == nil || etcd.Version != "v3.7.2" || etcd.Digest != "sha256:abc" || etcd.Source.Kustomization != "csi-install" {
			t.Errorf("unexpected etcd entry: %+v", etcd)
		}
		if findEntry(entries, inventory.KindImage, "docker.io/library/busybox") == nil {
			t.Error("expected the Docker Hub image to resolve to docker.io/library/busybox")
		}
		if result.Inventory.Version != inventory.InventoryVersion || result.Inventory.Context != "local" {
			t.Errorf("unexpected header: %+v", result.Inventory)
		}
	})

	t.Run("RendersOfflineWithInlineSubstitutions", func(t *testing.T) {
		// Given a blueprint ConfigMap that overrides a Kustomization substitution, and a deferred value
		m := setupMocks(t)
		lister := newLister(m)
		bp := testBlueprint()
		bp.ConfigMaps = map[string]map[string]string{"values-common": {"chart_registry": "", "domain": "test"}}
		bp.Kustomizations[0].Substitutions = map[string]string{"domain": "override", "token": blueprintcomposer.DeferredPlaceholder}

		// When List is called
		if _, err := lister.List(bp, inventory.Options{}); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then flux renders in dry-run mode with the values inline
		args := filepath.ToSlash(strings.Join(m.fluxArgs[0], " "))
		for _, want := range []string{"flux build kustomization csi-install", "--path /project/kustomize/csi/install", "--dry-run", "--in-memory-build"} {
			if !strings.Contains(args, want) {
				t.Errorf("expected %q in %q", want, args)
			}
		}
		file := m.files["/tmp/inventory/csi-install.yaml"]
		for _, want := range []string{"chart_registry: \"\"", "domain: test", "token: windsor-deferred-value", "- openebs"} {
			if !strings.Contains(file, want) {
				t.Errorf("expected %q in the kustomization file:\n%s", want, file)
			}
		}
		if strings.Contains(file, "substituteFrom") {
			t.Errorf("expected no substituteFrom in the kustomization file:\n%s", file)
		}
	})

	t.Run("RendersAnOCISourceFromTheCache", func(t *testing.T) {
		// Given a Kustomization from an oci:// source
		m := setupMocks(t)
		lister := newLister(m)
		bp := testBlueprint()
		bp.Sources = []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}}
		bp.Kustomizations[0].Source = "core"

		// When List is called
		result, err := lister.List(bp, inventory.Options{})

		// Then the path is in the OCI cache and the source is a blueprint entry
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !strings.Contains(filepath.ToSlash(strings.Join(m.fluxArgs[0], " ")), "--path /project/.windsor/cache/oci/core/kustomize/csi/install") {
			t.Errorf("expected the cache path, got %v", m.fluxArgs[0])
		}
		core := findEntry(result.Inventory.Artifacts, inventory.KindBlueprint, "ghcr.io/windsorcli/core")
		if core == nil || core.Version != "v0.9.0" || core.Destination != "" {
			t.Errorf("unexpected blueprint entry: %+v", core)
		}
	})

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given only a destroy-only Kustomization
		m := setupMocks(t)
		m.shims.LookPath = func(file string) (string, error) { return "", errors.New("not found") }
		lister := newLister(m)
		bp := testBlueprint()
		destroyOnly := true
		bp.Kustomizations[0].DestroyOnly = &destroyOnly

		// When List is called
		result, err := lister.List(bp, inventory.Options{})

		// Then nothing renders and the flux CLI is not needed
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result.Inventory.Artifacts) != 0 || len(m.fluxArgs) != 0 {
			t.Errorf("expected no render, got %v", m.fluxArgs)
		}
	})

	t.Run("GivesImagesTheirMirror", func(t *testing.T) {
		// Given docker.registries mirrors for quay.io and Docker Hub
		m := setupMocks(t)
		m.configHandler.GetFunc = func(key string) any {
			return map[string]any{
				"quay.io":              map[string]any{"hostname": "quay.mirror.test", "hostport": 5000},
				"registry-1.docker.io": map[string]any{"local": "https://docker.io", "hostname": "hub.mirror.test"},
				"gcr.io":               map[string]any{"remote": "https://gcr.io"},
			}
		}
		lister := newLister(m)

		// When List is called
		result, err := lister.List(testBlueprint(), inventory.Options{})

		// Then each image goes to the mirror of its host under its upstream path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		etcd := findEntry(result.Inventory.Artifacts, inventory.KindImage, "quay.io/coreos/etcd")
		if etcd.Destination != "quay.mirror.test:5000/coreos/etcd:v3.7.2@sha256:abc" {
			t.Errorf("unexpected etcd destination %q", etcd.Destination)
		}
		busybox := findEntry(result.Inventory.Artifacts, inventory.KindImage, "docker.io/library/busybox")
		if busybox.Destination != "hub.mirror.test/library/busybox:1.37" {
			t.Errorf("unexpected busybox destination %q", busybox.Destination)
		}
	})

	t.Run("ReadsTheObjectsOfAnExtractor", func(t *testing.T) {
		// Given an extractor that derives a Deployment
		m := setupMocks(t)
		lister := newLister(m)
		lister.SetExtractors([]inventory.Extractor{&fakeExtractor{objects: []map[string]any{{
			"kind": "Deployment",
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"image": "ghcr.io/fluxcd/source-controller:v1.9.0"}},
			}}},
		}}}})

		// When List is called
		result, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the derived image names its parent object
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		controller := findEntry(result.Inventory.Artifacts, inventory.KindImage, "ghcr.io/fluxcd/source-controller")
		if controller == nil || controller.Source.Object != "StatefulSet/system-dns/etcd" {
			t.Errorf("unexpected derived entry: %+v", controller)
		}
	})

	t.Run("ReturnsExtractorError", func(t *testing.T) {
		// Given an extractor that fails
		m := setupMocks(t)
		lister := newLister(m)
		lister.SetExtractors([]inventory.Extractor{&fakeExtractor{err: errors.New("pull failed")}})

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error names the object
		if err == nil || !strings.Contains(err.Error(), "StatefulSet/system-dns/etcd") {
			t.Fatalf("expected an error naming the object, got %v", err)
		}
	})

	t.Run("FailsWhenADeferredValueReachesAnImage", func(t *testing.T) {
		// Given a render whose image holds the deferred marker
		m := setupMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "kind: Pod\nmetadata:\n  name: p\nspec:\n  containers:\n  - image: windsor-deferred-value/app:1\n", nil
		}
		lister := newLister(m)

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error names the object
		if err == nil || !strings.Contains(err.Error(), "Pod/p") {
			t.Fatalf("expected a deferred value error, got %v", err)
		}
	})

	t.Run("NotesUnpinnedAndUnresolvedCharts", func(t *testing.T) {
		// Given a semver range, a missing HelmRepository and an OCIRepository without a tag
		m := setupMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return `kind: HelmRepository
metadata: {name: repo, namespace: ns}
spec: {url: "oci://ghcr.io/org/charts"}
---
kind: HelmRelease
metadata: {name: ranged, namespace: ns}
spec: {chart: {spec: {chart: app, version: ">=1.0.0", sourceRef: {kind: HelmRepository, name: repo}}}}
---
kind: HelmRelease
metadata: {name: orphan, namespace: ns}
spec: {chart: {spec: {chart: other, version: 1.0.0, sourceRef: {kind: HelmRepository, name: missing}}}}
---
kind: HelmRelease
metadata: {name: byref, namespace: ns}
spec: {chartRef: {kind: OCIRepository, name: digest-only}}
---
kind: OCIRepository
metadata: {name: digest-only, namespace: ns}
spec: {url: "oci://ghcr.io/org/digest-only", ref: {digest: "sha256:def"}}
`, nil
		}
		lister := newLister(m)

		// When List is called with a chart registry
		result, err := lister.List(testBlueprint(), inventory.Options{ChartsTo: "registry.example.com"})

		// Then the charts without one version have no destination and each gives a note
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		ranged := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/org/charts/app")
		if ranged == nil || ranged.Destination != "" {
			t.Errorf("unexpected ranged entry: %+v", ranged)
		}
		digestOnly := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/org/digest-only")
		if digestOnly == nil || digestOnly.Destination != "" {
			t.Errorf("unexpected digest-only entry: %+v", digestOnly)
		}
		if len(result.Notes) != 3 {
			t.Errorf("expected 3 notes, got %v", result.Notes)
		}
	})

	t.Run("ResolvesSourcesOfNamespacelessObjects", func(t *testing.T) {
		// Given a HelmRelease and its sources with no namespace
		m := setupMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return `kind: HelmRepository
metadata: {name: repo}
spec: {url: "https://charts.example.com"}
---
kind: HelmRelease
metadata: {name: app}
spec: {chart: {spec: {chart: app, version: 1.0.0, sourceRef: {kind: HelmRepository, name: repo}}}}
---
kind: HelmRelease
metadata: {name: other}
spec: {chartRef: {kind: OCIRepository, name: other}}
---
kind: OCIRepository
metadata: {name: other}
spec: {url: "oci://ghcr.io/org/other", ref: {tag: 2.0.0}}
`, nil
		}
		lister := newLister(m)

		// When List is called
		result, err := lister.List(testBlueprint(), inventory.Options{})

		// Then both charts are listed with no notes
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if findEntry(result.Inventory.Artifacts, inventory.KindChart, "https://charts.example.com") == nil || findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/org/other") == nil {
			t.Errorf("expected both charts, got %+v", result.Inventory.Artifacts)
		}
		if len(result.Notes) != 0 {
			t.Errorf("expected no notes, got %v", result.Notes)
		}
	})

	t.Run("ReadsImagesFromRenderedCharts", func(t *testing.T) {
		// Given a chart whose output holds a Deployment and a nested HelmRelease
		m := setupMocks(t)
		lister := newLister(m)
		renderer := &fakeRenderer{out: map[string]string{
			"ingress-nginx": `kind: Deployment
metadata: {name: controller, namespace: system-ingress}
spec: {template: {spec: {containers: [{image: "registry.k8s.io/ingress-nginx/controller:v1.15.1"}]}}}
---
kind: OCIRepository
metadata: {name: nested, namespace: system-ingress}
spec: {url: "oci://ghcr.io/org/nested", ref: {tag: 1.0.0}}
---
kind: HelmRelease
metadata: {name: nested, namespace: system-ingress}
spec: {chartRef: {kind: OCIRepository, name: nested}}`,
			"nested": `kind: Job
metadata: {name: hook, namespace: system-ingress}
spec: {template: {spec: {containers: [{image: "ghcr.io/org/hook:2.0.0"}]}}}`,
		}}
		lister.SetChartRenderer(renderer)

		// When List is called
		result, err := lister.List(testBlueprint(), inventory.Options{})

		// Then each chart renders once and its images name the HelmRelease
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		controller := findEntry(result.Inventory.Artifacts, inventory.KindImage, "registry.k8s.io/ingress-nginx/controller")
		if controller == nil || controller.Source.Object != "HelmRelease/system-ingress/ingress-nginx" {
			t.Errorf("unexpected controller entry: %+v", controller)
		}
		hook := findEntry(result.Inventory.Artifacts, inventory.KindImage, "ghcr.io/org/hook")
		if hook == nil || hook.Source.Object != "HelmRelease/system-ingress/nested" {
			t.Errorf("unexpected hook entry: %+v", hook)
		}
		if findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/org/nested") == nil {
			t.Error("expected the nested chart")
		}
		if len(renderer.requests) != 2 || renderer.requests[0].Chart.Transport != inventory.TransportHTTP || len(renderer.requests[0].Objects) == 0 {
			t.Errorf("unexpected render requests: %+v", renderer.requests)
		}
	})

	t.Run("ReturnsChartRenderError", func(t *testing.T) {
		// Given a chart that fails to render
		m := setupMocks(t)
		lister := newLister(m)
		lister.SetChartRenderer(&fakeRenderer{err: errors.New("pull failed")})

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error names the HelmRelease
		if err == nil || !strings.Contains(err.Error(), "HelmRelease/system-ingress/ingress-nginx") {
			t.Fatalf("expected a render error, got %v", err)
		}
	})

	t.Run("FailsWithoutFlux", func(t *testing.T) {
		// Given no flux CLI
		m := setupMocks(t)
		m.shims.LookPath = func(file string) (string, error) { return "", errors.New("not found") }
		lister := newLister(m)

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error asks for the flux CLI
		if err == nil || !strings.Contains(err.Error(), "flux CLI is required") {
			t.Fatalf("expected a missing flux error, got %v", err)
		}
	})

	t.Run("FailsWithAnOldFlux", func(t *testing.T) {
		// Given flux v2.2
		m := setupMocks(t)
		m.shell.ExecSilentFunc = func(command string, args ...string) (string, error) { return "flux: v2.2.0\n", nil }
		lister := newLister(m)

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error names the minimum version
		if err == nil || !strings.Contains(err.Error(), "v2.3 or later") {
			t.Fatalf("expected a version error, got %v", err)
		}
	})

	t.Run("ReturnsRenderError", func(t *testing.T) {
		// Given a flux build that fails
		m := setupMocks(t)
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "", errors.New("build failed")
		}
		lister := newLister(m)

		// When List is called
		_, err := lister.List(testBlueprint(), inventory.Options{})

		// Then the error names the Kustomization
		if err == nil || !strings.Contains(err.Error(), `kustomization "csi-install"`) {
			t.Fatalf("expected a render error, got %v", err)
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a lister
		m := setupMocks(t)
		lister := newLister(m)

		// When List is called with no blueprint
		_, err := lister.List(nil, inventory.Options{})

		// Then an error is returned
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("ListsTheBlueprintWideSetFromManifests", func(t *testing.T) {
		// Given an artifact-manifest.yaml in the OCI cache and none in the project root
		m := setupMocks(t)
		m.shims.ReadFile = func(name string) ([]byte, error) {
			if filepath.ToSlash(name) == "/project/.windsor/cache/oci/core/artifact-manifest.yaml" {
				return []byte(`version: v1alpha1
artifacts:
- type: docker
  transport: oci
  reference: registry.k8s.io/pause:3.10
  digest: sha256:123
  source: {file: kustomize/a.yaml, datasource: docker}
- type: helm
  transport: oci
  reference: ingress-nginx
  version: 4.15.1
  repository: https://kubernetes.github.io/ingress-nginx
  source: {file: kustomize/b.yaml, datasource: helm}
- type: helm
  transport: oci
  reference: kyverno
  version: 3.9.2
  repository: oci://ghcr.io/kyverno/charts
  source: {file: kustomize/c.yaml, datasource: helm}
- type: github-release
  transport: tarball
  reference: siderolabs/talos
  version: v1.14.5
  source: {file: terraform/d.tf, datasource: github-releases}
`), nil
			}
			return nil, os.ErrNotExist
		}
		lister := newLister(m)
		bp := testBlueprint()
		bp.Sources = []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"}}

		// When List is called with All
		result, err := lister.List(bp, inventory.Options{All: true, ChartsTo: "registry.example.com"})

		// Then the docker and helm entries are listed without a render
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(m.fluxArgs) != 0 {
			t.Errorf("expected no render, got %v", m.fluxArgs)
		}
		pause := findEntry(result.Inventory.Artifacts, inventory.KindImage, "registry.k8s.io/pause")
		if pause == nil || pause.Version != "3.10" || pause.Digest != "sha256:123" || pause.Source.File != "kustomize/a.yaml" {
			t.Errorf("unexpected pause entry: %+v", pause)
		}
		kyverno := findEntry(result.Inventory.Artifacts, inventory.KindChart, "ghcr.io/kyverno/charts/kyverno")
		if kyverno == nil || kyverno.Destination != "registry.example.com/kyverno:3.9.2" {
			t.Errorf("unexpected kyverno entry: %+v", kyverno)
		}
		if findEntry(result.Inventory.Artifacts, inventory.KindChart, "https://kubernetes.github.io/ingress-nginx") == nil {
			t.Error("expected the HTTP chart")
		}
		if len(result.Inventory.Artifacts) != 4 {
			t.Errorf("expected 4 entries, got %+v", result.Inventory.Artifacts)
		}
		if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], `"local"`) {
			t.Errorf("expected a note for the project source, got %v", result.Notes)
		}
	})
}
