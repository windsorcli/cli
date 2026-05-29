// The AwsEnvPrinter is a specialized component that manages AWS environment configuration.
// It provides AWS-specific environment variable management and configuration,
// The AwsEnvPrinter handles AWS profile, endpoint, and S3 configuration settings,
// ensuring proper AWS CLI integration and environment setup for AWS operations.

package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// GetEnvVars returns the AWS env vars for the current context. In project mode
// AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE point at the context's .aws/
// so `aws configure` stays scoped to the context instead of ~/.aws/. AWS_PROFILE
// defaults to the context name (or aws.profile if set). When the parent env
// already carries AWS credentials, project mode suppresses AWS_PROFILE and the
// two file vars so the AWS CLI does not chase a [profile <context>] block
// against the empty context .aws/; global mode keeps AWS_PROFILE flowing.
func (e *AwsEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()
	ambient := hasAmbientAWSCredentials()

	if !global && !ambient {
		configRoot, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
		}
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

	if !ambient || global {
		if awsProfileOverride != "" {
			envVars["AWS_PROFILE"] = awsProfileOverride
		} else if ctx := e.configHandler.GetContext(); ctx != "" {
			envVars["AWS_PROFILE"] = ctx
		}
	}

	return envVars, nil
}

// hasAmbientAWSCredentials reports whether the parent env carries AWS
// credentials via IRSA, ECS container creds, or static access keys. Callers
// use it to skip context-scoped overrides that would otherwise mask the
// native credential chain. IMDS is not covered — no env var detects it.
func hasAmbientAWSCredentials() bool {
	if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
		return true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" {
		return true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}
	return false
}

// contextHasAWSProfile reports whether the named profile is defined in the
// context's .aws/config or .aws/credentials. The check is a line scan for the
// expected section header — "[profile <name>]" in config (or "[default]" for the
// default profile) and "[<name>]" in credentials. The AWS SDK treats a profile
// found in either file as satisfying the lookup, so a single match is enough.
func contextHasAWSProfile(configRoot, profileName string) bool {
	awsDir := filepath.Join(configRoot, ".aws")
	configHeader := "[profile " + profileName + "]"
	if profileName == "default" {
		configHeader = "[default]"
	}
	if iniContainsSection(filepath.Join(awsDir, "config"), configHeader) {
		return true
	}
	return iniContainsSection(filepath.Join(awsDir, "credentials"), "["+profileName+"]")
}

// iniContainsSection scans the file at path for a line whose trimmed contents
// match section exactly. Returns false on any read error so a missing or
// unreadable file is treated as "no section present" rather than fatal.
func iniContainsSection(path, section string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == section {
			return true
		}
	}
	return false
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure AwsEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*AwsEnvPrinter)(nil)
