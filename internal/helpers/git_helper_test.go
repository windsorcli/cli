package helpers

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestGitHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell("unix")

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of GitHelper
		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		// When: Initialize is called
		err = gitHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestGitHelper_NewGitHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create DI container without registering cliConfigHandler
		diContainer := di.NewContainer()

		// Attempt to create GitHelper
		_, err := NewGitHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create DI container and register only cliConfigHandler
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Attempt to create GitHelper
		_, err := NewGitHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container and register cliConfigHandler and shell
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// Attempt to create GitHelper
		_, err := NewGitHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestGitHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						RsyncExclude: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE),
						RsyncProtect: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT),
						Username:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME),
						Password:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD),
						WebhookUrl:   ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL),
						Image:        ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE),
						Create:       ptrBool(true),
						VerifySsl:    ptrBool(false),
					},
				},
			}, nil
		}
		mockShell := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		// Create GitHelper
		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		// When: GetComposeConfig is called with git livereload enabled
		composeConfig, err := gitHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: it should return the expected service configuration without livereload.enabled label
		expectedConfig := &types.Config{
			Services: []types.ServiceConfig{
				{
					Name:    "git.test",
					Image:   constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE,
					Restart: "always",
					Environment: map[string]*string{
						"RSYNC_EXCLUDE": strPtr(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE),
						"RSYNC_PROTECT": strPtr(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT),
						"GIT_USERNAME":  strPtr(constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME),
						"GIT_PASSWORD":  strPtr(constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD),
						"VERIFY_SSL":    strPtr("false"),
						"WEBHOOK_URL":   strPtr(constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL),
					},
					Volumes: []types.ServiceVolumeConfig{
						{
							Type:   "bind",
							Source: "${WINDSOR_PROJECT_ROOT}",
							Target: "/repos/mount/project",
						},
					},
					Labels: map[string]string{
						"role":       "git-repository",
						"managed_by": "windsor",
					},
				},
			},
		}
		if !reflect.DeepEqual(composeConfig, expectedConfig) {
			t.Fatalf("expected %v, got %v", expectedConfig, composeConfig)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context error")
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock context error")
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving context") {
			t.Fatalf("expected error retrieving context, got %v", err)
		}
	})

	t.Run("GitLivereloadNotEnabled", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create: ptrBool(false),
					},
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		composeConfig, err := gitHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if composeConfig != nil {
			t.Fatalf("expected nil config, got %v", composeConfig)
		}
	})

	t.Run("ErrorRetrievingRsyncExclude", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:       ptrBool(true),
						RsyncExclude: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving rsync_exclude")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving rsync_exclude") {
			t.Fatalf("expected error retrieving rsync_exclude, got %v", err)
		}
	})

	t.Run("ErrorRetrievingRsyncProtect", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:       ptrBool(true),
						RsyncProtect: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving rsync_protect")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving rsync_protect") {
			t.Fatalf("expected error retrieving rsync_protect, got %v", err)
		}
	})

	t.Run("ErrorRetrievingGitUsername", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:   ptrBool(true),
						Username: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving git username")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving git username") {
			t.Fatalf("expected error retrieving git username, got %v", err)
		}
	})

	t.Run("ErrorRetrievingGitPassword", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:   ptrBool(true),
						Password: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving git password")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving git password") {
			t.Fatalf("expected error retrieving git password, got %v", err)
		}
	})

	t.Run("ErrorRetrievingWebhookUrl", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:     ptrBool(true),
						WebhookUrl: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving webhook url")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving webhook url") {
			t.Fatalf("expected error retrieving webhook url, got %v", err)
		}
	})

	t.Run("ErrorRetrievingVerifySsl", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create:    ptrBool(true),
						VerifySsl: nil,
					},
				},
			}, fmt.Errorf("mock error retrieving verify_ssl")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving verify_ssl") {
			t.Fatalf("expected error retrieving verify_ssl, got %v", err)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create: ptrBool(true),
					},
				},
			}, nil
		}
		mockShell := shell.NewMockShell("unix")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}

func TestGitHelper_NoOpFunctions(t *testing.T) {
	// Create a mock DI container and register necessary components
	diContainer := di.NewContainer()
	mockConfigHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell("unix")
	mockContext := context.NewMockContext()
	diContainer.Register("cliConfigHandler", mockConfigHandler)
	diContainer.Register("shell", mockShell)
	diContainer.Register("context", mockContext)

	// Create GitHelper
	gitHelper, err := NewGitHelper(diContainer)
	if err != nil {
		t.Fatalf("NewGitHelper() error = %v", err)
	}

	t.Run("GetEnvVars", func(t *testing.T) {
		envVars, err := gitHelper.GetEnvVars()
		if envVars == nil || err != nil {
			t.Fatalf("expected non-nil envVars and nil error; got %v, %v", envVars, err)
		}
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		err := gitHelper.PostEnvExec()
		if err != nil {
			t.Fatalf("expected nil; got %v", err)
		}
	})

	t.Run("WriteConfig", func(t *testing.T) {
		err := gitHelper.WriteConfig()
		if err != nil {
			t.Fatalf("expected nil; got %v", err)
		}
	})
}

func TestGitHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of GitHelper
		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		// When: Up is called
		err = gitHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestGitHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of GitHelper
		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		// When: Info is called
		info, err := gitHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
