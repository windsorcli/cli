// The AwsEnvPrinter is a specialized component that manages AWS environment configuration.
// It provides AWS-specific environment variable management and configuration,
// The AwsEnvPrinter handles AWS profile, endpoint, and S3 configuration settings,
// ensuring proper AWS CLI integration and environment setup for AWS operations.

package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/internal/awsprofile"
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

// GetEnvVars returns the AWS env vars for the current context. In project mode
// AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE always point at the context's
// .aws/ so `aws configure` stays scoped to the context. AWS_PROFILE is emitted
// only when the named profile is actually defined in the relevant AWS config:
// the context's .aws/ in project mode, or the operator's ambient ~/.aws/ (or
// AWS_CONFIG_FILE override) in global mode. When the profile is absent the var
// is omitted so the AWS SDK falls through to env keys, IMDS, ECS task creds,
// or whatever else the credential chain finds rather than failing with
// "profile not found" against a file the named profile was never in.
func (e *AwsEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	var configRoot string
	if !global {
		root, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
		}
		configRoot = root
		awsConfigDir := filepath.Join(configRoot, ".aws")
		envVars["AWS_CONFIG_FILE"] = filepath.ToSlash(filepath.Join(awsConfigDir, "config"))
		envVars["AWS_SHARED_CREDENTIALS_FILE"] = filepath.ToSlash(filepath.Join(awsConfigDir, "credentials"))
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

	profileName := awsProfileOverride
	if profileName == "" {
		profileName = e.configHandler.GetContext()
	}
	if profileName != "" {
		resolver := awsprofile.ForContext(configRoot)
		if global {
			resolver = awsprofile.Ambient()
		}
		if resolver.HasProfile(profileName) {
			envVars["AWS_PROFILE"] = profileName
		}
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure AwsEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AwsEnvPrinter)(nil)
