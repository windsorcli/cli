// The plan functions of the TerraformStack run terraform plan for the components of a blueprint.
// It provides the plan output, the JSON plan, and the plan summaries for apply and destroy.
// The helpers parse the plan JSON and the plan diagnostics.

package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

const (
	ActionUnknown Action = iota
	ActionCreate
	ActionUpdate
	ActionDelete
	ActionReplace
)

// =============================================================================
// Types
// =============================================================================

// planDiagnostic mirrors one `diagnostic` event from a terraform -json plan stream.
type planDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Address  string `json:"address"`
	Detail   string `json:"detail"`
}

// TerraformComponentPlan holds the plan result for a single Terraform component.
// ComponentID is the unique identifier (Name when set, else Path). Path carries the
// component's blueprint Path field (e.g., "cluster/aws-eks") so renderers can show
// the underlying module location alongside or instead of the short ID. Add, Change,
// and Destroy reflect terraform's "to add / to change / to destroy" counts.
// NoChanges is true when terraform reports no changes. IsNew is true when no state
// exists for the component in the configured backend — the component has never been
// applied. Resources is the per-resource change list extracted from the plan output;
// it is empty when IsNew is true, when the plan errored, or when terraform reports
// no changes. Err is non-nil when the component's init or plan step failed;
// subsequent layers may still be attempted.
//
// IsNew supersedes the count fields: when IsNew is true, terraform plan is not
// executed (it would either misreport "all creates" or fail reading dependent
// upstream state), so Add/Change/Destroy are zero and NoChanges is false. JSON
// consumers detecting "pending work" must check IsNew alongside the counts —
// `IsNew || Add+Change+Destroy > 0` rather than counts alone.
//
// IsNew has two operator-facing meanings depending on which producer populated
// the struct, and the struct itself does not carry a mode flag — callers route
// this disambiguation. Apply-side producers (PlanSummary,
// PlanComponentSummary) set IsNew when the component has never been applied,
// rendered as "(new)". Destroy-side producers (PlanDestroySummary,
// PlanDestroyComponentSummary) set IsNew when there is no state to destroy,
// rendered as "(no state)". Renderers must therefore route apply and destroy
// results through distinct formatters — calling the apply-side formatter on a
// destroy-side result would emit "(new)" for a component that has nothing to
// tear down, which is misleading.
type TerraformComponentPlan struct {
	ComponentID string
	Path        string
	Add         int
	Change      int
	Destroy     int
	NoChanges   bool
	IsNew       bool
	Resources   []ResourceChange
	// Protected lists resource addresses that terraform reports cannot be
	// destroyed because their config sets `lifecycle { prevent_destroy = true }`.
	// Populated only on destroy-plan paths; nil on apply-plan paths.
	// Informational — cmd/destroy.go surfaces these as a stderr warning so the
	// operator knows terraform will halt the destroy partway. The wrapper does
	// not refuse: half-completed destroys orphan more state than letting the
	// operator see the addresses and decide. Module authors who want a hard
	// gate should consume TF_VAR_operation / TF_VAR_ephemeral and design the
	// destroy contract in HCL.
	Protected []string
	// DriftedAttributes lists destroy-behavior attributes (see destroyBehaviorAttributes)
	// whose config value disagrees with state on a resource this plan will destroy.
	// plan -destroy never re-diffs these against config, so a stale state value would
	// otherwise persist silently through destroy. Populated only on destroy-plan paths.
	// Best-effort: a failure detecting drift never fails the plan, so this can be nil
	// even when drift exists.
	DriftedAttributes []DestroyAttributeDrift
	Err               error
}

// Action enumerates the kinds of changes a plan can produce for a single resource.
// ActionUnknown is reserved for inputs that don't correspond to any operator-visible
// change (terraform "noop"/"read" or unrecognised action strings) and is dropped
// from rendered resource lists.
type Action int

// ResourceChange identifies one resource changed by a plan along with its action.
// Address is the terraform resource address with the leading "module.main." wrapper
// stripped, since every Windsor component wraps its resources in a single module
// and the prefix would otherwise be repeated noise on every line.
type ResourceChange struct {
	Address string
	Action  Action
}

// =============================================================================
// Public Methods
// =============================================================================

