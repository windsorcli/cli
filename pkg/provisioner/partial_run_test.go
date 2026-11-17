package provisioner_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/provisioner"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

// runPlanBlueprint returns a blueprint with the terraform components a, b, c in apply order
// and the kustomizations base, app (depends on base), and other.
func runPlanBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{Name: "a", Path: "p/a"},
			{Name: "b", Path: "p/b", DependsOn: []string{"a"}},
			{Name: "c", Path: "p/c", DependsOn: []string{"b"}},
		},
		Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "base"},
			{Name: "app", DependsOn: []string{"base"}},
			{Name: "other"},
		},
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBuildRunPlan(t *testing.T) {
	t.Run("TerraformTargetRunsEveryComponentUpToAndIncludingIt", func(t *testing.T) {
		// Given a blueprint with the components a, b, c
		bp := runPlanBlueprint()

		// When the target is the component b
		plan, err := provisioner.BuildRunPlan(bp, "up", "b", nil, nil)

		// Then the plan applies a and b, and does not install
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(plan.Apply, []string{"a", "b"}) {
			t.Errorf("Expected apply [a b], got %v", plan.Apply)
		}
		if plan.Install || !plan.Stops() {
			t.Errorf("Expected a stop with no install, got install=%v stops=%v", plan.Install, plan.Stops())
		}
	})

	t.Run("KustomizationTargetInstallsItsDependencyClosure", func(t *testing.T) {
		// Given a blueprint where app depends on base
		bp := runPlanBlueprint()

		// When the target is the kustomization app
		plan, err := provisioner.BuildRunPlan(bp, "up", "app", nil, nil)

		// Then all components apply and the install holds app and base, but not other
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(plan.Apply, []string{"a", "b", "c"}) {
			t.Errorf("Expected apply [a b c], got %v", plan.Apply)
		}
		if !plan.Install || !plan.KustomizeSubset || !slices.Equal(plan.Kustomize, []string{"base", "app"}) {
			t.Errorf("Expected install of [base app], got install=%v subset=%v %v", plan.Install, plan.KustomizeSubset, plan.Kustomize)
		}
		installed := plan.InstallBlueprint(bp).AllKustomizations()
		if len(installed) != 2 || installed[0].Name != "base" || installed[1].Name != "app" {
			t.Errorf("Expected the install blueprint to hold base and app, got %v", installed)
		}
	})

	t.Run("NoTargetRunsEverything", func(t *testing.T) {
		// Given a blueprint and no target
		// When the plan builds
		plan, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "", nil, nil)

		// Then it applies every component, installs everything, and does not stop
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(plan.Apply, []string{"a", "b", "c"}) || !plan.Install || plan.KustomizeSubset || plan.Stops() {
			t.Errorf("Expected a full run, got %+v", plan)
		}
	})

	t.Run("UnknownNameFailsAndListsValidNames", func(t *testing.T) {
		// Given a blueprint
		// When the target matches nothing
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "nope", nil, nil)

		// Then the error names the target and lists the valid names
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) {
			t.Fatalf("Expected ErrInvalidRunTarget, got %v", err)
		}
		we := werror.Find(err)
		if we == nil || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(we.Remediation, "a, b, c") || !strings.Contains(we.Remediation, "base, app, other") {
			t.Errorf("Expected the error to list the valid names, got %v", err)
		}
	})

	t.Run("WrongKindPrefixFails", func(t *testing.T) {
		// Given a blueprint
		// When the target uses the terraform/ prefix with a kustomization name
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "terraform/app", nil, nil)

		// Then the target is unknown
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) {
			t.Fatalf("Expected ErrInvalidRunTarget, got %v", err)
		}
	})

	t.Run("AmbiguousNameNeedsAPrefix", func(t *testing.T) {
		// Given a name that is a component and a kustomization
		bp := runPlanBlueprint()
		bp.Kustomizations = append(bp.Kustomizations, blueprintv1alpha1.Kustomization{Name: "b"})

		// When the target is the bare name
		_, err := provisioner.BuildRunPlan(bp, "up", "b", nil, nil)

		// Then the error asks for a prefix, and each prefix selects its kind
		we := werror.Find(err)
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) || we == nil || !strings.Contains(we.Remediation, "terraform/b") {
			t.Fatalf("Expected an ambiguity error with the prefixes, got %v", err)
		}
		tf, err := provisioner.BuildRunPlan(bp, "up", "terraform/b", nil, nil)
		if err != nil || tf.StopKind != "terraform" {
			t.Errorf("Expected a terraform target, got %+v, %v", tf, err)
		}
		k, err := provisioner.BuildRunPlan(bp, "up", "kustomize/b", nil, nil)
		if err != nil || k.StopKind != "kustomize" {
			t.Errorf("Expected a kustomize target, got %+v, %v", k, err)
		}
	})

	t.Run("NameInsideBackendTierFails", func(t *testing.T) {
		// Given a backend tier of a and b
		tier := []string{"a", "b"}

		// When the target is a, which is inside the tier and not its last component
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "bootstrap", "a", nil, tier)

		// Then the error names the last component of the tier
		we := werror.Find(err)
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) || we == nil || !strings.Contains(we.Remediation, "--to b") {
			t.Fatalf("Expected a backend tier error, got %v", err)
		}
		if _, err := provisioner.BuildRunPlan(runPlanBlueprint(), "bootstrap", "b", nil, tier); err != nil {
			t.Errorf("Expected the last tier component to be valid, got %v", err)
		}
	})

	t.Run("ContinueSkipsTheFinishedComponents", func(t *testing.T) {
		// Given a record that finished a
		record := &provisioner.PartialRun{Command: "up", Terraform: []string{"a"}, Components: []string{"a", "b", "c"}}

		// When the run continues with no target
		plan, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "", record, nil)

		// Then a is skipped and b and c apply
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(plan.Skip, []string{"a"}) || !slices.Equal(plan.Apply, []string{"b", "c"}) {
			t.Errorf("Expected skip [a] and apply [b c], got %v and %v", plan.Skip, plan.Apply)
		}
	})

	t.Run("ContinueWithATargetKeepsTheSkippedComponents", func(t *testing.T) {
		// Given a record that finished a
		record := &provisioner.PartialRun{Command: "up", Terraform: []string{"a"}, Components: []string{"a", "b", "c"}}

		// When the run continues to b
		plan, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "b", record, nil)

		// Then only b applies, and a and b are finished
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(plan.Apply, []string{"b"}) || !slices.Equal(plan.Finished(), []string{"a", "b"}) {
			t.Errorf("Expected apply [b] and finished [a b], got %v and %v", plan.Apply, plan.Finished())
		}
	})

	t.Run("ContinueRejectsATargetThatAlreadyFinished", func(t *testing.T) {
		// Given a record that finished a and b
		record := &provisioner.PartialRun{Command: "up", Terraform: []string{"a", "b"}, Components: []string{"a", "b", "c"}}

		// When the run continues to a
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "a", record, nil)

		// Then the run fails
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) {
			t.Fatalf("Expected ErrInvalidRunTarget, got %v", err)
		}
	})

	t.Run("ContinueRejectsARecordFromAnotherCommand", func(t *testing.T) {
		// Given a record from bootstrap
		record := &provisioner.PartialRun{Command: "bootstrap", Terraform: []string{"a"}, Components: []string{"a", "b", "c"}}

		// When up continues
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "", record, nil)

		// Then the error names the command that stopped
		we := werror.Find(err)
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) || we == nil || !strings.Contains(we.Remediation, "windsor bootstrap --continue") {
			t.Fatalf("Expected a command mismatch error, got %v", err)
		}
	})

	t.Run("ContinueRejectsAChangedComponentList", func(t *testing.T) {
		// Given a record made for other components
		record := &provisioner.PartialRun{Command: "up", Terraform: []string{"a"}, Components: []string{"a", "x"}}

		// When the run continues
		_, err := provisioner.BuildRunPlan(runPlanBlueprint(), "up", "", record, nil)

		// Then the run fails
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) {
			t.Fatalf("Expected ErrInvalidRunTarget, got %v", err)
		}
	})
}

