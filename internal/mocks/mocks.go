package cmd

import (
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

// SuperMocks holds all the mock instances needed for testing commands.
type SuperMocks struct {
	CLIConfigHandler *config.MockConfigHandler
	Shell            *shell.MockShell
	TerraformHelper  *helpers.MockHelper
	AwsHelper        *helpers.MockHelper
	ColimaHelper     *helpers.MockHelper
	DockerHelper     *helpers.MockHelper
	Container        di.ContainerInterface
}

// CreateSuperMocks initializes all necessary mocks and returns them in a SuperMocks struct.
func CreateSuperMocks() SuperMocks {
	// Create mock instances
	mockCLIConfigHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell("cmd")
	mockTerraformHelper := helpers.NewMockHelper()
	mockAwsHelper := helpers.NewMockHelper()
	mockColimaHelper := helpers.NewMockHelper()
	mockDockerHelper := helpers.NewMockHelper()

	// Create and setup the dependency injection container
	container := di.NewContainer()
	container.Register("cliConfigHandler", mockCLIConfigHandler)
	container.Register("context", context.NewMockContext())
	container.Register("shell", mockShell)
	container.Register("terraformHelper", mockTerraformHelper)
	container.Register("awsHelper", mockAwsHelper)
	container.Register("colimaHelper", mockColimaHelper)
	container.Register("dnsHelper", helpers.NewMockHelper())
	container.Register("dockerHelper", mockDockerHelper)
	container.Register("gitHelper", helpers.NewMockHelper())
	container.Register("kubeHelper", helpers.NewMockHelper())
	container.Register("omniHelper", helpers.NewMockHelper())
	container.Register("talosHelper", helpers.NewMockHelper())
	container.Register("terraformHelper", helpers.NewMockHelper())

	return SuperMocks{
		CLIConfigHandler: mockCLIConfigHandler,
		Shell:            mockShell,
		TerraformHelper:  mockTerraformHelper,
		AwsHelper:        mockAwsHelper,
		ColimaHelper:     mockColimaHelper,
		DockerHelper:     mockDockerHelper,
		Container:        container,
	}
}
