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
	Injector         *di.MockInjector
	ColimaVirt       *virt.MockVirt
	DockerVirt       *virt.MockVirt
}

// CreateSuperMocks initializes all necessary mocks and returns them in a SuperMocks struct.
// It can take an optional mockInjector, in which case it will use this one instead of creating a new DI injector.
func CreateSuperMocks(mockInjector ...*di.MockInjector) SuperMocks {
	var injector *di.MockInjector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
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
	mockCLIConfigHandler.GetConfigFunc = func() (*config.Context, error) { return &config.Context{}, nil }

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
	mockSecureShell := shell.NewMockShell(injector)
	mockSSHClient := &ssh.MockClient{}

	mockColimaVirt := virt.NewMockVirt()
	mockColimaVirt.WriteConfigFunc = func() error { return nil }

	mockDockerVirt := virt.NewMockVirt()
	mockDockerVirt.WriteConfigFunc = func() error { return nil }

	// Create mock environment instances
	mockAwsEnv := env.NewMockEnv(injector)
	mockDockerEnv := env.NewMockEnv(injector)
	mockKubeEnv := env.NewMockEnv(injector)
	mockOmniEnv := env.NewMockEnv(injector)
	mockSopsEnv := env.NewMockEnv(injector)
	mockTalosEnv := env.NewMockEnv(injector)
	mockTerraformEnv := env.NewMockEnv(injector)
	mockWindsorEnv := env.NewMockEnv(injector)

	// Create and setup dependency injection
	injector.Register("cliConfigHandler", mockCLIConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("awsHelper", mockAwsHelper)
	injector.Register("dnsHelper", mockDnsHelper)
	injector.Register("dockerHelper", mockDockerHelper)
	injector.Register("gitHelper", mockGitHelper)
	injector.Register("kubeHelper", mockKubeHelper)
	injector.Register("omniHelper", mockOmniHelper)
	injector.Register("talosHelper", mockTalosHelper)
	injector.Register("sshClient", mockSSHClient)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("colimaVirt", mockColimaVirt)
	injector.Register("dockerVirt", mockDockerVirt)
	injector.Register("awsEnv", mockAwsEnv)
	injector.Register("dockerEnv", mockDockerEnv)
	injector.Register("kubeEnv", mockKubeEnv)
	injector.Register("omniEnv", mockOmniEnv)
	injector.Register("sopsEnv", mockSopsEnv)
	injector.Register("talosEnv", mockTalosEnv)
	injector.Register("terraformEnv", mockTerraformEnv)
	injector.Register("windsorEnv", mockWindsorEnv)

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
		Injector:         injector,
		ColimaVirt:       mockColimaVirt,
		DockerVirt:       mockDockerVirt,
	}
}
