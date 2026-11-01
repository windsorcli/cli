// The secret methods of the Provisioner resolve and place the secrets of flux systems.
// It provides ResolveSecrets, which resolves every secret before Install, and PlaceSecrets.
// PlaceSecrets writes each secret into its target namespaces and prunes secrets that the context does not use.

package provisioner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Constants
// =============================================================================

// namespaceInferenceHint is the remediation for a secret whose target namespace Windsor cannot infer.
const namespaceInferenceHint = "Set namespaces: on the secret to choose where to place it."

// =============================================================================
// Types
// =============================================================================

// ResolvedSecrets maps an owning kustomization name to the Secrets ready to place in the namespace(s)
// it creates: Secret name -> ResolvedSecret (target namespaces plus resolved plaintext data). It holds
// resolved secret material in memory only (never serialized), produced by ResolveSecrets before Install
// and consumed by PlaceSecrets after.
type ResolvedSecrets map[string]map[string]ResolvedSecret

// ResolvedSecret is one Secret ready to place: its resolved plaintext data and the namespaces it targets.
// Namespaces is empty when the authored entry named none, in which case placement auto-resolves the single
// namespace the owning kustomization creates.
type ResolvedSecret struct {
	Namespaces []string
	Data       map[string]string
	Type       string
}

// pendingPlacement is one secret awaiting its target namespace during PlaceSecrets.
type pendingPlacement struct {
	kustomization string
	secretName    string
	secret        ResolvedSecret
}

// =============================================================================
// Public Methods
// =============================================================================

// ResolveSecrets resolves the secrets of every flux system to plaintext before Install.
// A misconfigured secret then fails the command before Windsor applies anything to the cluster.
// A reference that resolves to nil or to an empty string returns an error with a remediation hint,
// unless it has a ?? default. A ?? default makes the key optional. Windsor does not create a secret
// whose keys all resolve away. The shell masks each resolved value in command output, whatever its
// source. The result maps each owning kustomization to its secrets for PlaceSecrets.
func (i *Provisioner) ResolveSecrets(blueprint *blueprintv1alpha1.Blueprint) (ResolvedSecrets, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	resolved := make(ResolvedSecrets)
	bp := withCrdLayer(blueprint)
	for _, k := range bp.Kustomizations {
		if len(k.Secrets) == 0 {
			continue
		}
		for secretName, entry := range k.Secrets {
			stringData := make(map[string]string, len(entry.Data))
			for key, ref := range entry.Data {
				value, err := i.evaluator.Evaluate(ref, "", nil, true)
				if err != nil {
					return nil, werror.Wrap(err, "resolving secret %q key %q", secretName, key)
				}
				s := ""
				if value != nil {
					s = fmt.Sprint(value)
				}
				if s == "" {
					if strings.Contains(ref, "??") {
						continue
					}
					return nil, werror.New(fmt.Sprintf("resolving secret %q key %q: reference %q resolved to empty", secretName, key, ref), "Add a ?? default, for example ${... ?? ''}, to make the key optional.", nil)
				}
				i.shell.RegisterSecret(s)
				stringData[key] = s
			}
			if len(stringData) > 0 {
				if resolved[k.Name] == nil {
					resolved[k.Name] = make(map[string]ResolvedSecret)
				}
				resolved[k.Name][secretName] = ResolvedSecret{
					Namespaces: slices.Clone(entry.Namespaces),
					Data:       stringData,
					Type:       entry.Type,
				}
			}
		}
	}
	return resolved, nil
}

// PlaceSecrets writes the secrets that ResolveSecrets returned into their namespaces, and rolls the
// workloads that consume each one. Each round places every secret whose namespace exists and polls
// for the others, so a secret never waits behind another. A secret goes to the namespaces its entry
// names, or else to the one namespace of its kustomization. prune removes placed secrets that the
// context no longer declares. Without a kubernetes manager, it does nothing only when nothing waits.
func (i *Provisioner) PlaceSecrets(ctx context.Context, resolved ResolvedSecrets, blueprint *blueprintv1alpha1.Blueprint, prune bool) error {
	if i.KubernetesManager == nil {
		if len(resolved) == 0 {
			return nil
		}
		return fmt.Errorf("kubernetes manager not configured")
	}
	if len(resolved) == 0 && !prune {
		return nil
	}

	return i.placeSecrets(ctx, stepProgress{ctx}, resolved, blueprint, prune)
}

