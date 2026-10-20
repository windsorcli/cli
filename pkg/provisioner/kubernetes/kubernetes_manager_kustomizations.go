// The kubernetes_manager_kustomizations file holds the Kustomization and source methods of the BaseKubernetesManager.
// It provides apply, status, readiness, and wait operations for Flux Kustomizations.
// It applies GitRepository and OCIRepository sources and checks their Ready condition.
// It sorts a Ready condition into ready, pending, or failed.

package kubernetes

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// =============================================================================
// Constants
// =============================================================================

// kustomizationWaitErrorBudgetFraction is the part of the total wait that a streak of list errors can use.
const kustomizationWaitErrorBudgetFraction = 0.25

// kustomizationWaitMessageLimit caps the length of a failure message in a progress line.
const kustomizationWaitMessageLimit = 200

// kustomizationsGVR is the Flux Kustomization resource.
var kustomizationsGVR = schema.GroupVersionResource{
	Group:    "kustomize.toolkit.fluxcd.io",
	Version:  "v1",
	Resource: "kustomizations",
}

const (
	kustomizationFailureHard kustomizationFailureKind = iota
	kustomizationFailurePending
)

// kustomizationFailureReasons maps each Ready=False reason that counts as a failure to its kind.
var kustomizationFailureReasons = map[string]kustomizationFailureKind{
	meta.BuildFailedReason:          kustomizationFailureHard,
	meta.ArtifactFailedReason:       kustomizationFailureHard,
	meta.ReconciliationFailedReason: kustomizationFailurePending,
}

// =============================================================================
// Types
// =============================================================================

// kustomizationFailureStreak tracks when a Kustomization began failing and when it last did.
type kustomizationFailureStreak struct {
	start    time.Time
	lastSeen time.Time
}

// kustomizationFailureKind classifies a Ready=False Reason: hard reasons never recover, pending
// reasons are ones kustomize-controller retries on its own.
type kustomizationFailureKind int

// kustomizationFailedError signals a Ready condition with a Reason in kustomizationFailureReasons.
type kustomizationFailedError struct {
	name    string
	reason  string
	message string
}

// =============================================================================
// Public Methods
// =============================================================================

