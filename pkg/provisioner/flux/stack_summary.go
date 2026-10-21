// The summary methods of the FluxStack return structured plan results for kustomizations.
// It provides PlanSummary, PlanDestroySummary, and the summaries for one component.
// It parses the output of flux diff and kustomize build into resource changes.

package flux

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
)

// =============================================================================
// Constants
// =============================================================================

const (
	ActionUnknown Action = iota
	ActionCreate
	ActionUpdate
	ActionDelete
)

// =============================================================================
// Types
// =============================================================================

// KustomizePlan holds the plan result for a single Flux kustomization.
// For kustomizations that already exist in the cluster, Added and Removed count
// diff lines from "flux diff". For new kustomizations (no cluster or not yet
// deployed), Added counts rendered resources from "kustomize build" and IsNew
// is true. Resources is the per-resource change list extracted from flux diff
// (existing kustomizations) or kustomize build (new kustomizations); it is empty
// when Degraded is true or when the diff produced no parseable resource banners.
// Degraded is true when the required CLI tool was absent and no counts could be
// produced. Err is non-nil when the component could not be planned.
type KustomizePlan struct {
	Name      string
	Added     int
	Removed   int
	IsNew     bool
	Degraded  bool
	Resources []ResourceChange
	Err       error
}

// Action enumerates the kinds of changes a kustomization plan can produce for a
// single Kubernetes resource. ActionUnknown is reserved for inputs that don't
// correspond to any operator-visible change (flux "unchanged"/"skipped" or
// unrecognised verbs) and is dropped from rendered resource lists. Replacement
// is intentionally absent: flux's SSA layer models replacements as a delete +
// create pair rather than a single banner verb, so no input can map to it.
type Action int

// ResourceChange identifies one Kubernetes resource changed by a kustomization
// plan along with its action. Address follows flux's own banner convention,
// "<Kind>/<namespace>/<name>" for namespaced resources and "<Kind>/<name>" for
// cluster-scoped ones.
type ResourceChange struct {
	Address string
	Action  Action
}

// =============================================================================
// Public Methods
// =============================================================================

// PlanSummary runs a best-effort plan for every non-destroyOnly kustomization in
// the blueprint, returning per-component counts without printing raw output.
// The second return value carries upgrade hints to display to the user when a
// required CLI tool is absent. Missing tools degrade gracefully: each component
// row is marked Degraded=true rather than returning an error entry. Cluster
// connectivity failures are also handled gracefully: when KustomizationExists
// returns an error the kustomization is treated as new and planned via kustomize
// build instead.
func (s *FluxStack) PlanSummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, []string) {
	if blueprint == nil {
		return nil, nil
	}

	var hints []string

	fluxMissing := false
	if _, err := s.shims.LookPath("flux"); err != nil {
		hints = append(hints, "flux CLI not found — install to see kustomize diffs for existing clusters\nhttps://fluxcd.io/flux/installation/")
		fluxMissing = true
	} else if err := s.checkFluxVersion(); err != nil {
		hints = append(hints, err.Error())
		fluxMissing = true
	}

	kustomizeMissing := false
	if _, err := s.shims.LookPath("kustomize"); err != nil {
		hints = append(hints, "kustomize CLI not found — install to see resource counts for new kustomizations\nhttps://kubectl.docs.kubernetes.io/installation/kustomize/")
		kustomizeMissing = true
	}

	namespace := s.gitopsNamespace()

	var results []KustomizePlan
	for _, k := range blueprint.Kustomizations {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		results = append(results, s.planOneKustomizeSummary(blueprint, k, namespace, fluxMissing, kustomizeMissing))
	}

	return results, hints
}

