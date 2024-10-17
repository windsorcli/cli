package helpers

import (
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Default Git live reload image
var DEFAULT_GIT_LIVE_RELOAD_IMAGE = "ghcr.io/windsor-hotel/git-livereload-server:v0.2.1"

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

// GetEnvVars is a no-op function
func (h *GitHelper) GetEnvVars() (map[string]string, error) {
	return nil, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *GitHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *GitHelper) SetConfig(key, value string) error {
	if value == "" {
		return nil
	}

	context, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	// Handle the git enabled condition
	if key == "enabled" {
		isEnabled := value == "true"
		err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.enabled", context), isEnabled)
		if err != nil {
			return fmt.Errorf("error setting config value for %s: %w", key, err)
		}
	}

	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *GitHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	enabled, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.enabled", context), "false")
	if err != nil {
		return nil, fmt.Errorf("error retrieving git livereload enabled status: %w", err)
	}

	if enabled != "true" {
		return nil, nil
	}

	// Prepare the services slice for docker-compose
	var services []types.ServiceConfig

	// Retrieve environment variables from config with defaults
	rsyncExclude, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.rsync_exclude", context), ".docker-cache,.terraform,data,.venv")
	if err != nil {
		return nil, fmt.Errorf("error retrieving rsync_exclude: %w", err)
	}

	rsyncProtect, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.rsync_protect", context), "flux-system")
	if err != nil {
		return nil, fmt.Errorf("error retrieving rsync_protect: %w", err)
	}

	gitUsername, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.username", context), "local")
	if err != nil {
		return nil, fmt.Errorf("error retrieving git username: %w", err)
	}

	gitPassword, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.password", context), "local")
	if err != nil {
		return nil, fmt.Errorf("error retrieving git password: %w", err)
	}

	webhookUrl, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.webhook_url", context), "")
	if err != nil {
		return nil, fmt.Errorf("error retrieving webhook url: %w", err)
	}

	verifySsl, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.verify_ssl", context), "false")
	if err != nil {
		return nil, fmt.Errorf("error retrieving verify_ssl: %w", err)
	}

	image, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.git.livereload.image", context), DEFAULT_GIT_LIVE_RELOAD_IMAGE)
	if err != nil {
		return nil, fmt.Errorf("error retrieving git livereload image: %w", err)
	}

	// Prepare environment variables map
	envVars := map[string]*string{
		"RSYNC_EXCLUDE": strPtr(rsyncExclude),
		"RSYNC_PROTECT": strPtr(rsyncProtect),
		"GIT_USERNAME":  strPtr(gitUsername),
		"GIT_PASSWORD":  strPtr(gitPassword),
		"VERIFY_SSL":    strPtr(verifySsl),
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
				Source: projectRoot,
				Target: fmt.Sprintf("/repos/mount/%s", gitFolderName),
			},
		},
	})

	return services, nil
}

// Ensure GitHelper implements Helper interface
var _ Helper = (*GitHelper)(nil)

// strPtr is a helper function to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
