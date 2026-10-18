// The kubernetes_manager_blueprint file holds the blueprint operations of the BaseKubernetesManager.
// It provides apply, delete, and prune of a full blueprint and of the sources that it references.
// It orders a delete so that dependents go before their dependencies.
// It suspends each Kustomization before a delete and resumes it when its delete starts or the delete aborts.

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/constants"
	runtimegit "github.com/windsorcli/cli/pkg/runtime/git"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Public Methods
// =============================================================================

// ApplyBlueprint applies the entire blueprint to the cluster in the proper sequence.
// It creates the target namespace, applies all blueprint source repositories (Git and OCI),
// applies all individual sources, applies any standalone ConfigMaps, and finally applies
// all kustomizations and their associated ConfigMaps. This orchestrates a complete
// blueprint installation following the intended order. Context ownership labels are stamped on
// each Kustomization's ObjectMeta (so the objects are selectable by context) and propagated to
// managed resources via CommonMetadata, using context info from the config handler.
// Returns an error if any step fails.
func (k *BaseKubernetesManager) ApplyBlueprint(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if err := k.CreateNamespace(namespace); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	mode := k.gitopsMode()

	if blueprint.Repository.Url != "" {
		var secretName string
		if blueprint.Repository.SecretName != nil {
			secretName = *blueprint.Repository.SecretName
		}
		source := blueprintv1alpha1.Source{
			Name:       blueprint.Metadata.Name,
			Url:        blueprint.Repository.Url,
			Ref:        blueprint.Repository.Ref,
			SecretName: secretName,
		}
		if err := k.applyBlueprintSource(source, namespace, true); err != nil {
			return fmt.Errorf("failed to apply blueprint repository: %w", err)
		}
	}

	for _, source := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(source) {
			continue
		}
		if err := k.applyBlueprintSource(source, namespace, false); err != nil {
			return fmt.Errorf("failed to apply source %s: %w", source.Name, err)
		}
	}

	defaultSourceName := blueprint.Metadata.Name

	if blueprint.ConfigMaps != nil {
		for configMapName, data := range blueprint.ConfigMaps {
			if err := k.ApplyConfigMap(configMapName, namespace, data); err != nil {
				return fmt.Errorf("failed to create ConfigMap %s: %w", configMapName, err)
			}
		}
	}

	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		if len(kustomization.Substitutions) > 0 {
			configMapName := fmt.Sprintf("values-%s", kustomization.Name)
			if err := k.ApplyConfigMap(configMapName, namespace, kustomization.Substitutions); err != nil {
				return fmt.Errorf("failed to create ConfigMap for kustomization %s: %w", kustomization.Name, err)
			}
		}
		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources, mode, blueprint.ConfigMaps)

		fluxKustomization.Labels = k.ownershipLabels()
		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: k.ownershipLabels(),
		}

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			return fmt.Errorf("failed to apply kustomization %s: %w", kustomization.Name, err)
		}
	}

	return nil
}

