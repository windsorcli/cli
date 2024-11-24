package mocks

import (
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/ssh"
	"github.com/windsor-hotel/cli/internal/virt"
)

// SuperMocks holds all the mock instances needed for testing commands.
type SuperMocks struct {
	ConfigHandler        *config.MockConfigHandler
	ContextHandler       *context.MockContext
	Shell                *shell.MockShell
	AwsHelper            *helpers.MockHelper
	ColimaHelper         *helpers.MockHelper
	DockerHelper         *helpers.MockHelper
	DnsHelper            *helpers.MockHelper
	GitHelper            *helpers.MockHelper
	KubeHelper           *helpers.MockHelper
	OmniHelper           *helpers.MockHelper
	TalosHelper          *helpers.MockHelper
	AwsEnv               *env.MockEnvPrinter
	DockerEnv            *env.MockEnvPrinter
	KubeEnv              *env.MockEnvPrinter
	OmniEnv              *env.MockEnvPrinter
	SopsEnv              *env.MockEnvPrinter
	TalosEnv             *env.MockEnvPrinter
	TerraformEnv         *env.MockEnvPrinter
	WindsorEnv           *env.MockEnvPrinter
	SSHClient            *ssh.MockClient
	SecureShell          *shell.MockShell
	Injector             *di.MockInjector
	ColimaVirt           *virt.MockVirt
	DockerVirt           *virt.MockVirt
	ColimaNetworkManager *network.MockNetworkManager
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
	mockConfigHandler := config.NewMockConfigHandler()
	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell()

	// Create mock helper instances
	mockAwsHelper := helpers.NewMockHelper()
	mockColimaHelper := helpers.NewMockHelper()
	mockDockerHelper := helpers.NewMockHelper()
	mockDnsHelper := helpers.NewMockHelper()
	mockGitHelper := helpers.NewMockHelper()
	mockKubeHelper := helpers.NewMockHelper()
	mockOmniHelper := helpers.NewMockHelper()
	mockTalosHelper := helpers.NewMockHelper()
	mockSecureShell := shell.NewMockShell()
	mockSSHClient := &ssh.MockClient{}

	// Create mock virt instances
	mockColimaVirt := virt.NewMockVirt()
	mockDockerVirt := virt.NewMockVirt()

	// Create mock environment instances
	mockAwsEnv := env.NewMockEnvPrinter()
	mockDockerEnv := env.NewMockEnvPrinter()
	mockKubeEnv := env.NewMockEnvPrinter()
	mockOmniEnv := env.NewMockEnvPrinter()
	mockSopsEnv := env.NewMockEnvPrinter()
	mockTalosEnv := env.NewMockEnvPrinter()
	mockTerraformEnv := env.NewMockEnvPrinter()
	mockWindsorEnv := env.NewMockEnvPrinter()

	// Create mock network manager instance
	mockColimaNetworkManager := network.NewMockNetworkManager()

	// Create and setup dependency injection
	injector.Register("configHandler", mockConfigHandler)
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
	injector.Register("colimaNetworkManager", mockColimaNetworkManager)

	return SuperMocks{
		ConfigHandler:        mockConfigHandler,
		ContextHandler:       mockContext,
		Shell:                mockShell,
		AwsHelper:            mockAwsHelper,
		ColimaHelper:         mockColimaHelper,
		DockerHelper:         mockDockerHelper,
		DnsHelper:            mockDnsHelper,
		GitHelper:            mockGitHelper,
		KubeHelper:           mockKubeHelper,
		OmniHelper:           mockOmniHelper,
		TalosHelper:          mockTalosHelper,
		AwsEnv:               mockAwsEnv,
		DockerEnv:            mockDockerEnv,
		KubeEnv:              mockKubeEnv,
		OmniEnv:              mockOmniEnv,
		SopsEnv:              mockSopsEnv,
		TalosEnv:             mockTalosEnv,
		TerraformEnv:         mockTerraformEnv,
		WindsorEnv:           mockWindsorEnv,
		SSHClient:            mockSSHClient,
		SecureShell:          mockSecureShell,
		Injector:             injector,
		ColimaVirt:           mockColimaVirt,
		DockerVirt:           mockDockerVirt,
		ColimaNetworkManager: mockColimaNetworkManager,
	}
}
