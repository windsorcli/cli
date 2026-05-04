// Package flux provides Flux kustomization stack management functionality.
// It provides a unified interface for planning Flux kustomization changes
// by shelling out to the flux CLI (https://fluxcd.io/flux/installation/).
// The FluxStack is the primary orchestrator for Flux-based operations,
// coordinating shell operations and blueprint handling.

package flux

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/tui"
	sigsyaml "sigs.k8s.io/yaml"
)

// minFluxMajor and minFluxMinor define the minimum supported flux CLI version (v2.3.0).
// flux diff kustomization --kustomization-file was introduced in v2.3.0.
const (
	minFluxMajor = 2
	minFluxMinor = 3
)

// =============================================================================
// Interface
// =============================================================================

// Stack defines the interface for Flux kustomization operations.
type Stack interface {
	Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAll(blueprint *blueprintv1alpha1.Blueprint) error
	PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error
	PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error
	PlanSummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, []string)
	PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan
	PlanDestroySummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, error)
	PlanDestroyComponentSummary(blueprint *blueprintv1alpha1.Blueprint, name string) KustomizePlan
}

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

const (
	ActionUnknown Action = iota
	ActionCreate
	ActionUpdate
	ActionDelete
)

// ResourceChange identifies one Kubernetes resource changed by a kustomization
// plan along with its action. Address follows flux's own banner convention,
// "<Kind>/<namespace>/<name>" for namespaced resources and "<Kind>/<name>" for
// cluster-scoped ones.
type ResourceChange struct {
	Address string
	Action  Action
}

// =============================================================================
// Types
// =============================================================================

// FluxStack manages Flux kustomization operations by invoking the flux CLI.
// It resolves the target namespace from configuration, checks cluster state,
// and selects between a standard diff (kustomization exists) and a scratch diff
// (kustomization not yet deployed) for each requested component.
type FluxStack struct {
	runtime           *runtime.Runtime
	shims             *Shims
	kubernetesManager kubernetes.KubernetesManager
}

// =============================================================================
// Constructor
// =============================================================================

// NewStack creates a new FluxStack. Panics if runtime or kubernetesManager are nil.
func NewStack(rt *runtime.Runtime, kubernetesManager kubernetes.KubernetesManager, opts ...*FluxStack) Stack {
	if rt == nil {
		panic("runtime is required")
	}
	if kubernetesManager == nil {
		panic("kubernetes manager is required")
	}

	stack := &FluxStack{
		runtime:           rt,
		shims:             NewShims(),
		kubernetesManager: kubernetesManager,
	}

	if len(opts) > 0 && opts[0] != nil {
		if opts[0].shims != nil {
			stack.shims = opts[0].shims
		}
		if opts[0].kubernetesManager != nil {
			stack.kubernetesManager = opts[0].kubernetesManager
		}
	}

	return stack
}

// =============================================================================
// Public Methods
// =============================================================================

// PlanAll runs flux diff for every non-destroyOnly kustomization in the blueprint.
// The flux CLI is only required when there are kustomizations to plan; blueprints
// with no non-destroyOnly kustomizations succeed without it.
func (s *FluxStack) PlanAll(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	namespace := s.gitopsNamespace()

	var targets []blueprintv1alpha1.Kustomization
	for _, k := range blueprint.Kustomizations {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		targets = append(targets, k)
	}

	if len(targets) == 0 {
		return nil
	}

	if _, err := s.shims.LookPath("flux"); err != nil {
		return fmt.Errorf("flux CLI is required for 'plan kustomize'\nInstall: https://fluxcd.io/flux/installation/")
	}
	if err := s.checkFluxVersion(); err != nil {
		return err
	}

	for _, k := range targets {
		if err := s.planOne(blueprint, k, namespace); err != nil {
			return err
		}
	}
	return nil
}

// Plan runs flux diff for a single kustomization identified by componentID.
// Requires the flux CLI to be installed. Returns an error if the flux CLI is not found,
// the kustomization name is not in the blueprint, or the diff fails.
// If the kustomization does not yet exist in the cluster, the plan is generated from
// the blueprint definition via --kustomization-file.
func (s *FluxStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("flux"); err != nil {
		return fmt.Errorf("flux CLI is required for 'plan kustomize'\nInstall: https://fluxcd.io/flux/installation/")
	}
	if err := s.checkFluxVersion(); err != nil {
		return err
	}

	namespace := s.gitopsNamespace()

	k, found := findKustomization(blueprint, componentID)
	if !found {
		return fmt.Errorf("kustomization %q not found in blueprint", componentID)
	}
	return s.planOne(blueprint, k, namespace)
}

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