// DeleteBlueprint tears the blueprint down in two phases: destroy-only kustomizations first,
// then regular ones in reverse-topological order. Each delete blocks on
// spec.deletionPolicy=WaitForTermination, so cloud resources release before the object
// disappears.
//
// A Phase 2 failure skips only that Kustomization's own dependency closure (see
// dependencyClosure), not the rest of the run. A retry re-walks the whole blueprint; anything
// already gone returns immediately.
func (k *BaseKubernetesManager) DeleteBlueprint(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	defaultSourceName := blueprint.Metadata.Name

	destroyOnly := []blueprintv1alpha1.Kustomization{}
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly == nil || !*kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		destroyOnly = append(destroyOnly, kustomization)
	}
	if len(destroyOnly) > 0 {
		if errs := k.processDestroyOnlyKustomizations(ctx, destroyOnly, blueprint, namespace, defaultSourceName); len(errs) > 0 {
			return fmt.Errorf("destroy-only hooks failed: %w", errors.Join(errs...))
		}
	}

	eligible := make([]blueprintv1alpha1.Kustomization, 0, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		destroy := kustomization.Destroy.ToBool()
		if destroy != nil && !*destroy {
			continue
		}
		eligible = append(eligible, kustomization)
	}
	for _, kustomization := range eligible {
		if err := k.setKustomizationSuspend(kustomization.Name, namespace, true); err != nil {
			return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: failed to suspend kustomization %q: %w", kustomization.Name, err))
		}
	}

	if err := k.remediateLoadBalancerOwners(eligible, namespace); err != nil {
		return k.abortDestroy(eligible, namespace, fmt.Errorf("destroy aborted: %w", err))
	}

	blocked := map[string]bool{}
	var failures []error
	for _, kustomization := range orderForDestroy(eligible, "destroy") {
		if blocked[kustomization.Name] {
			continue
		}
		opCtx, end := presenter.Begin(ctx, "kustomize:"+kustomization.Name, fmt.Sprintf("Destroying kustomization %s", kustomization.Name))
		if err := k.setKustomizationSuspend(kustomization.Name, namespace, false); err != nil {
			failure := fmt.Errorf("failed to resume kustomization %q before delete: %w", kustomization.Name, err)
			end(failure)
			failures = append(failures, failure)
			k.blockDependencyClosure(eligible, kustomization.Name, blocked)
			continue
		}
		k.waitForResumeReconcile(kustomization.Name, namespace)
		destroy := kustomization.Destroy.ToBool()
		expectWaitForTermination := destroy == nil || *destroy
		if err := k.deleteKustomization(opCtx, kustomization.Name, namespace, &expectWaitForTermination, kustomizationDeleteTimeout(kustomization), true); err != nil {
			failure := fmt.Errorf("failed to delete kustomization %q: %w", kustomization.Name, err)
			end(failure)
			failures = append(failures, failure)
			k.blockDependencyClosure(eligible, kustomization.Name, blocked)
			continue
		}
		end(nil)
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d kustomization(s) failed to delete. Each one's dependency closure was left alone, to avoid orphaning what it may still need: %w", len(failures), errors.Join(failures...))
	}
	return nil
}

// PruneBlueprint deletes Kustomizations belonging to the current context that are no longer part of
// the blueprint. It scopes strictly to this context — only objects carrying the
// windsorcli.dev/context-id label for this context are considered, so kustomizations owned by other
// contexts (or by no Windsor context) are never touched. Every non-DestroyOnly kustomization in the
// blueprint is treated as desired; the live remainder is deleted in reverse-dependency order (read
// from each object's live spec.dependsOn) so dependents tear down before their dependencies, each
// honoring its own deletionPolicy. The caller passes the same prepared blueprint Install applied
// (CRD layers included) so the synthesized crds/crds-<source> layers are recognized as desired and
// not pruned. Deletion errors are collected and joined rather than aborting on the first.
func (k *BaseKubernetesManager) PruneBlueprint(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	orphans, err := k.contextOrphanKustomizations(blueprint, namespace)
	if err != nil {
		return err
	}

	if len(orphans) == 0 {
		return nil
	}

	var errs []error
	for _, orphan := range orderForDestroy(orphans, "prune") {
		opCtx, end := presenter.Begin(ctx, "kustomize:"+orphan.Name, fmt.Sprintf("Pruning orphaned kustomization %s", orphan.Name))
		if err := k.DeleteKustomization(opCtx, orphan.Name, namespace); err != nil {
			failure := fmt.Errorf("failed to prune kustomization %q: %w", orphan.Name, err)
			end(failure)
			errs = append(errs, failure)
			continue
		}
		end(nil)
	}
	return errors.Join(errs...)
}

