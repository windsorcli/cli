package services

import (
	"errors"
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

	// Set up the mock config handler to return a safe default configuration for Git
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

func TestGitService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and service
		mocks := setupSafeGitServiceMocks()
		gitService := NewGitService(mocks.Injector)

		// When: Initialize is called
		err := gitService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create injector without registering configHandler
		mockInjector := di.NewMockInjector()
		setupSafeGitServiceMocks(mockInjector)
		mockInjector.SetResolveError("configHandler", errors.New("mock error resolving configHandler"))

		// Attempt to create GitService
		gitService := NewGitService(mockInjector)
		if gitService == nil {
			t.Fatalf("expected GitService, got nil")
		}

		// Initialize the service
		err := gitService.Initialize()
		if err == nil {
			t.Fatalf("Expected an error during initialization, got nil")
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create injector and register only configHandler
		mockInjector := di.NewMockInjector()
		setupSafeGitServiceMocks(mockInjector)
		mockInjector.SetResolveError("contextHandler", errors.New("mock error resolving contextHandler"))

		// Attempt to create GitService
		gitService := NewGitService(mockInjector)
		if gitService == nil {
			t.Fatalf("expected GitService, got nil")
		}

		// Initialize the service
		err := gitService.Initialize()
		if err == nil {
			t.Fatalf("Expected an error during initialization, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create injector and register configHandler and context
		mockInjector := di.NewMockInjector()
		setupSafeGitServiceMocks(mockInjector)
		mockInjector.SetResolveError("shell", errors.New("mock error resolving shell"))

		// Attempt to create GitService
		gitService := NewGitService(mockInjector)
		if gitService == nil {
			t.Fatalf("expected GitService, got nil")
		}

		// Initialize the service
		err := gitService.Initialize()
		if err == nil {
			t.Fatalf("Expected an error during initialization, got nil")
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
}
