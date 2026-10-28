package terraform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_GenerateTerraformArgs(t *testing.T) {
	t.Run("GeneratesArgsSuccessfully", func(t *testing.T) {
		// Given a provider with proper configuration
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then args should be generated successfully
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedTFDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test/path"))
		if args.TFDataDir != expectedTFDataDir {
			t.Errorf("Expected TFDataDir %s, got %s", expectedTFDataDir, args.TFDataDir)
		}

		if len(args.InitArgs) == 0 {
			t.Error("Expected InitArgs to be populated")
		}

		expectedPlanPath := filepath.ToSlash(filepath.Join(expectedTFDataDir, "terraform.tfplan"))
		if len(args.PlanArgs) == 0 || !strings.Contains(args.PlanArgs[0], expectedPlanPath) {
			t.Errorf("Expected PlanArgs to contain plan path, got %v", args.PlanArgs)
		}

		if len(args.ApplyArgs) == 0 || args.ApplyArgs[len(args.ApplyArgs)-1] != expectedPlanPath {
			t.Errorf("Expected ApplyArgs to end with plan path, got %v", args.ApplyArgs)
		}
	})

	t.Run("PassesEveryFieldOfTheBackendBlock", func(t *testing.T) {
		// Given an s3 backend block with a field the header does not define
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "s3"})
		mocks.ConfigHandler.GetFunc = func(key string) any {
			if key == "terraform.backend.s3" {
				return map[string]any{"bucket": "state", "role_arn": "arn:aws:iam::123456789012:role/state"}
			}
			return nil
		}

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then the init args carry both fields of the block
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		initArgs := strings.Join(args.InitArgs, " ")
		for _, want := range []string{"bucket=state", "role_arn=arn:aws:iam::123456789012:role/state"} {
			if !strings.Contains(initArgs, want) {
				t.Errorf("Expected init args to contain %q, got %v", want, args.InitArgs)
			}
		}
		if args.KubernetesNamespace != "" {
			t.Errorf("Expected no kubernetes namespace for s3, got %q", args.KubernetesNamespace)
		}
	})

	t.Run("ReportsTheKubernetesBackendNamespace", func(t *testing.T) {
		// Given a kubernetes backend block with a namespace
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "kubernetes"})
		mocks.ConfigHandler.GetFunc = func(key string) any {
			if key == "terraform.backend.kubernetes" {
				return map[string]any{"namespace": "system-terraform", "config_path": "/kube/config"}
			}
			return nil
		}

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then the namespace matches the backend-config argument terraform receives
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if args.KubernetesNamespace != "system-terraform" {
			t.Errorf("Expected namespace system-terraform, got %q", args.KubernetesNamespace)
		}
		initArgs := strings.Join(args.InitArgs, " ")
		for _, arg := range []string{"namespace=system-terraform", "secret_suffix=test-path"} {
			if !strings.Contains(initArgs, arg) {
				t.Errorf("Expected init args to contain %q, got %v", arg, args.InitArgs)
			}
		}
	})

	t.Run("LeavesTheKubernetesNamespaceEmptyWhenUnset", func(t *testing.T) {
		// Given a kubernetes backend with no namespace in the config
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "kubernetes"})
		mocks.ConfigHandler.GetFunc = func(key string) any { return nil }

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then the namespace is empty, so backend.tfvars or terraform decides it
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if args.KubernetesNamespace != "" {
			t.Errorf("Expected an empty namespace, got %q", args.KubernetesNamespace)
		}
	})

	t.Run("IncludesVarFileArgs", func(t *testing.T) {
		// Given a provider with tfvars file
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		tfvarsPath := filepath.Join(configRoot, "terraform", "test/path.tfvars")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if filepath.ToSlash(path) == filepath.ToSlash(tfvarsPath) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then var-file args should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundVarFile := false
		for _, arg := range args.PlanArgs {
			if strings.Contains(arg, "test/path.tfvars") {
				foundVarFile = true
				break
			}
		}
		if !foundVarFile {
			t.Errorf("Expected PlanArgs to contain var-file for test/path.tfvars, got %v", args.PlanArgs)
		}
	})

	t.Run("IncludesParallelismWhenComponentHasIt", func(t *testing.T) {
		// Given a provider with component that has parallelism
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		parallelism := 5
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    parallelism: 5`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then parallelism should be included in apply and destroy args
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundParallelismInApply := false
		for _, arg := range args.ApplyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelism) {
				foundParallelismInApply = true
				break
			}
		}
		if !foundParallelismInApply {
			t.Errorf("Expected ApplyArgs to contain -parallelism=%d, got %v", parallelism, args.ApplyArgs)
		}

		foundParallelismInDestroy := false
		for _, arg := range args.DestroyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelism) {
				foundParallelismInDestroy = true
				break
			}
		}
		if !foundParallelismInDestroy {
			t.Errorf("Expected DestroyArgs to contain -parallelism=%d, got %v", parallelism, args.DestroyArgs)
		}
	})

	t.Run("HandlesIntExpressionParallelism", func(t *testing.T) {
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		parallelismValue := 10
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    parallelism: 10`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		component := mocks.Provider.GetTerraformComponent("test/path")
		if component == nil {
			t.Fatal("Expected component to be loaded")
		}

		if component.Parallelism == nil {
			t.Fatal("Expected parallelism to be set")
		}

		parallelismInt := component.Parallelism.ToInt()
		if parallelismInt == nil {
			t.Fatal("Expected ToInt() to return non-nil value")
		}
		if *parallelismInt != parallelismValue {
			t.Errorf("Expected parallelism to be %d, got %d", parallelismValue, *parallelismInt)
		}

		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundParallelismInApply := false
		for _, arg := range args.ApplyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelismValue) {
				foundParallelismInApply = true
				break
			}
		}
		if !foundParallelismInApply {
			t.Errorf("Expected ApplyArgs to contain -parallelism=%d, got %v", parallelismValue, args.ApplyArgs)
		}

		foundParallelismInDestroy := false
		for _, arg := range args.DestroyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelismValue) {
				foundParallelismInDestroy = true
				break
			}
		}
		if !foundParallelismInDestroy {
			t.Errorf("Expected DestroyArgs to contain -parallelism=%d, got %v", parallelismValue, args.DestroyArgs)
		}
	})

	t.Run("HandlesNilParallelism", func(t *testing.T) {
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		for _, arg := range args.ApplyArgs {
			if strings.Contains(arg, "-parallelism=") {
				t.Errorf("Expected ApplyArgs to not contain parallelism arg, got %v", args.ApplyArgs)
			}
		}

		for _, arg := range args.DestroyArgs {
			if strings.Contains(arg, "-parallelism=") {
				t.Errorf("Expected DestroyArgs to not contain parallelism arg, got %v", args.DestroyArgs)
			}
		}
	})

	t.Run("IncludesAutoApproveForNonInteractive", func(t *testing.T) {
		// Given a provider with non-interactive mode
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating terraform args with non-interactive mode
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", false)

		// Then auto-approve should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(args.DestroyArgs) == 0 || args.DestroyArgs[0] != "-auto-approve" {
			t.Errorf("Expected DestroyArgs to start with -auto-approve for non-interactive, got %v", args.DestroyArgs)
		}
	})

	t.Run("ReturnsErrorWhenConfigRootFails", func(t *testing.T) {
		// Given a provider with GetConfigRoot that fails
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetConfigRoot fails")
		}
		if !strings.Contains(err.Error(), "config root") {
			t.Errorf("Expected error about config root, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWindsorScratchPathFails", func(t *testing.T) {
		// Given a provider with GetWindsorScratchPath that fails
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("windsor scratch path error")
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStatFails", func(t *testing.T) {
		// Given a provider with Stat that fails
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, fmt.Errorf("stat error")
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when Stat fails")
		}
		if !strings.Contains(err.Error(), "error checking file") {
			t.Errorf("Expected error about checking file, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGenerateBackendConfigArgsFails", func(t *testing.T) {
		// Given a provider with invalid backend type
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "invalid"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GenerateBackendConfigArgs fails")
		}
		if !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected error about backend, got: %v", err)
		}
	})
}

func TestTerraformProvider_GenerateTerraformArgs_LockTimeout(t *testing.T) {
	// setupLockTimeoutMocks builds a provider with a minimal blueprint (single
	// "test/path" terraform component, local backend) so GenerateTerraformArgs
	// runs end-to-end. The GetString hook routes the named override key to the
	// supplied value; everything else falls through to defaultValue or "".
	setupLockTimeoutMocks := func(t *testing.T, lockKey string, lockValue string) *Mocks {
		t.Helper()
		mocks := setupMocks(t)
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) { return "/test/scratch", nil }
		mocks.ConfigHandler.GetContextFunc = func() string { return "default" }
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if lockKey != "" && key == lockKey {
				return lockValue
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) { return "/test/project", nil }
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join("/test/config", "blueprint.yaml") {
				return []byte("apiVersion: blueprints.windsorcli.dev/v1alpha1\nkind: Blueprint\nmetadata:\n  name: test\nterraform:\n  - path: test/path\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.Stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		return mocks
	}

	containsLockTimeout := func(args []string, want string) bool {
		for _, a := range args {
			if a == want {
				return true
			}
		}
		return false
	}

	t.Run("DefaultsToFiveMinutesWhenNotConfigured", func(t *testing.T) {
		// Given a provider with no terraform.lock.timeout override
		mocks := setupLockTimeoutMocks(t, "", "")

		// When generating args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then -lock-timeout=5m surfaces in every state-mutating args set
		want := "-lock-timeout=5m"
		for _, set := range []struct {
			name string
			args []string
		}{
			{"InitArgs", args.InitArgs},
			{"PlanArgs", args.PlanArgs},
			{"ApplyArgs", args.ApplyArgs},
			{"RefreshArgs", args.RefreshArgs},
			{"DestroyArgs", args.DestroyArgs},
			{"PlanDestroyArgs", args.PlanDestroyArgs},
			{"ImportArgs", args.ImportArgs},
		} {
			if !containsLockTimeout(set.args, want) {
				t.Errorf("expected %s to contain %q, got %v", set.name, want, set.args)
			}
		}
	})

	t.Run("HonoursConfiguredTimeout", func(t *testing.T) {
		// Given a provider with terraform.lock.timeout overridden
		mocks := setupLockTimeoutMocks(t, "terraform.lock.timeout", "30s")

		// When generating args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then -lock-timeout=30s surfaces in every state-mutating args set
		want := "-lock-timeout=30s"
		for _, set := range []struct {
			name string
			args []string
		}{
			{"InitArgs", args.InitArgs},
			{"PlanArgs", args.PlanArgs},
			{"ApplyArgs", args.ApplyArgs},
			{"RefreshArgs", args.RefreshArgs},
			{"DestroyArgs", args.DestroyArgs},
			{"PlanDestroyArgs", args.PlanDestroyArgs},
			{"ImportArgs", args.ImportArgs},
		} {
			if !containsLockTimeout(set.args, want) {
				t.Errorf("expected %s to contain %q, got %v", set.name, want, set.args)
			}
		}
	})

	t.Run("RejectsMalformedTimeoutValue", func(t *testing.T) {
		// Given a provider whose terraform.lock.timeout is unparseable
		mocks := setupLockTimeoutMocks(t, "terraform.lock.timeout", "oops")

		// When generating args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", true)

		// Then an actionable windsor-level error surfaces (not a downstream terraform parse error)
		if err == nil {
			t.Fatal("expected error for malformed timeout, got nil")
		}
		if !strings.Contains(err.Error(), "terraform.lock.timeout") || !strings.Contains(err.Error(), "oops") {
			t.Errorf("expected error to name the config key and the bad value, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "terraform.lock.timeout") {
			t.Errorf("expected a remediation that names terraform.lock.timeout, got: %v", we)
		}
	})

	t.Run("ApplyArgsKeepsTfPlanPathLast", func(t *testing.T) {
		// Given the default config
		mocks := setupLockTimeoutMocks(t, "", "")

		// When generating args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Then ApplyArgs ends with the tfplan path positional, not the flag —
		// terraform apply requires the plan-file argument to be last
		if len(args.ApplyArgs) == 0 {
			t.Fatal("ApplyArgs is empty")
		}
		last := args.ApplyArgs[len(args.ApplyArgs)-1]
		if !strings.HasSuffix(last, "terraform.tfplan") {
			t.Errorf("expected ApplyArgs to end with tfplan path, got %q (full %v)", last, args.ApplyArgs)
		}
	})
}

func TestTerraformProvider_GetTFDataDir(t *testing.T) {
	t.Run("ReturnsTFDataDirForComponentID", func(t *testing.T) {
		// Given a provider with component ID
		mocks := setupMocks(t)

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		// When getting TF data dir
		tfDataDir, err := mocks.Provider.GetTFDataDir("test-component")
		// Then it should return correct path
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test-component"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("UsesComponentGetIDWhenComponentExists", func(t *testing.T) {
		// Given a provider with component that exists in blueprint
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster/path
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		// When getting TF data dir
		tfDataDir, err := mocks.Provider.GetTFDataDir("cluster")
		// Then it should use component GetID
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "cluster"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		// Given a provider with GetWindsorScratchPath that fails
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		// When getting TF data dir
		_, err := mocks.Provider.GetTFDataDir("test-component")
		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "scratch path error") {
			t.Errorf("Expected error about scratch path, got: %v", err)
		}
	})
}

func TestTerraformProvider_GetEnvVars(t *testing.T) {
	t.Run("OnlyIncludesOutputsAccessedViaTerraformOutput", func(t *testing.T) {
		// Given a blueprint with component inputs using terraform_output
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'`

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "isolated_subnet_ids": {"value": ["subnet-1"]}, "private_subnet_ids": {"value": ["subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then only outputs accessed via terraform_output should be included
		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		unexpectedVars := []string{
			"TF_VAR_isolated_subnet_ids",
			"TF_VAR_private_subnet_ids",
			"TF_VAR_public_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			// All outputs are cached when fetched, even if not accessed via terraform_output()
			if isolatedSubnets, exists := networkCache["isolated_subnet_ids"]; !exists {
				t.Error("Expected isolated_subnet_ids to be cached (all outputs are cached when fetched)")
			} else {
				// Verify it's the actual value, not a deferred expression
				expected := []any{"subnet-1"}
				if fmt.Sprintf("%v", isolatedSubnets) != fmt.Sprintf("%v", expected) {
					t.Errorf("Expected isolated_subnet_ids to be cached as actual value, got: %v", isolatedSubnets)
				}
			}
		}
	})

	t.Run("AccumulatesMultipleRequestedOutputs", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'
      subnet_ids: '${terraform_output("network", "private_subnet_ids")}'`

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "private_subnet_ids": {"value": ["subnet-1", "subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}, "isolated_subnet_ids": {"value": ["subnet-4"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		expectedSubnetIds := `["subnet-1","subnet-2"]`
		if envVars["TF_VAR_subnet_ids"] != expectedSubnetIds {
			t.Errorf("Expected TF_VAR_subnet_ids to be %s, got: %s", expectedSubnetIds, envVars["TF_VAR_subnet_ids"])
		}

		unexpectedVars := []string{
			"TF_VAR_public_subnet_ids",
			"TF_VAR_isolated_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			if str, isString := networkCache["private_subnet_ids"].(string); isString && strings.HasPrefix(str, "terraform_output(") {
				t.Errorf("Expected private_subnet_ids to be accessed (not deferred), got: %v", networkCache["private_subnet_ids"])
			}
		}
	})

	t.Run("EvaluatesInputsWithContainsExpression", func(t *testing.T) {
		// Given a blueprint with inputs containing expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			result := make(map[string]any)
			for key, value := range values {
				if strVal, ok := value.(string); ok && strings.Contains(strVal, "${") {
					result[key] = "evaluated-vpc-id"
				} else {
					result[key] = value
				}
			}
			return result, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then inputs with expressions should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "evaluated-vpc-id" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'evaluated-vpc-id', got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("SkipsTFVarForInputsWithoutExpressions", func(t *testing.T) {
		// Given a blueprint with inputs without expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      plain_value: "plain"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-expression inputs should not be emitted as TF_VAR_*
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_cluster_name"]; exists {
			t.Error("Expected TF_VAR_cluster_name to be skipped for non-expression input")
		}
		if _, exists := envVars["TF_VAR_plain_value"]; exists {
			t.Error("Expected TF_VAR_plain_value to be skipped for non-expression input")
		}
	})

	t.Run("IncludesBaseContextAndProjectRootVars", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "my-context"
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/my/project", nil
		}

		envVars, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_context"] != "my-context" {
			t.Errorf("Expected TF_VAR_context to be 'my-context', got %q", envVars["TF_VAR_context"])
		}
		if envVars["TF_VAR_project_root"] != "/my/project" {
			t.Errorf("Expected TF_VAR_project_root to be '/my/project', got %q", envVars["TF_VAR_project_root"])
		}
	})

	t.Run("HandlesComponentNotFoundWithProjectRoot", func(t *testing.T) {
		// Given a provider with project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		// When getting env vars for nonexistent component
		envVars, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return empty env vars without error
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars == nil {
			t.Error("Expected envVars to be non-nil")
		}
	})

	t.Run("HandlesComponentNotFoundWithoutProjectRoot", func(t *testing.T) {
		// Given a provider without project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		// When getting env vars for nonexistent component
		_, _, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when component not found and project root unavailable")
		}
	})

	t.Run("HandlesEvaluateMapError", func(t *testing.T) {
		// Given a provider with evaluator that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, errors.New("evaluation error")
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when EvaluateMap fails")
		}
	})

	t.Run("HandlesNonStringEvaluatedValue", func(t *testing.T) {
		// Given a provider with evaluator that returns non-string value
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			return map[string]any{
				"vpc_id": 42,
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`"42"`), nil
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-string values should be JSON marshaled
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != `"42"` {
			t.Errorf("Expected TF_VAR_vpc_id to be JSON marshaled, got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		// Given a provider with JsonMarshal that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if !evaluateDeferred {
				return map[string]any{
					"vpc_id": "${terraform_output('vpc', 'id')}",
				}, nil
			}
			return map[string]any{
				"vpc_id": make(chan int),
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then marshal error should be skipped and var not set
		if err != nil {
			t.Fatalf("Expected no error (marshal error is skipped), got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when JSON marshal fails")
		}
	})

	t.Run("HandlesGenerateTerraformArgsError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when GenerateTerraformArgs fails")
		}
		if !strings.Contains(err.Error(), "error generating terraform args") {
			t.Errorf("Expected error to mention generating terraform args, got: %v", err)
		}
	})

	t.Run("HandlesGetBaseEnvVarsForComponentError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// Only getBaseEnvVarsForComponent calls GetProjectRoot in this flow.
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		_, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when getBaseEnvVarsForComponent fails")
		}
		if !strings.Contains(err.Error(), "project root") {
			t.Errorf("Expected error to mention project root, got: %v", err)
		}
	})

	t.Run("HandlesEvaluatedKeyNotExist", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when key doesn't exist in evaluated result")
		}
	})

	t.Run("SkipsTFVarForNonRuntimeScopeExpressions", func(t *testing.T) {
		// Given a provider with config scope and component input referencing scope
		configHandler := config.NewMockConfigHandler()
		configHandler.GetContextFunc = func() string { return "default" }
		configHandler.GetConfigRootFunc = func() (string, error) { return "/test/config", nil }
		configHandler.GetWindsorScratchPathFunc = func() (string, error) { return "/test/scratch", nil }
		configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BackendType: "none", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

		patches := []any{map[string]any{"op": "add", "path": "/machine/foo"}}
		scope := map[string]any{
			"some_block": map[string]any{
				"patchVars": map[string]any{
					"common_config_patches": patches,
				},
			},
		}
		mocks.Provider.SetConfigScope(scope)
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"common_config_patches": "${some_block.patchVars.common_config_patches}",
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-runtime expressions should remain tfvars-only and not emit TF_VAR
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_common_config_patches"]; exists {
			t.Fatal("Expected TF_VAR_common_config_patches to be skipped for non-runtime scope expression")
		}
	})

	t.Run("SkipsTFVarForResolvedInputsWithoutExpressions", func(t *testing.T) {
		// Given a provider with component having resolved (non-expression) input
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		patches := []any{map[string]any{"op": "add", "path": "/machine/bar"}}
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"common_config_patches": patches,
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		// When getting env vars
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-expression input should remain tfvars-only and not emit TF_VAR
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_common_config_patches"]; exists {
			t.Fatal("Expected TF_VAR_common_config_patches to be skipped for resolved non-expression input")
		}
	})

	t.Run("SkipsTFVarForSimpleExpression", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      talos_version: "${config.kubernetes.version}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, exists := envVars["TF_VAR_talos_version"]; exists {
			t.Fatal("Expected TF_VAR_talos_version to be skipped for simple expression")
		}
	})

	t.Run("AllowsRuntimeDeferredExpression", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      talos_version: "${terraform_output('bootstrap', 'talos_version')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			if evaluateDeferred {
				return map[string]any{"talos_version": "v1.12.6"}, nil
			}
			return map[string]any{"talos_version": "${terraform_output('bootstrap', 'talos_version')}"}, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if envVars["TF_VAR_talos_version"] != "v1.12.6" {
			t.Fatalf("Expected runtime deferred TF_VAR_talos_version to be exported, got %q", envVars["TF_VAR_talos_version"])
		}
	})

	t.Run("DoesNotSerializeDeferredValueWrapperIntoTFVar", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		component := blueprintv1alpha1.TerraformComponent{
			Path: "cluster",
			Inputs: map[string]any{
				"network_cidr": evaluator.DeferredValue{
					Expression: "${terraform_output(\"workstation\", \"network_cidr\")}",
				},
				"network_name": &evaluator.DeferredValue{
					Expression: "${terraform_output(\"workstation\", \"network_name\")}",
				},
			},
		}
		component.FullPath = "/test/project/terraform/cluster"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			out := make(map[string]any, len(values))
			for key, value := range values {
				out[key] = value
			}
			return out, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		for key, value := range envVars {
			if strings.HasPrefix(key, "TF_VAR_") && strings.HasPrefix(value, "{\"Expression\":") {
				t.Fatalf("Expected TF_VAR to never serialize DeferredValue wrapper, got %s=%s", key, value)
			}
		}

		if got := envVars["TF_VAR_network_cidr"]; got != "${terraform_output(\"workstation\", \"network_cidr\")}" {
			t.Fatalf("Expected TF_VAR_network_cidr to contain raw expression string, got %q", got)
		}
		if got := envVars["TF_VAR_network_name"]; got != "${terraform_output(\"workstation\", \"network_name\")}" {
			t.Fatalf("Expected TF_VAR_network_name to contain raw expression string, got %q", got)
		}
	})

	t.Run("SkipsTFVarWhenDeferredOutputResolvesToNil", func(t *testing.T) {
		// Given a component whose input depends on a terraform_output from an unapplied dependency.
		// The evaluator returns nil for the deferred pass (no outputs yet), which must NOT result
		// in TF_VAR_network_cidr being set to the string "null".
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		component := blueprintv1alpha1.TerraformComponent{
			Path: "compute",
			Inputs: map[string]any{
				"network_cidr": "${terraform_output(\"workstation\", \"network_cidr\")}",
			},
		}
		component.FullPath = "/test/project/terraform/compute"
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			out := make(map[string]any, len(values))
			for key, value := range values {
				if evaluateDeferred {
					// Dependency not applied yet — resolve to nil.
					out[key] = nil
				} else {
					// First pass: preserve the expression so the caller sees it is deferred.
					out[key] = value
				}
			}
			return out, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then TF_VAR_network_cidr must not be set (not even to the string "null")
		if val, exists := envVars["TF_VAR_network_cidr"]; exists {
			t.Errorf("Expected TF_VAR_network_cidr to be absent when dependency is not applied, got %q", val)
		}
	})

	t.Run("MergesTerraformScopedDotEnvValues", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file with a plain value
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the terraform/.env value should be present
		if envVars["HYPERV_HOST"] != "hyperv.local" {
			t.Errorf("Expected HYPERV_HOST=hyperv.local, got %q", envVars["HYPERV_HOST"])
		}
	})

	t.Run("WarnsOnLooseTerraformScopedDotEnvPermissions", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file with group/world-readable permissions
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0644}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.Goos = func() string { return "linux" }
		var stderr bytes.Buffer
		mocks.Provider.warningWriter = &stderr

		// When GetEnvVars is called
		if _, _, _, err := mocks.Provider.GetEnvVars("compute", false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then a warning should be written to the injected writer
		if !strings.Contains(stderr.String(), "Warning") {
			t.Errorf("Expected a permission warning, got %q", stderr.String())
		}
	})

	t.Run("NoWarningOnRestrictedTerraformScopedDotEnvPermissions", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file restricted to the owner
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte("HYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.Goos = func() string { return "linux" }
		var stderr bytes.Buffer
		mocks.Provider.warningWriter = &stderr

		// When GetEnvVars is called
		if _, _, _, err := mocks.Provider.GetEnvVars("compute", false); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then no warning should be written
		if stderr.String() != "" {
			t.Errorf("Expected no warning, got %q", stderr.String())
		}
	})

	t.Run("ResolvesSecretExpressionInTerraformScopedDotEnv", func(t *testing.T) {
		// Given a terraform/.env file with a secret expression and a mock evaluator
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte(`HYPERV_PASSWORD=${secret("op://vault/item/field")}` + "\n"), nil
			}
			return nil, os.ErrNotExist
		}
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			if strings.Contains(expression, "secret(") {
				return "resolved-secret", nil
			}
			return expression, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When GetEnvVars is called
		envVars, _, _, err := mocks.Provider.GetEnvVars("compute", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the expression should be resolved through the evaluator
		if envVars["HYPERV_PASSWORD"] != "resolved-secret" {
			t.Errorf("Expected resolved-secret, got %q", envVars["HYPERV_PASSWORD"])
		}
	})

	t.Run("ReturnsErrorWhenTerraformScopedDotEnvReadFails", func(t *testing.T) {
		// Given a terraform/.env file that exists but fails to read
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if path == dotEnvPath {
				return fakeFileInfo{mode: 0600}, nil
			}
			return nil, os.ErrNotExist
		}
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return nil, fmt.Errorf("mock read error")
			}
			return nil, os.ErrNotExist
		}

		// When GetEnvVars is called
		_, _, _, err := mocks.Provider.GetEnvVars("compute", false)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

// TestTerraformProvider_TerraformScopedEnvKeys tests the TerraformScopedEnvKeys method.
func TestTerraformProvider_TerraformScopedEnvKeys(t *testing.T) {
	t.Run("ReturnsKeyNamesWithoutResolvingValues", func(t *testing.T) {
		// Given a terraform/.env file with a secret expression and an evaluator that must not be called
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return []byte(`HYPERV_PASSWORD=${secret("op://vault/item/field")}` + "\nHYPERV_HOST=hyperv.local\n"), nil
			}
			return nil, os.ErrNotExist
		}
		evaluateCalled := false
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateFunc = func(expression string, facetPath string, scope map[string]any, evaluateDeferred bool) (any, error) {
			evaluateCalled = true
			return "resolved", nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When TerraformScopedEnvKeys is called
		keys, err := mocks.Provider.TerraformScopedEnvKeys()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then both key names should be returned and the evaluator never invoked
		if len(keys) != 2 {
			t.Fatalf("Expected 2 keys, got %v", keys)
		}
		if evaluateCalled {
			t.Error("Expected evaluator not to be called")
		}
	})

	t.Run("ReturnsEmptyWhenFileAbsent", func(t *testing.T) {
		// Given no terraform/.env file
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		// When TerraformScopedEnvKeys is called
		keys, err := mocks.Provider.TerraformScopedEnvKeys()

		// Then no error and no keys should be returned
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("Expected no keys, got %v", keys)
		}
	})

	t.Run("ReturnsErrorWhenReadFailsForOtherReason", func(t *testing.T) {
		// Given a terraform/.env file that fails to read for a non-NotExist reason
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		dotEnvPath := filepath.Join("/test/config", "terraform", ".env")
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == dotEnvPath {
				return nil, fmt.Errorf("mock permission denied")
			}
			return nil, os.ErrNotExist
		}

		// When TerraformScopedEnvKeys is called
		_, err := mocks.Provider.TerraformScopedEnvKeys()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

func TestTerraformProvider_FormatArgsForEnv(t *testing.T) {
	t.Run("FormatsVarFileArgs", func(t *testing.T) {
		// Given args with var-file
		mocks := setupMocks(t)

		args := []string{"-var-file=/path/to/file.tfvars"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then var-file should be quoted
		expected := `-var-file="/path/to/file.tfvars"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsOutArgs", func(t *testing.T) {
		// Given args with out
		mocks := setupMocks(t)

		args := []string{"-out=plan.tfplan"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then out should be quoted
		expected := `-out="plan.tfplan"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsBackendConfigArgs", func(t *testing.T) {
		// Given args with backend-config
		mocks := setupMocks(t)

		args := []string{"-backend-config=key=value"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then backend-config should be quoted
		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesAlreadyQuotedBackendConfig", func(t *testing.T) {
		// Given args with already quoted backend-config
		mocks := setupMocks(t)

		args := []string{`-backend-config="key=value"`}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then quotes should be preserved
		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsRelativePaths", func(t *testing.T) {
		// Given args with relative path
		mocks := setupMocks(t)

		args := []string{"./relative/path"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then relative path should be quoted
		expected := `"./relative/path"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsWindowsDrivePaths", func(t *testing.T) {
		// Given args with Windows drive paths
		mocks := setupMocks(t)

		args := []string{"C:/windows/path", "D:\\windows\\path"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then Windows paths should be quoted
		if !strings.Contains(result, `"C:/windows/path"`) {
			t.Errorf("Expected result to contain quoted C:/windows/path, got %s", result)
		}
		if !strings.Contains(result, `"D:\windows\path"`) {
			t.Errorf("Expected result to contain quoted D:\\windows\\path, got %s", result)
		}
	})

	t.Run("PreservesFlags", func(t *testing.T) {
		// Given args with flags
		mocks := setupMocks(t)

		args := []string{"-flag", "--long-flag", "-backend=true"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then flags should be preserved without quotes
		expected := `-flag --long-flag -backend=true`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("HandlesMultipleArgs", func(t *testing.T) {
		// Given args with multiple types
		mocks := setupMocks(t)

		args := []string{"-var-file=file.tfvars", "-out=plan.tfplan", "/path/to/file"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then all args should be formatted correctly
		if !strings.Contains(result, `-var-file="file.tfvars"`) {
			t.Errorf("Expected result to contain formatted var-file, got %s", result)
		}
		if !strings.Contains(result, `-out="plan.tfplan"`) {
			t.Errorf("Expected result to contain formatted out, got %s", result)
		}
		if !strings.Contains(result, `"/path/to/file"`) {
			t.Errorf("Expected result to contain quoted path, got %s", result)
		}
	})

	t.Run("HandlesEmptyArgs", func(t *testing.T) {
		// Given empty args
		mocks := setupMocks(t)

		args := []string{}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then it should return empty string
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func Test_sanitizeForK8s(t *testing.T) {
	t.Run("SanitizesBasicString", func(t *testing.T) {
		// Given a string with mixed case and underscores
		// When sanitizing for K8s
		result := sanitizeForK8s("Test-String_123")

		// Then it should convert to lowercase and replace underscores
		expected := "test-string-123"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("TrimsTo63Characters", func(t *testing.T) {
		// Given a long string
		longString := strings.Repeat("a", 100)

		// When sanitizing for K8s
		result := sanitizeForK8s(longString)

		// Then it should be trimmed to 63 characters
		if len(result) > 63 {
			t.Errorf("Expected max 63 characters, got %d: %s", len(result), result)
		}
	})

	t.Run("HandlesMultipleDashes", func(t *testing.T) {
		// Given a string with multiple dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("test---string")

		// Then multiple dashes should be collapsed
		if strings.Contains(result, "---") {
			t.Errorf("Expected multiple dashes to be collapsed, got %s", result)
		}
	})

	t.Run("TrimsLeadingTrailingDashes", func(t *testing.T) {
		// Given a string with leading and trailing dashes
		// When sanitizing for K8s
		result := sanitizeForK8s("-test-string-")

		// Then leading and trailing dashes should be trimmed
		if strings.HasPrefix(result, "-") || strings.HasSuffix(result, "-") {
			t.Errorf("Expected no leading/trailing dashes, got %s", result)
		}
	})
}
