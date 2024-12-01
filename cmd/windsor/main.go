package main

import (
	"github.com/windsor-hotel/cli/cmd"
	"github.com/windsor-hotel/cli/internal/controller"
	"github.com/windsor-hotel/cli/internal/di"
)

func main() {
	// Create a new DI injector
	injector := di.NewInjector()

	// Create a new controller
	controller := controller.NewController(injector)

	// Execute the root command and handle the error silently,
	// allowing the CLI framework to report the error
	_ = cmd.Execute(controller)
}
