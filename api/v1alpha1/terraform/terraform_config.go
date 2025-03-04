package terraform

// TerraformConfig represents the Terraform configuration
type TerraformConfig struct {
	Enabled *bool          `yaml:"enabled,omitempty"`
	Backend *BackendConfig `yaml:"backend,omitempty"`
}

type BackendConfig struct {
	Type       string             `yaml:"type"`
	S3         *S3Backend         `yaml:"s3,omitempty"`
	Kubernetes *KubernetesBackend `yaml:"kubernetes,omitempty"`
	Local      *LocalBackend      `yaml:"local,omitempty"`
	Prefix     *string            `yaml:"prefix,omitempty"`
}

// https://developer.hashicorp.com/terraform/language/backend/s3#configuration
type S3Backend struct {
	Bucket                         *string   `yaml:"bucket,omitempty"`
	Key                            *string   `yaml:"key,omitempty"`
	Region                         *string   `yaml:"region,omitempty"`
	AccessKey                      *string   `yaml:"access_key,omitempty"`
	SecretKey                      *string   `yaml:"secret_key,omitempty"`
	SessionToken                   *string   `yaml:"session_token,omitempty"`
	RoleArn                        *string   `yaml:"role_arn,omitempty"`
	ExternalId                     *string   `yaml:"external_id,omitempty"`
	Profile                        *string   `yaml:"profile,omitempty"`
	SharedCredentialsFiles         *[]string `yaml:"shared_credentials_files,omitempty"`
	Token                          *string   `yaml:"token,omitempty"`
	SkipCredentialsValidation      *bool     `yaml:"skip_credentials_validation,omitempty"`
	SkipRegionValidation           *bool     `yaml:"skip_region_validation,omitempty"`
	SkipRequestingAccountId        *bool     `yaml:"skip_requesting_account_id,omitempty"`
	SkipMetadataApiCheck           *bool     `yaml:"skip_metadata_api_check,omitempty"`
	SkipS3Checksum                 *bool     `yaml:"skip_s3_checksum,omitempty"`
	UseDualstackEndpoint           *bool     `yaml:"use_dualstack_endpoint,omitempty"`
	UseFipsEndpoint                *bool     `yaml:"use_fips_endpoint,omitempty"`
	DynamoDBTable                  *string   `yaml:"dynamodb_table,omitempty"`
	UseLockfile                    *bool     `yaml:"use_lockfile,omitempty"`
	AllowedAccountIds              *[]string `yaml:"allowed_account_ids,omitempty"`
	CustomCaBundle                 *string   `yaml:"custom_ca_bundle,omitempty"`
	Ec2MetadataServiceEndpoint     *string   `yaml:"ec2_metadata_service_endpoint,omitempty"`
	Ec2MetadataServiceEndpointMode *string   `yaml:"ec2_metadata_service_endpoint_mode,omitempty"`
	ForbiddenAccountIds            *[]string `yaml:"forbidden_account_ids,omitempty"`
	HttpProxy                      *string   `yaml:"http_proxy,omitempty"`
	HttpsProxy                     *string   `yaml:"https_proxy,omitempty"`
	Insecure                       *bool     `yaml:"insecure,omitempty"`
	NoProxy                        *string   `yaml:"no_proxy,omitempty"`
	MaxRetries                     *int      `yaml:"max_retries,omitempty"`
	RetryMode                      *string   `yaml:"retry_mode,omitempty"`
	SharedConfigFiles              *[]string `yaml:"shared_config_files,omitempty"`
	StsRegion                      *string   `yaml:"sts_region,omitempty"`
	UsePathStyle                   *bool     `yaml:"use_path_style,omitempty"`
	WorkspaceKeyPrefix             *string   `yaml:"workspace_key_prefix,omitempty"`
	KmsKeyId                       *string   `yaml:"kms_key_id,omitempty"`
	SseCustomerKey                 *string   `yaml:"sse_customer_key,omitempty"`
}

// KubernetesBackend represents the configuration for the Kubernetes backend
type KubernetesBackend struct {
	SecretSuffix          *string            `yaml:"secret_suffix,omitempty"`
	Labels                *map[string]string `yaml:"labels,omitempty"`
	Namespace             *string            `yaml:"namespace,omitempty"`
	InClusterConfig       *bool              `yaml:"in_cluster_config,omitempty"`
	Host                  *string            `yaml:"host,omitempty"`
	Username              *string            `yaml:"username,omitempty"`
	Password              *string            `yaml:"password,omitempty"`
	Insecure              *bool              `yaml:"insecure,omitempty"`
	ClientCertificate     *string            `yaml:"client_certificate,omitempty"`
	ClientKey             *string            `yaml:"client_key,omitempty"`
	ClusterCACertificate  *string            `yaml:"cluster_ca_certificate,omitempty"`
	ConfigPath            *string            `yaml:"config_path,omitempty"`
	ConfigPaths           *[]string          `yaml:"config_paths,omitempty"`
	ConfigContext         *string            `yaml:"config_context,omitempty"`
	ConfigContextAuthInfo *string            `yaml:"config_context_auth_info,omitempty"`
	ConfigContextCluster  *string            `yaml:"config_context_cluster,omitempty"`
	Token                 *string            `yaml:"token,omitempty"`
	Exec                  *ExecConfig        `yaml:"exec,omitempty"`
}

// https://developer.hashicorp.com/terraform/language/backend/local#configuration-variables
type LocalBackend struct {
	Path *string `yaml:"path,omitempty"`
}

// ExecConfig represents the exec-based credential plugin configuration
type ExecConfig struct {
	APIVersion *string            `yaml:"api_version,omitempty"`
	Command    *string            `yaml:"command,omitempty"`
	Args       *[]string          `yaml:"args,omitempty"`
	Env        *map[string]string `yaml:"env,omitempty"`
}

// Merge performs a simple merge of the current TerraformConfig with another TerraformConfig.
func (base *TerraformConfig) Merge(overlay *TerraformConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Backend != nil {
		base.Backend = overlay.Backend
	}
}

// Copy creates a deep copy of the TerraformConfig object
func (c *TerraformConfig) Copy() *TerraformConfig {
	if c == nil {
		return nil
	}
	return &TerraformConfig{
		Enabled: c.Enabled,
		Backend: c.Backend,
	}
}
