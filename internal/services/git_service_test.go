package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func setupSafeGitServiceMocks(optionalInjector ...di.Injector) *MockComponents {
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
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	// Set up the mock config handler to return minimal configuration for Git
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			Git: &config.GitConfig{
				Livereload: &config.GitLivereloadConfig{
					Create: ptrBool(true),
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

func TestGitService_NewGitService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeGitServiceMocks()

		// When: a new GitService is created
		gitService := NewGitService(mocks.Injector)
		if gitService == nil {
		}

		// Then: the GitService should not be nil
		if gitService == nil {
			t.Fatalf("expected GitService, got nil")
		}

		// And: the GitService should have the correct injector
		if gitService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, gitService.injector)
		}
	})
}

func TestGitService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeGitServiceMocks()
		gitService := NewGitService(mocks.Injector)
		err := gitService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := gitService.GetComposeConfig()
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

	t.Run("ErrorGettingContext", func(t *testing.T) {
		mocks := setupSafeGitServiceMocks()
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context")
		}

		// When: a new GitService is created and initialized
		gitService := NewGitService(mocks.Injector)
		err := gitService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = gitService.GetComposeConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving context") {
			t.Fatalf("expected error retrieving context, got %v", err)
		}
	})

	t.Run("GitNotEnabled", func(t *testing.T) {
		mocks := setupSafeGitServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Git: nil,
			}
		}
		gitService := NewGitService(mocks.Injector)
		err := gitService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := gitService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: verify the configuration is nil
		if composeConfig != nil {
			t.Errorf("expected nil configuration, got %+v", composeConfig)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		mocks := setupSafeGitServiceMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// When: a new GitService is created and initialized
		gitService := NewGitService(mocks.Injector)
		err := gitService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := gitService.GetComposeConfig()

		// Then: verify the configuration is empty and an error should be returned
		if composeConfig != nil && len(composeConfig.Services) > 0 {
			t.Errorf("expected empty configuration, got %+v", composeConfig)
		}
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}
