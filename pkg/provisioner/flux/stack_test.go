package flux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// exitError returns a real *exec.ExitError with the requested exit code by
// running a trivial shell one-liner. Tests that need to simulate flux diff
// exit code 1 (changes detected) use this helper.
func exitError(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for code %d", code)
	}
	return err
}

// =============================================================================
// Test Setup
// =============================================================================

type fluxMocks struct {
	shell             *shell.MockShell
	configHandler     *config.MockConfigHandler
	kubernetesManager *kubernetes.MockKubernetesManager
	shims             *Shims
	runtime           *runtime.Runtime
}

func setupFluxMocks(t *testing.T) *fluxMocks {
	t.Helper()

	mockShell := shell.NewMockShell()
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "flux" && len(args) > 0 && args[0] == "version" {
			return "flux: v2.4.0\n", nil
		}
		return "", nil
	}
	mockShell.ExecProgressFunc = func(message, command string, args ...string) (string, error) {
		return "", nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
		return true, nil
	}

	shims := NewShims()
	shims.LookPath = func(file string) (string, error) { return "/usr/local/bin/flux", nil }
	shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
	shims.RemoveAll = func(path string) error { return nil }
	shims.ReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }
	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }
	shims.ExecCommand = func(command string, args ...string) (string, string, error) {
		return "", "", nil
	}

	rt := &runtime.Runtime{
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		ProjectRoot:   t.TempDir(),
	}

	return &fluxMocks{
		shell:             mockShell,
		configHandler:     mockConfigHandler,
		kubernetesManager: mockKubernetesManager,
		shims:             shims,
		runtime:           rt,
	}
}

func newTestFluxStack(m *fluxMocks) *FluxStack {
	return &FluxStack{
		runtime:           m.runtime,
		shims:             m.shims,
		kubernetesManager: m.kubernetesManager,
	}
}