// PlanDestroySummary returns the per-kustomization preview of what
// `windsor destroy` will tear down at the flux layer. Unlike PlanSummary,
// this is sourced from the cluster's live state — specifically the
// .status.inventory.entries on each Kustomization, which is exactly what flux
// uses to drive prune behavior — rather than from `kustomize build` of the
// blueprint. Showing blueprint-derived resources for a destroy preview would
// lie when the cluster has drifted, so we read truth from the cluster.
//
// The kustomization set mirrors DeleteBlueprint's eligibility gate: regular
// kustomizations only (DestroyOnly hooks are applied during destroy and are
// not user-facing here), and any pinned with destroy=false are filtered out.
// A kustomization that is absent from the cluster yields IsNew=true so the
// renderer shows "(not deployed)". An error from the cluster propagates as
// the function-level error, since destroy itself cannot proceed without
// cluster connectivity — falling back would produce a misleading plan.
func (s *FluxStack) PlanDestroySummary(blueprint *blueprintv1alpha1.Blueprint) ([]KustomizePlan, error) {
	if blueprint == nil {
		return nil, nil
	}

	namespace := s.gitopsNamespace()

	var results []KustomizePlan
	for _, k := range blueprint.Kustomizations {
		if !kustomizationDestroyEligible(k) {
			continue
		}
		result, err := s.planOneKustomizeDestroySummary(k, namespace)
		if err != nil {
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

// kustomizationDestroyEligible mirrors the gate inside DeleteBlueprint so the
// destroy-plan set matches the destroy-execute set. DestroyOnly kustomizations
// are skipped (they're hooks applied during destroy, not user-visible
// resources to preview), and any pinned destroy=false are skipped. The
// explicit nil check on Destroy mirrors componentDestroyEnabled on the
// terraform side: ToBool happens to handle a nil receiver today, but the
// guard reads more clearly at the call site and survives future refactors of
// ToBool without breakage.
func kustomizationDestroyEligible(k blueprintv1alpha1.Kustomization) bool {
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

// planOneKustomizeDestroySummary computes the destroy preview for one
// kustomization. Pulls flux's live inventory and tags every entry as Delete.
// IsNew marks the not-deployed case so the renderer shows "(not deployed)" —
// reusing the apply-side IsNew field rather than introducing a new flag.
func (s *FluxStack) planOneKustomizeDestroySummary(k blueprintv1alpha1.Kustomization, namespace string) (KustomizePlan, error) {
	result := KustomizePlan{Name: k.Name}

	entries, err := s.kubernetesManager.GetKustomizationInventory(k.Name, namespace)
	if err != nil {
		return result, fmt.Errorf("error querying kustomization %q inventory: %w", k.Name, err)
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

// PlanAllJSON runs kustomize build for every non-destroyOnly kustomization in the blueprint,
// converts the rendered YAML manifests to JSON, and writes a JSON array of
// {"kustomization": name, "resources": [...]} objects to stdout. Requires the kustomize CLI.
func (s *FluxStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("kustomize"); err != nil {
		return fmt.Errorf("kustomize CLI is required for 'plan kustomize --json'\nInstall: https://kubectl.docs.kubernetes.io/installation/kustomize/")
	}

	namespace := s.gitopsNamespace()

	var targets []blueprintv1alpha1.Kustomization
	for _, k := range blueprint.Kustomizations {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		targets = append(targets, k)
	}

	return s.encodeKustomizationsJSON(os.Stdout, blueprint, namespace, targets)
}

// PlanJSON runs kustomize build for a single kustomization identified by componentID,
// converts the rendered YAML manifests to JSON, and writes a JSON array of
// {"kustomization": name, "resources": [...]} objects to stdout.
// Unlike Plan, this always uses kustomize build regardless of cluster state, producing
// the full desired state as JSON for machine consumption. Requires the kustomize CLI.
func (s *FluxStack) PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("kustomize"); err != nil {
		return fmt.Errorf("kustomize CLI is required for 'plan kustomize --json'\nInstall: https://kubectl.docs.kubernetes.io/installation/kustomize/")
	}

	k, found := findKustomization(blueprint, componentID)
	if !found {
		return fmt.Errorf("kustomization %q not found in blueprint", componentID)
	}

	namespace := s.gitopsNamespace()

	return s.encodeKustomizationsJSON(os.Stdout, blueprint, namespace, []blueprintv1alpha1.Kustomization{k})
}

// =============================================================================
// Private Methods
// =============================================================================

// gitopsNamespace returns the configured gitops namespace, defaulting to DefaultGitopsNamespace.
func (s *FluxStack) gitopsNamespace() string {
	return s.runtime.ConfigHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
}

// gitopsMode returns the configured gitops mode, defaulting to pull. Plan/render
// paths use this so their output reflects the same intervals ApplyBlueprint would
// actually write — otherwise windsor plan and the applied manifest would drift.
func (s *FluxStack) gitopsMode() constants.GitopsMode {
	return constants.ParseGitopsMode(s.runtime.ConfigHandler.GetString("gitops.mode", ""))
}

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

// planOne runs flux diff for a single kustomization. It checks whether the kustomization
// already exists in the cluster and dispatches to the appropriate diff strategy.
// If the cluster is not reachable, the kustomization is treated as new and planned
// via kustomize build instead of flux diff.
func (s *FluxStack) planOne(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization, namespace string) error {
	exists, err := s.kubernetesManager.KustomizationExists(k.Name, namespace)
	if err != nil {
		// Cluster not reachable — treat as new.
		exists = false
	}

	sourceRoot := s.resolveSourceRoot(blueprint, k)
	fluxK := k.ToFluxKustomization(namespace, blueprint.Metadata.Name, blueprint.Sources, s.gitopsMode())
	localPath := filepath.Join(sourceRoot, fluxK.Spec.Path)

	if exists {
		return s.runFluxDiff(k.Name, "diff", "kustomization", k.Name, "--namespace", namespace, "--path", localPath)
	}

	return s.runFromScratch(k, fluxK.Spec.Components, localPath, sourceRoot)
}

// resolveSourceRoot returns the local filesystem root directory for a kustomization's source.
// For OCI sources (url starts with oci://), the root is the extracted OCI cache directory at
// <projectRoot>/.windsor/cache/oci/<key>. For Git/local sources it is the project root.
// The OCI cache key mirrors the GetCacheDir logic in pkg/composer/artifact.
func (s *FluxStack) resolveSourceRoot(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization) string {
	sourceName := k.Source
	if sourceName == "" {
		sourceName = blueprint.Metadata.Name
	}
	if sourceName == "template" && !blueprintv1alpha1.HasRemoteTemplateSource(blueprint.Sources) {
		sourceName = blueprint.Metadata.Name
	}

	for _, source := range blueprint.Sources {
		if source.Name != sourceName {
			continue
		}
		if !strings.HasPrefix(source.Url, "oci://") {
			break
		}
		ref := strings.TrimPrefix(source.Url, "oci://")
		extractionKey := strings.ReplaceAll(strings.ReplaceAll(ref, "/", "_"), ":", "_")
		return filepath.Join(s.runtime.ProjectRoot, ".windsor", "cache", "oci", extractionKey)
	}

	return s.runtime.ProjectRoot
}

// runFromScratch renders the raw kustomize manifests for a kustomization that has not yet
// been deployed to the cluster. Both flux diff and flux build require the kustomization
// object to exist in the cluster by name, so this path uses kustomize build directly.
// A synthetic kustomization.yaml is written to <sourceRoot>/.windsor/plan/<name>/ that
// mirrors what flux would generate at reconcile time: if localPath is a Component, it is
// listed under components: (not resources:), matching flux's own wrapping behaviour.
func (s *FluxStack) runFromScratch(k blueprintv1alpha1.Kustomization, components []string, localPath, sourceRoot string) error {
	baseIsComponent := s.isKustomizeComponent(localPath)

	if !baseIsComponent && len(components) == 0 {
		return s.runKustomizeBuild(k.Name, localPath)
	}

	planDir := filepath.Join(sourceRoot, ".windsor", "plan", k.Name)
	if err := s.shims.MkdirAll(planDir, 0700); err != nil {
		return fmt.Errorf("failed to create plan dir for kustomization %q: %w", k.Name, err)
	}
	defer s.shims.RemoveAll(planDir)

	if err := s.writeSyntheticKustomization(k.Name, planDir, localPath, baseIsComponent, components); err != nil {
		return err
	}

	return s.runKustomizeBuild(k.Name, planDir)
}

// writeSyntheticKustomization writes a synthetic kustomization.yaml into planDir that mirrors
// what flux would generate at reconcile time. If the base path is a Component it is listed
// under components:; otherwise it is listed under resources: followed by any extra components.
// The planDir must already exist. Returns an error if any relative-path computation or the
// file write fails.
func (s *FluxStack) writeSyntheticKustomization(name, planDir, localPath string, baseIsComponent bool, components []string) error {
	relBase, err := filepath.Rel(planDir, localPath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path for kustomization %q: %w", name, err)
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\n")
	if baseIsComponent {
		sb.WriteString("components:\n")
		sb.WriteString(fmt.Sprintf("- %s\n", relBase))
	} else {
		sb.WriteString("resources:\n")
		sb.WriteString(fmt.Sprintf("- %s\n", relBase))
		sb.WriteString("components:\n")
	}
	for _, comp := range components {
		relComp, err := filepath.Rel(planDir, filepath.Join(localPath, comp))
		if err != nil {
			return fmt.Errorf("failed to compute relative path for component %q: %w", comp, err)
		}
		sb.WriteString(fmt.Sprintf("- %s\n", relComp))
	}

	if err := s.shims.WriteFile(filepath.Join(planDir, "kustomization.yaml"), []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write plan kustomization for %q: %w", name, err)
	}
	return nil
}

// isKustomizeComponent returns true if the kustomization.yaml in path declares kind: Component.
// Flux wraps Component paths in a synthetic Kustomization at reconcile time; we must do the same.
func (s *FluxStack) isKustomizeComponent(path string) bool {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml"} {
		data, err := s.shims.ReadFile(filepath.Join(path, name))
		if err != nil {
			continue
		}
		return strings.Contains(string(data), "kind: Component")
	}
	return false
}

// runKustomizeBuild executes "kustomize build <path>" to render all kubernetes manifests
// for a kustomization that does not yet exist in the cluster. Unlike flux diff/build,
// kustomize build requires no cluster access and always emits rendered YAML to stdout.
func (s *FluxStack) runKustomizeBuild(name, path string) error {
	fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Kustomize: "+name))
	stdout, stderr, err := s.shims.ExecCommand("kustomize", "build", path)
	if err != nil {
		if stderr != "" {
			return fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	if stdout != "" {
		fmt.Print(stdout)
	} else {
		fmt.Fprintln(os.Stderr, "No changes.")
	}
	return nil
}

// runFluxDiff executes "flux <args>" via the ExecCommand shim, which captures
// stdout and stderr separately and sets NO_COLOR=1 to prevent flux from writing
// progress indicators directly to the terminal TTY.
// flux diff exits 0 (no changes) or 1 (changes exist) — both are treated as success.
// On exit 0 "No changes." is printed. On exit 1 the diff (stdout) is printed.
// Any other exit code is returned as an error with stderr details.
func (s *FluxStack) runFluxDiff(name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Kustomize: "+name))
	stdout, stderr, err := s.shims.ExecCommand("flux", args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			if stdout != "" {
				fmt.Print(stdout)
			}
			return nil
		}
		if stderr != "" {
			return fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	fmt.Fprintln(os.Stderr, "No changes.")
	return nil
}

// checkFluxVersion verifies that the installed flux CLI is at least minFluxMajor.minFluxMinor.
// It runs "flux version --client" and parses the "flux: vX.Y.Z" line.
func (s *FluxStack) checkFluxVersion() error {
	out, err := s.runtime.Shell.ExecSilent("flux", "version", "--client")
	if err != nil {
		return fmt.Errorf("failed to get flux version: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "flux:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			break
		}
		ver := strings.TrimPrefix(parts[1], "v")
		segments := strings.SplitN(ver, ".", 3)
		if len(segments) < 2 {
			break
		}
		major, err1 := strconv.Atoi(segments[0])
		minor, err2 := strconv.Atoi(segments[1])
		if err1 != nil || err2 != nil {
			break
		}
		if major > minFluxMajor || (major == minFluxMajor && minor >= minFluxMinor) {
			return nil
		}
		return fmt.Errorf("flux CLI v%d.%d or later is required (found v%s)\nUpgrade: https://fluxcd.io/flux/installation/", minFluxMajor, minFluxMinor, ver)
	}
	return fmt.Errorf("could not determine flux CLI version from output: %q", out)
}

// captureFluxDiff runs "flux <args>" and returns stdout without printing.
// Exit code 1 (changes detected) is treated as success, matching runFluxDiff semantics.
// Any other non-zero exit code is returned as an error.
func (s *FluxStack) captureFluxDiff(args ...string) (string, error) {
	stdout, stderr, err := s.shims.ExecCommand("flux", args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return stdout, nil
		}
		if stderr != "" {
			return "", fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
		}
		return "", err
	}
	return stdout, nil
}

// captureKustomizeBuild renders the kustomize manifests for a kustomization that does
// not yet exist in the cluster and returns the raw YAML string without printing.
// It follows the same synthetic-kustomization-file logic as runFromScratch.
func (s *FluxStack) captureKustomizeBuild(k blueprintv1alpha1.Kustomization, components []string, localPath, sourceRoot string) (string, error) {
	baseIsComponent := s.isKustomizeComponent(localPath)

	if !baseIsComponent && len(components) == 0 {
		stdout, stderr, err := s.shims.ExecCommand("kustomize", "build", localPath)
		if err != nil {
			if stderr != "" {
				return "", fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
			}
			return "", err
		}
		return stdout, nil
	}

	planDir := filepath.Join(sourceRoot, ".windsor", "plan", k.Name)
	if err := s.shims.MkdirAll(planDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create plan dir for kustomization %q: %w", k.Name, err)
	}
	defer s.shims.RemoveAll(planDir)

	if err := s.writeSyntheticKustomization(k.Name, planDir, localPath, baseIsComponent, components); err != nil {
		return "", err
	}

	stdout, stderr, err := s.shims.ExecCommand("kustomize", "build", planDir)
	if err != nil {
		if stderr != "" {
			return "", fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
		}
		return "", err
	}
	return stdout, nil
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

// encodeKustomizationsJSON builds each kustomization in targets via kustomize build,
// converts the YAML output to JSON, and writes a JSON array of
// {"kustomization": name, "resources": [...]} objects to w.
func (s *FluxStack) encodeKustomizationsJSON(w io.Writer, blueprint *blueprintv1alpha1.Blueprint, namespace string, targets []blueprintv1alpha1.Kustomization) error {
	type entry struct {
		Kustomization string            `json:"kustomization"`
		Resources     []json.RawMessage `json:"resources"`
	}

	var results []entry
	for _, k := range targets {
		fluxK := k.ToFluxKustomization(namespace, blueprint.Metadata.Name, blueprint.Sources, s.gitopsMode())
		sourceRoot := s.resolveSourceRoot(blueprint, k)
		localPath := filepath.Join(sourceRoot, fluxK.Spec.Path)

		yamlStr, err := s.captureKustomizeBuild(k, fluxK.Spec.Components, localPath, sourceRoot)
		if err != nil {
			return fmt.Errorf("error building kustomization %q: %w", k.Name, err)
		}

		results = append(results, entry{Kustomization: k.Name, Resources: yamlDocumentsToJSON(yamlStr)})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// yamlDocumentsToJSON splits a multi-document YAML string on "---" separators,
// converts each non-empty document to JSON using sigs.k8s.io/yaml, and returns
// the results as a slice of raw JSON messages. Documents that fail to convert are skipped.
func yamlDocumentsToJSON(yamlStr string) []json.RawMessage {
	var resources []json.RawMessage
	for _, doc := range strings.Split(yamlStr, "\n---") {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		jsonBytes, err := sigsyaml.YAMLToJSON([]byte(doc))
		if err != nil || string(jsonBytes) == "null" {
			continue
		}
		resources = append(resources, json.RawMessage(jsonBytes))
	}
	return resources
}

// findKustomization returns the Kustomization with the given name from the blueprint.
func findKustomization(blueprint *blueprintv1alpha1.Blueprint, name string) (blueprintv1alpha1.Kustomization, bool) {
	for _, k := range blueprint.Kustomizations {
		if k.Name == name {
			return k, true
		}
	}
	return blueprintv1alpha1.Kustomization{}, false
}
