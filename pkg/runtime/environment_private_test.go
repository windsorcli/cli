package runtime

import (
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestRuntime_initializeEnvPrinters(t *testing.T) {
	t.Run("InitializesAwsEnvWhenAWSConfigPresent", func(t *testing.T) {
		// Given a runtime with an AWS config block (aws.enabled flag is not required)
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				AWS: &cliconfig.AWS{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then AWS env printer should be initialized

		if rt.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be initialized")
		}
	})

	t.Run("InitializesAwsEnvWhenPlatformIsAws", func(t *testing.T) {
		// Given a runtime with platform=aws and no aws block
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "aws"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then AWS env printer should be initialized from the platform signal alone
		if rt.EnvPrinters.AwsEnv == nil {
			t.Error("Expected AwsEnv to be initialized when platform=aws")
		}
	})

	t.Run("InitializesAzureEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with Azure enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "azure.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				Azure: &cliconfig.Azure{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Azure env printer should be initialized

		if rt.EnvPrinters.AzureEnv == nil {
			t.Error("Expected AzureEnv to be initialized")
		}
	})

	t.Run("InitializesGcpEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with a gcp: config block present
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				GCP: &cliconfig.GCP{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized

		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized")
		}
	})

	t.Run("InitializesGcpEnvWhenPlatformIsGcp", func(t *testing.T) {
		// Given a runtime with platform=gcp and no gcp block
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "gcp"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then GCP env printer should be initialized from the platform signal alone
		if rt.EnvPrinters.GcpEnv == nil {
			t.Error("Expected GcpEnv to be initialized when platform=gcp")
		}
	})

	t.Run("InitializesDockerEnvWhenPlatformDockerAndWorkstationRuntimeDockerDesktop", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "docker"
			}
			if key == "workstation.runtime" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized when platform=docker and workstation.runtime=docker-desktop")
		}
	})

	t.Run("InitializesDockerEnvWhenPlatformDockerAndWorkstationRuntimeColima", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			if key == "platform" {
				return "docker"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.DockerEnv == nil {
			t.Error("Expected DockerEnv to be initialized when platform=docker and workstation.runtime=colima even if docker.enabled=false")
		}
	})

	t.Run("InitializesKubeEnvWhenCloudDriverSet", func(t *testing.T) {
		// Given a runtime with cluster driver set
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Kube env printer should be initialized

		if rt.EnvPrinters.KubeEnv == nil {
			t.Error("Expected KubeEnv to be initialized")
		}
	})

	t.Run("InitializesTalosEnvWhenDriverIsTalos", func(t *testing.T) {
		// Given a runtime with Talos cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Talos env printer should be initialized

		if rt.EnvPrinters.TalosEnv == nil {
			t.Error("Expected TalosEnv to be initialized")
		}
	})

	t.Run("SkipsTalosEnvWhenDriverIsNotTalos", func(t *testing.T) {
		// Given a runtime with non-talos cluster driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "eks"
			}
			return ""
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Talos env printer should not be initialized
		if rt.EnvPrinters.TalosEnv != nil {
			t.Error("Expected TalosEnv to be nil for non-talos cluster driver")
		}
	})

	t.Run("InitializesKubeEnvWhenDriverSet", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "gke"
			}
			return ""
		}

		rt.initializeEnvPrinters()

		if rt.EnvPrinters.KubeEnv == nil {
			t.Error("Expected KubeEnv to be initialized when cluster.driver is set")
		}
	})

	t.Run("InitializesTerraformEnvWhenEnabled", func(t *testing.T) {
		// Given a runtime with Terraform enabled
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "terraform.enabled" {
				return true
			}
			return false
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Terraform env printer should be initialized

		if rt.EnvPrinters.TerraformEnv == nil {
			t.Error("Expected TerraformEnv to be initialized")
		}
	})

	t.Run("InitializesWindsorEnvAlways", func(t *testing.T) {
		// Given a runtime with mocks
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Windsor env printer should always be initialized

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized")
		}
	})

	t.Run("PanicsWhenShellIsNil", func(t *testing.T) {
		// Given a runtime with nil shell
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.Shell = nil

		// When initializeEnvPrinters is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when Shell is nil")
			}
		}()
		rt.initializeEnvPrinters()
	})

	t.Run("PanicsWhenConfigHandlerIsNil", func(t *testing.T) {
		// Given a runtime with nil config handler
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		rt.ConfigHandler = nil

		// When initializeEnvPrinters is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when ConfigHandler is nil")
			}
		}()
		rt.initializeEnvPrinters()
	})

	t.Run("DoesNotOverrideExistingPrinters", func(t *testing.T) {
		// Given a runtime with an existing env printer and an AWS config block that would
		// otherwise trigger the gate (presence of an aws: block activates AWS injection now
		// that aws.enabled has been removed).
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingAwsEnv := env.NewMockEnvPrinter()
		rt.EnvPrinters.AwsEnv = existingAwsEnv

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.GetHeaderFunc = func() *cliconfig.Context {
			return &cliconfig.Context{
				AWS: &cliconfig.AWS{},
			}
		}

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then existing printer should be preserved

		if rt.EnvPrinters.AwsEnv != existingAwsEnv {
			t.Error("Expected existing AwsEnv to be preserved")
		}
	})

	t.Run("InitializesWindsorEnvWithResolver", func(t *testing.T) {
		// Given a runtime with a Resolver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		mockSopsProvider := secrets.NewMockProvider()
		mockOnepasswordProvider := secrets.NewMockProvider()
		rt.Resolver = secrets.NewResolver([]secrets.Provider{mockSopsProvider, mockOnepasswordProvider}, mocks.Shell)

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then Windsor env printer should be initialized with Resolver

		if rt.EnvPrinters.WindsorEnv == nil {
			t.Error("Expected WindsorEnv to be initialized with Resolver")
		}
	})

	t.Run("AlwaysInitializesDotEnvEnv", func(t *testing.T) {
		// Given a runtime with no platform-specific configuration at all
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then DotEnvEnv should be initialized unconditionally
		if rt.EnvPrinters.DotEnvEnv == nil {
			t.Error("Expected DotEnvEnv to always be initialized")
		}
	})

	t.Run("DoesNotOverrideExistingDotEnvEnv", func(t *testing.T) {
		// Given a runtime with an existing DotEnvEnv printer
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime

		existingDotEnvEnv := env.NewMockEnvPrinter()
		rt.EnvPrinters.DotEnvEnv = existingDotEnvEnv

		// When initializeEnvPrinters is called
		rt.initializeEnvPrinters()

		// Then the existing printer should be preserved
		if rt.EnvPrinters.DotEnvEnv != existingDotEnvEnv {
			t.Error("Expected existing DotEnvEnv to be preserved")
		}
	})
}

// TestRuntime_getAllEnvPrinters tests the ordering returned by getAllEnvPrinters.
func TestRuntime_getAllEnvPrinters(t *testing.T) {
	t.Run("DotEnvEnvIsFirst", func(t *testing.T) {
		// Given a runtime with all env printers initialized
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.initializeEnvPrinters()

		// When getAllEnvPrinters is called
		printers := rt.getAllEnvPrinters()

		// Then DotEnvEnv should be first (lowest precedence) and WindsorEnv last
		if len(printers) == 0 {
			t.Fatal("Expected at least one printer")
		}
		if printers[0] != rt.EnvPrinters.DotEnvEnv {
			t.Error("Expected DotEnvEnv to be first in getAllEnvPrinters")
		}
		if printers[len(printers)-1] != rt.EnvPrinters.WindsorEnv {
			t.Error("Expected WindsorEnv to be last in getAllEnvPrinters")
		}
	})
}