// ApplyKustomization creates or updates a Kustomization resource using SSA
func (k *BaseKubernetesManager) ApplyKustomization(kustomization kustomizev1.Kustomization) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(&kustomization)
	if err != nil {
		return werror.Wrap(err, "failed to convert kustomization to unstructured")
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return werror.Wrap(err, "invalid kustomization fields")
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// WaitForKustomizations waits until each Kustomization is Ready, within the timeout of the longest
// dependency chain. It returns ctx.Err() on cancellation. BuildFailed and ArtifactFailed fail at once.
// List errors and ReconciliationFailed fail after an error budget: a quarter of the total, or the own
// timeout of the Kustomization when that is shorter, and never less than the minimum. It reports one
// step per Kustomization to layers, starting each when its dependencies are Ready and failing the one
// that fails the wait.
func (k *BaseKubernetesManager) WaitForKustomizations(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, layers KustomizationReporter) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	timeout := k.calculateTotalWaitTime(blueprint)
	kustomizationNames := make([]string, 0, len(blueprint.Kustomizations))
	ownTimeouts := make(map[string]time.Duration, len(blueprint.Kustomizations))
	seenNames := make(map[string]bool, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		if kustomization.DestroyOnly != nil && *kustomization.DestroyOnly {
			continue
		}
		if seenNames[kustomization.Name] {
			continue
		}
		seenNames[kustomization.Name] = true
		kustomizationNames = append(kustomizationNames, kustomization.Name)
		ownTimeouts[kustomization.Name] = constants.DefaultFluxKustomizationTimeout
		if kustomization.Timeout != nil && kustomization.Timeout.Duration != 0 {
			ownTimeouts[kustomization.Name] = kustomization.Timeout.Duration
		}
	}

	maxErrorDuration := time.Duration(float64(timeout) * kustomizationWaitErrorBudgetFraction)
	if maxErrorDuration < k.kustomizationWaitMinErrorDuration {
		maxErrorDuration = k.kustomizationWaitMinErrorDuration
	}
	failureBudget := func(name string) time.Duration {
		budget := maxErrorDuration
		if own := ownTimeouts[name]; own < budget {
			budget = own
		}
		if budget < k.kustomizationWaitMinErrorDuration {
			budget = k.kustomizationWaitMinErrorDuration
		}
		return budget
	}

	dependsOn := make(map[string][]string, len(blueprint.Kustomizations))
	for _, kustomization := range blueprint.Kustomizations {
		dependsOn[kustomization.Name] = kustomization.DependsOn
	}

	timeoutChan := time.After(timeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	var errorStreakStart time.Time
	readyKustomizations := make(map[string]bool, len(kustomizationNames))
	failureStreaks := make(map[string]kustomizationFailureStreak, len(kustomizationNames))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for kustomizations%s", k.describeNotReadyKustomizations(kustomizationNames, k.gitopsNamespace()))
		case <-ticker.C:
			objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
			if err != nil && isNotFoundError(err) {
				errorStreakStart = time.Time{}
				continue
			}
			if err != nil {
				if errorStreakStart.IsZero() {
					errorStreakStart = time.Now()
				}
				if time.Since(errorStreakStart) >= maxErrorDuration {
					return werror.Wrap(err, "kustomization readiness checks failing for over %s", maxErrorDuration)
				}
				continue
			}
			if objList == nil {
				objList = &unstructured.UnstructuredList{}
			}

			byName := make(map[string]*unstructured.Unstructured, len(objList.Items))
			for i := range objList.Items {
				byName[objList.Items[i].GetName()] = &objList.Items[i]
			}

			for _, name := range kustomizationNames {
				if readyKustomizations[name] {
					continue
				}
				obj, exists := byName[name]
				if !exists {
					continue
				}
				ready, pending, failed := kustomizationConditionStatus(obj)
				if failed != nil {
					layers.Fail(name, failed)
					return werror.Wrap(failed, "kustomization will not become ready")
				}
				if pending != nil {
					budget := failureBudget(name)
					now := time.Now()
					streak, tracking := failureStreaks[name]
					if !tracking || now.Sub(streak.lastSeen) >= budget+k.kustomizationWaitPollInterval {
						streak.start = now
					}
					streak.lastSeen = now
					failureStreaks[name] = streak
					layers.Progress(name, kustomizationFailureText(pending))
					if now.Sub(streak.start) >= budget {
						layers.Fail(name, pending)
						return werror.Wrap(pending, "kustomization failing for over %s", budget)
					}
					continue
				}
				if ready {
					delete(failureStreaks, name)
					readyKustomizations[name] = true
					layers.Done(name)
				}
			}

			errorStreakStart = time.Time{}
			if len(readyKustomizations) == len(kustomizationNames) {
				return nil
			}
			for _, name := range kustomizationNames {
				if !readyKustomizations[name] && dependenciesReady(dependsOn[name], kustomizationNames, readyKustomizations) {
					layers.Start(name)
				}
			}
		}
	}
}

// GetHelmReleasesForKustomization gets HelmReleases associated with a Kustomization
func (k *BaseKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]helmv2.HelmRelease, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	obj, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		if isNotFoundError(err) {
			return []helmv2.HelmRelease{}, nil
		}
		return nil, werror.Wrap(err, "failed to get kustomization")
	}

	var kustomization kustomizev1.Kustomization
	if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomization); err != nil {
		return nil, werror.Wrap(err, "failed to convert kustomization")
	}

	var helmReleases []helmv2.HelmRelease
	if kustomization.Status.Inventory == nil {
		return helmReleases, nil
	}

	for _, entry := range kustomization.Status.Inventory.Entries {
		decoded, ok := decodeInventoryID(entry.ID)
		if !ok || decoded.Group != "helm.toolkit.fluxcd.io" || decoded.Kind != "HelmRelease" {
			continue
		}
		helmRelease, err := k.getHelmRelease(decoded.Name, decoded.Namespace)
		if err != nil {
			return nil, err
		}
		helmReleases = append(helmReleases, *helmRelease)
	}

	return helmReleases, nil
}

