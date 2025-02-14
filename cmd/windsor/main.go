package main

import (
	"fmt"
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

	// Initialize components
	if err := controller.InitializeComponents(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing components: %v\n", err)
		os.Exit(1)
	}

	// Execute the root command and handle the error,
	// exiting with a non-zero exit code if there's an error
	if err := cmd.Execute(controller); err != nil {
		os.Exit(1)
	}
}
