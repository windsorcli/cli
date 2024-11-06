package mocks

import (
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

// SuperMocks holds all the mock instances needed for testing commands.
type SuperMocks struct {
	CLIConfigHandler *config.MockConfigHandler
	ContextInstance  *context.MockContext
	Shell            *shell.MockShell
	AwsHelper        *helpers.MockHelper
	ColimaHelper     *helpers.MockHelper
	DockerHelper     *helpers.MockHelper
	DnsHelper        *helpers.MockHelper
	GitHelper        *helpers.MockHelper
	KubeHelper       *helpers.MockHelper
	OmniHelper       *helpers.MockHelper
	TalosHelper      *helpers.MockHelper
	AwsEnv           *env.MockEnv
	DockerEnv        *env.MockEnv
	KubeEnv          *env.MockEnv
	OmniEnv          *env.MockEnv
	SopsEnv          *env.MockEnv
	TalosEnv         *env.MockEnv
	TerraformEnv     *env.MockEnv
	WindsorEnv       *env.MockEnv
	SSHClient        *ssh.MockClient
	SecureShell      *shell.MockShell
	Container        di.ContainerInterface
	ColimaVirt       *virt.MockVirt
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

	mockContext := context.NewMockContext()
	mockContext.GetContextFunc = func() (string, error) { return "mock-context", nil }
	mockContext.SetContextFunc = func(context string) error { return nil }

	mockShell := shell.NewMockShell()
	mockAwsHelper := helpers.NewMockHelper()
	mockColimaHelper := helpers.NewMockHelper()
	mockDockerHelper := helpers.NewMockHelper()
	mockDnsHelper := helpers.NewMockHelper()
	mockGitHelper := helpers.NewMockHelper()
	mockKubeHelper := helpers.NewMockHelper()
	mockOmniHelper := helpers.NewMockHelper()
	mockTalosHelper := helpers.NewMockHelper()
	mockSecureShell := shell.NewMockShell(container)
	mockSSHClient := &ssh.MockClient{}
	colimaVirt := virt.NewMockVirt()

	// Create mock environment instances
	mockAwsEnv := env.NewMockEnv(container)
	mockDockerEnv := env.NewMockEnv(container)
	mockKubeEnv := env.NewMockEnv(container)
	mockOmniEnv := env.NewMockEnv(container)
	mockSopsEnv := env.NewMockEnv(container)
	mockTalosEnv := env.NewMockEnv(container)
	mockTerraformEnv := env.NewMockEnv(container)
	mockWindsorEnv := env.NewMockEnv(container)

	// Create and setup the dependency injection container
	container.Register("cliConfigHandler", mockCLIConfigHandler)
	container.Register("contextHandler", mockContext)
	container.Register("shell", mockShell)
	container.Register("awsHelper", mockAwsHelper)
	container.Register("dnsHelper", mockDnsHelper)
	container.Register("dockerHelper", mockDockerHelper)
	container.Register("gitHelper", mockGitHelper)
	container.Register("kubeHelper", mockKubeHelper)
	container.Register("omniHelper", mockOmniHelper)
	container.Register("talosHelper", mockTalosHelper)
	container.Register("sshClient", mockSSHClient)
	container.Register("secureShell", mockSecureShell)
	container.Register("colimaVirt", colimaVirt)
	container.Register("awsEnv", mockAwsEnv)
	container.Register("dockerEnv", mockDockerEnv)
	container.Register("kubeEnv", mockKubeEnv)
	container.Register("omniEnv", mockOmniEnv)
	container.Register("sopsEnv", mockSopsEnv)
	container.Register("talosEnv", mockTalosEnv)
	container.Register("terraformEnv", mockTerraformEnv)
	container.Register("windsorEnv", mockWindsorEnv)

	return SuperMocks{
		CLIConfigHandler: mockCLIConfigHandler,
		ContextInstance:  mockContext,
		Shell:            mockShell,
		AwsHelper:        mockAwsHelper,
		ColimaHelper:     mockColimaHelper,
		DockerHelper:     mockDockerHelper,
		DnsHelper:        mockDnsHelper,
		GitHelper:        mockGitHelper,
		KubeHelper:       mockKubeHelper,
		OmniHelper:       mockOmniHelper,
		TalosHelper:      mockTalosHelper,
		AwsEnv:           mockAwsEnv,
		DockerEnv:        mockDockerEnv,
		KubeEnv:          mockKubeEnv,
		OmniEnv:          mockOmniEnv,
		SopsEnv:          mockSopsEnv,
		TalosEnv:         mockTalosEnv,
		TerraformEnv:     mockTerraformEnv,
		WindsorEnv:       mockWindsorEnv,
		SSHClient:        mockSSHClient,
		SecureShell:      mockSecureShell,
		Container:        container,
		ColimaVirt:       colimaVirt,
	}
}
