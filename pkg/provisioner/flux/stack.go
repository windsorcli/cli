// Package flux provides Flux kustomization stack management functionality.
// It provides a unified interface for planning Flux kustomization changes
// by shelling out to the flux CLI (https://fluxcd.io/flux/installation/).
// The FluxStack is the primary orchestrator for Flux-based operations,
// coordinating shell operations and blueprint handling.

package flux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
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

// Plan runs flux diff for a single kustomization or all kustomizations when componentID is "all".
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

	namespace := s.runtime.ConfigHandler.GetString("flux.namespace", constants.DefaultFluxSystemNamespace)

	if componentID == "all" {
		for _, k := range blueprint.Kustomizations {
			if k.DestroyOnly != nil && *k.DestroyOnly {
				continue
			}
			if err := s.planOne(blueprint, k, namespace); err != nil {
				return err
			}
		}
		return nil
	}

	k, found := findKustomization(blueprint, componentID)
	if !found {
		return fmt.Errorf("kustomization %q not found in blueprint", componentID)
	}
	return s.planOne(blueprint, k, namespace)
}

// =============================================================================
// Private Methods
// =============================================================================

// planOne runs flux diff for a single kustomization. It checks whether the kustomization
// already exists in the cluster and dispatches to the appropriate diff strategy.
func (s *FluxStack) planOne(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization, namespace string) error {
	exists, err := s.kubernetesManager.KustomizationExists(k.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to check if kustomization %q exists: %w", k.Name, err)
	}

	sourceRoot := s.resolveSourceRoot(blueprint, k)
	fluxK := k.ToFluxKustomization(namespace, blueprint.Metadata.Name, blueprint.Sources)
	localPath := filepath.Join(sourceRoot, fluxK.Spec.Path)

	if exists {
		return s.runFluxDiff(
			fmt.Sprintf("📋 Planning kustomize changes for %s", k.Name),
			"diff", "kustomization", k.Name, "--namespace", namespace, "--path", localPath,
		)
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
	label := fmt.Sprintf("📋 Planning kustomize changes for %s", k.Name)

	baseIsComponent := s.isKustomizeComponent(localPath)

	if !baseIsComponent && len(components) == 0 {
		return s.runKustomizeBuild(label, localPath)
	}

	planDir := filepath.Join(sourceRoot, ".windsor", "plan", k.Name)
	if err := s.shims.MkdirAll(planDir, 0700); err != nil {
		return fmt.Errorf("failed to create plan dir for kustomization %q: %w", k.Name, err)
	}
	defer s.shims.RemoveAll(planDir)

	relBase, err := filepath.Rel(planDir, localPath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path for kustomization %q: %w", k.Name, err)
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

	// spec.components are relative to spec.path (localPath).
	for _, comp := range components {
		relComp, err := filepath.Rel(planDir, filepath.Join(localPath, comp))
		if err != nil {
			return fmt.Errorf("failed to compute relative path for component %q: %w", comp, err)
		}
		sb.WriteString(fmt.Sprintf("- %s\n", relComp))
	}

	if err := s.shims.WriteFile(filepath.Join(planDir, "kustomization.yaml"), []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write plan kustomization for %q: %w", k.Name, err)
	}

	return s.runKustomizeBuild(label, planDir)
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
func (s *FluxStack) runKustomizeBuild(label, path string) error {
	fmt.Fprintf(os.Stderr, "%s\n", label)
	stdout, stderr, err := s.shims.ExecCommand("kustomize", "build", path)
	if err != nil {
		if stderr != "" {
			return fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	if stdout != "" {
		fmt.Print(stdout)
	}
	return nil
}

// runFluxDiff executes "flux <args>" via the ExecCommand shim, which captures
// stdout and stderr separately and sets NO_COLOR=1 to prevent flux from writing
// progress indicators directly to the terminal TTY.
// flux diff exits 0 (no changes) or 1 (changes exist) — both are treated as success.
// On exit 0 output is suppressed. On exit 1 the diff (stdout) is printed.
// Any other exit code is returned as an error with stderr details.
func (s *FluxStack) runFluxDiff(label string, args ...string) error {
	fmt.Fprintf(os.Stderr, "%s\n", label)
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

// findKustomization returns the Kustomization with the given name from the blueprint.
func findKustomization(blueprint *blueprintv1alpha1.Blueprint, name string) (blueprintv1alpha1.Kustomization, bool) {
	for _, k := range blueprint.Kustomizations {
		if k.Name == name {
			return k, true
		}
	}
	return blueprintv1alpha1.Kustomization{}, false
}
