// The partial run methods of the Provisioner stop a run part-way and continue it later.
// It provides PlanRun, RecordPartialRun, ClearPartialRun, and the RunPlan type.
// A run that --to stops writes a record of the finished Terraform components.
// A run with --continue skips those components, so it does not apply them again.

package provisioner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

const (
	partialRunFilename = "partial-run.yaml"
	terraformPrefix    = "terraform/"
	kustomizePrefix    = "kustomize/"
	kindTerraform      = "terraform"
	kindKustomize      = "kustomize"
)

// =============================================================================
// Errors
// =============================================================================

// ErrInvalidRunTarget marks a --to or --continue request that the blueprint or the saved record cannot satisfy.
var ErrInvalidRunTarget = errors.New("invalid run target")

// =============================================================================
// Types
// =============================================================================

// PartialRun is the record that a stopped run saves in the scratch directory of the context.
// Terraform lists the finished component IDs in apply order. Components lists every enabled
// component ID at the stop, so a later change to the blueprint is detectable.
type PartialRun struct {
	Command      string   `yaml:"command"`
	StoppedAfter string   `yaml:"stoppedAfter"`
	Terraform    []string `yaml:"terraform"`
	Components   []string `yaml:"components"`
}

// RunPlan describes which parts of a run to do.
// Skip lists the Terraform components that an earlier stopped run finished.
// Apply lists the Terraform components to apply now, in blueprint order.
// Install is true when the run installs Kustomizations. Kustomize lists the names to install when
// KustomizeSubset is true, and the run installs all of them otherwise.
// StopKind and StopName name the --to target, and both are empty for a run to the end.
type RunPlan struct {
	Command         string
	Skip            []string
	Apply           []string
	Install         bool
	KustomizeSubset bool
	Kustomize       []string
	StopKind        string
	StopName        string
}

// =============================================================================
// Public Methods
// =============================================================================

// PlanRun builds the run plan for a run with --to, --continue, or both, and keeps it for the next Up.
// It returns nil, and keeps nothing, when both are unset.
// It reads the saved record when continuing. It rejects an unknown or ambiguous name, and a name
// inside the backend tier that is not its last component, because the tier runs as a unit.
// It runs before any work starts, so a bad request changes nothing.
func (i *Provisioner) PlanRun(bp *blueprintv1alpha1.Blueprint, command, to string, continuing bool) (*RunPlan, error) {
	if to == "" && !continuing {
		return nil, nil
	}
	if bp == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	var record *PartialRun
	if continuing {
		var err error
		record, err = i.readPartialRun()
		if err != nil {
			return nil, err
		}
	}
	plan, err := BuildRunPlan(bp, command, to, record, i.backendTierIDs(bp))
	if err != nil {
		return nil, err
	}
	i.runPlan = plan
	return plan, nil
}

// RecordPartialRun saves the progress of a stopped run, so that --continue can skip the finished components.
// It overwrites an earlier record. It returns an error when the scratch directory is not set.
func (i *Provisioner) RecordPartialRun(bp *blueprintv1alpha1.Blueprint, plan *RunPlan) error {
	path, err := i.partialRunPath()
	if err != nil {
		return err
	}
	record := PartialRun{
		Command:      plan.Command,
		StoppedAfter: plan.StopKind + "/" + plan.StopName,
		Terraform:    plan.Finished(),
		Components:   enabledTerraformIDs(bp),
	}
	data, err := i.Shims.YamlMarshal(record)
	if err != nil {
		return werror.Wrap(err, "error encoding the partial run record")
	}
	if err := i.Shims.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return werror.Wrap(err, "error creating the scratch directory")
	}
	if err := i.Shims.WriteFile(path, data, 0o644); err != nil {
		return werror.Wrap(err, "error writing the partial run record")
	}
	return nil
}

// ClearPartialRun removes the saved record. A missing record is not an error.
func (i *Provisioner) ClearPartialRun() error {
	path, err := i.partialRunPath()
	if err != nil {
		return err
	}
	if err := i.Shims.Remove(path); err != nil && !isNotExist(err) {
		return werror.Wrap(err, "error removing the partial run record")
	}
	return nil
}

