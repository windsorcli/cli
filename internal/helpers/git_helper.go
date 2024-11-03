package helpers

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// GitHelper is a helper struct that provides various utility functions
type GitHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewGitHelper is a constructor for GitHelper
func NewGitHelper(di *di.DIContainer) (*GitHelper, error) {
	cliConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	resolvedShell, err := di.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}

	resolvedContext, err := di.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &GitHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Shell:         resolvedShell.(shell.Shell),
		Context:       resolvedContext.(context.ContextInterface),
	}, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *GitHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetEnvVars is a no-op function
func (h *GitHelper) GetEnvVars() (map[string]string, error) {
	return map[string]string{}, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *GitHelper) PostEnvExec() error {
	return nil
}

// GetComposeConfig returns the top-level compose configuration including a list of container data for docker-compose.
func (h *GitHelper) GetComposeConfig() (*types.Config, error) {
	contextName, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	config, err := h.ConfigHandler.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config: %w", err)
	}

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
	projectRoot, err := h.Shell.GetProjectRoot()
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

// WriteConfig is a no-op function
func (h *GitHelper) WriteConfig() error {
	return nil
}

// Up executes necessary commands to instantiate the tool or environment.
func (h *GitHelper) Up(verbose ...bool) error {
	return nil
}

// Info returns information about the helper.
func (h *GitHelper) Info() (interface{}, error) {
	return nil, nil
}

// Ensure GitHelper implements Helper interface
var _ Helper = (*GitHelper)(nil)

// strPtr is a helper function to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