// Plan runs terraform init and plan for a single component identified by componentID.
// It resolves the component from the blueprint, sets up the environment, and executes
// init then plan without applying any changes. Returns an error if the component is not
// found, the directory does not exist, or any terraform operation fails.
func (s *TerraformStack) Plan(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInitReadOnly(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
	planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		return werror.Wrap(err, "error running terraform plan for %s", component.Path)
	}
	if planOutput != "" {
		fmt.Fprint(os.Stdout, planOutput)
	}

	return nil
}

// PlanAll runs terraform init and plan for every enabled component in the blueprint,
// streaming output directly to stdout. Stops on the first error. Returns an error if
// blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAll(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, false)
}

// PlanJSON runs terraform init and plan -json for a single component identified by componentID,
// streaming machine-readable JSON lines to stdout. Returns an error if the component is not
// found, the directory does not exist, or any terraform operation fails.
func (s *TerraformStack) PlanJSON(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}
	if componentID == "" {
		return fmt.Errorf("component ID not provided")
	}

	component, terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentOp(blueprint, componentID)
	if err != nil {
		return err
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInitReadOnly(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		return err
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-json", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
	planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		wrapped := werror.Wrap(err, "error running terraform plan for %s", component.Path)
		return wrapPlanError(wrapped, extractPlanErrorDiagnostics(planOutput))
	}
	if planOutput != "" {
		fmt.Fprint(os.Stdout, planOutput)
	}

	return nil
}

// PlanAllJSON runs terraform init and plan -json for every enabled component in the blueprint,
// streaming machine-readable JSON lines to stdout. Stops on the first error. Returns an error
// if blueprint is nil or any component's init or plan step fails.
func (s *TerraformStack) PlanAllJSON(blueprint *blueprintv1alpha1.Blueprint) error {
	return s.planComponents(blueprint, true)
}

// PlanSummary runs terraform init and plan for every enabled component in the blueprint,
// capturing output to parse add/change/destroy counts rather than printing them.
// Errors are recorded per-component; the summary continues even if a component fails,
// so callers receive partial results for independent layers. Returns nil if blueprint is nil.
func (s *TerraformStack) PlanSummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan {
	if blueprint == nil {
		return nil
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	results := make([]TerraformComponentPlan, 0, len(components))
	for i := range components {
		results = append(results, s.planOneTerraformSummary(&components[i]))
	}
	return results
}

// PlanDestroySummary runs terraform init and `plan -destroy` for every component
// in the blueprint that is not pinned with Destroy=false, capturing the destroy
// counts and the per-resource list rather than printing them. Components with
// Destroy=false are filtered out so the rendered plan matches what Teardown
// would actually destroy — including them would lie. As with PlanSummary,
// errors are recorded per-component and the summary continues so callers
// receive partial results for independent layers. Returns nil if blueprint is
// nil or projectRoot is unset.
func (s *TerraformStack) PlanDestroySummary(blueprint *blueprintv1alpha1.Blueprint) []TerraformComponentPlan {
	if blueprint == nil {
		return nil
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return nil
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)
	results := make([]TerraformComponentPlan, 0, len(components))
	for i := range components {
		if !componentDestroyEnabled(&components[i]) {
			continue
		}
		results = append(results, s.planOneTerraformDestroySummary(&components[i]))
	}
	return results
}

// PlanDestroyComponentSummary runs terraform init and plan -destroy for one component.
// It returns the plan result of the component, and puts a failure in the Err field of the result.
// A component that is not in the blueprint sets Err with a remediation hint. A component with
// destroy set to false also sets Err with a hint, because Teardown skips that component.
func (s *TerraformStack) PlanDestroyComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: componentID}

	if blueprint == nil {
		result.Err = fmt.Errorf("blueprint not provided")
		return result
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		result.Err = fmt.Errorf("error getting project root: project root is empty")
		return result
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
		result.Err = werror.New(fmt.Sprintf("terraform component %q not found in blueprint", componentID), componentNotFoundHint, nil)
		return result
	}
	if !componentDestroyEnabled(component) {
		result.Err = werror.New(fmt.Sprintf("terraform component %q is pinned with destroy=false", componentID), "Set destroy to true on the component in the blueprint to allow its destroy.", nil)
		return result
	}

	return s.planOneTerraformDestroySummary(component)
}

