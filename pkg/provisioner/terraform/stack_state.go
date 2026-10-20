// The state functions of the TerraformStack read and move terraform state.
// It provides the local state checks and the state migration to a new backend.
// It also refreshes the state of a component before an apply or a destroy.

package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
)

// =============================================================================
// Types
// =============================================================================

// tfStateModule is a node in the module tree emitted by `terraform show -json`. Windsor
// blueprints wrap resources in a `module "main"` block, so the root's own Resources is
// always empty — emptiness must be determined by walking the tree.
type tfStateModule struct {
	Resources    []json.RawMessage `json:"resources"`
	ChildModules []tfStateModule   `json:"child_modules"`
}

// tfState is the minimal shape of `terraform show -json` we need for state emptiness.
type tfState struct {
	Values *struct {
		RootModule tfStateModule `json:"root_module"`
	} `json:"values"`
}

// =============================================================================
// Public Methods
// =============================================================================

// HasLocalStateWithResources reports whether the per-component local state
// file exists and contains at least one resource entry. Reads the file as
// JSON; no terraform invocation. Missing files report (false, nil).
func (s *TerraformStack) HasLocalStateWithResources(componentID string) (bool, error) {
	statePath, err := s.runtime.TerraformProvider.GetStatePath(componentID)
	if err != nil {
		return false, fmt.Errorf("error resolving local state path for %s: %w", componentID, err)
	}
	data, err := s.shims.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("error reading local state file %s: %w", statePath, err)
	}
	var state struct {
		Resources []json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return false, fmt.Errorf("error parsing local state file %s: %w", statePath, err)
	}
	return len(state.Resources) > 0, nil
}

// RemoveLocalState removes the per-component local state file (and its
// .backup sibling) under the windsor scratch path. Missing files are
// tolerated.
func (s *TerraformStack) RemoveLocalState(componentID string) error {
	statePath, err := s.runtime.TerraformProvider.GetStatePath(componentID)
	if err != nil {
		return fmt.Errorf("error resolving local state path for %s: %w", componentID, err)
	}
	if err := s.shims.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing local state file %s: %w", statePath, err)
	}
	if err := s.shims.Remove(statePath + ".backup"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing local state backup %s: %w", statePath, err)
	}
	return nil
}

// ListLocalStateComponentIDs returns every componentID with local Terraform state on disk,
// whether or not it is still declared in the active blueprint. Thin passthrough:
// TerraformProvider owns the on-disk .tfstate layout.
func (s *TerraformStack) ListLocalStateComponentIDs() ([]string, error) {
	return s.runtime.TerraformProvider.ListLocalStateComponentIDs()
}

// MigrateState runs `terraform init -migrate-state -force-copy` per component to move state
// to the currently configured backend. Components whose directories don't exist on disk are
// skipped and their IDs returned in the skipped slice, paired with any error so callers see
// both what migrated and what didn't. Stops on the first failure; safe to retry.
func (s *TerraformStack) MigrateState(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) ([]string, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current directory: %v", err)
	}
	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil, fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	var skipped []string
	err = presenter.Track(ctx, "terraform:migrate-state", "Migrating terraform state", func(context.Context) error {
		for _, component := range components {
			migrated, err := s.migrateOneComponent(&component, &backendOverridePaths)
			if err != nil {
				return err
			}
			if !migrated {
				skipped = append(skipped, component.GetID())
			}
		}
		return nil
	})
	return skipped, err
}

