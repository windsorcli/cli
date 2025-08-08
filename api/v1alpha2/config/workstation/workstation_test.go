package workstation

import (
	"reflect"
	"testing"

	clusterconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/cluster"
	dnsconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/dns"
	gitconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/git"
	localstackconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/localstack"
	networkconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/network"
	registriesconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/registries"
	vmconfig "github.com/windsorcli/cli/api/v1alpha2/config/workstation/vm"
)

// TestWorkstationConfig_Merge tests the Merge method of WorkstationConfig
func TestWorkstationConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if !reflect.DeepEqual(base, original) {
			t.Errorf("Expected no change when merging with nil overlay")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
		}
		original := base.DeepCopy()

		overlay := &WorkstationConfig{}
		base.Merge(overlay)

		if !reflect.DeepEqual(base, original) {
			t.Errorf("Expected no change when merging with empty overlay")
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
		}

		overlay := &WorkstationConfig{
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(true),
				Driver:  ptrString("talos"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(true),
				Domain:  ptrString("test.local"),
			},
		}

		base.Merge(overlay)

		if base.Cluster == nil {
			t.Errorf("Expected Cluster to be initialized")
		}
		if base.DNS == nil {
			t.Errorf("Expected DNS to be initialized")
		}
		if !*base.Cluster.Enabled {
			t.Errorf("Expected Cluster.Enabled to be true")
		}
		if *base.Cluster.Driver != "talos" {
			t.Errorf("Expected Cluster.Driver to be 'talos', got %s", *base.Cluster.Driver)
		}
		if !*base.DNS.Enabled {
			t.Errorf("Expected DNS.Enabled to be true")
		}
		if *base.DNS.Domain != "test.local" {
			t.Errorf("Expected DNS.Domain to be 'test.local', got %s", *base.DNS.Domain)
		}
	})

	t.Run("MergeWithCompleteOverlay", func(t *testing.T) {
		base := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(false),
				Driver:  ptrString("kind"),
			},
		}

		overlay := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(true),
				Driver:  ptrString("talos"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(true),
				Domain:  ptrString("test.local"),
			},
		}

		base.Merge(overlay)

		if !*base.Cluster.Enabled {
			t.Errorf("Expected Cluster.Enabled to be true after merge")
		}
		if *base.Cluster.Driver != "talos" {
			t.Errorf("Expected Cluster.Driver to be 'talos' after merge")
		}
		if base.DNS == nil {
			t.Errorf("Expected DNS to be initialized")
		}
		if !*base.DNS.Enabled {
			t.Errorf("Expected DNS.Enabled to be true")
		}
	})

	t.Run("MergeWithAllConfigTypes", func(t *testing.T) {
		base := &WorkstationConfig{}

		overlay := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{
				Enabled: ptrBool(true),
			},
			Git: &gitconfig.GitConfig{
				Livereload: &gitconfig.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
			VM: &vmconfig.VMConfig{
				Driver: ptrString("colima"),
			},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(true),
			},
			Network: &networkconfig.NetworkConfig{
				CIDRBlock: ptrString("10.0.0.0/24"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(true),
			},
			Localstack: &localstackconfig.LocalstackConfig{
				Enabled: ptrBool(true),
			},
		}

		base.Merge(overlay)

		// Verify all configs are initialized
		if base.Registries == nil {
			t.Errorf("Expected Registries to be initialized")
		}
		if base.Git == nil {
			t.Errorf("Expected Git to be initialized")
		}
		if base.VM == nil {
			t.Errorf("Expected VM to be initialized")
		}
		if base.Cluster == nil {
			t.Errorf("Expected Cluster to be initialized")
		}
		if base.Network == nil {
			t.Errorf("Expected Network to be initialized")
		}
		if base.DNS == nil {
			t.Errorf("Expected DNS to be initialized")
		}
		if base.Localstack == nil {
			t.Errorf("Expected Localstack to be initialized")
		}

		// Verify all configs have expected values
		if !*base.Registries.Enabled {
			t.Errorf("Expected Registries.Enabled to be true")
		}
		if base.Git.Livereload == nil || !*base.Git.Livereload.Enabled {
			t.Errorf("Expected Git.Livereload.Enabled to be true")
		}
		if base.VM.Driver == nil || *base.VM.Driver != "colima" {
			t.Errorf("Expected VM.Driver to be 'colima'")
		}
		if !*base.Cluster.Enabled {
			t.Errorf("Expected Cluster.Enabled to be true")
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected Network.CIDRBlock to be '10.0.0.0/24'")
		}
		if !*base.DNS.Enabled {
			t.Errorf("Expected DNS.Enabled to be true")
		}
		if !*base.Localstack.Enabled {
			t.Errorf("Expected Localstack.Enabled to be true")
		}
	})

	t.Run("MergeWithExistingConfigs", func(t *testing.T) {
		base := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{
				Enabled: ptrBool(false),
			},
			Git: &gitconfig.GitConfig{
				Livereload: &gitconfig.GitLivereloadConfig{
					Enabled: ptrBool(false),
				},
			},
			VM: &vmconfig.VMConfig{
				Driver: ptrString("docker"),
			},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(false),
			},
			Network: &networkconfig.NetworkConfig{
				CIDRBlock: ptrString("192.168.0.0/24"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(false),
			},
			Localstack: &localstackconfig.LocalstackConfig{
				Enabled: ptrBool(false),
			},
		}

		overlay := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{
				Enabled: ptrBool(true),
			},
			Git: &gitconfig.GitConfig{
				Livereload: &gitconfig.GitLivereloadConfig{
					Enabled: ptrBool(true),
				},
			},
			VM: &vmconfig.VMConfig{
				Driver: ptrString("colima"),
			},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(true),
			},
			Network: &networkconfig.NetworkConfig{
				CIDRBlock: ptrString("10.0.0.0/24"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(true),
			},
			Localstack: &localstackconfig.LocalstackConfig{
				Enabled: ptrBool(true),
			},
		}

		base.Merge(overlay)

		// Verify all configs are updated
		if !*base.Registries.Enabled {
			t.Errorf("Expected Registries.Enabled to be true after merge")
		}
		if base.Git.Livereload == nil || !*base.Git.Livereload.Enabled {
			t.Errorf("Expected Git.Livereload.Enabled to be true after merge")
		}
		if base.VM.Driver == nil || *base.VM.Driver != "colima" {
			t.Errorf("Expected VM.Driver to be 'colima' after merge")
		}
		if !*base.Cluster.Enabled {
			t.Errorf("Expected Cluster.Enabled to be true after merge")
		}
		if base.Network.CIDRBlock == nil || *base.Network.CIDRBlock != "10.0.0.0/24" {
			t.Errorf("Expected Network.CIDRBlock to be '10.0.0.0/24' after merge")
		}
		if !*base.DNS.Enabled {
			t.Errorf("Expected DNS.Enabled to be true after merge")
		}
		if !*base.Localstack.Enabled {
			t.Errorf("Expected Localstack.Enabled to be true after merge")
		}
	})
}

