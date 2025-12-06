package services

import (
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
		mocks := setupServicesMocks(t)

		// When a new GitLivereloadService is created
		gitLivereloadService := NewGitLivereloadService(mocks.Runtime)

		// Then the GitService should not be nil
		if gitLivereloadService == nil {
			t.Fatalf("expected GitLivereloadService, got nil")
		}

	})
}

func TestGitLivereloadService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupServicesMocks(t)
		gitLivereloadService := NewGitLivereloadService(mocks.Runtime)

		// Set the service name
		gitLivereloadService.SetName("git")

		// When GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then verify the configuration contains the expected service
		expectedName := "git"
		expectedImage := constants.DefaultGitLiveReloadImage
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

	t.Run("SuccessWithRsyncInclude", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service with rsync_include configured
		mocks := setupServicesMocks(t)
		gitLivereloadService := NewGitLivereloadService(mocks.Runtime)

		// Set the service name
		gitLivereloadService.SetName("git")

		// Configure rsync_include in the mock config
		mocks.ConfigHandler.Set("git.livereload.rsync_include", "kustomize")

		// When GetComposeConfig is called
		composeConfig, err := gitLivereloadService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then verify the configuration contains the expected service with RSYNC_INCLUDE
		expectedName := "git"
		serviceFound := false
		rsyncIncludeFound := false

		for _, service := range composeConfig.Services {
			if service.Name == expectedName {
				serviceFound = true
				// Check if RSYNC_INCLUDE environment variable is set
				if rsyncInclude, exists := service.Environment["RSYNC_INCLUDE"]; exists && rsyncInclude != nil {
					if *rsyncInclude == "kustomize" {
						rsyncIncludeFound = true
					}
				}
				break
			}
		}

		if !serviceFound {
			t.Errorf("expected service with name %q to be in the list of configurations", expectedName)
		}
		if !rsyncIncludeFound {
			t.Errorf("expected RSYNC_INCLUDE environment variable to be set to 'kustomize'")
		}
	})

	t.Run("SuccessWithoutRsyncInclude", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service without rsync_include configured
		mocks := setupServicesMocks(t)
		gitLivereloadService := NewGitLivereloadService(mocks.Runtime)

		// Set the service name
		gitLivereloadService.SetName("git")

		// When GetComposeConfig is called (using default empty rsync_include)
		composeConfig, err := gitLivereloadService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then verify the configuration contains the expected service with default RSYNC_INCLUDE
		expectedName := "git"
		serviceFound := false
		rsyncIncludeFound := false

		for _, service := range composeConfig.Services {
			if service.Name == expectedName {
				serviceFound = true
				// Check if RSYNC_INCLUDE environment variable is set to default value
				if rsyncInclude, exists := service.Environment["RSYNC_INCLUDE"]; exists && rsyncInclude != nil {
					if *rsyncInclude == "kustomize" {
						rsyncIncludeFound = true
					}
				}
				break
			}
		}

		if !serviceFound {
			t.Errorf("expected service with name %q to be in the list of configurations", expectedName)
		}
		if !rsyncIncludeFound {
			t.Errorf("expected RSYNC_INCLUDE environment variable to be set to default value 'kustomize'")
		}
	})
}
