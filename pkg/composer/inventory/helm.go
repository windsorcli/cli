// The HelmRenderer is the chart render step of the Lister.
// It provides the objects that a HelmRelease installs, rendered client-side the way helm-controller renders them.
// It pulls each chart into the project cache, merges values in helm-controller's order, and runs the Helm v4 SDK.
// A Kustomize post-renderer applies after the render, because it can change an image.

package inventory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/chartutil"
	"github.com/go-logr/logr"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/loader"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/downloader"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/registry"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/repo/v1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// ChartRequest describes one HelmRelease render.
// Objects holds every rendered object, so valuesFrom resolves and CRDs add API versions.
type ChartRequest struct {
	Chart       Entry
	HelmRelease map[string]any
	Objects     []map[string]any
}

// HelmRenderer renders charts with the Helm v4 SDK.
type HelmRenderer struct {
	runtime *runtime.Runtime
	shims   *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// ChartRenderer renders the chart of a HelmRelease into Kubernetes objects.
type ChartRenderer interface {
	Render(req ChartRequest) ([]map[string]any, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewHelmRenderer creates a HelmRenderer that caches charts under the project root of rt.
func NewHelmRenderer(rt *runtime.Runtime, shims *Shims) *HelmRenderer {
	return &HelmRenderer{runtime: rt, shims: shims}
}

// =============================================================================
// Public Methods
// =============================================================================

// Render pulls the chart of req, merges chart defaults, valuesFrom and spec.values, and renders the
// chart client-side with hooks and CRDs. Test hooks render only when spec.test.enable is true.
// Capabilities hold Helm's default Kubernetes version and the built-in APIs plus each CRD in
// req.Objects and in the chart. Spec.postRenderers apply last. Each object with no namespace gets
// the release namespace, as Helm installs it there.
func (r *HelmRenderer) Render(req ChartRequest) ([]map[string]any, error) {
	chartPath, err := r.pull(req.Chart)
	if err != nil {
		return nil, err
	}
	loaded, err := loader.Load(chartPath)
	if err != nil {
		return nil, werror.Wrap(err, "failed to load chart %s", req.Chart.Upstream())
	}
	chrt, ok := loaded.(*chartv2.Chart)
	if !ok {
		return nil, fmt.Errorf("chart %s has an API version that the inventory does not render", req.Chart.Upstream())
	}

	hr := req.HelmRelease
	namespace := nestedString(hr, "spec", "targetNamespace")
	if namespace == "" {
		namespace = nestedString(hr, "metadata", "namespace")
	}
	values, err := releaseValues(hr, req.Objects, nestedString(hr, "metadata", "namespace"))
	if err != nil {
		return nil, err
	}

	install := action.NewInstall(action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil))))
	install.DryRunStrategy = action.DryRunClient
	install.Replace = true
	install.IncludeCRDs = true
	install.ReleaseName = releaseName(hr)
	install.Namespace = namespace
	install.APIVersions = apiVersions(req.Objects, chrt)

	released, err := install.Run(chrt, values)
	if err != nil {
		return nil, werror.Wrap(err, "failed to render chart %s", req.Chart.Upstream())
	}
	rel, ok := released.(*releasev1.Release)
	if !ok {
		return nil, fmt.Errorf("chart %s rendered an unexpected release type %T", req.Chart.Upstream(), released)
	}

	var manifest strings.Builder
	manifest.WriteString(rel.Manifest)
	testsEnabled, _ := nestedMap(hr, "spec", "test")["enable"].(bool)
	for _, hook := range rel.Hooks {
		if !testsEnabled && slices.Contains(hook.Events, releasev1.HookTest) {
			continue
		}
		manifest.WriteString("\n---\n")
		manifest.WriteString(hook.Manifest)
	}

	rendered := manifest.String()
	for _, pr := range nestedSlice(hr, "spec", "postRenderers") {
		postRenderer, _ := pr.(map[string]any)
		if k := nestedMap(postRenderer, "kustomize"); k != nil {
			if rendered, err = kustomizePostRender(rendered, k); err != nil {
				return nil, werror.Wrap(err, "failed to post-render chart %s", req.Chart.Upstream())
			}
		}
	}
	objects, err := decodeObjects(rendered)
	if err != nil {
		return nil, err
	}
	for _, obj := range objects {
		if nestedString(obj, "metadata", "namespace") != "" {
			continue
		}
		if md := nestedMap(obj, "metadata"); md != nil {
			md["namespace"] = namespace
		}
	}
	return objects, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// pull returns the path of the chart archive of chart in <project>/.windsor/cache/charts. It downloads
// the chart when the cache has no archive for its name, version and digest. An OCI chart pulls through the
// registry client, which reads the Docker config for credentials.
func (r *HelmRenderer) pull(chart Entry) (string, error) {
	cacheDir := filepath.Join(r.runtime.ProjectRoot, ".windsor", "cache", "charts", strings.NewReplacer("/", "_", ":", "_").Replace(chart.Reference))
	file := fmt.Sprintf("%s-%s.tgz", chart.Name, chart.Version)
	if chart.Digest != "" {
		file = fmt.Sprintf("%s-%s-%s.tgz", chart.Name, chart.Version, strings.ReplaceAll(chart.Digest, ":", "_"))
	}
	archive := filepath.Join(cacheDir, file)
	if _, err := r.shims.Stat(archive); err == nil {
		return archive, nil
	}
	if err := r.shims.MkdirAll(cacheDir, 0750); err != nil {
		return "", werror.Wrap(err, "failed to create the chart cache")
	}

	settings := cli.New()
	registryClient, err := registry.NewClient(registry.ClientOptWriter(io.Discard))
	if err != nil {
		return "", werror.Wrap(err, "failed to create a registry client")
	}
	d := downloader.ChartDownloader{
		Out:              io.Discard,
		Verify:           downloader.VerifyNever,
		Getters:          getter.All(settings),
		Options:          []getter.Option{getter.WithRegistryClient(registryClient)},
		RegistryClient:   registryClient,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		ContentCache:     cacheDir,
	}

	ref, version := "oci://"+chart.Reference+":"+chart.Version, ""
	if chart.Digest != "" {
		ref += "@" + chart.Digest
	}
	if chart.Transport == TransportHTTP {
		url, err := repo.FindChartInRepoURL(chart.Reference, chart.Name, getter.All(settings), repo.WithChartVersion(chart.Version))
		if err != nil {
			return "", werror.Wrap(err, "failed to find chart %s", chart.Upstream())
		}
		ref = url
	}

	saved, _, err := d.DownloadTo(ref, version, cacheDir)
	if err != nil {
		return "", werror.Wrap(err, "failed to pull chart %s", chart.Upstream())
	}
	if saved != archive {
		if err := r.shims.Rename(saved, archive); err != nil {
			return "", werror.Wrap(err, "failed to cache chart %s", chart.Upstream())
		}
	}
	return archive, nil
}

// =============================================================================
// Helpers
// =============================================================================

// releaseValues merges the valuesFrom references and spec.values of hr with chartutil, against a
// fake client that holds the ConfigMaps and Secrets in objects. Helm merges the chart defaults below them.
// A referenced ConfigMap or Secret that does not convert, such as a SOPS-encrypted Secret, is an error.
func releaseValues(hr map[string]any, objects []map[string]any, namespace string) (map[string]any, error) {
	var refs []meta.ValuesReference
	if raw := nestedSlice(hr, "spec", "valuesFrom"); len(raw) > 0 {
		data, err := sigsyaml.Marshal(raw)
		if err != nil {
			return nil, err
		}
		if err := sigsyaml.Unmarshal(data, &refs); err != nil {
			return nil, werror.Wrap(err, "failed to read valuesFrom")
		}
	}
	values := nestedMap(hr, "spec", "values")

	var clientObjects []client.Object
	for _, obj := range objects {
		var typed client.Object
		switch obj["kind"] {
		case "ConfigMap":
			typed = &corev1.ConfigMap{}
		case "Secret":
			typed = &corev1.Secret{}
		default:
			continue
		}
		if err := k8sruntime.DefaultUnstructuredConverter.FromUnstructured(obj, typed); err != nil {
			if referenced(refs, obj, namespace) {
				return nil, werror.Wrap(err, "failed to read %s %s/%s for the values of HelmRelease %s/%s", obj["kind"], namespace, nestedString(obj, "metadata", "name"), namespace, nestedString(hr, "metadata", "name"))
			}
			continue
		}
		if secret, ok := typed.(*corev1.Secret); ok && len(secret.StringData) > 0 {
			if secret.Data == nil {
				secret.Data = make(map[string][]byte)
			}
			for k, v := range secret.StringData {
				secret.Data[k] = []byte(v)
			}
		}
		clientObjects = append(clientObjects, typed)
	}
	scheme := k8sruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clientObjects...).Build()

	merged, err := chartutil.ChartValuesFromReferences(context.Background(), logr.Discard(), c, namespace, values, refs...)
	if err != nil {
		return nil, werror.Wrap(err, "failed to merge the values of HelmRelease %s/%s", namespace, nestedString(hr, "metadata", "name"))
	}
	return merged, nil
}

