package services

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// The GitLivereloadService is a service component that manages Git repository synchronization
// It provides live reload capabilities for Git repositories with rsync integration
// The GitLivereloadService enables automatic synchronization of Git repositories
// with configurable rsync options and webhook notifications for repository changes

// =============================================================================
// Types
// =============================================================================

// GitLivereloadService is a service struct that provides various utility functions
type GitLivereloadService struct {
	BaseService
}

// =============================================================================
// Constructor
// =============================================================================

// NewGitLivereloadService is a constructor for GitLivereloadService
func NewGitLivereloadService(injector di.Injector) *GitLivereloadService {
	return &GitLivereloadService{
		BaseService: *NewBaseService(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetComposeConfig constructs and returns a docker-compose configuration for the GitLivereloadService.
// It retrieves configuration values for environment variables, image, and service metadata from the config handler.
// The method builds the environment variable map, sets up service labels, and binds the project root as a volume.
// Returns a types.Config pointer containing the service definition, or an error if the project root cannot be determined.
func (s *GitLivereloadService) GetComposeConfig() (*types.Config, error) {
	contextName := s.configHandler.GetContext()
	rsyncInclude := s.configHandler.GetString("git.livereload.rsync_include", constants.DefaultGitLiveReloadRsyncInclude)
	rsyncExclude := s.configHandler.GetString("git.livereload.rsync_exclude", constants.DefaultGitLiveReloadRsyncExclude)
	rsyncProtect := s.configHandler.GetString("git.livereload.rsync_protect", constants.DefaultGitLiveReloadRsyncProtect)
	gitUsername := s.configHandler.GetString("git.livereload.username", constants.DefaultGitLiveReloadUsername)
	gitPassword := s.configHandler.GetString("git.livereload.password", constants.DefaultGitLiveReloadPassword)
	webhookUrl := s.configHandler.GetString("git.livereload.webhook_url", constants.DefaultGitLiveReloadWebhookURL)
	verifySsl := s.configHandler.GetBool("git.livereload.verify_ssl", false)
	image := s.configHandler.GetString("git.livereload.image", constants.DefaultGitLiveReloadImage)

	envVars := map[string]*string{
		"RSYNC_INCLUDE": ptrString(rsyncInclude),
		"RSYNC_EXCLUDE": ptrString(rsyncExclude),
		"RSYNC_PROTECT": ptrString(rsyncProtect),
		"GIT_USERNAME":  ptrString(gitUsername),
		"GIT_PASSWORD":  ptrString(gitPassword),
		"VERIFY_SSL":    ptrString(fmt.Sprintf("%t", verifySsl)),
	}

	if webhookUrl != "" {
		envVars["WEBHOOK_URL"] = ptrString(webhookUrl)
	}

	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}

	gitFolderName := filepath.Base(projectRoot)
	serviceName := s.name

	serviceConfig := types.ServiceConfig{
		Name:          serviceName,
		ContainerName: s.GetContainerName(),
		Image:         image,
		Restart:       "always",
		Environment:   envVars,
		Labels: map[string]string{
			"role":       "git-repository",
			"managed_by": "windsor",
			"context":    contextName,
		},
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: "${WINDSOR_PROJECT_ROOT}",
				Target: fmt.Sprintf("/repos/mount/%s", gitFolderName),
			},
		},
	}

	services := types.Services{
		serviceName: serviceConfig,
	}

	return &types.Config{
		Services: services,
	}, nil
}

// Ensure GitService implements Service interface
var _ Service = (*GitLivereloadService)(nil)
