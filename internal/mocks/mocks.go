package mocks

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
	AwsHelper        *helpers.AwsHelper
	ColimaHelper     *helpers.ColimaHelper
	DockerHelper     *helpers.DockerHelper
	DnsHelper        *helpers.DNSHelper
	GitHelper        *helpers.GitHelper
	KubeHelper       *helpers.KubeHelper
	OmniHelper       *helpers.OmniHelper
	TalosHelper      *helpers.TalosHelper
	TerraformHelper  *helpers.TerraformHelper
	Container        di.ContainerInterface
}

// CreateSuperMocks initializes all necessary mocks and returns them in a SuperMocks struct.
// It can take an optional mockContainer, in which case it will use this one instead of creating a new DI container.
func CreateSuperMocks(mockContainer ...di.ContainerInterface) SuperMocks {
	var container di.ContainerInterface
	if len(mockContainer) > 0 {
		container = mockContainer[0]
	} else {
		container = di.NewContainer()
	}

	// Create mock instances
	mockCLIConfigHandler := config.NewMockConfigHandler()
	mockCLIConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) { return "mock-value", nil }
	mockCLIConfigHandler.GetIntFunc = func(key string, defaultValue ...int) (int, error) { return 0, nil }
	mockCLIConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) { return false, nil }
	mockCLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	mockCLIConfigHandler.SaveConfigFunc = func(path string) error { return nil }
	mockCLIConfigHandler.GetFunc = func(key string) (interface{}, error) { return nil, nil }
	mockCLIConfigHandler.SetDefaultFunc = func(context config.Context) error { return nil }
	mockCLIConfigHandler.GetConfigFunc = func() (*config.Context, error) { return nil, nil }

	mockShell := shell.NewMockShell("cmd")
	mockAwsHelper := helpers.NewMockHelper()
	mockColimaHelper := helpers.NewMockHelper()
	mockDockerHelper := helpers.NewMockHelper()
	mockDnsHelper := helpers.NewMockHelper()
	mockGitHelper := helpers.NewMockHelper()
	mockKubeHelper := helpers.NewMockHelper()
	mockOmniHelper := helpers.NewMockHelper()
	mockTalosHelper := helpers.NewMockHelper()
	mockTerraformHelper := helpers.NewMockHelper()

	// Create and setup the dependency injection container
	container.Register("cliConfigHandler", mockCLIConfigHandler)
	container.Register("context", context.NewMockContext())
	container.Register("shell", mockShell)
	container.Register("awsHelper", mockAwsHelper)
	container.Register("colimaHelper", mockColimaHelper)
	container.Register("dnsHelper", mockDnsHelper)
	container.Register("dockerHelper", mockDockerHelper)
	container.Register("gitHelper", mockGitHelper)
	container.Register("kubeHelper", mockKubeHelper)
	container.Register("omniHelper", mockOmniHelper)
	container.Register("talosHelper", mockTalosHelper)
	container.Register("terraformHelper", mockTerraformHelper)

	return SuperMocks{
		CLIConfigHandler: mockCLIConfigHandler,
		Shell:            mockShell,
		AwsHelper:        mockAwsHelper,
		ColimaHelper:     mockColimaHelper,
		DockerHelper:     mockDockerHelper,
		DnsHelper:        mockDnsHelper,
		GitHelper:        mockGitHelper,
		KubeHelper:       mockKubeHelper,
		OmniHelper:       mockOmniHelper,
		TalosHelper:      mockTalosHelper,
		TerraformHelper:  mockTerraformHelper,
		Container:        container,
	}
}
