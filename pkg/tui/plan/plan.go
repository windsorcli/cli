// Package plan renders Terraform and Kustomize plan summaries for the CLI.
// It lives under pkg/tui because plan output is a terminal-display concern,
// alongside the existing tui primitives (section headers, spinners, color
// codes). The cmd layer calls Summary or SummaryJSON; the layout, color
// vocabulary, and resource enumeration live here so command files stay focused
// on flag handling. Counts displayed under each component are derived from
// the per-resource change list rather than terraform's raw `change_summary`
// whenever the resource list is available — this keeps the header line and
// the indented resource list internally consistent (a terraform "replace"
// appears once, not once as +1 and once as -1).
package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// resourceListCap bounds the number of per-resource lines rendered under any
// single component. Kustomizations with hundreds of resources would otherwise
// dominate the summary; operators who need the full set can drill in with
// `windsor plan kustomize <name>`.
const resourceListCap = 20

// Summary writes the combined Terraform and Kustomize plan summary
// to w. Component names are left-aligned in a column wide enough to fit the
// longest name. Each row shows action-bucketed counts (+a ~c -d, with ±r
// appended when replaces are present), with "(no changes)" when all buckets
// are zero and "(error: ...)" when the plan failed. Components with non-empty
// Resources slices have each changed resource listed below the row, prefixed
// with the action symbol (+ ~ - ±). The list is capped at resourceListCap
// entries per component to keep large kustomizations from drowning the
// summary; the cap is followed by a "… and N more" line. When at least one
// component has actual changes, a footer hint points the user at the streaming
// `windsor plan terraform/kustomize <name>` form for full diffs. Any upgrade
// hints from missing CLI tools are printed in a footnote block at the bottom
// when present.
func Summary(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan, hints []string, noColor bool) {
	nameWidth := 20
	for _, p := range tfPlans {
		if n := len(terraformDisplayName(p)); n > nameWidth {
			nameWidth = n
		}
	}
	for _, p := range k8sPlans {
		if len(p.Name) > nameWidth {
			nameWidth = len(p.Name)
		}
	}
	nameWidth += 2

	sep := strings.Repeat("═", nameWidth+26)
	fmt.Fprintf(w, "\nWindsor Plan Summary\n%s\n", sep)

	if len(tfPlans) > 0 {
		fmt.Fprintln(w, "\nTerraform")
		for _, p := range tfPlans {
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, terraformDisplayName(p), formatTerraformPlan(p, noColor))
			if p.Err != nil {
				lines := strings.Split(strings.TrimSpace(p.Err.Error()), "\n")
				for _, line := range lines[1:] {
					fmt.Fprintf(w, "  %s  %s\n", strings.Repeat(" ", nameWidth), line)
				}
			}
			writeResourceList(w, terraformResourceChanges(p.Resources), noColor)
		}
	}

	if len(k8sPlans) > 0 {
		fmt.Fprintln(w, "\nKustomize")
		for _, p := range k8sPlans {
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, p.Name, formatKustomizePlan(p, noColor))
			if p.Err != nil {
				lines := strings.Split(strings.TrimSpace(p.Err.Error()), "\n")
				for _, line := range lines[1:] {
					fmt.Fprintf(w, "  %s  %s\n", strings.Repeat(" ", nameWidth), line)
				}
			}
			writeResourceList(w, kustomizeResourceChanges(p.Resources), noColor)
		}
	}

	if len(tfPlans) == 0 && len(k8sPlans) == 0 {
		fmt.Fprintln(w, "\n  (no components in blueprint)")
	}

	footerSep := strings.Repeat("─", nameWidth+26)
	if planHasChanges(tfPlans, k8sPlans) {
		fmt.Fprintf(w, "\n%s\n", footerSep)
		fmt.Fprintln(w, "  Run `windsor plan terraform <name>` or `windsor plan kustomize <name>` for full diffs.")
	}

	if len(hints) > 0 {
		fmt.Fprintf(w, "\n%s\n", footerSep)
		for _, h := range hints {
			for _, line := range strings.Split(h, "\n") {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
	}

	fmt.Fprintln(w)
}

// SummaryJSON encodes the plan results as JSON to w.
//
// is_new supersedes the count fields: when is_new is true the component has no
// state in the configured backend, terraform plan is not executed, and add/
// change/destroy/no_changes are zero/false. Consumers detecting "pending work"
// from this output must check is_new alongside the counts — using add+change+
// destroy>0 alone will silently miss never-applied components.
func SummaryJSON(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan) error {
	type resourceRow struct {
		Address string `json:"address"`
		Action  string `json:"action"`
	}
	type tfRow struct {
		Component string        `json:"component"`
		Path      string        `json:"path,omitempty"`
		Add       int           `json:"add"`
		Change    int           `json:"change"`
		Destroy   int           `json:"destroy"`
		NoChanges bool          `json:"no_changes"`
		IsNew     bool          `json:"is_new"`
		Resources []resourceRow `json:"resources,omitempty"`
		Error     string        `json:"error,omitempty"`
	}
	type k8sRow struct {
		Name      string        `json:"name"`
		Added     int           `json:"added"`
		Removed   int           `json:"removed"`
		IsNew     bool          `json:"is_new"`
		Degraded  bool          `json:"degraded"`
		Resources []resourceRow `json:"resources,omitempty"`
		Error     string        `json:"error,omitempty"`
	}
	type output struct {
		Terraform []tfRow  `json:"terraform,omitempty"`
		Kustomize []k8sRow `json:"kustomize,omitempty"`
	}

	out := output{}
	for _, p := range tfPlans {
		row := tfRow{
			Component: p.ComponentID,
			Path:      p.Path,
			Add:       p.Add,
			Change:    p.Change,
			Destroy:   p.Destroy,
			NoChanges: p.NoChanges,
			IsNew:     p.IsNew,
		}
		for _, r := range p.Resources {
			row.Resources = append(row.Resources, resourceRow{Address: r.Address, Action: terraformActionString(r.Action)})
		}
		if p.Err != nil {
			row.Error = p.Err.Error()
		}
		out.Terraform = append(out.Terraform, row)
	}
	for _, p := range k8sPlans {
		row := k8sRow{Name: p.Name, Added: p.Added, Removed: p.Removed, IsNew: p.IsNew, Degraded: p.Degraded}
		for _, r := range p.Resources {
			row.Resources = append(row.Resources, resourceRow{Address: r.Address, Action: kustomizeActionString(r.Action)})
		}
		if p.Err != nil {
			row.Error = p.Err.Error()
		}
		out.Kustomize = append(out.Kustomize, row)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// =============================================================================
// Internal types
// =============================================================================

// planResource is the renderer's adapter for the per-layer ResourceChange
// types. Both terraform and flux carry equivalent (address, action) pairs in
// their own packages; this struct collapses them into a single shape so
// writeResourceList can be reused without templating over the layer types.
type planResource struct {
	address string
	action  planAction
}

// planAction is the renderer's local action enum, decoupled from the per-layer
// Action types. The value order matches the rendering sort: destructive
// actions (delete, replace) precede modifications and creations so the
// operator's eye lands on the riskiest changes first.
type planAction int

const (
	planActionDelete planAction = iota
	planActionReplace
	planActionUpdate
	planActionCreate
)

// =============================================================================
// Internal helpers
// =============================================================================

// terraformDisplayName returns the identifier shown to the operator for a
// Terraform component row. The blueprint Path (e.g., "cluster/aws-eks")
// locates the underlying module and is more informative than the short
// ComponentID alias (e.g., "cluster"), so it's preferred when set; ComponentID
// is the fallback.
func terraformDisplayName(p terraforminfra.TerraformComponentPlan) string {
	if p.Path != "" {
		return p.Path
	}
	return p.ComponentID
}

// formatTerraformPlan returns the right-hand-side status string for one
// Terraform component. Counts are derived from the Resources slice when
// populated so the header line matches what the operator sees expanded below
// it; otherwise the raw Add/Change/Destroy fields are used (terraform's
// change_summary, which double-counts replaces). IsNew renders as "(new)"
// without counts because plan was not run.
func formatTerraformPlan(p terraforminfra.TerraformComponentPlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.IsNew {
		if noColor {
			return "(new)"
		}
		return "\033[36m(new)\033[0m"
	}
	if len(p.Resources) > 0 {
		return formatResourceCounts(terraformResourceChanges(p.Resources), noColor)
	}
	if p.NoChanges || (p.Add == 0 && p.Change == 0 && p.Destroy == 0) {
		return "(no changes)"
	}
	return formatRawCounts(p.Add, p.Change, p.Destroy, 0, noColor)
}

// formatKustomizePlan returns the right-hand-side status string for one
// Kustomize component. Counts are derived from the Resources slice when
// populated, giving per-resource accounting consistent with the indented list
// below the row. Without Resources, the diff-line counts (Added/Removed) are
// used as a fallback for the Degraded path.
func formatKustomizePlan(p fluxinfra.KustomizePlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.Degraded {
		if p.IsNew {
			return "(new)"
		}
		return "(existing)"
	}
	if p.IsNew {
		// The kustomize-build resource count for a brand-new kustomization is
		// rarely the count actually applied (Flux may dedupe, skip, or
		// reconcile asynchronously), and showing "+N resources" alongside
		// terraform's bare "(new)" is visually inconsistent. Both layers
		// report "(new)" for the same condition: nothing exists yet.
		if noColor {
			return "(new)"
		}
		return "\033[36m(new)\033[0m"
	}
	if len(p.Resources) > 0 {
		return formatResourceCounts(kustomizeResourceChanges(p.Resources), noColor)
	}
	if p.Added == 0 && p.Removed == 0 {
		return "(no changes)"
	}
	if noColor {
		return fmt.Sprintf("+%d  -%d  lines", p.Added, p.Removed)
	}
	return fmt.Sprintf("\033[32m+%d\033[0m  \033[31m-%d\033[0m  lines", p.Added, p.Removed)
}

// formatResourceCounts buckets a resource list into the four action categories
// and renders them as "+a  ~c  -d" with " ±r" appended when r > 0. The ±
// bucket is omitted when zero so rows without replaces look identical to the
// pre-replace-tracking format. All-zero produces "(no changes)" — only
// reachable if every entry mapped to ActionUnknown and was filtered out.
func formatResourceCounts(rs []planResource, noColor bool) string {
	var a, c, d, r int
	for _, x := range rs {
		switch x.action {
		case planActionCreate:
			a++
		case planActionUpdate:
			c++
		case planActionDelete:
			d++
		case planActionReplace:
			r++
		}
	}
	if a == 0 && c == 0 && d == 0 && r == 0 {
		return "(no changes)"
	}
	return formatRawCounts(a, c, d, r, noColor)
}

// formatRawCounts renders the four-bucket count line, omitting the replace
// bucket entirely when zero so the common no-replace case stays compact.
func formatRawCounts(a, c, d, r int, noColor bool) string {
	if noColor {
		base := fmt.Sprintf("+%d  ~%d  -%d", a, c, d)
		if r > 0 {
			return base + fmt.Sprintf("  ±%d", r)
		}
		return base
	}
	base := fmt.Sprintf("\033[32m+%d\033[0m  \033[33m~%d\033[0m  \033[31m-%d\033[0m", a, c, d)
	if r > 0 {
		return base + fmt.Sprintf("  \033[33m±%d\033[0m", r)
	}
	return base
}

// terraformResourceChanges adapts terraform.ResourceChange entries to the
// renderer's planResource shape, dropping any with action ActionUnknown.
func terraformResourceChanges(rs []terraforminfra.ResourceChange) []planResource {
	out := make([]planResource, 0, len(rs))
	for _, r := range rs {
		var a planAction
		switch r.Action {
		case terraforminfra.ActionCreate:
			a = planActionCreate
		case terraforminfra.ActionUpdate:
			a = planActionUpdate
		case terraforminfra.ActionDelete:
			a = planActionDelete
		case terraforminfra.ActionReplace:
			a = planActionReplace
		default:
			continue
		}
		out = append(out, planResource{address: r.Address, action: a})
	}
	return out
}

// kustomizeResourceChanges adapts flux.ResourceChange entries to the
// renderer's planResource shape, dropping any with action ActionUnknown.
func kustomizeResourceChanges(rs []fluxinfra.ResourceChange) []planResource {
	out := make([]planResource, 0, len(rs))
	for _, r := range rs {
		var a planAction
		switch r.Action {
		case fluxinfra.ActionCreate:
			a = planActionCreate
		case fluxinfra.ActionUpdate:
			a = planActionUpdate
		case fluxinfra.ActionDelete:
			a = planActionDelete
		case fluxinfra.ActionReplace:
			a = planActionReplace
		default:
			continue
		}
		out = append(out, planResource{address: r.Address, action: a})
	}
	return out
}

// writeResourceList renders an indented per-resource changeset under a
// component row. Resources are sorted destructive-first then alphabetically so
// the operator's eye lands on deletes and replaces before creates. Lists
// longer than resourceListCap are truncated with a trailing "… and N more"
// line.
func writeResourceList(w io.Writer, resources []planResource, noColor bool) {
	if len(resources) == 0 {
		return
	}
	sorted := append([]planResource(nil), resources...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].action != sorted[j].action {
			return sorted[i].action < sorted[j].action
		}
		return sorted[i].address < sorted[j].address
	})

	limit := len(sorted)
	if limit > resourceListCap {
		limit = resourceListCap
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintf(w, "    %s %s\n", actionSymbol(sorted[i].action, noColor), sorted[i].address)
	}
	if rem := len(sorted) - limit; rem > 0 {
		fmt.Fprintf(w, "    … and %d more\n", rem)
	}
}

// actionSymbol returns the colored single-character notation for a planAction.
// Symbols mirror terraform's own +/~/- vocabulary, plus ± for replace, so the
// notation is familiar to anyone who has read a terraform plan.
func actionSymbol(a planAction, noColor bool) string {
	switch a {
	case planActionCreate:
		if noColor {
			return "+"
		}
		return "\033[32m+\033[0m"
	case planActionUpdate:
		if noColor {
			return "~"
		}
		return "\033[33m~\033[0m"
	case planActionDelete:
		if noColor {
			return "-"
		}
		return "\033[31m-\033[0m"
	case planActionReplace:
		if noColor {
			return "±"
		}
		return "\033[33m±\033[0m"
	default:
		return " "
	}
}

// planHasChanges reports whether at least one component has a non-trivial plan
// result that warrants the "drill in for full diffs" footer hint. New
// components count as changes (they're pending work even though plan wasn't
// run), as do non-zero counts on either layer. Errored or degraded components
// don't trigger the hint on their own — the failure is already visible inline.
func planHasChanges(tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan) bool {
	for _, p := range tfPlans {
		if p.IsNew {
			return true
		}
		if p.Err == nil && (p.Add+p.Change+p.Destroy) > 0 {
			return true
		}
	}
	for _, p := range k8sPlans {
		if p.IsNew {
			return true
		}
		if p.Err == nil && !p.Degraded && (p.Added+p.Removed) > 0 {
			return true
		}
	}
	return false
}

// terraformActionString renders a terraform Action as a stable JSON string.
// The vocabulary matches terraform's own plan -json verbs ("create", "update",
// "delete", "replace") rather than the renderer's symbols, since machine
// consumers expect English keywords.
func terraformActionString(a terraforminfra.Action) string {
	switch a {
	case terraforminfra.ActionCreate:
		return "create"
	case terraforminfra.ActionUpdate:
		return "update"
	case terraforminfra.ActionDelete:
		return "delete"
	case terraforminfra.ActionReplace:
		return "replace"
	default:
		return "unknown"
	}
}

// kustomizeActionString renders a flux Action as a stable JSON string,
// matching terraform's vocabulary so machine consumers can treat both layers
// uniformly.
func kustomizeActionString(a fluxinfra.Action) string {
	switch a {
	case fluxinfra.ActionCreate:
		return "create"
	case fluxinfra.ActionUpdate:
		return "update"
	case fluxinfra.ActionDelete:
		return "delete"
	case fluxinfra.ActionReplace:
		return "replace"
	default:
		return "unknown"
	}
}

// truncateFirstLine returns the first line of s, stripping any trailing
// content after \n or \r\n.
func truncateFirstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx != -1 {
		return strings.TrimRight(s[:idx], "\r")
	}
	return s
}
