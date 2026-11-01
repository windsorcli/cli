package kubernetes_test

import (
	"encoding/json"
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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
		marker, err := kubernetes.BuildVersionMarker(blueprint)

		// Then it is idle, schema-versioned, and records the repository and remote source only
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if marker.Phase != kubernetes.VersionMarkerPhaseIdle {
			t.Errorf("Expected phase %q, got %q", kubernetes.VersionMarkerPhaseIdle, marker.Phase)
		}
		if marker.SchemaVersion != 1 {
			t.Errorf("Expected schema version %d, got %d", 1, marker.SchemaVersion)
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
		marker, err := kubernetes.BuildVersionMarker(blueprint)

		// Then the commit wins over the tag
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
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
		_, err := kubernetes.BuildVersionMarker(blueprint)

		// Then it returns an error
		if err == nil {
			t.Error("Expected an error on a duplicate source name")
		}
	})

	t.Run("NilBlueprintYieldsIdleMarkerWithNoSources", func(t *testing.T) {
		// Given a nil blueprint
		var blueprint *blueprintv1alpha1.Blueprint

		// When the marker is built
		marker, err := kubernetes.BuildVersionMarker(blueprint)

		// Then it is idle with an empty source set
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if marker.Phase != kubernetes.VersionMarkerPhaseIdle {
			t.Errorf("Expected idle phase, got %q", marker.Phase)
		}
		if len(marker.AppliedSources) != 0 {
			t.Errorf("Expected no sources, got %d", len(marker.AppliedSources))
		}
	})
}

func TestVersionMarker_ConfigMapRoundTrip(t *testing.T) {
	t.Run("EncodesAndDecodesToTheSameMarker", func(t *testing.T) {
		// Given an in-flight marker carrying both applied and target source sets
		original := kubernetes.VersionMarker{
			SchemaVersion:  1,
			Phase:          kubernetes.VersionMarkerPhaseUpgrading,
			AppliedSources: map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
			TargetSources:  map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v2.0.0"}},
		}

		// When it is encoded to ConfigMap data and parsed back
		data, encodeErr := original.ToConfigMapData()
		parsed, ok, parseErr := kubernetes.ParseVersionMarker(data)

		// Then the round-trip preserves the phase and both source sets
		if encodeErr != nil {
			t.Fatalf("Expected no encode error, got %v", encodeErr)
		}
		if parseErr != nil {
			t.Fatalf("Expected no parse error, got %v", parseErr)
		}
		if !ok {
			t.Fatal("Expected marker to be present")
		}
		if parsed.Phase != original.Phase || parsed.AppliedSources["core"] != original.AppliedSources["core"] || parsed.TargetSources["core"] != original.TargetSources["core"] {
			t.Errorf("Round-trip mismatch: got %+v, want %+v", parsed, original)
		}
	})

	t.Run("ParseReportsAbsentWhenNoMarkerKey", func(t *testing.T) {
		// Given ConfigMap data without the marker key
		data := map[string]string{"other": "value"}

		// When it is parsed
		_, ok, err := kubernetes.ParseVersionMarker(data)

		// Then it reports absent without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ok {
			t.Error("Expected ok=false for data without a marker key")
		}
	})

	t.Run("RejectsUnknownSchemaVersion", func(t *testing.T) {
		// Given ConfigMap data holding a marker with an unknown schema version
		data := map[string]string{"marker": `{"schemaVersion":999,"phase":"idle"}`}

		// When it is parsed
		_, _, err := kubernetes.ParseVersionMarker(data)

		// Then it returns an error
		if err == nil {
			t.Error("Expected an error for an unsupported schema version")
		}
	})
}

func TestBuildTransitionMarker(t *testing.T) {
	t.Run("PreservesAppliedAndRecordsTargetUnderUpgradingPhase", func(t *testing.T) {
		// Given the applied source set and a blueprint pinned to a newer ref
		applied := map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}}
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "bp"},
			Sources:  []blueprintv1alpha1.Source{{Name: "core", Url: "oci://example/core", Ref: blueprintv1alpha1.Reference{SemVer: "v2.0.0"}}},
		}

		// When the transition marker is built
		marker, err := kubernetes.BuildTransitionMarker(applied, blueprint)

		// Then it is upgrading, keeps the applied ref, and records the target ref
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if marker.Phase != kubernetes.VersionMarkerPhaseUpgrading {
			t.Errorf("Expected phase %q, got %q", kubernetes.VersionMarkerPhaseUpgrading, marker.Phase)
		}
		if marker.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("Expected applied ref 'v1.0.0', got %q", marker.AppliedSources["core"].Ref)
		}
		if marker.TargetSources["core"].Ref != "v2.0.0" {
			t.Errorf("Expected target ref 'v2.0.0', got %q", marker.TargetSources["core"].Ref)
		}
	})

	t.Run("PropagatesBlueprintReductionError", func(t *testing.T) {
		// Given a blueprint whose repository name collides with a declared source
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "core"},
			Repository: blueprintv1alpha1.Repository{Url: "http://git.test/core", Ref: blueprintv1alpha1.Reference{Branch: "main"}},
			Sources:    []blueprintv1alpha1.Source{{Name: "core", Url: "oci://example/core", Ref: blueprintv1alpha1.Reference{SemVer: "v2.0.0"}}},
		}

		// When the transition marker is built
		_, err := kubernetes.BuildTransitionMarker(nil, blueprint)

		// Then the reduction error is returned
		if err == nil {
			t.Error("Expected an error when the blueprint cannot be reduced to an unambiguous set")
		}
	})
}

