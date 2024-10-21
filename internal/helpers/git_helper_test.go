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
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// Attempt to create GitHelper
		_, err := NewGitHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestGitHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			switch key {
			case "contexts.test-context.git.livereload.rsync_exclude":
				return constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE, nil
			case "contexts.test-context.git.livereload.rsync_protect":
				return constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT, nil
			case "contexts.test-context.git.livereload.username":
				return constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME, nil
			case "contexts.test-context.git.livereload.password":
				return constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD, nil
			case "contexts.test-context.git.livereload.webhook_url":
				return constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL, nil
			case "contexts.test-context.git.livereload.image":
				return constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE, nil
			default:
				return "", nil
			}
		}
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			if key == "contexts.test-context.git.livereload.verify_ssl" {
				return false, nil
			}
			return false, nil
		}
		mockShell, _ := shell.NewMockShell("unix")
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

		// When: GetContainerConfig is called with git livereload enabled
		config, err := gitHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: it should return the expected service configuration without livereload.enabled label
		expectedConfig := []types.ServiceConfig{
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
						Source: "/mock/project",
						Target: "/repos/mount/project",
					},
				},
				Labels: map[string]string{
					"role":       "git-repository",
					"managed_by": "windsor",
				},
			},
		}
		if !reflect.DeepEqual(config, expectedConfig) {
			t.Fatalf("expected %v, got %v", expectedConfig, config)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock context error")
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			return "", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving context") {
			t.Fatalf("expected error retrieving context, got %v", err)
		}
	})

	t.Run("ErrorRetrievingEnabledStatus", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return false, fmt.Errorf("mock enabled status error")
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving git livereload enabled status") {
			t.Fatalf("expected error retrieving git livereload enabled status, got %v", err)
		}
	})

	t.Run("GitLivereloadNotEnabled", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return false, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		config, err := gitHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if config != nil {
			t.Fatalf("expected nil config, got %v", config)
		}
	})

	t.Run("ErrorRetrievingRsyncExclude", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.rsync_exclude" {
				return "", fmt.Errorf("mock error retrieving rsync_exclude")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving rsync_exclude") {
			t.Fatalf("expected error retrieving rsync_exclude, got %v", err)
		}
	})

	t.Run("ErrorRetrievingRsyncProtect", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.rsync_protect" {
				return "", fmt.Errorf("mock error retrieving rsync_protect")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving rsync_protect") {
			t.Fatalf("expected error retrieving rsync_protect, got %v", err)
		}
	})

	t.Run("ErrorRetrievingGitUsername", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.username" {
				return "", fmt.Errorf("mock error retrieving git username")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving git username") {
			t.Fatalf("expected error retrieving git username, got %v", err)
		}
	})

	t.Run("ErrorRetrievingGitPassword", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.password" {
				return "", fmt.Errorf("mock error retrieving git password")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving git password") {
			t.Fatalf("expected error retrieving git password, got %v", err)
		}
	})

	t.Run("ErrorRetrievingWebhookUrl", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.webhook_url" {
				return "", fmt.Errorf("mock error retrieving webhook url")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving webhook url") {
			t.Fatalf("expected error retrieving webhook url, got %v", err)
		}
	})

	t.Run("ErrorRetrievingVerifySsl", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			if key == "contexts.test-context.git.livereload.verify_ssl" {
				return false, fmt.Errorf("mock error retrieving verify_ssl")
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving verify_ssl") {
			t.Fatalf("expected error retrieving verify_ssl, got %v", err)
		}
	})

	t.Run("ErrorRetrievingGitLivereloadImage", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			if key == "contexts.test-context.git.livereload.image" {
				return "", fmt.Errorf("mock error retrieving git livereload image")
			}
			return "", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		mockShell, _ := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		gitHelper, err := NewGitHelper(diContainer)
		if err != nil {
			t.Fatalf("NewGitHelper() error = %v", err)
		}

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving git livereload image") {
			t.Fatalf("expected error retrieving git livereload image, got %v", err)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string) (bool, error) {
			if key == "contexts.test-context.git.livereload.enabled" {
				return true, nil
			}
			return false, nil
		}
		mockConfigHandler.GetStringFunc = func(key string) (string, error) {
			switch key {
			case "contexts.test-context.git.livereload.rsync_exclude":
				return constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE, nil
			case "contexts.test-context.git.livereload.rsync_protect":
				return constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT, nil
			case "contexts.test-context.git.livereload.username":
				return constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME, nil
			case "contexts.test-context.git.livereload.password":
				return constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD, nil
			case "contexts.test-context.git.livereload.webhook_url":
				return constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL, nil
			case "contexts.test-context.git.livereload.image":
				return constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE, nil
			default:
				return "", nil
			}
		}
		mockShell, _ := shell.NewMockShell("unix")
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

		_, err = gitHelper.GetContainerConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}

func TestGitHelper_NoOpFunctions(t *testing.T) {
	// Create a mock DI container and register necessary components
	diContainer := di.NewContainer()
	mockConfigHandler := config.NewMockConfigHandler()
	mockShell, _ := shell.NewMockShell("unix")
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
		if envVars != nil || err != nil {
			t.Fatalf("expected nil, nil; got %v, %v", envVars, err)
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
