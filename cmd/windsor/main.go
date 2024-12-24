package main

import (
	"github.com/windsorcli/cli/cmd"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

func main() {
	// Create a new dependency injector
	injector := di.NewInjector()

	// Create a new controller
	controller := controller.NewRealController(injector)

	// Execute the root command and handle the error silently,
	// allowing the CLI framework to report the error
	_ = cmd.Execute(controller)
}
