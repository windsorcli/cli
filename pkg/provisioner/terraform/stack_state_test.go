package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	terraformRuntime "github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_MigrateState(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if _, err := stack.MigrateState(context.Background(), nil); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("RunsInitWithMigrateStateForEachComponent", func(t *testing.T) {
		// Given a stack and a two-component blueprint
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		migrateStateInits := 0
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-migrate-state" {
						migrateStateInits++
						break
					}
				}
			}
			return "", nil
		}

		// When MigrateState runs
		skipped, err := stack.MigrateState(context.Background(), blueprint)
		if err != nil {
			t.Fatalf("Expected MigrateState to succeed, got %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("Expected no skipped components when all dirs exist, got %v", skipped)
		}

		// Then terraform init -migrate-state fired once per component and no plan/apply ran.
		if migrateStateInits != 2 {
			t.Errorf("Expected 2 migrate-state inits (one per component), got %d", migrateStateInits)
		}
	})

	t.Run("DoesNotRefreshWhenMigrateStateSucceeds", func(t *testing.T) {
		// refreshProviderLock reaches the provider registry. Running it on every call
		// would break migrate-state in network-restricted or air-gapped environments
		// that previously worked offline. It must run only after a failed init.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var refreshCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-backend=false") {
				refreshCalls++
			}
			return "", nil
		}

		if _, err := stack.MigrateState(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected MigrateState to succeed, got %v", err)
		}

		if refreshCalls != 0 {
			t.Errorf("Expected no provider-lock refresh when migrate-state succeeds, got %d calls", refreshCalls)
		}
	})

	t.Run("RefreshesAndRetriesAfterAStaleLockFailure", func(t *testing.T) {
		// A stale .terraform.lock.hcl left over from a prior init isn't invalidated
		// when the module's provider constraint changes upstream, and -migrate-state
		// must never carry -upgrade to self-heal it. On that specific failure,
		// migrateOneComponent refreshes the lock once, then retries -migrate-state once.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[0].GetID()

		var migrateStateInits int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) < 2 || args[1] != "init" {
				return "", nil
			}
			if slices.Contains(args, "-migrate-state") {
				migrateStateInits++
				if migrateStateInits == 1 {
					return "", fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions")
				}
			}
			return "", nil
		}

		if err := stack.MigrateComponentState(blueprint, targetID); err != nil {
			t.Fatalf("Expected MigrateComponentState to succeed after the retry, got %v", err)
		}

		if migrateStateInits != 2 {
			t.Errorf("Expected the -migrate-state init to run once, fail, then run once more, got %d calls", migrateStateInits)
		}
	})

	t.Run("StopsAfterAFailedRefresh", func(t *testing.T) {
		// If the provider-lock refresh itself fails, migrateOneComponent must not
		// retry -migrate-state at all. A component with unresolved providers
		// has no business attempting a state move.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var migrateStateInits int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) < 2 || args[1] != "init" {
				return "", nil
			}
			if slices.Contains(args, "-migrate-state") {
				migrateStateInits++
				return "", fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions")
			}
			return "", fmt.Errorf("registry unreachable")
		}

		_, err := stack.MigrateState(context.Background(), blueprint)
		if err == nil {
			t.Fatal("Expected MigrateState to return an error when the provider-lock refresh fails")
		}
		if !strings.Contains(err.Error(), "registry unreachable") {
			t.Errorf("Expected error to wrap the refresh failure, got: %v", err)
		}
		if migrateStateInits != 1 {
			t.Errorf("Expected exactly one failed -migrate-state init and no retry, got %d", migrateStateInits)
		}
	})

	t.Run("DoesNotPassUpgradeFlag", func(t *testing.T) {
		// The -migrate-state init call itself must not include -upgrade: its
		// contract is moving state, not reinstalling providers. The separate,
		// backend-disconnected refreshProviderLock pre-step legitimately passes
		// -upgrade before this runs — see TestTerraformStack_refreshProviderLock —
		// but the migrate-state call must never carry it.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var upgradeOnMigrateState bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-migrate-state") {
				if slices.Contains(args, "-upgrade") {
					upgradeOnMigrateState = true
				}
			}
			return "", nil
		}

		// When MigrateState runs
		if _, err := stack.MigrateState(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected MigrateState to succeed, got %v", err)
		}

		// Then the -migrate-state init call never carried -upgrade
		if upgradeOnMigrateState {
			t.Error("Expected the -migrate-state init call not to include -upgrade; state migration must not reinstall providers")
		}
	})

	t.Run("ReturnsPartialSkippedListAlongsideError", func(t *testing.T) {
		// Given a two-component blueprint where the first component's directory is missing
		// (will be appended to skipped) and the second component's terraform init fails.
		// MigrateState must return BOTH: the skipped list so far AND the error, so a
		// caller (e.g. bootstrap) can render "skipped A, then B failed" in a single
		// diagnostic rather than losing the skip signal on the error path.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		statCallsByPath := make(map[string]int)
		originalStat := mocks.Shims.Stat
		firstSeen := ""
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			// The MigrateState loop calls Stat once on component.FullPath per component.
			// Treat the first unique path seen as the "missing" one to get a skip; every
			// other path falls through to the default behavior so init can run against it.
			if strings.HasSuffix(path, "backend_override.tf") {
				return originalStat(path)
			}
			statCallsByPath[path]++
			if firstSeen == "" {
				firstSeen = path
			}
			if path == firstSeen {
				return nil, os.ErrNotExist
			}
			return originalStat(path)
		}

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		skipped, err := stack.MigrateState(context.Background(), blueprint)

		// Then the error from the second component surfaces AND the skip from the first
		// survives — callers that only looked at err previously would have silently lost
		// the fact that something upstream was already missing
		if err == nil {
			t.Fatal("Expected MigrateState to return an error")
		}
		if len(skipped) == 0 {
			t.Error("Expected skipped list to include the first component whose directory was missing")
		}
	})

	t.Run("ReturnsErrorWhenMigrationInitFails", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-migrate-state") {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		_, err := stack.MigrateState(context.Background(), blueprint)
		if err == nil {
			t.Fatal("Expected MigrateState to return an error when init fails")
		}
		if !strings.Contains(err.Error(), "error running terraform init") {
			t.Errorf("Expected error to mention terraform init failure, got: %v", err)
		}
		if !strings.Contains(err.Error(), "backend not reachable") {
			t.Errorf("Expected error to wrap the underlying cause, got: %v", err)
		}
	})

	t.Run("HintsAtStaleProviderLockWithoutAddingUpgrade", func(t *testing.T) {
		// Given a component whose migrate-state init still fails with the stale-
		// provider-lock signature even after the pre-flight refreshProviderLock
		// step succeeds — the -migrate-state call itself must still never pass
		// -upgrade (cli#3157: the hint is diagnostic only, not a retry, precisely
		// because this contract must hold)
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var upgradeOnMigrateState bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-migrate-state") {
				if slices.Contains(args, "-upgrade") {
					upgradeOnMigrateState = true
				}
				return "", fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions")
			}
			return "", nil
		}

		_, err := stack.MigrateState(context.Background(), blueprint)

		// Then the error carries the hint but the -migrate-state call never added -upgrade
		if err == nil {
			t.Fatal("Expected MigrateState to return an error")
		}
		if !strings.Contains(err.Error(), "provider version outside the current constraint") && !strings.Contains(err.Error(), "local terraform cache for this component is stale") {
			t.Errorf("Expected a stale-provider-lock hint, got: %v", err)
		}
		if upgradeOnMigrateState {
			t.Error("Expected the -migrate-state init call never to pass -upgrade, even when hinting at a stale lock")
		}
	})

	t.Run("SkipsComponentsWithMissingDirectoriesAndReportsThem", func(t *testing.T) {
		// Given a stat shim that reports every component directory as missing — the
		// blueprint may list components that were never applied (or were already
		// torn down manually), and MigrateState is called before destroy precisely
		// when some of that state may be absent. The skipped components must be
		// returned to the caller so bootstrap (which calls MigrateState after Up
		// has materialized all dirs) can treat a non-empty skip as an anomaly,
		// while pre-destroy migration can discard the list and proceed.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		var initsSeen int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				initsSeen++
			}
			return "", nil
		}

		// When MigrateState runs
		skipped, err := stack.MigrateState(context.Background(), blueprint)
		if err != nil {
			t.Fatalf("Expected no error when every component dir is missing, got %v", err)
		}

		// Then no terraform init was invoked — the missing components were skipped —
		// and both component IDs appear in the returned skip list.
		if initsSeen != 0 {
			t.Errorf("Expected 0 init invocations when all dirs are missing, got %d", initsSeen)
		}
		if len(skipped) != 2 {
			t.Fatalf("Expected 2 skipped component IDs, got %d: %v", len(skipped), skipped)
		}
	})
}

