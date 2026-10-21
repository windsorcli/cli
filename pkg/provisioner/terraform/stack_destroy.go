// The destroy functions of the TerraformStack destroy the terraform components of a blueprint.
// It provides DestroyAll and Destroy, and retries a destroy that fails.
// It warns about prevent_destroy resources, and about drift in the attributes that change how a provider deletes a resource.

package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// destroyBehaviorAttributes are the resource attributes known to change how a provider
// performs a delete. plan -destroy never re-diffs these against config, since the resource
// is being removed, not updated. A stale value can then persist silently through destroy
// until an intervening plan or apply reconciles it — see cli#3328. This list stays small and
// curated on purpose. General attribute drift is common and mostly harmless to an imminent
// destroy; flagging all of it would make the warning too noisy to trust.
var destroyBehaviorAttributes = []string{
	"deletion_policy",
	"deletion_protection",
	"force_destroy",
	"skip_final_snapshot",
}

// =============================================================================
// Types
// =============================================================================

// DestroyAttributeDrift names one destroy-behavior attribute whose config value disagrees
// with what terraform has recorded in state for a resource. State is the value plan -destroy
// will actually use; Config is what the current .tf declares.
type DestroyAttributeDrift struct {
	Address   string
	Attribute string
	State     string
	Config    string
}

// DestroyOutcome tallies what a single terraform destroy pass produced.
// Destroyed lists component IDs whose terraform destroy ran to completion;
// Skipped lists component IDs whose state was empty (no-op); Failed lists
// per-component failures collected when continue-on-error is true. Aggregate
// cross-layer concerns (terraform deferral, kustomize counts) live on the
// provisioner-layer result type, not here.
type DestroyOutcome struct {
	Destroyed []string
	Skipped   []string
	Failed    []ComponentFailure
}

// =============================================================================
// Public Methods
// =============================================================================

