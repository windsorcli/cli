package services

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/di"
)

// GitService is a service struct that provides various utility functions
type GitService struct {
	BaseService
}

// NewGitService is a constructor for GitService
func NewGitService(injector di.Injector) *GitService {
	return &GitService{
		BaseService: BaseService{
			injector: injector,
		},
	}
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (s *GitService) GetComposeConfig() (*types.Config, error) {
	contextName, err := s.contextHandler.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	config := s.configHandler.GetConfig()

	if config.Git == nil ||
		config.Git.Livereload == nil ||
		config.Git.Livereload.Create == nil ||
		!*config.Git.Livereload.Create {
		return nil, nil
	}

	// Prepare the services slice for docker-compose
	var services []types.ServiceConfig

	// Retrieve environment variables from config with defaults
	rsyncExclude := config.Git.Livereload.RsyncExclude
	if rsyncExclude == nil || *rsyncExclude == "" {
		defaultRsyncExclude := constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE
		rsyncExclude = &defaultRsyncExclude
	}

	rsyncProtect := config.Git.Livereload.RsyncProtect
	if rsyncProtect == nil || *rsyncProtect == "" {
		defaultRsyncProtect := constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT
		rsyncProtect = &defaultRsyncProtect
	}

	gitUsername := config.Git.Livereload.Username
	if gitUsername == nil || *gitUsername == "" {
		defaultGitUsername := constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME
		gitUsername = &defaultGitUsername
	}

	gitPassword := config.Git.Livereload.Password
	if gitPassword == nil || *gitPassword == "" {
		defaultGitPassword := constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD
		gitPassword = &defaultGitPassword
	}

	webhookUrl := config.Git.Livereload.WebhookUrl
	if webhookUrl == nil || *webhookUrl == "" {
		defaultWebhookUrl := constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL
		webhookUrl = &defaultWebhookUrl
	}

	verifySsl := config.Git.Livereload.VerifySsl
	if verifySsl == nil {
		verifySsl = new(bool)
	}

	image := config.Git.Livereload.Image
	if image == nil || *image == "" {
		defaultImage := constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE
		image = &defaultImage
	}

	// Prepare environment variables map
	envVars := map[string]*string{
		"RSYNC_EXCLUDE": rsyncExclude,
		"RSYNC_PROTECT": rsyncProtect,
		"GIT_USERNAME":  gitUsername,
		"GIT_PASSWORD":  gitPassword,
		"VERIFY_SSL":    strPtr(fmt.Sprintf("%t", *verifySsl)),
	}

	// Add webhook URL if provided
	if webhookUrl != nil && *webhookUrl != "" {
		envVars["WEBHOOK_URL"] = webhookUrl
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
		Name:        "git.test",
		Image:       *image,
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
var _ Service = (*GitService)(nil)

// strPtr is a helper function to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
