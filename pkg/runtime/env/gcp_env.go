// The GcpEnvPrinter is a specialized component that manages GCP environment configuration.
// It provides GCP-specific environment variable management and configuration,
// The GcpEnvPrinter handles GCP configuration settings and environment setup,
// ensuring proper gcloud CLI integration and environment setup for operations.

package env

import (
	"path/filepath"

	"github.com/windsorcli/cli/internal/werror"
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

// GetEnvVars returns the GCP environment variables of the current context.
// Project mode always sets CLOUDSDK_CONFIG. Google client libraries read GOOGLE_APPLICATION_CREDENTIALS.
// Windsor sets it from gcp.credentials_path, the service-account file, or the gcloud ADC file, in that order.
// Windsor stat-checks the two fallback files and never guesses a path. It recomputes the value unless the operator set one.
// GOOGLE_CLOUD_PROJECT, GCLOUD_PROJECT, and GOOGLE_CLOUD_QUOTA_PROJECT need a gcp: field.
func (e *GcpEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	config := e.configHandler.GetHeader()
	var credentialsPath *string
	if config != nil && config.GCP != nil {
		credentialsPath = config.GCP.CredentialsPath
	}

	shouldSetCredentials := e.ShouldSetManagedValue("GOOGLE_APPLICATION_CREDENTIALS")

	if !global {
		configRoot, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, werror.Wrap(err, "error retrieving configuration root directory")
		}
		gcpConfigDir := filepath.Join(configRoot, ".gcp")
		gcloudConfigDir := filepath.Join(gcpConfigDir, "gcloud")
		if err := e.shims.MkdirAll(gcloudConfigDir, 0755); err != nil {
			return nil, werror.Wrap(err, "error creating GCP config directory")
		}
		envVars["CLOUDSDK_CONFIG"] = filepath.ToSlash(gcloudConfigDir)

		if shouldSetCredentials {
			if credentialsPath != nil {
				envVars["GOOGLE_APPLICATION_CREDENTIALS"] = *credentialsPath
			} else {
				serviceAccountPath := filepath.Join(gcpConfigDir, "service-accounts", "default.json")
				adcPath := filepath.Join(gcloudConfigDir, "application_default_credentials.json")
				if _, statErr := e.shims.Stat(serviceAccountPath); statErr == nil {
					envVars["GOOGLE_APPLICATION_CREDENTIALS"] = filepath.ToSlash(serviceAccountPath)
				} else if _, statErr := e.shims.Stat(adcPath); statErr == nil {
					envVars["GOOGLE_APPLICATION_CREDENTIALS"] = filepath.ToSlash(adcPath)
				}
			}
		}
	} else if credentialsPath != nil && shouldSetCredentials {
		envVars["GOOGLE_APPLICATION_CREDENTIALS"] = *credentialsPath
	}

	if config != nil && config.GCP != nil {
		if config.GCP.ProjectID != nil {
			envVars["GOOGLE_CLOUD_PROJECT"] = *config.GCP.ProjectID
			envVars["GCLOUD_PROJECT"] = *config.GCP.ProjectID
		}

		if config.GCP.QuotaProject != nil {
			envVars["GOOGLE_CLOUD_QUOTA_PROJECT"] = *config.GCP.QuotaProject
		}
	}

	for key := range envVars {
		e.SetManagedEnv(key)
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure GcpEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*GcpEnvPrinter)(nil)