// ApplyGitRepository creates or updates a GitRepository resource using SSA
func (k *BaseKubernetesManager) ApplyGitRepository(repo *sourcev1.GitRepository) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(repo)
	if err != nil {
		return werror.Wrap(err, "failed to convert gitrepository to unstructured")
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return werror.Wrap(err, "invalid gitrepository fields")
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// ApplyOCIRepository creates or updates an OCIRepository resource using SSA
func (k *BaseKubernetesManager) ApplyOCIRepository(repo *sourcev1.OCIRepository) error {
	obj := &unstructured.Unstructured{}
	unstructuredMap, err := k.shims.ToUnstructured(repo)
	if err != nil {
		return werror.Wrap(err, "failed to convert ocirepository to unstructured")
	}
	obj.Object = unstructuredMap

	if err := validateFields(obj); err != nil {
		return werror.Wrap(err, "invalid ocirepository fields")
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "ocirepositories",
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// CheckGitRepositoryStatus checks the status of all GitRepository and OCIRepository resources
func (k *BaseKubernetesManager) CheckGitRepositoryStatus() error {
	gitGvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	gitObjList, err := k.client.ListResources(gitGvr, k.gitopsNamespace())
	if err != nil {
		return werror.Wrap(err, "failed to list git repositories")
	}

	for _, obj := range gitObjList.Items {
		var gitRepo sourcev1.GitRepository
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &gitRepo); err != nil {
			return werror.Wrap(err, "failed to convert git repository %s", gitRepo.Name)
		}

		for _, condition := range gitRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", gitRepo.Name, condition.Message)
			}
		}
	}

	ociGvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "ocirepositories",
	}

	ociObjList, err := k.client.ListResources(ociGvr, k.gitopsNamespace())
	if err != nil {
		return werror.Wrap(err, "failed to list oci repositories")
	}

	for _, obj := range ociObjList.Items {
		var ociRepo sourcev1.OCIRepository
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &ociRepo); err != nil {
			return werror.Wrap(err, "failed to convert oci repository %s", ociRepo.Name)
		}

		for _, condition := range ociRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", ociRepo.Name, condition.Message)
			}
		}
	}

	return nil
}

// GetKustomizationStatus returns a map indicating readiness for each specified kustomization in the default
// Flux system namespace. If a kustomization is not found, its status is set to false. If any kustomization
// has a Ready condition with Status False and a Reason in kustomizationFailureReasons, an error is
// returned with the failure message.
func (k *BaseKubernetesManager) GetKustomizationStatus(names []string) (map[string]bool, error) {
	objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
	if err != nil {
		return nil, werror.Wrap(err, "failed to list kustomizations")
	}

	status := make(map[string]bool)
	found := make(map[string]bool)

	for _, obj := range objList.Items {
		var kustomizeObj kustomizev1.Kustomization
		if err := k.shims.FromUnstructured(obj.UnstructuredContent(), &kustomizeObj); err != nil {
			return nil, werror.Wrap(err, "failed to convert kustomization %s", kustomizeObj.Name)
		}

		found[kustomizeObj.Name] = true
		ready := false
		for _, condition := range kustomizeObj.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == "True" {
					ready = true
				} else if condition.Status == "False" {
					if _, failed := kustomizationFailureReasons[condition.Reason]; failed {
						return nil, fmt.Errorf("kustomization %s failed: %s", kustomizeObj.Name, condition.Message)
					}
				}
				break
			}
		}
		status[kustomizeObj.Name] = ready
	}

	for _, name := range names {
		if !found[name] {
			status[name] = false
			continue
		}
	}

	return status, nil
}