// DestroyAll destroys components in reverse dependency order. For each component, it runs
// init, checks state, refreshes, checks state again, then destroys, skipping the rest of
// the flow once state is empty.
//
//   - Destroy=false: skips the component, but still clears its backend pointer.
//   - excludeIDs: skips the component entirely. The cmd layer uses this to destroy the
//     backend component last, after migrating its state to local.
//   - continueOnError=true: collects each error in DestroyOutcome.Failed and continues.
//   - continueOnError=false: stops at the first error and returns a partial DestroyOutcome.
//
// Each destroy is bounded by destroyIdleTimeout/destroyAbsoluteTimeout and retried via
// execTerraformDestroyWithRetry; each refresh by refreshBeforeDestroy.
func (s *TerraformStack) DestroyAll(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, continueOnError bool, excludeIDs ...string) (DestroyOutcome, error) {
	var result DestroyOutcome
	if blueprint == nil {
		return result, fmt.Errorf("blueprint not provided")
	}

	currentDir, err := s.shims.Getwd()
	if err != nil {
		return result, werror.Wrap(err, "error getting current directory")
	}

	defer func() {
		_ = s.shims.Chdir(currentDir)
	}()

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return result, fmt.Errorf("error getting project root: project root is empty")
	}
	components := s.resolveTerraformComponents(blueprint, projectRoot)

	excluded := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}

	var backendOverridePaths []string
	defer func() {
		for _, path := range backendOverridePaths {
			_ = s.shims.Remove(path)
		}
	}()

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]

		if excluded[component.GetID()] {
			continue
		}

		if component.Destroy != nil {
			destroy := component.Destroy.ToBool()
			if destroy != nil && !*destroy {
				if terraformVars, _, _, err := s.runtime.TerraformProvider.GetEnvVars(component.GetID(), false); err == nil {
					s.clearBackendPointer(terraformVars)
				}
				continue
			}
		}

		if _, err := s.shims.Stat(component.FullPath); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, component.GetID())
			continue
		}

		terraformVars, scopedKeys, terraformArgs, err := s.setupTerraformEnvironment(component)
		backendOverridePath := filepath.Join(component.FullPath, "backend_override.tf")
		if _, statErr := s.shims.Stat(backendOverridePath); statErr == nil {
			backendOverridePaths = append(backendOverridePaths, backendOverridePath)
		}
		if err != nil {
			if continueOnError {
				result.Failed = append(result.Failed, ComponentFailure{ID: component.GetID(), Err: err})
				continue
			}
			return result, err
		}

		componentSkipped := false
		if err := presenter.Track(ctx, "terraform:"+component.GetID(), fmt.Sprintf("Destroying %s", component.Path), func(ctx context.Context) error {
			terraformVars["TF_VAR_operation"] = "destroy"

			if err := s.runTerraformInit(&component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
				return err
			}

			// Skip refresh if state is already empty: refresh can only drop resources, never
			// add them, and downstream data sources may fail against torn-down upstreams.
			hasResourcesPre, err := s.hasStateResources(&component, terraformVars, scopedKeys)
			if err != nil {
				return err
			}
			if !hasResourcesPre {
				componentSkipped = true
				return nil
			}

			refreshFailed, err := s.refreshBeforeDestroy(ctx, &component, terraformVars, scopedKeys, terraformArgs)
			if err != nil {
				return err
			}

			// Skip the post-refresh empty-state check when refresh failed — the state we would
			// be reading is the pre-refresh snapshot we already classified as non-empty, so the
			// check would be redundant; if refresh succeeded but reconciliation dropped every
			// resource, we still want the skip path.
			if !refreshFailed {
				hasResources, err := s.hasStateResources(&component, terraformVars, scopedKeys)
				if err != nil {
					return err
				}
				if !hasResources {
					componentSkipped = true
					return nil
				}
			}

			terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()

			// -refresh=false on the happy path because refreshComponentState already ran;
			// -refresh=true on the fallback path so terraform retries refresh inside destroy.
			destroyRefreshFlag := "-refresh=false"
			if refreshFailed {
				destroyRefreshFlag = "-refresh=true"
			}
			destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy", destroyRefreshFlag}
			destroyArgs = append(destroyArgs, noColorArgs()...)
			destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
			destroyEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
			output, err := s.execTerraformDestroyWithRetry(ctx, component.Path, terraformCommand, destroyEnv, destroyArgs)
			if err != nil {
				if trimmed := strings.TrimSpace(output); trimmed != "" {
					presenter.Warn(ctx, fmt.Sprintf("terraform destroy output for %s:\n%s", component.Path, trimmed))
				}
				return werror.Wrap(err, "error running terraform destroy for %s", component.Path)
			}
			return nil
		}); err != nil {
			if continueOnError {
				result.Failed = append(result.Failed, ComponentFailure{ID: component.GetID(), Err: err})
				s.clearBackendPointer(terraformVars)
				continue
			}
			return result, err
		}
		if componentSkipped {
			result.Skipped = append(result.Skipped, component.GetID())
		} else {
			result.Destroyed = append(result.Destroyed, component.GetID())
		}
		s.clearBackendPointer(terraformVars)
	}

	return result, nil
}

