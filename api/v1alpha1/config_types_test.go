package v1alpha1

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1/aws"
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
			Blueprint: ptrString("1.0.0"),
		}

		overlay := &Context{
			AWS: &aws.AWSConfig{
				AWSEndpointURL: ptrString("https://overlay.aws.endpoint"),
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
			Blueprint: ptrString("2.0.0"),
		}

		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://overlay.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://overlay.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
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
		if len(base.Environment) != 2 || base.Environment["KEY1"] != "value1" || base.Environment["KEY2"] != "value2" {
			t.Errorf("Environment merge mismatch: expected map with 'KEY1' and 'KEY2', got %v", base.Environment)
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "10.0.0.0/8" {
			t.Errorf("Network CIDRBlock mismatch: expected '10.0.0.0/8', got '%s'", *base.Network.CIDRBlock)
		}
		if base.Blueprint == nil || *base.Blueprint != "2.0.0" {
			t.Errorf("Blueprint mismatch: expected '2.0.0', got '%s'", *base.Blueprint)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &Context{
			AWS: &aws.AWSConfig{
				Enabled:        ptrBool(true),
				AWSEndpointURL: ptrString("https://base.aws.endpoint"),
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
			Blueprint: ptrString("1.0.0"),
		}

		var overlay *Context = nil
		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://base.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://base.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
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
		if base.Blueprint == nil || *base.Blueprint != "1.0.0" {
			t.Errorf("Blueprint mismatch: expected '1.0.0', got '%s'", *base.Blueprint)
		}
	})

	t.Run("MergeWithNilBase", func(t *testing.T) {
		base := &Context{}

		overlay := &Context{
			AWS: &aws.AWSConfig{
				AWSEndpointURL: ptrString("https://overlay.aws.endpoint"),
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
			Blueprint: ptrString("2.0.0"),
		}

		base.Merge(overlay)

		if base.AWS.AWSEndpointURL == nil || *base.AWS.AWSEndpointURL != "https://overlay.aws.endpoint" {
			t.Errorf("AWS AWSEndpointURL mismatch: expected 'https://overlay.aws.endpoint', got '%s'", *base.AWS.AWSEndpointURL)
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
		if base.Blueprint == nil || *base.Blueprint != "2.0.0" {
			t.Errorf("Blueprint mismatch: expected '2.0.0', got '%s'", *base.Blueprint)
		}
	})

	t.Run("MergeWithProjectName", func(t *testing.T) {
		base := &Context{
			ProjectName: ptrString("BaseProject"),
		}

		overlay := &Context{
			ProjectName: ptrString("OverlayProject"),
		}

		base.Merge(overlay)

		if base.ProjectName == nil || *base.ProjectName != "OverlayProject" {
			t.Errorf("ProjectName mismatch: expected 'OverlayProject', got '%s'", *base.ProjectName)
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
			Blueprint: ptrString("1.0.0"),
		}

		copy := original.DeepCopy()

		// Ensure all fields are deeply equal by comparing values
		if original.Environment["KEY"] != copy.Environment["KEY"] {
			t.Errorf("Environment mismatch: expected %v, got %v", original.Environment["KEY"], copy.Environment["KEY"])
		}
		if original.AWS.Enabled == nil || copy.AWS.Enabled == nil || *original.AWS.Enabled != *copy.AWS.Enabled {
			t.Errorf("AWS Enabled mismatch: expected %v, got %v", *original.AWS.Enabled, *copy.AWS.Enabled)
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
		if original.Blueprint == nil || copy.Blueprint == nil || *original.Blueprint != *copy.Blueprint {
			t.Errorf("Blueprint mismatch: expected %v, got %v", *original.Blueprint, *copy.Blueprint)
		}

		// Modify the copy and ensure original is unchanged
		copy.Docker.Enabled = ptrBool(false)
		if original.Docker.Enabled == nil || *original.Docker.Enabled == *copy.Docker.Enabled {
			t.Errorf("Original Docker Enabled was modified: expected %v, got %v", true, *copy.Docker.Enabled)
		}

		copy.Cluster.Enabled = ptrBool(false)
		if original.Cluster.Enabled == nil || *original.Cluster.Enabled == *copy.Cluster.Enabled {
			t.Errorf("Original Cluster Enabled was modified: expected %v, got %v", true, *copy.Cluster.Enabled)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &Context{
			Environment: nil,
			AWS:         nil,
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
