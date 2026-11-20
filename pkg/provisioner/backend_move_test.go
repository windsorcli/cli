package provisioner_test

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/provisioner"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// moveFixture is a fake terraform stack that keeps a backend pointer, a local state, and a remote
// state for each component. MigrateComponentState and MigrateState copy the state toward the
// configured backend and update the pointer. Each state call is recorded in ops.
type moveFixture struct {
	prov     *provisioner.Provisioner
	mocks    *ProvisionerTestMocks
	stack    *terraforminfra.MockStack
	backend  string
	pointers map[string]string
	local    map[string]*terraforminfra.StateSnapshot
	remote   map[string]*terraforminfra.StateSnapshot
	ops      []string
	copyFn   func(*terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot
}

// setupMoveFixture returns a moveFixture on the configured backend type. The cluster is reachable.
func setupMoveFixture(t *testing.T, backend string) *moveFixture {
	t.Helper()
	f := &moveFixture{
		mocks:    setupProvisionerMocks(t),
		stack:    terraforminfra.NewMockStack(),
		backend:  backend,
		pointers: map[string]string{},
		local:    map[string]*terraforminfra.StateSnapshot{},
		remote:   map[string]*terraforminfra.StateSnapshot{},
		copyFn: func(s *terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot {
			c := *s
			return &c
		},
	}
	mockCH := f.mocks.ConfigHandler.(*config.MockConfigHandler)
	baseGetString := mockCH.GetStringFunc
	mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "terraform.backend.type" {
			return f.backend
		}
		return baseGetString(key, defaultValue...)
	}
	mockCH.SetFunc = func(key string, value any) error {
		if key == "terraform.backend.type" {
			f.backend = value.(string)
		}
		return nil
	}
	migrate := func(id string) {
		f.ops = append(f.ops, fmt.Sprintf("migrate:%s@%s", id, f.backend))
		if f.backend == "local" {
			if f.remote[id] != nil {
				f.local[id] = f.copyFn(f.remote[id])
			}
		} else if f.local[id] != nil {
			f.remote[id] = f.copyFn(f.local[id])
		}
		f.pointers[id] = f.backend
	}
	f.stack.BackendPointerTypeFunc = func(id string) (string, error) { return f.pointers[id], nil }
	f.stack.HasLocalStateWithResourcesFunc = func(id string) (bool, error) {
		return f.local[id] != nil && f.local[id].Resources > 0, nil
	}
	f.stack.HasRemoteStateFunc = func(_ *blueprintv1alpha1.Blueprint, id string) (bool, error) {
		return f.remote[id] != nil, nil
	}
	f.stack.LocalStateSnapshotFunc = func(id string) (*terraforminfra.StateSnapshot, error) { return f.local[id], nil }
	f.stack.StateSnapshotFunc = func(_ *blueprintv1alpha1.Blueprint, id string) (*terraforminfra.StateSnapshot, error) {
		return f.remote[id], nil
	}
	f.stack.InitComponentFunc = func(_ *blueprintv1alpha1.Blueprint, id string) error {
		f.pointers[id] = f.backend
		return nil
	}
	f.stack.MigrateComponentStateFunc = func(_ *blueprintv1alpha1.Blueprint, id string) error {
		migrate(id)
		return nil
	}
	f.stack.MigrateStateFunc = func(bp *blueprintv1alpha1.Blueprint) ([]string, error) {
		for _, c := range bp.TerraformComponents {
			migrate(c.GetID())
		}
		return nil, nil
	}
	f.stack.RemoveLocalStateFunc = func(id string) error {
		f.ops = append(f.ops, "remove:"+id)
		delete(f.local, id)
		return nil
	}
	f.stack.UpFunc = func(bp *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
		var ids []string
		for _, c := range bp.TerraformComponents {
			ids = append(ids, c.GetID())
		}
		f.ops = append(f.ops, fmt.Sprintf("up:%s@%s", strings.Join(ids, ","), f.backend))
		return false, nil
	}
	f.prov = provisioner.NewProvisioner(f.mocks.Runtime, f.mocks.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack:    f.stack,
		KubernetesManager: f.mocks.KubernetesManager,
	})
	return f
}

