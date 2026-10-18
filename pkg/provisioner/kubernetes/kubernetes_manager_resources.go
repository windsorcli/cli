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

	"github.com/windsorcli/cli/internal/werror"
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

// secretChecksumAnnotationPrefix starts the pod-template annotation that holds the digest of one consumed Secret.
const secretChecksumAnnotationPrefix = "checksum.windsorcli.dev/"

// secretOwnerLabel names the Kustomization of a Secret that the CLI placed. Only ApplySecret sets it.
const secretOwnerLabel = "windsorcli.dev/secret-owner" // #nosec G101 -- label key, not a credential

// dockerConfigSecretKey is the stringData key of the registry auth in a dockerconfigjson Secret.
const dockerConfigSecretKey = ".dockerconfigjson" // #nosec G101 -- well-known stringData key, not a credential

// defaultDockerConfigServer is the registry that secretTypeAndData uses when docker-server is not set.
const defaultDockerConfigServer = "ghcr.io"

// =============================================================================
// Public Methods
// =============================================================================

// ApplyConfigMap creates or updates a ConfigMap using SSA. It returns a WindsorError, and writes
// nothing, when a value still holds an unresolved expression such as terraform_output().
func (k *BaseKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	for key, value := range data {
		if evaluator.ContainsExpression(value) {
			return werror.New(fmt.Sprintf("ConfigMap %s has an unresolved expression in key %q.", name, key),
				"Run windsor apply or windsor upgrade to resolve the expression first.", nil)
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
		return werror.Wrap(err, "invalid configmap fields")
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && isImmutableConfigMap(existing) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return werror.Wrap(err, "failed to delete immutable configmap")
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// ApplySecret creates or updates a Secret using SSA, with plaintext values in stringData that it never logs.
// docker-username and docker-password, or a .dockerconfigjson key, give a dockerconfigjson Secret; other
// data gives an Opaque one. Kubernetes rejects a change of Secret.type, so it deletes a Secret of another
// type first. It sets the context labels and a secret-owner label that names owner, for PruneSecrets.
func (k *BaseKubernetesManager) ApplySecret(name, namespace string, stringData map[string]string, owner string) error {
	secretType, resolvedData, err := secretTypeAndData(stringData)
	if err != nil {
		return werror.Wrap(err, "failed to resolve secret type for %q", name)
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
		return werror.Wrap(err, "invalid secret fields")
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	existing, err := k.client.GetResource(gvr, namespace, name)
	if err == nil && secretTypeChanged(existing, secretType) {
		if err := k.client.DeleteResource(gvr, namespace, name, metav1.DeleteOptions{}); err != nil {
			return werror.Wrap(err, "failed to delete secret with changed type")
		}
		time.Sleep(time.Second)
	}

	opts := metav1.ApplyOptions{
		FieldManager: "windsor-cli",
		Force:        false,
	}

	return k.applyWithRetry(gvr, obj, opts)
}

// PruneSecrets deletes each secret that the CLI placed for this context and that desired does not hold.
// desired maps a namespace to the set of secret names just placed there.
// It selects on the context id and the secret-owner label, which only ApplySecret sets.
// Thus it never deletes a Flux secret that has the context labels from CommonMetadata.
// It returns a WindsorError when the context id is empty or is not a valid label value.
func (k *BaseKubernetesManager) PruneSecrets(desired map[string]map[string]bool) error {
	contextID := k.configHandler.GetString("id")
	if contextID == "" {
		return werror.New("The context has no id, so windsor cannot find the secrets that it placed.",
			"Run windsor init to set the context id.", nil)
	}
	if errs := validation.IsValidLabelValue(contextID); len(errs) > 0 {
		return werror.New(fmt.Sprintf("Context id %q is not a valid Kubernetes label value: %s.", contextID, strings.Join(errs, "; ")),
			"Set the context id to 63 characters or fewer: letters, digits, '-', '_', or '.', with a letter or digit at each end.", nil)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	selector := fmt.Sprintf("windsorcli.dev/context-id=%s,%s", contextID, secretOwnerLabel)
	list, err := k.client.ListResourcesByLabel(gvr, "", selector)
	if err != nil {
		return werror.Wrap(err, "failed to list CLI-placed secrets")
	}

	for i := range list.Items {
		item := list.Items[i]
		namespace := item.GetNamespace()
		if desired[namespace][item.GetName()] {
			continue
		}
		if err := k.client.DeleteResource(gvr, namespace, item.GetName(), metav1.DeleteOptions{}); err != nil {
			return werror.Wrap(err, "failed to delete orphaned secret %q in namespace %q", item.GetName(), namespace)
		}
	}
	return nil
}

// RollWorkloadsForSecret sets digest as a pod-template annotation on each Deployment, StatefulSet, and
// DaemonSet in namespace that consumes secretName, so Kubernetes rolls it. A workload consumes the Secret
// through envFrom, a secretKeyRef, or a secret volume, in any container or init container. It skips a
// workload that already has digest. It returns an error only on an API failure, and ctx bounds the patches.
func (k *BaseKubernetesManager) RollWorkloadsForSecret(ctx context.Context, namespace, secretName, digest string) error {
	annotationKey := secretChecksumAnnotationKey(secretName)
	for _, resource := range []string{"deployments", "statefulsets", "daemonsets"} {
		gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: resource}
		list, err := k.client.ListResources(gvr, namespace)
		if err != nil {
			return werror.Wrap(err, "listing %s in namespace %q", resource, namespace)
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
				return werror.Wrap(err, "building rollout patch for %s %q", resource, obj.GetName())
			}
			opts := metav1.PatchOptions{FieldManager: "windsor-cli"}
			if _, err := k.client.PatchResource(ctx, gvr, namespace, obj.GetName(), types.MergePatchType, patch, opts); err != nil {
				return werror.Wrap(err, "patching %s %q in namespace %q", resource, obj.GetName(), namespace)
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
		return werror.Wrap(err, "failed to encode version marker")
	}
	return k.ApplyConfigMap(VersionMarkerConfigMapName, namespace, data)
}

// GetVersionMarker reads the version marker ConfigMap from namespace. It returns false when the
// ConfigMap is missing, before bootstrap, or has no data, on a legacy cluster. It returns an error only
// when a read or decode fails, so a caller can tell "no marker" from "cluster unreachable".
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
		return VersionMarker{}, false, werror.Wrap(err, "failed to read version marker")
	}
	data, found, err := unstructured.NestedStringMap(obj.Object, "data")
	if err != nil {
		return VersionMarker{}, false, werror.Wrap(err, "failed to read version marker data")
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
		return "", nil, werror.Wrap(err, "failed to encode docker config json")
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
