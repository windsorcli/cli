// The version methods of the Provisioner manage the blueprint version marker on the cluster.
// It provides WriteVersionMarker, BeginVersionTransition, GetVersionMarker, and CheckVersionGate.
// CheckVersionGate compares the marker with the source refs of the blueprint.

package provisioner

import (
	"fmt"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Types
// =============================================================================

// VersionGate describes how the blueprint a command is about to apply relates to the version marker
// recorded in the cluster. It is the input to apply's version-equality seam: apply may reconcile in
// place only when a settled marker matches the blueprint it would apply. Any other state — a version
// mismatch or an in-flight transition — belongs to upgrade. The caller decides policy (proceed,
// refuse, or honor --force); this struct only reports the relation.
type VersionGate struct {
	MarkerFound  bool // a marker exists for this context (false = pre-bootstrap, no cluster, or legacy)
	InFlight     bool // the marker phase is not idle (an upgrade is in flight)
	VersionMatch bool // the blueprint's resolved source-ref set equals the applied set
}

// =============================================================================
// Public Methods
// =============================================================================

// WriteVersionMarker records the blueprint version applied to this context as a marker ConfigMap
// in the gitops namespace, capturing the resolved reference of each applied source. Only bootstrap
// and upgrade write the marker; apply and plan only read it, so the marker stays an authoritative
// record of what was deliberately rolled out rather than drifting with every reconcile.
func (i *Provisioner) WriteVersionMarker(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	marker, err := kubernetes.BuildVersionMarker(blueprint)
	if err != nil {
		return fmt.Errorf("failed to build version marker: %w", err)
	}
	if err := i.KubernetesManager.ApplyVersionMarker(i.fluxNamespace(), marker); err != nil {
		return fmt.Errorf("failed to write version marker: %w", err)
	}

	return nil
}

// BeginVersionTransition writes the in-flight marker for an upgrade toward blueprint: it preserves
// the applied source set from the existing marker (empty for a legacy context) and records the
// blueprint's set as the target under the upgrading phase. apply's version gate refuses while the
// marker is non-idle; WriteVersionMarker settles it to idle on success.
func (i *Provisioner) BeginVersionTransition(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	current, found, err := i.GetVersionMarker()
	if err != nil {
		return fmt.Errorf("failed to read current version marker: %w", err)
	}
	var applied map[string]kubernetes.SourceRef
	if found {
		applied = current.AppliedSources
	}

	marker, err := kubernetes.BuildTransitionMarker(applied, blueprint)
	if err != nil {
		return fmt.Errorf("failed to build transition marker: %w", err)
	}
	if err := i.KubernetesManager.ApplyVersionMarker(i.fluxNamespace(), marker); err != nil {
		return fmt.Errorf("failed to write transition marker: %w", err)
	}
	return nil
}

// GetVersionMarker reads the applied-version marker for this context's gitops namespace, reporting
// false when no marker exists (a pre-bootstrap, no-cluster, or legacy context). apply and plan read
// the marker to gate on the blueprint version; only bootstrap and upgrade write it. Returns false
// without consulting the cluster when no context-scoped kubeconfig is present — there is no cluster,
// so nothing is applied and there is no version to read.
func (i *Provisioner) GetVersionMarker() (kubernetes.VersionMarker, bool, error) {
	if i.KubernetesManager == nil {
		return kubernetes.VersionMarker{}, false, fmt.Errorf("kubernetes manager not configured")
	}
	if !i.kubeconfigPresent() {
		return kubernetes.VersionMarker{}, false, nil
	}
	return i.KubernetesManager.GetVersionMarker(i.fluxNamespace())
}

// CheckVersionGate reads the version marker and reports how the given blueprint relates to it,
// without deciding policy. A missing marker (pre-bootstrap, no cluster, or legacy) yields
// MarkerFound=false so callers treat it as "nothing applied yet — proceed". When a marker exists,
// InFlight reflects a non-idle phase and VersionMatch compares the blueprint's resolved source-ref
// set against the applied set. Returns an error only on a real read/decode failure or when the
// blueprint's sources cannot be reduced to an unambiguous set; callers may treat a read failure as
// best-effort (cluster unreachable) rather than a hard stop.
func (i *Provisioner) CheckVersionGate(blueprint *blueprintv1alpha1.Blueprint) (VersionGate, error) {
	if blueprint == nil {
		return VersionGate{}, fmt.Errorf("blueprint not provided")
	}

	marker, found, err := i.GetVersionMarker()
	if err != nil {
		return VersionGate{}, err
	}
	if !found {
		return VersionGate{MarkerFound: false}, nil
	}

	target, err := kubernetes.BuildVersionMarker(blueprint)
	if err != nil {
		return VersionGate{}, fmt.Errorf("failed to derive blueprint version: %w", err)
	}

	return VersionGate{
		MarkerFound:  true,
		InFlight:     marker.Phase != "" && marker.Phase != kubernetes.VersionMarkerPhaseIdle,
		VersionMatch: kubernetes.SourcesEqual(marker.AppliedSources, target.AppliedSources),
	}, nil
}