// snapshot returns a state snapshot with the given lineage, serial 5, and 3 resources.
func snapshot(lineage string) *terraforminfra.StateSnapshot {
	return &terraforminfra.StateSnapshot{Lineage: lineage, Serial: 5, Resources: 3}
}

// componentsBlueprint returns a blueprint with one terraform component for each path.
func componentsBlueprint(paths ...string) *blueprintv1alpha1.Blueprint {
	bp := &blueprintv1alpha1.Blueprint{}
	for _, p := range paths {
		bp.TerraformComponents = append(bp.TerraformComponents, blueprintv1alpha1.TerraformComponent{Path: p})
	}
	return bp
}

// captureMessages runs fn with a presenter context and returns its info and warning messages.
func captureMessages(t *testing.T, fn func(ctx context.Context)) (info, warn string) {
	t.Helper()
	rec := &eventRecorder{}
	fn(presenter.NewContext(context.Background(), rec))
	var infos, warns []string
	for _, e := range rec.events {
		if e.Kind != presenter.KindMessage {
			continue
		}
		if e.Level == slog.LevelWarn {
			warns = append(warns, e.Message)
		} else {
			infos = append(infos, e.Message)
		}
	}
	return strings.Join(infos, "\n"), strings.Join(warns, "\n")
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_PendingBackendMove(t *testing.T) {
	t.Run("ListsTheComponentsWhoseStateIsInTheOldBackend", func(t *testing.T) {
		// Given a kubernetes backend, two components with local state, and one new component
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cluster"], f.local["cluster"] = "local", snapshot("a")
		f.local["cni"] = snapshot("b")

		// When PendingBackendMove is called
		move, err := f.prov.PendingBackendMove(componentsBlueprint("cluster", "backend", "cni"))

		// Then it names the move and the two components
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := []provisioner.ComponentMove{{ID: "cluster", From: "local"}, {ID: "cni", From: "local"}}
		if move == nil || move.To != "kubernetes" || !slices.Equal(move.Components, want) {
			t.Errorf("Expected a local to kubernetes move of cluster and cni, got %+v", move)
		}
	})

	t.Run("RecordsTheSourceOfEachComponent", func(t *testing.T) {
		// Given an s3 backend, one component with local state, and one with state in the cluster
		f := setupMoveFixture(t, "s3")
		f.pointers["cluster"], f.local["cluster"] = "local", snapshot("a")
		f.pointers["cni"] = "kubernetes"

		// When PendingBackendMove is called
		move, err := f.prov.PendingBackendMove(componentsBlueprint("cluster", "cni"))

		// Then each component keeps its own source, and Sources names both
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := []provisioner.ComponentMove{{ID: "cluster", From: "local"}, {ID: "cni", From: "kubernetes"}}
		if move == nil || !slices.Equal(move.Components, want) || !slices.Equal(move.Sources(), []string{"local", "kubernetes"}) {
			t.Errorf("Expected sources local and kubernetes, got %+v", move)
		}
	})

	t.Run("ReturnsNilWhenEveryPointerNamesTheConfiguredBackend", func(t *testing.T) {
		// Given pointers that match the configured backend
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cluster"], f.remote["cluster"] = "kubernetes", snapshot("a")

		// When PendingBackendMove is called
		move, err := f.prov.PendingBackendMove(componentsBlueprint("cluster"))

		// Then no move is pending
		if err != nil || move != nil {
			t.Errorf("Expected no move and no error, got %+v, %v", move, err)
		}
	})

	t.Run("IgnoresALocalPointerWithNoLocalState", func(t *testing.T) {
		// Given a local pointer and no local state file
		f := setupMoveFixture(t, "s3")
		f.pointers["network"] = "local"

		// When PendingBackendMove is called
		move, err := f.prov.PendingBackendMove(componentsBlueprint("network"))

		// Then no move is pending
		if err != nil || move != nil {
			t.Errorf("Expected no move and no error, got %+v, %v", move, err)
		}
	})
}

func TestProvisioner_Up_BackendMove(t *testing.T) {
	t.Run("MovesEveryComponentFromLocalToKubernetes", func(t *testing.T) {
		// Given a kubernetes backend declared by a backend component, and local state for the others
		f := setupMoveFixture(t, "kubernetes")
		for _, id := range []string{"pki", "cluster", "cni", "flux"} {
			f.pointers[id], f.local[id] = "local", snapshot(id)
		}
		bp := componentsBlueprint("pki", "cluster", "backend", "cni", "flux")
		bp.Backend = "backend"

		// When Up is called
		if _, err := f.prov.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then every state is in the cluster, no local file is left, and the rest applies after the move
		for _, id := range []string{"pki", "cluster", "cni", "flux"} {
			if f.remote[id] == nil || f.remote[id].Lineage != id {
				t.Errorf("Expected the state of %s in the cluster, got %+v", id, f.remote[id])
			}
			if f.local[id] != nil {
				t.Errorf("Expected no local state for %s, got %+v", id, f.local[id])
			}
			if f.pointers[id] != "kubernetes" {
				t.Errorf("Expected the pointer of %s to name kubernetes, got %q", id, f.pointers[id])
			}
		}
		last := f.ops[len(f.ops)-1]
		if last != "up:cni,flux@kubernetes" {
			t.Errorf("Expected the last operation to apply cni and flux on kubernetes, got %v", f.ops)
		}
		if slices.Index(f.ops, "migrate:cni@kubernetes") > slices.Index(f.ops, last) {
			t.Errorf("Expected cni to move before the apply, got %v", f.ops)
		}
	})

	t.Run("MovesEveryComponentFromKubernetesToLocal", func(t *testing.T) {
		// Given a local backend and state in the cluster for two components
		f := setupMoveFixture(t, "local")
		for _, id := range []string{"cluster", "cni"} {
			f.pointers[id], f.remote[id] = "kubernetes", snapshot(id)
		}

		// When Up is called
		var err error
		info, _ := captureMessages(t, func(ctx context.Context) {
			_, err = f.prov.Up(ctx, componentsBlueprint("cluster", "cni"))
		})

		// Then each state is in a local file before the apply, and the cluster copy stays
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, id := range []string{"cluster", "cni"} {
			if f.local[id] == nil || f.local[id].Lineage != id {
				t.Errorf("Expected local state for %s, got %+v", id, f.local[id])
			}
			if f.remote[id] == nil {
				t.Errorf("Expected the cluster copy of %s to stay", id)
			}
		}
		if f.ops[len(f.ops)-1] != "up:cluster,cni@local" {
			t.Errorf("Expected the apply after the moves, got %v", f.ops)
		}
		if !strings.Contains(info, "Secrets stay in the cluster") {
			t.Errorf("Expected a message that the Secrets stay, got %q", info)
		}
	})

	t.Run("ResumesAnInterruptedMove", func(t *testing.T) {
		// Given one moved component that kept its local file, one stale local file, and one component not moved
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cluster"], f.remote["cluster"], f.local["cluster"] = "kubernetes", snapshot("a"), snapshot("a")
		f.pointers["cni"], f.remote["cni"], f.local["cni"] = "kubernetes", snapshot("b"), snapshot("other")
		f.pointers["flux"], f.local["flux"] = "local", snapshot("c")

		// When Up is called
		var err error
		_, warn := captureMessages(t, func(ctx context.Context) {
			_, err = f.prov.Up(ctx, componentsBlueprint("cluster", "cni", "flux"))
		})

		// Then the matching file goes, the stale file stays with a warning, and the rest moves
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if f.local["cluster"] != nil {
			t.Errorf("Expected the local file of cluster to go, got %+v", f.local["cluster"])
		}
		if f.local["cni"] == nil || !strings.Contains(warn, `"cni" does not match`) {
			t.Errorf("Expected the local file of cni to stay with a warning, got %+v, %q", f.local["cni"], warn)
		}
		if f.remote["flux"] == nil || f.local["flux"] != nil {
			t.Errorf("Expected flux to move, got remote %+v local %+v", f.remote["flux"], f.local["flux"])
		}
	})

	t.Run("RefusesWhenTheClusterIsUnreachable", func(t *testing.T) {
		// Given a move out of the cluster and a cluster that does not answer
		f := setupMoveFixture(t, "local")
		f.pointers["cluster"], f.remote["cluster"] = "kubernetes", snapshot("a")
		f.mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(context.Context, string, func(string), ...string) error {
			return fmt.Errorf("connection refused")
		}

		// When Up is called
		_, err := f.prov.Up(context.Background(), componentsBlueprint("cluster"))

		// Then Up refuses before any state operation or apply
		if err == nil || !strings.Contains(err.Error(), "cannot reach the cluster") {
			t.Fatalf("Expected an unreachable cluster error, got %v", err)
		}
		if len(f.ops) != 0 {
			t.Errorf("Expected no operation, got %v", f.ops)
		}
	})

	t.Run("RefusesWhenALaterComponentReadsAnUnreachableCluster", func(t *testing.T) {
		// Given an s3 backend, a first component on local state, a second on cluster state, and no cluster
		f := setupMoveFixture(t, "s3")
		f.pointers["cluster"], f.local["cluster"] = "local", snapshot("a")
		f.pointers["cni"], f.remote["cni"] = "kubernetes", snapshot("b")
		f.mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(context.Context, string, func(string), ...string) error {
			return fmt.Errorf("connection refused")
		}

		// When Up is called
		_, err := f.prov.Up(context.Background(), componentsBlueprint("cluster", "cni"))

		// Then Up refuses before any state operation and names the kubernetes source
		if err == nil || !strings.Contains(err.Error(), "from the kubernetes backend to the s3 backend") {
			t.Fatalf("Expected an unreachable cluster error for the kubernetes source, got %v", err)
		}
		if len(f.ops) != 0 {
			t.Errorf("Expected no operation, got %v", f.ops)
		}
	})

	t.Run("StopsAndKeepsTheSourceWhenTheCopyDoesNotMatch", func(t *testing.T) {
		// Given a copy that loses a resource
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cni"], f.local["cni"] = "local", snapshot("a")
		f.copyFn = func(s *terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot {
			c := *s
			c.Resources--
			return &c
		}

		// When Up is called
		_, err := f.prov.Up(context.Background(), componentsBlueprint("cni"))

		// Then Up fails, keeps the local file, and applies nothing
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("Expected a mismatch error, got %v", err)
		}
		if f.local["cni"] == nil {
			t.Error("Expected the local file to stay")
		}
		for _, op := range f.ops {
			if strings.HasPrefix(op, "up:") || strings.HasPrefix(op, "remove:") {
				t.Errorf("Expected no apply and no removal, got %v", f.ops)
			}
		}
	})
}

func TestProvisioner_Up_BackendMoveMismatchReport(t *testing.T) {
	cases := []struct {
		name string
		copy func(*terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot
		want string
	}{
		{"CopyReadsBackEmpty", func(*terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot { return nil }, "kubernetes: read back empty"},
		{"CopyHasLowerSerial", func(s *terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot {
			c := *s
			c.Serial--
			return &c
		}, "kubernetes: lineage a, serial 4, 3 resources"},
		{"CopyHasOtherLineage", func(s *terraforminfra.StateSnapshot) *terraforminfra.StateSnapshot {
			c := *s
			c.Lineage = "b"
			return &c
		}, "kubernetes: lineage b, serial 5, 3 resources"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given a copy that differs from the local source
			f := setupMoveFixture(t, "kubernetes")
			f.pointers["cni"], f.local["cni"] = "local", snapshot("a")
			f.copyFn = tc.copy

			// When Up is called
			_, err := f.prov.Up(context.Background(), componentsBlueprint("cni"))

			// Then the error reports both states
			if err == nil {
				t.Fatal("Expected a mismatch error")
			}
			for _, want := range []string{"local: lineage a, serial 5, 3 resources", tc.want} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Expected %q in %q", want, err.Error())
				}
			}
		})
	}
}

func TestProvisioner_Apply_BackendMove(t *testing.T) {
	t.Run("RefusesAComponentWithAPendingMove", func(t *testing.T) {
		// Given a component whose state is still local on a kubernetes backend
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cni"], f.local["cni"] = "local", snapshot("a")
		applied := false
		f.stack.ApplyFunc = func(*blueprintv1alpha1.Blueprint, string) error {
			applied = true
			return nil
		}

		// When Apply is called for that component
		err := f.prov.Apply(context.Background(), componentsBlueprint("cni"), "cni")

		// Then Apply refuses and points to windsor apply
		if err == nil || !strings.Contains(err.Error(), "in the local backend") {
			t.Fatalf("Expected a pending move error, got %v", err)
		}
		if applied {
			t.Error("Expected no terraform apply")
		}
	})
}

func TestProvisioner_PlanTerraformSummary_BackendMove(t *testing.T) {
	t.Run("PlansEachComponentAgainstTheBackendThatHoldsItsState", func(t *testing.T) {
		// Given a kubernetes backend with two components still on local state and one new component
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cluster"], f.local["cluster"] = "local", snapshot("a")
		f.pointers["cni"], f.local["cni"] = "local", snapshot("b")
		var calls []string
		f.stack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			var out []terraforminfra.TerraformComponentPlan
			for _, c := range bp.TerraformComponents {
				calls = append(calls, fmt.Sprintf("%s@%s", c.GetID(), f.backend))
				out = append(out, terraforminfra.TerraformComponentPlan{ComponentID: c.GetID()})
			}
			return out
		}

		// When PlanTerraformSummary is called
		summary, err := f.prov.PlanTerraformSummary(componentsBlueprint("cluster", "backend", "cni"))

		// Then each component plans against its state backend, in blueprint order, and nothing moves
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if want := []string{"cluster@local", "backend@kubernetes", "cni@local"}; !slices.Equal(calls, want) {
			t.Errorf("Expected plans %v, got %v", want, calls)
		}
		var ids []string
		for _, r := range summary.Terraform {
			ids = append(ids, r.ComponentID)
		}
		if !slices.Equal(ids, []string{"cluster", "backend", "cni"}) {
			t.Errorf("Expected the summary in blueprint order, got %v", ids)
		}
		if f.backend != "kubernetes" || len(f.ops) != 0 || f.remote["cluster"] != nil {
			t.Errorf("Expected no move and the backend type restored, got backend %q ops %v", f.backend, f.ops)
		}
	})

	t.Run("RefusesToReadAnUnreachableCluster", func(t *testing.T) {
		// Given state in a cluster that does not answer and a local backend
		f := setupMoveFixture(t, "local")
		f.pointers["cluster"], f.remote["cluster"] = "kubernetes", snapshot("a")
		f.mocks.KubernetesManager.WaitForKubernetesHealthyFunc = func(context.Context, string, func(string), ...string) error {
			return fmt.Errorf("connection refused")
		}

		// When PlanTerraformSummary is called
		_, err := f.prov.PlanTerraformSummary(componentsBlueprint("cluster"))

		// Then the plan refuses with the unreachable cluster error
		if err == nil || !strings.Contains(err.Error(), "cannot reach the cluster") {
			t.Errorf("Expected an unreachable cluster error, got %v", err)
		}
	})
}

func TestProvisioner_Plan_BackendMove(t *testing.T) {
	t.Run("PlansOneComponentAgainstItsStateBackend", func(t *testing.T) {
		// Given a component still on local state on a kubernetes backend
		f := setupMoveFixture(t, "kubernetes")
		f.pointers["cni"], f.local["cni"] = "local", snapshot("a")
		var planned string
		f.stack.PlanFunc = func(*blueprintv1alpha1.Blueprint, string) error {
			planned = f.backend
			return nil
		}

		// When Plan is called for the component
		err := f.prov.Plan(componentsBlueprint("cni"), "cni")

		// Then the plan reads the local state and the backend type is restored
		if err != nil || planned != "local" || f.backend != "kubernetes" {
			t.Errorf("Expected a local plan and kubernetes after, got planned %q backend %q err %v", planned, f.backend, err)
		}
	})
}