// ListPrunableKustomizations returns the names of this context's Kustomizations that the blueprint no
// longer declares — exactly what PruneBlueprint would delete, in reverse-dependency order. It is the
// read-only input to plan's prune preview and upgrade's confirmation gate; it deletes nothing.
func (k *BaseKubernetesManager) ListPrunableKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	orphans, err := k.contextOrphanKustomizations(blueprint, namespace)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(orphans))
	for _, orphan := range orderForDestroy(orphans, "prune") {
		names = append(names, orphan.Name)
	}
	return names, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// blockDependencyClosure adds failedName's dependencyClosure to blocked and warns about the rest
// of it, so the walk skips them rather than tearing down a controller the stall may still need.
// failedName itself is not warned about again; its own failure already produced an error.
func (k *BaseKubernetesManager) blockDependencyClosure(eligible []blueprintv1alpha1.Kustomization, failedName string, blocked map[string]bool) {
	closure := dependencyClosure(eligible, failedName)
	skipped := make([]string, 0, len(closure))
	for name := range closure {
		blocked[name] = true
		if name != failedName {
			skipped = append(skipped, name)
		}
	}
	if len(skipped) == 0 {
		return
	}
	slices.Sort(skipped)
	word := "dependency"
	if len(skipped) != 1 {
		word = "dependencies"
	}
	fmt.Fprintf(os.Stderr, "warning: kustomization %s failed to delete; skipping %d %s it may still need: %s\n", failedName, len(skipped), word, strings.Join(skipped, ", "))
}

// abortDestroy un-suspends every eligible Kustomization before propagating cause, so a
// DeleteBlueprint failure never leaves objects suspended with nothing left to resume them.
// setKustomizationSuspend is a no-op against objects already unsuspended or gone. It stops early
// on a request timeout rather than repeating a call the unreachable cluster will fail again.
func (k *BaseKubernetesManager) abortDestroy(eligible []blueprintv1alpha1.Kustomization, namespace string, cause error) error {
	errs := []error{cause}
	for idx, kustomization := range eligible {
		err := k.setKustomizationSuspend(kustomization.Name, namespace, false)
		if err == nil {
			continue
		}
		errs = append(errs, fmt.Errorf("failed to un-suspend kustomization %q during abort cleanup: %w", kustomization.Name, err))
		if errors.Is(err, context.DeadlineExceeded) {
			if remaining := len(eligible) - idx - 1; remaining > 0 {
				errs = append(errs, fmt.Errorf("stopping abort cleanup after a request timeout: %d more kustomization(s) left un-suspended, since the cluster is not answering", remaining))
			}
			break
		}
	}
	return errors.Join(errs...)
}

