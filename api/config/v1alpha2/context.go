// The Context is the typed header of a v1alpha2 context configuration.
// It provides the settings the CLI reads from values.yaml, and the schema fragment that types them.
// Keys that the header does not define stay untyped and pass through to the blueprint.
// The field order of Context is the order that the CLI writes header keys.

package v1alpha2

import (
	_ "embed"
	"slices"
)

// =============================================================================
// Constants
// =============================================================================

// Platforms lists the platforms the CLI supports, in the order the schema enum lists them.
var Platforms = []string{"none", "docker", "incus", "metal", "hetzner", "aws", "azure", "gcp", "hyperv", "vsphere"}

//go:embed schema.yaml
var schema []byte

// =============================================================================
// Types
// =============================================================================

// Context holds the settings the CLI reads from a context.
type Context struct {
	ID          string            `yaml:"id,omitempty"`
	Platform    string            `yaml:"platform,omitempty"`
	Dev         *bool             `yaml:"dev,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Secrets     *Secrets          `yaml:"secrets,omitempty"`
	AWS         *AWS              `yaml:"aws,omitempty"`
	Azure       *Azure            `yaml:"azure,omitempty"`
	GCP         *GCP              `yaml:"gcp,omitempty"`
	VSphere     *VSphere          `yaml:"vsphere,omitempty"`
	Terraform   *Terraform        `yaml:"terraform,omitempty"`
	Workstation *Workstation      `yaml:"workstation,omitempty"`
	DNS         *DNS              `yaml:"dns,omitempty"`
	Network     *Network          `yaml:"network,omitempty"`
	Cluster     *Cluster          `yaml:"cluster,omitempty"`
	Docker      *Docker           `yaml:"docker,omitempty"`
	GitOps      *GitOps           `yaml:"gitops,omitempty"`
}

// Secrets holds the secret providers the CLI reads.
type Secrets struct {
	SOPS        *SOPS        `yaml:"sops,omitempty"`
	OnePassword *OnePassword `yaml:"onepassword,omitempty"`
}

// SOPS turns SOPS decryption on for the context.
type SOPS struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// OnePassword holds the 1Password vaults, keyed by the alias that secret() references.
type OnePassword struct {
	Vaults map[string]OnePasswordVault `yaml:"vaults,omitempty"`
}

// OnePasswordVault identifies one 1Password vault.
type OnePasswordVault struct {
	ID   *string `yaml:"id,omitempty"`
	Name *string `yaml:"name,omitempty"`
	URL  *string `yaml:"url,omitempty"`
}

// AWS holds the AWS settings that the CLI exports and checks.
type AWS struct {
	Profile      *string `yaml:"profile,omitempty"`
	Region       *string `yaml:"region,omitempty"`
	EndpointURL  *string `yaml:"endpoint_url,omitempty"`
	MWAAEndpoint *string `yaml:"mwaa_endpoint,omitempty"`
	S3Hostname   *string `yaml:"s3_hostname,omitempty"`
}

// Azure holds the Azure settings that the CLI exports and checks.
type Azure struct {
	Environment    *string `yaml:"environment,omitempty"`
	KubeloginMode  *string `yaml:"kubelogin_mode,omitempty"`
	Region         *string `yaml:"region,omitempty"`
	SubscriptionID *string `yaml:"subscription_id,omitempty"`
	TenantID       *string `yaml:"tenant_id,omitempty"`
}

// GCP holds the GCP settings that the CLI exports.
type GCP struct {
	CredentialsPath *string `yaml:"credentials_path,omitempty"`
	ProjectID       *string `yaml:"project_id,omitempty"`
}

// VSphere holds the vCenter connection settings that the CLI exports.
type VSphere struct {
	Server   *string `yaml:"server,omitempty"`
	User     *string `yaml:"user,omitempty"`
	Insecure *bool   `yaml:"insecure,omitempty"`
}

// Terraform holds the terraform settings the CLI reads.
type Terraform struct {
	Enabled *bool             `yaml:"enabled,omitempty"`
	Backend *TerraformBackend `yaml:"backend,omitempty"`
	Lock    *TerraformLock    `yaml:"lock,omitempty"`
}

// TerraformBackend holds the backend type and the backend fields the CLI reads. The CLI passes
// each backend block to terraform as a whole, so the remaining fields stay untyped.
type TerraformBackend struct {
	Type       *string         `yaml:"type,omitempty"`
	Prefix     *string         `yaml:"prefix,omitempty"`
	S3         *S3Backend      `yaml:"s3,omitempty"`
	AzureRM    *AzureRMBackend `yaml:"azurerm,omitempty"`
	GCS        *GCSBackend     `yaml:"gcs,omitempty"`
	Kubernetes map[string]any  `yaml:"kubernetes,omitempty"`
}

// S3Backend holds the S3 backend fields the CLI reads.
type S3Backend struct {
	Bucket *string `yaml:"bucket,omitempty"`
}

// AzureRMBackend holds the azurerm backend fields the CLI reads.
type AzureRMBackend struct {
	ContainerName      *string `yaml:"container_name,omitempty"`
	StorageAccountName *string `yaml:"storage_account_name,omitempty"`
}

// GCSBackend holds the GCS backend fields the CLI reads.
type GCSBackend struct {
	Bucket *string `yaml:"bucket,omitempty"`
}

// TerraformLock holds the stack lock settings.
type TerraformLock struct {
	Timeout *string `yaml:"timeout,omitempty"`
}

// Workstation holds the local workstation settings the CLI reads.
type Workstation struct {
	Runtime *string         `yaml:"runtime,omitempty"`
	Address *string         `yaml:"address,omitempty"`
	Arch    *string         `yaml:"arch,omitempty"`
	DNS     *WorkstationDNS `yaml:"dns,omitempty"`
}

// WorkstationDNS holds the workstation DNS server address.
type WorkstationDNS struct {
	Address *string `yaml:"address,omitempty"`
}

// DNS holds the DNS settings the CLI reads.
type DNS struct {
	Domain  *string `yaml:"domain,omitempty"`
	Enabled *bool   `yaml:"enabled,omitempty"`
}

// Network holds the network settings the CLI reads.
type Network struct {
	CIDRBlock       *string  `yaml:"cidr_block,omitempty"`
	LoadBalancerIPs *IPRange `yaml:"loadbalancer_ips,omitempty"`
}

// IPRange is an inclusive range of IPv4 addresses.
type IPRange struct {
	Start *string `yaml:"start,omitempty"`
	End   *string `yaml:"end,omitempty"`
}

// Cluster holds the cluster settings the CLI reads.
type Cluster struct {
	Driver *string `yaml:"driver,omitempty"`
}

// Docker holds the docker settings the CLI reads.
type Docker struct {
	Enabled     *bool   `yaml:"enabled,omitempty"`
	RegistryURL *string `yaml:"registry_url,omitempty"`
}

// GitOps holds the GitOps settings the CLI reads.
type GitOps struct {
	Mode      *string `yaml:"mode,omitempty"`
	Namespace *string `yaml:"namespace,omitempty"`
}

// =============================================================================
// Public Methods
// =============================================================================

// Schema returns a copy of the embedded schema fragment that types the header.
func Schema() []byte {
	return slices.Clone(schema)
}