func testBlueprint() *blueprintv1alpha1.Blueprint {
	destroyOnly := true
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test-blueprint"},
		Sources: []blueprintv1alpha1.Source{
			{Name: "test-source"},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "my-app"},
			{Name: "infra-base"},
			{Name: "cleanup-only", DestroyOnly: &destroyOnly},
		},
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFluxStack_Plan(t *testing.T) {
	t.Run("Success_ExistingKustomization", func(t *testing.T) {
		// Given a blueprint with a kustomization that already exists in the cluster
		m := setupFluxMocks(t)
		var capturedArgs []string
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			capturedArgs = args
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called for a specific existing kustomization
		err := s.Plan(testBlueprint(), "my-app")

		// Then no error is returned and flux diff kustomization is invoked with --path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(capturedArgs) < 2 || capturedArgs[0] != "diff" || capturedArgs[1] != "kustomization" {
			t.Errorf("expected flux diff kustomization, got args %v", capturedArgs)
		}
		if capturedArgs[2] != "my-app" {
			t.Errorf("expected kustomization name my-app, got %q", capturedArgs[2])
		}
		foundPath := false
		for i, a := range capturedArgs {
			if a == "--path" && i+1 < len(capturedArgs) {
				foundPath = true
				break
			}
		}
		if !foundPath {
			t.Errorf("expected --path flag in args, got %v", capturedArgs)
		}
	})

	t.Run("Success_All", func(t *testing.T) {
		// Given a blueprint with multiple kustomizations
		m := setupFluxMocks(t)
		var calledNames []string
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			for i, a := range args {
				if a == "kustomization" && i+1 < len(args) {
					calledNames = append(calledNames, args[i+1])
				}
			}
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called with "all"
		err := s.Plan(testBlueprint(), "all")

		// Then no error is returned and each non-destroy-only kustomization is planned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(calledNames) != 2 {
			t.Errorf("expected 2 kustomizations planned, got %d: %v", len(calledNames), calledNames)
		}
		for _, name := range calledNames {
			if name == "cleanup-only" {
				t.Errorf("expected cleanup-only (destroyOnly) to be skipped, but it was planned")
			}
		}
	})

	t.Run("Success_All_SkipsDestroyOnly", func(t *testing.T) {
		// Given a blueprint where all kustomizations are destroy-only
		m := setupFluxMocks(t)
		destroyOnly := true
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cleanup", DestroyOnly: &destroyOnly}},
		}
		var callCount int
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			callCount++
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called with "all"
		err := s.Plan(bp, "all")

		// Then no error is returned and no flux calls are made
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if callCount != 0 {
			t.Errorf("expected no flux calls for destroy-only kustomizations, got %d", callCount)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a valid stack
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When Plan is called with a nil blueprint
		err := s.Plan(nil, "my-app")

		// Then an error is returned
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ErrorFluxNotInstalled", func(t *testing.T) {
		// Given a stack where flux is not on the PATH
		m := setupFluxMocks(t)
		m.shims.LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("executable file not found in $PATH")
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then an error mentioning the install URL is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "flux CLI is required") {
			t.Errorf("expected install hint in error, got %v", err)
		}
	})

	t.Run("ErrorFluxVersionTooOld", func(t *testing.T) {
		// Given a stack where flux is installed but below the minimum version
		m := setupFluxMocks(t)
		m.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "flux: v2.1.0\n", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then an error mentioning the required version is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "v2.3 or later is required") {
			t.Errorf("expected version requirement in error, got %v", err)
		}
	})

	t.Run("ErrorCannotGetFluxVersion", func(t *testing.T) {
		// Given a stack where flux version check fails
		m := setupFluxMocks(t)
		m.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("flux: command not found")
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then an error is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get flux version") {
			t.Errorf("expected version fetch error, got %v", err)
		}
	})

	t.Run("ErrorKustomizationNotInBlueprint", func(t *testing.T) {
		// Given a blueprint that does not contain the requested kustomization
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When Plan is called for a name that isn't in the blueprint
		err := s.Plan(testBlueprint(), "nonexistent")

		// Then an error mentioning the name is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("expected kustomization name in error, got %v", err)
		}
	})

	t.Run("ClusterUnreachableFallsBackToScratch", func(t *testing.T) {
		// Given a stack whose kubernetes manager cannot reach the cluster
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, fmt.Errorf("stat /no/such/kubeconfig: no such file or directory")
		}
		var capturedCommand string
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			capturedCommand = command
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then no error is returned and kustomize build is used instead of flux diff
		if err != nil {
			t.Fatalf("expected no error on cluster unreachable, got %v", err)
		}
		if capturedCommand != "kustomize" {
			t.Errorf("expected kustomize build fallback, got command %q", capturedCommand)
		}
	})

	t.Run("ErrorExecProgressFails", func(t *testing.T) {
		// Given a stack where flux diff exits with a real error (exit status 2)
		m := setupFluxMocks(t)
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			return "", "server error", fmt.Errorf("exit status 2")
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then the exec error is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "exit status 2") {
			t.Errorf("expected exec error, got %v", err)
		}
	})

	t.Run("ErrorFromScratchKustomizeFails", func(t *testing.T) {
		// Given a stack where the kustomization does not exist and kustomize build fails
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			return "", "kustomize: no such file or directory", fmt.Errorf("exit status 1")
		}
		s := newTestFluxStack(m)

		// When Plan is called for a non-existent kustomization
		err := s.Plan(testBlueprint(), "my-app")

		// Then the kustomize error is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("expected kustomize error, got %v", err)
		}
	})

	t.Run("Success_FromScratch", func(t *testing.T) {
		// Given a blueprint with a kustomization that does NOT exist in the cluster
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		var capturedCommand string
		var capturedArgs []string
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			capturedCommand = command
			capturedArgs = args
			return "rendered: yaml", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called for a kustomization that is not yet deployed
		err := s.Plan(testBlueprint(), "my-app")

		// Then no error is returned and kustomize build is invoked with the local path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if capturedCommand != "kustomize" {
			t.Errorf("expected kustomize command, got %q", capturedCommand)
		}
		if len(capturedArgs) < 2 || capturedArgs[0] != "build" {
			t.Errorf("expected kustomize build, got args %v", capturedArgs)
		}
	})

	t.Run("Success_FromScratch_WithComponents", func(t *testing.T) {
		// Given a blueprint with a kustomization that has components and does NOT exist in the cluster
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "dns", Components: []string{"coredns", "external-dns"}},
			},
		}
		var capturedKustomizationYAML string
		m.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			capturedKustomizationYAML = string(data)
			return nil
		}
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			return "rendered: yaml", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called for a kustomization with components
		err := s.Plan(bp, "dns")

		// Then no error is returned and the temp kustomization.yaml includes the components
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !strings.Contains(capturedKustomizationYAML, "components:") {
			t.Errorf("expected components section in temp kustomization.yaml, got:\n%s", capturedKustomizationYAML)
		}
		if !strings.Contains(capturedKustomizationYAML, "coredns") || !strings.Contains(capturedKustomizationYAML, "external-dns") {
			t.Errorf("expected component paths in temp kustomization.yaml, got:\n%s", capturedKustomizationYAML)
		}
	})

	t.Run("ErrorFromScratch_MkdirAllFails", func(t *testing.T) {
		// Given a stack where the kustomization has components but plan dir creation fails
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "dns", Components: []string{"coredns"}},
			},
		}
		m.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("no space left on device")
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(bp, "dns")

		// Then the plan dir error is returned
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no space left on device") {
			t.Errorf("expected plan dir error, got %v", err)
		}
	})

	t.Run("Success_FromScratch_OCISource", func(t *testing.T) {
		// Given a kustomization whose source is an OCI registry
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "upstream", Url: "oci://registry.local:5000/windsor/core:latest"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "dns", Source: "upstream"},
			},
		}
		var capturedBuildPath string
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			if command == "kustomize" && len(args) >= 2 && args[0] == "build" {
				capturedBuildPath = args[1]
			}
			return "rendered: yaml", "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called for the OCI-sourced kustomization
		err := s.Plan(bp, "dns")

		// Then no error and the build path is inside the OCI cache directory
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// colons and slashes in the OCI ref are replaced with _ to form the cache key
		expectedCacheDir := filepath.Join(m.runtime.ProjectRoot, ".windsor", "cache", "oci", "registry.local_5000_windsor_core_latest")
		if !strings.HasPrefix(capturedBuildPath, expectedCacheDir) {
			t.Errorf("expected build path inside OCI cache %q, got %q", expectedCacheDir, capturedBuildPath)
		}
	})
}

