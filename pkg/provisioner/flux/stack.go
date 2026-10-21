// The FluxStack plans Flux kustomization changes by running the flux and kustomize CLIs.
// It provides Plan, PlanAll, PlanJSON, and PlanAllJSON, and the Stack interface that the provisioner uses.
// It checks the flux CLI version and runs flux diff and kustomize build for each kustomization.

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
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/tui"
	sigsyaml "sigs.k8s.io/yaml"
)

// =============================================================================
// Constants
// =============================================================================

// minFluxMajor and minFluxMinor define the minimum supported flux CLI version (v2.3.0).
// flux diff kustomization --kustomization-file was introduced in v2.3.0.
const (
	minFluxMajor = 2
	minFluxMinor = 3
)

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
// Interfaces
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

// PlanAll runs flux diff for every kustomization in the blueprint that is not destroyOnly.
// It needs the flux CLI only when there are kustomizations to plan. A missing flux CLI
// returns an error with a remediation hint.
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
		return werror.New("flux CLI is required for 'plan kustomize'", "Install the flux CLI from https://fluxcd.io/flux/installation/.", nil)
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

// Plan runs flux diff for the kustomization with the given name. It returns an error if the
// diff fails. When the kustomization is not in the cluster yet, it plans from the blueprint
// definition with --kustomization-file. A missing flux CLI, or a name that is not in the
// blueprint, returns an error with a remediation hint.
func (s *FluxStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("flux"); err != nil {
		return werror.New("flux CLI is required for 'plan kustomize'", "Install the flux CLI from https://fluxcd.io/flux/installation/.", nil)
	}
	if err := s.checkFluxVersion(); err != nil {
		return err
	}

	namespace := s.gitopsNamespace()

	k, found := findKustomization(blueprint, componentID)
	if !found {
		return werror.New(fmt.Sprintf("kustomization %q not found in blueprint", componentID), "Use the name of a kustomization in the blueprint. Run windsor show kustomization to see the kustomizations.", nil)
	}
	return s.planOne(blueprint, k, namespace)
}

// PlanAllJSON runs kustomize build for every kustomization in the blueprint that is not destroyOnly.
// It converts the YAML manifests to JSON and writes a JSON array of {"kustomization": name,
// "resources": [...]} objects to stdout. A missing kustomize CLI returns an error with a remediation hint.
func (s *FluxStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("kustomize"); err != nil {
		return werror.New("kustomize CLI is required for 'plan kustomize --json'", "Install the kustomize CLI from https://kubectl.docs.kubernetes.io/installation/kustomize/.", nil)
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

// PlanJSON runs kustomize build for the kustomization with the given name and writes the result as JSON.
// It writes a JSON array of {"kustomization": name, "resources": [...]} objects to stdout.
// It always uses kustomize build, so the output is the full desired state for machine use.
// A missing kustomize CLI, or a name that is not in the blueprint, returns an error with a remediation hint.
func (s *FluxStack) PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	if _, err := s.shims.LookPath("kustomize"); err != nil {
		return werror.New("kustomize CLI is required for 'plan kustomize --json'", "Install the kustomize CLI from https://kubectl.docs.kubernetes.io/installation/kustomize/.", nil)
	}

	k, found := findKustomization(blueprint, componentID)
	if !found {
		return werror.New(fmt.Sprintf("kustomization %q not found in blueprint", componentID), "Use the name of a kustomization in the blueprint. Run windsor show kustomization to see the kustomizations.", nil)
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
// localPath must itself hold a kustomization.yaml or .yml. Unlike Flux's kustomize-controller,
// the local kustomize CLI this method shells out to cannot render a bare directory. See
// requireKustomizationFile for the actionable error this produces instead of kustomize's own
// low-level failure.
func (s *FluxStack) runFromScratch(k blueprintv1alpha1.Kustomization, components []string, localPath, sourceRoot string) error {
	data, err := s.requireKustomizationFile(k.Name, localPath)
	if err != nil {
		return err
	}

	baseIsComponent := isKustomizeComponent(data)

	if !baseIsComponent && len(components) == 0 {
		return s.runKustomizeBuild(k.Name, localPath)
	}

	planDir := filepath.Join(sourceRoot, ".windsor", "plan", k.Name)
	if err := s.shims.MkdirAll(planDir, 0700); err != nil {
		return werror.Wrap(err, "failed to create plan dir for kustomization %q", k.Name)
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
		return werror.Wrap(err, "failed to compute relative path for kustomization %q", name)
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
			return werror.Wrap(err, "failed to compute relative path for component %q", comp)
		}
		sb.WriteString(fmt.Sprintf("- %s\n", relComp))
	}

	if err := s.shims.WriteFile(filepath.Join(planDir, "kustomization.yaml"), []byte(sb.String()), 0600); err != nil {
		return werror.Wrap(err, "failed to write plan kustomization for %q", name)
	}
	return nil
}

// requireKustomizationFile returns the contents of the kustomization.yaml or kustomization.yml in path.
// name identifies the kustomization in an error. The local kustomize build always needs the file,
// also for a directory that kustomize-controller can render without it. A missing file returns an
// error with a remediation hint. A read failure, such as a permission error, returns that error.
func (s *FluxStack) requireKustomizationFile(name, path string) ([]byte, error) {
	var readErr error
	for _, fileName := range []string{"kustomization.yaml", "kustomization.yml"} {
		data, err := s.shims.ReadFile(filepath.Join(path, fileName))
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			readErr = err
		}
	}
	if readErr != nil {
		return nil, werror.Wrap(readErr, "failed to read a kustomization.yaml for %q in %q", name, path)
	}
	return nil, werror.New(fmt.Sprintf("kustomization %q has no kustomization.yaml or kustomization.yml in %q", name, path), "Add a kustomization.yaml to the directory. An empty \"resources: []\" is enough for kustomize to build from.", nil)
}

// runKustomizeBuild executes "kustomize build <path>" to render all kubernetes manifests
// for a kustomization that does not yet exist in the cluster. Unlike flux diff/build,
// kustomize build requires no cluster access and always emits rendered YAML to stdout.
// Runs through the shell layer so rendered output (which may include Secret resources)
// passes through the same scrubbing as every other executed command.
func (s *FluxStack) runKustomizeBuild(name, path string) error {
	fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Kustomize: "+name))
	stdout, err := s.runtime.Shell.ExecCaptureWithEnv("kustomize", nil, "build", path)
	if err != nil {
		return err
	}
	if stdout != "" {
		fmt.Print(stdout)
	} else {
		fmt.Fprintln(os.Stderr, "No changes.")
	}
	return nil
}

// runFluxDiff executes "flux <args>" through the shell layer, which scrubs the
// captured stdout/stderr and sets NO_COLOR=1 to prevent flux from writing progress
// indicators directly to the terminal TTY.
// flux diff exits 0 (no changes) or 1 (changes exist) — both are treated as success.
// On exit 0 "No changes." is printed. On exit 1 the diff (stdout) is printed.
// Any other exit code is returned as an error with stderr details.
func (s *FluxStack) runFluxDiff(name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "\n%s\n", tui.SectionHeader("Kustomize: "+name))
	stdout, err := s.runtime.Shell.ExecCaptureWithEnv("flux", map[string]string{"NO_COLOR": "1"}, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			if stdout != "" {
				fmt.Print(stdout)
			}
			return nil
		}
		return err
	}
	fmt.Fprintln(os.Stderr, "No changes.")
	return nil
}