// PlanComponentSummary runs terraform init and plan for one component and returns its plan result.
// It resolves only that component from the blueprint, so it plans no other component.
// It puts a failure in the Err field of the result, as PlanSummary does. A component that
// is not in the blueprint sets Err with a remediation hint.
func (s *TerraformStack) PlanComponentSummary(blueprint *blueprintv1alpha1.Blueprint, componentID string) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: componentID}

	if blueprint == nil {
		result.Err = fmt.Errorf("blueprint not provided")
		return result
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		result.Err = fmt.Errorf("error getting project root: project root is empty")
		return result
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
		result.Err = werror.New(fmt.Sprintf("terraform component %q not found in blueprint", componentID), componentNotFoundHint, nil)
		return result
	}

	return s.planOneTerraformSummary(component)
}

// =============================================================================
// Private Methods
// =============================================================================

// planComponents runs terraform init and plan for every enabled component in the blueprint.
// When jsonMode is true, -json and -no-color are appended to the plan args so that output
// is machine-readable JSON lines; otherwise human-readable output is streamed. Stops on
// the first error. Returns an error if blueprint is nil or any component's init or plan fails.
func (s *TerraformStack) planComponents(blueprint *blueprintv1alpha1.Blueprint, jsonMode bool) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	projectRoot := s.runtime.ProjectRoot
	if projectRoot == "" {
		return fmt.Errorf("error getting project root: project root is empty")
	}

	components := s.resolveTerraformComponents(blueprint, projectRoot)

	for i := range components {
		component := &components[i]

		s.printComponentHeader(component.Path)

		terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
		if err != nil {
			return err
		}
		terraformVars["TF_VAR_operation"] = "apply"

		if err := s.runTerraformInitReadOnly(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
			cleanup()
			return err
		}

		terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
		planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan"}
		if jsonMode {
			planArgs = append(planArgs, "-json", "-no-color")
		}
		planArgs = append(planArgs, terraformArgs.PlanArgs...)
		planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
		planOutput, err := s.runtime.Shell.ExecSilentWithEnv(terraformCommand, planEnv, planArgs...)
		cleanup()
		if err != nil {
			wrapped := werror.Wrap(err, "error running terraform plan for %s", component.Path)
			return wrapPlanError(wrapped, extractPlanErrorDiagnostics(planOutput))
		}
		if planOutput != "" {
			fmt.Fprint(os.Stdout, planOutput)
		}
	}

	return nil
}

// planOneTerraformSummary runs terraform init and plan -no-color for a single component
// and returns its structured result. It is shared by PlanSummary and PlanComponentSummary
// to avoid duplicating the per-component setup, init, plan, and cleanup logic.
//
// Before running plan, hasStateResources is used to determine whether the configured
// backend has any state recorded for this component. Empty state means the component
// has never been applied — IsNew is set and plan is skipped, since plan would either
// produce a misleading "all creates" picture or fail when `data "terraform_remote_state"`
// references upstream layers that are also empty.
func (s *TerraformStack) planOneTerraformSummary(component *blueprintv1alpha1.TerraformComponent) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: component.GetID(), Path: component.Path}

	terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
	if err != nil {
		result.Err = err
		return result
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "apply"

	if err := s.runTerraformInitReadOnly(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		result.Err = err
		return result
	}

	hasState, err := s.hasStateResources(component, terraformVars, scopedKeys)
	if err != nil {
		result.Err = err
		return result
	}
	if !hasState {
		result.IsNew = true
		return result
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	// -json emits a machine-parseable event stream we use to extract both the
	// summary counts and the per-resource change list in one invocation; -no-color
	// is required by terraform alongside -json.
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-json", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
	planOutput, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, planArgs...)
	if err != nil {
		wrapped := werror.Wrap(err, "error running terraform plan for %s", component.Path)
		result.Err = wrapPlanError(wrapped, extractPlanErrorDiagnostics(planOutput))
		return result
	}

	result.Add, result.Change, result.Destroy, result.NoChanges, result.Resources = parseTerraformPlanJSON(planOutput)
	return result
}

