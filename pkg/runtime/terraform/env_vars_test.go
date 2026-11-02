package terraform_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
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
