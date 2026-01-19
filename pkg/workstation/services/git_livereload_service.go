package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
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
func NewGitLivereloadService(rt *runtime.Runtime) *GitLivereloadService {
	return &GitLivereloadService{
		BaseService: *NewBaseService(rt),
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
	webhookUrl := s.configHandler.GetString("git.livereload.webhook_url", s.computeDefaultWebhookURL())
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

	projectRoot := s.projectRoot

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

// GetIncusConfig returns the Incus configuration for the Git livereload service.
// It configures a container with project root mounted and environment variables for Git and rsync.
func (s *GitLivereloadService) GetIncusConfig() (*IncusConfig, error) {
	projectRoot := s.projectRoot
	gitFolderName := filepath.Base(projectRoot)

	rsyncInclude := s.configHandler.GetString("git.livereload.rsync_include", constants.DefaultGitLiveReloadRsyncInclude)
	rsyncExclude := s.configHandler.GetString("git.livereload.rsync_exclude", constants.DefaultGitLiveReloadRsyncExclude)
	rsyncProtect := s.configHandler.GetString("git.livereload.rsync_protect", constants.DefaultGitLiveReloadRsyncProtect)
	gitUsername := s.configHandler.GetString("git.livereload.username", constants.DefaultGitLiveReloadUsername)
	gitPassword := s.configHandler.GetString("git.livereload.password", constants.DefaultGitLiveReloadPassword)
	webhookUrl := s.configHandler.GetString("git.livereload.webhook_url", s.computeDefaultWebhookURL())
	verifySsl := s.configHandler.GetBool("git.livereload.verify_ssl", false)
	image := s.configHandler.GetString("git.livereload.image", constants.DefaultGitLiveReloadImage)

	config := make(map[string]string)
	config["environment.RSYNC_INCLUDE"] = rsyncInclude
	config["environment.RSYNC_EXCLUDE"] = rsyncExclude
	config["environment.RSYNC_PROTECT"] = rsyncProtect
	config["environment.GIT_USERNAME"] = gitUsername
	config["environment.GIT_PASSWORD"] = gitPassword
	config["environment.VERIFY_SSL"] = fmt.Sprintf("%t", verifySsl)
	if webhookUrl != "" {
		config["environment.WEBHOOK_URL"] = webhookUrl
	}

	devices := make(map[string]map[string]string)
	devices["project-root"] = map[string]string{
		"type":   "disk",
		"source": projectRoot,
		"path":   fmt.Sprintf("/repos/mount/%s", gitFolderName),
	}

	var finalImage string
	if strings.HasPrefix(image, "ghcr.io/") {
		finalImage = "ghcr:" + strings.TrimPrefix(image, "ghcr.io/")
	} else {
		finalImage = "docker:" + image
	}

	return &IncusConfig{
		Type:    "container",
		Image:   finalImage,
		Config:  config,
		Devices: devices,
	}, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// computeDefaultWebhookURL dynamically computes the webhook URL based on cluster configuration.
// For LoadBalancer mode (colima/incus): uses LoadBalancer IP on port 9292.
// For NodePort mode (docker-desktop): uses node hostname on port 30292.
// If workers exist (workers.count > 0), uses worker-1; otherwise uses controlplane-1.
func (s *GitLivereloadService) computeDefaultWebhookURL() string {
	lbIP := s.configHandler.GetString("network.loadbalancer_ips.start", "")
	if lbIP != "" {
		return fmt.Sprintf("http://%s:%d%s", lbIP, constants.DefaultGitLiveReloadWebhookLBPort, constants.DefaultGitLiveReloadWebhookPath)
	}

	domain := s.configHandler.GetString("dns.domain", "test")
	workersCount := s.configHandler.GetInt("cluster.workers.count", 0)

	var nodeHost string
	if workersCount > 0 {
		nodeHost = fmt.Sprintf("worker-1.%s", domain)
	} else {
		nodeHost = fmt.Sprintf("controlplane-1.%s", domain)
	}

	return fmt.Sprintf("http://%s:%d%s", nodeHost, constants.DefaultGitLiveReloadWebhookNodePort, constants.DefaultGitLiveReloadWebhookPath)
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure GitService implements Service interface
var _ Service = (*GitLivereloadService)(nil)
