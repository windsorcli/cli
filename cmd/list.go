package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd is a top-level shortcut for `windsor get contexts`.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available contexts.",
	Long:  `List contexts in the project. This is a shortcut for 'windsor get contexts' and prints the same table.`,
	Example: `windsor list

# Sample output:
#   NAME    PROVIDER  BACKEND  CURRENT
#   local   docker    <none>   *
#   prod    aws       s3`,
	Annotations: map[string]string{
		"docs.seealso": "[`get contexts`](get-contexts.md), [`set context`](set-context.md)",
		"docs.source": "cmd/list.go",
	},
	SilenceUsage: true,
	RunE:         getContextsCmd.RunE,
}

func init() {
	rootCmd.AddCommand(listCmd)
}
