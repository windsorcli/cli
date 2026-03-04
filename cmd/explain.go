package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
)

var explainCmd = &cobra.Command{
	Use:   "explain <path>",
	Short: "Explain where a blueprint value comes from",
	Long:  "Explain a value in the composed blueprint by path (e.g. terraform.cluster.inputs.common_config_patches, kustomize.dns.substitutions.external_domain, configMaps.values-common.DOMAIN). Prints the value and its contributions.",
	Args:  cobra.ExactArgs(1),
	RunE:  runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) error {
	pathStr := args[0]
	if _, err := blueprint.ParseExplainPath(pathStr); err != nil {
		return err
	}

	var opts []*project.Project
	if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
		opts = []*project.Project{overridesVal.(*project.Project)}
	}
	proj := project.NewProject("", opts...)
	proj.Runtime.Shell.SetVerbosity(verbose)
	if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
		return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
	}
	if err := proj.Configure(nil); err != nil {
		return err
	}
	if bh, ok := proj.Composer.BlueprintHandler.(*blueprint.BaseBlueprintHandler); ok {
		bh.SetTraceCollector(blueprint.NewTraceCollector())
	}
	var validationErr error
	if err := proj.ComposeBlueprint(); err != nil {
		validationErr = err
	}

	trace, err := proj.Composer.BlueprintHandler.Explain(pathStr)
	if err != nil {
		return err
	}

	printTrace(trace)
	if validationErr != nil {
		fmt.Fprintf(os.Stderr, "\033[33mWarning: %v\033[0m\n", validationErr)
	}
	return nil
}

func printTrace(t *blueprint.ExplainTrace) {
	isList := strings.HasSuffix(t.Path, ".components")
	if isList {
		fmt.Printf("%s\n", t.Path)
		idx := 0
		for _, c := range t.Contributions {
			if !c.Effective {
				continue
			}
			fmt.Printf("  [%d] %s\n", idx, c.Expression)
			idx++
			printContribution(c)
		}
		return
	}
	value := t.Value
	switch {
	case value == "":
		fmt.Printf("%s (empty)\n", t.Path)
	case strings.Contains(value, "${"):
		fmt.Printf("%s (deferred)\n", t.Path)
	case len(value) > 60:
		fmt.Printf("%s\n", t.Path)
	default:
		fmt.Printf("%s = %s\n", t.Path, value)
	}
	for _, c := range t.Contributions {
		if !c.Effective {
			continue
		}
		printContribution(c)
	}
}

func printContribution(c blueprint.ExplainContribution) {
	indent := "  "
	if c.AbsFacetPath != "" && c.Line > 0 {
		fmt.Printf("%s%s:%d\n", indent, c.AbsFacetPath, c.Line)
	} else if c.FacetPath != "" {
		fmt.Printf("%s%s\n", indent, c.FacetPath)
	} else {
		fmt.Printf("%s%s\n", indent, c.SourceName)
		if len(c.ScopeRefs) == 0 {
			return
		}
	}
	for _, ref := range c.ScopeRefs {
		printScopeRef(ref, "    ")
	}
}

func printScopeRef(ref blueprint.ExplainScopeRef, indent string) {
	switch ref.Status {
	case "not set":
		fmt.Printf("%s%s (not set)\n", indent, ref.Name)
	case "deferred":
		fmt.Printf("%s%s (deferred)\n", indent, ref.Name)
	case "cycle":
		fmt.Printf("%s%s (cycle)\n", indent, ref.Name)
	default:
		fmt.Printf("%s%s\n", indent, ref.Name)
	}
	if ref.Source != "" && ref.Line > 0 {
		fmt.Printf("%s  %s:%d\n", indent, ref.Source, ref.Line)
	}
	for _, n := range ref.Nested {
		printScopeRef(n, indent+"  ")
	}
}
