// The plan package renders the Terraform and Kustomize plan summaries for the CLI.
// It provides Summary for terminal output and SummaryJSON for machine output.
// It counts the list of changed resources when it is available, so a replace counts once.
// The cmd layer calls these functions and keeps only the flag handling.

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

// =============================================================================
// Constants
// =============================================================================

// resourceListCap is the maximum number of resource lines that the summary shows under one component.
const resourceListCap = 20

// =============================================================================
// Types
// =============================================================================

// planResource is one changed resource, with its address and action, from either layer.
// It lets writeResourceList render terraform changes and flux changes with the same code.
type planResource struct {
	address string
	action  planAction
}

// planAction is the action of a planResource.
// The values sort deletes and replaces before updates and creates, so the riskiest changes show first.
type planAction int

const (
	planActionDelete planAction = iota
	planActionReplace
	planActionUpdate
	planActionCreate
)

// =============================================================================
// Public Methods
// =============================================================================

// Summary writes the combined Terraform and Kustomize plan summary to w.
// Each component row shows a count for each action, "(no changes)", or "(error: ...)".
// Below each row, it lists the changed resources, up to resourceListCap entries.
// When a component has changes, a footer tells the user how to see the full diffs.
// It writes the upgrade hints for missing CLI tools at the bottom.
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

// SummaryJSON writes the plan results to w as JSON.
// When is_new is true, terraform did not run a plan, so all counts are zero.
// A consumer that looks for pending work MUST check is_new and the counts together.
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
// Helpers
// =============================================================================

// terraformDisplayName returns the name of a Terraform component row.
// It uses the blueprint Path when it is set, and the ComponentID when it is not.
func terraformDisplayName(p terraforminfra.TerraformComponentPlan) string {
	if p.Path != "" {
		return p.Path
	}
	return p.ComponentID
}

// formatTerraformPlan returns the status text for one Terraform component.
// It counts the Resources list when it has entries, because the raw change summary counts a replace twice.
// It returns "(new)" without counts for a new component, because terraform did not run a plan.
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

// formatKustomizePlan returns the status text for one Kustomize component.
// It counts the Resources list when it has entries. If not, it counts the added and removed diff lines.
// It returns "(new)" without counts for a new kustomization, the same as for terraform.
// The kustomize build count for a new kustomization is often not the count that Flux applies.
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

// formatResourceCounts counts a resource list by action and returns the text "+a  ~c  -d".
// It adds " ±r" only when there are replaces. It returns "(no changes)" when all counts are zero.
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

// formatRawCounts returns the count text for the four actions.
// It leaves out the replace count when it is zero.
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

// terraformResourceChanges converts terraform resource changes to planResource values.
// It drops the changes that have ActionUnknown.
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

// kustomizeResourceChanges converts flux resource changes to planResource values.
// It drops the changes that have ActionUnknown.
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
		default:
			continue
		}
		out = append(out, planResource{address: r.Address, action: a})
	}
	return out
}

// writeResourceList writes the changed resources below a component row.
// It sorts deletes and replaces first, then sorts by address.
// When the list is longer than resourceListCap, it ends with an "… and N more" line.
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

// actionSymbol returns the symbol for a planAction.
// It returns +, ~ or - as terraform does, and ± for a replace. It adds color unless noColor is true.
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

// planHasChanges reports whether a component has changes, so that Summary shows the footer hint.
// A new component counts as a change. A failed or degraded component does not, because its row shows the failure.
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

// terraformActionString returns the JSON string for a terraform Action.
// It uses the terraform plan verbs: create, update, delete and replace.
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

// kustomizeActionString returns the JSON string for a flux Action, with the same verbs as terraform.
// It has no replace verb, because flux shows a replace as a delete and a create.
func kustomizeActionString(a fluxinfra.Action) string {
	switch a {
	case fluxinfra.ActionCreate:
		return "create"
	case fluxinfra.ActionUpdate:
		return "update"
	case fluxinfra.ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// truncateFirstLine returns the first line of s, without a trailing carriage return.
func truncateFirstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx != -1 {
		return strings.TrimRight(s[:idx], "\r")
	}
	return s
}
