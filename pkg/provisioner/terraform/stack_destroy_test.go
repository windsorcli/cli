package terraform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	terraformRuntime "github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_DestroyAll(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims

		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", "contexts", "local", "terraform", "remote", "path"),
				},
			}
		}

		// Default `terraform show -json` to return state JSON containing one
		// resource AND a plan JSON containing an "update" action, so the full
		// flow runs: classifyDestroyAction sees non-empty state, then a plan
		// with updates, routes to PrepThenDestroy. Tests that want to verify
		// the Noop or DestroyOnly branches override this themselves.
		// Distinguish state-show vs plan-show by len(args): `show -json` is 3
		// args, `show -json <planpath>` is 4.
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("UsesDestroyIdleAbsoluteAndGracePeriod", func(t *testing.T) {
		// Given a stack whose terraform destroy exec is bounded by an idle timeout, an
		// absolute backstop, and a grace period. A hard kill on timeout gave terraform no
		// chance to checkpoint state or release its backend lock, so destroy must run
		// through the idle-timeout variant.
		stack, mocks := setup(t)
		var gotIdleTimeout, gotAbsoluteTimeout, gotGracePeriod time.Duration
		var sawDestroy bool
		mocks.Shell.ExecSilentWithEnvAndIdleTimeoutFunc = func(command string, env map[string]string, args []string, idleTimeout, absoluteTimeout, gracePeriod time.Duration) (string, error) {
			sawDestroy = true
			gotIdleTimeout = idleTimeout
			gotAbsoluteTimeout = absoluteTimeout
			gotGracePeriod = gracePeriod
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When DestroyAll runs
		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the destroy exec runs with the bounded idle timeout, absolute backstop, and
		// grace period, not the immediate-kill variant
		if !sawDestroy {
			t.Fatal("Expected terraform destroy to run via ExecSilentWithEnvAndIdleTimeout")
		}
		if gotIdleTimeout != constants.DefaultTerraformDestroyIdleTimeout {
			t.Errorf("Expected idle timeout %v, got %v", constants.DefaultTerraformDestroyIdleTimeout, gotIdleTimeout)
		}
		if gotAbsoluteTimeout != constants.DefaultTerraformDestroyAbsoluteTimeout {
			t.Errorf("Expected absolute timeout %v, got %v", constants.DefaultTerraformDestroyAbsoluteTimeout, gotAbsoluteTimeout)
		}
		if gotGracePeriod != constants.DefaultTerraformDestroyGracePeriod {
			t.Errorf("Expected grace period %v, got %v", constants.DefaultTerraformDestroyGracePeriod, gotGracePeriod)
		}
	})

	t.Run("UsesRefreshTimeout", func(t *testing.T) {
		// Given a stack whose terraform refresh exec is bounded by a timeout
		stack, mocks := setup(t)
		var gotTimeout time.Duration
		var sawRefresh bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "refresh" {
				sawRefresh = true
				gotTimeout = timeout
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When DestroyAll runs
		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the refresh exec runs with the bounded timeout: a component's own
		// kubernetes/helm provider can't hang the whole destroy dialing a broken cluster
		if !sawRefresh {
			t.Fatal("Expected terraform refresh to run via ExecSilentWithEnvAndTimeout")
		}
		if gotTimeout != constants.DefaultTerraformRefreshTimeout {
			t.Errorf("Expected timeout %v, got %v", constants.DefaultTerraformRefreshTimeout, gotTimeout)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		blueprint := createTestBlueprint()
		_, err := stack.DestroyAll(context.Background(), blueprint, false)
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ReportsMissingDirectoriesAsSkipped", func(t *testing.T) {
		// Given a stat shim that reports every component directory as missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When DestroyAll runs against a blueprint with two components
		blueprint := createTestBlueprint()
		skipped, err := stack.DestroyAll(context.Background(), blueprint, false)

		// Then no error is returned and both components appear in the skipped list,
		// matching MigrateState's contract for missing-on-disk components.
		if err != nil {
			t.Fatalf("Expected no error when directory doesn't exist, got %v", err)
		}
		if len(skipped.Skipped) != len(blueprint.TerraformComponents) {
			t.Fatalf("Expected %d skipped components, got %d: %v", len(blueprint.TerraformComponents), len(skipped.Skipped), skipped.Skipped)
		}
	})

	t.Run("SkipsComponentsListedInExcludeIDs", func(t *testing.T) {
		// Given a blueprint with multiple components and DestroyAll called with one
		// of their IDs in excludeIDs, the named component must not run terraform
		// init/destroy at all — symmetric-destroy peels the backend off the bulk
		// pass at the cmd layer and destroys it last after migrating its state.
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{Source: "source1", Path: "backend"},
			{Source: "source1", Path: "cluster"},
		}

		for _, p := range []string{"backend", "cluster"} {
			dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", p)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				terraformCommands = append(terraformCommands, strings.Join(args, " "))
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false, "backend"); err != nil {
			t.Errorf("Expected DestroyAll to return nil, got %v", err)
		}

		for _, c := range terraformCommands {
			if strings.Contains(c, "backend") && !strings.Contains(c, "cluster") {
				t.Errorf("Expected no terraform commands for excluded \"backend\" component, but found: %q", c)
			}
		}
		if len(terraformCommands) == 0 {
			t.Errorf("Expected terraform commands for non-excluded \"cluster\" component, got none")
		}
	})

	t.Run("ExcludesComponentsWithEnabledFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		enabledFalse := false
		blueprint := createTestBlueprint()
		blueprint.Sources = append(blueprint.Sources, blueprintv1alpha1.Source{
			Name: "source2",
			Url:  "https://github.com/example/example2.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		})
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "module/path1",
			},
			{
				Source:  "source2",
				Path:    "module/path2",
				Enabled: &blueprintv1alpha1.BoolExpression{Value: &enabledFalse, IsExpr: false},
			},
		}

		path1Dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", "module/path1")
		if err := os.MkdirAll(path1Dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				terraformCommands = append(terraformCommands, strings.Join(args, " "))
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		for _, cmd := range terraformCommands {
			if strings.Contains(cmd, "path2") {
				t.Errorf("Expected no terraform commands for path2 (enabled: false), but found: %v", terraformCommands)
				break
			}
		}
		if len(terraformCommands) == 0 {
			t.Errorf("Expected terraform commands for path1 (enabled), but got none: %v", terraformCommands)
		}
	})

	t.Run("SkipComponentsWithDestroyFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		destroyFalse := false
		blueprint := createTestBlueprint()
		blueprint.Sources = append(blueprint.Sources, blueprintv1alpha1.Source{
			Name: "source2",
			Url:  "https://github.com/example/example2.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		})
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "source1",
				Path:     "module/path1",
				FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "module/path1"),
				Destroy:  &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
			},
			{
				Source:   "source2",
				Path:     "module/path2",
				FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "module/path2"),
			},
		}

		if err := os.MkdirAll(blueprint.TerraformComponents[0].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.MkdirAll(blueprint.TerraformComponents[1].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				cmdStr := strings.Join(args, " ")
				terraformCommands = append(terraformCommands, cmdStr)
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		foundPath1Commands := false
		foundPath2Commands := false

		for _, cmd := range terraformCommands {
			if strings.Contains(cmd, "module/path1") || strings.Contains(cmd, "/path1") || (strings.Contains(cmd, "path1") && !strings.Contains(cmd, "path2")) {
				foundPath1Commands = true
			}
			if strings.Contains(cmd, "module/path2") || strings.Contains(cmd, "/path2") || (strings.Contains(cmd, "path2") && !strings.Contains(cmd, "path1")) {
				foundPath2Commands = true
			}
		}

		if foundPath1Commands {
			t.Errorf("Expected no terraform commands for path1 (destroy: false), but found commands: %v", terraformCommands)
		}
		if !foundPath2Commands {
			t.Errorf("Expected terraform commands for path2 (destroy: true), but found none. Commands executed: %v", terraformCommands)
		}
	})

	t.Run("ClearsBackendPointerForComponentWithDestroyFalse", func(t *testing.T) {
		// A skipped destroy: false component still gets its backend pointer cleared.
		stack, mocks := setup(t)

		mocks.Runtime.TerraformProvider.ClearCache()

		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		destroyFalse := false
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "source1",
				Path:     "module/path1",
				FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "module/path1"),
				Destroy:  &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
			},
		}
		if err := os.MkdirAll(blueprint.TerraformComponents[0].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected DestroyAll to return nil, got %v", err)
		}

		var pointerRemoved bool
		for _, p := range removed {
			if strings.HasSuffix(p, "/terraform.tfstate") {
				pointerRemoved = true
				break
			}
		}
		if !pointerRemoved {
			t.Errorf("Expected the backend pointer for the skipped component to be removed, got removals: %v", removed)
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		// DestroyAll runs every terraform subcommand via ExecSilentWithEnv so the outer
		// "Destroying <path>" progress span is the only label the user sees. state list
		// is seeded non-empty so the idempotency short-circuit does not fire. The mock
		// returns a recognizable terraform diagnostic alongside the error to verify it
		// is surfaced via presenter.Warn rather than discarded.
		stack, mocks := setup(t)
		var captured strings.Builder
		ctx := withWarnings(&captured)

		const tfDiagnostic = "Error: deleting S3 Bucket (terraform-state-x): operation error S3: BucketNotEmpty"
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "destroy" {
				return tfDiagnostic, fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		_, err := stack.DestroyAll(ctx, blueprint, false)
		expectedError := "error running terraform destroy for"
		if err == nil {
			t.Fatalf("Expected destroy error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, tfDiagnostic) {
			t.Errorf("Expected terraform diagnostic %q to be surfaced via presenter.Warn, got %q", tfDiagnostic, warningOutput)
		}
	})

	t.Run("RetriesTerraformDestroyThenSucceeds", func(t *testing.T) {
		// Given a terraform destroy that fails once (a transient async cloud-side
		// dependency, e.g. an orphaned firewall) then succeeds on retry
		stack, mocks := setup(t)
		var captured strings.Builder
		ctx := withWarnings(&captured)
		var destroyCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" && strings.Contains(filepath.ToSlash(args[0]), "/local/path") {
				destroyCalls++
				if destroyCalls == 1 {
					return "boom", fmt.Errorf("mock transient destroy failure")
				}
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When DestroyAll runs
		result, err := stack.DestroyAll(ctx, blueprint, false)

		// Then the component destroys successfully after one retry, and the failed
		// first attempt is still visible via presenter.Warn rather than vanishing
		if err != nil {
			t.Fatalf("Expected the retry to absorb the transient failure, got %v", err)
		}
		if destroyCalls != 2 {
			t.Errorf("Expected terraform destroy to run twice, got %d", destroyCalls)
		}
		if !slices.Contains(result.Destroyed, "local/path") {
			t.Errorf("Expected local/path to be reported as destroyed, got %v", result)
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, "attempt 1/") || !strings.Contains(warningOutput, "boom") {
			t.Errorf("Expected the failed first attempt to be logged via presenter.Warn, got %q", warningOutput)
		}
	})

	t.Run("GivesUpAfterExhaustingDestroyRetries", func(t *testing.T) {
		// Given a terraform destroy that fails on every attempt with a distinct
		// diagnostic each time
		stack, mocks := setup(t)
		var captured strings.Builder
		ctx := withWarnings(&captured)
		var destroyCalls int
		var sleepCalls int
		mocks.Shims.TimeSleep = func(time.Duration) { sleepCalls++ }
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" && strings.Contains(filepath.ToSlash(args[0]), "/local/path") {
				destroyCalls++
				return fmt.Sprintf("boom-%d", destroyCalls), fmt.Errorf("mock persistent destroy failure %d", destroyCalls)
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When DestroyAll runs
		_, err := stack.DestroyAll(ctx, blueprint, false)

		// Then every attempt is exhausted, with a backoff wait and a presenter.Warn
		// line naming each non-final attempt's own diagnostic before the wrapped
		// error surfaces the last one
		if err == nil {
			t.Fatal("Expected an error once retries are exhausted")
		}
		if destroyCalls != stack.destroyRetryAttempts {
			t.Errorf("Expected %d destroy attempts, got %d", stack.destroyRetryAttempts, destroyCalls)
		}
		if sleepCalls != stack.destroyRetryAttempts-1 {
			t.Errorf("Expected %d backoff waits, got %d", stack.destroyRetryAttempts-1, sleepCalls)
		}
		warningOutput := captured.String()
		for attempt := 1; attempt < stack.destroyRetryAttempts; attempt++ {
			if !strings.Contains(warningOutput, fmt.Sprintf("boom-%d", attempt)) {
				t.Errorf("Expected attempt %d's own diagnostic %q to be logged via presenter.Warn, got %q", attempt, fmt.Sprintf("boom-%d", attempt), warningOutput)
			}
		}
	})

	t.Run("StopsRetryingOnDestroyTimeout", func(t *testing.T) {
		// Given a terraform destroy that times out. A timeout means the provider is
		// unreachable rather than momentarily inconsistent, so it must not be retried.
		stack, mocks := setup(t)
		var destroyCalls int
		var sleepCalls int
		mocks.Shims.TimeSleep = func(time.Duration) { sleepCalls++ }
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalls++
				return "", fmt.Errorf("%w after %v", shell.ErrCommandTimedOut, timeout)
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When DestroyAll runs
		_, err := stack.DestroyAll(context.Background(), blueprint, false)

		// Then the timeout fails immediately, with no retry and no backoff wait
		if err == nil || !errors.Is(err, shell.ErrCommandTimedOut) {
			t.Fatalf("Expected an error wrapping ErrCommandTimedOut, got %v", err)
		}
		if destroyCalls != 1 {
			t.Errorf("Expected exactly one destroy attempt before giving up on timeout, got %d", destroyCalls)
		}
		if sleepCalls != 0 {
			t.Errorf("Expected no backoff wait after a timeout, got %d", sleepCalls)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		_, err := stack.DestroyAll(context.Background(), nil, false)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()
		_, err := stack.DestroyAll(context.Background(), blueprint, false)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("IgnoresErrorRemovingBackendOverride", func(t *testing.T) {
		// Given a stack with Remove that fails
		stack, mocks := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		backendOverridePath := filepath.Join(projectRoot, ".windsor", "contexts", "local", "remote", "path", "backend_override.tf")
		if err := os.MkdirAll(filepath.Dir(backendOverridePath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(backendOverridePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backend override file: %v", err)
		}
		mocks.Shims.Remove = func(path string) error {
			return fmt.Errorf("remove error")
		}
		blueprint := createTestBlueprint()

		// When running Down
		_, err := stack.DestroyAll(context.Background(), blueprint, false)

		// Then it should succeed (cleanup errors are ignored)
		if err != nil {
			t.Errorf("Expected no error (cleanup is best-effort), got: %v", err)
		}
	})

	t.Run("HandlesNamedComponent", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/cluster",
					Source: "",
					Inputs: map[string]any{
						"node_count": 3,
					},
				},
			},
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected Down to succeed with named component, got %v", err)
		}
	})

	t.Run("HandlesNamedComponentWithSource", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "network")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			Sources: []blueprintv1alpha1.Source{
				{
					Name: "source1",
					Url:  "https://github.com/example/example.git",
					Ref:  blueprintv1alpha1.Reference{Branch: "main"},
				},
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Name:   "network",
					Path:   "terraform/network",
					Source: "source1",
					Inputs: map[string]any{
						"cidr": "10.0.0.0/16",
					},
				},
			},
		}

		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Errorf("Expected Down to succeed with named component with source, got %v", err)
		}
	})

	t.Run("SetsTFVarOperationToDestroy", func(t *testing.T) {
		// Given a stack and a blueprint; state list is seeded non-empty so the
		// idempotency short-circuit does not fire.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When DestroyAll is called, capture the env passed to terraform destroy.
		// All destroy-phase subcommands run via ExecSilentWithEnv now.
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_, _ = stack.DestroyAll(context.Background(), blueprint, false)

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsRefreshGatedDestroyPerComponentInReverseOrder", func(t *testing.T) {
		// Given a stack that records the per-component terraform steps in order, and
		// state JSON reports a resource so the idempotency short-circuit does not fire.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		type step struct {
			component  string
			subcommand string
			operation  string
		}
		var sequence []step
		componentOf := func(args []string) string {
			if len(args) == 0 {
				return ""
			}
			// Normalize OS-native separators to forward slashes before matching: FullPath
			// goes through filepath.FromSlash, so on Windows args[0] is "-chdir=...\remote\path"
			// and the literal substring "remote/path" never matches.
			normalized := filepath.ToSlash(args[0])
			switch {
			case strings.Contains(normalized, "remote/path"):
				return "remote/path"
			case strings.Contains(normalized, "local/path"):
				return "local/path"
			}
			return ""
		}
		record := func(env map[string]string, args []string) {
			if len(args) <= 1 {
				return
			}
			sequence = append(sequence, step{
				component:  componentOf(args),
				subcommand: args[1],
				operation:  env["TF_VAR_operation"],
			})
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}

		// When destroying all components
		if _, err := stack.DestroyAll(context.Background(), blueprint, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then components are walked in reverse dependency order, and each component is
		// taken through the idempotent destroy flow end-to-end before the next one begins:
		// init → show (pre-refresh state JSON) → refresh → show (post-refresh state JSON) →
		// destroy. The pre-refresh show-json is load-bearing for the empty-state skip; refresh
		// reconciles state with cloud reality for partial-destroy cases; the post-refresh
		// show-json drives the second skip check. TF_VAR_operation=destroy is forwarded to
		// commands that include TF_VAR_* in their env (show, refresh, destroy); init does
		// not forward TF_VAR_* and relies on the process env set by windsor env.
		type ordered struct {
			component  string
			subcommand string
		}
		expected := []ordered{
			{component: "local/path", subcommand: "init"},
			{component: "local/path", subcommand: "show"},
			{component: "local/path", subcommand: "refresh"},
			{component: "local/path", subcommand: "show"},
			{component: "local/path", subcommand: "destroy"},
			{component: "remote/path", subcommand: "init"},
			{component: "remote/path", subcommand: "show"},
			{component: "remote/path", subcommand: "refresh"},
			{component: "remote/path", subcommand: "show"},
			{component: "remote/path", subcommand: "destroy"},
		}
		if len(sequence) != len(expected) {
			t.Fatalf("Expected %d steps, got %d: %+v", len(expected), len(sequence), sequence)
		}
		for i, want := range expected {
			got := ordered{component: sequence[i].component, subcommand: sequence[i].subcommand}
			if got != want {
				t.Errorf("step %d: got %+v, want %+v", i, got, want)
			}
			switch sequence[i].subcommand {
			case "refresh", "show", "destroy":
				if sequence[i].operation != "destroy" {
					t.Errorf("step %d (%s %s): TF_VAR_operation = %q, want %q", i, sequence[i].component, sequence[i].subcommand, sequence[i].operation, "destroy")
				}
			}
		}
	})

	t.Run("RefreshFailureFallsThroughToDestroyWithRefreshTrue", func(t *testing.T) {
		// Given a non-empty-state component in a bulk destroy where refresh fails — same
		// fallback contract as single-component Destroy. A transient refresh failure on one
		// component must not abort the whole bulk pass; we fall through to `terraform destroy
		// -refresh=true` for that component and continue iterating the rest in reverse order.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Capture warnings via the injected writer rather than redirecting os.Stderr
		// (the TUI spinner shares os.Stderr; pipe-based redirect deadlocks on Windows).
		var captured strings.Builder
		ctx := withWarnings(&captured)

		var destroyArgsByComponent = map[string][]string{}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			componentOf := func() string {
				if len(args) == 0 {
					return ""
				}
				// Normalize OS-native separators (Windows uses \) before matching.
				normalized := filepath.ToSlash(args[0])
				switch {
				case strings.Contains(normalized, "remote/path"):
					return "remote/path"
				case strings.Contains(normalized, "local/path"):
					return "local/path"
				}
				return ""
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				if componentOf() == "local/path" {
					return "", fmt.Errorf("mock refresh failure for local/path")
				}
				return "", nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgsByComponent[componentOf()] = append([]string(nil), args...)
				return "", nil
			}
			return "", nil
		}

		skipped, err := stack.DestroyAll(ctx, blueprint, false)
		warningOutput := captured.String()

		if err != nil {
			t.Fatalf("Expected DestroyAll to tolerate refresh failure on one component, got %v", err)
		}
		if len(skipped.Skipped) != 0 {
			t.Errorf("Expected no skipped components (state was non-empty), got %v", skipped.Skipped)
		}

		// local/path (refresh failed) must have used -refresh=true on destroy.
		localArgs, ok := destroyArgsByComponent["local/path"]
		if !ok {
			t.Fatal("Expected destroy to be called for local/path despite refresh failure")
		}
		hasRefreshTrue := false
		for _, a := range localArgs {
			if a == "-refresh=true" {
				hasRefreshTrue = true
			}
			if a == "-refresh=false" {
				t.Errorf("local/path: expected -refresh=true on refresh-fallback path, got -refresh=false in %v", localArgs)
			}
		}
		if !hasRefreshTrue {
			t.Errorf("local/path: expected destroy args to include -refresh=true, got %v", localArgs)
		}

		// remote/path (refresh succeeded) must have used -refresh=false on destroy.
		remoteArgs, ok := destroyArgsByComponent["remote/path"]
		if !ok {
			t.Fatal("Expected destroy to be called for remote/path")
		}
		hasRefreshFalse := false
		for _, a := range remoteArgs {
			if a == "-refresh=false" {
				hasRefreshFalse = true
			}
		}
		if !hasRefreshFalse {
			t.Errorf("remote/path: expected -refresh=false on happy path, got %v", remoteArgs)
		}

		// Warning must surface for the failed component only — silent fallback would hide
		// a recurring credential or connectivity issue across a bulk destroy.
		if !strings.Contains(warningOutput, "terraform refresh failed for local/path") {
			t.Errorf("Expected warning for failed component, got: %q", warningOutput)
		}
		if !strings.Contains(warningOutput, "mock refresh failure for local/path") {
			t.Errorf("Expected warning to include underlying refresh error, got: %q", warningOutput)
		}
		if strings.Contains(warningOutput, "terraform refresh failed for remote/path") {
			t.Errorf("No warning expected for remote/path (refresh succeeded), got: %q", warningOutput)
		}
	})

	t.Run("RefreshTimeoutAbortsWithoutDestroyFallback", func(t *testing.T) {
		// Given a component whose refresh times out. A timeout means the provider is
		// unreachable, not that the failure is transient, so destroy must abort this
		// component now instead of retrying refresh inside `terraform destroy -refresh=true`.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var sawDestroy bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "refresh" {
				return "", fmt.Errorf("%w after %v", shell.ErrCommandTimedOut, timeout)
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "destroy" {
				sawDestroy = true
			}
			return "", nil
		}

		// When DestroyAll runs
		_, err := stack.DestroyAll(context.Background(), blueprint, false)

		// Then it fails fast with an actionable error and never runs destroy
		if err == nil {
			t.Fatal("Expected DestroyAll to fail when refresh times out")
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "provider looks unreachable") {
			t.Errorf("Expected an actionable unreachable-provider error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "is reachable") {
			t.Errorf("Expected a reachability remediation, got %v", err)
		}
		if sawDestroy {
			t.Error("Expected terraform destroy not to run after a refresh timeout")
		}
	})

	t.Run("ContinueOnErrorCollectsPerComponentFailures", func(t *testing.T) {
		// Given a blueprint with two components where the first-processed component's
		// terraform destroy returns an error, continueOnError=true must record the
		// failure and move on rather than aborting the loop.
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{Source: "source1", Path: "first"},
			{Source: "source1", Path: "second"},
		}
		for _, p := range []string{"first", "second"} {
			dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", p)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
		}

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				if strings.Contains(filepath.ToSlash(args[0]), "/second") {
					return "boom", fmt.Errorf("mock destroy failure for second")
				}
				return "", nil
			}
			return "", nil
		}

		// When DestroyAll runs with continueOnError=true
		result, err := stack.DestroyAll(context.Background(), blueprint, true)

		// Then the failure is collected and the follow-up component still destroys
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb per-component failure, got %v", err)
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "second" {
			t.Errorf("Expected exactly one failure for \"second\", got %v", result.Failed)
		}
		if len(result.Destroyed) != 1 || result.Destroyed[0] != "first" {
			t.Errorf("Expected \"first\" to destroy successfully after \"second\" failed, got %v", result.Destroyed)
		}
	})

	t.Run("NoContinueAbortsOnFirstFailure", func(t *testing.T) {
		// Given the same two-component blueprint where the first-processed (reverse
		// order: "second") destroy fails every retry attempt, continueOnError=false
		// must abort once retries are exhausted and the follow-up component must
		// never be attempted.
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{Source: "source1", Path: "first"},
			{Source: "source1", Path: "second"},
		}
		for _, p := range []string{"first", "second"} {
			dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", p)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
		}

		var destroyAttempts []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				normalized := filepath.ToSlash(args[0])
				switch {
				case strings.Contains(normalized, "/second"):
					destroyAttempts = append(destroyAttempts, "second")
					return "boom", fmt.Errorf("mock destroy failure for second")
				case strings.Contains(normalized, "/first"):
					destroyAttempts = append(destroyAttempts, "first")
				}
			}
			return "", nil
		}

		// When DestroyAll runs with continueOnError=false
		_, err := stack.DestroyAll(context.Background(), blueprint, false)

		// Then every retry attempt targets "second" and the follow-up component
		// is never attempted
		if err == nil {
			t.Fatal("Expected destroy to abort once retries are exhausted")
		}
		if len(destroyAttempts) != stack.destroyRetryAttempts {
			t.Fatalf("Expected %d attempts against \"second\", got %v", stack.destroyRetryAttempts, destroyAttempts)
		}
		for _, attempt := range destroyAttempts {
			if attempt != "second" {
				t.Errorf("Expected only \"second\" to have been attempted before abort, got %v", destroyAttempts)
				break
			}
		}
	})

	t.Run("ContinueOnErrorAbsorbsSetupEnvironmentFailure", func(t *testing.T) {
		// Given a stack whose TerraformProvider.GetEnvVars fails for one
		// component (e.g. malformed provider alias, missing env var) and
		// succeeds for another. Before the fix, setupTerraformEnvironment
		// returned mid-loop with the error and the entire continueOnError
		// loop aborted, silently skipping all subsequent components.
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{Source: "source1", Path: "first"},
			{Source: "source1", Path: "broken"},
		}
		for _, p := range []string{"first", "broken"} {
			dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", p)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
		}

		// Inject a TerraformProvider whose GetEnvVars fails for "broken" only.
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			GetEnvVarsFunc: func(componentID string, interactive bool) (map[string]string, []string, *terraformRuntime.TerraformArgs, error) {
				if strings.Contains(componentID, "broken") {
					return nil, nil, nil, fmt.Errorf("mock setup failure for %s", componentID)
				}
				return map[string]string{}, nil, &terraformRuntime.TerraformArgs{}, nil
			},
		}

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}

		// When DestroyAll runs with continueOnError=true
		result, err := stack.DestroyAll(context.Background(), blueprint, true)

		// Then the setup failure is collected and the follow-up component still destroys
		if err != nil {
			t.Fatalf("Expected continueOnError to absorb setup failure, got %v", err)
		}
		if len(result.Failed) != 1 || result.Failed[0].ID != "broken" {
			t.Errorf("Expected exactly one failure for \"broken\", got %v", result.Failed)
		}
		if len(result.Destroyed) != 1 || result.Destroyed[0] != "first" {
			t.Errorf("Expected \"first\" to destroy successfully after \"broken\" failed setup, got %v", result.Destroyed)
		}
	})

}

