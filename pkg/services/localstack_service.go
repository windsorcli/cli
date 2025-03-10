package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
)

// Valid AWS service names that use the same endpoint
var ValidLocalstackServiceNames = []string{
	"acm", "apigateway", "cloudformation", "cloudwatch", "config", "dynamodb", "dynamodbstreams",
	"ec2", "es", "events", "firehose", "iam", "kinesis", "kms", "lambda", "logs", "opensearch",
	"redshift", "resource-groups", "resourcegroupstaggingapi", "route53", "route53resolver", "s3",
	"s3control", "scheduler", "secretsmanager", "ses", "sns", "sqs", "ssm", "stepfunctions", "sts",
	"support", "swf", "transcribe",
}

// Invalid Terraform AWS service names that do not get an endpoint configuration
var InvalidTerraformAwsServiceNames = []string{
	"dynamodbstreams", "resource-groups", "support", "logs", "opensearch", "scheduler",
}

// LocalstackService is a service struct that provides Localstack-specific utility functions
type LocalstackService struct {
	BaseService
}

// NewLocalstackService is a constructor for LocalstackService
func NewLocalstackService(injector di.Injector) *LocalstackService {
	return &LocalstackService{
		BaseService: BaseService{
			injector: injector,
			name:     "aws",
		},
	}
}

// GetComposeConfig constructs and returns a Docker Compose configuration for the Localstack service.
// It retrieves the context configuration, checks for a Localstack authentication token, and determines
// the appropriate image to use. It also gathers the list of Localstack services to enable, constructs
// the full domain name, and sets up the service configuration with environment variables, labels, and
// port settings. If an authentication token is present, it adds it to the service secrets.
func (s *LocalstackService) GetComposeConfig() (*types.Config, error) {
	contextConfig := s.configHandler.GetConfig()
	localstackAuthToken := os.Getenv("LOCALSTACK_AUTH_TOKEN")

	image := constants.DEFAULT_AWS_LOCALSTACK_IMAGE
	if localstackAuthToken != "" {
		image = constants.DEFAULT_AWS_LOCALSTACK_PRO_IMAGE
	}

	servicesList := ""
	if contextConfig.AWS.Localstack.Services != nil {
		services := s.configHandler.GetStringSlice("aws.localstack.services", []string{})
		validServices, invalidServices := validateServices(services)
		if len(invalidServices) > 0 {
			return nil, fmt.Errorf("invalid services found: %s", strings.Join(invalidServices, ", "))
		}
		servicesList = strings.Join(validServices, ",")
	}

	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld

	port, err := strconv.ParseUint(constants.DEFAULT_AWS_LOCALSTACK_PORT, 10, 32)
	if err != nil {
		// Can't test this error until the port is configurable
		return nil, fmt.Errorf("invalid port format: %w", err)
	}
	port32 := uint32(port)

	services := []types.ServiceConfig{
		{
			Name:          fullName,
			ContainerName: fullName,
			Image:         image,
			Restart:       "always",
			Environment: map[string]*string{
				"ENFORCE_IAM":   ptrString("1"),
				"PERSISTENCE":   ptrString("1"),
				"IAM_SOFT_MODE": ptrString("0"),
				"DEBUG":         ptrString("0"),
				"SERVICES":      ptrString(servicesList),
			},
			Labels: map[string]string{
				"role":       "aws",
				"managed_by": "windsor",
			},
			Ports: []types.ServicePortConfig{
				{
					Target:    port32,
					Published: constants.DEFAULT_AWS_LOCALSTACK_PORT,
					Protocol:  "tcp",
				},
			},
		},
	}

	if localstackAuthToken != "" {
		services[0].Environment["LOCALSTACK_AUTH_TOKEN"] = ptrString("${LOCALSTACK_AUTH_TOKEN}")
	}

	return &types.Config{Services: services}, nil
}

// SetAddress updates the service address and configures default AWS service endpoints.
// It ensures S3 hostname, MWAA endpoint, and general endpoint URL are set if not provided.
func (s *LocalstackService) SetAddress(address string) error {
	if err := s.BaseService.SetAddress(address); err != nil {
		return err
	}

	tld := s.configHandler.GetString("dns.domain", "test")
	fullName := s.name + "." + tld
	port := constants.DEFAULT_AWS_LOCALSTACK_PORT

	awsConfig := s.configHandler.GetConfig().AWS
	if awsConfig == nil {
		return fmt.Errorf("AWS configuration not found")
	}

	s3Hostname := s.configHandler.GetString("aws.s3_hostname", "")
	if s3Hostname == "" {
		s3Address := fmt.Sprintf("http://s3.%s:%s", fullName, port)
		if err := s.configHandler.SetContextValue("aws.s3_hostname", s3Address); err != nil {
			return fmt.Errorf("failed to set aws.s3_hostname: %w", err)
		}
	}

	mwaaEndpoint := s.configHandler.GetString("aws.mwaa_endpoint", "")
	if mwaaEndpoint == "" {
		mwaaAddress := fmt.Sprintf("http://mwaa.%s:%s", fullName, port)
		if err := s.configHandler.SetContextValue("aws.mwaa_endpoint", mwaaAddress); err != nil {
			return fmt.Errorf("failed to set aws.mwaa_endpoint: %w", err)
		}
	}
	endpointURL := s.configHandler.GetString("aws.endpoint_url", "")
	if endpointURL == "" {
		endpointAddress := fmt.Sprintf("http://%s:%s", fullName, port)
		if err := s.configHandler.SetContextValue("aws.endpoint_url", endpointAddress); err != nil {
			return fmt.Errorf("failed to set aws.endpoint_url: %w", err)
		}
	}

	return nil
}

// validateServices checks the input services and returns valid and invalid services.
func validateServices(services []string) ([]string, []string) {
	validServicesMap := make(map[string]struct{}, len(ValidLocalstackServiceNames))
	for _, serviceName := range ValidLocalstackServiceNames {
		validServicesMap[serviceName] = struct{}{}
	}

	var validServices []string
	var invalidServices []string
	for _, service := range services {
		if _, exists := validServicesMap[service]; exists {
			validServices = append(validServices, service)
		} else {
			invalidServices = append(invalidServices, service)
		}
	}
	return validServices, invalidServices
}

// SupportsWildcard returns true if the Localstack service supports wildcard subdomains
func (s *LocalstackService) SupportsWildcard() bool {
	return true
}

// Ensure LocalstackService implements Service interface
var _ Service = (*LocalstackService)(nil)