// planOneTerraformDestroySummary computes the destroy plan for one component.
// Mirrors planOneTerraformSummary but invokes `plan -destroy -json` and uses
// terraform destroy-mode env (TF_VAR_operation=destroy + DestroyArgs) so the
// rendered plan reflects what Teardown will actually do, including any
// destroy-time-only provider config the component opts into.
//
// IsNew is repurposed for the destroy context: a component with no state has
// nothing to destroy, so we set IsNew=true and skip plan. Callers MUST route
// this output through a destroy-aware formatter that renders IsNew as
// "(no state)" rather than "(new)". The TerraformComponentPlan struct does
// not carry a mode flag, so passing a destroy-side result through the
// apply-side formatter would emit "(new)" for a component that has nothing to
// tear down — see the IsNew contract on TerraformComponentPlan for the full
// disambiguation rule.
func (s *TerraformStack) planOneTerraformDestroySummary(component *blueprintv1alpha1.TerraformComponent) TerraformComponentPlan {
	result := TerraformComponentPlan{ComponentID: component.GetID(), Path: component.Path}

	terraformVars, scopedKeys, terraformArgs, cleanup, err := s.prepareComponentEnv(component)
	if err != nil {
		result.Err = err
		return result
	}
	defer cleanup()
	terraformVars["TF_VAR_operation"] = "destroy"

	if err := s.runTerraformInitReadOnly(component, terraformVars, scopedKeys, terraformArgs, defaultInitFlags...); err != nil {
		result.Err = err
		return result
	}

	hasState, err := s.hasStateResources(component, terraformVars, scopedKeys)
	if err != nil {
		result.Err = err
		return result
	}
	if !hasState {
		result.IsNew = true
		return result
	}

	terraformCommand := s.runtime.ToolsManager.GetTerraformCommand()
	planArgs := []string{fmt.Sprintf("-chdir=%s", component.FullPath), "plan", "-json", "-no-color"}
	planArgs = append(planArgs, terraformArgs.PlanDestroyArgs...)
	planEnv := selectTerraformCommandEnv(terraformVars, true, scopedKeys)
	planOutput, err := s.runtime.Shell.ExecCaptureWithEnv(terraformCommand, planEnv, planArgs...)
	if err == nil {
		result.Add, result.Change, result.Destroy, result.NoChanges, result.Resources = parseTerraformPlanJSON(planOutput)
		drifted := s.destroyBehaviorDrift(component, terraformVars, scopedKeys, terraformArgs)
		result.DriftedAttributes = filterDriftToDestroyed(drifted, result.Resources)
		return result
	}

	// A protected resource and a real error can occur in the same run; check both.
	diags := planDiagnostics(planOutput)
	protected := preventDestroyAddressesFromDiagnostics(diags)
	skip := map[string]bool{}
	if len(protected) > 0 {
		skip[preventDestroySummary] = true
	}
	otherErrors := planErrorMessages(diags, skip)

	// Parse counts even on failure, so a protected-resource warning isn't paired with "(no changes)".
	result.Add, result.Change, result.Destroy, result.NoChanges, result.Resources = parseTerraformPlanJSON(planOutput)
	result.Protected = protected
	if len(protected) > 0 && len(otherErrors) == 0 {
		return result
	}

	wrapped := werror.Wrap(err, "error running terraform plan -destroy for %s", component.Path)
	result.Err = wrapPlanError(wrapped, otherErrors)
	return result
}

// =============================================================================
// Helpers
// =============================================================================

// planArgsWithoutOut drops any -out=<path> entries, so a caller can supply its own -out=
// without producing two conflicting flags on the same command.
func planArgsWithoutOut(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-out=") {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered
}