// TestWorkstationConfig_Copy tests the Copy method of WorkstationConfig
func TestWorkstationConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *WorkstationConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Errorf("Expected nil when copying nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &WorkstationConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy of empty config")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
			Cluster: &clusterconfig.ClusterConfig{
				Enabled: ptrBool(true),
				Driver:  ptrString("talos"),
			},
			DNS: &dnsconfig.DNSConfig{
				Enabled: ptrBool(true),
				Domain:  ptrString("test.local"),
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}

		// Verify deep copy by modifying original
		*config.Cluster.Enabled = false
		if !*copied.Cluster.Enabled {
			t.Errorf("Expected copy to be independent of original")
		}
	})

	t.Run("CopyWithAllFieldsPopulated", func(t *testing.T) {
		config := &WorkstationConfig{
			Registries: &registriesconfig.RegistriesConfig{},
			Git:        &gitconfig.GitConfig{},
			VM:         &vmconfig.VMConfig{},
			Cluster:    &clusterconfig.ClusterConfig{},
			Network:    &networkconfig.NetworkConfig{},
			DNS:        &dnsconfig.DNSConfig{},
			Localstack: &localstackconfig.LocalstackConfig{},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})
}

// Helper function for creating string pointers
func ptrString(s string) *string {
	return &s
}