// referenced reports whether a valuesFrom entry in refs names obj in namespace.
func referenced(refs []meta.ValuesReference, obj map[string]any, namespace string) bool {
	if nestedString(obj, "metadata", "namespace") != namespace {
		return false
	}
	for _, ref := range refs {
		if ref.Kind == obj["kind"] && ref.Name == nestedString(obj, "metadata", "name") {
			return true
		}
	}
	return false
}

// releaseName returns spec.releaseName, or <targetNamespace>-<name> when spec.targetNamespace is
// set, or the HelmRelease name, as helm-controller names a release. A name over 53 characters
// becomes its first 40 characters, a dash and 12 characters of its SHA-256 sum, as helm-controller
// shortens it.
func releaseName(hr map[string]any) string {
	name := nestedString(hr, "spec", "releaseName")
	if name == "" {
		name = nestedString(hr, "metadata", "name")
		if target := nestedString(hr, "spec", "targetNamespace"); target != "" {
			name = target + "-" + name
		}
	}
	if len(name) <= 53 {
		return name
	}
	return name[:40] + "-" + fmt.Sprintf("%x", sha256.Sum256([]byte(name)))[:12]
}

// apiVersions returns group/version and group/version/Kind for each version that a
// CustomResourceDefinition in objects or in the crds/ directory of chrt serves.
func apiVersions(objects []map[string]any, chrt *chartv2.Chart) common.VersionSet {
	crds := make([]map[string]any, 0)
	for _, obj := range objects {
		if obj["kind"] == "CustomResourceDefinition" {
			crds = append(crds, obj)
		}
	}
	for _, file := range chrt.CRDObjects() {
		decoded, err := decodeObjects(string(file.File.Data))
		if err != nil {
			continue
		}
		crds = append(crds, decoded...)
	}

	var out common.VersionSet
	for _, crd := range crds {
		group := nestedString(crd, "spec", "group")
		kind := nestedString(crd, "spec", "names", "kind")
		for _, v := range nestedSlice(crd, "spec", "versions") {
			version, _ := v.(map[string]any)
			name, _ := version["name"].(string)
			if served, ok := version["served"].(bool); ok && !served {
				continue
			}
			out = append(out, group+"/"+name, group+"/"+name+"/"+kind)
		}
	}
	return out
}