// PlanComponentSummary plans a single kustomization by name and returns its structured
// result. Only the requested kustomization is planned. If the kustomization is not found,
// a result with a non-nil Err is returned rather than an error, consistent with PlanSummary.
func (s *FluxStack) PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan {
	result := KustomizePlan{Name: name}

	if blueprint == nil {
		result.Err = fmt.Errorf("blueprint not provided")
		return result
	}

	k, ok := findKustomization(blueprint, name)
	if !ok {
		result.Err = fmt.Errorf("kustomization %q not found in blueprint", name)
		return result
	}

	fluxMissing := false
	if _, err := s.shims.LookPath("flux"); err != nil {
		fluxMissing = true
	} else if err := s.checkFluxVersion(); err != nil {
		fluxMissing = true
	}

	kustomizeMissing := false
	if _, err := s.shims.LookPath("kustomize"); err != nil {
		kustomizeMissing = true
	}

	namespace := s.gitopsNamespace()

	return s.planOneKustomizeSummary(blueprint, k, namespace, fluxMissing, kustomizeMissing)
}

// PlanDestroySummary previews what `windsor destroy` will remove at the flux
// layer, read from each Kustomization's live .status.inventory.entries. A
// kustomization absent from the cluster, or a cluster with no kubeconfig, is
// reported as not-deployed (IsNew=true).
//
// A kubeconfig that still names a since-destroyed cluster fails with a
// network-layer error, not a missing-file error. The first such failure marks
// the cluster unreachable for the rest of this call. Every remaining
// kustomization is then reported as not-deployed, not queried again. Any
// other error still propagates.
func (s *FluxStack) PlanDestroySummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, error) {
	if blueprint == nil {
		return nil, nil
	}

	namespace := s.gitopsNamespace()

	var results []KustomizePlan
	clusterUnreachable := false
	for _, k := range blueprint.Kustomizations {
		if !KustomizationDestroyEligible(k) {
			continue
		}
		if clusterUnreachable {
			results = append(results, notDeployedKustomizePlan(k.Name))
			continue
		}
		result, err := s.planOneKustomizeDestroySummary(k, namespace)
		if err != nil {
			if isClusterUnreachable(err) {
				clusterUnreachable = true
				results = append(results, notDeployedKustomizePlan(k.Name))
				continue
			}
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// PlanDestroyComponentSummary returns the destroy preview for a single named
// kustomization. A pinned destroy=false or DestroyOnly kustomization returns
// Err — DeleteBlueprint would skip it, so producing a plan would mislead.
func (s *FluxStack) PlanDestroyComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan {
	result := KustomizePlan{Name: name}

	if blueprint == nil {
		result.Err = fmt.Errorf("blueprint not provided")
		return result
	}

	k, ok := findKustomization(blueprint, name)
	if !ok {
		result.Err = fmt.Errorf("kustomization %q not found in blueprint", name)
		return result
	}
	if k.DestroyOnly != nil && *k.DestroyOnly {
		result.Err = fmt.Errorf("kustomization %q is destroyOnly; not eligible for direct destroy", name)
		return result
	}
	if d := k.Destroy.ToBool(); d != nil && !*d {
		result.Err = fmt.Errorf("kustomization %q is pinned with destroy=false", name)
		return result
	}

	resolved, err := s.planOneKustomizeDestroySummary(k, s.gitopsNamespace())
	if err != nil {
		resolved.Err = err
	}
	return resolved
}

// =============================================================================
// Private Methods
// =============================================================================

// planOneKustomizeSummary computes the summary plan result for a single kustomization.
// It is shared by PlanSummary (which iterates all kustomizations) and PlanComponentSummary
// (which targets one). fluxMissing and kustomizeMissing are pre-computed tool-availability
// flags so tool detection does not repeat per-component in the all-summary path.
func (s *FluxStack) planOneKustomizeSummary(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization, namespace string, fluxMissing, kustomizeMissing bool) KustomizePlan {
	result := KustomizePlan{Name: k.Name}

	exists, err := s.kubernetesManager.KustomizationExists(k.Name, namespace)
	if err != nil {
		exists = false
	}

	sourceRoot := s.resolveSourceRoot(blueprint, k)
	fluxK := k.ToFluxKustomization(namespace, blueprint.Metadata.Name, blueprint.Sources, s.gitopsMode())
	localPath := filepath.Join(sourceRoot, fluxK.Spec.Path)

	if exists {
		if fluxMissing {
			result.Degraded = true
		} else {
			stdout, diffErr := s.captureFluxDiff("diff", "kustomization", k.Name, "--namespace", namespace, "--path", localPath)
			if diffErr != nil {
				result.Err = diffErr
			} else {
				result.Added, result.Removed = countDiffLines(stdout)
				result.Resources = parseFluxDiffResources(stdout)
			}
		}
	} else {
		result.IsNew = true
		if kustomizeMissing {
			result.Degraded = true
		} else {
			stdout, buildErr := s.captureKustomizeBuild(k, fluxK.Spec.Components, localPath, sourceRoot)
			if buildErr != nil {
				result.Err = buildErr
			} else {
				result.Added = countKustomizeResources(stdout)
				result.Resources = parseKustomizeBuildResources(stdout)
			}
		}
	}

	return result
}

// planOneKustomizeDestroySummary computes the destroy preview for one
// kustomization. Pulls flux's live inventory and tags every entry as Delete.
// IsNew marks the not-deployed case so the renderer shows "(not deployed)" —
// reusing the apply-side IsNew field rather than introducing a new flag.
func (s *FluxStack) planOneKustomizeDestroySummary(k blueprintv1alpha1.Kustomization, namespace string) (KustomizePlan, error) {
	result := KustomizePlan{Name: k.Name}

	entries, err := s.kubernetesManager.GetKustomizationInventory(k.Name, namespace)
	if err != nil {
		// A missing kubeconfig means the cluster the kustomization would live on does
		// not exist — typically right after `windsor destroy` tore down the cluster's
		// terraform component. Treat as "not deployed" so subsequent destroy cycles
		// can still produce a plan.
		if errors.Is(err, os.ErrNotExist) {
			result.IsNew = true
			return result, nil
		}
		return result, werror.Wrap(err, "error querying kustomization %q inventory", k.Name)
	}
	if entries == nil {
		// Kustomization not present in the cluster: nothing to destroy.
		result.IsNew = true
		return result, nil
	}

	resources := make([]ResourceChange, 0, len(entries))
	for _, e := range entries {
		resources = append(resources, ResourceChange{
			Address: inventoryAddress(e),
			Action:  ActionDelete,
		})
	}
	result.Resources = resources
	result.Removed = len(resources)
	return result, nil
}

// captureFluxDiff runs "flux <args>" and returns stdout without printing.
// Exit code 1 (changes detected) is treated as success, matching runFluxDiff semantics.
// Any other non-zero exit code is returned as an error.
func (s *FluxStack) captureFluxDiff(args ...string) (string, error) {
	stdout, err := s.runtime.Shell.ExecCaptureWithEnv("flux", map[string]string{"NO_COLOR": "1"}, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return stdout, nil
		}
		return "", err
	}
	return stdout, nil
}

// =============================================================================
// Helpers
// =============================================================================

// KustomizationDestroyEligible mirrors the gate inside DeleteBlueprint so the
// destroy-plan set matches the destroy-execute set. DestroyOnly kustomizations
// are skipped (they're hooks applied during destroy, not user-visible
// resources to preview), and any pinned destroy=false are skipped. The
// explicit nil check on Destroy mirrors componentDestroyEnabled on the
// terraform side: ToBool happens to handle a nil receiver today, but the
// guard reads more clearly at the call site and survives future refactors of
// ToBool without breakage.
func KustomizationDestroyEligible(k blueprintv1alpha1.Kustomization) bool {
	if k.DestroyOnly != nil && *k.DestroyOnly {
		return false
	}
	if k.Destroy == nil {
		return true
	}
	if d := k.Destroy.ToBool(); d != nil && !*d {
		return false
	}
	return true
}

// notDeployedKustomizePlan builds the destroy-plan result for a kustomization known
// not to be on the cluster.
func notDeployedKustomizePlan(name string) KustomizePlan {
	return KustomizePlan{Name: name, IsNew: true}
}

// isClusterUnreachable reports whether err is a network-layer failure, not a
// response from the API server. A refused connection or a DNS failure counts:
// both fail during dial, before any connection exists. A client-side timeout
// also counts. A reset or closed connection does not: it had already reached
// a live server, so it is not treated as the cluster being gone. A TLS or
// certificate error implements net.Error too, but not as a dial *net.OpError
// or a real timeout, so it does not count either.
func isClusterUnreachable(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// inventoryAddress renders a Kubernetes inventory entry in flux's banner
// convention: "<Kind>/<namespace>/<name>" for namespaced resources,
// "<Kind>/<name>" for cluster-scoped ones. Group is intentionally omitted from
// the visible address — Kind is what operators recognise; group disambiguates
// CRDs but at the cost of every line getting a noisy "apps." or "networking.
// k8s.io." prefix. Same trade-off as the apply-side parser. Residual risk:
// when a cluster carries two CRDs with the same Kind across different groups
// (e.g., a vendor's Deployment alongside apps/Deployment), the rendered
// addresses collide and the operator must consult the cluster directly to
// distinguish them. Rare in practice, but worth knowing about when reviewing
// a plan against a heavily-customised cluster.
func inventoryAddress(e kubernetes.InventoryEntry) string {
	if e.Namespace == "" {
		return fmt.Sprintf("%s/%s", e.Kind, e.Name)
	}
	return fmt.Sprintf("%s/%s/%s", e.Kind, e.Namespace, e.Name)
}

// countDiffLines counts added and removed lines in a unified diff.
// Lines starting with "+" (but not "+++") are additions; lines starting with
// "-" (but not "---") are removals.
func countDiffLines(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}

// countKustomizeResources counts rendered Kubernetes resources in kustomize build output
// by counting top-level "kind:" lines (no leading whitespace). Only column-0 occurrences
// are counted to avoid matching "kind:" inside indented ConfigMap data or nested objects.
func countKustomizeResources(yaml string) int {
	count := 0
	for _, line := range strings.Split(yaml, "\n") {
		if strings.HasPrefix(line, "kind:") {
			count++
		}
	}
	return count
}

// parseFluxDiffResources extracts per-resource change banners from the stdout of
// `flux diff kustomization`. Flux v2.3+ prefixes each affected resource with "►"
// followed by "<Kind>/<namespace>/<name> <verb>" (or "<Kind>/<name> <verb>" for
// cluster-scoped resources). Recognised verbs map to the shared Action enum:
// "created"→Create, "drifted"/"configured"→Update, "deleted"→Delete. "unchanged"
// and "skipped" yield ActionUnknown and are dropped, since they don't represent
// pending work. Lines that don't match the banner pattern are ignored, so unified
// diff content under each banner does not leak into the result.
func parseFluxDiffResources(diff string) []ResourceChange {
	var result []ResourceChange
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "►") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "►"))
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		identifier := fields[0]
		verb := fields[len(fields)-1]
		action := mapFluxAction(verb)
		if action == ActionUnknown {
			continue
		}
		result = append(result, ResourceChange{Address: identifier, Action: action})
	}
	return result
}