// GetKustomizationReadiness returns whether each named Kustomization currently reports Ready=True, in the
// gitops namespace. Unlike GetKustomizationStatus it never fails on a Kustomization in a failed state — a
// failed one is simply reported not-ready — so a convergence driver can keep nudging it toward Ready rather
// than aborting. Names absent from the cluster report false; only an API list error propagates.
func (k *BaseKubernetesManager) GetKustomizationReadiness(names []string) (map[string]bool, error) {
	objList, err := k.client.ListResources(kustomizationsGVR, k.gitopsNamespace())
	if err != nil {
		return nil, werror.Wrap(err, "failed to list kustomizations")
	}

	ready := make(map[string]bool, len(names))
	for _, name := range names {
		ready[name] = false
	}
	for i := range objList.Items {
		obj := &objList.Items[i]
		if _, wanted := ready[obj.GetName()]; wanted {
			ready[obj.GetName()] = kustomizationReady(obj)
		}
	}
	return ready, nil
}

// KustomizationExists returns true if a Kustomization resource with the given name exists in the given namespace.
// Returns false (not an error) when the resource is simply absent; propagates other API errors.
func (k *BaseKubernetesManager) KustomizationExists(name, namespace string) (bool, error) {
	_, err := k.client.GetResource(kustomizationsGVR, namespace, name)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// describeNotReadyKustomizations returns a ": name (condition), ..." suffix for each named Kustomization
// that is not Ready, with its most useful status condition. It returns "" when all are Ready.
// A Kustomization that it cannot read, or that it reaches after notReadyDescribeBudget, shows its name only.
// The budget exists because the API can be slow after a wait times out.
func (k *BaseKubernetesManager) describeNotReadyKustomizations(names []string, namespace string) string {
	start := time.Now()
	var stuck []string
	for _, name := range names {
		if time.Since(start) >= k.notReadyDescribeBudget {
			stuck = append(stuck, name)
			continue
		}
		obj, err := k.client.GetResource(kustomizationsGVR, namespace, name)
		if err != nil {
			stuck = append(stuck, name)
			continue
		}
		if kustomizationReady(obj) {
			continue
		}
		stuck = append(stuck, name+describeStuckKustomization(obj))
	}
	if len(stuck) == 0 {
		return ""
	}
	return ": " + strings.Join(stuck, ", ")
}

// Error returns the name, reason, and message of the failed Kustomization.
func (e *kustomizationFailedError) Error() string {
	return fmt.Sprintf("kustomization %s failed (%s): %s", e.name, e.reason, e.message)
}

// getHelmRelease gets a HelmRelease by name and namespace
func (k *BaseKubernetesManager) getHelmRelease(name, namespace string) (*helmv2.HelmRelease, error) {
	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	obj, err := k.client.GetResource(gvr, namespace, name)
	if err != nil {
		return nil, werror.Wrap(err, "failed to get helm release")
	}

	var helmRelease helmv2.HelmRelease
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &helmRelease); err != nil {
		return nil, werror.Wrap(err, "failed to convert helm release")
	}

	return &helmRelease, nil
}

// calculateTotalWaitTime calculates the total timeout for the longest dependency chain
// by summing the timeouts of all kustomizations along the path. It traverses the dependency graph
// to find the path with the maximum cumulative timeout. Returns the calculated timeout or the default
// if no kustomizations exist. Cycles are not detected and may cause stack overflow.
func (k *BaseKubernetesManager) calculateTotalWaitTime(blueprint *blueprintv1alpha1.Blueprint) time.Duration {
	if len(blueprint.Kustomizations) == 0 {
		return constants.DefaultKustomizationWaitTotalTimeout
	}

	nameToIndex := make(map[string]int)
	for i, kustomization := range blueprint.Kustomizations {
		nameToIndex[kustomization.Name] = i
	}

	var calculateChainTimeout func(componentIndex int, visited map[int]bool) time.Duration
	calculateChainTimeout = func(componentIndex int, visited map[int]bool) time.Duration {
		if visited[componentIndex] {
			return 0
		}
		visited[componentIndex] = true
		defer delete(visited, componentIndex)

		kustomization := blueprint.Kustomizations[componentIndex]

		currentTimeout := constants.DefaultFluxKustomizationTimeout
		if kustomization.Timeout != nil && kustomization.Timeout.Duration != 0 {
			currentTimeout = kustomization.Timeout.Duration
		}

		if len(kustomization.DependsOn) == 0 {
			return currentTimeout
		}

		maxDependencyTimeout := time.Duration(0)
		for _, depName := range kustomization.DependsOn {
			if depIndex, exists := nameToIndex[depName]; exists {
				depTimeout := calculateChainTimeout(depIndex, visited)
				if depTimeout > maxDependencyTimeout {
					maxDependencyTimeout = depTimeout
				}
			}
		}

		return currentTimeout + maxDependencyTimeout
	}

	maxTimeout := time.Duration(0)
	for i := range blueprint.Kustomizations {
		timeout := calculateChainTimeout(i, make(map[int]bool))
		if timeout > maxTimeout {
			maxTimeout = timeout
		}
	}

	if maxTimeout == 0 {
		return constants.DefaultKustomizationWaitTotalTimeout
	}

	return maxTimeout
}

