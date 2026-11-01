package provisioner_test

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner"

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
		opts := &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, opts)

		// When writing the marker for a blueprint with one remote source
		if err := prov.WriteVersionMarker(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it is written to the gitops namespace recording the source's resolved ref
		if capturedNamespace != constants.DefaultGitopsNamespace {
			t.Errorf("Expected namespace %q, got %q", constants.DefaultGitopsNamespace, capturedNamespace)
		}
		if capturedMarker.AppliedSources["source1"].Ref != "main" {
			t.Errorf("Expected source1 ref 'main', got %q", capturedMarker.AppliedSources["source1"].Ref)
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		// When writing the marker for a nil blueprint
		// Then an error is returned
		if err := prov.WriteVersionMarker(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager is nil
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		prov.KubernetesManager = nil
		// When writing the marker
		// Then an error is returned
		if err := prov.WriteVersionMarker(createTestBlueprint()); err == nil {
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition toward the blueprint
		if err := prov.BeginVersionTransition(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the marker is upgrading, keeps the old applied set, and records the target
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
		// Given a legacy context with no recorded marker
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, nil
		}
		var captured kubernetes.VersionMarker
		mocks.KubernetesManager.ApplyVersionMarkerFunc = func(namespace string, marker kubernetes.VersionMarker) error {
			captured = marker
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition
		if err := prov.BeginVersionTransition(createTestBlueprint()); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the applied set is empty and the target is recorded
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When beginning a transition
		// Then the read error is returned
		if err := prov.BeginVersionTransition(createTestBlueprint()); err == nil {
			t.Error("Expected an error when the current marker cannot be read")
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		// When beginning a transition for a nil blueprint
		// Then an error is returned
		if err := prov.BeginVersionTransition(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("NilManagerReturnsError", func(t *testing.T) {
		// Given a provisioner whose kubernetes manager is nil
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		prov.KubernetesManager = nil
		// When beginning a transition
		// Then an error is returned
		if err := prov.BeginVersionTransition(createTestBlueprint()); err == nil {
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the marker is read
		_, found, err := prov.GetVersionMarker()

		// Then the marker is found without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Error("Expected the marker to be found")
		}
	})

	t.Run("ReturnsAbsentWithoutClusterWhenNoKubeconfig", func(t *testing.T) {
		// Given a context whose config root has no kubeconfig
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()
		called := false
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			called = true
			return kubernetes.VersionMarker{}, false, fmt.Errorf("should not be called")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the marker is read
		_, found, err := prov.GetVersionMarker()

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
		// Given a provisioner whose kubernetes manager is nil
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		prov.KubernetesManager = nil
		// When the marker is read
		// Then an error is returned
		if _, _, err := prov.GetVersionMarker(); err == nil {
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := prov.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports that no marker is found
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := prov.CheckVersionGate(createTestBlueprint())
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := prov.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports that the version does not match
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
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := prov.CheckVersionGate(createTestBlueprint())
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then it reports an in-flight transition
		if !gate.InFlight {
			t.Errorf("Expected InFlight=true for a non-idle phase, got %+v", gate)
		}
	})

	t.Run("NoKubeconfigShortCircuitsToLegacy", func(t *testing.T) {
		// Given a context with no kubeconfig
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.ConfigRoot = t.TempDir()
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("should not be called")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		gate, err := prov.CheckVersionGate(createTestBlueprint())

		// Then it reports that no marker is found
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if gate.MarkerFound {
			t.Error("Expected MarkerFound=false with no kubeconfig")
		}
	})

	t.Run("ReadFailurePropagates", func(t *testing.T) {
		// Given a marker read that fails because the cluster is unreachable
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetVersionMarkerFunc = func(namespace string) (kubernetes.VersionMarker, bool, error) {
			return kubernetes.VersionMarker{}, false, fmt.Errorf("connection refused")
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})

		// When the gate is checked
		// Then the read error is returned
		if _, err := prov.CheckVersionGate(createTestBlueprint()); err == nil {
			t.Error("Expected an error when the marker read fails")
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a provisioner with a kubernetes manager
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
		// When the gate is checked for a nil blueprint
		// Then an error is returned
		if _, err := prov.CheckVersionGate(nil); err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})
}
