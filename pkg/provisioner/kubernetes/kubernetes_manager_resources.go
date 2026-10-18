// The kubernetes_manager_resources file holds the ConfigMap and Secret methods of the BaseKubernetesManager.
// It provides apply of ConfigMaps and Secrets, and prune of the Secrets that the CLI placed.
// It rolls the workloads that consume a Secret when the Secret content changes.
// It reads and writes the version marker ConfigMap.

package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// =============================================================================
// Constants
// =============================================================================

// secretChecksumAnnotationPrefix prefixes the pod-template annotation whose value is a content digest
// of a secret the workload consumes. The suffix is the secret name, so each consumed secret gets its
// own annotation and a change to one rolls only its consumers.
const secretChecksumAnnotationPrefix = "checksum.windsorcli.dev/"

// secretOwnerLabel names the kustomization a CLI-placed Secret belongs to. It is set only by
// ApplySecret, never by Flux, so it is the reliable marker for finding secrets the CLI itself placed —
// PruneSecrets selects on it (scoped to the context) to reclaim orphans without touching Flux-managed
// secrets that happen to carry the context labels via CommonMetadata.
const secretOwnerLabel = "windsorcli.dev/secret-owner" // #nosec G101 -- label key, not a credential

// dockerConfigSecretKey is the well-known stringData key a kubernetes.io/dockerconfigjson Secret
// carries its registry auth under.
const dockerConfigSecretKey = ".dockerconfigjson" // #nosec G101 -- well-known stringData key, not a credential

// defaultDockerConfigServer is the registry secretTypeAndData assumes when docker-username and
// docker-password are set but docker-server is not.
const defaultDockerConfigServer = "ghcr.io"

// =============================================================================
// Public Methods
// =============================================================================

