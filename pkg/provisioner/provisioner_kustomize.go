// The kustomize methods of the Provisioner apply, install, prune, and uninstall flux kustomizations.
// It provides ApplyKustomize, Install, Prune, and Uninstall.
// It waits for the kustomizations to converge and reports the progress of each layer.
// It reconciles stalled kustomizations again and forces stalled HelmReleases to reconcile.

package provisioner

import (
	"context"
	"fmt"
	"slices"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
)

// =============================================================================
// Constants
// =============================================================================

// kustomizationNotFoundHint is the remediation for a kustomization name that is not in the blueprint.
const kustomizationNotFoundHint = "Use the name of a kustomization in the blueprint. Run windsor show kustomization to see the kustomizations."

// =============================================================================
// Types
// =============================================================================

// layerReporter receives per-kustomization start, progress, and completion while kustomizations install.
type layerReporter interface {
	Start(name string)
	Progress(name, text string)
	Done(name string)
}

// stepProgress is a layerReporter that reports progress on the operation in ctx and ignores Done.
type stepProgress struct {
	ctx context.Context
}

// =============================================================================
// Public Methods
// =============================================================================

// ApplyKustomize applies one kustomization to the cluster and places its secrets.
// It filters the blueprint to that kustomization and keeps the sources, so Flux creates the correct source.
// It resolves the secrets, applies the kustomization, places the secrets, and waits until it is Ready.
// It does not prune secrets. It returns an error if the blueprint is nil, the kustomization is destroyOnly,
// or a step fails. A kustomization that is not in the blueprint returns an error with a remediation hint.
func (i *Provisioner) ApplyKustomize(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	var found *blueprintv1alpha1.Kustomization
	for _, k := range blueprint.Kustomizations {
		if k.Name == componentID {
			kCopy := k
			found = &kCopy
			break
		}
	}
	if found == nil {
		return werror.New(fmt.Sprintf("kustomization %q not found in blueprint", componentID), kustomizationNotFoundHint, nil)
	}
	if found.DestroyOnly != nil && *found.DestroyOnly {
		return fmt.Errorf("kustomization %q is destroy-only and cannot be applied", componentID)
	}

	filtered := *blueprint
	filtered.Kustomizations = []blueprintv1alpha1.Kustomization{*found}

	resolvedSecrets, err := i.ResolveSecrets(&filtered)
	if err != nil {
		return werror.Wrap(err, "error resolving secrets")
	}

	var placeErr, waitErr error
	if err := presenter.Track(ctx, "kustomize:"+componentID, fmt.Sprintf("Applying kustomization %s", componentID), func(ctx context.Context) error {
		if err := i.KubernetesManager.ApplyBlueprint(&filtered, i.fluxNamespace()); err != nil {
			return err
		}
		_ = i.Notify(ctx, &filtered)
		if placeErr = i.PlaceSecrets(ctx, resolvedSecrets, &filtered, false); placeErr != nil {
			return placeErr
		}
		waitErr = i.KubernetesManager.WaitForKustomizations(ctx, &filtered, stepProgress{ctx})
		return waitErr
	}); err != nil {
		if placeErr != nil {
			return werror.Wrap(placeErr, "error placing secrets")
		}
		if waitErr != nil {
			return fmt.Errorf("%w: %w", ErrKustomizationsNotReady, waitErr)
		}
		return werror.Wrap(err, "failed to apply kustomization %s", componentID)
	}

	return nil
}

// ApplyKustomizeAll applies all non-destroyOnly kustomizations in the blueprint to the cluster and places
// their declared secrets. Delegates to Install with pruning off, since applying the full set is additive
// here — kustomization and secret pruning are separate, explicitly-flagged steps. Returns an error if the
// blueprint is nil, the kubernetes manager is not configured, or the apply, placement, or wait fails.
func (i *Provisioner) ApplyKustomizeAll(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	return i.Install(ctx, blueprint, false)
}