func TestSourcesEqual(t *testing.T) {
	base := map[string]kubernetes.SourceRef{"core": {URL: "u", Ref: "v1"}}

	t.Run("EqualForIdenticalSets", func(t *testing.T) {
		// Given a set identical to base
		other := map[string]kubernetes.SourceRef{"core": {URL: "u", Ref: "v1"}}

		// When the sets are compared
		equal := kubernetes.SourcesEqual(base, other)

		// Then they are equal
		if !equal {
			t.Error("Expected identical sets to be equal")
		}
	})

	t.Run("UnequalWhenRefDiffers", func(t *testing.T) {
		// Given a set whose ref differs from base
		other := map[string]kubernetes.SourceRef{"core": {URL: "u", Ref: "v2"}}

		// When the sets are compared
		equal := kubernetes.SourcesEqual(base, other)

		// Then they are unequal
		if equal {
			t.Error("Expected differing refs to be unequal")
		}
	})

	t.Run("UnequalWhenKeyMissing", func(t *testing.T) {
		// Given a set with a different key than base
		other := map[string]kubernetes.SourceRef{"other": {URL: "u", Ref: "v1"}}

		// When the sets are compared
		equal := kubernetes.SourcesEqual(base, other)

		// Then they are unequal
		if equal {
			t.Error("Expected sets with different keys to be unequal")
		}
	})

	t.Run("UnequalWhenLengthDiffers", func(t *testing.T) {
		// Given a set with one more entry than base
		other := map[string]kubernetes.SourceRef{"core": {URL: "u", Ref: "v1"}, "extra": {URL: "x", Ref: "v1"}}

		// When the sets are compared
		equal := kubernetes.SourcesEqual(base, other)

		// Then they are unequal
		if equal {
			t.Error("Expected sets of different sizes to be unequal")
		}
	})
}

func TestBaseKubernetesManager_GetVersionMarker(t *testing.T) {
	// setup returns a manager whose client answers GetResource with getResource.
	setup := func(t *testing.T, getResource func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error)) *kubernetes.BaseKubernetesManager {
		t.Helper()
		mocks := setupKubernetesMocks(t)
		c := client.NewMockKubernetesClient()
		c.GetResourceFunc = getResource
		return kubernetes.NewKubernetesManager(c, mocks.ConfigHandler)
	}

	t.Run("ReturnsParsedMarkerWhenPresent", func(t *testing.T) {
		// Given a marker ConfigMap present in the gitops namespace
		marker := kubernetes.VersionMarker{
			SchemaVersion:  1,
			Phase:          kubernetes.VersionMarkerPhaseIdle,
			AppliedSources: map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
		}
		data, err := marker.ToConfigMapData()
		if err != nil {
			t.Fatalf("encode marker: %v", err)
		}
		manager := setup(t, func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
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
		})

		// When the marker is read
		got, found, err := manager.GetVersionMarker("system-gitops")

		// Then it is found and decoded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected marker to be found")
		}
		if got.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("Expected core ref 'v1.0.0', got %q", got.AppliedSources["core"].Ref)
		}
	})

	t.Run("ReportsAbsentWhenConfigMapNotFound", func(t *testing.T) {
		// Given no marker ConfigMap
		manager := setup(t, func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("configmaps %q not found", name)
		})

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
		// Given a marker ConfigMap that carries no data
		manager := setup(t, func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": name, "namespace": ns},
			}}, nil
		})

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
		manager := setup(t, func(gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("connection refused")
		})

		// When the marker is read
		_, _, err := manager.GetVersionMarker("system-gitops")

		// Then it returns an error
		if err == nil {
			t.Error("Expected an error when the marker read fails")
		}
	})
}

func TestBaseKubernetesManager_ApplyVersionMarker(t *testing.T) {
	t.Run("WritesMarkerConfigMapWithEncodedMarker", func(t *testing.T) {
		// Given a manager capturing applied ConfigMaps
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
		manager := newManager(setupKubernetesMocks(t), kubernetesClient)
		manager.Shims.ToUnstructured = func(obj any) (map[string]any, error) {
			return runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		}
		manager.Shims.FromUnstructured = func(obj map[string]any, target any) error {
			return runtime.DefaultUnstructuredConverter.FromUnstructured(obj, target)
		}
		marker := kubernetes.VersionMarker{
			SchemaVersion:  1,
			Phase:          kubernetes.VersionMarkerPhaseIdle,
			AppliedSources: map[string]kubernetes.SourceRef{"core": {URL: "oci://example/core", Ref: "v1.0.0"}},
		}

		// When the marker is applied
		err := manager.ApplyVersionMarker("system-gitops", marker)

		// Then the marker ConfigMap is written with the encoded marker
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if appliedName != kubernetes.VersionMarkerConfigMapName {
			t.Errorf("Expected ConfigMap name %q, got %q", kubernetes.VersionMarkerConfigMapName, appliedName)
		}
		if appliedNamespace != "system-gitops" {
			t.Errorf("Expected namespace 'system-gitops', got %q", appliedNamespace)
		}
		var decoded kubernetes.VersionMarker
		if err := json.Unmarshal([]byte(appliedData["marker"]), &decoded); err != nil {
			t.Fatalf("Expected marker data to be valid JSON, got error %v", err)
		}
		if decoded.AppliedSources["core"].Ref != "v1.0.0" {
			t.Errorf("Expected applied marker to record core ref 'v1.0.0', got %q", decoded.AppliedSources["core"].Ref)
		}
	})
}
