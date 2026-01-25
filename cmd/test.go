package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/test"
)

var testCmd = &cobra.Command{
	Use:          "test [test-name]",
	Short:        "Run blueprint composition tests",
	Long:         "Run static tests that validate blueprint composition against expected outputs. Tests are defined in contexts/_template/tests/ directory.",
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