func TestFluxStack_PlanComponentSummary(t *testing.T) {
	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called with nil blueprint
		result := s.PlanComponentSummary(nil, "my-app")

		// Then an error is set on the result
		if result.Err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ReturnsErrorForMissingKustomization", func(t *testing.T) {
		// Given a blueprint that does not contain the requested kustomization
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called for a non-existent name
		result := s.PlanComponentSummary(testBlueprint(), "nonexistent")

		// Then an error is set indicating not found
		if result.Err == nil {
			t.Error("expected not-found error, got nil")
		}
		if !strings.Contains(result.Err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", result.Err)
		}
	})

	t.Run("MarksNewKustomizationAsNew", func(t *testing.T) {
		// Given a kustomization that does not exist in the cluster (cluster unreachable)
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, fmt.Errorf("cluster not reachable")
		}
		s := newTestFluxStack(m)

		// When PlanComponentSummary is called
		result := s.PlanComponentSummary(testBlueprint(), "my-app")

		// Then IsNew is set
		if !result.IsNew {
			t.Error("expected IsNew=true for unreachable cluster, got false")
		}
		if result.Name != "my-app" {
			t.Errorf("expected Name=my-app, got %q", result.Name)
		}
	})
}

func TestFluxStack_PlanSummary(t *testing.T) {
	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanSummary is called with nil blueprint
		result, hints := s.PlanSummary(nil)

		// Then nil is returned for both
		if result != nil {
			t.Errorf("expected nil results, got %v", result)
		}
		if hints != nil {
			t.Errorf("expected nil hints, got %v", hints)
		}
	})

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a blueprint with one normal and one destroyOnly kustomization
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then only the non-destroyOnly kustomizations appear in results (cleanup-only is excluded)
		for _, r := range results {
			if r.Name == "cleanup-only" {
				t.Errorf("expected cleanup-only to be excluded, but it appeared in results")
			}
		}
	})

	t.Run("ParsesDiffLinesForExistingKustomization", func(t *testing.T) {
		// Given an existing kustomization that returns a unified diff with additions and removals
		m := setupFluxMocks(t)
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			if command == "flux" {
				// Exit code 1 means changes detected — real *exec.ExitError required for errors.As
				diff := "--- a/deploy.yaml\n+++ b/deploy.yaml\n@@ -1,3 +1,4 @@\n+added line\n-removed line\n unchanged\n"
				return diff, "", exitError(t, 1)
			}
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called for the blueprint
		results, _ := s.PlanSummary(testBlueprint())

		// Then the first result has parsed add/remove counts
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if r.IsNew {
			t.Errorf("expected IsNew=false for existing kustomization")
		}
		if r.Added != 1 || r.Removed != 1 {
			t.Errorf("expected +1 -1, got +%d -%d", r.Added, r.Removed)
		}
	})

	t.Run("TreatsKustomizationAsNewWhenClusterUnreachable", func(t *testing.T) {
		// Given a kubernetes manager that returns an error on KustomizationExists
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, fmt.Errorf("connection refused")
		}
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			if command == "kustomize" {
				return "apiVersion: v1\nkind: Namespace\n", "", nil
			}
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then the component is treated as new and resources are counted
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if !r.IsNew {
			t.Errorf("expected IsNew=true when cluster is unreachable")
		}
	})

	t.Run("CountsResourcesFromKustomizeBuildForNewKustomization", func(t *testing.T) {
		// Given a kustomization that does not exist in the cluster and kustomize build returns two resources
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			if command == "kustomize" {
				return "apiVersion: v1\nkind: Namespace\n---\napiVersion: apps/v1\nkind: Deployment\n", "", nil
			}
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then the result is marked new and Added reflects the resource count
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if !r.IsNew {
			t.Errorf("expected IsNew=true")
		}
		if r.Added != 2 {
			t.Errorf("expected Added=2, got %d", r.Added)
		}
	})

	t.Run("ReturnsDegradedRowsAndHintWhenFluxNotInstalled", func(t *testing.T) {
		// Given flux CLI is not on PATH
		m := setupFluxMocks(t)
		m.shims.LookPath = func(file string) (string, error) {
			if file == "flux" {
				return "", fmt.Errorf("not found")
			}
			return "/usr/bin/" + file, nil
		}
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return true, nil // existing — would need flux
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, hints := s.PlanSummary(testBlueprint())

		// Then each result is marked Degraded and a hint is returned instead of an error entry
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if r.Err != nil {
				t.Errorf("expected no error for degraded row, got %v", r.Err)
			}
			if !r.Degraded {
				t.Errorf("expected Degraded=true for %q", r.Name)
			}
		}
		if len(hints) == 0 {
			t.Error("expected at least one hint when flux is missing")
		}
		found := false
		for _, h := range hints {
			if strings.Contains(h, "flux") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected flux hint, got %v", hints)
		}
	})

	t.Run("RecordsErrorWhenFluxDiffFails", func(t *testing.T) {
		// Given an existing kustomization where flux diff fails with a non-1 exit code
		m := setupFluxMocks(t)
		m.shims.ExecCommand = func(command string, args ...string) (string, string, error) {
			if command == "flux" {
				return "", "unexpected error", exitError(t, 2)
			}
			return "", "", nil
		}
		s := newTestFluxStack(m)

		// When PlanSummary is called
		results, _ := s.PlanSummary(testBlueprint())

		// Then an error is recorded for the affected kustomization
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if results[0].Err == nil {
			t.Errorf("expected error for failing diff, got nil")
		}
	})
}

