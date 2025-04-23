package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// The GitLivereloadServiceTest is a test suite for the GitLivereloadService implementation
// It provides comprehensive test coverage for Git repository synchronization and live reload
// The GitLivereloadServiceTest ensures proper service configuration and error handling
// enabling reliable Git repository management in the Windsor CLI

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGitLivereloadService_NewGitLivereloadService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupMocks(t)

		// When a new GitLivereloadService is created
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)

		// Then the GitService should not be nil
		if gitLivereloadService == nil {
			t.Fatalf("expected GitLivereloadService, got nil")
		}

		// And the GitService should have the correct injector
		if gitLivereloadService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, gitLivereloadService.injector)
		}
	})
}

func TestGitLivereloadService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupMocks(t)
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)
		err := gitLivereloadService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Set the service name
		gitLivereloadService.SetName("git")

		// When GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then verify the configuration contains the expected service
		expectedName := "git"
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
		// Given a mock config handler with error on GetProjectRoot
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// And a new GitService is created and initialized
		gitLivereloadService := NewGitLivereloadService(mocks.Injector)
		err := gitLivereloadService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()

		// Then verify the configuration is empty and an error should be returned
		if composeConfig != nil && len(composeConfig.Services) > 0 {
			t.Errorf("expected empty configuration, got %+v", composeConfig)
		}
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})
}