// ApplyConfigMap creates or updates a ConfigMap using SSA. It refuses to write a value that still
// contains an unresolved expression, such as an unresolved terraform_output().
func (k *BaseKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	for key, value := range data {
		if evaluator.ContainsExpression(value) {
			return fmt.Errorf("configmap %s: value for %q still contains an unresolved expression; run `windsor apply` (or `windsor upgrade`) first to resolve it", name, key)
		}
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid configmap fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && isImmutableConfigMap(existing) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete immutable configmap: %w", err)
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// ApplySecret creates or updates a Secret using SSA. Values are supplied as plaintext in
// stringData (write-only; the API server folds them into data) and are never logged. Mirrors
// ApplyConfigMap's server-side-apply handling, including its immutable-field guard: Kubernetes
// rejects an update that changes Secret.type, so if an existing Secret's type differs from the
// newly resolved one, ApplySecret deletes it first rather than SSA-merging a rejected change. It
// stamps the context ownership labels plus a secret-owner label naming the kustomization the
// secret belongs to; that label is set only by CLI placement (never by Flux), so PruneSecrets can
// find and reclaim CLI-placed secrets without ever touching a Flux-managed one. The Secret's type
// and stringData are resolved by secretTypeAndData: stringData already carrying
// ".dockerconfigjson", or carrying docker-username/docker-password (docker-server optional),
// produces a kubernetes.io/dockerconfigjson Secret for imagePullSecrets; anything else stays Opaque.
func (k *BaseKubernetesManager) ApplySecret(name, namespace string, stringData map[string]string, owner string) error {
	secretType, resolvedData, err := secretTypeAndData(stringData)
	if err != nil {
		return fmt.Errorf("failed to resolve secret type for %q: %w", name, err)
	}

	labels := k.ownershipLabels()
	labels[secretOwnerLabel] = owner
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       secretType,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"stringData": resolvedData,
		},
	}
	obj.SetLabels(labels)

	if err := validateFields(obj); err != nil {
		return fmt.Errorf("invalid secret fields: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && secretTypeChanged(existing, secretType) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete secret with changed type: %w", err)
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// PruneSecrets deletes the CLI-placed secrets for this context that the latest placement no longer
// wants, reconciling the cluster to the desired set. desired maps a namespace to the set of secret
// names just placed there; a secret whose (namespace, name) is absent is deleted. It lists only secrets
// bearing this context's id and the secret-owner marker — a label set solely by ApplySecret, never by
// Flux — so it reclaims exactly what the CLI placed (a secret dropped from a fan-out list, a secret
// removed from a system, or every CLI secret when desired is empty) and never a Flux-managed secret
// that merely inherited the context labels via CommonMetadata. It fails closed when the context id is
// unset or is not a valid label value, since without a well-formed id pruning cannot be scoped to this
// context — a malformed id would otherwise build a selector the API server rejects, failing every
// placement with an opaque error rather than a message that names the bad id.
func (k *BaseKubernetesManager) PruneSecrets(desired map[string]map[string]bool) error {
	contextID := k.configHandler.GetString("id")
	if contextID == "" {
		return fmt.Errorf("context id not set; cannot scope secret pruning to this context")
	}
	if errs := validation.IsValidLabelValue(contextID); len(errs) > 0 {
		return fmt.Errorf("context id %q is not a valid label value, cannot scope secret pruning: %s", contextID, strings.Join(errs, "; "))
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	selector := fmt.Sprintf("windsorcli.dev/context-id=%s,%s", contextID, secretOwnerLabel)
	list, err := k.client.ListResourcesByLabel(gvr, "", selector)
	if err != nil {
		return fmt.Errorf("failed to list CLI-placed secrets: %w", err)
	}

	for i := range list.Items {
		item := list.Items[i]
		namespace := item.GetNamespace()
		if desired[namespace][item.GetName()] {
			continue
		}
		if err := k.client.DeleteResource(gvr, namespace, item.GetName(), metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete orphaned secret %q in namespace %q: %w", item.GetName(), namespace, err)
		}
	}
	return nil
}

// RollWorkloadsForSecret rolls the workloads in a namespace that consume the named Secret so they pick
// up new content, the way Kubernetes only ever rolls on a pod-template change. Because the CLI holds the
// resolved plaintext, it passes a precomputed content digest and stamps it as a pod-template annotation
// (checksum.windsorcli.dev/<secret>) on every Deployment, StatefulSet, and DaemonSet whose pod spec
// references the Secret via envFrom, a secretKeyRef, or a secret volume — including init containers. The
// digest is one-way and never surfaced beyond the namespace, so a reader of the annotation learns
// nothing the Secret's own RBAC did not already grant. A workload already carrying the digest is left
// untouched (idempotent, so unchanged content does not churn pods); a workload that does not reference
// the Secret is never patched. Finding no consumers is not an error — on a first apply the workload is
// created later by Flux and reads the Secret fresh — so this returns an error only on an API failure. The
// caller's context bounds the patch calls so a slow API server cannot outlast its deadline.
func (k *BaseKubernetesManager) RollWorkloadsForSecret(ctx context.Context, namespace, secretName, digest string) error {
	annotationKey := secretChecksumAnnotationKey(secretName)
	for _, resource := range []string{"deployments", "statefulsets", "daemonsets"} {
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: resource}
		list, err := k.client.ListResources(gvr, namespace)
		if err != nil {
			return fmt.Errorf("listing %s in namespace %q: %w", resource, namespace, err)
		}
		if list == nil {
			continue
		}
		for i := range list.Items {
			obj := &list.Items[i]
			podSpec, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
			if err != nil || !found || !podSpecReferencesSecret(podSpec, secretName) {
				continue
			}
			current, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "metadata", "annotations", annotationKey)
			if current == digest {
				continue
			}
			patch, err := json.Marshal(map[string]any{
				"spec": map[string]any{
					"template": map[string]any{
						"metadata": map[string]any{
							"annotations": map[string]any{annotationKey: digest},
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("building rollout patch for %s %q: %w", resource, obj.GetName(), err)
			}
			opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
			if _, err := k.client.PatchResource(ctx, gvr, namespace, obj.GetName(), types.MergePatchType, patch, opts); err != nil {
				return fmt.Errorf("patching %s %q in namespace %q: %w", resource, obj.GetName(), namespace, err)
			}
		}
	}
	return nil
}

// ApplyVersionMarker writes the applied-version marker ConfigMap to the namespace, recording which
// blueprint version the context is running. The marker is stored as JSON in a single ConfigMap so
// its encoding can evolve without churning Kustomization labels.
func (k *BaseKubernetesManager) ApplyVersionMarker(namespace string, marker VersionMarker) error {
	data, err := marker.ToConfigMapData()
	if err != nil {
		return fmt.Errorf("failed to encode version marker: %w", err)
	}
	return k.ApplyConfigMap(VersionMarkerConfigMapName, namespace, data)
}

// GetVersionMarker reads the applied-version marker ConfigMap from the namespace, reporting false
// when no marker is present — a missing ConfigMap (pre-bootstrap context) or one without marker data
// (legacy cluster). It returns an error only on a real read or decode failure, so callers can tell
// "no marker yet" (proceed as legacy) apart from "could not read the marker" (cluster unreachable).
func (k *BaseKubernetesManager) GetVersionMarker(namespace string) (VersionMarker, bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	obj, err := k.client.GetResource(gvr, namespace, VersionMarkerConfigMapName)
	if err != nil {
		if isNotFoundError(err) {
			return VersionMarker{}, false, nil
		}
		return VersionMarker{}, false, fmt.Errorf("failed to read version marker: %w", err)
	}
	data, found, err := unstructured.NestedStringMap(obj.Object, "data")
	if err != nil {
		return VersionMarker{}, false, fmt.Errorf("failed to read version marker data: %w", err)
	}
	if !found {
		return VersionMarker{}, false, nil
	}
	return ParseVersionMarker(data)
}

// =============================================================================
// Helpers
// =============================================================================

// secretChecksumAnnotationKey builds the pod-template annotation key for a secret's content digest. The
// name segment after the prefix must be at most 63 characters, but Secret names may be longer; for an
// over-long name it substitutes a deterministic, collision-resistant segment (the truncated name plus a
// short hash of the full name) so the roll still works and the key stays valid rather than failing with
// an opaque API validation error.
func secretChecksumAnnotationKey(secretName string) string {
	const maxNameSegment = 63
	segment := secretName
	if len(segment) > maxNameSegment {
		sum := sha256.Sum256([]byte(secretName))
		segment = secretName[:maxNameSegment-9] + "-" + hex.EncodeToString(sum[:])[:8]
	}
	return secretChecksumAnnotationPrefix + segment
}

// podSpecReferencesSecret reports whether an unstructured pod spec consumes the named Secret through any
// path that requires a restart to pick up new content: an envFrom secretRef, an env secretKeyRef, or a
// secret volume, across both containers and init containers.
func podSpecReferencesSecret(podSpec map[string]any, secretName string) bool {
	if volumes, ok := podSpec["volumes"].([]any); ok {
		for _, v := range volumes {
			vm, _ := v.(map[string]any)
			if s, ok := vm["secret"].(map[string]any); ok {
				if name, _ := s["secretName"].(string); name == secretName {
					return true
				}
			}
		}
	}
	for _, field := range []string{"containers", "initContainers"} {
		containers, ok := podSpec[field].([]any)
		if !ok {
			continue
		}
		for _, c := range containers {
			cm, _ := c.(map[string]any)
			if envFrom, ok := cm["envFrom"].([]any); ok {
				for _, e := range envFrom {
					em, _ := e.(map[string]any)
					if sr, ok := em["secretRef"].(map[string]any); ok {
						if name, _ := sr["name"].(string); name == secretName {
							return true
						}
					}
				}
			}
			if env, ok := cm["env"].([]any); ok {
				for _, e := range env {
					em, _ := e.(map[string]any)
					vf, ok := em["valueFrom"].(map[string]any)
					if !ok {
						continue
					}
					if skr, ok := vf["secretKeyRef"].(map[string]any); ok {
						if name, _ := skr["name"].(string); name == secretName {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// secretTypeAndData resolves the Secret type and stringData ApplySecret should apply. stringData
// already carrying dockerConfigSecretKey passes through unchanged as kubernetes.io/dockerconfigjson.
// Otherwise, docker-username plus docker-password (docker-server optional, defaulting to
// defaultDockerConfigServer) synthesizes a dockerConfigSecretKey entry and drops the docker-* keys,
// also as kubernetes.io/dockerconfigjson. Anything else stays Opaque, unchanged. stringData is never
// mutated; a new map is returned whenever synthesis applies.
func secretTypeAndData(stringData map[string]string) (string, map[string]string, error) {
	if stringData[dockerConfigSecretKey] != "" {
		return "kubernetes.io/dockerconfigjson", stringData, nil
	}

	username := stringData["docker-username"]
	password := stringData["docker-password"]
	if username == "" || password == "" {
		return "Opaque", stringData, nil
	}

	server := stringData["docker-server"]
	if server == "" {
		server = defaultDockerConfigServer
	}

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	dockerConfig := map[string]any{
		"auths": map[string]any{
			server: map[string]any{
				"username": username,
				"password": password,
				"auth":     auth,
			},
		},
	}
	encoded, err := json.Marshal(dockerConfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode docker config json: %w", err)
	}

	resolved := make(map[string]string, len(stringData))
	for k, v := range stringData {
		if k == "docker-username" || k == "docker-password" || k == "docker-server" {
			continue
		}
		resolved[k] = v
	}
	resolved[dockerConfigSecretKey] = string(encoded)

	return "kubernetes.io/dockerconfigjson", resolved, nil
}

// secretTypeChanged reports whether an existing Secret's type differs from newType. Kubernetes
// treats Secret.type as immutable after creation — the API server rejects an update that changes
// it — so ApplySecret deletes and recreates rather than SSA-merging when the resolved type (e.g.
// via docker-* key synthesis) differs from what's already there.
func secretTypeChanged(obj *unstructured.Unstructured, newType string) bool {
	existingType, _ := obj.Object["type"].(string)
	return existingType != "" && existingType != newType
}

// isImmutableConfigMap checks if a ConfigMap is immutable
func isImmutableConfigMap(obj *unstructured.Unstructured) bool {
	if obj.GetKind() != "ConfigMap" {
		return false
	}

	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		return false
	}

	immutable, ok := spec["immutable"].(bool)
	return ok && immutable
}
