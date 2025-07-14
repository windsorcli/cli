package v1alpha1

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/azure"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/api/v1alpha1/dns"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/api/v1alpha1/git"
	"github.com/windsorcli/cli/api/v1alpha1/network"
	"github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/api/v1alpha1/vm"
)

func TestConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &Context{
			AWS: &aws.AWSConfig{
				Enabled:        ptrBool(true),
				AWSEndpointURL: ptrString("https://base.aws.endpoint"),
			},
			Azure: &azure.AzureConfig{
				Enabled:        ptrBool(true),
				SubscriptionID: ptrString("base-sub"),
				TenantID:       ptrString("base-tenant"),
				Environment:    ptrString("base-cloud"),
			},
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(true),
			},
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
			Terraform: &terraform.TerraformConfig{
				Enabled: ptrBool(true),
			},
			VM: &vm.VMConfig{
				Address: ptrString("192.168.1.1"),
			},
			Cluster: &cluster.ClusterConfig{
				Enabled: ptrBool(true),
			},
			DNS: &dns.DNSConfig{
				Enabled: ptrBool(true),
			},
			Secrets: &secrets.SecretsConfig{
				OnePasswordConfig: secrets.OnePasswordConfig{
					Vaults: map[string]secrets.OnePasswordVault{
						"vault1": {URL: "https://url.com", Name: "Vault"},
					},
				},
			},
			Environment: map[string]string{
				"KEY1": "value1",
			},
			Network: &network.NetworkConfig{
				CIDRBlock: ptrString("192.168.0.0/16"),
			},
			Platform: ptrString("aws"),
		}

		overlay := &Context{
			AWS: &aws.AWSConfig{
				AWSEndpointURL: ptrString("https://overlay.aws.endpoint"),
			},
			Azure: &azure.AzureConfig{
				Enabled:        ptrBool(false),
				SubscriptionID: ptrString("overlay-sub"),
				TenantID:       ptrString("overlay-tenant"),
				Environment:    ptrString("overlay-cloud"),
			},
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(false),
			},
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(false),
				},
			},
			Terraform: &terraform.TerraformConfig{
				Enabled: ptrBool(false),
			},
			VM: &vm.VMConfig{
				Address: ptrString("192.168.1.2"),
			},
			Cluster: &cluster.ClusterConfig{
				Enabled: ptrBool(false),
			},
			DNS: &dns.DNSConfig{
				Enabled: ptrBool(false),
			},
			Secrets: &secrets.SecretsConfig{
				OnePasswordConfig: secrets.OnePasswordConfig{
					Vaults: map[string]secrets.OnePasswordVault{
						"vault1": {URL: "https://url.com", Name: "Vault"},
					},
				},
			},
			Environment: map[string]string{
				"KEY2": "value2",
			},
			Network: &network.NetworkConfig{
				CIDRBlock: ptrString("10.0.0.0/8"),
			},
			Platform: ptrString("azure"),
		}

		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://overlay.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://overlay.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
		}
		if base.Azure.Enabled == nil || *base.Azure.Enabled != false {
			t.Errorf("Azure Enabled mismatch: expected false, got %v", *base.Azure.Enabled)
		}
		if base.Azure.SubscriptionID == nil || *base.Azure.SubscriptionID != "overlay-sub" {
			t.Errorf("Azure SubscriptionID mismatch: expected 'overlay-sub', got '%s'", *base.Azure.SubscriptionID)
		}
		if base.Azure.TenantID == nil || *base.Azure.TenantID != "overlay-tenant" {
			t.Errorf("Azure TenantID mismatch: expected 'overlay-tenant', got '%s'", *base.Azure.TenantID)
		}
		if base.Azure.Environment == nil || *base.Azure.Environment != "overlay-cloud" {
			t.Errorf("Azure Environment mismatch: expected 'overlay-cloud', got '%s'", *base.Azure.Environment)
		}
		if base.Docker.Enabled == nil || *base.Docker.Enabled != false {
			t.Errorf("Docker Enabled mismatch: expected false, got %v", *base.Docker.Enabled)
		}
		if base.Git.Livereload.Enabled == nil || *base.Git.Livereload.Enabled != false {
			t.Errorf("Git Livereload Enabled mismatch: expected false, got %v", *base.Git.Livereload.Enabled)
		}
		if base.Terraform.Enabled == nil || *base.Terraform.Enabled != false {
			t.Errorf("Terraform Enabled mismatch: expected false, got %v", *base.Terraform.Enabled)
		}
		if base.VM.Address == nil || *base.VM.Address != "192.168.1.2" {
			t.Errorf("VM Address mismatch: expected '192.168.1.2', got '%s'", *base.VM.Address)
		}
		if base.Cluster.Enabled == nil || *base.Cluster.Enabled != false {
			t.Errorf("Cluster Enabled mismatch: expected false, got %v", *base.Cluster.Enabled)
		}
		if base.DNS.Enabled == nil || *base.DNS.Enabled != false {
			t.Errorf("DNS Enabled mismatch: expected false, got %v", *base.DNS.Enabled)
		}
		if base.Secrets.OnePasswordConfig.Vaults["vault1"].URL != "https://url.com" {
			t.Errorf("Secrets Vault URL mismatch: expected 'https://url.com', got '%s'", base.Secrets.OnePasswordConfig.Vaults["vault1"].URL)
		}
		if len(base.Environment) != 2 || base.Environment["KEY2"] != "value2" {
			t.Errorf("Environment mismatch: expected map with 'KEY2', got %v", base.Environment)
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "10.0.0.0/8" {
			t.Errorf("Network CIDRBlock mismatch: expected '10.0.0.0/8', got '%s'", *base.Network.CIDRBlock)
		}
		if base.Platform == nil || *base.Platform != "azure" {
			t.Errorf("Platform mismatch: expected 'azure', got '%s'", *base.Platform)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &Context{
			AWS: &aws.AWSConfig{
				Enabled:        ptrBool(true),
				AWSEndpointURL: ptrString("https://base.aws.endpoint"),
			},
			Azure: &azure.AzureConfig{
				Enabled:        ptrBool(true),
				SubscriptionID: ptrString("base-sub"),
				TenantID:       ptrString("base-tenant"),
				Environment:    ptrString("base-cloud"),
			},
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(true),
			},
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
			Terraform: &terraform.TerraformConfig{
				Enabled: ptrBool(true),
			},
			VM: &vm.VMConfig{
				Address: ptrString("192.168.1.1"),
			},
			Cluster: &cluster.ClusterConfig{
				Enabled: ptrBool(true),
			},
			DNS: &dns.DNSConfig{
				Enabled: ptrBool(true),
			},
			Secrets: &secrets.SecretsConfig{
				OnePasswordConfig: secrets.OnePasswordConfig{
					Vaults: map[string]secrets.OnePasswordVault{
						"vault1": {URL: "https://url.com", Name: "Vault"},
					},
				},
			},
			Environment: map[string]string{
				"KEY1": "value1",
			},
			Network: &network.NetworkConfig{
				CIDRBlock: ptrString("192.168.0.0/16"),
			},
			Platform: ptrString("aws"),
		}

		var overlay *Context = nil
		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://base.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://base.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
		}
		if base.Azure.Enabled == nil || *base.Azure.Enabled != true {
			t.Errorf("Azure Enabled mismatch: expected true, got %v", *base.Azure.Enabled)
		}
		if base.Azure.SubscriptionID == nil || *base.Azure.SubscriptionID != "base-sub" {
			t.Errorf("Azure SubscriptionID mismatch: expected 'base-sub', got '%s'", *base.Azure.SubscriptionID)
		}
		if base.Azure.TenantID == nil || *base.Azure.TenantID != "base-tenant" {
			t.Errorf("Azure TenantID mismatch: expected 'base-tenant', got '%s'", *base.Azure.TenantID)
		}
		if base.Azure.Environment == nil || *base.Azure.Environment != "base-cloud" {
			t.Errorf("Azure Environment mismatch: expected 'base-cloud', got '%s'", *base.Azure.Environment)
		}
		if base.Docker.Enabled == nil || *base.Docker.Enabled != true {
			t.Errorf("Docker Enabled mismatch: expected true, got %v", *base.Docker.Enabled)
		}
		if base.Git.Livereload.Enabled == nil || *base.Git.Livereload.Enabled != true {
			t.Errorf("Git Livereload Enabled mismatch: expected true, got %v", *base.Git.Livereload.Enabled)
		}
		if base.Terraform.Enabled == nil || *base.Terraform.Enabled != true {
			t.Errorf("Terraform Enabled mismatch: expected true, got %v", *base.Terraform.Enabled)
		}
		if base.VM.Address == nil || *base.VM.Address != "192.168.1.1" {
			t.Errorf("VM Address mismatch: expected '192.168.1.1', got '%s'", *base.VM.Address)
		}
		if base.Cluster.Enabled == nil || *base.Cluster.Enabled != true {
			t.Errorf("Cluster Enabled mismatch: expected true, got %v", *base.Cluster.Enabled)
		}
		if base.DNS.Enabled == nil || *base.DNS.Enabled != true {
			t.Errorf("DNS Enabled mismatch: expected true, got %v", *base.DNS.Enabled)
		}
		if base.Secrets.OnePasswordConfig.Vaults["vault1"].URL != "https://url.com" {
			t.Errorf("Secrets Vault URL mismatch: expected 'https://url.com', got '%s'", base.Secrets.OnePasswordConfig.Vaults["vault1"].URL)
		}
		if len(base.Environment) != 1 || base.Environment["KEY1"] != "value1" {
			t.Errorf("Environment mismatch: expected map with 'KEY1', got %v", base.Environment)
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "192.168.0.0/16" {
			t.Errorf("Network CIDRBlock mismatch: expected '192.168.0.0/16', got '%s'", *base.Network.CIDRBlock)
		}
		if base.Platform == nil || *base.Platform != "aws" {
			t.Errorf("Platform mismatch: expected 'aws', got '%s'", *base.Platform)
		}
	})

	t.Run("MergeWithNilBase", func(t *testing.T) {
		base := &Context{}

		overlay := &Context{
			AWS: &aws.AWSConfig{
				AWSEndpointURL: ptrString("https://overlay.aws.endpoint"),
			},
			Azure: &azure.AzureConfig{
				Enabled:        ptrBool(false),
				SubscriptionID: ptrString("overlay-sub"),
				TenantID:       ptrString("overlay-tenant"),
				Environment:    ptrString("overlay-cloud"),
			},
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(false),
			},
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(false),
				},
			},
			Terraform: &terraform.TerraformConfig{
				Enabled: ptrBool(false),
			},
			VM: &vm.VMConfig{
				Address: ptrString("192.168.1.2"),
			},
			Cluster: &cluster.ClusterConfig{
				Enabled: ptrBool(false),
			},
			DNS: &dns.DNSConfig{
				Enabled: ptrBool(false),
			},
			Secrets: &secrets.SecretsConfig{
				OnePasswordConfig: secrets.OnePasswordConfig{
					Vaults: map[string]secrets.OnePasswordVault{
						"vault1": {URL: "https://url.com", Name: "Vault"},
					},
				},
			},
			Environment: map[string]string{
				"KEY2": "value2",
			},
			Network: &network.NetworkConfig{
				CIDRBlock: ptrString("10.0.0.0/8"),
			},
			Platform: ptrString("azure"),
		}

		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://overlay.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://overlay.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
		}
		if base.Azure.Enabled == nil || *base.Azure.Enabled != false {
			t.Errorf("Azure Enabled mismatch: expected false, got %v", *base.Azure.Enabled)
		}
		if base.Azure.SubscriptionID == nil || *base.Azure.SubscriptionID != "overlay-sub" {
			t.Errorf("Azure SubscriptionID mismatch: expected 'overlay-sub', got '%s'", *base.Azure.SubscriptionID)
		}
		if base.Azure.TenantID == nil || *base.Azure.TenantID != "overlay-tenant" {
			t.Errorf("Azure TenantID mismatch: expected 'overlay-tenant', got '%s'", *base.Azure.TenantID)
		}
		if base.Azure.Environment == nil || *base.Azure.Environment != "overlay-cloud" {
			t.Errorf("Azure Environment mismatch: expected 'overlay-cloud', got '%s'", *base.Azure.Environment)
		}
		if base.Docker.Enabled == nil || *base.Docker.Enabled != false {
			t.Errorf("Docker Enabled mismatch: expected false, got %v", *base.Docker.Enabled)
		}
		if base.Git.Livereload.Enabled == nil || *base.Git.Livereload.Enabled != false {
			t.Errorf("Git Livereload Enabled mismatch: expected false, got %v", *base.Git.Livereload.Enabled)
		}
		if base.Terraform.Enabled == nil || *base.Terraform.Enabled != false {
			t.Errorf("Terraform Enabled mismatch: expected false, got %v", *base.Terraform.Enabled)
		}
		if base.VM.Address == nil || *base.VM.Address != "192.168.1.2" {
			t.Errorf("VM Address mismatch: expected '192.168.1.2', got '%s'", *base.VM.Address)
		}
		if base.Cluster.Enabled == nil || *base.Cluster.Enabled != false {
			t.Errorf("Cluster Enabled mismatch: expected false, got %v", *base.Cluster.Enabled)
		}
		if base.DNS.Enabled == nil || *base.DNS.Enabled != false {
			t.Errorf("DNS Enabled mismatch: expected false, got %v", *base.DNS.Enabled)
		}
		if base.Secrets.OnePasswordConfig.Vaults["vault1"].URL != "https://url.com" {
			t.Errorf("Secrets Vault URL mismatch: expected 'https://url.com', got '%s'", base.Secrets.OnePasswordConfig.Vaults["vault1"].URL)
		}
		if len(base.Environment) != 1 || base.Environment["KEY2"] != "value2" {
			t.Errorf("Environment mismatch: expected map with 'KEY2', got %v", base.Environment)
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "10.0.0.0/8" {
			t.Errorf("Network CIDRBlock mismatch: expected '10.0.0.0/8', got '%s'", *base.Network.CIDRBlock)
		}
		if base.Platform == nil || *base.Platform != "azure" {
			t.Errorf("Platform mismatch: expected 'azure', got '%s'", *base.Platform)
		}
	})

	t.Run("MergeWithID", func(t *testing.T) {
		base := &Context{
			ID: ptrString("base-id"),
		}

		overlay := &Context{
			ID: ptrString("overlay-id"),
		}

		base.Merge(overlay)

		if base.ID == nil || *base.ID != "overlay-id" {
			t.Errorf("ID mismatch: expected 'overlay-id', got '%s'", *base.ID)
		}
	})
}

func TestConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &Context{
			Environment: map[string]string{
				"KEY": "value",
			},
			AWS: &aws.AWSConfig{
				Enabled:        ptrBool(true),
				AWSEndpointURL: ptrString("https://original.aws.endpoint"),
			},
			Azure: &azure.AzureConfig{
				Enabled:        ptrBool(true),
				SubscriptionID: ptrString("original-sub"),
				TenantID:       ptrString("original-tenant"),
				Environment:    ptrString("original-cloud"),
			},
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(true),
			},
			Git: &git.GitConfig{
				Livereload: &git.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
			Terraform: &terraform.TerraformConfig{
				Enabled: ptrBool(true),
			},
			VM: &vm.VMConfig{
				Address: ptrString("192.168.1.1"),
			},
			Cluster: &cluster.ClusterConfig{
				Enabled: ptrBool(true),
			},
			DNS: &dns.DNSConfig{
				Enabled: ptrBool(true),
			},
			Network: &network.NetworkConfig{
				CIDRBlock: ptrString("192.168.0.0/16"),
				LoadBalancerIPs: &struct {
					Start *string `yaml:"start,omitempty"`
					End   *string `yaml:"end,omitempty"`
				}{
					Start: ptrString("192.168.0.1"),
					End:   ptrString("192.168.0.255"),
				},
			},
			Platform: ptrString("local"),
		}

		copy := original.DeepCopy()

		// Ensure all fields are deeply equal by comparing values
		if original.Environment["KEY"] != copy.Environment["KEY"] {
			t.Errorf("Environment mismatch: expected %v, got %v", original.Environment["KEY"], copy.Environment["KEY"])
		}
		if original.AWS.Enabled == nil || copy.AWS.Enabled == nil || *original.AWS.Enabled != *copy.AWS.Enabled {
			t.Errorf("AWS Enabled mismatch: expected %v, got %v", *original.AWS.Enabled, *copy.AWS.Enabled)
		}
		if original.Azure.Enabled == nil || copy.Azure.Enabled == nil || *original.Azure.Enabled != *copy.Azure.Enabled {
			t.Errorf("Azure Enabled mismatch: expected %v, got %v", *original.Azure.Enabled, *copy.Azure.Enabled)
		}
		if original.Azure.SubscriptionID == nil || copy.Azure.SubscriptionID == nil || *original.Azure.SubscriptionID != *copy.Azure.SubscriptionID {
			t.Errorf("Azure SubscriptionID mismatch: expected %v, got %v", *original.Azure.SubscriptionID, *copy.Azure.SubscriptionID)
		}
		if original.Azure.TenantID == nil || copy.Azure.TenantID == nil || *original.Azure.TenantID != *copy.Azure.TenantID {
			t.Errorf("Azure TenantID mismatch: expected %v, got %v", *original.Azure.TenantID, *copy.Azure.TenantID)
		}
		if original.Azure.Environment == nil || copy.Azure.Environment == nil || *original.Azure.Environment != *copy.Azure.Environment {
			t.Errorf("Azure Environment mismatch: expected %v, got %v", *original.Azure.Environment, *copy.Azure.Environment)
		}
		if original.Docker.Enabled == nil || copy.Docker.Enabled == nil || *original.Docker.Enabled != *copy.Docker.Enabled {
			t.Errorf("Docker Enabled mismatch: expected %v, got %v", *original.Docker.Enabled, *copy.Docker.Enabled)
		}
		if original.Git.Livereload.Enabled == nil || copy.Git.Livereload.Enabled == nil || *original.Git.Livereload.Enabled != *copy.Git.Livereload.Enabled {
			t.Errorf("Git Livereload Enabled mismatch: expected %v, got %v", *original.Git.Livereload.Enabled, *copy.Git.Livereload.Enabled)
		}
		if original.Terraform.Enabled == nil || copy.Terraform.Enabled == nil || *original.Terraform.Enabled != *copy.Terraform.Enabled {
			t.Errorf("Terraform Enabled mismatch: expected %v, got %v", *original.Terraform.Enabled, *copy.Terraform.Enabled)
		}
		if original.VM.Address == nil || copy.VM.Address == nil || *original.VM.Address != *copy.VM.Address {
			t.Errorf("VM Address mismatch: expected %v, got %v", *original.VM.Address, *copy.VM.Address)
		}
		if original.Cluster.Enabled == nil || copy.Cluster.Enabled == nil || *original.Cluster.Enabled != *copy.Cluster.Enabled {
			t.Errorf("Cluster Enabled mismatch: expected %v, got %v", *original.Cluster.Enabled, *copy.Cluster.Enabled)
		}
		if original.DNS.Enabled == nil || copy.DNS.Enabled == nil || *original.DNS.Enabled != *copy.DNS.Enabled {
			t.Errorf("DNS Enabled mismatch: expected %v, got %v", *original.DNS.Enabled, *copy.DNS.Enabled)
		}
		if original.Network.CIDRBlock == nil || copy.Network.CIDRBlock == nil || *original.Network.CIDRBlock != *copy.Network.CIDRBlock {
			t.Errorf("Network CIDRBlock mismatch: expected %v, got %v", *original.Network.CIDRBlock, *copy.Network.CIDRBlock)
		}
		if original.Platform == nil || copy.Platform == nil || *original.Platform != *copy.Platform {
			t.Errorf("Platform mismatch: expected %v, got %v", *original.Platform, *copy.Platform)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &Context{
			Environment: nil,
			AWS:         nil,
			Azure:       nil,
			Docker:      nil,
			Git:         nil,
			Terraform:   nil,
			VM:          nil,
			Cluster:     nil,
			DNS:         nil,
		}

		copy := original.DeepCopy()

		if copy.Environment != nil {
			t.Errorf("Environment should be nil, got %v", copy.Environment)
		}
		if copy.AWS != nil {
			t.Errorf("AWS should be nil, got %v", copy.AWS)
		}
		if copy.Azure != nil {
			t.Errorf("Azure should be nil, got %v", copy.Azure)
		}
		if copy.Docker != nil {
			t.Errorf("Docker should be nil, got %v", copy.Docker)
		}
		if copy.Git != nil {
			t.Errorf("Git should be nil, got %v", copy.Git)
		}
		if copy.Terraform != nil {
			t.Errorf("Terraform should be nil, got %v", copy.Terraform)
		}
		if copy.VM != nil {
			t.Errorf("VM should be nil, got %v", copy.VM)
		}
		if copy.Cluster != nil {
			t.Errorf("Cluster should be nil, got %v", copy.Cluster)
		}
		if copy.DNS != nil {
			t.Errorf("DNS should be nil, got %v", copy.DNS)
		}
	})

	t.Run("CopyNilContext", func(t *testing.T) {
		var original *Context = nil
		copy := original.DeepCopy()
		if copy != nil {
			t.Errorf("Expected nil copy for nil original, got %v", copy)
		}
	})
}