// =============================================================================
// Private Methods
// =============================================================================

// placeSecrets places the resolved secrets as PlaceSecrets describes, and reports each wait and each
// placement as progress of the owning kustomization in layers. After a placement it reconciles the
// owner and forces its stalled HelmReleases. While secrets wait, it nudges the frontier, as
// nudgeFrontier describes, and ends the step of each Ready kustomization. A timeout returns an error
// with a remediation hint.
func (i *Provisioner) placeSecrets(ctx context.Context, layers layerReporter, resolved ResolvedSecrets, blueprint *blueprintv1alpha1.Blueprint, prune bool) error {
	pending := make([]pendingPlacement, 0)
	for kustomizationName, secrets := range resolved {
		for secretName, secret := range secrets {
			pending = append(pending, pendingPlacement{kustomizationName, secretName, secret})
		}
	}

	placed := make(map[string]map[string]bool)
	pollInterval := i.pollInterval()
	waitCtx, cancel := context.WithTimeout(ctx, constants.DefaultFluxKustomizationInstallTimeout)
	defer cancel()

	nudged := make(map[string]struct{})
	for len(pending) > 0 {
		var stillPending []pendingPlacement
		placedOwners := make(map[string]struct{})
		for _, p := range pending {
			namespaces, ready, err := i.trySecretNamespaces(p.kustomization, p.secret.Namespaces)
			if err != nil {
				return err
			}
			if !ready {
				stillPending = append(stillPending, p)
				layers.Progress(p.kustomization, fmt.Sprintf("waiting on a namespace for secret %s", p.secretName))
				continue
			}
			for _, namespace := range namespaces {
				if err := i.KubernetesManager.ApplySecret(p.secretName, namespace, p.secret.Type, p.secret.Data, p.kustomization); err != nil {
					return werror.Wrap(err, "applying secret %q to namespace %q", p.secretName, namespace)
				}
				if err := i.KubernetesManager.RollWorkloadsForSecret(ctx, namespace, p.secretName, secretDigest(p.secret.Data)); err != nil {
					return werror.Wrap(err, "rolling workloads for secret %q in namespace %q", p.secretName, namespace)
				}
				if placed[namespace] == nil {
					placed[namespace] = make(map[string]bool)
				}
				placed[namespace][p.secretName] = true
			}
			placedOwners[p.kustomization] = struct{}{}
			layers.Progress(p.kustomization, fmt.Sprintf("placed secret %s", p.secretName))
		}
		pending = stillPending

		if len(placedOwners) > 0 {
			owners := sortedStringKeys(placedOwners)
			i.reconcileKustomizations(ctx, owners)
			i.forceStalledHelmReleases(ctx, owners)
		}
		if len(pending) == 0 {
			break
		}
		notReady, reconciling := i.nudgeFrontier(ctx, blueprint, nudged)
		trackLayers(layers, convergeNames(blueprint), notReady, reconciling)
		select {
		case <-waitCtx.Done():
			return werror.New(fmt.Sprintf("timed out waiting on namespaces before placing secrets: %s", pendingSummary(pending)), "Make sure that the kustomizations that create these namespaces become ready, then run the command again.", nil)
		case <-time.After(pollInterval):
		}
	}

	if !prune {
		return nil
	}
	if err := i.KubernetesManager.PruneSecrets(placed); err != nil {
		return werror.Wrap(err, "pruning orphaned secrets")
	}
	return nil
}