// Destroy tears down a single component idempotently: init → pre-refresh state check →
// refresh → post-refresh state check → destroy, skipping the rest when state is empty at
// either check. Returns (true, nil) when skipped, (false, nil) on success, (false, err) on
// failure. Destroy-mode config (S3 force_destroy, etc.) is honored via TF_VAR_operation;
// no prep-apply is run because that could recreate resources refresh just dropped.
//
// The whole sequence is wrapped in presenter.Track("Destroying <path>") so the visible
// label matches the bulk DestroyAll loop body — mixing this method into the per-component
// destroy dance (which calls bulk DestroyAll for non-backend components and this method
// for backend) would otherwise produce inconsistent "Destroying X" / "Destroying terraform
// for X" lines side by side. The terraform destroy exec runs silently inside the spinner
// for the same reason: the bulk loop is silent, so single-component Destroy must be too.
// Destroy is bounded by destroyIdleTimeout/destroyAbsoluteTimeout and retried via
// execTerraformDestroyWithRetry; refresh by refreshBeforeDestroy.
func (s *TerraformStack) Destroy(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
	if blueprint == nil {
		return false, fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return false, fmt.Errorf("component ID not provided")
	}

	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return false, err
	}
	defer cleanup()

	terraformVars["TF_VAR_operation"] = "destroy"

	skipped := false
	if err := presenter.Track(ctx, "terraform:"+component.GetID(), fmt.Sprintf("Destroying %s", component.Path), func(ctx context.Context) error {
		if err := s.runTerraformInit(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
			return err
		}

		hasResourcesPre, err := s.hasStateResources(component, terraformVars, scopedKeys)
		if err != nil {
			return err
		}
		if !hasResourcesPre {
			skipped = true
			return nil
		}

		refreshFailed, err := s.refreshBeforeDestroy(ctx, component, terraformVars, scopedKeys, terraformArgs)
		if err != nil {
			return err
		}

		if !refreshFailed {
			hasResources, err := s.hasStateResources(component, terraformVars, scopedKeys)
			if err != nil {
				return err
			}
			if !hasResources {
				skipped = true
				return nil
			}
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		destroyRefreshFlag := "-refresh=false"
		if refreshFailed {
			destroyRefreshFlag = "-refresh=true"
		}
		destroyArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "destroy", destroyRefreshFlag}
		destroyArgs = append(destroyArgs, noColorArgs()...)
		destroyArgs = append(destroyArgs, terraformArgs.DestroyArgs...)
		destroyEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)

		output, err := s.execTerraformDestroyWithRetry(ctx, component.Path, terraformCommand, destroyEnv, destroyArgs)
		if err != nil {
			if trimmed := strings.TrimSpace(output); trimmed != "" {
				presenter.Warn(ctx, fmt.Sprintf("terraform destroy output for %s:\n%s", component.Path, trimmed))
			}
			return werror.Wrap(err, "error running terraform destroy for %s", component.Path)
		}
		return nil
	}); err != nil {
		return false, err
	}
	s.clearBackendPointer(terraformVars)

	return skipped, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// execTerraformDestroyWithRetry runs `terraform destroy` for one component, retrying up to
// destroyRetryAttempts times with destroyRetryBackoff between attempts to absorb a transient
// async cloud-side dependency. Each failed non-final attempt is reported with presenter.Warn before
// retrying, so a failure reason that changes between attempts stays visible. Every attempt is
// bounded by destroyIdleTimeout and destroyAbsoluteTimeout, not a flat wall clock; a timeout
// (shell.ErrCommandTimedOut) fails immediately without retrying.
//
// A timeout interrupts terraform before it kills the process. See
// ExecSilentWithEnvAndIdleTimeout. A hard kill on timeout gave terraform no chance to write
// a state checkpoint or release its backend lock. Returns the last attempt's output and error.
func (s *TerraformStack) execTerraformDestroyWithRetry(ctx context.Context, componentPath, terraformCommand string, destroyEnv map[string]string, destroyArgs []string) (string, error) {
	var output string
	var err error
	for attempt := 1; attempt <= s.destroyRetryAttempts; attempt++ {
		output, err = s.runtime.Shell.ExecSilentWithEnvAndIdleTimeout(terraformCommand, destroyEnv, destroyArgs, s.destroyIdleTimeout, s.destroyAbsoluteTimeout, s.destroyGracePeriod)
		if err == nil {
			return output, nil
		}
		if errors.Is(err, shell.ErrCommandTimedOut) {
			return output, err
		}
		if attempt < s.destroyRetryAttempts {
			if trimmed := strings.TrimSpace(output); trimmed != "" {
				presenter.Warn(ctx, fmt.Sprintf("terraform destroy attempt %d/%d for %s failed, retrying:\n%s", attempt, s.destroyRetryAttempts, componentPath, trimmed))
			} else {
				presenter.Warn(ctx, fmt.Sprintf("terraform destroy attempt %d/%d for %s failed, retrying: %v", attempt, s.destroyRetryAttempts, componentPath, err))
			}
			s.shims.TimeSleep(s.destroyRetryBackoff)
		}
	}
	return output, err
}

