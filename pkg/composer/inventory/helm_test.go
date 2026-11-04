package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/chart/loader"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"

	"github.com/windsorcli/cli/pkg/composer/inventory"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Test Setup
// =============================================================================

const testChartTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  template:
    spec:
      containers:
      - image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
{{- if .Capabilities.APIVersions.Has "monitoring.coreos.com/v1/ServiceMonitor" }}
      - image: ghcr.io/example/metrics:1.0.0
{{- end }}
`

const testChartHook = `apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-test
  annotations:
    helm.sh/hook: test
spec:
  template:
    spec:
      containers:
      - image: ghcr.io/example/test:1.0.0
`

// cacheTestChart packages a chart into the chart cache path of an OCI entry under projectRoot.
func cacheTestChart(t *testing.T, projectRoot string) inventory.Entry {
	t.Helper()
	src := t.TempDir()
	files := map[string]string{
		"Chart.yaml":          "apiVersion: v2\nname: demo\nversion: 1.2.3\n",
		"values.yaml":         "image:\n  repository: ghcr.io/example/app\n  tag: default\n",
		"templates/app.yaml":  testChartTemplate,
		"templates/test.yaml": testChartHook,
	}
	for name, content := range files {
		path := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	entry := inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: "ghcr.io/example/charts/demo", Name: "demo", Version: "1.2.3"}
	cacheDir := filepath.Join(projectRoot, ".windsor", "cache", "charts", "ghcr.io_example_charts_demo")
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := chartutil.Save(loaded.(*chartv2.Chart), cacheDir); err != nil {
		t.Fatal(err)
	}
	return entry
}

func images(objects []map[string]any) []string {
	var out []string
	for _, obj := range objects {
		containers, _ := obj["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
		for _, c := range containers {
			out = append(out, c.(map[string]any)["image"].(string))
		}
	}
	return out
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestHelmRenderer_Render(t *testing.T) {
	t.Run("RendersWithMergedValuesAndPostRenderers", func(t *testing.T) {
		// Given a cached chart, a values ConfigMap, spec.values, an images post-renderer and a CRD
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: {name: demo, namespace: apps}
spec:
  valuesFrom:
  - kind: ConfigMap
    name: demo-values
  values:
    image: {repository: ghcr.io/example/app}
  postRenderers:
  - kustomize:
      images:
      - name: ghcr.io/example/app
        newName: ghcr.io/example/patched`)[0]
		objects := decode(`apiVersion: v1
kind: ConfigMap
metadata: {name: demo-values, namespace: apps}
data:
  values.yaml: "image: {tag: from-configmap}"
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: servicemonitors.monitoring.coreos.com}
spec:
  group: monitoring.coreos.com
  names: {kind: ServiceMonitor}
  versions: [{name: v1, served: true}]`)

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr, Objects: objects})

		// Then the image takes the ConfigMap tag, the post-renderer name and the CRD-gated container
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		got := strings.Join(images(out), ",")
		if got != "ghcr.io/example/patched:from-configmap,ghcr.io/example/metrics:1.0.0" {
			t.Errorf("unexpected images %q", got)
		}
	})

	t.Run("RendersTestHooksOnlyWhenEnabled", func(t *testing.T) {
		// Given a cached chart and a HelmRelease with tests enabled
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}\nspec: {test: {enable: true}}")[0]

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then the test hook image is in the output
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !strings.Contains(strings.Join(images(out), ","), "ghcr.io/example/test:1.0.0") {
			t.Errorf("expected the test hook image, got %v", images(out))
		}
	})

	t.Run("LeavesOutTestHooksByDefault", func(t *testing.T) {
		// Given a cached chart and a HelmRelease without tests
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}")[0]

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then only the default image is in the output
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := strings.Join(images(out), ","); got != "ghcr.io/example/app:default" {
			t.Errorf("unexpected images %q", got)
		}
	})

	t.Run("FailsForAMissingValuesReference", func(t *testing.T) {
		// Given a HelmRelease whose valuesFrom ConfigMap is not rendered
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}\nspec: {valuesFrom: [{kind: ConfigMap, name: missing}]}")[0]

		// When Render is called
		_, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then the error names the HelmRelease
		if err == nil || !strings.Contains(err.Error(), "apps/demo") {
			t.Fatalf("expected a values error, got %v", err)
		}
	})

	t.Run("ShortensALongReleaseName", func(t *testing.T) {
		// Given a HelmRelease whose release name is over 53 characters
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: kube-prometheus-stack-operator, namespace: apps}\nspec: {targetNamespace: system-monitoring-observability}")[0]

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then the render succeeds with a 53-character name
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		name, _ := out[0]["metadata"].(map[string]any)["name"].(string)
		if len(name) != 53 || !strings.HasPrefix(name, "system-monitoring-observability-kube-pro-") {
			t.Errorf("unexpected release name %q", name)
		}
	})

	t.Run("GivesTheReleaseNamespaceToObjectsWithNone", func(t *testing.T) {
		// Given a chart whose template sets no namespace
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}\nspec: {targetNamespace: target, test: {enable: true}}")[0]

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then the test hook Job takes the target namespace
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		for _, obj := range out {
			if obj["kind"] == "Job" && obj["metadata"].(map[string]any)["namespace"] != "target" {
				t.Errorf("expected namespace target, got %v", obj["metadata"])
			}
		}
	})

	t.Run("FailsForAReferencedSecretThatDoesNotConvert", func(t *testing.T) {
		// Given a valuesFrom Secret whose data is not base64, as SOPS leaves it
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}\nspec: {valuesFrom: [{kind: Secret, name: sealed, optional: true}]}")[0]
		objects := decode("apiVersion: v1\nkind: Secret\nmetadata: {name: sealed, namespace: apps}\ndata: {values.yaml: 'ENC[AES256_GCM,data:x]'}")

		// When Render is called
		_, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr, Objects: objects})

		// Then the error names the Secret
		if err == nil || !strings.Contains(err.Error(), "Secret apps/sealed") {
			t.Fatalf("expected a Secret error, got %v", err)
		}
	})

	t.Run("ReadsADigestPinnedChartFromItsOwnCacheFile", func(t *testing.T) {
		// Given a chart cached under the archive name of its digest
		root := t.TempDir()
		chart := cacheTestChart(t, root)
		chart.Digest = "sha256:abc"
		cacheDir := filepath.Join(root, ".windsor", "cache", "charts", "ghcr.io_example_charts_demo")
		if err := os.Rename(filepath.Join(cacheDir, "demo-1.2.3.tgz"), filepath.Join(cacheDir, "demo-1.2.3-sha256_abc.tgz")); err != nil {
			t.Fatal(err)
		}
		renderer := inventory.NewHelmRenderer(&runtime.Runtime{ProjectRoot: root}, inventory.NewShims())
		hr := decode("kind: HelmRelease\nmetadata: {name: demo, namespace: apps}")[0]

		// When Render is called
		out, err := renderer.Render(inventory.ChartRequest{Chart: chart, HelmRelease: hr})

		// Then the chart renders from the digest cache file
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := strings.Join(images(out), ","); got != "ghcr.io/example/app:default" {
			t.Errorf("unexpected images %q", got)
		}
	})
}
