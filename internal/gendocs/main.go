// gendocs generates the published reference documentation for the windsor CLI
// by introspecting source-of-truth artifacts (cobra command tree, schemas,
// env-var registry) and emitting markdown under docs/reference/.
//
// Subcommands map 1:1 to reference-page categories:
//
//	commands  cobra → docs/reference/commands/
//
// Future subcommands (schema, env, contexts) follow the same pattern: one file
// per generator, one entry under main(), one Taskfile target.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "gendocs",
		Short: "Generate Windsor CLI reference documentation",
	}
	root.AddCommand(commandsCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