// refreshBeforeDestroy runs refreshComponentState with the constants.DefaultTerraformRefreshTimeout limit.
// A timeout means that the provider is unreachable. It returns an error with a remediation hint,
// and the caller stops the destroy of this component. Another failure can be temporary. It returns
// refreshFailed=true with no error, and the caller runs terraform destroy -refresh=true.
func (s *TerraformStack) refreshBeforeDestroy(ctx context.Context, component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs) (bool, error) {
	err := s.refreshComponentState(component, terraformVars, scopedKeys, terraformArgs, constants.DefaultTerraformRefreshTimeout)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, shell.ErrCommandTimedOut) {
		return false, werror.New(fmt.Sprintf("terraform refresh for %s timed out after %s; its provider looks unreachable", component.Path, constants.DefaultTerraformRefreshTimeout), "Make sure that the provider endpoint of the component is reachable, then run the command again.", err)
	}
	presenter.Warn(ctx, fmt.Sprintf("terraform refresh failed for %s; falling through to destroy -refresh=true (terraform will retry refresh during destroy): %v", component.Path, err))
	return true, nil
}

// destroyBehaviorDrift runs a regular (non-destroy) plan for the component and returns any
// destroy-behavior attribute (see destroyBehaviorAttributes) whose config value disagrees
// with state. -refresh=false skips a live cloud refresh, so this stays a fast, local diff
// against whatever state is already on disk. The plan is saved to its own scratch file, not
// the component's shared apply-mode plan file, and removed afterward. Best-effort: any
// failure returns nil rather than surfacing a new error, since this is a supplementary
// check, not a requirement for the destroy plan itself.
func (s *TerraformStack) destroyBehaviorDrift(component *blueprintv1alpha1.TerraformComponent, terraformVars map[string]string, scopedKeys []string, terraformArgs *envvars.TerraformArgs) []DestroyAttributeDrift {
	sharedPlanFile := planOutputPath(terraformArgs.PlanArgs)
	if sharedPlanFile == "" {
		return nil
	}
	driftPlanFile := path.Join(path.Dir(sharedPlanFile), destroyBehaviorDriftPlanFile)
	defer func() { _ = s.shims.Remove(driftPlanFile) }()

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)

	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-refresh=false", "-no-color", fmt.Sprintf("-out=%s", driftPlanFile)}
	planArgs = append(planArgs, planArgsWithoutOut(terraformArgs.PlanArgs)...)
	if _, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, planArgs...); err != nil {
		return nil
	}

	showArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "show", "-json", driftPlanFile}
	planJSON, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, showArgs...)
	if err != nil {
		return nil
	}

	return parseDestroyBehaviorDrift(planJSON)
}

// =============================================================================
// Helpers
// =============================================================================

// destroyBehaviorDriftPlanFile is the scratch plan file destroyBehaviorDrift saves its own
// regular plan to, in the same TF_DATA_DIR the component's real plan/apply artifacts live
// in. It is deliberately never the shared apply-mode plan file (terraformArgs.PlanArgs' own
// -out=): `windsor env` points a manually-run `terraform apply` at that same file via
// TF_CLI_ARGS_apply, so overwriting it here — even when the destroy is later declined —
// would leave a stale, unreviewed create/update plan for a later bare `terraform apply` to
// silently consume.
const destroyBehaviorDriftPlanFile = "windsor-destroy-drift.tfplan"

