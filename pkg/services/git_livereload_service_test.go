package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/config/git"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeGitLivereloadServiceMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()
	mockService := NewMockService()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("gitService", mockService)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() string {
		return "mock-context"
	}

	// Set up the mock config handler to return minimal configuration for Git
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
		}
	}

	return &MockComponents{
		Injector:          injector,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
		MockService:       mockService,
	}
}

func TestGitLivereloadService_NewGitLivereloadService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeGitLivereloadServiceMocks()

		// When: a new GitLivereloadService is created
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)
		if gitLivereloadService == nil {
		}

		// Then: the GitService should not be nil
		if gitLivereloadService == nil {
			t.Fatalf("expected GitLivereloadService, got nil")
		}

		// And: the GitService should have the correct injector
		if gitLivereloadService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, gitLivereloadService.injector)
		}
	})
}

func TestGitLivereloadService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeGitLivereloadServiceMocks()
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)
		err := gitLivereloadService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: verify the configuration contains the expected service
		expectedName := "git.test"
		expectedImage := constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE
		serviceFound := false

		for _, service := range composeConfig.Services {
			if service.Name == expectedName && service.Image == expectedImage {
				serviceFound = true
				break
			}
		}

		if !serviceFound {
			t.Errorf("expected service with name %q and image %q to be in the list of configurations:\n%+v", expectedName, expectedImage, composeConfig.Services)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		mocks := setupSafeGitLivereloadServiceMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// When: a new GitService is created and initialized
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)
		err := gitLivereloadService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()

		// Then: verify the configuration is empty and an error should be returned
		if composeConfig != nil && len(composeConfig.Services) > 0 {
			t.Errorf("expected empty configuration, got %+v", composeConfig)
		}
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}