func TestCountDiffLines(t *testing.T) {
	t.Run("CountsAddedAndRemovedLines", func(t *testing.T) {
		// Given a unified diff with additions and removals
		diff := "--- a/file.yaml\n+++ b/file.yaml\n+added line\n-removed line\n unchanged\n"

		// When counted
		added, removed := countDiffLines(diff)

		// Then headers are excluded and only content lines counted
		if added != 1 || removed != 1 {
			t.Errorf("expected +1 -1, got +%d -%d", added, removed)
		}
	})

	t.Run("ReturnsZeroForEmptyDiff", func(t *testing.T) {
		// Given an empty string
		added, removed := countDiffLines("")

		// Then both counts are zero
		if added != 0 || removed != 0 {
			t.Errorf("expected 0 0, got %d %d", added, removed)
		}
	})
}

func TestCountKustomizeResources(t *testing.T) {
	t.Run("CountsKindOccurrences", func(t *testing.T) {
		// Given YAML output with two resources
		yaml := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: foo\n---\napiVersion: apps/v1\nkind: Deployment\n"

		// When counted
		n := countKustomizeResources(yaml)

		// Then two resources are detected
		if n != 2 {
			t.Errorf("expected 2, got %d", n)
		}
	})

	t.Run("ReturnsZeroForEmptyOutput", func(t *testing.T) {
		// Given empty output
		n := countKustomizeResources("")

		// Then zero is returned
		if n != 0 {
			t.Errorf("expected 0, got %d", n)
		}
	})
}
