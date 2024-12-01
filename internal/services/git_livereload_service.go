package services

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
)

// GitLivereloadService is a service struct that provides various utility functions
type GitLivereloadService struct {
	BaseService
}

// NewGitLivereloadService is a constructor for GitLivereloadService
func NewGitLivereloadService(injector di.Injector) *GitLivereloadService {
	return &GitLivereloadService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (s *GitLivereloadService) GetComposeConfig() (*types.Config, error) {
	// Get the top level domain from the configuration
	tld := s.configHandler.GetString("dns.name", "test")

	contextName, err := s.contextHandler.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	// Prepare the services slice for docker-compose
	var services []types.ServiceConfig

	// Retrieve environment variables from config with defaults using Get* functions
	rsyncExclude := s.configHandler.GetString("git.livereload.rsync_exclude", constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE)
	rsyncProtect := s.configHandler.GetString("git.livereload.rsync_protect", constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT)
	gitUsername := s.configHandler.GetString("git.livereload.username", constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME)
	gitPassword := s.configHandler.GetString("git.livereload.password", constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD)
	webhookUrl := s.configHandler.GetString("git.livereload.webhook_url", constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL)
	verifySsl := s.configHandler.GetBool("git.livereload.verify_ssl", false)
	image := s.configHandler.GetString("git.livereload.image", constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE)

	// Prepare environment variables map
	envVars := map[string]*string{
		"RSYNC_EXCLUDE": ptrString(rsyncExclude),
		"RSYNC_PROTECT": ptrString(rsyncProtect),
		"GIT_USERNAME":  ptrString(gitUsername),
		"GIT_PASSWORD":  ptrString(gitPassword),
		"VERIFY_SSL":    ptrString(fmt.Sprintf("%t", verifySsl)),
	}

	// Add webhook URL if provided
	if webhookUrl != "" {
		envVars["WEBHOOK_URL"] = ptrString(webhookUrl)
	}

	// Get the project root using the shell
	projectRoot, err := s.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}

	// Get the git folder name
	gitFolderName := filepath.Base(projectRoot)

	// Add the git-livereload service
	services = append(services, types.ServiceConfig{
		Name:        fmt.Sprintf("git.%s", tld),
		Image:       image,
		Restart:     "always",
		Environment: envVars,
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
	})

	return &types.Config{
		Services: services,
	}, nil
}

// Ensure GitService implements Service interface
var _ Service = (*GitLivereloadService)(nil)
