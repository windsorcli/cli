package kubernetes

import (
	"encoding/json"
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBuildVersionMarker(t *testing.T) {
	t.Run("CapturesRepositoryAndRemoteSourcesWithResolvedRefs", func(t *testing.T) {
		// Given a blueprint with a repository, a remote source, and a local template source
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "local"},
			Repository: blueprintv1alpha1.Repository{Url: "http://git.test/git/core", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.6.0", Ref: blueprintv1alpha1.Reference{SemVer: "v0.6.0"}},
				{Name: "template"},
			},
		}

		// When the marker is built
		marker, err := BuildVersionMarker(blueprint)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is idle, schema-versioned, and records the repository and remote source (not the local template)
		if marker.Phase != VersionMarkerPhaseIdle {
			t.Errorf("Expected phase %q, got %q", VersionMarkerPhaseIdle, marker.Phase)
		}
		if marker.SchemaVersion != versionMarkerSchemaVersion {
			t.Errorf("Expected schema version %d, got %d", versionMarkerSchemaVersion, marker.SchemaVersion)
		}
		repo, ok := marker.AppliedSources["local"]
		if !ok || repo.URL != "http://git.test/git/core" || repo.Ref != "main" {
			t.Errorf("Expected repository source 'local' url+ref, got %+v (present=%v)", repo, ok)
		}
		core, ok := marker.AppliedSources["core"]
		if !ok || core.Ref != "v0.6.0" {
			t.Errorf("Expected source 'core' ref 'v0.6.0', got %+v (present=%v)", core, ok)
		}
		if _, ok := marker.AppliedSources["template"]; ok {
			t.Error("Expected local template source to be skipped")
		}
	})

	t.Run("ResolvesRefInCommitSemverTagBranchPriority", func(t *testing.T) {
		// Given a source pinned by both a commit and a tag
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "bp"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "s", Url: "oci://example/s", Ref: blueprintv1alpha1.Reference{Tag: "v1", Commit: "abc123"}},
			},
		}

		// When the marker is built
		marker, err := BuildVersionMarker(blueprint)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the commit wins over the tag
		if marker.AppliedSources["s"].Ref != "abc123" {
			t.Errorf("Expected ref 'abc123' (commit priority), got %q", marker.AppliedSources["s"].Ref)
		}
	})

	t.Run("ErrorsOnDuplicateSourceName", func(t *testing.T) {
		// Given a blueprint whose repository name collides with a declared source name
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "core"},
			Repository: blueprintv1alpha1.Repository{Url: "http://git.test/git/core", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.6.0", Ref: blueprintv1alpha1.Reference{SemVer: "v0.6.0"}},
			},
		}

		// When the marker is built
		_, err := BuildVersionMarker(blueprint)

		// Then it errors rather than silently overwriting the colliding entry
		if err == nil {
			t.Error("Expected an error on a duplicate source name")
		}
	})

	t.Run("NilBlueprintYieldsIdleMarkerWithNoSources", func(t *testing.T) {
		// When the marker is built from a nil blueprint
		marker, err := BuildVersionMarker(nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is idle with an empty source set
		if marker.Phase != VersionMarkerPhaseIdle {
			t.Errorf("Expected idle phase, got %q", marker.Phase)
		}
		if len(marker.AppliedSources) != 0 {
			t.Errorf("Expected no sources, got %d", len(marker.AppliedSources))
		}
	})
}

func TestVersionMarker_ConfigMapRoundTrip(t *testing.T) {
	t.Run("EncodesAndDecodesToTheSameMarker", func(t *testing.T) {
		// Given a marker
		original := VersionMarker{
			SchemaVersion:  versionMarkerSchemaVersion,
			Phase:          VersionMarkerPhaseIdle,
			AppliedSources: map[string]SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
		}

		// When it is encoded to ConfigMap data and parsed back
		data, err := original.ToConfigMapData()
		if err != nil {
			t.Fatalf("Expected no encode error, got %v", err)
		}
		parsed, ok, err := ParseVersionMarker(data)
		if err != nil {
			t.Fatalf("Expected no parse error, got %v", err)
		}

		// Then the round-trip preserves the marker
		if !ok {
			t.Fatal("Expected marker to be present")
		}
		if parsed.Phase != original.Phase || parsed.AppliedSources["core"] != original.AppliedSources["core"] {
			t.Errorf("Round-trip mismatch: got %+v, want %+v", parsed, original)
		}
	})

	t.Run("ParseReportsAbsentWhenNoMarkerKey", func(t *testing.T) {
		// When parsing ConfigMap data without the marker key
		_, ok, err := ParseVersionMarker(map[string]string{"other": "value"})

		// Then it reports absent without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ok {
			t.Error("Expected ok=false for data without a marker key")
		}
	})

	t.Run("RejectsUnknownSchemaVersion", func(t *testing.T) {
		// Given ConfigMap data holding a marker written under a newer, unknown schema version
		data := map[string]string{versionMarkerDataKey: `{"schemaVersion":999,"phase":"idle"}`}

		// When parsing it
		_, _, err := ParseVersionMarker(data)

		// Then it errors rather than returning silently zero-valued fields from an unrecognized schema
		if err == nil {
			t.Error("Expected an error for an unsupported schema version")
		}
	})
}