// =============================================================================
// Helpers
// =============================================================================

// dependenciesReady reports whether every dependency that is part of the wait set is Ready.
// A dependency outside the wait set does not block.
func dependenciesReady(deps, waitSet []string, ready map[string]bool) bool {
	for _, dep := range deps {
		if slices.Contains(waitSet, dep) && !ready[dep] {
			return false
		}
	}
	return true
}

// kustomizationFailureText renders a kustomization failure as "reason: message" on one line, with the
// message cut to kustomizationWaitMessageLimit.
func kustomizationFailureText(failure *kustomizationFailedError) string {
	message := strings.Join(strings.Fields(failure.message), " ")
	if len(message) > kustomizationWaitMessageLimit {
		message = message[:kustomizationWaitMessageLimit] + "..."
	}
	return failure.reason + ": " + message
}

// describeStuckKustomization returns the most useful status condition of obj as
// " (Type=Status Reason: message)", for a timeout error. It prefers Stalled, then a Ready that is
// not True, then any other condition that is not True. It returns "" for a nil object or when no
// condition has a message.
func describeStuckKustomization(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return ""
	}
	var ready, stalled, other map[string]any
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		switch {
		case condType == "Stalled" && condStatus == "True":
			stalled = cond
		case condType == "Ready" && condStatus != "True":
			ready = cond
		case condStatus != "True" && other == nil:
			other = cond
		}
	}
	pick := stalled
	if pick == nil {
		pick = ready
	}
	if pick == nil {
		pick = other
	}
	if pick == nil {
		return ""
	}
	condType, _ := pick["type"].(string)
	condStatus, _ := pick["status"].(string)
	reason, _ := pick["reason"].(string)
	message, _ := pick["message"].(string)
	message = strings.ReplaceAll(strings.TrimSpace(message), "\n", " ")
	if message == "" {
		return ""
	}
	if reason != "" {
		return fmt.Sprintf(" (%s=%s %s: %s)", condType, condStatus, reason, message)
	}
	return fmt.Sprintf(" (%s=%s: %s)", condType, condStatus, message)
}

// kustomizationReady reports whether a Kustomization's status carries a
// Ready=True condition. A nil object or a missing status or conditions slice reads as not ready.
func kustomizationReady(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

// kustomizationConditionStatus reports a Kustomization's Ready condition: ready true on
// Ready=True, failed set on a hard-failure Reason, pending set on a retryable one.
func kustomizationConditionStatus(obj *unstructured.Unstructured) (ready bool, pending *kustomizationFailedError, failed *kustomizationFailedError) {
	if obj == nil {
		return false, nil, nil
	}
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false, nil, nil
	}
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]any)
		if !ok {
			continue
		}
		if condMap["type"] != "Ready" {
			continue
		}
		if condMap["status"] == "True" {
			return true, nil, nil
		}
		if condMap["status"] == "False" {
			reason, _ := condMap["reason"].(string)
			if kind, isFailure := kustomizationFailureReasons[reason]; isFailure {
				message, _ := condMap["message"].(string)
				failedErr := &kustomizationFailedError{name: obj.GetName(), reason: reason, message: message}
				if kind == kustomizationFailureHard {
					return false, nil, failedErr
				}
				return false, failedErr, nil
			}
		}
		break
	}
	return false, nil, nil
}