// kustomizePostRender applies the patches and images of a Kustomize post-renderer to manifests.
func kustomizePostRender(manifests string, spec map[string]any) (string, error) {
	fs := filesys.MakeFsInMemory()
	if err := fs.WriteFile("/render/resources.yaml", []byte(manifests)); err != nil {
		return "", err
	}
	kustomization := map[string]any{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  []string{"resources.yaml"},
	}
	if patches := nestedSlice(spec, "patches"); len(patches) > 0 {
		kustomization["patches"] = patches
	}
	if images := nestedSlice(spec, "images"); len(images) > 0 {
		kustomization["images"] = images
	}
	data, err := sigsyaml.Marshal(kustomization)
	if err != nil {
		return "", err
	}
	if err := fs.WriteFile("/render/kustomization.yaml", data); err != nil {
		return "", err
	}
	resMap, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(fs, "/render")
	if err != nil {
		return "", err
	}
	out, err := resMap.AsYaml()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// nestedSlice returns the slice at path in obj, or nil.
func nestedSlice(obj map[string]any, path ...string) []any {
	if len(path) == 0 {
		return nil
	}
	parent := nestedMap(obj, path[:len(path)-1]...)
	if parent == nil {
		return nil
	}
	s, _ := parent[path[len(path)-1]].([]any)
	return s
}
