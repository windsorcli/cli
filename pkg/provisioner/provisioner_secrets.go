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
// whose keys all resolve away. The result maps each owning kustomization to its secrets for PlaceSecrets.
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
				// Mask the materialized value in command output regardless of how it was sourced —
				// env(), a config reference, or secret() — matching the redaction secret() already
				// gets at resolution time.
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
				}
			}
		}
	}
	return resolved, nil
}

// PlaceSecrets materializes secrets that ResolveSecrets produced into their target namespace(s), and —
// when prune is set — reconciles by deleting the CLI-placed secrets this context no longer wants. It runs
// after Install. Placement is not serialized: on each round it places every secret whose target namespace
// already exists and defers the rest, then polls, so a secret whose namespace is ready is never blocked
// behind one whose namespace is not. This matters because kustomizations commonly depend on a secret the
// CLI places (e.g. a cloud-controller-manager needs its token before nodes initialize and downstream
// namespaces appear); placing the ready ones first breaks that cycle rather than deadlocking on a namespace
// that only appears once an earlier secret is placed. For each secret it resolves the target namespace(s) —
// the ones the entry named (gated on those existing in the cluster) or, when it named none, the namespace
// its owning kustomization creates or deploys into (failing closed on more than one so a secret is never
// placed by guessing) — applies an Opaque Secret into each, and rolls the workloads there that consume it,
// keyed by a content digest, so a changed value reaches running pods rather than sitting stale. It surfaces
// progress on the current operation naming what it is placing or waiting on, and times out naming the secrets whose
// namespaces never appeared. As it places a secret it requests an immediate flux reconcile of the owning
// kustomization (the secret is what that kustomization was waiting on), and while waiting it nudges the
// dependency closure of the still-pending owners on a throttle, so a chain unblocked by a placement
// advances in seconds rather than one flux interval per hop. When prune is set it records every
// (namespace, secret) it placed and hands
// that desired set to PruneSecrets, which reclaims any CLI-placed secret not in it. Pruning is gated so
// secret reclaim mirrors Kustomization prune: on under apply's --prune and upgrade, off for the place-only
// flows. When no kubernetes manager is configured it is a no-op only if there is also nothing to place.
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

// placeSecrets places the resolved secrets as PlaceSecrets describes. It reports each secret that
// waits for its namespace, or that it places, as progress of the owning kustomization in layers.
// It ends the step of each kustomization that the readiness poll finds Ready. A timeout while it
// waits for the namespaces returns an error with a remediation hint.
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
				if err := i.KubernetesManager.ApplySecret(p.secretName, namespace, p.secret.Data, p.kustomization); err != nil {
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

		// A secret we just placed is what its owner was waiting on — reconcile that owner now so it
		// applies and re-checks readiness immediately, and force any HelmRelease it owns that had
		// stalled for lack of the secret, rather than waiting on its scheduled interval.
		if len(placedOwners) > 0 {
			owners := sortedStringKeys(placedOwners)
			i.reconcileKustomizations(ctx, owners)
			i.forceStalledHelmReleases(ctx, owners)
		}
		if len(pending) == 0 {
			break
		}
		// Drive the DAG toward creating the still-pending namespaces by nudging only the frontier —
		// not-Ready kustomizations whose dependencies are all Ready — once each. Already-Ready
		// kustomizations (e.g. crds) are never touched, and a member blocked upstream is left alone
		// until its dependency clears, so this advances the chain without churning healthy resources.
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