func TestStack_MigrateComponentState(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if err := stack.MigrateComponentState(nil, "backend"); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("ReturnsErrorForEmptyComponentID", func(t *testing.T) {
		stack, _ := setup(t)
		if err := stack.MigrateComponentState(createTestBlueprint(), ""); err == nil {
			t.Fatal("Expected error for empty component ID")
		}
	})

	t.Run("ReturnsErrorWhenComponentNotFound", func(t *testing.T) {
		// Given a blueprint that does not declare the named component, the per-
		// component variant must fail loudly rather than silently no-op — the caller
		// (bootstrap) explicitly asked for a specific component to be migrated.
		stack, _ := setup(t)
		err := stack.MigrateComponentState(createTestBlueprint(), "nonexistent")
		if err == nil {
			t.Fatal("Expected error when component is not in the blueprint")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("Expected error to name the missing component, got: %v", err)
		}
	})

	t.Run("RunsInitWithMigrateStateForOneComponent", func(t *testing.T) {
		// Given a two-component blueprint and a request to migrate only the second
		// component, exactly one terraform init -migrate-state must fire and only
		// against that component's directory.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[1].GetID()

		var migrateStateInits int
		var initChdirs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-migrate-state") {
				migrateStateInits++
				if strings.HasPrefix(args[0], "-chdir=") {
					initChdirs = append(initChdirs, strings.TrimPrefix(args[0], "-chdir="))
				}
			}
			return "", nil
		}

		if err := stack.MigrateComponentState(blueprint, targetID); err != nil {
			t.Fatalf("Expected MigrateComponentState to succeed, got %v", err)
		}
		if migrateStateInits != 1 {
			t.Errorf("Expected 1 migrate-state init for the targeted component, got %d", migrateStateInits)
		}
		// targetID is the blueprint Path (forward slashes) but FullPath runs through
		// filepath.FromSlash, so initChdirs[0] is OS-native (backslashes on Windows).
		// Normalize before substring matching so the assertion is portable.
		if len(initChdirs) != 1 || !strings.Contains(filepath.ToSlash(initChdirs[0]), targetID) {
			t.Errorf("Expected init to run against the targeted component dir, got %v", initChdirs)
		}
	})

	t.Run("ReturnsErrorWhenComponentDirectoryMissing", func(t *testing.T) {
		// Given the targeted component's directory is missing on disk, the per-
		// component variant treats this as an error (unlike MigrateState's bulk
		// loop, which collects skipped IDs). The single-component caller asked for
		// a specific migration and got nothing — silent skip would leave the caller
		// believing the migration succeeded.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[0].GetID()

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		err := stack.MigrateComponentState(blueprint, targetID)
		if err == nil {
			t.Fatal("Expected error when component directory is missing")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Expected error to mention missing directory, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInitFails", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[0].GetID()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" && slices.Contains(args, "-migrate-state") {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		err := stack.MigrateComponentState(blueprint, targetID)
		if err == nil {
			t.Fatal("Expected error when terraform init fails")
		}
		if !strings.Contains(err.Error(), "backend not reachable") {
			t.Errorf("Expected error to wrap the underlying cause, got %v", err)
		}
	})
}

func TestStack_ListLocalStateComponentIDs(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("DelegatesToTerraformProvider", func(t *testing.T) {
		// Given a stack whose TerraformProvider reports two componentIDs with local state
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			ListLocalStateComponentIDsFunc: func() ([]string, error) {
				return []string{"network", "crossplane-identity-gcp"}, nil
			},
		}

		// When listing local state componentIDs
		ids, err := stack.ListLocalStateComponentIDs()

		// Then the TerraformProvider's own list is returned unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(ids) != 2 || ids[0] != "network" || ids[1] != "crossplane-identity-gcp" {
			t.Errorf("Expected the provider's componentIDs, got %v", ids)
		}
	})

	t.Run("PropagatesProviderError", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			ListLocalStateComponentIDsFunc: func() ([]string, error) {
				return nil, fmt.Errorf("scratch path error")
			},
		}

		_, err := stack.ListLocalStateComponentIDs()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}
