// AWSGenerator scaffolding
package generators

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
)

// AWSGenerator is a generator that creates AWS configuration files.
type AWSGenerator struct {
	BaseGenerator
}

// NewAWSGenerator creates a new AWSGenerator instance.
func NewAWSGenerator(injector di.Injector) *AWSGenerator {
	return &AWSGenerator{
		BaseGenerator: BaseGenerator{injector: injector},
	}
}

// Write creates an "aws" directory in the project root and modifies
// the AWS config file if it exists. It ensures the default section
// has cli_pager, region, and output set. It also modifies the
// specific profile section and s3 block based on configuration.
func (g *AWSGenerator) Write() error {
	configRoot, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return err
	}

	awsConfigFilePath := filepath.Join(configRoot, ".aws", "config")
	if _, err := osStat(awsConfigFilePath); os.IsNotExist(err) {
		awsFolderPath := filepath.Dir(awsConfigFilePath)
		if err := osMkdirAll(awsFolderPath, os.ModePerm); err != nil {
			return err
		}
	}

	cfg, err := iniLoad(awsConfigFilePath)
	if err != nil {
		cfg = iniEmpty()
	}

	// Set default section values
	defaultSection := cfg.Section("default")
	defaultSection.Key("cli_pager").SetValue(g.configHandler.GetString("aws.cli_pager", ""))
	defaultSection.Key("output").SetValue(g.configHandler.GetString("aws.output", "text"))
	defaultSection.Key("region").SetValue(g.configHandler.GetString("aws.region", constants.DEFAULT_AWS_REGION))

	// Set profile-specific section values
	profile := g.configHandler.GetString("aws.profile", "default")
	sectionName := "default"
	if profile != "default" {
		sectionName = "profile " + profile
	}

	section := cfg.Section(sectionName)
	section.Key("region").SetValue(g.configHandler.GetString("aws.region", constants.DEFAULT_AWS_REGION))

	// Access Localstack configuration
	if g.configHandler.GetBool("aws.localstack.enabled", false) {
		service, ok := g.injector.Resolve("localstackService").(services.Service)
		if !ok {
			return fmt.Errorf("localstackService not found")
		}
		tld := g.configHandler.GetString("dns.domain", "test")
		fullName := service.GetName() + "." + tld

		// Build a single endpoint
		localstackPort := constants.DEFAULT_AWS_LOCALSTACK_PORT
		localstackEndpoint := "http://" + fullName + ":" + localstackPort

		// Modify AWS config with Localstack endpoint
		section.Key("endpoint_url").SetValue(localstackEndpoint)

		// Set AWS access key and secret key for Localstack using recommended values
		section.Key("aws_access_key_id").SetValue(constants.DEFAULT_AWS_LOCALSTACK_ACCESS_KEY)
		section.Key("aws_secret_access_key").SetValue(constants.DEFAULT_AWS_LOCALSTACK_SECRET_KEY)
	}

	if err := iniSaveTo(cfg, awsConfigFilePath); err != nil {
		return err
	}

	return nil
}

// Ensure AWSGenerator implements the Generator interface
var _ Generator = (*AWSGenerator)(nil)
