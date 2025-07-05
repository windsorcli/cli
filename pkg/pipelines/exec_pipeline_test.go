package pipelines

import (
	"context"
	"fmt"
	"os"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestExecPipeline_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		pipeline := NewExecPipeline()

		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ConfigHandlerInitializationError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler initialization failed")
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to initialize config handler: config handler initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ShellInitializationError", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to initialize shell: shell initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SecretsProviderInitializationError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
		mockSecretsProvider.InitializeFunc = func() error {
			return fmt.Errorf("secrets provider initialization failed")
		}

		mockShims := &Shims{
			Stat: func(name string) (os.FileInfo, error) {
				if name == "/test/config/secrets.enc.yaml" {
					return nil, nil
				}
				return nil, os.ErrNotExist
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewSopsSecretsProvider: func(string, di.Injector) secrets.SecretsProvider {
				return mockSecretsProvider
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to initialize secrets provider: secrets provider initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("EnvPrinterInitializationError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "aws.enabled"
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("env printer initialization failed")
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewAwsEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to initialize env printer: env printer initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ReuseExistingComponents", func(t *testing.T) {
		injector := di.NewInjector()
		existingShell := shell.NewMockShell()
		existingConfigHandler := config.NewMockConfigHandler()

		injector.Register("shell", existingShell)
		injector.Register("configHandler", existingConfigHandler)

		pipeline := NewExecPipeline()
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if pipeline.shell != existingShell {
			t.Error("Expected to reuse existing shell")
		}

		if pipeline.configHandler != existingConfigHandler {
			t.Error("Expected to reuse existing config handler")
		}
	})
}

func TestExecPipeline_Execute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "output", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")
		ctx = context.WithValue(ctx, "args", []string{"arg1", "arg2"})

		err = pipeline.Execute(ctx)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		injector := di.NewInjector()
		pipeline := NewExecPipeline()
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "no command provided"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SecretsLoadError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets load failed")
		}

		mockShims := &Shims{
			Stat: func(name string) (os.FileInfo, error) {
				if name == "/test/config/secrets.enc.yaml" {
					return nil, nil
				}
				return nil, os.ErrNotExist
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewSopsSecretsProvider: func(string, di.Injector) secrets.SecretsProvider {
				return mockSecretsProvider
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "error loading secrets: secrets load failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("get env vars failed")
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "error getting environment variables: get env vars failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook failed")
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "error executing PostEnvHook: post env hook failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetenvError", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}

		mockShims := &Shims{
			Setenv: func(key, value string) error {
				return fmt.Errorf("setenv failed")
			},
			Stat: func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "error setting environment variable \"TEST_VAR\": setenv failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ShellExecError", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("exec failed")
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(di.Injector) shell.Shell {
				return mockShell
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return mockEnvPrinter
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)
		if err != nil {
			t.Fatalf("Failed to initialize pipeline: %v", err)
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "command", "test-command")
		ctx = context.WithValue(ctx, "args", []string{"arg1"})

		err = pipeline.Execute(ctx)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "command execution failed: exec failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func TestExecPipeline_createSecretsProviders(t *testing.T) {
	t.Run("NoSecretsFiles", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		mockShims := &Shims{
			Stat: func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(pipeline.secretsProviders) != 0 {
			t.Errorf("Expected 0 secrets providers, got %d", len(pipeline.secretsProviders))
		}
	})

	t.Run("SopsSecretsProvider", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}

		mockShims := &Shims{
			Stat: func(name string) (os.FileInfo, error) {
				if name == "/test/config/secrets.enc.yaml" {
					return nil, nil
				}
				return nil, os.ErrNotExist
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewSopsSecretsProvider: func(string, di.Injector) secrets.SecretsProvider {
				return secrets.NewMockSecretsProvider(injector)
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(pipeline.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(pipeline.secretsProviders))
		}
	})

	t.Run("OnePasswordSecretsProvider", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/test/config", nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test"
		}
		mockConfigHandler.GetFunc = func(key string) any {
			if key == "contexts.test.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {URL: "https://test.1password.com"},
				}
			}
			return nil
		}

		mockShims := &Shims{
			Stat: func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
			Getenv: func(key string) string {
				if key == "OP_SERVICE_ACCOUNT_TOKEN" {
					return "token"
				}
				return ""
			},
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewOnePasswordSDKSecretsProvider: func(secretsConfigType.OnePasswordVault, di.Injector) secrets.SecretsProvider {
				return secrets.NewMockSecretsProvider(injector)
			},
			NewShims: func() *Shims {
				return mockShims
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(pipeline.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(pipeline.secretsProviders))
		}
	})
}

func TestExecPipeline_createEnvPrinters(t *testing.T) {
	t.Run("AllEnvPrintersEnabled", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewAwsEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewAzureEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewDockerEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewKubeEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewOmniEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewTalosEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewTerraformEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedCount := 8
		if len(pipeline.envPrinters) != expectedCount {
			t.Errorf("Expected %d env printers, got %d", expectedCount, len(pipeline.envPrinters))
		}
	})

	t.Run("OnlyWindsorEnvPrinter", func(t *testing.T) {
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}

		constructors := ExecConstructors{
			NewConfigHandler: func(di.Injector) config.ConfigHandler {
				return mockConfigHandler
			},
			NewShell: func(injector di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
			NewWindsorEnvPrinter: func(di.Injector) env.EnvPrinter {
				return env.NewMockEnvPrinter()
			},
			NewShims: func() *Shims {
				return NewShims()
			},
		}

		pipeline := NewExecPipeline(constructors)
		err := pipeline.Initialize(injector)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(pipeline.envPrinters) != 1 {
			t.Errorf("Expected 1 env printer, got %d", len(pipeline.envPrinters))
		}
	})
}
