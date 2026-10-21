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

// =============================================================================
// Test Setup
// =============================================================================

// validKustomizationYAML is a minimal kustomization.yaml body, used by tests that need
// readKustomizationFile to find a real file at a fake local path.
const validKustomizationYAML = "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\n"

// fakeTimeoutError implements net.Error without being a *net.OpError, standing
// in for a TLS/certificate failure.
type fakeTimeoutError struct {
	msg     string
	timeout bool
}

func (e fakeTimeoutError) Error() string { return e.msg }

func (e fakeTimeoutError) Timeout() bool { return e.timeout }

func (e fakeTimeoutError) Temporary() bool { return e.timeout }

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
	mockShell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
		return "", nil
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
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			capturedArgs = args
			return "", nil
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
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		var capturedCommand string
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			capturedCommand = command
			return "", nil
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
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "", fmt.Errorf("exit status 2")
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
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "", fmt.Errorf("exit status 1")
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

	t.Run("ErrorFromScratchMissingKustomizationFile", func(t *testing.T) {
		// Given a stack where the kustomization does not exist and its local path has
		// no kustomization.yaml or .yml of its own
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		var ranKustomize bool
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			ranKustomize = true
			return "", nil
		}
		s := newTestFluxStack(m)

		// When Plan is called for a non-existent kustomization
		err := s.Plan(testBlueprint(), "my-app")

		// Then an actionable error is returned naming the kustomization and its path,
		// and kustomize is never invoked against an invalid base
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "my-app") || !strings.Contains(err.Error(), "kustomization.yaml") {
			t.Errorf("expected an error naming the kustomization and the missing file, got %v", err)
		}
		if ranKustomize {
			t.Error("expected kustomize build to be skipped, but it ran")
		}
	})

	t.Run("ErrorFromScratchKustomizationFileUnreadable", func(t *testing.T) {
		// Given a stack where the kustomization does not exist and its local path's
		// kustomization.yaml exists but cannot be read (a permission error, not an
		// absence)
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		m.shims.ReadFile = func(name string) ([]byte, error) {
			return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
		}
		s := newTestFluxStack(m)

		// When Plan is called
		err := s.Plan(testBlueprint(), "my-app")

		// Then the real read error is surfaced, not the "add one" missing-file message
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), "Add one") {
			t.Errorf("expected the permission error to be reported, not the missing-file message, got %v", err)
		}
		if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("expected the underlying permission error, got %v", err)
		}
	})

	t.Run("Success_FromScratch", func(t *testing.T) {
		// Given a blueprint with a kustomization that does NOT exist in the cluster
		m := setupFluxMocks(t)
		m.kubernetesManager.KustomizationExistsFunc = func(name, namespace string) (bool, error) {
			return false, nil
		}
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		var capturedCommand string
		var capturedArgs []string
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			capturedCommand = command
			capturedArgs = args
			return "rendered: yaml", nil
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
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		var capturedKustomizationYAML string
		m.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			capturedKustomizationYAML = string(data)
			return nil
		}
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "rendered: yaml", nil
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
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
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
		m.shims.ReadFile = func(name string) ([]byte, error) { return []byte(validKustomizationYAML), nil }
		var capturedBuildPath string
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "kustomize" && len(args) >= 2 && args[0] == "build" {
				capturedBuildPath = args[1]
			}
			return "rendered: yaml", nil
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

func TestFluxStack_PlanAll(t *testing.T) {
	t.Run("PlansEveryNonDestroyOnlyKustomization", func(t *testing.T) {
		// Given a blueprint with multiple kustomizations
		m := setupFluxMocks(t)
		var calledNames []string
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			for i, a := range args {
				if a == "kustomization" && i+1 < len(args) {
					calledNames = append(calledNames, args[i+1])
				}
			}
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanAll is called
		err := s.PlanAll(testBlueprint())

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

	t.Run("SkipsDestroyOnlyKustomizations", func(t *testing.T) {
		// Given a blueprint where all kustomizations are destroy-only
		m := setupFluxMocks(t)
		destroyOnly := true
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cleanup", DestroyOnly: &destroyOnly}},
		}
		var callCount int
		m.shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			callCount++
			return "", nil
		}
		s := newTestFluxStack(m)

		// When PlanAll is called
		err := s.PlanAll(bp)

		// Then no error is returned and no flux calls are made
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if callCount != 0 {
			t.Errorf("expected no flux calls for destroy-only kustomizations, got %d", callCount)
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a valid stack
		m := setupFluxMocks(t)
		s := newTestFluxStack(m)

		// When PlanAll is called with a nil blueprint
		err := s.PlanAll(nil)

		// Then an error is returned
		if err == nil {
			t.Fatal("expected error for nil blueprint, got nil")
		}
	})
}
