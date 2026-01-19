package services

import (
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

func TestGitLivereloadService_GetIncusConfig(t *testing.T) {
	setup := func(t *testing.T) (*GitLivereloadService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupServicesMocks(t)
		service := NewGitLivereloadService(mocks.Runtime)
		service.SetName("git")
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a GitLivereloadService with mock components
		service, _ := setup(t)

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the Incus configuration should be correctly populated
		if incusConfig == nil {
			t.Fatalf("expected non-nil incusConfig, got %v", incusConfig)
		}
		if incusConfig.Type != "container" {
			t.Errorf("expected Type to be 'container', got %q", incusConfig.Type)
		}
		expectedImage := constants.DefaultGitLiveReloadImage
		if strings.HasPrefix(expectedImage, "ghcr.io/") {
			expectedImage = "ghcr:" + strings.TrimPrefix(expectedImage, "ghcr.io/")
		}
		if incusConfig.Image != expectedImage {
			t.Errorf("expected Image to be %q, got %q", expectedImage, incusConfig.Image)
		}
		if incusConfig.Config["environment.RSYNC_INCLUDE"] != constants.DefaultGitLiveReloadRsyncInclude {
			t.Errorf("expected RSYNC_INCLUDE to be %q, got %q", constants.DefaultGitLiveReloadRsyncInclude, incusConfig.Config["environment.RSYNC_INCLUDE"])
		}
		projectRootDevice, exists := incusConfig.Devices["project-root"]
		if !exists {
			t.Fatalf("expected 'project-root' device to exist")
		}
		if projectRootDevice["type"] != "disk" {
			t.Errorf("expected project-root device type to be 'disk', got %q", projectRootDevice["type"])
		}
	})

	t.Run("WithGhcrImage", func(t *testing.T) {
		// Given a GitLivereloadService with ghcr.io image
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("git.livereload.image", "ghcr.io/test/image:latest")

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the image should be converted to ghcr: format
		if incusConfig.Image != "ghcr:test/image:latest" {
			t.Errorf("expected Image to be 'ghcr:test/image:latest', got %q", incusConfig.Image)
		}
	})

	t.Run("WithWebhookUrl", func(t *testing.T) {
		// Given a GitLivereloadService with webhook URL configured
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("git.livereload.webhook_url", "https://example.com/webhook")

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the webhook URL should be in the config
		if incusConfig.Config["environment.WEBHOOK_URL"] != "https://example.com/webhook" {
			t.Errorf("expected WEBHOOK_URL to be 'https://example.com/webhook', got %q", incusConfig.Config["environment.WEBHOOK_URL"])
		}
	})

	t.Run("WithDockerHubImage", func(t *testing.T) {
		// Given a GitLivereloadService with Docker Hub image
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("git.livereload.image", "myuser/myimage:latest")

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}

		// Then the image should have docker: prefix
		if incusConfig.Image != "docker:myuser/myimage:latest" {
			t.Errorf("expected Image to be 'docker:myuser/myimage:latest', got %q", incusConfig.Image)
		}
	})

}

func TestGitLivereloadService_computeDefaultWebhookURL(t *testing.T) {
	setup := func(t *testing.T) (*GitLivereloadService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupServicesMocks(t)
		service := NewGitLivereloadService(mocks.Runtime)
		service.SetName("git")
		return service, mocks
	}

	t.Run("UsesLoadBalancerWhenConfigured", func(t *testing.T) {
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("network.loadbalancer_ips.start", "10.5.1.1")

		url := service.computeDefaultWebhookURL()

		if !strings.Contains(url, "10.5.1.1:9292") {
			t.Errorf("expected URL to contain '10.5.1.1:9292', got %q", url)
		}
	})

	t.Run("UsesNodePortWhenNoLoadBalancer", func(t *testing.T) {
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "test")

		url := service.computeDefaultWebhookURL()

		if !strings.Contains(url, ":30292") {
			t.Errorf("expected URL to contain ':30292', got %q", url)
		}
	})

	t.Run("UsesControlplaneWhenNoWorkers", func(t *testing.T) {
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "test")
		mocks.ConfigHandler.Set("cluster.workers.count", 0)

		url := service.computeDefaultWebhookURL()

		if !strings.Contains(url, "controlplane-1.test") {
			t.Errorf("expected URL to contain 'controlplane-1.test', got %q", url)
		}
	})

	t.Run("UsesWorkerWhenWorkersExist", func(t *testing.T) {
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "test")
		mocks.ConfigHandler.Set("cluster.workers.count", 1)

		url := service.computeDefaultWebhookURL()

		if !strings.Contains(url, "worker-1.test") {
			t.Errorf("expected URL to contain 'worker-1.test', got %q", url)
		}
	})

	t.Run("UsesCustomDomain", func(t *testing.T) {
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "mydomain.local")
		mocks.ConfigHandler.Set("cluster.workers.count", 0)

		url := service.computeDefaultWebhookURL()

		if !strings.Contains(url, "controlplane-1.mydomain.local") {
			t.Errorf("expected URL to contain 'controlplane-1.mydomain.local', got %q", url)
		}
	})
}