// BuildRunPlan resolves --to and the record into a RunPlan, with no side effects.
// A nil record means the run does not continue. tier lists the backend tier component IDs that
// the run applies through the backend pivot, or nothing when no pivot runs.
// The Terraform part of a --to Terraform target is every enabled component up to and including
// the target, which includes all dependencies, because the blueprint order is a topological order.
// The Kustomization part of a --to Kustomization target is the target and its dependsOn closure.
// A name that matches a Terraform component and a Kustomization needs the terraform/ or kustomize/ prefix.
func BuildRunPlan(bp *blueprintv1alpha1.Blueprint, command, to string, record *PartialRun, tier []string) (*RunPlan, error) {
	ids := enabledTerraformIDs(bp)
	names := kustomizationNames(bp)

	plan := &RunPlan{Command: command, Install: true}
	if record != nil {
		if err := validatePartialRun(record, command, ids); err != nil {
			return nil, err
		}
		plan.Skip = slices.Clone(record.Terraform)
	}
	plan.Apply = subtract(ids, plan.Skip)

	if to == "" {
		return plan, nil
	}

	kind, name, err := resolveRunTarget(to, ids, names)
	if err != nil {
		return nil, err
	}
	plan.StopKind = kind
	plan.StopName = name

	if kind == kindKustomize {
		plan.KustomizeSubset = true
		plan.Kustomize = dependencyClosure(bp, name)
		return plan, nil
	}

	if slices.Contains(plan.Skip, name) {
		return nil, werror.New(fmt.Sprintf("terraform component %q already finished in the stopped run", name), "Name a later component or kustomization with --to.", ErrInvalidRunTarget)
	}
	if len(tier) > 0 && slices.Contains(tier, name) && name != tier[len(tier)-1] {
		return nil, werror.New(fmt.Sprintf("terraform component %q is inside the backend tier, which runs as one unit", name), fmt.Sprintf("Use --to %s to stop after the backend tier.", tier[len(tier)-1]), ErrInvalidRunTarget)
	}
	plan.Apply = subtract(ids[:slices.Index(ids, name)+1], plan.Skip)
	plan.Install = false
	return plan, nil
}

// Stops reports whether the run ends at a --to target. A nil plan does not stop.
func (p *RunPlan) Stops() bool {
	return p != nil && p.StopName != ""
}

// Finished returns the Terraform components that are finished when the run ends, in apply order.
func (p *RunPlan) Finished() []string {
	return append(slices.Clone(p.Skip), p.Apply...)
}

// StopLabel names the --to target for a message, such as `terraform component "cluster"`.
func (p *RunPlan) StopLabel() string {
	if p.StopKind == kindKustomize {
		return fmt.Sprintf("kustomization %q", p.StopName)
	}
	return fmt.Sprintf("terraform component %q", p.StopName)
}

// TerraformBlueprint returns a copy of bp that holds only the components in Apply.
// It clears the backend field when the backend component is not in Apply, so the run applies the
// remaining components directly against the configured backend and does not run the pivot again.
// A nil plan returns bp unchanged.
func (p *RunPlan) TerraformBlueprint(bp *blueprintv1alpha1.Blueprint) *blueprintv1alpha1.Blueprint {
	if p == nil {
		return bp
	}
	cp := *bp
	cp.TerraformComponents = make([]blueprintv1alpha1.TerraformComponent, 0, len(bp.TerraformComponents))
	for _, c := range bp.TerraformComponents {
		if slices.Contains(p.Apply, c.GetID()) {
			cp.TerraformComponents = append(cp.TerraformComponents, c)
		}
	}
	if cp.Backend != "" && !slices.Contains(p.Apply, cp.Backend) {
		cp.Backend = ""
	}
	return &cp
}