func TestRunPlan_TerraformBlueprint(t *testing.T) {
	t.Run("ClearsBackendWhenTheBackendComponentIsSkipped", func(t *testing.T) {
		// Given a blueprint whose backend is a, and a plan that skips a
		bp := runPlanBlueprint()
		bp.Backend = "a"
		plan, err := provisioner.BuildRunPlan(bp, "bootstrap", "",
			&provisioner.PartialRun{Command: "bootstrap", Terraform: []string{"a"}, Components: []string{"a", "b", "c"}}, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// When the terraform blueprint builds
		got := plan.TerraformBlueprint(bp)

		// Then it holds b and c, has no backend, and leaves the original unchanged
		if len(got.TerraformComponents) != 2 || got.Backend != "" {
			t.Errorf("Expected 2 components and no backend, got %d and %q", len(got.TerraformComponents), got.Backend)
		}
		if len(bp.TerraformComponents) != 3 || bp.Backend != "a" {
			t.Errorf("Expected the original blueprint to stay unchanged")
		}
	})
}

func TestProvisioner_PartialRun(t *testing.T) {
	t.Run("StopRecordsProgressAndContinueSkipsTheFinishedComponents", func(t *testing.T) {
		// Given a provisioner with a scratch directory and a stack that records the applied components
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.WindsorScratchPath = t.TempDir()
		var applied []string
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(bp *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			for _, c := range bp.TerraformComponents {
				applied = append(applied, c.GetID())
			}
			return false, nil
		}
		bp := runPlanBlueprint()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})

		// When the run stops after b and records its progress
		plan, err := prov.PlanRun(bp, "up", "b", false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, err := prov.Up(context.Background(), bp); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if err := prov.RecordPartialRun(bp, plan); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the stack applied a and b only
		if !slices.Equal(applied, []string{"a", "b"}) {
			t.Fatalf("Expected applied [a b], got %v", applied)
		}

		// When the next run continues
		applied = nil
		var replayed []string
		contPlan, err := prov.PlanRun(bp, "up", "", true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		hook := func(_ context.Context, id string) (bool, error) {
			replayed = append(replayed, id)
			return false, nil
		}
		if _, err := prov.Up(context.Background(), bp, hook); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the stack applies c only, and the hook runs again for a and b
		if !slices.Equal(applied, []string{"c"}) {
			t.Errorf("Expected applied [c], got %v", applied)
		}
		if !slices.Equal(replayed, []string{"a", "b"}) {
			t.Errorf("Expected the hooks to replay for [a b], got %v", replayed)
		}
		if contPlan.Stops() {
			t.Errorf("Expected the continued run to go to the end")
		}

		// When the record clears
		if err := prov.ClearPartialRun(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then a later continue fails with a hint
		_, err = prov.PlanRun(bp, "up", "", true)
		if !errors.Is(err, provisioner.ErrInvalidRunTarget) {
			t.Errorf("Expected ErrInvalidRunTarget after the record clears, got %v", err)
		}
	})

	t.Run("ReplayHaltStopsTheRun", func(t *testing.T) {
		// Given a record that finished a, and a hook that halts after a
		mocks := setupProvisionerMocks(t)
		mocks.Runtime.WindsorScratchPath = t.TempDir()
		stackRan := false
		mockStack := terraforminfra.NewMockStack()
		mockStack.UpFunc = func(_ *blueprintv1alpha1.Blueprint, _ ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
			stackRan = true
			return false, nil
		}
		bp := runPlanBlueprint()
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{TerraformStack: mockStack})
		plan, err := prov.PlanRun(bp, "up", "a", false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if err := prov.RecordPartialRun(bp, plan); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, err := prov.PlanRun(bp, "up", "", true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// When the continued run replays the hook for a
		halted, err := prov.Up(context.Background(), bp, func(_ context.Context, id string) (bool, error) {
			return id == "a", nil
		})

		// Then the run halts and the stack does not run
		if err != nil || !halted || stackRan {
			t.Errorf("Expected a halt with no stack run, got halted=%v ran=%v err=%v", halted, stackRan, err)
		}
	})

	t.Run("NoFlagsKeepsNoPlan", func(t *testing.T) {
		// Given a provisioner
		mocks := setupProvisionerMocks(t)
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler)

		// When the plan builds with no --to and no --continue
		plan, err := prov.PlanRun(runPlanBlueprint(), "up", "", false)

		// Then there is no plan
		if err != nil || plan != nil {
			t.Errorf("Expected no plan, got %v, %v", plan, err)
		}
	})
}
