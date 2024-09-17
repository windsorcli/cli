package di

import (
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/helpers"
)

type Container struct {
	ConfigHandler config.ConfigHandler
	BaseHelper    helpers.Helper
}

func NewContainer() *Container {
	// Initialize the config handler
	configHandler := config.NewViperConfigHandler()

	// Initialize the base helper with the config handler
	baseHelper := helpers.NewBaseHelper(configHandler)

	return &Container{
		ConfigHandler: configHandler,
		BaseHelper:    baseHelper,
	}
}
