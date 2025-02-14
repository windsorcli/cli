package main

import (
	"os"

	"github.com/windsorcli/cli/cmd"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

func main() {
	// Create a new dependency injector
	injector := di.NewInjector()

	// Create a new controller
	controller := controller.NewRealController(injector)

	// Execute the root command and handle the error,
	// exiting with a non-zero exit code if there's an error
	if err := cmd.Execute(controller); err != nil {
		os.Exit(1)
	}
}
