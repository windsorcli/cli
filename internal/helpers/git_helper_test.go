package helpers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestGitHelper(t *testing.T) {
	t.Run("NewGitHelper", func(t *testing.T) {
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
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("SetEnabledConfigSuccess", func(t *testing.T) {
			// Given: a mock config handler, shell, and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockShell, _ := shell.NewMockShell("unix")
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

			// When: SetConfig is called with "enabled" key
			err = gitHelper.SetConfig("enabled", "true")

			// Then: it should return no error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			enabled, _ := mockConfigHandler.GetConfigValue("contexts.test-context.git.livereload.enabled", "false")
			if enabled != "true" {
				t.Errorf("expected enabled to be true, got %v", enabled)
			}
		})

		t.Run("SetEnabledConfigError", func(t *testing.T) {
			// Given: a mock context that returns an error
			mockContextWithError := context.NewMockContext()
			mockContextWithError.GetContextFunc = func() (string, error) {
				return "", errors.New("error retrieving current context")
			}
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContextWithError)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Create GitHelper
			gitHelper, err := NewGitHelper(diContainer)
			if err != nil {
				t.Fatalf("NewGitHelper() error = %v", err)
			}

			// When: SetConfig is called with "enabled" key
			err = gitHelper.SetConfig("enabled", "true")

			// Then: it should return an error
			expectedError := "error retrieving current context"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler, shell, and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string, defaultValue string) (string, error) {
				return "", nil
			}
			mockShell, _ := shell.NewMockShell("unix")
			if mockShell != nil {
				mockShell.GetProjectRootFunc = func() (string, error) {
					return "/mock/project", nil
				}
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
			mockConfigHandler.SetConfigValue("contexts.test-context.git.livereload.enabled", "true")
			config, err := gitHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: it should return the expected service configuration without livereload.enabled label
			expectedConfig := []types.ServiceConfig{
				{
					Name:    "git.test",
					Image:   DEFAULT_GIT_LIVE_RELOAD_IMAGE,
					Restart: "always",
					Environment: map[string]*string{
						"RSYNC_EXCLUDE": strPtr(".docker-cache,.terraform,data,.venv"),
						"RSYNC_PROTECT": strPtr("flux-system"),
						"GIT_USERNAME":  strPtr("local"),
						"GIT_PASSWORD":  strPtr("local"),
						"VERIFY_SSL":    strPtr("false"),
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
	})
}
