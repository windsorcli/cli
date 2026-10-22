// The destroy functions of the plan package render a destroy plan preview for the CLI.
// They provide DestroySummary for terminal output and DestroySummaryJSON for machine output.
// A destroy plan has only delete actions, so each component shows one count.

package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Public Methods
// =============================================================================

// DestroySummary writes a destroy plan preview to w, in the same layout as Summary.
// The header is "Windsor Destroy Plan", and each count is "-N".
// A new component shows "(no state)" or "(not deployed)", because it has nothing to destroy.
// It writes no footer and no hints. The caller writes the confirmation text.
func DestroySummary(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan, noColor bool) {
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
	fmt.Fprintf(w, "\nWindsor Destroy Plan\n%s\n", sep)

	if len(tfPlans) > 0 {
		fmt.Fprintln(w, "\nTerraform")
		for _, p := range tfPlans {
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, terraformDisplayName(p), formatTerraformDestroyPlan(p, noColor))
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
			fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, p.Name, formatKustomizeDestroyPlan(p, noColor))
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

	fmt.Fprintln(w)
}

// DestroySummaryJSON writes the destroy plan results to w as JSON, in the same schema as SummaryJSON.
// When is_new is true, the component has no state to destroy.
// The only resource action in a destroy plan is "delete".
func DestroySummaryJSON(w io.Writer, tfPlans []terraforminfra.TerraformComponentPlan, k8sPlans []fluxinfra.KustomizePlan) error {
	type resourceRow struct {
		Address string `json:"address"`
		Action  string `json:"action"`
	}
	type tfRow struct {
		Component string        `json:"component"`
		Path      string        `json:"path,omitempty"`
		Destroy   int           `json:"destroy"`
		NoChanges bool          `json:"no_changes"`
		IsNew     bool          `json:"is_new"`
		Resources []resourceRow `json:"resources,omitempty"`
		Error     string        `json:"error,omitempty"`
	}
	type k8sRow struct {
		Name      string        `json:"name"`
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
		row := k8sRow{Name: p.Name, Removed: p.Removed, IsNew: p.IsNew, Degraded: p.Degraded}
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

// formatTerraformDestroyPlan returns the status text for one Terraform component in a destroy plan.
// It returns "(no state)" for a new component, because there is nothing to destroy.
// It counts the Resources list when it has entries. If not, it uses the raw Destroy count.
func formatTerraformDestroyPlan(p terraforminfra.TerraformComponentPlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.IsNew {
		if noColor {
			return "(no state)"
		}
		return "\033[36m(no state)\033[0m"
	}
	if len(p.Resources) > 0 {
		return formatDestroyCount(len(terraformResourceChanges(p.Resources)), noColor)
	}
	if p.NoChanges || p.Destroy == 0 {
		return "(nothing to destroy)"
	}
	return formatDestroyCount(p.Destroy, noColor)
}

// formatKustomizeDestroyPlan returns the status text for one Flux kustomization in a destroy plan.
// It returns "(not deployed)" when the kustomization is not in the cluster.
// It counts the Resources list when it has entries. If not, it uses the Removed count.
func formatKustomizeDestroyPlan(p fluxinfra.KustomizePlan, noColor bool) string {
	if p.Err != nil {
		msg := truncateFirstLine(p.Err.Error())
		if noColor {
			return fmt.Sprintf("(error: %s)", msg)
		}
		return fmt.Sprintf("\033[31m(error: %s)\033[0m", msg)
	}
	if p.IsNew {
		if noColor {
			return "(not deployed)"
		}
		return "\033[36m(not deployed)\033[0m"
	}
	if len(p.Resources) > 0 {
		return formatDestroyCount(len(kustomizeResourceChanges(p.Resources)), noColor)
	}
	if p.Removed == 0 {
		return "(nothing to destroy)"
	}
	return formatDestroyCount(p.Removed, noColor)
}

// formatDestroyCount returns the "-N" count of a destroy plan, in red unless noColor is true.
func formatDestroyCount(n int, noColor bool) string {
	if noColor {
		return fmt.Sprintf("-%d", n)
	}
	return fmt.Sprintf("\033[31m-%d\033[0m", n)
}
