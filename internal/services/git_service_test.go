package services

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

func TestGitService_NewGitService(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create injector without registering configHandler
		diContainer := di.NewInjector()

		// Attempt to create GitService
		_, err := NewGitService(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create injector and register only configHandler
		diContainer := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("configHandler", mockConfigHandler)

		// Attempt to create GitService
		_, err := NewGitService(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create injector and register configHandler and shell
		diContainer := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)

		// Attempt to create GitService
		_, err := NewGitService(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestGitService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
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
			}
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project", nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewInjector()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		// Create GitService
		gitService, err := NewGitService(diContainer)
		if err != nil {
			t.Fatalf("NewGitService() error = %v", err)
		}

		// When: GetComposeConfig is called with git livereload enabled
		composeConfig, err := gitService.GetComposeConfig()
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
						"context":    "test-context",
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
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return nil
		}
		diContainer := di.NewInjector()
		diContainer.Register("configHandler", mockConfigHandler)
		mockShell := shell.NewMockShell()
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		gitService, err := NewGitService(diContainer)
		if err != nil {
			t.Fatalf("NewGitService() error = %v", err)
		}

		_, err = gitService.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving context") {
			t.Fatalf("expected error retrieving context, got %v", err)
		}
	})

	t.Run("GitLivereloadNotEnabled", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create: ptrBool(false),
					},
				},
			}
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewInjector()
		diContainer.Register("configHandler", mockConfigHandler)
		mockShell := shell.NewMockShell()
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		gitService, err := NewGitService(diContainer)
		if err != nil {
			t.Fatalf("NewGitService() error = %v", err)
		}

		composeConfig, err := gitService.GetComposeConfig()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if composeConfig != nil {
			t.Fatalf("expected nil config, got %v", composeConfig)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Git: &config.GitConfig{
					Livereload: &config.GitLivereloadConfig{
						Create: ptrBool(true),
					},
				},
			}
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer := di.NewInjector()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)

		gitService, err := NewGitService(diContainer)
		if err != nil {
			t.Fatalf("NewGitService() error = %v", err)
		}

		_, err = gitService.GetComposeConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}