// parseTerraformPlanJSON parses the line-delimited JSON event stream emitted by
// `terraform plan -json` and extracts both the summary counts and the per-resource
// change list. Resources whose action maps to ActionUnknown (terraform "noop" /
// "read" or unrecognised verbs) are dropped — they don't represent operator-
// visible work. noChanges is true when a change_summary event was observed and
// reported zero adds/changes/removes; absence of any change_summary is treated
// as noChanges=false so callers can distinguish "no plan output produced" from
// "plan said nothing changes". Lines that are blank, non-JSON, or fail to parse
// are silently skipped, since terraform may emit non-event diagnostics.
func parseTerraformPlanJSON(output string) (add, change, destroy int, noChanges bool, resources []ResourceChange) {
	type changeSummary struct {
		Add    int `json:"add"`
		Change int `json:"change"`
		Remove int `json:"remove"`
	}
	type resourceAddr struct {
		Addr string `json:"addr"`
	}
	type planChange struct {
		Resource resourceAddr `json:"resource"`
		Action   string       `json:"action"`
	}
	type event struct {
		Type    string         `json:"type"`
		Changes *changeSummary `json:"changes,omitempty"`
		Change  *planChange    `json:"change,omitempty"`
	}

	sawSummary := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "change_summary":
			if ev.Changes == nil {
				continue
			}
			add = ev.Changes.Add
			change = ev.Changes.Change
			destroy = ev.Changes.Remove
			sawSummary = true
		case "planned_change":
			if ev.Change == nil {
				continue
			}
			action := mapTerraformAction(ev.Change.Action)
			if action == ActionUnknown {
				continue
			}
			resources = append(resources, ResourceChange{
				Address: stripModuleMain(ev.Change.Resource.Addr),
				Action:  action,
			})
		}
	}
	if sawSummary && add == 0 && change == 0 && destroy == 0 {
		noChanges = true
	}
	return
}

// planOutputPath extracts the -out=<path> value from a set of plan arguments, or "" when
// none is present.
func planOutputPath(args []string) string {
	for _, a := range args {
		if path, ok := strings.CutPrefix(a, "-out="); ok {
			return path
		}
	}
	return ""
}

// planDiagnostics returns every diagnostic event from a `plan [-destroy]
// -json` stream, in order. Non-event lines are skipped.
func planDiagnostics(output string) []planDiagnostic {
	type event struct {
		Type       string          `json:"type"`
		Diagnostic *planDiagnostic `json:"diagnostic,omitempty"`
	}

	var diags []planDiagnostic
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type != "diagnostic" || ev.Diagnostic == nil {
			continue
		}
		diags = append(diags, *ev.Diagnostic)
	}
	return diags
}

// planErrorMessages formats each error-severity diagnostic as "summary:
// detail", skipping summaries named in skip. Flattens embedded newlines and
// drops duplicates.
func planErrorMessages(diags []planDiagnostic, skip map[string]bool) []string {
	var messages []string
	seen := make(map[string]bool, len(diags))
	for _, d := range diags {
		if d.Severity != "error" || d.Summary == "" || skip[d.Summary] {
			continue
		}
		msg := sanitizeDiagnosticLine(d.Summary)
		if d.Detail != "" {
			msg = fmt.Sprintf("%s: %s", msg, sanitizeDiagnosticLine(d.Detail))
		}
		if seen[msg] {
			continue
		}
		seen[msg] = true
		messages = append(messages, msg)
	}
	return messages
}

// sanitizeDiagnosticLine collapses a diagnostic summary or detail to one
// line, replacing embedded newlines with a space.
func sanitizeDiagnosticLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// extractPlanErrorDiagnostics returns every error diagnostic in a raw -json
// plan stream. `plan -json` reports its real error on stdout, not stderr, so
// a bare exec error carries none of it.
func extractPlanErrorDiagnostics(output string) []string {
	return planErrorMessages(planDiagnostics(output), nil)
}

// wrapPlanError appends messages to err's text, one per line. Returns err
// unchanged when messages is empty.
func wrapPlanError(err error, messages []string) error {
	if len(messages) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(messages, "\n"))
}

// mapTerraformAction maps a `terraform plan -json` change.action string to the
// shared Action enum. "noop" and "read" return ActionUnknown so callers can
// drop them from the rendered resource list. Replace variants vary across
// terraform versions; all collapse to ActionReplace.
func mapTerraformAction(s string) Action {
	switch s {
	case "create":
		return ActionCreate
	case "update":
		return ActionUpdate
	case "delete":
		return ActionDelete
	case "replace", "delete-then-create", "create-then-delete":
		return ActionReplace
	default:
		return ActionUnknown
	}
}

// stripModuleMain removes the leading "module.main." wrapper from a terraform
// resource address. Every Windsor component wraps its resources in this single
// module, so the prefix would otherwise be repeated noise on every rendered
// line. Addresses that don't start with the wrapper are returned unchanged.
func stripModuleMain(addr string) string {
	return strings.TrimPrefix(addr, "module.main.")
}