// InstallBlueprint returns a copy of bp that holds only the Kustomizations in Kustomize, or bp itself
// when the run installs all of them. The CRD layers stay, because the install adds them from the sources.
// A nil plan returns bp unchanged.
func (p *RunPlan) InstallBlueprint(bp *blueprintv1alpha1.Blueprint) *blueprintv1alpha1.Blueprint {
	if p == nil || !p.KustomizeSubset {
		return bp
	}
	cp := *bp
	cp.Kustomizations = nil
	for _, k := range bp.AllKustomizations() {
		if slices.Contains(p.Kustomize, k.Name) {
			cp.Kustomizations = append(cp.Kustomizations, k)
		}
	}
	cp.FluxSystems = nil
	return &cp
}

// =============================================================================
// Private Methods
// =============================================================================

// replayFinished runs the apply hooks and the post-apply hooks for the components in Skip, in order.
// The hooks of the workstation save state and can ask for a host change after a component. A run
// that continues must run them again, so it ends in the same state as a run with no stop.
// It returns true when a hook halts the run.
func (i *Provisioner) replayFinished(ctx context.Context, plan *RunPlan, onApply ...func(ctx context.Context, id string) (bool, error)) (bool, error) {
	hooks := append([]func(ctx context.Context, id string) (bool, error){}, i.onTerraformApply...)
	hooks = append(hooks, onApply...)
	for _, id := range plan.Skip {
		halted := false
		for _, fn := range hooks {
			if fn == nil {
				continue
			}
			halt, err := fn(ctx, id)
			if err != nil {
				return false, werror.Wrap(err, "post-apply hook %s", id)
			}
			halted = halted || halt
		}
		for _, fn := range i.onTerraformPostApply {
			if fn == nil {
				continue
			}
			if err := fn(ctx, id); err != nil {
				return false, werror.Wrap(err, "post-apply hook %s", id)
			}
		}
		if halted {
			return true, nil
		}
	}
	return false, nil
}

// backendTierIDs returns the enabled component IDs of the backend tier when the run applies it through the pivot.
// It returns nothing when the backend type is local or empty, or when the blueprint declares no backend.
func (i *Provisioner) backendTierIDs(bp *blueprintv1alpha1.Blueprint) []string {
	backendType := i.configHandler.GetTerraformBackendType()
	if backendType == "" || backendType == "local" {
		return nil
	}
	enabled := enabledTerraformIDs(bp)
	var tier []string
	for _, c := range bp.BackendComponents() {
		if slices.Contains(enabled, c.GetID()) {
			tier = append(tier, c.GetID())
		}
	}
	return tier
}

// readPartialRun loads the saved record. A missing record returns an error that tells the operator to run without --continue.
func (i *Provisioner) readPartialRun() (*PartialRun, error) {
	path, err := i.partialRunPath()
	if err != nil {
		return nil, err
	}
	data, err := i.Shims.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return nil, werror.New("no stopped run to continue", "Run the command without --continue.", ErrInvalidRunTarget)
		}
		return nil, werror.Wrap(err, "error reading the partial run record")
	}
	var record PartialRun
	if err := i.Shims.YamlUnmarshal(data, &record); err != nil {
		return nil, werror.New("the partial run record is not valid", fmt.Sprintf("Remove %s, then run the command without --continue.", path), ErrInvalidRunTarget)
	}
	return &record, nil
}

// partialRunPath returns the path of the record in the scratch directory of the context.
func (i *Provisioner) partialRunPath() (string, error) {
	if i.runtime == nil || i.runtime.WindsorScratchPath == "" {
		return "", fmt.Errorf("scratch path is empty")
	}
	return filepath.Join(i.runtime.WindsorScratchPath, partialRunFilename), nil
}

// =============================================================================
// Helpers
// =============================================================================

// validatePartialRun checks that the record fits the command and the blueprint.
// The finished components MUST be a prefix of the enabled components, and the component list MUST match.
func validatePartialRun(record *PartialRun, command string, ids []string) error {
	if record.Command != command {
		return werror.New(fmt.Sprintf("the stopped run came from 'windsor %s', not 'windsor %s'", record.Command, command), fmt.Sprintf("Run 'windsor %s --continue', or run this command without --continue.", record.Command), ErrInvalidRunTarget)
	}
	if !slices.Equal(record.Components, ids) || len(record.Terraform) > len(ids) || !slices.Equal(record.Terraform, ids[:len(record.Terraform)]) {
		return werror.New("the Terraform components changed since the run stopped", "Run the command without --continue.", ErrInvalidRunTarget)
	}
	return nil
}