// checkFluxVersion checks that the installed flux CLI is at least minFluxMajor.minFluxMinor.
// It runs "flux version --client" and parses the "flux: vX.Y.Z" line. An older flux CLI
// returns an error with a remediation hint.
func (s *FluxStack) checkFluxVersion() error {
	out, err := s.runtime.Shell.ExecSilent("flux", "version", "--client")
	if err != nil {
		return werror.Wrap(err, "failed to get flux version")
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
		return werror.New(fmt.Sprintf("flux CLI v%d.%d or later is required (found v%s)", minFluxMajor, minFluxMinor, ver), "Upgrade the flux CLI from https://fluxcd.io/flux/installation/.", nil)
	}
	return fmt.Errorf("could not determine flux CLI version from output: %q", out)
}

// captureKustomizeBuild renders the kustomize manifests for a kustomization that does
// not yet exist in the cluster and returns the raw YAML string without printing.
// It follows the same synthetic-kustomization-file logic as runFromScratch, including its
// requireKustomizationFile precondition.
func (s *FluxStack) captureKustomizeBuild(k blueprintv1alpha1.Kustomization, components []string, localPath, sourceRoot string) (string, error) {
	data, err := s.requireKustomizationFile(k.Name, localPath)
	if err != nil {
		return "", err
	}

	baseIsComponent := isKustomizeComponent(data)

	if !baseIsComponent && len(components) == 0 {
		return s.runtime.Shell.ExecCaptureWithEnv("kustomize", nil, "build", localPath)
	}

	planDir := filepath.Join(sourceRoot, ".windsor", "plan", k.Name)
	if err := s.shims.MkdirAll(planDir, 0700); err != nil {
		return "", werror.Wrap(err, "failed to create plan dir for kustomization %q", k.Name)
	}
	defer s.shims.RemoveAll(planDir)

	if err := s.writeSyntheticKustomization(k.Name, planDir, localPath, baseIsComponent, components); err != nil {
		return "", err
	}

	return s.runtime.Shell.ExecCaptureWithEnv("kustomize", nil, "build", planDir)
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
			return werror.Wrap(err, "error building kustomization %q", k.Name)
		}

		results = append(results, entry{Kustomization: k.Name, Resources: yamlDocumentsToJSON(yamlStr)})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// =============================================================================
// Helpers
// =============================================================================

// isKustomizeComponent returns true if data declares kind: Component. Flux wraps Component
// paths in a synthetic Kustomization at reconcile time; we must do the same.
func isKustomizeComponent(data []byte) bool {
	return strings.Contains(string(data), "kind: Component")
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