// parseKustomizeBuildResources extracts the kind/namespace/name of each rendered
// resource in a kustomize build YAML stream. All resources are tagged
// ActionCreate, since this parser is invoked only for kustomizations that do not
// yet exist in the cluster — every rendered resource is net-new from the
// operator's perspective. Documents missing kind or metadata.name are skipped
// rather than returning an error: kustomize occasionally emits empty separator
// documents and we don't want them to fail the whole summary.
func parseKustomizeBuildResources(yamlStr string) []ResourceChange {
	var result []ResourceChange
	for _, raw := range yamlDocumentsToJSON(yamlStr) {
		var doc struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}
		if doc.Kind == "" || doc.Metadata.Name == "" {
			continue
		}
		var address string
		if doc.Metadata.Namespace != "" {
			address = fmt.Sprintf("%s/%s/%s", doc.Kind, doc.Metadata.Namespace, doc.Metadata.Name)
		} else {
			address = fmt.Sprintf("%s/%s", doc.Kind, doc.Metadata.Name)
		}
		result = append(result, ResourceChange{Address: address, Action: ActionCreate})
	}
	return result
}

// mapFluxAction maps the trailing verb in a flux diff resource banner to the
// shared Action enum. Verbs flux emits but that don't represent a change
// (unchanged, skipped) collapse to ActionUnknown so callers drop them.
func mapFluxAction(verb string) Action {
	switch verb {
	case "created":
		return ActionCreate
	case "drifted", "configured":
		return ActionUpdate
	case "deleted":
		return ActionDelete
	default:
		return ActionUnknown
	}
}
