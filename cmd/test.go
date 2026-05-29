package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/test"
)

var testCmd = &cobra.Command{
	Use:   "test [test-name]",
	Short: "Run blueprint composition tests.",
	Long: `Run static tests that compare blueprint composition against expected outputs. Tests live in contexts/_template/tests/ as '*.test.yaml' files.

A test runs the blueprint composer in isolation (no terraform, no cluster, no live secrets) and asserts the resulting blueprint, kustomization, or values match a fixture. Use 'windsor test' to validate that schema or facet changes don't accidentally regress composition.

When a test name is provided, only that test runs; otherwise every test under contexts/_template/tests/ runs.`,
	Example: `# Run all tests
windsor test

# Run a single named test
windsor test cluster-defaults`,
	Annotations: map[string]string{
		"docs.seealso": "[Testing reference](../testing.md)",
		"docs.source": "cmd/test.go",
	},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			opts = []*project.Project{overridesVal.(*project.Project)}
		}

		proj := project.NewProject("", opts...)

		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		comp := proj.Composer
		if comp == nil {
			comp = composer.NewComposer(proj.Runtime)
		}

		var testRunner *test.TestRunner
		if overrideVal := cmd.Context().Value(testRunnerOverridesKey); overrideVal != nil {
			testRunner = overrideVal.(*test.TestRunner)
		} else {
			testRunner = test.NewTestRunner(proj.Runtime, comp.ArtifactBuilder)
		}

		var testFilter string
		if len(args) > 0 {
			testFilter = args[0]
		}

		return testRunner.RunAndPrint(testFilter)
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
}
