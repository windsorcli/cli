package terraform

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	terraformRuntime "github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformStack_initComponent_AutoPrefix(t *testing.T) {
	// setup returns a stack on the s3 backend whose backend pointer holds pointerJSON, and a func
	// that returns the args of every terraform init it ran.
	setup := func(t *testing.T, pointerJSON string) (*TerraformStack, func() [][]string) {
		t.Helper()
		mocks := setupWindsorStackMocks(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    terraform:
      backend:
        type: s3`})
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if pointerJSON == "" {
				return nil, os.ErrNotExist
			}
			return []byte(pointerJSON), nil
		}
		var inits [][]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "init" {
				inits = append(inits, args)
			}
			return "", nil
		}
		return stack, func() [][]string { return inits }
	}
	component := &blueprintv1alpha1.TerraformComponent{Path: "dns", FullPath: "/project/terraform/dns"}
	vars := map[string]string{"TF_DATA_DIR": "/tmp/scratch/.terraform/dns"}
	scopedArgs := func() *envvars.TerraformArgs {
		return &envvars.TerraformArgs{
			InitArgs:       []string{"-backend=true", "-backend-config=key=staging-w8k2x9q/dns/terraform.tfstate"},
			LegacyKeyField: "key",
			LegacyKeyValue: "dns/terraform.tfstate",
		}
	}
	const legacyPointer = `{"backend":{"type":"s3","config":{"bucket":"b","key":"dns/terraform.tfstate"}}}`

	t.Run("MigratesStateWhenThePointerHoldsTheLegacyKey", func(t *testing.T) {
		// Given a context that turned auto_prefix on and a pointer at the legacy key
		stack, inits := setup(t, legacyPointer)

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then terraform copies the state to the scoped key
		got := inits()
		if len(got) != 1 {
			t.Fatalf("expected one init, got %v", got)
		}
		for _, flag := range []string{"-migrate-state", "-force-copy", "-backend-config=key=staging-w8k2x9q/dns/terraform.tfstate"} {
			if !slices.Contains(got[0], flag) {
				t.Errorf("expected init args to contain %q, got %v", flag, got[0])
			}
		}
	})

	t.Run("PlanReadsTheLegacyKeyWithoutMigrating", func(t *testing.T) {
		// Given a pointer at the legacy key
		stack, inits := setup(t, legacyPointer)

		// When a plan inits the component
		if err := stack.runTerraformInitReadOnly(component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then terraform reconfigures against the legacy key and writes nothing
		got := strings.Join(inits()[0], " ")
		if !strings.Contains(got, "-reconfigure") || !strings.Contains(got, "-backend-config=key=dns/terraform.tfstate") {
			t.Errorf("expected -reconfigure against the legacy key, got %s", got)
		}
		if strings.Contains(got, "-migrate-state") || strings.Contains(got, "staging-w8k2x9q") {
			t.Errorf("expected no migration and no scoped key, got %s", got)
		}
	})

	t.Run("ApplyAfterPlanStillMigrates", func(t *testing.T) {
		// Given a plan already ran init against the legacy key in this process
		stack, inits := setup(t, legacyPointer)
		if err := stack.runTerraformInitReadOnly(component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When an apply path inits the same component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the init cache does not skip the migration
		got := inits()
		if len(got) != 2 || !slices.Contains(got[1], "-migrate-state") {
			t.Errorf("expected a second init with -migrate-state, got %v", got)
		}
	})

	t.Run("DoesNotMigrateWhenThePointerHoldsAnotherKey", func(t *testing.T) {
		// Given a pointer at a key that neither the legacy nor the scoped layout produces
		stack, inits := setup(t, `{"backend":{"type":"s3","config":{"key":"other/dns/terraform.tfstate"}}}`)

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the init is plain and terraform decides
		if got := strings.Join(inits()[0], " "); strings.Contains(got, "-migrate-state") {
			t.Errorf("expected no migration, got %s", got)
		}
	})

	t.Run("DoesNotMigrateWhenThePointerHoldsTheScopedKey", func(t *testing.T) {
		// Given a pointer that already holds the scoped key
		stack, inits := setup(t, `{"backend":{"type":"s3","config":{"key":"staging-w8k2x9q/dns/terraform.tfstate"}}}`)

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the init is plain
		if got := strings.Join(inits()[0], " "); strings.Contains(got, "-migrate-state") {
			t.Errorf("expected no migration, got %s", got)
		}
	})

	t.Run("DoesNotMigrateWhenAutoPrefixIsOff", func(t *testing.T) {
		// Given a context without auto_prefix and a pointer at the legacy key
		stack, inits := setup(t, legacyPointer)
		args := &envvars.TerraformArgs{InitArgs: []string{"-backend=true", "-backend-config=key=dns/terraform.tfstate"}}

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the init is plain and the key is unchanged
		got := strings.Join(inits()[0], " ")
		if strings.Contains(got, "-migrate-state") || !strings.Contains(got, "-backend-config=key=dns/terraform.tfstate") {
			t.Errorf("expected a plain init at the legacy key, got %s", got)
		}
	})

	t.Run("DoesNotMigrateWithoutAPointer", func(t *testing.T) {
		// Given no backend pointer, such as on a fresh checkout
		stack, inits := setup(t, "")

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, scopedArgs(), defaultInitFlags...); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the init is plain
		if got := strings.Join(inits()[0], " "); strings.Contains(got, "-migrate-state") {
			t.Errorf("expected no migration, got %s", got)
		}
	})
}

func TestTerraformStack_warnSharedBackend(t *testing.T) {
	t.Run("WarnsOncePerProcess", func(t *testing.T) {
		// Given a provider that reports a context sharing the backend
		mocks := setupWindsorStackMocks(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    terraform:
      backend:
        type: s3`})
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		calls := 0
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			SharedBackendContextsFunc: func() []string {
				calls++
				return []string{"contexts a and b share keys"}
			},
		}
		rec := &eventRecorder{}
		ctx := presenter.NewContext(context.Background(), rec)

		// When the stack warns twice
		stack.warnSharedBackend(ctx)
		stack.warnSharedBackend(ctx)

		// Then the provider is asked once and one warning is emitted
		if calls != 1 {
			t.Errorf("expected one provider call, got %d", calls)
		}
		warnings := 0
		for _, e := range rec.events {
			if strings.Contains(e.Message, "share keys") {
				warnings++
			}
		}
		if warnings != 1 {
			t.Errorf("expected one warning, got %d in %+v", warnings, rec.events)
		}
	})
}
