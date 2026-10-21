package provisioner

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_WriteVersionMarker(t *testing.T) {
	t.Run("WritesMarkerToGitopsNamespaceFromBlueprintSources", func(t *testing.T) {
		// Given a provisioner capturing the version marker write
		mocks := setupProvisionerMocks(t)
		var capturedNamespace string
		var capturedMarker kubernetes.VersionMarker
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			capturedNamespace = namespace
			capturedMarker = marker
			return nil
		}
		opts := &Provisioner{KubernetesManager: mocks.KubernetesManager}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When writing the marker for a blueprint with one remote source
		if err := provisioner.WriteVersionMarker(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is written to the gitops namespace recording the source's resolved ref
		if capturedNamespace != provisioner.fluxNamespace() {
			t.Errorf("Expected namespace %q, got %q", provisioner.fluxNamespace(), capturedNamespace)
		}
		if capturedMarker.AppliedSources["source1"].Ref != "main" {
			t.Errorf("Expected source1 ref 'main', got %q", capturedMarker.AppliedSources["source1"].Ref)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		if err := provisioner.WriteVersionMarker(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		provisioner.KubernetesManager = nil
		if err := provisioner.WriteVersionMarker(createTestBlueprint()); err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
	})
}

func TestProvisioner_BeginVersionTransition(t *testing.T) {
	t.Run("PreservesAppliedAndRecordsTargetUnderUpgradingPhase", func(t *testing.T) {
		// Given a context already running an older version
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{
				Phase:          kubernetes.VersionMarkerPhaseIdle,
				AppliedSources: map[string]kubernetes.SourceRef{"source1": {URL: "old", Ref: "v0"}},
			}, true, nil
		}
		var captured kubernetes.VersionMarker
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			captured = marker
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition toward the blueprint
		if err := provisioner.BeginVersionTransition(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the written marker is in-flight, preserves the old applied set, and records the new target
		if captured.Phase != kubernetes.VersionMarkerPhaseUpgrading {
			t.Errorf("Expected phase %q, got %q", kubernetes.VersionMarkerPhaseUpgrading, captured.Phase)
		}
		if captured.AppliedSources["source1"].Ref != "v0" {
			t.Errorf("Expected applied ref preserved as 'v0', got %q", captured.AppliedSources["source1"].Ref)
		}
		target, _ := kubernetes.BuildVersionMarker(createTestBlueprint())
		if !kubernetes.SourcesEqual(captured.TargetSources, target.AppliedSources) {
			t.Errorf("Expected target to equal the blueprint source set, got %+v", captured.TargetSources)
		}
	})

	t.Run("EmptyAppliedForLegacyContext", func(t *testing.T) {
		// Given a legacy/first-upgrade context with no recorded marker
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, nil
		}
		var captured kubernetes.VersionMarker
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			captured = marker
			return nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition
		if err := provisioner.BeginVersionTransition(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then there is no applied set to preserve, but the target is still recorded
		if len(captured.AppliedSources) != 0 {
			t.Errorf("Expected empty applied set for a legacy context, got %+v", captured.AppliedSources)
		}
		if len(captured.TargetSources) == 0 {
			t.Error("Expected the target set to be recorded")
		}
	})

	t.Run("ReadFailurePropagates", func(t *testing.T) {
		// Given a marker read that fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("connection refused")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition, the read error surfaces rather than writing a wrong marker
		if err := provisioner.BeginVersionTransition(createTestBlueprint()); err == nil {
			t.Error("Expected an error when the current marker cannot be read")
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		if err := provisioner.BeginVersionTransition(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		provisioner.KubernetesManager = nil
		if err := provisioner.BeginVersionTransition(createTestBlueprint()); err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
	})
}

func TestProvisioner_GetVersionMarker(t *testing.T) {
	t.Run("DelegatesToManagerWhenKubeconfigPresent", func(t *testing.T) {
		// Given a manager that returns a marker and a context with a kubeconfig
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{Phase: kubernetes.VersionMarkerPhaseIdle}, true, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the marker is read
		_, found, err := provisioner.GetVersionMarker()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Error("Expected the marker to be found")
		}
	})

	t.Run("ReturnsAbsentWithoutClusterWhenNoKubeconfig", func(t *testing.T) {
		// Given a context whose config root has no kubeconfig (no cluster)
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()
		called := false
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			called = true
			return kubernetes.VersionMarker{}, false, fmt.Errorf("should not be called")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the marker is read
		_, found, err := provisioner.GetVersionMarker()

		// Then it reports absent without consulting the cluster
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if found {
			t.Error("Expected found=false when no kubeconfig is present")
		}
		if called {
			t.Error("Expected the cluster not to be consulted when no kubeconfig is present")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		provisioner.KubernetesManager = nil
		if _, _, err := provisioner.GetVersionMarker(); err == nil {
			t.Error("Expected error for nil kubernetes manager")
		}
	})
}

func TestProvisioner_CheckVersionGate(t *testing.T) {
	// matchingMarker returns a settled marker whose source set equals the test blueprint's.
	matchingMarker := func() kubernetes.VersionMarker {
		target, err := kubernetes.BuildVersionMarker(createTestBlueprint())
		if err != nil {
			t.Fatalf("build target marker: %v", err)
		}
		return target
	}

	t.Run("MarkerNotFoundIsTreatedAsLegacy", func(t *testing.T) {
		// Given no marker recorded for the context
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := provisioner.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports no marker — the caller proceeds as a legacy/pre-bootstrap context
		if gate.MarkerFound {
			t.Error("Expected MarkerFound=false when no marker is recorded")
		}
	})

	t.Run("VersionMatchesWhenSourceSetEqualsApplied", func(t *testing.T) {
		// Given a settled marker whose source set matches the blueprint
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return matchingMarker(), true, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := provisioner.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports a settled, matching version
		if !gate.MarkerFound || gate.InFlight || !gate.VersionMatch {
			t.Errorf("Expected found+match+settled, got %+v", gate)
		}
	})

	t.Run("VersionDiffersWhenSourceRefChanged", func(t *testing.T) {
		// Given a marker whose source ref differs from the blueprint
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			marker := matchingMarker()
			for name, ref := range marker.AppliedSources {
				ref.Ref = "different"
				marker.AppliedSources[name] = ref
			}
			return marker, true, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := provisioner.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the version does not match — apply must redirect to upgrade
		if !gate.MarkerFound || gate.VersionMatch {
			t.Errorf("Expected found with VersionMatch=false, got %+v", gate)
		}
	})

	t.Run("InFlightWhenMarkerPhaseNotIdle", func(t *testing.T) {
		// Given a marker mid-transition
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			marker := matchingMarker()
			marker.Phase = "migrating"
			return marker, true, nil
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := provisioner.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports an in-flight transition
		if !gate.InFlight {
			t.Errorf("Expected InFlight=true for a non-idle phase, got %+v", gate)
		}
	})

	t.Run("NoKubeconfigShortCircuitsToLegacy", func(t *testing.T) {
		// Given a context with no kubeconfig (no cluster)
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("should not be called")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked, it short-circuits to "nothing applied"
		gate, err := provisioner.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if gate.MarkerFound {
			t.Error("Expected MarkerFound=false with no kubeconfig")
		}
	})

	t.Run("ReadFailurePropagates", func(t *testing.T) {
		// Given a marker read that fails (cluster unreachable)
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("connection refused")
		}
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked, the error surfaces for the caller to treat as best-effort
		if _, err := provisioner.CheckVersionGate(createTestBlueprint()); err == nil {
			t.Error("Expected an error when the marker read fails")
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		mocks := setupProvisionerMocks(t)
		provisioner := NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &Provisioner{KubernetesManager: mocks.KubernetesManager})
		if _, err := provisioner.CheckVersionGate(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})
}