// Install applies the blueprint's kustomization layer and places its declared secrets as one unit, so
// every command that installs kustomizations also materializes their secrets rather than re-wiring that
// sequence itself. It first resolves the blueprint's declared Secrets to plaintext, failing before any
// cluster mutation on a misconfigured secret; then applies all blueprint resources in order — namespace,
// source repositories, and each kustomization — firing a best-effort flux webhook notification inside the
// same progress scope so flux reconciles immediately instead of at the next interval (notification
// failures never abort the install); then places the resolved secrets into the namespaces their owning
// kustomizations create, gating each on namespace creation rather than kustomization readiness so a
// consumer whose readiness depends on its secret cannot deadlock placement. prune reclaims CLI-placed
// secrets this context no longer declares, mirroring kustomization prune (on for upgrade and apply
// --prune, off otherwise). ctx is threaded into Notify and placement so a cancelled parent context
// (e.g. Ctrl+C) tears down promptly. The install is one "Applying kustomizations" operation with one
// child step per kustomization; secret placement shows as progress on the owning kustomization. A
// readiness wait then runs alongside a converge pass on the same steps, and its failure returns an
// error wrapping ErrKustomizationsNotReady. The blueprint must be provided. Returns an error if
// resolution, apply, placement, or the wait fails.
func (i *Provisioner) Install(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, prune bool) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	resolvedSecrets, err := i.ResolveSecrets(blueprint)
	if err != nil {
		return werror.Wrap(err, "error resolving secrets")
	}

	applied := withCrdLayer(blueprint)
	names := convergeNames(applied)

	return presenter.Track(ctx, "kustomize", "Applying kustomizations", func(ctx context.Context) (err error) {
		layers := presenter.NewGroup(ctx, "kustomization:")
		defer func() { layers.Close(err) }()

		if err := i.KubernetesManager.ApplyBlueprint(applied, i.fluxNamespace()); err != nil {
			return werror.Wrap(err, "failed to apply blueprint")
		}
		_ = i.Notify(ctx, applied)

		if err := i.placeSecrets(ctx, layers, resolvedSecrets, applied, prune); err != nil {
			return werror.Wrap(err, "error placing secrets")
		}

		convergeCtx, cancel := context.WithTimeout(ctx, constants.DefaultConvergeTimeout)
		defer cancel()
		converged := make(chan struct{})
		go func() {
			defer close(converged)
			i.converge(convergeCtx, layers, applied, names, i.pollInterval())
		}()
		waitErr := i.KubernetesManager.WaitForKustomizations(ctx, applied, layers)
		cancel()
		<-converged
		if waitErr != nil {
			return fmt.Errorf("%w: %w", ErrKustomizationsNotReady, waitErr)
		}
		return nil
	})
}

// Prune removes Kustomizations belonging to this context that are no longer present in the
// blueprint, leaving every still-declared kustomization (platform and user) and any other
// context's kustomizations untouched. It prepares the blueprint with the synthesized CRD layers —
// matching what Install applied — so those layers are recognized as desired and not pruned.
func (i *Provisioner) Prune(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	if err := i.KubernetesManager.PruneBlueprint(ctx, blueprint, i.fluxNamespace()); err != nil {
		return werror.Wrap(err, "failed to prune blueprint")
	}

	return nil
}

// PrunableKustomizations returns the names of this context's Kustomizations that the blueprint no
// longer declares — exactly what Prune would delete. It is the read-only input to plan's prune
// preview and upgrade's confirmation gate; it deletes nothing. It prepares the blueprint with the
// synthesized CRD layers so the desired set matches what Prune deletes against.
func (i *Provisioner) PrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	if i.KubernetesManager == nil {
		return nil, fmt.Errorf("kubernetes manager not configured")
	}

	blueprint = withCrdLayer(blueprint)

	names, err := i.KubernetesManager.ListPrunableKustomizations(blueprint, i.fluxNamespace())
	if err != nil {
		return nil, werror.Wrap(err, "failed to list prunable kustomizations")
	}
	return names, nil
}