// filterDriftToDestroyed keeps only the drift entries whose address this destroy plan will
// actually delete, so a drifted attribute on a resource this run leaves untouched is never
// reported as a destroy-time risk.
func filterDriftToDestroyed(drifted []DestroyAttributeDrift, resources []ResourceChange) []DestroyAttributeDrift {
	destroyed := make(map[string]bool, len(resources))
	for _, r := range resources {
		if r.Action == ActionDelete {
			destroyed[r.Address] = true
		}
	}
	var filtered []DestroyAttributeDrift
	for _, d := range drifted {
		if destroyed[d.Address] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// componentDestroyEnabled reports whether a component should be included in a
// destroy plan. A component is included unless its Destroy field is set and
// resolves to false; absent or true means "destroy normally." This mirrors the
// gate inside DestroyAll so the planned set matches the executed set.
func componentDestroyEnabled(component *blueprintv1alpha1.TerraformComponent) bool {
	if component.Destroy == nil {
		return true
	}
	d := component.Destroy.ToBool()
	if d == nil {
		return true
	}
	return *d
}

// parseDestroyBehaviorDrift extracts destroy-behavior attribute drift (see
// destroyBehaviorAttributes) from the structured JSON `terraform show -json <planfile>`
// emits for a regular plan. Only resource_changes carrying an "update" action are
// considered — those are exactly the resources whose config disagrees with state.
// Malformed or unrecognized input yields no drift rather than an error, consistent with
// destroyBehaviorDrift's best-effort contract.
func parseDestroyBehaviorDrift(output string) []DestroyAttributeDrift {
	var plan struct {
		ResourceChanges []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string        `json:"actions"`
				Before  json.RawMessage `json:"before"`
				After   json.RawMessage `json:"after"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal([]byte(output), &plan); err != nil {
		return nil
	}

	var drifted []DestroyAttributeDrift
	for _, rc := range plan.ResourceChanges {
		if !slices.Contains(rc.Change.Actions, "update") {
			continue
		}
		var before, after map[string]any
		if json.Unmarshal(rc.Change.Before, &before) != nil || json.Unmarshal(rc.Change.After, &after) != nil {
			continue
		}
		for _, attr := range destroyBehaviorAttributes {
			beforeVal, hasBefore := before[attr]
			afterVal, hasAfter := after[attr]
			if !hasBefore || !hasAfter || fmt.Sprint(beforeVal) == fmt.Sprint(afterVal) {
				continue
			}
			drifted = append(drifted, DestroyAttributeDrift{
				Address:   stripModuleMain(rc.Address),
				Attribute: attr,
				State:     formatDriftValue(beforeVal),
				Config:    formatDriftValue(afterVal),
			})
		}
	}
	return drifted
}

// formatDriftValue renders a decoded JSON attribute value for a drift warning: quoted for a
// string, "null" for an absent value, and Go's default format otherwise (bool, number).
func formatDriftValue(v any) string {
	if v == nil {
		return "null"
	}
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// preventDestroySummary is the diagnostic summary terraform emits verbatim for
// a resource protected by `lifecycle { prevent_destroy = true }`.
const preventDestroySummary = "Instance cannot be destroyed"

// preventDestroyAddressesFromDiagnostics returns the addresses of resources
// protected by `lifecycle { prevent_destroy = true }`, in order. Falls back to
// parsing detail when address is absent.
func preventDestroyAddressesFromDiagnostics(diags []planDiagnostic) []string {
	var addresses []string
	for _, d := range diags {
		if d.Severity != "error" || d.Summary != preventDestroySummary {
			continue
		}
		addr := d.Address
		if addr == "" {
			addr = preventDestroyAddressFromDetail(d.Detail)
		}
		if addr == "" {
			continue
		}
		addresses = append(addresses, stripModuleMain(addr))
	}
	return addresses
}

// extractPreventDestroyAddresses runs preventDestroyAddressesFromDiagnostics
// over a raw -json plan stream. See that function for the extraction rules.
func extractPreventDestroyAddresses(output string) []string {
	return preventDestroyAddressesFromDiagnostics(planDiagnostics(output))
}

// preventDestroyAddressFromDetail extracts the resource address from a
// "Instance cannot be destroyed" diagnostic detail. Terraform's detail string
// always opens with "Resource <addr> has lifecycle.prevent_destroy set"; we
// pull the substring between those markers. Returns "" when the pattern
// doesn't match, so the caller can skip the malformed event.
func preventDestroyAddressFromDetail(detail string) string {
	const prefix = "Resource "
	const suffix = " has lifecycle.prevent_destroy set"
	if !strings.HasPrefix(detail, prefix) {
		return ""
	}
	end := strings.Index(detail, suffix)
	if end < len(prefix) {
		return ""
	}
	return detail[len(prefix):end]
}
