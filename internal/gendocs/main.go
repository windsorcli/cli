// The gendocs command generates the reference documentation of the Windsor CLI.
// It provides one subcommand for each type of reference page, and writes markdown under docs/reference/.
// The commands subcommand reads the cobra command tree and writes docs/reference/commands/.
// The schema subcommand reads the schemas in pkg/runtime/config/schemas/artifacts/ and writes docs/reference/<name>.md.
// To add a generator, add one file, one AddCommand call in main and one Taskfile target.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// main runs the gendocs root command, and exits with status 1 when the command fails.
func main() {
	root := &cobra.Command{
		Use:   "gendocs",
		Short: "Generate Windsor CLI reference documentation",
	}
	root.AddCommand(commandsCmd())
	root.AddCommand(schemaCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