func TestSourcesEqual(t *testing.T) {
	base := map[string]SourceRef{"core": {URL: "u", Ref: "v1"}}

	t.Run("EqualForIdenticalSets", func(t *testing.T) {
		if !SourcesEqual(base, map[string]SourceRef{"core": {URL: "u", Ref: "v1"}}) {
			t.Error("Expected identical sets to be equal")
		}
	})

	t.Run("UnequalWhenRefDiffers", func(t *testing.T) {
		if SourcesEqual(base, map[string]SourceRef{"core": {URL: "u", Ref: "v2"}}) {
			t.Error("Expected differing refs to be unequal")
		}
	})

	t.Run("UnequalWhenKeyMissing", func(t *testing.T) {
		if SourcesEqual(base, map[string]SourceRef{"other": {URL: "u", Ref: "v1"}}) {
			t.Error("Expected sets with different keys to be unequal")
		}
	})

	t.Run("UnequalWhenLengthDiffers", func(t *testing.T) {
		if SourcesEqual(base, map[string]SourceRef{"core": {URL: "u", Ref: "v1"}, "extra": {URL: "x", Ref: "v1"}}) {
			t.Error("Expected sets of different sizes to be unequal")
		}
	})
}

func TestBaseKubernetesManager_GetVersionMarker(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		return NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
	}

	t.Run("ReturnsParsedMarkerWhenPresent", func(t *testing.T) {
		// Given a marker ConfigMap present in the gitops namespace
		manager := setup(t)
		marker := VersionMarker{
			SchemaVersion:  versionMarkerSchemaVersion,
			Phase:          VersionMarkerPhaseIdle,
			AppliedSources: map[string]SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
		}
		data, err := marker.ToConfigMapData()
		if err != nil {
			t.Fatalf("encode marker: %v", err)
		}
		c := client.NewMockKubernetesClient()
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			d := map[string]any{}
			for k, v := range data {
				d[k] = v
			}
			return &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": name, "namespace": ns},
				"data":       d,
			}}, nil
		}
		manager.client = c

		// When the marker is read
		got, found, err := manager.GetVersionMarker("system-gitops")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is found and decoded
		if !found {
			t.Fatal("Expected marker to be found")
		}
		if got.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("Expected core ref 'v1.0.0', got %q", got.AppliedSources["core"].Ref)
		}
	})

	t.Run("ReportsAbsentWhenConfigMapNotFound", func(t *testing.T) {
		// Given no marker ConfigMap (pre-bootstrap context)
		manager := setup(t)
		c := client.NewMockKubernetesClient()
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("configmaps %q not found", name)
		}
		manager.client = c

		// When the marker is read
		_, found, err := manager.GetVersionMarker("system-gitops")

		// Then it reports absent without error
		if err != nil {
			t.Fatalf("Expected no error for a missing marker, got %v", err)
		}
		if found {
			t.Error("Expected found=false when the ConfigMap is absent")
		}
	})

	t.Run("ReportsAbsentWhenConfigMapHasNoData", func(t *testing.T) {
		// Given a marker ConfigMap that carries no data (legacy)
		manager := setup(t)
		c := client.NewMockKubernetesClient()
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": name, "namespace": ns},
			}}, nil
		}
		manager.client = c

		// When the marker is read
		_, found, err := manager.GetVersionMarker("system-gitops")

		// Then it reports absent without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if found {
			t.Error("Expected found=false when the ConfigMap carries no data")
		}
	})

	t.Run("ErrorsOnReadFailure", func(t *testing.T) {
		// Given a cluster that is unreachable
		manager := setup(t)
		c := client.NewMockKubernetesClient()
		c.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		}
		manager.client = c

		// When the marker is read, the failure surfaces rather than being mistaken for "absent"
		if _, _, err := manager.GetVersionMarker("system-gitops"); err == nil {
			t.Error("Expected an error when the marker read fails")
		}
	})
}

func TestBaseKubernetesManager_ApplyVersionMarker(t *testing.T) {
	setup := func(t *testing.T) *BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		manager := NewKubernetesManager(mocks.KubernetesClient, mocks.ConfigHandler)
		manager.shims = mocks.Shims
		manager.shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}
		return manager
	}

	t.Run("WritesMarkerConfigMapWithEncodedMarker", func(t *testing.T) {
		// Given a manager capturing applied ConfigMaps
		manager := setup(t)
		kubernetesClient := client.NewMockKubernetesClient()

		var appliedName, appliedNamespace string
		var appliedData map[string]string
		kubernetesClient.ApplyResourceFunc = func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.ApplyOptions) (*unstructured.Unstructured, error) {
			if gvr.Resource == "configmaps" {
				appliedName = obj.GetName()
				appliedNamespace = obj.GetNamespace()
				if d, ok := obj.Object["data"].(map[string]string); ok {
					appliedData = d
				}
			}
			return obj, nil
		}
		kubernetesClient.GetResourceFunc = func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("not found")
		}
		manager.client = kubernetesClient

		marker := VersionMarker{
			SchemaVersion:  versionMarkerSchemaVersion,
			Phase:          VersionMarkerPhaseIdle,
			AppliedSources: map[string]SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
		}

		// When the marker is applied
		if err := manager.ApplyVersionMarker("system-gitops", marker); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the well-known marker ConfigMap is written with the encoded marker
		if appliedName != VersionMarkerConfigMapName {
			t.Errorf("Expected ConfigMap name %q, got %q", VersionMarkerConfigMapName, appliedName)
		}
		if appliedNamespace != "system-gitops" {
			t.Errorf("Expected namespace 'system-gitops', got %q", appliedNamespace)
		}
		var decoded VersionMarker
		if err := json.Unmarshal([]byte(appliedData[versionMarkerDataKey]), &decoded); err != nil {
			t.Fatalf("Expected marker data to be valid JSON, got error %v", err)
		}
		if decoded.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("Expected applied marker to record core ref 'v1.0.0', got %q", decoded.AppliedSources["core"].Ref)
		}
	})
}
