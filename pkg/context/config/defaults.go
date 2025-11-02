// This file defines default configurations for various components of the Windsor CLI application.
// It includes configurations for AWS, Docker, Terraform, Cluster, DNS, and VM settings.
// The configurations are structured using the v1alpha1.Context type, which aggregates settings
// from different modules like AWS, Docker, and others. The file also defines common configurations
// that can be reused across different contexts, such as commonDockerConfig, commonGitConfig, etc.
// These common configurations are used to create specific default configurations like
// DefaultConfig_Localhost and DefaultConfig_Full.

package config

import (
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/api/v1alpha1/dns"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/api/v1alpha1/git"
	"github.com/windsorcli/cli/api/v1alpha1/network"
	"github.com/windsorcli/cli/api/v1alpha1/terraform"
	"github.com/windsorcli/cli/pkg/constants"
)

// DefaultConfig returns the default configuration
var DefaultConfig = v1alpha1.Context{
	Provider:  ptrString("generic"),
	Cluster:   commonClusterConfig.Copy(),
	Terraform: commonTerraformConfig.Copy(),
	DNS:       commonDNSConfig.Copy(),
}

var commonDockerConfig = docker.DockerConfig{
	Enabled: ptrBool(true),
	Registries: map[string]docker.RegistryConfig{
		"registry.test": {},
		"registry-1.docker.io": {
			Remote: "https://registry-1.docker.io",
			Local:  "https://docker.io",
		},
		"registry.k8s.io": {
			Remote: "https://registry.k8s.io",
		},
		"gcr.io": {
			Remote: "https://gcr.io",
		},
		"ghcr.io": {
			Remote: "https://ghcr.io",
		},
		"quay.io": {
			Remote: "https://quay.io",
		},
	},
}

var commonGitConfig = git.GitConfig{
	Livereload: &git.GitLivereloadConfig{
		Enabled:      ptrBool(true),
		RsyncInclude: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_INCLUDE),
		RsyncProtect: ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_RSYNC_PROTECT),
		Username:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_USERNAME),
		Password:     ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_PASSWORD),
		WebhookUrl:   ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_WEBHOOK_URL),
		Image:        ptrString(constants.DEFAULT_GIT_LIVE_RELOAD_IMAGE),
		VerifySsl:    ptrBool(false),
	},
}

var commonTerraformConfig = terraform.TerraformConfig{
	Enabled: ptrBool(true),
	Backend: &terraform.BackendConfig{
		Type: "local",
	},
}

// commonClusterConfig_NoHostPorts is the base cluster configuration without hostports,
// used for VM drivers that use native networking (colima, docker)
var commonClusterConfig_NoHostPorts = cluster.ClusterConfig{
	Enabled: ptrBool(true),
	Driver:  ptrString("talos"),
	ControlPlanes: cluster.NodeGroupConfig{
		Count:     ptrInt(1),
		CPU:       ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_CPU),
		Memory:    ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_RAM),
		Nodes:     make(map[string]cluster.NodeConfig),
		HostPorts: []string{},
	},
	Workers: cluster.NodeGroupConfig{
		Count:     ptrInt(1),
		CPU:       ptrInt(constants.DEFAULT_TALOS_WORKER_CPU),
		Memory:    ptrInt(constants.DEFAULT_TALOS_WORKER_RAM),
		Nodes:     make(map[string]cluster.NodeConfig),
		HostPorts: []string{},
		Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/.volumes:/var/local"},
	},
}

// commonClusterConfig_WithHostPorts is the base cluster configuration with hostports,
// used for VM drivers that need port forwarding (docker-desktop)
var commonClusterConfig_WithHostPorts = cluster.ClusterConfig{
	Enabled: ptrBool(true),
	Driver:  ptrString("talos"),
	ControlPlanes: cluster.NodeGroupConfig{
		Count:     ptrInt(1),
		CPU:       ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_CPU),
		Memory:    ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_RAM),
		Nodes:     make(map[string]cluster.NodeConfig),
		HostPorts: []string{},
	},
	Workers: cluster.NodeGroupConfig{
		Count:     ptrInt(1),
		CPU:       ptrInt(constants.DEFAULT_TALOS_WORKER_CPU),
		Memory:    ptrInt(constants.DEFAULT_TALOS_WORKER_RAM),
		Nodes:     make(map[string]cluster.NodeConfig),
		HostPorts: []string{"8080:30080/tcp", "8443:30443/tcp", "9292:30292/tcp", "8053:30053/udp"},
		Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/.volumes:/var/local"},
	},
}

// Preserve the original commonClusterConfig for backwards compatibility with DefaultConfig
var commonClusterConfig = cluster.ClusterConfig{
	Enabled: ptrBool(true),
	Driver:  ptrString("talos"),
	ControlPlanes: cluster.NodeGroupConfig{
		Count:  ptrInt(1),
		CPU:    ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_CPU),
		Memory: ptrInt(constants.DEFAULT_TALOS_CONTROL_PLANE_RAM),
		Nodes:  make(map[string]cluster.NodeConfig),
	},
	Workers: cluster.NodeGroupConfig{
		Count:     ptrInt(1),
		CPU:       ptrInt(constants.DEFAULT_TALOS_WORKER_CPU),
		Memory:    ptrInt(constants.DEFAULT_TALOS_WORKER_RAM),
		Nodes:     make(map[string]cluster.NodeConfig),
		HostPorts: []string{},
		Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/.volumes:/var/local"},
	},
}

var commonDNSConfig = dns.DNSConfig{
	Enabled: ptrBool(true),
	Domain:  ptrString("test"),
}

var DefaultConfig_Localhost = v1alpha1.Context{
	Provider:    ptrString("generic"),
	Environment: map[string]string{},
	Docker:      commonDockerConfig.Copy(),
	Git:         commonGitConfig.Copy(),
	Terraform:   commonTerraformConfig.Copy(),
	Cluster:     commonClusterConfig_WithHostPorts.Copy(),
	Network: &network.NetworkConfig{
		CIDRBlock: ptrString(constants.DEFAULT_NETWORK_CIDR),
	},
	DNS: &dns.DNSConfig{
		Enabled: ptrBool(true),
		Domain:  ptrString("test"),
		Forward: []string{
			"10.5.0.1:8053",
		},
	},
}

var DefaultConfig_Full = v1alpha1.Context{
	Provider:    ptrString("generic"),
	Environment: map[string]string{},
	Docker:      commonDockerConfig.Copy(),
	Git:         commonGitConfig.Copy(),
	Terraform:   commonTerraformConfig.Copy(),
	Cluster:     commonClusterConfig_NoHostPorts.Copy(),
	Network: &network.NetworkConfig{
		CIDRBlock: ptrString(constants.DEFAULT_NETWORK_CIDR),
		LoadBalancerIPs: &struct {
			Start *string `yaml:"start,omitempty"`
			End   *string `yaml:"end,omitempty"`
		}{
			Start: ptrString("10.5.1.1"),
			End:   ptrString("10.5.1.10"),
		},
	},
	DNS: &dns.DNSConfig{
		Enabled: ptrBool(true),
		Domain:  ptrString("test"),
		Forward: []string{
			"10.5.1.1",
		},
	},
}
