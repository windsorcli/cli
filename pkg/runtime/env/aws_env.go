// The AwsEnvPrinter is a specialized component that manages AWS environment configuration.
// It provides AWS-specific environment variable management and configuration,
// The AwsEnvPrinter handles AWS profile, endpoint, and S3 configuration settings,
// ensuring proper AWS CLI integration and environment setup for AWS operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// AwsEnvPrinter is a struct that implements AWS environment configuration
type AwsEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewAwsEnvPrinter creates a new AwsEnvPrinter instance
func NewAwsEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *AwsEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &AwsEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the AWS environment.
// AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE are only emitted when the per-context
// .aws/config or .aws/credentials file exists and is non-empty; otherwise the AWS SDK
// falls through to its defaults (~/.aws/config, ~/.aws/credentials), so an existing
// SSO/profile setup keeps working without per-context duplication. AWS_PROFILE defaults
// to the current context name so `aws configure sso` creates a profile bound to the
// context, and subsequent aws/terraform/SDK calls pick it up without further configuration;
// an explicit aws.profile in the context's aws block overrides the default. AWS_REGION
// is emitted only when aws.region is set; downstream tools otherwise fall back to the
// profile's own `region =` line.
func (e *AwsEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	awsConfigDir := filepath.Join(configRoot, ".aws")
	awsConfigPath := filepath.Join(awsConfigDir, "config")
	awsCredentialsPath := filepath.Join(awsConfigDir, "credentials")

	if info, err := e.shims.Stat(awsConfigPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error checking %s: %w", awsConfigPath, err)
		}
	} else if info.Size() > 0 {
		envVars["AWS_CONFIG_FILE"] = filepath.ToSlash(awsConfigPath)
	}
	if info, err := e.shims.Stat(awsCredentialsPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error checking %s: %w", awsCredentialsPath, err)
		}
	} else if info.Size() > 0 {
		envVars["AWS_SHARED_CREDENTIALS_FILE"] = filepath.ToSlash(awsCredentialsPath)
	}

	contextConfigData := e.configHandler.GetConfig()
	awsProfileOverride := ""
	if contextConfigData != nil && contextConfigData.AWS != nil {
		if contextConfigData.AWS.AWSProfile != nil {
			awsProfileOverride = *contextConfigData.AWS.AWSProfile
		}
		if contextConfigData.AWS.AWSRegion != nil {
			envVars["AWS_REGION"] = *contextConfigData.AWS.AWSRegion
		}
		if contextConfigData.AWS.AWSEndpointURL != nil {
			envVars["AWS_ENDPOINT_URL"] = *contextConfigData.AWS.AWSEndpointURL
		}
		if contextConfigData.AWS.S3Hostname != nil {
			envVars["S3_HOSTNAME"] = *contextConfigData.AWS.S3Hostname
		}
		if contextConfigData.AWS.MWAAEndpoint != nil {
			envVars["MWAA_ENDPOINT"] = *contextConfigData.AWS.MWAAEndpoint
		}
	}

	if awsProfileOverride != "" {
		envVars["AWS_PROFILE"] = awsProfileOverride
	} else if ctx := e.configHandler.GetContext(); ctx != "" {
		envVars["AWS_PROFILE"] = ctx
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure AwsEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AwsEnvPrinter)(nil)
