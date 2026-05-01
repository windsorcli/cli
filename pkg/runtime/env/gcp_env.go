// The GcpEnvPrinter is a specialized component that manages GCP environment configuration.
// It provides GCP-specific environment variable management and configuration,
// The GcpEnvPrinter handles GCP configuration settings and environment setup,
// ensuring proper gcloud CLI integration and environment setup for operations.

package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// GcpEnvPrinter is a struct that implements GCP environment configuration
type GcpEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewGcpEnvPrinter creates a new GcpEnvPrinter instance
func NewGcpEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *GcpEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &GcpEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the GCP environment.
// In global mode (no windsor.yaml in the project tree) windsor defers to the
// operator's ambient gcloud setup: CLOUDSDK_CONFIG is not emitted, the
// context-scoped gcloud directory is not created, and GOOGLE_APPLICATION_CREDENTIALS
// is only emitted when gcp.credentials_path is set explicitly. The project
// identifiers (GOOGLE_CLOUD_PROJECT, GCLOUD_PROJECT, GOOGLE_CLOUD_QUOTA_PROJECT)
// are still emitted because they describe which GCP project the context
// targets, not whose credentials are used.
func (e *GcpEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	config := e.configHandler.GetConfig()
	if config != nil && config.GCP != nil {
		if !global {
			configRoot, err := e.configHandler.GetConfigRoot()
			if err != nil {
				return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
			}
			gcpConfigDir := filepath.Join(configRoot, ".gcp")
			gcloudConfigDir := filepath.Join(gcpConfigDir, "gcloud")
			if err := e.shims.MkdirAll(gcloudConfigDir, 0755); err != nil {
				return nil, fmt.Errorf("error creating GCP config directory: %w", err)
			}
			envVars["CLOUDSDK_CONFIG"] = filepath.ToSlash(gcloudConfigDir)

			if _, exists := e.shims.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS"); !exists {
				if config.GCP.CredentialsPath != nil {
					envVars["GOOGLE_APPLICATION_CREDENTIALS"] = *config.GCP.CredentialsPath
				} else {
					serviceAccountPath := filepath.Join(gcpConfigDir, "service-accounts", "default.json")
					envVars["GOOGLE_APPLICATION_CREDENTIALS"] = filepath.ToSlash(serviceAccountPath)
				}
			}
		} else if config.GCP.CredentialsPath != nil {
			if _, exists := e.shims.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS"); !exists {
				envVars["GOOGLE_APPLICATION_CREDENTIALS"] = *config.GCP.CredentialsPath
			}
		}

		if config.GCP.ProjectID != nil {
			envVars["GOOGLE_CLOUD_PROJECT"] = *config.GCP.ProjectID
			envVars["GCLOUD_PROJECT"] = *config.GCP.ProjectID
		}

		if config.GCP.QuotaProject != nil {
			envVars["GOOGLE_CLOUD_QUOTA_PROJECT"] = *config.GCP.QuotaProject
		}
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure GcpEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*GcpEnvPrinter)(nil)
