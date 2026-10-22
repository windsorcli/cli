// The windsor program is the entry point of the Windsor CLI.
// It runs the root command of the cmd package.

package main

import (
	"os"

	"github.com/windsorcli/cli/cmd"
)

// main runs the root command, and exits with status 1 when the command fails.
func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
