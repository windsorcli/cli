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

	resolvedContext, err := di.Resolve("context")
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
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	enabled, err := h.ConfigHandler.GetBool(fmt.Sprintf("contexts.%s.git.livereload.enabled", context), false)
	if err != nil {
		return nil, fmt.Errorf("error retrieving git livereload enabled status: %w", err)
	}

	if !enabled {
		return nil, nil
	}

	// Prepare the services slice for docker-compose
	var services []types.ServiceConfig

	// Retrieve environment variables from config with defaults
	rsyncExclude, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.rsync_exclude", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_EXCLUDE,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving rsync_exclude: %w", err)
	}

	rsyncProtect, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.rsync_protect", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving rsync_protect: %w", err)
	}

	gitUsername, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.username", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving git username: %w", err)
	}

	gitPassword, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.password", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving git password: %w", err)
	}

	webhookUrl, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.webhook_url", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving webhook url: %w", err)
	}

	verifySsl, err := h.ConfigHandler.GetBool(
		fmt.Sprintf("contexts.%s.git.livereload.verify_ssl", context),
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving verify_ssl: %w", err)
	}

	image, err := h.ConfigHandler.GetString(
		fmt.Sprintf("contexts.%s.git.livereload.image", context),
		constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE,
	)
	if err != nil {
		return nil, fmt.Errorf("error retrieving git livereload image: %w", err)
	}

	// Prepare environment variables map
	envVars := map[string]*string{
		"RSYNC_EXCLUDE": strPtr(rsyncExclude),
		"RSYNC_PROTECT": strPtr(rsyncProtect),
		"GIT_USERNAME":  strPtr(gitUsername),
		"GIT_PASSWORD":  strPtr(gitPassword),
		"VERIFY_SSL":    strPtr(fmt.Sprintf("%t", verifySsl)),
	}

	// Add webhook URL if provided
	if webhookUrl != "" {
		envVars["WEBHOOK_URL"] = strPtr(webhookUrl)
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
		Image:       image,
		Restart:     "always",
		Environment: envVars,
		Labels: map[string]string{
			"role":       "git-repository",
			"managed_by": "windsor",
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

// Ensure GitHelper implements Helper interface
var _ Helper = (*GitHelper)(nil)

// strPtr is a helper function to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
