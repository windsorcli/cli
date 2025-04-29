package main

import (
	"os"

	"github.com/windsorcli/cli/cmd"
)

func main() {
	// Execute the root command and handle the error,
	// exiting with a non-zero exit code if there's an error
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