// MigrateComponentState runs `terraform init -migrate-state -force-copy` for a single
// component identified by componentID, moving its state to the currently configured
// backend. Used by `windsor bootstrap` after applying just the backend component with a
// local backend, so only that component's state is moved to remote (the rest haven't
// been applied yet and will init directly against the configured backend on the next
// Up). Returns an error if the blueprint is nil, the component is not found, the
// component's directory does not exist, or any terraform operation fails.
func (s *TerraformStack) MigrateComponentState(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}
	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	var component *blueprintv1alpha1.TerraformComponent
	for i := range components {
		if components[i].GetID() == componentID {
			component = &components[i]
			break
		}
	}
	if component == nil {
		return fmt.Errorf("terraform component %q not found", componentID)
	}

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	// No progress UI here intentionally: MigrateComponentState runs as a hidden
	// step inside the per-component bootstrap/destroy dance for s3/azurerm. The
	// operator already saw "Applying <component>" before this fires, and adding
	// "Migrating terraform state for <component>" between two applies of the
	// same component reads as redundant noise. Bulk MigrateState (the
	// kubernetes full-cycle path) keeps its progress UI because it's the only
	// visible step covering many components at once.
	migrated, err := s.migrateOneComponent(component, &backendOverridePaths)
	if err != nil {
		return err
	}
	if !migrated {
		return fmt.Errorf("component %q directory %s does not exist", componentID, component.FullPath)
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// migrateOneComponent runs `terraform init -migrate-state -force-copy` for a single
// component, registering any generated backend_override.tf for cleanup via
// backendOverridePaths. Returns (false, nil) when the component's directory does not
// exist (so callers can decide whether absence is an error or a skip), (true, nil) on
// success, and (false, err) on any other failure. Shared by MigrateState (which
// tolerates missing dirs by collecting skipped IDs) and MigrateComponentState (which
// treats a missing dir as an error because the caller is asking for a specific
// component to be migrated). A failed init matching isStaleProviderLockError triggers
// one refreshProviderLock call, then one retry, so a normal migration stays offline.
func (s *TerraformStack) migrateOneComponent(component *blueprintv1alpha1.TerraformComponent, backendOverridePaths *[]string) (bool, error) {
	if _, statErr := s.shims.Stat(component.FullPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, fmt.Errorf("error checking component directory %s: %w", component.FullPath, statErr)
	}

	terraformVars, scopedKeys, terraformArgs, err := s.setupTerraformEnvironment(*component)
	backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
	if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
		*backendOverridePaths = append(*backendOverridePaths, backendOverridePath)
	}
	if err != nil {
		return false, err
	}

	migrateErr := s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, "-migrate-state", "-force-copy")
	if migrateErr != nil && isStaleProviderLockError(migrateErr) {
		if refreshErr := s.refreshProviderLock(component, terraformVars, scopedKeys); refreshErr != nil {
			return false, refreshErr
		}
		migrateErr = s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, "-migrate-state", "-force-copy")
	}
	if migrateErr != nil {
		return false, migrateErr
	}
	return true, nil
}

// hasResources reports whether this module or any descendant contains a resource in state.
func (m tfStateModule) hasResources() bool {
	if len(m.Resources) > 0 {
		return true
	}
	for _, child := range m.ChildModules {
		if child.hasResources() {
			return true
		}
	}
	return false
}

// hasStateResources reports whether the component's state contains any resources at any
// depth in the module tree. Used by destroy to short-circuit already-destroyed components,
// and by plan to classify never-applied components as IsNew (so plan is skipped rather than
// running against empty state, which fails when dependent layers reference upstream state).
func (s *TerraformStack) hasStateResources(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string) (bool, error) {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	stateShowArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "show", "-json"}
	stateJSON, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, selectTerraformCommandEnv(terraformVars, true, scopedKeys), stateShowArgs...)
	if err != nil {
		return false, fmt.Errorf("error reading terraform state JSON for %s: %w", component.Path, err)
	}
	var state tfState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return false, fmt.Errorf("error parsing terraform state JSON for %s: %w", component.Path, err)
	}
	if state.Values == nil {
		return false, nil
	}
	return state.Values.RootModule.hasResources(), nil
}

// refreshIfStateNonEmpty runs refresh only when the component's state contains resources,
// matching the §4.4 unified refresh policy: if state is empty there is nothing to reconcile,
// so refresh is skipped (and the subsequent plan will surface any deeper config issue with
// a clearer error than refresh would). When state is non-empty and refresh fails, the error
// is reported with presenter.Warn and swallowed — the caller's plan/apply will run anyway, and
// any persistent problem will resurface there with a more actionable message. The state-check
// itself failing is propagated; callers can't safely proceed without knowing whether state is
// empty. Used by Up and Apply; Destroy retains its own refresh handling because its fallback
// (-refresh=true on destroy) is destroy-specific.
func (s *TerraformStack) refreshIfStateNonEmpty(ctx context.Context, component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs) error {
	hasResources, err := s.hasStateResources(component, terraformVars, scopedKeys)
	if err != nil {
		return err
	}
	if !hasResources {
		return nil
	}
	if err := s.refreshComponentState(component, terraformVars, scopedKeys, terraformArgs, 0); err != nil {
		presenter.Warn(ctx, fmt.Sprintf("terraform refresh failed for %s; continuing with plan against the last-known state (plan runs with -refresh=false; any persistent state divergence will surface as a plan error): %v", component.Path, err))
	}
	return nil
}

// refreshComponentState runs `terraform refresh` to reconcile state with cloud reality.
// A timeout wraps shell.ErrCommandTimedOut. A zero timeout runs unbounded. See
// refreshBeforeDestroy for how destroy callers bound and classify the result.
func (s *TerraformStack) refreshComponentState(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs, timeout time.Duration) error {
	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	refreshArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "refresh"}
	refreshArgs = append(refreshArgs, terraformArgs.RefreshArgs...)
	refreshEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
	var err error
	if timeout > 0 {
		_, err = s.runtime.Shell.ExecSilentWithEnvAndTimeout(terraformCommand, refreshEnv, refreshArgs, timeout)
	} else {
		_, err = s.runtime.Shell.ExecSilentWithEnv(terraformCommand, refreshEnv, refreshArgs...)
	}
	if err != nil {
		return fmt.Errorf("error refreshing terraform state for %s: %w", component.Path, err)
	}
	return nil
}
