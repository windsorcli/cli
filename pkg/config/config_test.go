package config

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config/aws"
	"github.com/windsorcli/cli/pkg/config/cluster"
	"github.com/windsorcli/cli/pkg/config/dns"
	"github.com/windsorcli/cli/pkg/config/docker"
	"github.com/windsorcli/cli/pkg/config/git"
	"github.com/windsorcli/cli/pkg/config/terraform"
	"github.com/windsorcli/cli/pkg/config/vm"
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
		}

		copy := original.Copy()

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

		copy := original.Copy()

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
		copy := original.Copy()
		if copy != nil {
			t.Errorf("Expected nil copy for nil original, got %v", copy)
		}
	})
}