func TestStack_Destroy(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims

		// Default `terraform show -json` to return state JSON containing one
		// resource AND a plan JSON containing an "update" action, so the full
		// flow runs: classifyDestroyAction sees non-empty state, then a plan
		// with updates, routes to PrepThenDestroy. Tests that want to verify
		// the Noop or DestroyOnly branches override this themselves.
		// Distinguish state-show vs plan-show by len(args): `show -json` is 3
		// args, `show -json <planpath>` is 4.
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying the local component by ID
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Destroy to return nil, got %v", err)
		}
	})

	t.Run("UsesDestroyIdleAbsoluteAndGracePeriod", func(t *testing.T) {
		// Given a stack whose terraform destroy exec is bounded by an idle timeout, an
		// absolute backstop, and a grace period
		stack, mocks := setup(t)
		var gotIdleTimeout, gotAbsoluteTimeout, gotGracePeriod time.Duration
		var sawDestroy bool
		mocks.Shell.ExecSilentWithEnvAndIdleTimeoutFunc = func(command string, env map[string]string, args []string, idleTimeout, absoluteTimeout, gracePeriod time.Duration) (string, error) {
			sawDestroy = true
			gotIdleTimeout = idleTimeout
			gotAbsoluteTimeout = absoluteTimeout
			gotGracePeriod = gracePeriod
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying the local component by ID
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then the destroy exec runs with the bounded idle timeout, absolute backstop, and
		// grace period, not the immediate-kill variant
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !sawDestroy {
			t.Fatal("Expected terraform destroy to run via ExecSilentWithEnvAndIdleTimeout")
		}
		if gotIdleTimeout != constants.DefaultTerraformDestroyIdleTimeout {
			t.Errorf("Expected idle timeout %v, got %v", constants.DefaultTerraformDestroyIdleTimeout, gotIdleTimeout)
		}
		if gotAbsoluteTimeout != constants.DefaultTerraformDestroyAbsoluteTimeout {
			t.Errorf("Expected absolute timeout %v, got %v", constants.DefaultTerraformDestroyAbsoluteTimeout, gotAbsoluteTimeout)
		}
		if gotGracePeriod != constants.DefaultTerraformDestroyGracePeriod {
			t.Errorf("Expected grace period %v, got %v", constants.DefaultTerraformDestroyGracePeriod, gotGracePeriod)
		}
	})

	t.Run("PassesNoColor", func(t *testing.T) {
		stack, mocks := setup(t)
		var destroyArgs []string
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			destroyArgs = args
			return "", nil
		}

		blueprint := createTestBlueprint()
		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Contains(destroyArgs, "-no-color") {
			t.Errorf("Expected destroy args to contain -no-color, got %v", destroyArgs)
		}
	})

	t.Run("UsesRefreshTimeout", func(t *testing.T) {
		// Given a stack whose terraform refresh exec is bounded by a timeout
		stack, mocks := setup(t)
		var gotTimeout time.Duration
		var sawRefresh bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "refresh" {
				sawRefresh = true
				gotTimeout = timeout
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying the component
		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the refresh exec runs with the bounded timeout: a component's own
		// kubernetes/helm provider can't hang the whole destroy dialing a broken cluster
		if !sawRefresh {
			t.Fatal("Expected terraform refresh to run via ExecSilentWithEnvAndTimeout")
		}
		if gotTimeout != constants.DefaultTerraformRefreshTimeout {
			t.Errorf("Expected timeout %v, got %v", constants.DefaultTerraformRefreshTimeout, gotTimeout)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When destroying with nil blueprint
		_, err := stack.Destroy(context.Background(), nil, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyComponentID", func(t *testing.T) {
		// Given a stack with an empty component ID
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying with an empty component ID
		_, err := stack.Destroy(context.Background(), blueprint, "")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "component ID not provided") {
			t.Errorf("Expected component ID error, got: %v", err)
		}
	})

	t.Run("ComponentNotFound", func(t *testing.T) {
		// Given a stack and a blueprint with known components
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying a component that does not exist in the blueprint
		_, err := stack.Destroy(context.Background(), blueprint, "does/not/exist")

		// Then an error should occur naming the missing component
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a stack whose Getwd returns an error
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("Expected error to contain %q, got %q", "error getting current directory", err.Error())
		}
	})

	t.Run("ErrorDirectoryDoesNotExist", func(t *testing.T) {
		// Given a stack whose Stat reports the component directory is missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then an error should occur mentioning directory
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Fatalf("Expected error to contain %q, got %q", "directory", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given a stack whose shell fails on terraform init
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		// Given a stack whose shell fails on terraform destroy. The single Destroy
		// method uses ExecSilentWithEnv inside a tui.WithProgress wrapper to mirror
		// the bulk DestroyAll loop body. The mock returns a recognizable terraform
		// diagnostic alongside the error to verify it is surfaced via presenter.Warn.
		stack, mocks := setup(t)
		var captured strings.Builder
		ctx := withWarnings(&captured)

		const tfDiagnostic = "Error: deleting EC2 Instance (i-abc123): InvalidInstanceID.NotFound"
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				return tfDiagnostic, fmt.Errorf("mock error running terraform destroy")
			}
			if command == "terraform" && len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(ctx, blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error running terraform destroy for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform destroy for", err.Error())
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, tfDiagnostic) {
			t.Errorf("Expected terraform diagnostic %q to be surfaced via presenter.Warn, got %q", tfDiagnostic, warningOutput)
		}
	})

	t.Run("RetriesTerraformDestroyThenSucceeds", func(t *testing.T) {
		// Given a terraform destroy that fails once then succeeds on retry
		stack, mocks := setup(t)
		var captured strings.Builder
		ctx := withWarnings(&captured)
		var destroyCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalls++
				if destroyCalls == 1 {
					return "boom", fmt.Errorf("mock transient destroy failure")
				}
				return "", nil
			}
			if command == "terraform" && len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		skipped, err := stack.Destroy(ctx, blueprint, "local/path")

		// Then the component destroys successfully after one retry, and the failed
		// first attempt is still visible via presenter.Warn rather than vanishing
		if err != nil {
			t.Fatalf("Expected the retry to absorb the transient failure, got %v", err)
		}
		if skipped {
			t.Error("Expected the component not to be reported as skipped")
		}
		if destroyCalls != 2 {
			t.Errorf("Expected terraform destroy to run twice, got %d", destroyCalls)
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, "attempt 1/") || !strings.Contains(warningOutput, "boom") {
			t.Errorf("Expected the failed first attempt to be logged via presenter.Warn, got %q", warningOutput)
		}
	})

	t.Run("StopsRetryingOnDestroyTimeout", func(t *testing.T) {
		// Given a terraform destroy that times out. A timeout means the provider is
		// unreachable rather than momentarily inconsistent, so it must not be retried.
		stack, mocks := setup(t)
		var destroyCalls int
		var sleepCalls int
		mocks.Shims.TimeSleep = func(time.Duration) { sleepCalls++ }
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalls++
				return "", fmt.Errorf("%w after %v", shell.ErrCommandTimedOut, timeout)
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then the timeout fails immediately, with no retry and no backoff wait
		if err == nil || !errors.Is(err, shell.ErrCommandTimedOut) {
			t.Fatalf("Expected an error wrapping ErrCommandTimedOut, got %v", err)
		}
		if destroyCalls != 1 {
			t.Errorf("Expected exactly one destroy attempt before giving up on timeout, got %d", destroyCalls)
		}
		if sleepCalls != 0 {
			t.Errorf("Expected no backoff wait after a timeout, got %d", sleepCalls)
		}
	})

	t.Run("PassesRefreshArgsToRefreshCommand", func(t *testing.T) {
		// Given a stack whose provider returns RefreshArgs containing a var-file flag
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			GetEnvVarsFunc: func(componentID string, interactive bool) (map[string]string, []string, *terraformRuntime.TerraformArgs, error) {
				return map[string]string{}, nil, &terraformRuntime.TerraformArgs{
					RefreshArgs: []string{"-var-file=secrets.tfvars"},
				}, nil
			},
		}
		var refreshArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshArgs = args
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then no error should occur and RefreshArgs should be forwarded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		found := false
		for _, arg := range refreshArgs {
			if arg == "-var-file=secrets.tfvars" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected refresh command to include -var-file=secrets.tfvars, got args: %v", refreshArgs)
		}
	})

	t.Run("SetsTFVarOperationToDestroy", func(t *testing.T) {
		// Given a stack and a blueprint with a local component. The single Destroy
		// method now uses ExecSilentWithEnv inside a tui.WithProgress wrapper to
		// match the bulk DestroyAll loop body, so destroy invocations and the
		// state-show JSON both flow through ExecSilentWithEnvFunc.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_, _ = stack.Destroy(context.Background(), blueprint, "local/path")

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsRefreshGatedDestroy", func(t *testing.T) {
		// Given a stack that records the ordered sequence of terraform subcommands with
		// their TF_VAR_operation; state JSON reports a resource so the idempotency
		// short-circuit does not fire.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		type step struct {
			subcommand string
			operation  string
		}
		var sequence []step
		record := func(env map[string]string, args []string) {
			if len(args) <= 1 {
				return
			}
			sequence = append(sequence, step{subcommand: args[1], operation: env["TF_VAR_operation"]})
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}

		// When destroying a single component
		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the sequence is the idempotent destroy flow end-to-end: init, show
		// (pre-refresh state JSON), refresh, show (post-refresh state JSON), destroy. The
		// pre-refresh show-json drives the empty-state skip; refresh reconciles state with
		// reality for partial-destroy reconciliation; the post-refresh show-json drives the
		// second skip check. No intermediate plan or apply runs; terraform destroy plans
		// internally. TF_VAR_operation=destroy is forwarded to commands that include
		// TF_VAR_* in their env (show, refresh, destroy); init does not forward TF_VAR_*
		// and relies on the process env.
		expected := []string{"init", "show", "refresh", "show", "destroy"}
		if len(sequence) < len(expected) {
			t.Fatalf("Expected at least %d terraform steps, got %d: %+v", len(expected), len(sequence), sequence)
		}
		for i, want := range expected {
			if sequence[i].subcommand != want {
				t.Errorf("step %d: subcommand = %q, want %q (full sequence: %+v)", i, sequence[i].subcommand, want, sequence)
			}
		}
		for i, s := range sequence[:len(expected)] {
			switch s.subcommand {
			case "refresh", "show", "destroy":
				if s.operation != "destroy" {
					t.Errorf("step %d (%s): TF_VAR_operation = %q, want %q", i, s.subcommand, s.operation, "destroy")
				}
			}
		}
	})

	t.Run("DetectsResourcesInChildModules", func(t *testing.T) {
		// Given state JSON where all resources live under root_module.child_modules
		// instead of root_module.resources — windsor's canonical shape, because every
		// windsor blueprint wraps its resources in a `module "main"` block. An earlier
		// version only checked root_module.resources and so collapsed every windsor
		// destroy into a no-op (the real-world bug that motivated this test). The fix
		// walks the module tree recursively in tfStateModule.hasResources.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				// State JSON with resources only in a child module — no root-level
				// resources at all. This is what windsor's terraform actually emits.
				return `{"values":{"root_module":{"child_modules":[{"address":"module.main","resources":[{"address":"module.main.aws_s3_bucket.this"}]}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed against child-module state, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected destroy to run when child-module state has resources — the root-only check would miss them and short-circuit to noop")
		}
	})

	t.Run("DetectsResourcesInDeeplyNestedModules", func(t *testing.T) {
		// Given state JSON where resources live two levels deep (module "main" → module
		// "nested") — the recursion must cover arbitrary depth, not just windsor's
		// canonical one-level convention, so blueprint compositions that nest modules
		// still classify correctly.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"child_modules":[{"address":"module.main","child_modules":[{"address":"module.main.module.nested","resources":[{"address":"module.main.module.nested.aws_s3_bucket.x"}]}]}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed against deeply-nested state, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected destroy to run when nested-module state has resources")
		}
	})

	t.Run("ShortCircuitsWhenStateJSONHasNoResources", func(t *testing.T) {
		// Given a component whose `terraform show -json` returns state with an empty
		// root_module.resources array — the component has already been destroyed or was
		// never applied, and there is nothing left to tear down. This is the idempotency
		// case: re-running destroy must not fail just because the previous destroy
		// already did the job.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		planCalled := false
		applyCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				// State JSON with an empty resources array — the whole point.
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 {
				switch args[1] {
				case "plan":
					planCalled = true
				case "apply":
					applyCalled = true
				}
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy against empty state to succeed (idempotent), got %v", err)
		}

		// Then the plan/apply/destroy steps are all skipped — classifyDestroyAction
		// returned Noop and we returned immediately.
		if planCalled {
			t.Error("Expected classification plan to be skipped when state is empty")
		}
		if applyCalled {
			t.Error("Expected prep-apply to be skipped when state is empty")
		}
		if destroyCalled {
			t.Error("Expected destroy to be skipped when state is empty")
		}
	})

	t.Run("TreatsMissingValuesNodeAsEmpty", func(t *testing.T) {
		// Given a component whose `terraform show -json` returns state JSON where the
		// top-level "values" field is absent — this is what terraform emits when there
		// is literally no state file at all (never-applied component). The JSON is valid
		// but sparse; the typed decode against tfState should collapse this into the
		// Noop path, not a parse error.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"format_version":"1.0"}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected never-applied state to be treated as empty, got %v", err)
		}
		if destroyCalled {
			t.Error("Expected destroy to be skipped when state values node is absent")
		}
	})

	t.Run("PassesRefreshFalseToDestroyToAvoidDoubleRefresh", func(t *testing.T) {
		// Given a normal destroy path (state non-empty). refreshComponentState just ran
		// before the destroy call, so terraform destroy's internal refresh would be
		// redundant — twice the provider API load and twice the "Refreshing state..."
		// output per component. We pass -refresh=false to skip terraform's internal
		// refresh; state is already current.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var destroyArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgs = args
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed, got %v", err)
		}
		if destroyArgs == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		found := false
		for _, a := range destroyArgs {
			if a == "-refresh=false" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected destroy invocation to include -refresh=false to skip the redundant internal refresh, got args: %v", destroyArgs)
		}
	})

	t.Run("NeverRunsApplyDuringDestroy", func(t *testing.T) {
		// Given a normal destroy path (state has resources) — apply must never run
		// during destroy. Previous designs ran a prep-apply; that was unsafe because
		// a regular plan after refresh can include "create" actions for resources
		// dropped from state, which prep-apply would recreate right before destroy.
		// Destroy must go straight from state-check to terraform destroy; no apply.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		applyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				applyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed, got %v", err)
		}
		if applyCalled {
			t.Error("Expected apply never to run during destroy — prep-apply is unsafe because it can recreate resources dropped by refresh")
		}
	})

	t.Run("RefreshFailureFallsThroughToDestroyWithRefreshTrue", func(t *testing.T) {
		// Given a component with non-empty state where terraform refresh fails (transient
		// network blip, credential rotation, provider API hiccup), refresh failure must not
		// make a live component undestroyable. The pre-refresh check confirmed state is
		// non-empty, so we know there is something to destroy. Destroy must fall through to
		// `terraform destroy -refresh=true` so terraform's own refresh has a second shot.
		// The post-refresh state-show must be skipped (its result would be from the same
		// pre-refresh snapshot). Persistent refresh problems will then surface from destroy
		// itself, which yields a more actionable error than surfacing refresh's. A stderr
		// warning must also fire so the operator can correlate a later destroy failure with
		// the upstream refresh hiccup — silent fallback would hide a recurring credential
		// or connectivity issue until destroy itself errored.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Capture warnings via the injected writer rather than redirecting os.Stderr;
		// the TUI spinner shares os.Stderr and a pipe-based redirect deadlocks on Windows
		// where the spinner goroutine cannot be reliably synchronized with w.Close().
		var captured strings.Builder
		ctx := withWarnings(&captured)

		showCalls := 0
		var destroyArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				showCalls++
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				return "", fmt.Errorf("mock error refreshing state")
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgs = append([]string(nil), args...)
				return "", nil
			}
			return "", nil
		}

		skipped, err := stack.Destroy(ctx, blueprint, "local/path")
		warningOutput := captured.String()

		if err != nil {
			t.Fatalf("Expected destroy to fall through despite refresh error, got %v", err)
		}
		if skipped {
			t.Error("Destroy must not report skipped when refresh fails on a non-empty-state component")
		}
		if showCalls != 1 {
			t.Errorf("Expected exactly one state-show call (pre-refresh only); post-refresh check must be skipped on refresh failure, got %d show calls", showCalls)
		}
		if destroyArgs == nil {
			t.Fatal("Expected terraform destroy to be called when refresh fails on non-empty state")
		}
		hasRefreshTrue := false
		for _, a := range destroyArgs {
			if a == "-refresh=true" {
				hasRefreshTrue = true
			}
			if a == "-refresh=false" {
				t.Errorf("Expected destroy to use -refresh=true on refresh fallback path, got -refresh=false in args %v", destroyArgs)
			}
		}
		if !hasRefreshTrue {
			t.Errorf("Expected destroy args to include -refresh=true, got %v", destroyArgs)
		}
		if !strings.Contains(warningOutput, "terraform refresh failed for local/path") {
			t.Errorf("Expected warning naming the component, got: %q", warningOutput)
		}
		if !strings.Contains(warningOutput, "mock error refreshing state") {
			t.Errorf("Expected warning to include underlying refresh error for diagnostics, got: %q", warningOutput)
		}
	})

	t.Run("RefreshTimeoutAbortsWithoutDestroyFallback", func(t *testing.T) {
		// Given a component whose refresh times out. A timeout means the provider is
		// unreachable, not that the failure is transient, so destroy must abort now
		// instead of retrying refresh inside `terraform destroy -refresh=true`.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var sawDestroy bool
		mocks.Shell.ExecSilentWithEnvAndTimeoutFunc = func(command string, env map[string]string, args []string, timeout time.Duration) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "refresh" {
				return "", fmt.Errorf("%w after %v", shell.ErrCommandTimedOut, timeout)
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "destroy" {
				sawDestroy = true
			}
			return "", nil
		}

		// When destroying the component
		_, err := stack.Destroy(context.Background(), blueprint, "local/path")

		// Then it fails fast with an actionable error and never runs destroy
		if err == nil {
			t.Fatal("Expected Destroy to fail when refresh times out")
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "provider looks unreachable") {
			t.Errorf("Expected an actionable unreachable-provider error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "is reachable") {
			t.Errorf("Expected a reachability remediation, got %v", err)
		}
		if sawDestroy {
			t.Error("Expected terraform destroy not to run after a refresh timeout")
		}
	})

	t.Run("SkipsRefreshAndDestroyOnEmptyPreRefreshState", func(t *testing.T) {
		// Given state is already empty going into destroy — the bug case from the field
		// where an upstream component (e.g. a VPC) was destroyed first, then a downstream
		// module's refresh tried to read its `data "aws_vpc"` and failed because the cloud
		// object was gone. With the pre-refresh empty-state check, the entire flow
		// (refresh + destroy) must be skipped: refresh adds no signal when state is empty
		// going in (it can only drop resources, not add), and skipping it dodges any data
		// source that depends on already-torn-down infra. Destroy must report skipped=true.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		refreshCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				// Empty state — no resources at any module level.
				return `{"values":{"root_module":{}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshCalled = true
				return "", fmt.Errorf("refresh must not run on empty pre-refresh state")
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		skipped, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err != nil {
			t.Fatalf("Expected no error on empty-state skip, got %v", err)
		}
		if !skipped {
			t.Error("Destroy must report skipped=true when state is empty going in")
		}
		if refreshCalled {
			t.Error("refresh must not run when pre-refresh state is empty")
		}
		if destroyCalled {
			t.Error("destroy must not run when pre-refresh state is empty")
		}
	})

	t.Run("SurfacesStateShowJSONError", func(t *testing.T) {
		// Given a component where `terraform show -json` fails reading state — e.g. a
		// corrupted state file or a backend that rejects the call. Silently treating
		// this as "empty" would make destroy skip a component whose state is actually
		// intact, leaving resources orphaned.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return "", fmt.Errorf("Error: Failed to load state: corrupted state file")
			}
			return "", nil
		}

		_, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err == nil {
			t.Fatal("Expected state-show error to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "error reading terraform state JSON") {
			t.Errorf("Expected state-read error text, got %q", err.Error())
		}
	})

	t.Run("SurfacesMalformedStateJSON", func(t *testing.T) {
		// Given state JSON that terraform produced but which doesn't decode (garbage
		// from a broken plugin, truncated stream, etc.) — the parse error must surface
		// rather than collapsing into the Noop path. Same rationale as
		// SurfacesStateShowJSONError: a parse failure could otherwise mask real state.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `not valid json {`, nil
			}
			return "", nil
		}

		_, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err == nil {
			t.Fatal("Expected parse error to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "error parsing terraform state JSON") {
			t.Errorf("Expected state-parse error text, got %q", err.Error())
		}
	})

	t.Run("HandlesPartialDestructionViaRefreshReconciliation", func(t *testing.T) {
		// Given a scenario where some resources have been manually deleted between the
		// last apply and this destroy — the canonical idempotency scenario this flow
		// must handle. The pre-refresh check sees the pre-deletion state (non-empty,
		// so we proceed), refresh drops the missing resources, and the post-refresh
		// check sees only the survivors. The flow must then destroy those survivors,
		// NOT recreate the missing ones — no apply runs. The critical properties are:
		// refresh ran between the two state reads, destroy ran against the survivors,
		// and apply did not run.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Both state reads return a non-empty view: pre-refresh shows the original 5
		// (test only cares that it is non-empty so the pre-check does not skip), post-
		// refresh shows the 3 survivors. Mock returns the same payload to both — fine
		// because the property under test is "refresh runs and destroy runs" not the
		// exact pre/post payload differential.
		stateJSON := `{"values":{"root_module":{"resources":[
			{"address":"aws_s3_bucket.surviving_a"},
			{"address":"aws_rds_cluster.surviving_b"},
			{"address":"aws_ecr_repository.surviving_c"}
		]}}}`

		refreshCalled := false
		applyCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshCalled = true
				return "", nil
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return stateJSON, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				applyCalled = true
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected partial-destruction destroy to succeed, got %v", err)
		}

		// Then refresh ran first (reconciled state), the state check saw survivors, and
		// destroy tore them down. Apply must NOT have run — running apply against the
		// post-refresh state would try to reconcile config→state and recreate the
		// resources refresh just dropped, which is the exact bug the no-prep design
		// prevents.
		if !refreshCalled {
			t.Error("Expected refresh to run before the state check")
		}
		if applyCalled {
			t.Error("Expected apply never to run — it would recreate resources dropped by refresh")
		}
		if !destroyCalled {
			t.Error("Expected destroy to run against survivors")
		}
	})

	t.Run("ClearsBackendPointerAfterSuccessfulDestroy", func(t *testing.T) {
		// After destroy completes, the backend metadata file inside TF_DATA_DIR
		// must be removed so the next init doesn't try to talk to a remote
		// backend the destroy may have just torn down. Only that one file —
		// providers, modules, and lockfile stay so the next init is fast.
		stack, mocks := setup(t)
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}
		blueprint := createTestBlueprint()

		_, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		var pointerRemoved bool
		for _, p := range removed {
			if strings.HasSuffix(p, "/terraform.tfstate") {
				pointerRemoved = true
				break
			}
		}
		if !pointerRemoved {
			t.Errorf("Expected the backend pointer (terraform.tfstate inside TF_DATA_DIR) to be removed, got removals: %v", removed)
		}
	})

	t.Run("ClearsBackendPointerAfterEmptyStateSkip", func(t *testing.T) {
		// When pre-refresh state is already empty, destroy is skipped — but the
		// component still "completed destroy" semantically (nothing to do).
		// The pointer must still be cleared so re-runs don't trip over a stale
		// backend reference.
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}
		blueprint := createTestBlueprint()

		skipped, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !skipped {
			t.Fatal("Expected skipped=true on empty state")
		}

		var pointerRemoved bool
		for _, p := range removed {
			if strings.HasSuffix(p, "/terraform.tfstate") {
				pointerRemoved = true
				break
			}
		}
		if !pointerRemoved {
			t.Errorf("Expected backend pointer cleanup even on empty-state skip, got removals: %v", removed)
		}
	})

	t.Run("DoesNotClearBackendPointerWhenDestroyFails", func(t *testing.T) {
		// When destroy fails, the operator may want to retry — we must not
		// strip the pointer file out from under them. Cleanup only fires after
		// a successful (or no-op) destroy completes.
		stack, mocks := setup(t)
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				return "", fmt.Errorf("AccessDenied: not authorized")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		_, err := stack.Destroy(context.Background(), blueprint, "local/path")
		if err == nil {
			t.Fatal("Expected destroy to fail")
		}

		for _, p := range removed {
			if strings.HasSuffix(p, "/terraform.tfstate") {
				t.Errorf("Backend pointer must not be cleared on destroy failure, got removals: %v", removed)
			}
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestStack_destroyBehaviorDrift(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	component := &blueprintv1alpha1.TerraformComponent{Path: "database/gcp-cloudsql", FullPath: "/tmp/database"}
	sharedPlanFile := ".windsor/contexts/local/.terraform/database/terraform.tfplan"
	terraformArgs := &envvars.TerraformArgs{PlanArgs: []string{fmt.Sprintf("-out=%s", sharedPlanFile), "-var-file=terraform.tfvars.json"}}

	t.Run("ReturnsDriftFromItsOwnScratchPlanFile", func(t *testing.T) {
		// Given a regular plan that succeeds, then a show -json of that plan file
		// reporting drift on a curated attribute
		stack, mocks := setup(t)
		var sawRefreshFalse, sawVarFile bool
		var planOutArg, showedPlanFile string
		var removedPath string
		stack.shims.Remove = func(path string) error {
			removedPath = path
			return nil
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 3 && args[1] == "show" && args[2] == "-json" {
				showedPlanFile = args[3]
				return `{"resource_changes":[{"address":"aws_db_instance.primary","change":{"actions":["update"],"before":{"deletion_protection":true},"after":{"deletion_protection":false}}}]}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				sawRefreshFalse = slices.Contains(args, "-refresh=false")
				sawVarFile = slices.Contains(args, "-var-file=terraform.tfvars.json")
				for _, a := range args {
					if v, ok := strings.CutPrefix(a, "-out="); ok {
						planOutArg = v
					}
				}
				return "", nil
			}
			return "", nil
		}

		drifted := stack.destroyBehaviorDrift(component, map[string]string{}, nil, terraformArgs)

		if len(drifted) != 1 || drifted[0].Attribute != "deletion_protection" {
			t.Fatalf("expected 1 deletion_protection drift, got %#v", drifted)
		}
		if !sawRefreshFalse {
			t.Error("expected the drift-check plan to run with -refresh=false")
		}
		if !sawVarFile {
			t.Error("expected the drift-check plan to carry through the component's var-file args")
		}
		// The regular plan must never write to the shared apply-mode plan file: that
		// file is what `windsor env`'s TF_CLI_ARGS_apply points a manually-run
		// `terraform apply` at, so clobbering it here would leave a stale,
		// unreviewed plan for a later bare apply to silently consume (cli#3328).
		wantDriftFile := ".windsor/contexts/local/.terraform/database/windsor-destroy-drift.tfplan"
		if planOutArg != wantDriftFile {
			t.Errorf("expected -out=%q, got %q", wantDriftFile, planOutArg)
		}
		if planOutArg == sharedPlanFile {
			t.Fatal("drift check must not write to the shared apply-mode plan file")
		}
		if showedPlanFile != wantDriftFile {
			t.Errorf("expected show -json against the scratch plan file, got %q", showedPlanFile)
		}
		if removedPath != wantDriftFile {
			t.Errorf("expected the scratch plan file to be removed afterward, got %q", removedPath)
		}
	})

	t.Run("ReturnsNilWhenThePlanCallFails", func(t *testing.T) {
		// Given the regular plan itself fails
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("exit status 1")
			}
			return "", nil
		}

		// Then drift detection degrades to nil rather than propagating an error
		if drifted := stack.destroyBehaviorDrift(component, map[string]string{}, nil, terraformArgs); drifted != nil {
			t.Errorf("expected nil on plan failure, got %#v", drifted)
		}
	})

	t.Run("ReturnsNilWhenTheShowCallFails", func(t *testing.T) {
		// Given the plan succeeds but reading it back fails
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return "", fmt.Errorf("no such file")
			}
			return "", nil
		}

		if drifted := stack.destroyBehaviorDrift(component, map[string]string{}, nil, terraformArgs); drifted != nil {
			t.Errorf("expected nil when show fails, got %#v", drifted)
		}
	})

	t.Run("ReturnsNilWhenNoPlanFileIsConfigured", func(t *testing.T) {
		// Given terraformArgs.PlanArgs carries no -out=, so there is nothing to read back
		stack, mocks := setup(t)
		called := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			called = true
			return "", nil
		}

		if drifted := stack.destroyBehaviorDrift(component, map[string]string{}, nil, &envvars.TerraformArgs{}); drifted != nil {
			t.Errorf("expected nil, got %#v", drifted)
		}
		if called {
			t.Error("expected no terraform invocation when no plan file is configured")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestParseDestroyBehaviorDrift(t *testing.T) {
	t.Run("ReportsACuratedAttributeThatDisagreesWithState", func(t *testing.T) {
		// Given a plan JSON showing deletion_policy changing on an update action
		output := `{"resource_changes":[{"address":"module.main.google_service_networking_connection.cloudsql","change":{"actions":["update"],"before":{"deletion_policy":"DELETE"},"after":{"deletion_policy":"REMOVE_PEERING"}}}]}`

		// When parsed
		drifted := parseDestroyBehaviorDrift(output)

		// Then it reports the address (module.main. stripped), attribute, and both values
		if len(drifted) != 1 {
			t.Fatalf("expected 1 drift, got %#v", drifted)
		}
		want := DestroyAttributeDrift{
			Address:   "google_service_networking_connection.cloudsql",
			Attribute: "deletion_policy",
			State:     `"DELETE"`,
			Config:    `"REMOVE_PEERING"`,
		}
		if drifted[0] != want {
			t.Errorf("expected %#v, got %#v", want, drifted[0])
		}
	})

	t.Run("IgnoresAttributesOutsideTheCuratedList", func(t *testing.T) {
		// Given an update that changes an attribute we don't track
		output := `{"resource_changes":[{"address":"aws_s3_bucket.logs","change":{"actions":["update"],"before":{"tags":{"env":"old"}},"after":{"tags":{"env":"new"}}}}]}`

		drifted := parseDestroyBehaviorDrift(output)

		if len(drifted) != 0 {
			t.Errorf("expected no drift for an uncurated attribute, got %#v", drifted)
		}
	})

	t.Run("IgnoresResourcesNotBeingUpdated", func(t *testing.T) {
		// Given a create action carrying a curated attribute (before is absent, not drifted)
		output := `{"resource_changes":[{"address":"aws_db_instance.new","change":{"actions":["create"],"before":null,"after":{"deletion_protection":true}}}]}`

		drifted := parseDestroyBehaviorDrift(output)

		if len(drifted) != 0 {
			t.Errorf("expected no drift for a create action, got %#v", drifted)
		}
	})

	t.Run("IgnoresAnUnchangedCuratedAttribute", func(t *testing.T) {
		// Given an update where the curated attribute itself did not change
		output := `{"resource_changes":[{"address":"aws_db_instance.primary","change":{"actions":["update"],"before":{"deletion_protection":true,"other":"a"},"after":{"deletion_protection":true,"other":"b"}}}]}`

		drifted := parseDestroyBehaviorDrift(output)

		if len(drifted) != 0 {
			t.Errorf("expected no drift when the curated attribute is unchanged, got %#v", drifted)
		}
	})

	t.Run("ReturnsNilForMalformedInput", func(t *testing.T) {
		if drifted := parseDestroyBehaviorDrift("not json"); drifted != nil {
			t.Errorf("expected nil for malformed input, got %#v", drifted)
		}
		if drifted := parseDestroyBehaviorDrift(""); drifted != nil {
			t.Errorf("expected nil for empty input, got %#v", drifted)
		}
	})
}

func TestFilterDriftToDestroyed(t *testing.T) {
	t.Run("KeepsOnlyDriftOnResourcesBeingDeleted", func(t *testing.T) {
		drifted := []DestroyAttributeDrift{
			{Address: "aws_db_instance.primary", Attribute: "deletion_protection"},
			{Address: "aws_s3_bucket.untouched", Attribute: "force_destroy"},
		}
		resources := []ResourceChange{
			{Address: "aws_db_instance.primary", Action: ActionDelete},
			{Address: "aws_s3_bucket.untouched", Action: ActionUpdate},
		}

		filtered := filterDriftToDestroyed(drifted, resources)

		if len(filtered) != 1 || filtered[0].Address != "aws_db_instance.primary" {
			t.Errorf("expected only the destroyed resource's drift, got %#v", filtered)
		}
	})

	t.Run("ReturnsNilWhenNothingMatches", func(t *testing.T) {
		drifted := []DestroyAttributeDrift{{Address: "aws_db_instance.orphan"}}
		if filtered := filterDriftToDestroyed(drifted, nil); filtered != nil {
			t.Errorf("expected nil, got %#v", filtered)
		}
	})
}

func TestExtractPreventDestroyAddresses(t *testing.T) {
	t.Run("ReturnsNilForEmptyOrNonDiagnosticOutput", func(t *testing.T) {
		// Given an event stream with only planned_change and change_summary events
		output := strings.Join([]string{
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_s3_bucket.x"},"action":"delete"}}`,
			`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}`,
			``,
		}, "\n")

		// When extracting prevent_destroy addresses
		got := extractPreventDestroyAddresses(output)

		// Then none are reported
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}

		// And empty output produces nil too
		if extractPreventDestroyAddresses("") != nil {
			t.Error("expected nil for empty output")
		}
	})

	t.Run("ExtractsAddressesAndStripsModuleMainPrefix", func(t *testing.T) {
		// Given a stream with two prevent_destroy diagnostics and other noise
		output := strings.Join([]string{
			`Initialising backend...`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","address":"module.main.aws_db_instance.production"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_s3_bucket.audit"},"action":"delete"}}`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","address":"module.main.aws_s3_bucket.audit"}}`,
			`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}`,
			`trailing junk`,
			``,
		}, "\n")

		// When extracting
		got := extractPreventDestroyAddresses(output)

		// Then both addresses are reported in order, module.main. prefix stripped
		want := []string{"aws_db_instance.production", "aws_s3_bucket.audit"}
		if len(got) != len(want) {
			t.Fatalf("expected %d addresses, got %d: %#v", len(want), len(got), got)
		}
		for i, addr := range got {
			if addr != want[i] {
				t.Errorf("addr[%d]: expected %q, got %q", i, want[i], addr)
			}
		}
	})

	t.Run("IgnoresNonPreventDestroyDiagnostics", func(t *testing.T) {
		// Given diagnostics that aren't the prevent_destroy signal — wrong severity,
		// wrong summary, or wrong type entirely
		output := strings.Join([]string{
			`{"type":"diagnostic","diagnostic":{"severity":"warning","summary":"Instance cannot be destroyed","address":"module.main.aws_db_instance.warn_only"}}`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Provider produced inconsistent result","address":"module.main.aws_iam_role.x"}}`,
			`{"type":"some_other_event","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","address":"module.main.aws_s3_bucket.y"}}`,
			``,
		}, "\n")

		// When extracting
		got := extractPreventDestroyAddresses(output)

		// Then none qualify
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("FallsBackToDetailWhenAddressFieldIsMissing", func(t *testing.T) {
		// Given the diagnostic shape that real terraform emits — no `address` field
		// but `detail` opens with "Resource <addr> has lifecycle.prevent_destroy set"
		output := strings.Join([]string{
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","detail":"Resource module.main.null_resource.guarded has lifecycle.prevent_destroy set, but the plan calls for this resource to be destroyed."}}`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","detail":"Resource aws_db_instance.primary has lifecycle.prevent_destroy set."}}`,
			``,
		}, "\n")

		// When extracting
		got := extractPreventDestroyAddresses(output)

		// Then both addresses are parsed out of the detail
		want := []string{"null_resource.guarded", "aws_db_instance.primary"}
		if len(got) != len(want) {
			t.Fatalf("expected %d addresses, got %d: %#v", len(want), len(got), got)
		}
		for i, addr := range got {
			if addr != want[i] {
				t.Errorf("addr[%d]: expected %q, got %q", i, want[i], addr)
			}
		}
	})

	t.Run("SkipsDiagnosticsWithNoExtractableAddress", func(t *testing.T) {
		// Given a malformed diagnostic — no address, no parseable detail
		output := strings.Join([]string{
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","detail":"some unrelated message"}}`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed"}}`,
			``,
		}, "\n")

		// When extracting
		got := extractPreventDestroyAddresses(output)

		// Then nothing is reported — silent skip beats reporting empty strings
		if got != nil {
			t.Errorf("expected nil for unparseable diagnostics, got %#v", got)
		}
	})
}