// trySecretNamespaces finds the target namespaces of a secret and reports whether they all exist.
// It waits for each explicit namespace to exist in the cluster. With no explicit namespaces, it uses
// the namespace in the Flux inventory of the owning kustomization. An inventory with more than one
// namespace, or with none, returns an error with a remediation hint. (nil, false, nil) means: poll again.
func (i *Provisioner) trySecretNamespaces(kustomizationName string, explicit []string) ([]string, bool, error) {
	if len(explicit) > 0 {
		missing, err := i.missingClusterNamespaces(explicit)
		if err != nil {
			return nil, false, err
		}
		if len(missing) == 0 {
			return slices.Clone(explicit), true, nil
		}
		return nil, false, nil
	}
	entries, err := i.KubernetesManager.GetKustomizationInventory(kustomizationName, i.fluxNamespace())
	if err != nil {
		return nil, false, werror.Wrap(err, "reading inventory for kustomization %q", kustomizationName)
	}
	candidates := autoResolveNamespaces(entries)
	switch {
	case len(candidates) == 1:
		return candidates, true, nil
	case len(candidates) > 1:
		return nil, false, werror.New(fmt.Sprintf("kustomization %q spans multiple namespaces (%s)", kustomizationName, strings.Join(candidates, ", ")), namespaceInferenceHint, nil)
	case len(entries) > 0:
		return nil, false, werror.New(fmt.Sprintf("kustomization %q creates no namespace and deploys no namespaced resources to infer one from", kustomizationName), namespaceInferenceHint, nil)
	}
	return nil, false, nil
}

// missingClusterNamespaces returns the members of want that do not yet exist in the cluster,
// preserving want's order.
func (i *Provisioner) missingClusterNamespaces(want []string) ([]string, error) {
	var missing []string
	for _, ns := range want {
		exists, err := i.KubernetesManager.NamespaceExists(ns)
		if err != nil {
			return nil, werror.Wrap(err, "checking namespace %q", ns)
		}
		if !exists {
			missing = append(missing, ns)
		}
	}
	return missing, nil
}

// =============================================================================
// Helpers
// =============================================================================

// secretDigest returns a stable content digest for a secret's resolved data. Consumers use it to
// roll workloads only when the content changes. Keys are sorted, and each key and value is
// NUL-delimited so distinct maps cannot collide, then hashed with SHA-256. The digest is one-way:
// it detects change without revealing the plaintext.
func secretDigest(stringData map[string]string) string {
	keys := make([]string, 0, len(stringData))
	for k := range stringData {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(stringData[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pendingSummary renders secrets still awaiting a namespace as sorted placementLabel entries.
func pendingSummary(pending []pendingPlacement) string {
	parts := make([]string, 0, len(pending))
	for _, p := range pending {
		parts = append(parts, placementLabel(p))
	}
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}

// placementLabel names a pending secret as "secret (kustomization)", followed by "→" and its
// target namespaces when the entry declared them.
func placementLabel(p pendingPlacement) string {
	if len(p.secret.Namespaces) > 0 {
		return fmt.Sprintf("%s (%s)→%s", p.secretName, p.kustomization, strings.Join(p.secret.Namespaces, ","))
	}
	return fmt.Sprintf("%s (%s)", p.secretName, p.kustomization)
}

// autoResolveNamespaces infers where a secret should land when its entry names no namespace, using
// the owning kustomization's Flux inventory. It prefers namespaces the kustomization creates, and
// falls back to the namespaces its resources deploy into. The result is sorted.
func autoResolveNamespaces(entries []kubernetes.InventoryEntry) []string {
	created := make(map[string]struct{})
	deployed := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Kind == "Namespace" {
			created[entry.Name] = struct{}{}
		}
		if entry.Namespace != "" {
			deployed[entry.Namespace] = struct{}{}
		}
	}
	chosen := created
	if len(chosen) == 0 {
		chosen = deployed
	}
	out := make([]string, 0, len(chosen))
	for ns := range chosen {
		out = append(out, ns)
	}
	slices.Sort(out)
	return out
}
