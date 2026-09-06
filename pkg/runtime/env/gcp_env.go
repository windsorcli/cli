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

// GetEnvVars returns the GCP environment variables for the current context. Project mode
// always sets CLOUDSDK_CONFIG. Google client libraries do not read it. GOOGLE_APPLICATION_CREDENTIALS
// bridges the gap. Windsor tries gcp.credentials_path first, then the service-account
// file, then the gcloud ADC file, else it leaves the variable unset. A set-but-missing
// path breaks Google client libraries outright, so Windsor never guesses one. Global mode
// defers to the operator's ambient gcloud setup. GOOGLE_CLOUD_PROJECT, GCLOUD_PROJECT, and
// GOOGLE_CLOUD_QUOTA_PROJECT need a gcp: field. Windsor recomputes GOOGLE_APPLICATION_CREDENTIALS
// on every call unless an operator's own value already occupies it. This lets a removed
// service-account file or a switch to ADC take effect on the next command.
func (e *GcpEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	config := e.configHandler.GetConfig()
	var credentialsPath *string
	if config != nil && config.GCP != nil {
		credentialsPath = config.GCP.CredentialsPath
	}

	shouldSetCredentials := e.ShouldSetManagedValue("GOOGLE_APPLICATION_CREDENTIALS")

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