// resolveRunTarget returns the kind and the name for a --to value.
// A terraform/ or kustomize/ prefix selects the kind. Without a prefix, the name MUST match exactly one kind.
// An unknown name returns an error that lists the valid names.
func resolveRunTarget(to string, ids, names []string) (string, string, error) {
	switch {
	case strings.HasPrefix(to, terraformPrefix):
		return checkRunTarget(kindTerraform, strings.TrimPrefix(to, terraformPrefix), ids, names)
	case strings.HasPrefix(to, kustomizePrefix):
		return checkRunTarget(kindKustomize, strings.TrimPrefix(to, kustomizePrefix), ids, names)
	}
	inTerraform := slices.Contains(ids, to)
	inKustomize := slices.Contains(names, to)
	switch {
	case inTerraform && inKustomize:
		return "", "", werror.New(fmt.Sprintf("--to %q matches a terraform component and a kustomization", to), fmt.Sprintf("Use --to %s%s or --to %s%s.", terraformPrefix, to, kustomizePrefix, to), ErrInvalidRunTarget)
	case inTerraform:
		return kindTerraform, to, nil
	case inKustomize:
		return kindKustomize, to, nil
	}
	return "", "", unknownRunTarget(to, ids, names)
}

// checkRunTarget confirms that name exists in the list of its kind.
func checkRunTarget(kind, name string, ids, names []string) (string, string, error) {
	list := ids
	if kind == kindKustomize {
		list = names
	}
	if !slices.Contains(list, name) {
		return "", "", unknownRunTarget(kind+"/"+name, ids, names)
	}
	return kind, name, nil
}

// unknownRunTarget builds the error for a --to value that matches nothing. The remediation lists the valid names.
func unknownRunTarget(to string, ids, names []string) error {
	return werror.New(fmt.Sprintf("unknown --to target %q", to), fmt.Sprintf("Valid terraform components: %s. Valid kustomizations: %s.", joinOrNone(ids), joinOrNone(names)), ErrInvalidRunTarget)
}

// joinOrNone joins names with a comma, or returns "none" for an empty list.
func joinOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// enabledTerraformIDs returns the IDs of the enabled terraform components in blueprint order.
func enabledTerraformIDs(bp *blueprintv1alpha1.Blueprint) []string {
	ids := make([]string, 0, len(bp.TerraformComponents))
	for _, c := range bp.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		ids = append(ids, c.GetID())
	}
	return ids
}

// kustomizationNames returns the unique names of the Kustomizations that a run installs, in blueprint order.
func kustomizationNames(bp *blueprintv1alpha1.Blueprint) []string {
	var names []string
	for _, k := range bp.AllKustomizations() {
		if k.DestroyOnly != nil && *k.DestroyOnly {
			continue
		}
		if !slices.Contains(names, k.Name) {
			names = append(names, k.Name)
		}
	}
	return names
}

// dependencyClosure returns name and every Kustomization it depends on, directly or not, in blueprint order.
// A dependency that the blueprint does not declare, such as a CRD layer, is not in the result.
func dependencyClosure(bp *blueprintv1alpha1.Blueprint, name string) []string {
	all := bp.AllKustomizations()
	deps := make(map[string][]string, len(all))
	for _, k := range all {
		deps[k.Name] = k.DependsOn
	}
	seen := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if seen[n] {
			return
		}
		if _, ok := deps[n]; !ok {
			return
		}
		seen[n] = true
		for _, d := range deps[n] {
			visit(d)
		}
	}
	visit(name)
	var out []string
	for _, k := range all {
		if seen[k.Name] && !slices.Contains(out, k.Name) {
			out = append(out, k.Name)
		}
	}
	return out
}

// subtract returns the items of list that are not in remove, in list order.
func subtract(list, remove []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if !slices.Contains(remove, item) {
			out = append(out, item)
		}
	}
	return out
}

// isNotExist reports whether err means that a file does not exist.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