// contextOrphanKustomizations lists the live Kustomizations belonging to this context (by the
// windsorcli.dev/context-id label) that the blueprint no longer declares — the set Prune deletes and
// ListPrunableKustomizations reports. It scopes strictly to this context, so kustomizations owned by
// other contexts (or by no Windsor context) are never returned. DestroyOnly entries are not desired.
// Each orphan carries its live spec.dependsOn for reverse-dependency ordering.
func (k *BaseKubernetesManager) contextOrphanKustomizations(blueprint *blueprintv1alpha1.Blueprint, namespace string) ([]blueprintv1alpha1.Kustomization, error) {
	contextID := k.configHandler.GetString("id")
	if contextID == "" {
		return nil, fmt.Errorf("context id not set; cannot scope pruning to this context")
	}

	desired := make(map[string]bool, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		desired[kustomization.Name] = true
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	list, err := k.client.ListResources(gvr, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	orphans := make([]blueprintv1alpha1.Kustomization, 0)
	for i := range list.Items {
		item := list.Items[i]
		if item.GetLabels()["windsorcli.dev/context-id"] != contextID {
			continue
		}
		name := item.GetName()
		if desired[name] {
			continue
		}
		orphans = append(orphans, blueprintv1alpha1.Kustomization{
			Name:      name,
			DependsOn: dependsOnFromObject(item),
		})
	}
	return orphans, nil
}

// processDestroyOnlyKustomizations applies all destroy-only kustomizations, waits for all to become ready, then deletes all.
// This approach ensures dependencies remain available while Flux reconciles dependent kustomizations.
// Returns a slice of errors encountered during the process, which may be empty if no errors occurred.
func (k *BaseKubernetesManager) processDestroyOnlyKustomizations(ctx context.Context, kustomizations []blueprintv1alpha1.Kustomization, blueprint *blueprintv1alpha1.Blueprint, namespace, defaultSourceName string) []error {
	mode := k.gitopsMode()
	var errors []error

	destroyOnlyNames := make(map[string]bool)
	for _, kust := range kustomizations {
		destroyOnlyNames[kust.Name] = true
	}

	appliedKustomizations := []blueprintv1alpha1.Kustomization{}

	for _, kustomization := range kustomizations {
		if len(kustomization.Substitutions) > 0 {
			configMapName := fmt.Sprintf("values-%s", kustomization.Name)
			if err := k.ApplyConfigMap(configMapName, namespace, kustomization.Substitutions); err != nil {
				errors = append(errors, fmt.Errorf("failed to create ConfigMap for destroy-only kustomization %s: %w", kustomization.Name, err))
				for i := len(appliedKustomizations) - 1; i >= 0; i-- {
					appliedKust := appliedKustomizations[i]
					if deleteErr := k.deleteKustomization(ctx, appliedKust.Name, namespace, nil, kustomizationDeleteTimeout(appliedKust), true); deleteErr != nil {
						errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", appliedKust.Name, deleteErr))
					}
				}
				return errors
			}
		}

		fluxKustomization := kustomization.ToFluxKustomization(namespace, defaultSourceName, blueprint.Sources, mode, blueprint.ConfigMaps)

		fluxKustomization.Labels = k.ownershipLabels()
		fluxKustomization.Spec.CommonMetadata = &kustomizev1.CommonMetadata{
			Labels: k.ownershipLabels(),
		}

		filteredDependsOn := make([]kustomizev1.DependencyReference, 0)
		for _, dep := range fluxKustomization.Spec.DependsOn {
			if destroyOnlyNames[dep.Name] {
				filteredDependsOn = append(filteredDependsOn, dep)
			}
		}
		fluxKustomization.Spec.DependsOn = filteredDependsOn

		_, end := presenter.Begin(ctx, "kustomize:"+kustomization.Name, fmt.Sprintf("Applying destroy-only kustomization %s", kustomization.Name))

		if err := k.ApplyKustomization(fluxKustomization); err != nil {
			failure := fmt.Errorf("failed to apply destroy-only kustomization %s: %w", kustomization.Name, err)
			end(failure)
			errors = append(errors, failure)
			for i := len(appliedKustomizations) - 1; i >= 0; i-- {
				appliedKust := appliedKustomizations[i]
				if deleteErr := k.deleteKustomization(ctx, appliedKust.Name, namespace, nil, kustomizationDeleteTimeout(appliedKust), true); deleteErr != nil {
					errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", appliedKust.Name, deleteErr))
				}
			}
			return errors
		}
		end(nil)
		appliedKustomizations = append(appliedKustomizations, kustomization)
	}

	kustomizationNames := make([]string, len(kustomizations))
	for i, kust := range kustomizations {
		kustomizationNames[i] = kust.Name
	}

	_, endWait := presenter.Begin(ctx, "kustomize:destroy-only-wait", fmt.Sprintf("Waiting for %d destroy-only kustomization(s) to become ready", len(kustomizations)))

	waitTimeout := time.After(k.kustomizationReconcileTimeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	allReady := false
	statusCheckFailed := false

waitLoop:
	for !allReady {
		select {
		case <-waitTimeout:
			break waitLoop
		case <-ticker.C:
			status, err := k.GetKustomizationStatus(kustomizationNames)
			if err != nil {
				errors = append(errors, fmt.Errorf("destroy-only kustomizations failed: %w", err))
				statusCheckFailed = true
				break waitLoop
			}
			allReady = true
			for _, name := range kustomizationNames {
				if !status[name] {
					allReady = false
					break
				}
			}
		}
	}
	ticker.Stop()

	if !allReady {
		endWait(fmt.Errorf("destroy-only kustomizations did not become ready"))
		if !statusCheckFailed {
			errors = append(errors, fmt.Errorf("destroy-only kustomizations did not become ready within timeout - cleanup may not have completed%s", k.describeNotReadyKustomizations(kustomizationNames, k.gitopsNamespace())))
		}
		for i := len(kustomizations) - 1; i >= 0; i-- {
			kustomization := kustomizations[i]
			if deleteErr := k.deleteKustomization(ctx, kustomization.Name, namespace, nil, kustomizationDeleteTimeout(kustomization), true); deleteErr != nil {
				errors = append(errors, fmt.Errorf("failed to delete failed destroy-only kustomization %s: %w", kustomization.Name, deleteErr))
			}
		}
		return errors
	}
	endWait(nil)

	for _, kustomization := range orderForDestroy(kustomizations, "destroy-only") {
		opCtx, end := presenter.Begin(ctx, "kustomize:"+kustomization.Name, fmt.Sprintf("Destroying destroy-only kustomization %s", kustomization.Name))

		if err := k.deleteKustomization(opCtx, kustomization.Name, namespace, nil, kustomizationDeleteTimeout(kustomization), true); err != nil {
			failure := fmt.Errorf("failed to delete destroy-only kustomization %s: %w", kustomization.Name, err)
			end(failure)
			errors = append(errors, failure)
		} else {
			end(nil)
		}
	}

	return errors
}

// applyBlueprintSource applies a blueprint Source as a GitRepository or OCIRepository resource.
// It routes to the appropriate repository type based on the source URL and applies it to the
// cluster. isPrimary is true for the blueprint's own repository (the top-level "repository:"
// field) and selects the short, continuously-polled default interval rather than the long
// pinned-vendor-source default; see constants.FluxSourceInterval.
func (k *BaseKubernetesManager) applyBlueprintSource(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	if strings.HasPrefix(source.Url, "oci://") {
		return k.applyBlueprintOCIRepository(source, namespace, isPrimary)
	}
	return k.applyBlueprintGitRepository(source, namespace, isPrimary)
}

// setKustomizationSuspend patches spec.suspend on a Kustomization. DeleteBlueprint
// suspends every eligible Kustomization up front, since an un-deleted one that keeps
// reconciling can re-create a resource another component's Helm uninstall is mid-way
// through deleting, deadlocking it. It then resumes each one right before deleting it,
// since the finalizer only prunes inventory when not suspended — deleting while still
// suspended would orphan resources. A NotFound Kustomization is treated as success.
func (k *BaseKubernetesManager) setKustomizationSuspend(name, namespace string, suspend bool) error {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	patch := fmt.Appendf(nil, `{"spec":{"suspend":%t}}`, suspend)
	opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
	if _, err := k.client.PatchResource(context.Background(), gvr, namespace, name, types.MergePatchType, patch, opts); err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

// waitForResumeReconcile polls up to kustomizationReconcileSleep for a Kustomization's
// status.observedGeneration to catch up to metadata.generation after a resume. A resume
// triggers a reconcile. Deleting before that reconcile settles can race
// kustomize-controller and strand the object mid-delete. This check is best effort. It
// returns on any read error or on timeout. Each sleep between polls is capped to the
// time left in the budget, so it never adds a full kustomizationWaitPollInterval on top
// of an already-expired deadline. deleteKustomization's own wait loop is the real safety
// net.
func (k *BaseKubernetesManager) waitForResumeReconcile(name, namespace string) {
	deadline := k.shims.TimeNow().Add(k.kustomizationReconcileSleep)
	for {
		obj, err := k.client.GetResource(kustomizationsGVR, namespace, name)
		if err != nil {
			return
		}
		if reconcileGenerationSettled(obj) {
			return
		}
		remaining := deadline.Sub(k.shims.TimeNow())
		if remaining <= 0 {
			return
		}
		k.shims.TimeSleep(min(k.kustomizationWaitPollInterval, remaining))
	}
}

// applyBlueprintGitRepository converts and applies a blueprint Source as a GitRepository.
// isPrimary selects the short, continuously-polled interval default for the blueprint's own
// repository rather than the long pinned-vendor-source default; see constants.FluxSourceInterval.
func (k *BaseKubernetesManager) applyBlueprintGitRepository(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	sourceUrl := runtimegit.NormalizeRemoteURL(source.Url)

	gitRepo := &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: sourceUrl,
			Interval: metav1.Duration{
				Duration: constants.FluxSourceInterval(isPrimary),
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: &sourcev1.GitRepositoryRef{
				Branch: source.Ref.Branch,
				Tag:    source.Ref.Tag,
				SemVer: source.Ref.SemVer,
				Commit: source.Ref.Commit,
			},
		},
	}

	if source.SecretName != "" {
		gitRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return k.ApplyGitRepository(gitRepo)
}

// applyBlueprintOCIRepository converts and applies a blueprint Source as an OCIRepository.
// isPrimary selects the short, continuously-polled interval default for the blueprint's own
// repository rather than the long pinned-vendor-source default; see constants.FluxSourceInterval.
// Checks for a "@sha256:<hex>" digest before the tag split, since the digest also has a colon.
func (k *BaseKubernetesManager) applyBlueprintOCIRepository(source blueprintv1alpha1.Source, namespace string, isPrimary bool) error {
	ociURL := source.Url
	var ref *sourcev1.OCIRepositoryRef

	if atIdx := strings.Index(ociURL, "@sha256:"); atIdx > len("oci://") {
		ociURL = ociURL[:atIdx]
		ref = &sourcev1.OCIRepositoryRef{
			Digest: source.Url[atIdx+1:],
		}
	} else if lastColon := strings.LastIndex(ociURL, ":"); lastColon > len("oci://") {
		if tagPart := ociURL[lastColon+1:]; tagPart != "" && !strings.Contains(tagPart, "/") {
			ociURL = ociURL[:lastColon]
			ref = &sourcev1.OCIRepositoryRef{
				Tag: tagPart,
			}
		}
	}

	if ref == nil && (source.Ref.Tag != "" || source.Ref.SemVer != "" || source.Ref.Commit != "") {
		ref = &sourcev1.OCIRepositoryRef{
			Tag:    source.Ref.Tag,
			SemVer: source.Ref.SemVer,
			Digest: source.Ref.Commit,
		}
	}

	if ref == nil {
		ref = &sourcev1.OCIRepositoryRef{
			Tag: "latest",
		}
	}

	ociRepo := &sourcev1.OCIRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCIRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.OCIRepositorySpec{
			URL: ociURL,
			Interval: metav1.Duration{
				Duration: constants.FluxSourceInterval(isPrimary),
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: ref,
		},
	}

	if source.SecretName != "" {
		ociRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return k.ApplyOCIRepository(ociRepo)
}

// =============================================================================
// Helpers
// =============================================================================

// reconcileGenerationSettled reports whether a Kustomization's status.observedGeneration
// has caught up to metadata.generation. A match means the most recent spec change has
// been through a full reconcile.
func reconcileGenerationSettled(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	observed, found, err := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
	if err != nil || !found {
		return false
	}
	return observed >= obj.GetGeneration()
}

// dependsOnFromObject extracts the dependency names from a live Kustomization's spec.dependsOn,
// used to order pruned kustomizations so dependents are deleted before their dependencies.
func dependsOnFromObject(obj unstructured.Unstructured) []string {
	raw, found, err := unstructured.NestedSlice(obj.Object, "spec", "dependsOn")
	if err != nil || !found {
		return nil
	}
	deps := make([]string, 0, len(raw))
	for _, entry := range raw {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entryMap["name"].(string); ok && name != "" {
			deps = append(deps, name)
		}
	}
	return deps
}

// orderForDestroy returns the input slice ordered for destroy: reverse-topological
// when reverseTopologicalKustomizations succeeds, falling back to reverse-array
// order on cycle-detection error (with a stderr warning). Cycles are normally
// rejected by blueprint validation; the fallback is defensive so a malformed
// blueprint can still be torn down. The label is interpolated into the warning
// message to distinguish destroy contexts (e.g. "destroy" vs "destroy-only").
func orderForDestroy(ks []blueprintv1alpha1.Kustomization, label string) []blueprintv1alpha1.Kustomization {
	ordered, err := reverseTopologicalKustomizations(ks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not compute reverse-topological %s order (%v); falling back to reverse-array order\n", label, err)
		ordered = make([]blueprintv1alpha1.Kustomization, len(ks))
		for i, kustomization := range ks {
			ordered[len(ks)-1-i] = kustomization
		}
	}
	return ordered
}

// reverseTopologicalKustomizations returns ks in destroy order — each kustomization
// before its DependsOn entries. Independent nodes tie-break by reverse input order,
// so a topo-sorted input produces the same walk as a naive slice-reverse. Missing
// dependencies (a DependsOn name not in ks) are treated as no-edge, matching the
// apply-side walk. Returns an error on cycles across two or more nodes; a single-
// node input short-circuits before cycle detection, so a self-loop on a lone
// kustomization is not flagged — do not rely on this function to validate a
// single-entry slice.
func reverseTopologicalKustomizations(ks []blueprintv1alpha1.Kustomization) ([]blueprintv1alpha1.Kustomization, error) {
	if len(ks) == 0 {
		return []blueprintv1alpha1.Kustomization{}, nil
	}
	if len(ks) == 1 {
		out := make([]blueprintv1alpha1.Kustomization, 1)
		out[0] = ks[0]
		return out, nil
	}

	nameToIndex := make(map[string]int, len(ks))
	for i := range ks {
		nameToIndex[ks[i].Name] = i
	}

	forward := make([]int, 0, len(ks))
	visited := make(map[int]bool, len(ks))
	visiting := make(map[int]bool, len(ks))

	var visit func(idx int) error
	visit = func(idx int) error {
		if visiting[idx] {
			return fmt.Errorf("dependency cycle detected involving kustomization %q", ks[idx].Name)
		}
		if visited[idx] {
			return nil
		}
		visiting[idx] = true
		for _, dep := range ks[idx].DependsOn {
			depIdx, ok := nameToIndex[dep]
			if !ok {
				continue
			}
			if err := visit(depIdx); err != nil {
				return err
			}
		}
		visiting[idx] = false
		visited[idx] = true
		forward = append(forward, idx)
		return nil
	}

	for i := range ks {
		if !visited[i] {
			if err := visit(i); err != nil {
				return nil, err
			}
		}
	}

	out := make([]blueprintv1alpha1.Kustomization, len(ks))
	for i, idx := range forward {
		out[len(forward)-1-i] = ks[idx]
	}
	return out, nil
}

// dependencyClosure returns start and every name its DependsOn edges reach, transitively. In
// reverse-topological order these are exactly what is still pending when start stalls, and one of
// them may still be needed to resolve it. A DependsOn name missing from ks is treated as no-edge,
// matching reverseTopologicalKustomizations.
func dependencyClosure(ks []blueprintv1alpha1.Kustomization, start string) map[string]bool {
	byName := make(map[string]blueprintv1alpha1.Kustomization, len(ks))
	for _, k := range ks {
		byName[k.Name] = k
	}

	closure := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if closure[name] {
			return
		}
		closure[name] = true
		for _, dep := range byName[name].DependsOn {
			if _, ok := byName[dep]; ok {
				visit(dep)
			}
		}
	}
	visit(start)
	return closure
}
