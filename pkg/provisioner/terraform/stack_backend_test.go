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
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformStack_initComponent_KubernetesNamespace(t *testing.T) {
	// setup returns a stack on the kubernetes backend whose backend pointer holds pointerJSON,
	// and a func that returns the args of every terraform init it ran.
	setup := func(t *testing.T, pointerJSON string) (*TerraformStack, func() [][]string) {
		t.Helper()
		mocks := setupWindsorStackMocks(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    terraform:
      backend:
        type: kubernetes`})
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
	args := &envvars.TerraformArgs{
		InitArgs:            []string{"-backend=true", "-backend-config=secret_suffix=dns", "-backend-config=namespace=system-terraform"},
		KubernetesNamespace: "system-terraform",
	}
	const legacyPointer = `{"backend":{"type":"kubernetes","config":{"namespace":null,"secret_suffix":"dns"}}}`

	t.Run("MigratesStateWhenThePointerNamesAnotherNamespace", func(t *testing.T) {
		// Given a pointer that keeps the state in default
		stack, inits := setup(t, legacyPointer)

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then terraform copies the state into the configured namespace
		got := inits()
		if len(got) != 1 {
			t.Fatalf("Expected one init, got %v", got)
		}
		for _, flag := range []string{"-migrate-state", "-force-copy", "-backend-config=namespace=system-terraform"} {
			if !slices.Contains(got[0], flag) {
				t.Errorf("Expected init args to contain %q, got %v", flag, got[0])
			}
		}
	})

	t.Run("ReportsTheMoveUnderTheCallersOperation", func(t *testing.T) {
		// Given a pointer that keeps the state in default and an operation in progress
		stack, _ := setup(t, legacyPointer)
		rec := &eventRecorder{}
		ctx, done := presenter.Begin(presenter.NewContext(context.Background(), rec), "terraform:dns", "Applying dns")
		defer done(nil)

		// When an apply path inits the component
		if err := stack.runTerraformInit(ctx, component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the move message nests under that operation
		var opID, parentID string
		for _, e := range rec.events {
			if e.Kind == presenter.KindApplying {
				opID = e.ID
			}
			if e.Kind == presenter.KindMessage && strings.Contains(e.Message, "Moved terraform state of dns from namespace default to system-terraform") {
				parentID = e.ParentID
			}
		}
		if parentID == "" || parentID != opID {
			t.Errorf("Expected the move message under operation %q, got parent %q in %+v", opID, parentID, rec.events)
		}
	})

	t.Run("PlanReadsThePointerNamespaceWithoutMigrating", func(t *testing.T) {
		// Given a pointer that keeps the state in default
		stack, inits := setup(t, legacyPointer)

		// When a plan inits the component
		if err := stack.runTerraformInitReadOnly(component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then terraform reconfigures against default and does not migrate
		got := strings.Join(inits()[0], " ")
		if !strings.Contains(got, "-reconfigure") || !strings.Contains(got, "-backend-config=namespace=default") {
			t.Errorf("Expected -reconfigure against namespace default, got %s", got)
		}
		if strings.Contains(got, "-migrate-state") || strings.Contains(got, "namespace=system-terraform") {
			t.Errorf("Expected no migration and no configured namespace, got %s", got)
		}
	})

	t.Run("ApplyAfterPlanStillMigrates", func(t *testing.T) {
		// Given a plan already ran init against the old namespace in this process
		stack, inits := setup(t, legacyPointer)
		if err := stack.runTerraformInitReadOnly(component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// When an apply path inits the same component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the init cache does not skip the migration
		got := inits()
		if len(got) != 2 || !slices.Contains(got[1], "-migrate-state") {
			t.Errorf("Expected a second init with -migrate-state, got %v", got)
		}
	})

	t.Run("RunsAPlainInitWhenTheNamespaceMatches", func(t *testing.T) {
		// Given a pointer in the configured namespace
		stack, inits := setup(t, `{"backend":{"type":"kubernetes","config":{"namespace":"system-terraform"}}}`)

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no migration or reconfigure flag is added
		got := strings.Join(inits()[0], " ")
		if strings.Contains(got, "-migrate-state") || strings.Contains(got, "-reconfigure") {
			t.Errorf("Expected a plain init, got %s", got)
		}
	})

	t.Run("RunsAPlainInitWithoutAPointer", func(t *testing.T) {
		// Given no backend pointer
		stack, inits := setup(t, "")

		// When an apply path inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no migration flag is added
		if got := strings.Join(inits()[0], " "); strings.Contains(got, "-migrate-state") {
			t.Errorf("Expected a plain init, got %s", got)
		}
	})

	t.Run("RunsAPlainInitWhenTheConfigSetsNoNamespace", func(t *testing.T) {
		// Given a pointer namespace that backend.tfvars set, and no namespace in the config
		stack, inits := setup(t, `{"backend":{"type":"kubernetes","config":{"namespace":"tfstate"}}}`)
		unset := &envvars.TerraformArgs{InitArgs: []string{"-backend=true", "-backend-config=secret_suffix=dns"}}

		// When an apply path and a plan init the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, unset, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if err := stack.runTerraformInitReadOnly(component, vars, nil, unset, defaultInitFlags...); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then neither adds a migration or reconfigure flag
		for _, args := range inits() {
			if got := strings.Join(args, " "); strings.Contains(got, "-migrate-state") || strings.Contains(got, "-reconfigure") {
				t.Errorf("Expected a plain init, got %s", got)
			}
		}
	})

	t.Run("IgnoresAPointerForAnotherBackendType", func(t *testing.T) {
		// Given a local pointer, as after the bootstrap pivot
		stack, inits := setup(t, `{"backend":{"type":"local","config":{"path":"/tmp/state"}}}`)

		// When a state migration inits the component
		if err := stack.runTerraformInit(context.Background(), component, vars, nil, args, "-migrate-state", "-force-copy"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the caller's flags are passed once and unchanged
		got := inits()[0]
		count := 0
		for _, arg := range got {
			if arg == "-migrate-state" {
				count++
			}
		}
		if count != 1 || slices.Contains(got, "-reconfigure") {
			t.Errorf("Expected the caller's flags only, got %v", got)
		}
	})
}