// Uninstall orchestrates the high-level kustomization teardown process from the blueprint.
// It initializes the kubernetes manager and deletes all blueprint kustomizations, including the
// synthesized CRD layer (withCrdLayer) so its Flux Kustomization objects are removed symmetrically
// with apply; the layer keeps Prune disabled, so the vendored CRDs themselves are retained.
// DeleteBlueprint emits its own per-Kustomization progress (Start/Done/Fail spinners),
// so this method does not wrap the call in WithProgress — doing so would suppress the
// inner per-Kustomization output and produce a single opaque "Removing blueprint
// resources" line that hides the long per-Kustomization waits inherent to
// WaitForTermination-driven teardown.
//
// Uninstall is a no-op when no local kubeconfig exists. See kubeconfigPresent for the reasoning:
// a missing kubeconfig means the cluster is already gone, so there is nothing to suspend or
// delete, and building a client would only fail with a stat error.
func (i *Provisioner) Uninstall(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if i.KubernetesManager == nil {
		return fmt.Errorf("kubernetes manager not configured")
	}

	if !i.kubeconfigPresent() {
		return nil
	}

	blueprint = withCrdLayer(blueprint)

	if err := i.KubernetesManager.DeleteBlueprint(ctx, blueprint, i.fluxNamespace()); err != nil {
		return werror.Wrap(err, "failed to delete blueprint")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// converge polls readiness until every name is Ready or ctx ends, nudging the frontier each round.
// It ends each name's step in layers as it becomes Ready.
func (i *Provisioner) converge(ctx context.Context, layers *presenter.Group, bp *blueprintv1alpha1.Blueprint, names []string, pollInterval time.Duration) {
	nudged := make(map[string]struct{})
	for {
		notReady, reconciling := i.nudgeFrontier(ctx, bp, nudged)
		trackLayers(layers, names, notReady, reconciling)
		if len(notReady) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// nudgeFrontier reconciles the blueprint's "frontier" kustomizations: those not yet Ready whose
// dependencies are all Ready. It also force-reconciles any stalled HelmRelease they own. Each
// frontier member is nudged once per not-Ready streak, tracked in nudged, so repeat calls do not
// re-poke it. It returns the not-Ready names, and the subset whose dependencies are all Ready,
// which are the ones Flux is reconciling. On a readiness read error, every kustomization is
// reported not-ready, none as reconciling, and nothing is nudged.
func (i *Provisioner) nudgeFrontier(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, nudged map[string]struct{}) (notReady, reconciling []string) {
	if blueprint == nil {
		return nil, nil
	}
	names := convergeNames(blueprint)
	if len(names) == 0 {
		return nil, nil
	}
	readiness, err := i.KubernetesManager.GetKustomizationReadiness(names)
	if err != nil {
		return names, nil
	}
	deps := make(map[string][]string, len(blueprint.Kustomizations))
	for _, k := range blueprint.Kustomizations {
		deps[k.Name] = k.DependsOn
	}

	var frontier []string
	for _, name := range names {
		if readiness[name] {
			delete(nudged, name)
			continue
		}
		notReady = append(notReady, name)
		depsReady := true
		for _, d := range deps[name] {
			if r, known := readiness[d]; known && !r {
				depsReady = false
				break
			}
		}
		if !depsReady {
			continue
		}
		reconciling = append(reconciling, name)
		if _, done := nudged[name]; !done {
			frontier = append(frontier, name)
		}
	}
	if len(frontier) > 0 {
		slices.Sort(frontier)
		for _, n := range frontier {
			nudged[n] = struct{}{}
		}
		i.reconcileKustomizations(ctx, frontier)
		i.forceStalledHelmReleases(ctx, frontier)
	}
	slices.Sort(notReady)
	return notReady, reconciling
}

// reconcileKustomizations requests an immediate Flux reconcile of the named Kustomizations. It is
// best-effort: errors are swallowed, so a placement round never fails on a reconcile nudge. A nil
// or empty names slice is a no-op.
func (i *Provisioner) reconcileKustomizations(ctx context.Context, names []string) {
	if len(names) == 0 {
		return
	}
	if err := i.ensureNotifier(); err != nil {
		return
	}
	_ = i.Notifier.ReconcileKustomizations(ctx, names)
}

// forceStalledHelmReleases force-reconciles any HelmRelease owned by the given kustomizations that
// is not Ready. A release that failed to install or upgrade (for example, a missing secret) stalls
// and does not retry on a plain Kustomization reconcile, since its spec is unchanged. Forcing it
// makes helm-controller retry. Best-effort: failures are swallowed, and healthy releases are left
// untouched.
func (i *Provisioner) forceStalledHelmReleases(ctx context.Context, kustomizations []string) {
	var refs []fluxinfra.HelmReleaseRef
	seen := make(map[string]struct{})
	for _, name := range kustomizations {
		hrs, err := i.KubernetesManager.GetHelmReleasesForKustomization(name, i.fluxNamespace())
		if err != nil {
			continue
		}
		for _, hr := range hrs {
			if helmReleaseReady(hr) {
				continue
			}
			key := hr.Namespace + "/" + hr.Name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, fluxinfra.HelmReleaseRef{Namespace: hr.Namespace, Name: hr.Name})
		}
	}
	if len(refs) == 0 {
		return
	}
	if err := i.ensureNotifier(); err != nil {
		return
	}
	_ = i.Notifier.ReconcileHelmReleases(ctx, refs, true)
}

// Progress reports text on the operation in the context.
func (s stepProgress) Progress(_, text string) {
	presenter.Progress(s.ctx, text)
}

// Start does nothing, because a single step has no per-kustomization children.
func (s stepProgress) Start(string) {}

// Done does nothing, because a single step has no per-kustomization children.
func (s stepProgress) Done(string) {}

// Fail does nothing, because the single step fails with the error its caller returns.
func (s stepProgress) Fail(string, error) {}

// =============================================================================
// Helpers
// =============================================================================

// convergeNames returns the deduplicated names of the blueprint's non-destroyOnly kustomizations.
// This is the set converge drives toward Ready. withCrdLayer prepends CRD kustomizations
// without checking for name collisions. Dedup keeps a collision from double-printing in
// progress output. A nil blueprint has no names.
func convergeNames(bp *blueprintv1alpha1.Blueprint) []string {
	if bp == nil {
		return nil
	}
	names := make([]string, 0, len(bp.Kustomizations))
	seen := make(map[string]bool, len(bp.Kustomizations))
	for _, k := range bp.Kustomizations {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		if seen[k.Name] {
			continue
		}
		seen[k.Name] = true
		names = append(names, k.Name)
	}
	return names
}

// sortedStringKeys returns the keys of a set as a sorted slice.
func sortedStringKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// trackLayers walks names in order. It ends the step of each name not in notReady, and starts the
// step of each name in reconciling, so a kustomization shows only once Flux is working on it.
func trackLayers(layers layerReporter, names, notReady, reconciling []string) {
	pending := make(map[string]bool, len(notReady))
	for _, name := range notReady {
		pending[name] = true
	}
	active := make(map[string]bool, len(reconciling))
	for _, name := range reconciling {
		active[name] = true
	}
	for _, name := range names {
		switch {
		case !pending[name]:
			layers.Done(name)
		case active[name]:
			layers.Start(name)
		}
	}
}

// helmReleaseReady reports whether a HelmRelease currently carries a Ready=True condition.
func helmReleaseReady(hr helmv2.HelmRelease) bool {
	for _, c := range hr.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}
